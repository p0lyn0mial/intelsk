package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type CamerasHandler struct {
	svc        *services.CameraService
	cfg        *config.AppConfig
	mlClient   *services.MLClient
	storage    *services.Storage
	settings   *services.SettingsService
	streamer   *services.Streamer
	mu         sync.Mutex
	uploadJobs map[string]*uploadJob
}

type uploadJob struct {
	Events []uploadJobEvent
	doneCh chan struct{}
}

type uploadJobEvent struct {
	Stage       string `json:"stage"`                  // "transcoding", "done", "extracting", "indexing", "complete"
	File        string `json:"file,omitempty"`
	Current     int    `json:"current,omitempty"`
	Total       int    `json:"total,omitempty"`
	FramesDone  int    `json:"frames_done,omitempty"`
	FramesTotal int    `json:"frames_total,omitempty"`
}

func NewCamerasHandler(svc *services.CameraService, cfg *config.AppConfig, mlClient *services.MLClient, storage *services.Storage, settings *services.SettingsService, streamer *services.Streamer) *CamerasHandler {
	return &CamerasHandler{
		svc:        svc,
		cfg:        cfg,
		mlClient:   mlClient,
		storage:    storage,
		settings:   settings,
		streamer:   streamer,
		uploadJobs: make(map[string]*uploadJob),
	}
}

func (h *CamerasHandler) List(w http.ResponseWriter, r *http.Request) {
	cameras, err := h.svc.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cameras)
}

func (h *CamerasHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cam, err := h.svc.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CamerasHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cam, err := h.svc.Create(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, cam)
}

func (h *CamerasHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req models.UpdateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cam, err := h.svc.Update(id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CamerasHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	deleteData := r.URL.Query().Get("delete_data") == "true"

	if err := h.svc.Delete(id, deleteData); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *CamerasHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stats, err := h.svc.Stats(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *CamerasHandler) ListVideos(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	videos, err := h.svc.ListVideos(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, videos)
}

func (h *CamerasHandler) CleanData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope := r.URL.Query().Get("scope")
	if scope != "videos" && scope != "all" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be 'videos' or 'all'"})
		return
	}
	if err := h.svc.CleanData(id, scope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.svc.InvalidateThumbnail(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned"})
}

func (h *CamerasHandler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	date := r.URL.Query().Get("date")
	filename := r.URL.Query().Get("filename")
	if date == "" || filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date and filename are required"})
		return
	}
	if err := h.svc.DeleteVideo(id, date, filename); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.svc.InvalidateThumbnail(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *CamerasHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 32 MB memory limit; rest spills to disk
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files provided"})
		return
	}

	var paths []string
	for _, fh := range files {
		// Skip non-.mp4 files
		if strings.ToLower(filepath.Ext(fh.Filename)) != ".mp4" {
			continue
		}

		file, err := fh.Open()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
			return
		}

		path, err := h.svc.Upload(id, file, fh.Filename)
		file.Close()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no .mp4 files found in upload"})
		return
	}

	h.svc.InvalidateThumbnail(id)

	cam, err := h.svc.Get(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "uploaded", "paths": paths})
		return
	}

	shouldTranscode := services.ShouldTranscode(cam.Config)
	shouldProcess := services.ShouldProcessOnUpload(cam.Config)

	// Probe each file for HEVC codec (only if transcoding enabled)
	var hevcPaths []string
	if shouldTranscode {
		for _, p := range paths {
			codec, err := services.ProbeVideoCodec(p)
			if err != nil {
				log.Printf("Probe warning for %s: %v", p, err)
				continue
			}
			if codec == "hevc" {
				hevcPaths = append(hevcPaths, p)
			}
		}
	}

	needsTranscode := len(hevcPaths) > 0
	if !needsTranscode && !shouldProcess {
		writeJSON(w, http.StatusOK, map[string]any{"status": "uploaded", "paths": paths})
		return
	}

	// Start background upload job (transcode + extract + index)
	jobID := uuid.New().String()
	job := &uploadJob{
		doneCh: make(chan struct{}),
	}

	h.mu.Lock()
	h.uploadJobs[jobID] = job
	h.mu.Unlock()

	go h.runUploadJob(job, id, paths, hevcPaths, shouldProcess)

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "uploaded",
		"paths":  paths,
		"job_id": jobID,
	})
}

