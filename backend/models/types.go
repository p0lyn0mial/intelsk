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
