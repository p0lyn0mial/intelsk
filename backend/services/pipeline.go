package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/intelsk/backend/models"
)

type ProgressEvent struct {
	Stage       string `json:"stage"`
	CameraID    string `json:"camera_id"`
	FramesDone  int    `json:"frames_done"`
	FramesTotal int    `json:"frames_total"`
	Message     string `json:"message"`
}

type Pipeline struct {
	mlClient  *MLClient
	storage   *Storage
	batchSize int
}

func NewPipeline(mlClient *MLClient, storage *Storage, batchSize int) *Pipeline {
	return &Pipeline{
		mlClient:  mlClient,
		storage:   storage,
		batchSize: batchSize,
	}
}

func (p *Pipeline) IndexFrames(framesDir string, progress chan<- ProgressEvent) error {
	// 1. Read manifest.json
	manifestPath := filepath.Join(framesDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var frames []models.FrameMetadata
	if err := json.Unmarshal(data, &frames); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	if len(frames) == 0 {
		if progress != nil {
			progress <- ProgressEvent{Stage: "complete", Message: "no frames to index"}
		}
		return nil
	}

	// 2. Load index state for resume support
	statePath := filepath.Join(framesDir, "index_state.json")
	state := loadIndexState(statePath)

	// Filter out already-indexed frames
	var pending []models.FrameMetadata
	for _, f := range frames {
		id := frameID(f)
		if !state.IndexedFrames[id] {
			pending = append(pending, f)
		}
	}

	if len(pending) == 0 {
		if progress != nil {
			progress <- ProgressEvent{
				Stage:   "complete",
				Message: fmt.Sprintf("all %d frames already indexed", len(frames)),
			}
		}
		return nil
	}

	cameraID := frames[0].CameraID
	total := len(pending)
	done := 0

	if progress != nil {
		progress <- ProgressEvent{
			Stage:       "indexing",
			CameraID:    cameraID,
			FramesDone:  0,
			FramesTotal: total,
			Message:     fmt.Sprintf("indexing %d frames (%d already done)", total, len(frames)-total),
		}
	}

	// 3. Batch frames and process
	for i := 0; i < len(pending); i += p.batchSize {
		end := i + p.batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]

		// Collect absolute paths for the ML sidecar
		paths := make([]string, len(batch))
		for j, f := range batch {
			if filepath.IsAbs(f.FramePath) {
				paths[j] = f.FramePath
			} else {
				paths[j] = filepath.Join(framesDir, filepath.Base(f.FramePath))
			}
		}

		// 4. Encode images via ML sidecar
		embeddings, err := p.mlClient.EncodeImages(paths)
		if err != nil {
			return fmt.Errorf("encoding batch %d: %w", i/p.batchSize, err)
		}

		// 5. Store embeddings
		for j, f := range batch {
			id := frameID(f)
			embBytes := Float64sToBytes(embeddings[j])
			ts := f.Timestamp.Format(time.RFC3339)

			if err := p.storage.AddClipEmbedding(id, embBytes,
				f.CameraID, ts, f.FramePath, f.SourceVideo); err != nil {
				return fmt.Errorf("storing embedding for %s: %w", id, err)
			}

			state.IndexedFrames[id] = true
		}

		// 6. Save index state after each batch
		state.LastUpdated = time.Now()
		if err := saveIndexState(statePath, state); err != nil {
			return fmt.Errorf("saving index state: %w", err)
		}

		done += len(batch)
		if progress != nil {
			progress <- ProgressEvent{
				Stage:       "indexing",
				CameraID:    cameraID,
				FramesDone:  done,
				FramesTotal: total,
				Message:     fmt.Sprintf("batch %d complete", i/p.batchSize+1),
			}
		}
	}

	if progress != nil {
		progress <- ProgressEvent{
			Stage:       "complete",
			CameraID:    cameraID,
			FramesDone:  total,
			FramesTotal: total,
			Message:     "indexing complete",
		}
	}

	return nil
}

// frameID generates an ID like {camera_id}_{YYYYMMDD}_{HHMMSS}_{frame_number}
func frameID(f models.FrameMetadata) string {
	ts := f.Timestamp.Format("20060102_150405")
	return fmt.Sprintf("%s_%s_%06d", f.CameraID, ts, f.FrameNumber)
}

func loadIndexState(path string) *models.IndexState {
	state := &models.IndexState{
		IndexedFrames: make(map[string]bool),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	json.Unmarshal(data, state)
	if state.IndexedFrames == nil {
		state.IndexedFrames = make(map[string]bool)
	}
	return state
}

func saveIndexState(path string, state *models.IndexState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// resolveFramesDir finds the frames directory for a camera+date combination.
// It searches within the configured storage path: {storagePath}/{camera}/{date}/
func ResolveFramesDir(storagePath, cameraID, date string) (string, error) {
	// Validate date format
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", fmt.Errorf("invalid date format %q (expected YYYY-MM-DD): %w", date, err)
	}

	dir := filepath.Join(storagePath, cameraID, date)
	manifest := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no manifest found at %s — run 'extract' or 'process' first", manifest)
		}
		return "", err
	}

	return dir, nil
}

// FormatResultsTable formats search results as a human-readable table.
func FormatResultsTable(results []models.SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-6s %-8s %-42s %-12s %-20s %s\n",
		"Rank", "Score", "ID", "Camera", "Timestamp", "Frame Path")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 120))
	for i, r := range results {
		fmt.Fprintf(&b, "%-6d %-8.4f %-42s %-12s %-20s %s\n",
			i+1, r.Score, r.ID, r.CameraID, r.Timestamp, r.FramePath)
	}
	return b.String()
}
