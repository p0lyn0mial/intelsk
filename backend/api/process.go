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

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type ProcessHandler struct {
	cfg       *config.AppConfig
	mlClient  *services.MLClient
	storage   *services.Storage
	settings  *services.SettingsService
	cameraSvc *services.CameraService

	mu         sync.Mutex
	activeJobs map[string]*jobState
}

type jobState struct {
	ID       string
	Status   string // "running", "complete", "failed"
	Error    string
	Events   []services.ProgressEvent
	eventCh  chan services.ProgressEvent
	doneCh   chan struct{}
}

func NewProcessHandler(cfg *config.AppConfig, mlClient *services.MLClient, storage *services.Storage, settings *services.SettingsService, cameraSvc *services.CameraService) *ProcessHandler {
	return &ProcessHandler{
		cfg:        cfg,
		mlClient:   mlClient,
		storage:    storage,
		settings:   settings,
		cameraSvc:  cameraSvc,
		activeJobs: make(map[string]*jobState),
	}
}

func (h *ProcessHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.CameraIDs) == 0 || req.StartDate == "" {
		http.Error(w, `{"error":"camera_ids and start_date are required"}`, http.StatusBadRequest)
		return
	}

	if req.EndDate == "" {
		req.EndDate = req.StartDate
	}

	// Quick check: any new videos to process across all cameras × dates?
	// Skip cache check for hikvision cameras (they need NVR download first)
	history := loadProcessHistory(h.cfg.Process.HistoryPath)
	dates, err := dateRange(req.StartDate, req.EndDate)
	if err == nil {
		allCached := true
		for _, camID := range req.CameraIDs {
			cam, camErr := h.cameraSvc.Get(camID)
			if camErr == nil && cam.Type == "hikvision" {
				allCached = false
				break
			}
			for _, date := range dates {
				videosDir := filepath.Join(h.cfg.App.DataDir, "videos", camID, date)
				if len(newVideosForDate(history, camID, date, videosDir)) > 0 {
					allCached = false
					break
				}
			}
			if !allCached {
				break
			}
		}
		if allCached {
			writeJSON(w, http.StatusOK, models.ProcessResponse{
				JobID:  "",
				Status: "already_cached",
			})
			return
		}
	}

	// Create job
	jobID := fmt.Sprintf("job_%d", time.Now().UnixMilli())
	job := &jobState{
		ID:      jobID,
		Status:  "running",
		eventCh: make(chan services.ProgressEvent, 64),
		doneCh:  make(chan struct{}),
	}

	h.mu.Lock()
	h.activeJobs[jobID] = job
	h.mu.Unlock()

	// Run pipeline in background
	go h.runPipeline(job, req)

	writeJSON(w, http.StatusAccepted, models.ProcessResponse{
		JobID:  jobID,
		Status: "started",
	})
}

