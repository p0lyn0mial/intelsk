package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type SearchHandler struct {
	cfg      *config.AppConfig
	mlClient *services.MLClient
}

func NewSearchHandler(cfg *config.AppConfig, mlClient *services.MLClient) *SearchHandler {
	return &SearchHandler{
		cfg:      cfg,
		mlClient: mlClient,
	}
}

func (h *SearchHandler) TextSearch(w http.ResponseWriter, r *http.Request) {
	var req models.TextSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, `{"error":"query is required"}`, http.StatusBadRequest)
		return
	}

	if req.Limit == 0 {
		req.Limit = 20
	}

	results, err := h.mlClient.SearchByText(
		h.cfg.Storage.DBPath,
		req.Query,
		req.CameraIDs,
		req.StartTime,
		req.EndTime,
		req.Limit,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"search failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	apiResults := make([]models.APISearchResult, len(results))
	for i, r := range results {
		apiResults[i] = mapSearchResult(r, h.cfg.App.DataDir)
	}

	writeJSON(w, http.StatusOK, models.SearchResponse{
		Results: apiResults,
		Query:   req.Query,
		Total:   len(apiResults),
	})
}

// mapSearchResult converts an ML sidecar SearchResult into an API-facing
// APISearchResult, populating video URL and seek offset.
func mapSearchResult(r models.SearchResult, dataDir string) models.APISearchResult {
	result := models.APISearchResult{
		FrameID:   r.ID,
		FrameURL:  buildFrameURL(r.FramePath),
		CameraID:  r.CameraID,
		Timestamp: r.Timestamp,
		Score:     r.Score,
	}

	// Build source_video_url and seek_offset_sec
	if r.SourceVideo != "" {
		result.SourceVideoURL = buildVideoURL(r.SourceVideo)
		result.SeekOffsetSec = computeSeekOffset(r.Timestamp, r.SourceVideo)
	}

	return result
}

// buildFrameURL constructs the API URL for a frame image.
// frame_path like "frames/front_door/2026-02-18/frame_000042.jpg"
// or absolute path — we extract the relative part after "frames/"
func buildFrameURL(framePath string) string {
	// Normalize to forward slashes for URL
	fp := filepath.ToSlash(framePath)

	// Extract the part after "frames/" if present
	if idx := strings.Index(fp, "frames/"); idx >= 0 {
		fp = fp[idx+len("frames/"):]
	} else {
		fp = filepath.Base(fp)
	}

	return "/api/frames/" + fp
}

// buildVideoURL encodes a source_video path as a video ID URL.
// "videos/front_door/2026-02-18/1400.mp4" → "/api/videos/front_door--2026-02-18--1400/play"
func buildVideoURL(sourceVideo string) string {
	sv := filepath.ToSlash(sourceVideo)

	// Strip "videos/" prefix if present
	sv = strings.TrimPrefix(sv, "videos/")

	// Drop .mp4 extension
	sv = strings.TrimSuffix(sv, ".mp4")

	// Replace / with --
	videoID := strings.ReplaceAll(sv, "/", "--")

	return "/api/videos/" + videoID + "/play"
}

// computeSeekOffset calculates seconds into the video segment.
// Frame timestamp like "2026-02-18T14:23:05" from video "1400.mp4" (starts at 14:00)
// → 23*60 + 5 = 1385 seconds
func computeSeekOffset(timestamp, sourceVideo string) int {
	// Parse frame timestamp
	var frameTime time.Time
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		t, err := time.Parse(layout, timestamp)
		if err == nil {
			frameTime = t
			break
		}
	}
	if frameTime.IsZero() {
		return 0
	}

	// Extract hour from video filename (e.g., "1400.mp4" → hour 14)
	sv := filepath.ToSlash(sourceVideo)
	base := filepath.Base(sv)
	base = strings.TrimSuffix(base, ".mp4")

	if len(base) < 2 {
		return 0
	}

	hour, err := strconv.Atoi(base[:2])
	if err != nil {
		return 0
	}

	// Compute offset: frame time - segment start hour
	segmentStart := time.Date(frameTime.Year(), frameTime.Month(), frameTime.Day(),
		hour, 0, 0, 0, frameTime.Location())

	offset := int(frameTime.Sub(segmentStart).Seconds())
	if offset < 0 {
		return 0
	}
	return offset
}