func (h *CamerasHandler) runUploadJob(job *uploadJob, cameraID string, allPaths, hevcPaths []string, shouldProcess bool) {
	defer close(job.doneCh)

	// Phase 1: Transcode HEVC files
	if len(hevcPaths) > 0 {
		total := len(hevcPaths)
		for i, p := range hevcPaths {
			current := i + 1
			h.mu.Lock()
			job.Events = append(job.Events, uploadJobEvent{
				Stage:   "transcoding",
				File:    filepath.Base(p),
				Current: current,
				Total:   total,
			})
			h.mu.Unlock()

			if err := services.TranscodeIfNeeded(p); err != nil {
				log.Printf("Transcode error for %s: %v", p, err)
			}

			h.mu.Lock()
			job.Events = append(job.Events, uploadJobEvent{
				Stage:   "done",
				File:    filepath.Base(p),
				Current: current,
				Total:   total,
			})
			h.mu.Unlock()
		}
	}

	if !shouldProcess {
		h.mu.Lock()
		job.Events = append(job.Events, uploadJobEvent{Stage: "complete"})
		h.mu.Unlock()
		return
	}

	// Phase 2: Extract frames from all uploaded files
	date := time.Now().Format("2006-01-02")
	videosDir := filepath.Join(h.cfg.App.DataDir, "videos", cameraID, date)
	framesDir := filepath.Join(h.cfg.Extraction.StoragePath, cameraID, date)

	h.mu.Lock()
	job.Events = append(job.Events, uploadJobEvent{Stage: "extracting"})
	h.mu.Unlock()

	existingFrames, _ := services.LoadManifest(framesDir)
	var newFrames []models.FrameMetadata

	for _, p := range allPaths {
		h.mu.Lock()
		job.Events = append(job.Events, uploadJobEvent{
			Stage: "extracting",
			File:  filepath.Base(p),
		})
		h.mu.Unlock()

		frames, err := services.ExtractFramesTime(
			p, framesDir,
			h.settings.GetInt("extraction.time_interval_sec"),
			h.settings.GetInt("extraction.output_quality"),
		)
		if err != nil {
			log.Printf("Extraction failed for %s: %v", p, err)
			continue
		}

		if h.settings.GetBool("extraction.dedup_enabled") {
			frames, _ = services.DeduplicateFrames(frames, h.settings.GetInt("extraction.dedup_phash_threshold"))
		}

		newFrames = append(newFrames, frames...)
	}

	allFrames := append(existingFrames, newFrames...)
	if err := services.WriteManifest(framesDir, allFrames); err != nil {
		log.Printf("Writing manifest for %s/%s: %v", cameraID, date, err)
	}

	// Phase 3: Index frames via ML pipeline
	manifest := filepath.Join(framesDir, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		log.Printf("No manifest for %s/%s after extraction", cameraID, date)
		h.mu.Lock()
		job.Events = append(job.Events, uploadJobEvent{Stage: "complete"})
		h.mu.Unlock()
		return
	}

	// Wait for ML sidecar
	if err := h.mlClient.WaitForReady(120 * time.Second); err != nil {
		log.Printf("ML sidecar not ready: %v", err)
		h.mu.Lock()
		job.Events = append(job.Events, uploadJobEvent{Stage: "complete"})
		h.mu.Unlock()
		return
	}

	h.mu.Lock()
	job.Events = append(job.Events, uploadJobEvent{Stage: "indexing"})
	h.mu.Unlock()

	pipeline := services.NewPipeline(h.mlClient, h.storage, h.settings.GetInt("clip.batch_size"))

	// Bridge pipeline progress events into upload job events
	progressCh := make(chan services.ProgressEvent, 64)
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		for ev := range progressCh {
			if ev.Stage == "indexing" {
				h.mu.Lock()
				job.Events = append(job.Events, uploadJobEvent{
					Stage:       "indexing",
					FramesDone:  ev.FramesDone,
					FramesTotal: ev.FramesTotal,
				})
				h.mu.Unlock()
			}
		}
	}()

	if err := pipeline.IndexFrames(framesDir, progressCh); err != nil {
		log.Printf("Indexing failed for %s/%s: %v", cameraID, date, err)
	}
	close(progressCh)
	<-progressDone

	// Update process history
	allVideoFiles := ListVideoFiles(videosDir)
	AddProcessHistory(h.cfg.Process.HistoryPath, cameraID, date, allVideoFiles)

	h.mu.Lock()
	job.Events = append(job.Events, uploadJobEvent{Stage: "complete"})
	h.mu.Unlock()
}