func (h *ProcessHandler) runPipeline(job *jobState, req models.ProcessRequest) {
	defer close(job.doneCh)

	// Wait for ML sidecar to be ready before starting
	job.eventCh <- services.ProgressEvent{
		Stage:   "waiting",
		Message: "waiting for ML sidecar...",
	}
	if err := h.mlClient.WaitForReady(120 * time.Second); err != nil {
		job.eventCh <- services.ProgressEvent{
			Stage:   "error",
			Message: fmt.Sprintf("ML sidecar not ready: %v", err),
		}
		close(job.eventCh)
		h.mu.Lock()
		job.Status = "failed"
		job.Error = err.Error()
		h.mu.Unlock()
		return
	}

	pipeline := services.NewPipeline(h.mlClient, h.storage, h.settings.GetInt("clip.batch_size"))

	// Collect events from pipeline into job state
	go func() {
		for ev := range job.eventCh {
			h.mu.Lock()
			job.Events = append(job.Events, ev)
			h.mu.Unlock()
		}
	}()

	for _, camID := range req.CameraIDs {
		dates, err := dateRange(req.StartDate, req.EndDate)
		if err != nil {
			h.mu.Lock()
			job.Status = "failed"
			job.Error = fmt.Sprintf("invalid date range: %v", err)
			h.mu.Unlock()
			close(job.eventCh)
			return
		}

		// Check if this is a hikvision camera — download recordings from NVR
		cam, camErr := h.cameraSvc.Get(camID)
		if camErr == nil && cam.Type == "hikvision" {
			h.downloadFromNVR(job, cam, dates)
		}

		for _, date := range dates {
			videosDir := filepath.Join(h.cfg.App.DataDir, "videos", camID, date)
			framesDir := filepath.Join(h.cfg.Extraction.StoragePath, camID, date)

			// Determine which videos still need processing
			history := loadProcessHistory(h.cfg.Process.HistoryPath)
			videosToProcess := newVideosForDate(history, camID, date, videosDir)

			if len(videosToProcess) == 0 {
				job.eventCh <- services.ProgressEvent{
					Stage:    "skipped",
					CameraID: camID,
					Message:  fmt.Sprintf("%s/%s already indexed", camID, date),
				}
				continue
			}

			// Step 1: Extract frames from new videos only
			job.eventCh <- services.ProgressEvent{
				Stage:    "extracting",
				CameraID: camID,
				Message:  fmt.Sprintf("extracting frames from %d video(s) for %s/%s", len(videosToProcess), camID, date),
			}

			// Load existing manifest (frames from previously processed videos)
			existingFrames, _ := services.LoadManifest(framesDir)
			var newFrames []models.FrameMetadata

			for _, videoFile := range videosToProcess {
				videoPath := filepath.Join(videosDir, videoFile)
				frames, err := services.ExtractFramesTime(
					videoPath, framesDir,
					h.settings.GetInt("extraction.time_interval_sec"),
					h.settings.GetInt("extraction.output_quality"),
				)
				if err != nil {
					log.Printf("extraction failed for %s: %v", videoPath, err)
					continue
				}

				if h.settings.GetBool("extraction.dedup_enabled") {
					frames, _ = services.DeduplicateFrames(frames, h.settings.GetInt("extraction.dedup_phash_threshold"))
				}

				newFrames = append(newFrames, frames...)
			}

			// Merge with existing and write combined manifest
			allFrames := append(existingFrames, newFrames...)
			if err := services.WriteManifest(framesDir, allFrames); err != nil {
				log.Printf("writing manifest for %s/%s: %v", camID, date, err)
			}

			// Step 2: Index frames (pipeline handles incrementality via index_state.json)
			manifest := filepath.Join(framesDir, "manifest.json")
			if _, err := os.Stat(manifest); err != nil {
				job.eventCh <- services.ProgressEvent{
					Stage:    "error",
					CameraID: camID,
					Message:  fmt.Sprintf("no manifest for %s/%s after extraction", camID, date),
				}
				continue
			}

			job.eventCh <- services.ProgressEvent{
				Stage:    "indexing",
				CameraID: camID,
				Message:  fmt.Sprintf("indexing frames for %s/%s", camID, date),
			}

			if err := pipeline.IndexFrames(framesDir, job.eventCh); err != nil {
				log.Printf("indexing failed for %s/%s: %v", camID, date, err)
				job.eventCh <- services.ProgressEvent{
					Stage:    "error",
					CameraID: camID,
					Message:  fmt.Sprintf("indexing failed: %v", err),
				}
				continue
			}

			// Record all videos for this camera+date in process history
			allVideoFiles := ListVideoFiles(videosDir)
			AddProcessHistory(h.cfg.Process.HistoryPath, camID, date, allVideoFiles)
		}
	}

	job.eventCh <- services.ProgressEvent{Stage: "complete", Message: "all processing complete"}
	close(job.eventCh)

	h.mu.Lock()
	job.Status = "complete"
	h.mu.Unlock()
}

