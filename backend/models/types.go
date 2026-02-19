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