func (h *CamerasHandler) UploadStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, `{"error":"job_id query parameter required"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	job, ok := h.uploadJobs[jobID]
	h.mu.Unlock()

	if !ok {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	// SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send any events already collected
	h.mu.Lock()
	sent := len(job.Events)
	for _, ev := range job.Events {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	h.mu.Unlock()
	flusher.Flush()

	// Stream new events until done
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-job.doneCh:
			// Send remaining events
			h.mu.Lock()
			for i := sent; i < len(job.Events); i++ {
				data, _ := json.Marshal(job.Events[i])
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			h.mu.Unlock()
			flusher.Flush()
			return
		case <-ticker.C:
			h.mu.Lock()
			for i := sent; i < len(job.Events); i++ {
				data, _ := json.Marshal(job.Events[i])
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			sent = len(job.Events)
			h.mu.Unlock()
			flusher.Flush()
		}
	}
}

// Snapshot proxies a JPEG snapshot from a Hikvision camera.
func (h *CamerasHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cam, err := h.svc.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if cam.Type == "hikvision" {
		nvrIP := h.settings.Get("nvr.ip")
		if nvrIP == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "NVR IP not configured in settings"})
			return
		}
		nvrUsername := h.settings.Get("nvr.username")
		nvrPassword := h.settings.Get("nvr.password")
		channel := services.NVRChannel(cam)

		client := services.NewHikvisionClient(nvrIP, nvrUsername, nvrPassword)
		data, err := client.Snapshot(channel)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("snapshot failed: %v", err)})
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Write(data)
	} else {
		data, err := h.svc.Thumbnail(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(data)
	}
}

// StreamStart starts an HLS stream for a Hikvision camera.
func (h *CamerasHandler) StreamStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cam, err := h.svc.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if cam.Type != "hikvision" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "streaming only available for hikvision cameras"})
		return
	}

	nvrIP := h.settings.Get("nvr.ip")
	if nvrIP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "NVR IP not configured in settings"})
		return
	}
	nvrRTSPPort := h.settings.GetInt("nvr.rtsp_port")
	if nvrRTSPPort == 0 {
		nvrRTSPPort = 554
	}
	nvrUsername := h.settings.Get("nvr.username")
	nvrPassword := h.settings.Get("nvr.password")
	rtspURL := services.CameraRTSPUrl(cam, nvrIP, nvrRTSPPort, nvrUsername, nvrPassword, 2) // substream for live view
	if err := h.streamer.Start(id, rtspURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("stream start failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// StreamServe serves HLS files (index.m3u8 + .ts segments) for an active stream.
func (h *CamerasHandler) StreamServe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	filename := chi.URLParam(r, "filename")

	h.streamer.Touch(id)

	dir := h.streamer.Dir(id)
	if dir == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not active"})
		return
	}

	// Sanitize filename
	safeFile := filepath.Base(filename)
	if safeFile != filename || safeFile == ".." || safeFile == "." {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}

	filePath := filepath.Join(dir, safeFile)

	// Set appropriate content type
	switch {
	case strings.HasSuffix(safeFile, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case strings.HasSuffix(safeFile, ".ts"):
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Cache-Control", "no-cache, no-store")

	http.ServeFile(w, r, filePath)
}

// StreamStop stops an active HLS stream.
func (h *CamerasHandler) StreamStop(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.streamer.Stop(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