// downloadFromNVR downloads recordings from the NVR for a hikvision camera.
// Returns true if any new recordings were downloaded.
func (h *ProcessHandler) downloadFromNVR(job *jobState, cam *models.CameraInfo, dates []string) bool {
	nvrIP := h.settings.Get("nvr.ip")
	if nvrIP == "" {
		job.eventCh <- services.ProgressEvent{
			Stage:    "error",
			CameraID: cam.ID,
			Message:  "NVR IP not configured in settings",
		}
		return false
	}
	nvrUsername := h.settings.Get("nvr.username")
	nvrPassword := h.settings.Get("nvr.password")

	nvrClient := services.NewHikvisionClient(nvrIP, nvrUsername, nvrPassword)

	channel := services.NVRChannel(cam)
	downloaded := 0

	for _, date := range dates {
		dayStart, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		dayEnd := dayStart.Add(24*time.Hour - time.Second)

		job.eventCh <- services.ProgressEvent{
			Stage:    "downloading",
			CameraID: cam.ID,
			Message:  fmt.Sprintf("Searching recordings for %s on %s", cam.Name, date),
		}

		recordings, err := nvrClient.SearchRecordings(channel, dayStart, dayEnd)
		if err != nil {
			log.Printf("NVR search failed for %s/%s: %v", cam.ID, date, err)
			job.eventCh <- services.ProgressEvent{
				Stage:    "error",
				CameraID: cam.ID,
				Message:  fmt.Sprintf("NVR search failed for %s: %v", date, err),
			}
			continue
		}

		if len(recordings) == 0 {
			job.eventCh <- services.ProgressEvent{
				Stage:    "downloading",
				CameraID: cam.ID,
				Message:  fmt.Sprintf("No recordings found for %s on %s", cam.Name, date),
			}
			continue
		}

		videosDir := filepath.Join(h.cfg.App.DataDir, "videos", cam.ID, date)
		os.MkdirAll(videosDir, 0o755)

		// Clean up stale .tmp files from previous failed downloads
		cleanTmpFiles(videosDir)

		total := len(recordings)
		for i, rec := range recordings {
			filename := fmt.Sprintf("%s.mp4", rec.StartTime.Format("1504"))
			outputPath := filepath.Join(videosDir, filename)

			// Handle filename collision: if file exists, try _1, _2, etc.
			if _, err := os.Stat(outputPath); err == nil {
				base := rec.StartTime.Format("1504")
				found := false
				for j := 1; j <= 100; j++ {
					candidate := filepath.Join(videosDir, fmt.Sprintf("%s_%d.mp4", base, j))
					if _, err := os.Stat(candidate); os.IsNotExist(err) {
						outputPath = candidate
						filename = fmt.Sprintf("%s_%d.mp4", base, j)
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			job.eventCh <- services.ProgressEvent{
				Stage:    "downloading",
				CameraID: cam.ID,
				Message:  fmt.Sprintf("Downloading %s %s-%s (%d/%d)...", cam.Name, rec.StartTime.Format("15:04"), rec.EndTime.Format("15:04"), i+1, total),
			}

			if err := nvrClient.DownloadClip(rec.PlaybackURI, outputPath); err != nil {
				log.Printf("NVR download failed for %s: %v", filename, err)
				job.eventCh <- services.ProgressEvent{
					Stage:    "error",
					CameraID: cam.ID,
					Message:  fmt.Sprintf("Download failed for %s: %v", filename, err),
				}
				continue
			}

			// Transcode HEVC to H.264 if enabled for this camera
			if services.ShouldTranscode(cam.Config) {
				job.eventCh <- services.ProgressEvent{
					Stage:    "transcoding",
					CameraID: cam.ID,
					Message:  fmt.Sprintf("Transcoding %s (%d/%d)...", filename, i+1, total),
				}
				if err := services.TranscodeIfNeeded(outputPath); err != nil {
					log.Printf("Transcode failed for %s: %v", filename, err)
					job.eventCh <- services.ProgressEvent{
						Stage:    "error",
						CameraID: cam.ID,
						Message:  fmt.Sprintf("Transcode failed for %s: %v", filename, err),
					}
				}
			}

			downloaded++
		}
	}

	if downloaded > 0 {
		h.cameraSvc.InvalidateThumbnail(cam.ID)
	}
	return downloaded > 0
}

// cleanTmpFiles removes stale .tmp files from a directory.
func cleanTmpFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tmp") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (h *ProcessHandler) Status(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, `{"error":"job_id query parameter required"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	job, ok := h.activeJobs[jobID]
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

func (h *ProcessHandler) History(w http.ResponseWriter, r *http.Request) {
	history := loadProcessHistory(h.cfg.Process.HistoryPath)
	writeJSON(w, http.StatusOK, history)
}

// Process history helpers

func loadProcessHistory(path string) []models.ProcessHistoryEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var history []models.ProcessHistoryEntry
	json.Unmarshal(data, &history)
	return history
}

// ListVideoFiles returns basenames of .mp4 files in a directory.
func ListVideoFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") {
			continue
		}
		files = append(files, e.Name())
	}
	return files
}

// newVideosForDate returns video basenames in videosDir that haven't been
// processed yet according to the history. If the history entry for this
// camera+date has no Videos list (legacy format), it is treated as fully
// processed for backward compatibility.
func newVideosForDate(history []models.ProcessHistoryEntry, cameraID, date, videosDir string) []string {
	allVideos := ListVideoFiles(videosDir)
	if len(allVideos) == 0 {
		return nil
	}

	// Find history entry
	var processed []string
	for _, h := range history {
		if h.CameraID == cameraID && h.Date == date {
			if len(h.Videos) == 0 {
				// Legacy entry without video list — treat as fully processed
				return nil
			}
			processed = h.Videos
			break
		}
	}

	processedSet := make(map[string]bool, len(processed))
	for _, v := range processed {
		processedSet[v] = true
	}

	var newVids []string
	for _, v := range allVideos {
		if !processedSet[v] {
			newVids = append(newVids, v)
		}
	}
	return newVids
}

func AddProcessHistory(path, cameraID, date string, videos []string) {
	history := loadProcessHistory(path)

	// Update existing entry or add new one
	found := false
	for i := range history {
		if history[i].CameraID == cameraID && history[i].Date == date {
			history[i].Videos = videos
			history[i].IndexedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		history = append(history, models.ProcessHistoryEntry{
			CameraID:  cameraID,
			Date:      date,
			Videos:    videos,
			IndexedAt: time.Now(),
		})
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("creating history directory: %v", err)
		return
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		log.Printf("marshaling process history: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("writing process history: %v", err)
	}
}

func dateRange(start, end string) ([]string, error) {
	startDate, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}
	endDate, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	var dates []string
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format("2006-01-02"))
	}
	return dates, nil
}
