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
	cfg      *config.AppConfig
	mlClient *services.MLClient
	storage  *services.Storage

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

func NewProcessHandler(cfg *config.AppConfig, mlClient *services.MLClient, storage *services.Storage) *ProcessHandler {
	return &ProcessHandler{
		cfg:        cfg,
		mlClient:   mlClient,
		storage:    storage,
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

	// Check process history — skip already-indexed combos
	history := loadProcessHistory(h.cfg.Process.HistoryPath)
	allCached := true
	for _, camID := range req.CameraIDs {
		if !isProcessed(history, camID, req.StartDate) {
			allCached = false
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

	pipeline := services.NewPipeline(h.mlClient, h.storage, h.cfg.CLIP.BatchSize)

	// Collect events from pipeline into job state
	go func() {
		for ev := range job.eventCh {
			h.mu.Lock()
			job.Events = append(job.Events, ev)
			h.mu.Unlock()
		}
	}()

	for _, camID := range req.CameraIDs {
		// Generate list of dates from start to end
		dates, err := dateRange(req.StartDate, req.EndDate)
		if err != nil {
			h.mu.Lock()
			job.Status = "failed"
			job.Error = fmt.Sprintf("invalid date range: %v", err)
			h.mu.Unlock()
			close(job.eventCh)
			return
		}

		for _, date := range dates {
			if isProcessed(loadProcessHistory(h.cfg.Process.HistoryPath), camID, date) {
				job.eventCh <- services.ProgressEvent{
					Stage:    "skipped",
					CameraID: camID,
					Message:  fmt.Sprintf("%s/%s already indexed", camID, date),
				}
				continue
			}

			// Step 1: Extract frames from videos (if not already extracted)
			framesDir := filepath.Join(h.cfg.Extraction.StoragePath, camID, date)
			manifest := filepath.Join(framesDir, "manifest.json")

			if _, err := os.Stat(manifest); os.IsNotExist(err) {
				// Run extraction
				job.eventCh <- services.ProgressEvent{
					Stage:    "extracting",
					CameraID: camID,
					Message:  fmt.Sprintf("extracting frames for %s/%s", camID, date),
				}

				videosDir := filepath.Join(h.cfg.App.DataDir, "videos", camID, date)
				entries, err := os.ReadDir(videosDir)
				if err != nil {
					job.eventCh <- services.ProgressEvent{
						Stage:    "error",
						CameraID: camID,
						Message:  fmt.Sprintf("no videos found for %s/%s: %v", camID, date, err),
					}
					continue
				}

				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") {
						continue
					}
					videoPath := filepath.Join(videosDir, e.Name())
					frames, err := services.ExtractFramesTime(
						videoPath, framesDir,
						h.cfg.Extraction.TimeIntervalSec,
						h.cfg.Extraction.OutputQuality,
					)
					if err != nil {
						log.Printf("extraction failed for %s: %v", videoPath, err)
						continue
					}

					if h.cfg.Extraction.DedupEnabled {
						frames, _ = services.DeduplicateFrames(frames, h.cfg.Extraction.DedupPHashThreshold)
					}

					if err := services.WriteManifest(framesDir, frames); err != nil {
						log.Printf("writing manifest for %s: %v", videoPath, err)
					}
				}
			}

			// Step 2: Index frames
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

			// Record in process history
			addProcessHistory(h.cfg.Process.HistoryPath, camID, date)
		}
	}

	job.eventCh <- services.ProgressEvent{Stage: "complete", Message: "all processing complete"}
	close(job.eventCh)

	h.mu.Lock()
	job.Status = "complete"
	h.mu.Unlock()
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

func isProcessed(history []models.ProcessHistoryEntry, cameraID, date string) bool {
	for _, h := range history {
		if h.CameraID == cameraID && h.Date == date {
			return true
		}
	}
	return false
}

func addProcessHistory(path, cameraID, date string) {
	history := loadProcessHistory(path)

	// Don't add duplicates
	if isProcessed(history, cameraID, date) {
		return
	}

	history = append(history, models.ProcessHistoryEntry{
		CameraID:  cameraID,
		Date:      date,
		IndexedAt: time.Now(),
	})

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
