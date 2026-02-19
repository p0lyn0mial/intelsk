# Backend API (Go)

[Back to main design](DESIGN.md)

The backend is a Go HTTP server using the Chi router. It handles all API requests,
orchestrates the processing pipeline, manages SQLite storage, and proxies ML
inference and search to the
[Python ML sidecar](indexing-and-search.md#python-ml-sidecar-api).

## Endpoints

```
GET  /api/health                       Health check

GET  /api/cameras                      List configured cameras
GET  /api/cameras/{id}/snapshot        Live snapshot from camera

POST /api/process                      Start download+extract+index pipeline
GET  /api/process/status               SSE stream of pipeline progress
GET  /api/process/history              List previously processed camera+date combos

POST /api/search/text                  CLIP text search (via ML sidecar)
POST /api/search/person/{name}         Search by enrolled person name (via ML sidecar)

GET  /api/frames/{frame_id}            Get frame image
GET  /api/frames/{frame_id}/meta       Get frame metadata
GET  /api/videos/{video_id}/play       Stream source video segment

GET  /api/faces/registry               List enrolled persons
DELETE /api/faces/registry/{name}      Remove enrolled person
POST /api/faces/discover               Trigger face clustering on unassigned faces
GET  /api/faces/clusters               Get discovered face clusters
POST /api/faces/clusters/{id}/assign   Assign a name to a cluster

POST /api/cleanup                      Trigger data retention cleanup
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
    Score          float64 `json:"score"`          // cosine for CLIP, 1-distance for face
    SourceVideoURL string  `json:"source_video_url,omitempty"`
    SeekOffsetSec  int     `json:"seek_offset_sec" // seconds into the video segment
}

type SearchResponse struct {
    Results []SearchResult `json:"results"`
    Query   string         `json:"query"`
    Total   int            `json:"total"`
}
```

## Search Flow

For text search, the Go backend:
1. Calls the ML sidecar `POST /search/image` with the query text, DB path, and filters
2. ML sidecar encodes text via CLIP, loads embeddings from SQLite, performs
   brute-force cosine search, returns ranked results
3. Go maps results to `SearchResult` structs, populating video playback fields (see below),
   and returns JSON

For person search:
1. Reads the person's encodings from the face registry JSON file
2. Averages the encodings
3. Calls the ML sidecar `POST /search/face` with the averaged encoding, DB path,
   and filters
4. ML sidecar loads face embeddings from SQLite, computes L2 distance, returns
   ranked results within threshold
5. Go maps results and returns JSON. Since `face_embeddings` lacks a `source_video`
   column, Go joins against `clip_embeddings` by `frame_path` to retrieve it
   (see [indexing-and-search.md](indexing-and-search.md#face-search-source_video-lookup))

### Video Fields in Search Results

When mapping ML sidecar results to `SearchResult` structs, Go populates two
video-related fields:

- **`source_video_url`**: constructed from the ML sidecar's `source_video` path
  (e.g., `videos/front_door/2026-02-18/1400.mp4`) by encoding it as a video ID
  and building `/api/videos/{video_id}/play`. The video ID replaces `/` with `--`
  and drops the extension: `front_door--2026-02-18--1400`.
- **`seek_offset_sec`**: computed from the frame timestamp minus the video segment
  start hour. For example, a frame at `14:23:05` from `1400.mp4` (which starts at
  14:00:00) yields `23*60 + 5 = 1385` seconds.

## Face Discovery Request/Response

```go
// POST /api/faces/discover
// No request body — clusters all unassigned faces in the database
type DiscoverResponse struct {
    ClusterCount int `json:"cluster_count"`
    FaceCount    int `json:"face_count"`
}

// GET /api/faces/clusters
type FaceCluster struct {
    ClusterID           int              `json:"cluster_id"`
    Size                int              `json:"size"`
    RepresentativeFaces []FaceThumbnail  `json:"representative_faces"`
}

type FaceThumbnail struct {
    ID        string `json:"id"`
    FramePath string `json:"frame_path"`
    FrameURL  string `json:"frame_url"`
    CameraID  string `json:"camera_id"`
    Timestamp string `json:"timestamp"`
    BBox      BBox   `json:"bbox"`
}

type ClustersResponse struct {
    Clusters []FaceCluster `json:"clusters"`
}

// POST /api/faces/clusters/{id}/assign
type AssignRequest struct {
    Name string `json:"name"` // person name to assign
}

type AssignResponse struct {
    Name       string `json:"name"`
    FaceCount  int    `json:"face_count"`
}
```

## Cleanup Request/Response

```go
// POST /api/cleanup
// Optional query param: ?older_than=30d (defaults to storage.retention_days)
type CleanupResponse struct {
    DeletedEmbeddings int64 `json:"deleted_embeddings"`
    DeletedFrames     int64 `json:"deleted_frames"`
    DeletedVideos     int64 `json:"deleted_videos"`
}
```

Cleanup deletes:
- SQLite embedding rows older than N days
- Frame JPEG files from purged dates
- Video MP4 files from purged dates
- Process history entries for purged dates

Face registry entries and assigned face embeddings are **not** affected by cleanup.

## Static File Serving

The Go backend serves extracted frames and video clips from `data/` using
`http.FileServer`, with path validation to prevent directory traversal.

## Video Playback

`GET /api/videos/{video_id}/play` serves source video segments for inline playback
in the web UI.

### Video ID Encoding

The video ID is derived from the on-disk path by replacing `/` with `--` and
dropping the `.mp4` extension:

| On-disk path | Video ID |
|---|---|
| `videos/front_door/2026-02-18/1400.mp4` | `front_door--2026-02-18--1400` |
| `videos/driveway/2026-02-18/0800.mp4` | `driveway--2026-02-18--0800` |

### Handler

```go
func (h *VideoHandler) Play(w http.ResponseWriter, r *http.Request) {
    videoID := chi.URLParam(r, "video_id")

    // Decode video ID back to file path
    // "front_door--2026-02-18--1400" → "videos/front_door/2026-02-18/1400.mp4"
    relativePath := strings.ReplaceAll(videoID, "--", "/") + ".mp4"
    filePath := filepath.Join(h.dataDir, "videos", relativePath)

    // Path validation: ensure resolved path stays within data/videos/
    absPath, err := filepath.Abs(filePath)
    if err != nil || !strings.HasPrefix(absPath, h.videosDir) {
        http.Error(w, "invalid video ID", http.StatusBadRequest)
        return
    }

    // http.ServeFile handles Range requests (206 Partial Content) automatically,
    // enabling browser-native seeking in the HTML5 <video> element.
    http.ServeFile(w, r, absPath)
}
```

### Notes

- **Range requests**: `http.ServeFile` handles `Range` headers automatically,
  returning `206 Partial Content`. This allows the browser's `<video>` element to
  seek to any position without downloading the entire file.
- **MP4 moov atom**: For instant seeking, the MP4 moov atom should be at the start
  of the file. If seeking is slow (player must buffer from the beginning), the
  download step can fix this with `ffmpeg -movflags faststart`.
- **404 on purged videos**: If a video was deleted by the retention cleanup, the
  handler returns a standard 404. The frontend shows a "video unavailable" message.

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
