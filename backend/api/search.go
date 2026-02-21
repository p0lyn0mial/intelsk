package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
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
	settings *services.SettingsService
}

func NewSearchHandler(cfg *config.AppConfig, mlClient *services.MLClient, settings *services.SettingsService) *SearchHandler {
	return &SearchHandler{
		cfg:      cfg,
		mlClient: mlClient,
		settings: settings,
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
		req.Limit = h.settings.GetInt("search.default_limit")
	}

	minScore := h.settings.GetFloat64("search.min_score")

	// Request more results than needed to compensate for dedup filtering
	fetchLimit := req.Limit * 4
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	results, err := h.mlClient.SearchByText(
		h.cfg.Storage.DBPath,
		req.Query,
		req.CameraIDs,
		req.StartTime,
		req.EndTime,
		fetchLimit,
		minScore,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"search failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	// Deduplicate: keep only the best-scoring frame per camera
	// per time window (60s). Results are already sorted by score descending.
	results = deduplicateResults(results, 60)

	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	// Sort by timestamp ascending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp < results[j].Timestamp
	})

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
// Also handles absolute paths by extracting the part after "videos/"
func buildVideoURL(sourceVideo string) string {
	sv := filepath.ToSlash(sourceVideo)

	// Extract the part after "videos/" (handles both relative and absolute paths)
	if idx := strings.Index(sv, "videos/"); idx >= 0 {
		sv = sv[idx+len("videos/"):]
	}

	// Drop .mp4 extension
	sv = strings.TrimSuffix(sv, ".mp4")

	// Replace / with --
	videoID := strings.ReplaceAll(sv, "/", "--")

	return "/api/videos/" + videoID + "/play"
}

// computeSeekOffset calculates seconds into the video segment.
// Frame timestamp like "2026-02-18T14:23:05" from video "1400.mp4" (starts at 14:00)
// → 23*60 + 5 = 1385 seconds.
// For non-hour-based filenames, assumes the video starts at midnight and
// computes offset from there.
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

	// Try to extract hour from video filename (e.g., "1400.mp4" → hour 14).
	// Default to 0 (midnight) for non-hour filenames.
	hour := 0
	sv := filepath.ToSlash(sourceVideo)
	base := filepath.Base(sv)
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	if len(stem) >= 2 {
		if h, err := strconv.Atoi(stem[:2]); err == nil && h >= 0 && h <= 23 {
			hour = h
		}
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

// deduplicateResults removes near-duplicate search results. For each camera,
// it keeps only the best-scoring frame per time window (windowSec seconds).
// Results must be pre-sorted by descending score.
func deduplicateResults(results []models.SearchResult, windowSec int) []models.SearchResult {
	// Track accepted timestamps per camera
	type accepted struct {
		timestamps []time.Time
	}
	seen := make(map[string]*accepted)

	var out []models.SearchResult
	for _, r := range results {
		key := r.CameraID
		ts := parseTimestamp(r.Timestamp)

		if a, ok := seen[key]; ok {
			tooClose := false
			for _, prev := range a.timestamps {
				diff := ts.Sub(prev)
				if diff < 0 {
					diff = -diff
				}
				if diff < time.Duration(windowSec)*time.Second {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			a.timestamps = append(a.timestamps, ts)
		} else {
			seen[key] = &accepted{timestamps: []time.Time{ts}}
		}

		out = append(out, r)
	}
	return out
}

func parseTimestamp(ts string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		t, err := time.Parse(layout, ts)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}
