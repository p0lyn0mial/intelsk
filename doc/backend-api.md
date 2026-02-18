# Backend API (Go)

[Back to main design](DESIGN.md)

The backend is a Go HTTP server using the Chi router. It handles all API requests,
orchestrates the processing pipeline, and proxies ML inference to the
[Python ML sidecar](indexing-and-search.md#python-ml-sidecar-api).

## Endpoints

```
GET  /api/health                    Health check
GET  /api/cameras                   List configured cameras
GET  /api/cameras/{id}/snapshot     Live snapshot from camera

POST /api/process                   Start download+extract+index pipeline
GET  /api/process/status            SSE stream of pipeline progress
GET  /api/process/history           List previously processed camera+date combos

POST /api/search/text               CLIP text search
POST /api/search/person/{name}      Search by enrolled person name

GET  /api/frames/{frame_id}         Get frame image
GET  /api/frames/{frame_id}/meta    Get frame metadata
GET  /api/videos/{video_id}/play    Stream source video segment

GET  /api/faces/registry            List enrolled persons
POST /api/faces/registry            Enroll a new person (upload photo)
DELETE /api/faces/registry/{name}   Remove enrolled person
```

## Process Request/Response

```go
// POST /api/process
type ProcessRequest struct {
    CameraIDs []string `json:"camera_ids"`
    StartDate string   `json:"start_date"` // "2026-02-18"
    EndDate   string   `json:"end_date"`   // "2026-02-18"
}

type ProcessResponse struct {
    JobID  string `json:"job_id"`
    Status string `json:"status"` // "started" | "already_cached"
}

// GET /api/process/status?job_id=...  (SSE stream)
// Sends events like:
//   data: {"stage": "downloading", "camera_id": "front_door", "progress": 0.45}
//   data: {"stage": "extracting", "camera_id": "front_door", "progress": 0.80}
//   data: {"stage": "indexing", "frames_done": 120, "frames_total": 500}
//   data: {"stage": "complete"}
```

## Search Request/Response

```go
// POST /api/search/text
type TextSearchRequest struct {
    Query     string   `json:"query"`
    CameraIDs []string `json:"camera_ids,omitempty"`
    StartTime string   `json:"start_time,omitempty"` // ISO 8601
    EndTime   string   `json:"end_time,omitempty"`   // ISO 8601
    Limit     int      `json:"limit"`                // default 20
}

type SearchResult struct {
    FrameID        string  `json:"frame_id"`
    FrameURL       string  `json:"frame_url"`
    CameraID       string  `json:"camera_id"`
    CameraName     string  `json:"camera_name"`
    Timestamp      string  `json:"timestamp"`
    Score          float64 `json:"score"`     // cosine for CLIP, 1-distance for face
    SourceVideoURL string  `json:"source_video_url,omitempty"`
}

type SearchResponse struct {
    Results []SearchResult `json:"results"`
    Query   string         `json:"query"`
    Total   int            `json:"total"`
}
```

## Search Flow

For text search, the Go backend:
1. Calls the ML sidecar `POST /encode/text` to get the query embedding
2. Queries ChromaDB `clip_embeddings` collection with the embedding + filters
3. Maps results to `SearchResult` structs and returns JSON

For person search:
1. Reads the person's encodings from the face registry JSON file
2. Averages the encodings
3. Queries ChromaDB `face_embeddings` collection with the averaged embedding
4. Maps results and returns JSON

## Static File Serving

The Go backend serves extracted frames and video clips from `data/` using
`http.FileServer`, with path validation to prevent directory traversal.

## SSE Progress Streaming

Pipeline progress is streamed to the frontend using Server-Sent Events.
The Go backend uses `text/event-stream` content type and flushes after each
event using `http.Flusher`.

```go
func (h *ProcessHandler) Status(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, _ := w.(http.Flusher)

    for event := range h.progressChan {
        fmt.Fprintf(w, "data: %s\n\n", event.JSON())
        flusher.Flush()
    }
}
```
