package models

import "time"

type FrameMetadata struct {
	FramePath        string    `json:"frame_path"`
	CameraID         string    `json:"camera_id"`
	Timestamp        time.Time `json:"timestamp"`
	SourceVideo      string    `json:"source_video"`
	FrameNumber      int       `json:"frame_number"`
	ExtractionMethod string    `json:"extraction_method"`
}

type SearchResult struct {
	ID          string  `json:"id"`
	FramePath   string  `json:"frame_path"`
	CameraID    string  `json:"camera_id"`
	Timestamp   string  `json:"timestamp"`
	SourceVideo string  `json:"source_video"`
	Score       float64 `json:"score"`
}

type IndexState struct {
	IndexedFrames map[string]bool `json:"indexed_frames"`
	LastUpdated   time.Time       `json:"last_updated"`
}

// API request/response types

type ProcessRequest struct {
	CameraIDs []string `json:"camera_ids"`
	StartDate string   `json:"start_date"`
	EndDate   string   `json:"end_date"`
}

type ProcessResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

type TextSearchRequest struct {
	Query     string   `json:"query"`
	CameraIDs []string `json:"camera_ids,omitempty"`
	StartTime string   `json:"start_time,omitempty"`
	EndTime   string   `json:"end_time,omitempty"`
	Limit     int      `json:"limit"`
}

type APISearchResult struct {
	FrameID        string  `json:"frame_id"`
	FrameURL       string  `json:"frame_url"`
	CameraID       string  `json:"camera_id"`
	Timestamp      string  `json:"timestamp"`
	Score          float64 `json:"score"`
	SourceVideoURL string  `json:"source_video_url,omitempty"`
	SeekOffsetSec  int     `json:"seek_offset_sec"`
}

type SearchResponse struct {
	Results []APISearchResult `json:"results"`
	Query   string            `json:"query"`
	Total   int               `json:"total"`
}

type ProcessHistoryEntry struct {
	CameraID  string    `json:"camera_id"`
	Date      string    `json:"date"`
	Videos    []string  `json:"videos,omitempty"`
	IndexedAt time.Time `json:"indexed_at"`
}

type CameraInfo struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
	Status    string         `json:"status"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

type CreateCameraRequest struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

type UpdateCameraRequest struct {
	Name   string         `json:"name,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type DownloadRequest struct {
	URL string `json:"url"`
}

type SettingsResponse struct {
	Settings map[string]any `json:"settings"`
	Defaults map[string]any `json:"defaults"`
}

type SettingsUpdateRequest struct {
	Settings map[string]any `json:"settings"`
}
