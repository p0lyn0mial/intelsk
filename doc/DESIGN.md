# CCTV Video Intelligence System — Design Document

## Problem Statement

Searching through CCTV footage is manual and time-consuming. A user should be able to ask
"show me when Lukasz arrived" or "show all deliveries today" and get timestamped results
instantly. This system combines text-based scene search (CLIP) with person identification
(face recognition) over Hikvision camera footage, exposed through a web UI.

## Architecture

Two-process design: a **Go backend** handles HTTP serving, video downloading, frame
extraction, SQLite storage, and data retention. A **Python ML sidecar** handles CLIP
and face inference, vector search (brute-force NumPy over SQLite-stored embeddings),
and face clustering (the only parts that need PyTorch and dlib).

```
User selects cameras + date(s) in Web UI
                 |
                 v
        +--------+---------+
        |   Go Backend     |
        |   (:8000)        |
        +--------+---------+
                 |
      POST /api/process     (cameras, date range)
                 |
      +----------+-------------------------------------------+
      |                         |                             |
      v                         v                             v
+-----+------+        +---------+--------+        +-----------+-------+
| Downloader |        | Frame Extractor  |        |  Python ML        |
| (ISAPI)    |        | (ffmpeg subprocess|       |  Sidecar (:8001)  |
+-----+------+        +---------+--------+        |  - CLIP encode    |
      |                         |                  |  - face detect    |
      | MP4 clips               | JPEG frames     |  - vector search  |
      v                         v                  |  - face cluster   |
+-----+------+        +---------+--------+        +--------+----------+
| data/      |        | data/            |                  |
| videos/    |        | frames/          |         embeddings (JSON)
+------------+        +------------------+                  |
                                                            v
                                                  +---------+--------+
                                                  |  data/intelsk.db |
                          Processing complete,    |  (SQLite)        |
                          user can now search     |  clip_embeddings |
                                                  |  face_embeddings |
                          Search: Go → ML sidecar +--------+---------+
                          ML sidecar reads from            ^
                          SQLite, brute-force NumPy        |
                                                      query / search
                                                           |
                                                  +--------+---------+
                                                  |    Web UI        |
                                                  |  (React / Vite)  |
                                                  +------------------+
```

### Go Backend Responsibilities
- HTTP API (Chi router)
- Hikvision ISAPI client (HTTP digest auth, download clips)
- Frame extraction (ffmpeg subprocess)
- Pipeline orchestration (download → extract → call ML sidecar → store in SQLite)
- SQLite storage (embeddings + metadata)
- Data retention cleanup (purge frames, videos, and embeddings older than N days)
- SSE progress streaming
- Static file serving (frames, videos)
- Face registry CRUD (JSON file)
- YAML config loading

### Python ML Sidecar Responsibilities
- CLIP image encoding (MobileCLIP2-S0 via open_clip)
- CLIP text encoding
- Face detection + encoding (face_recognition / dlib)
- Vector search (loads embeddings from SQLite, brute-force NumPy cosine/L2 search)
- Face clustering (agglomerative clustering on unassigned face embeddings)
- Receives file paths, text, or DB path — returns embeddings, search results, or clusters

## User Workflow

1. User opens the web UI, selects one or more cameras and a date (or date range)
2. Clicks "Process" — backend downloads video from the selected cameras for that period
3. UI shows a progress bar: downloading → extracting frames → indexing
4. Once complete, the search bar becomes active for that dataset
5. Previously processed camera+date combinations are cached — re-selecting them
   skips straight to search
6. Clicking the play button on a search result opens the source video in an inline
   player, seeked to the exact moment the frame was captured

## Component Design Docs

| Component | Document | Description |
|-----------|----------|-------------|
| Video Downloader | [hikvision-downloader.md](hikvision-downloader.md) | Hikvision ISAPI integration, camera config, on-demand download |
| Frame Extractor | [frame-extraction.md](frame-extraction.md) | Time-based and motion-based extraction, de-duplication |
| Indexing + Search | [indexing-and-search.md](indexing-and-search.md) | Python ML sidecar API, CLIP encoding, face detection, SQLite storage, vector search |
| Backend API | [backend-api.md](backend-api.md) | Go HTTP endpoints, request/response types, SSE progress |
| Web UI | [web-ui.md](web-ui.md) | React frontend, pages, layout wireframe |
| Face Enrollment | [face-enrollment.md](face-enrollment.md) | Face registry format, discovery-based enrollment, matching logic |
| **Roadmap** | [roadmap.md](roadmap.md) | **Detailed task breakdown per phase with checkboxes** |

## Directory Structure

```
intelsk/
  doc/
    DESIGN.md                    # this document
    hikvision-downloader.md
    frame-extraction.md
    indexing-and-search.md
    backend-api.md
    web-ui.md
    face-enrollment.md
    roadmap.md
  config/
    app.yaml                     # app + server + ML + storage settings
    extraction.yaml              # frame extraction settings
  backend/                       # Go backend
    go.mod
    go.sum
    main.go                      # entrypoint (extract, process, index, search, serve)
    cmd/
      server/server.go           # HTTP server setup (Chi router)
    api/
      cameras.go                 # camera CRUD, upload, snapshot, live stream
      process.go                 # process pipeline + NVR download + SSE progress
      search.go                  # text search endpoint
      settings.go                # settings CRUD + NVR status check
      videos.go                  # video playback (range requests)
      helpers.go                 # shared HTTP utilities
    services/
      camera.go                  # camera CRUD, video management, thumbnails
      extractor.go               # frame extraction (ffmpeg subprocess)
      hikvision.go               # Hikvision NVR ISAPI client (digest auth, search, download)
      mlclient.go                # HTTP client for Python ML sidecar
      pipeline.go                # indexing pipeline with resume support
      settings.go                # runtime settings (DB-backed, in-memory cached)
      storage.go                 # SQLite storage (embeddings, settings, schema)
      streamer.go                # live stream management (RTSP → HLS via ffmpeg)
    config/
      config.go                  # YAML config loader
    models/
      types.go                   # shared types / structs
  mlservice/                     # Python ML sidecar
    main.py                      # FastAPI app
    clip_encoder.py              # CLIP model loading + encoding (switchable models)
    searcher.py                  # CLIP cosine similarity search
    requirements.txt
    run.sh
  frontend/
    package.json
    vite.config.ts
    src/
      App.tsx
      main.tsx
      pages/
        MainPage.tsx             # search results page
        CamerasPage.tsx          # camera list + management
        CameraDetailPage.tsx     # per-camera videos/stats
        ProcessPage.tsx          # process pipeline UI with event log
        SettingsPage.tsx         # runtime settings editor
      components/
        NavBar.tsx               # navigation header
        ResultCard.tsx           # search result thumbnail + metadata
        PlayButtonOverlay.tsx    # play icon overlay on result thumbnails
        VideoPlayerModal.tsx     # inline video player modal
        LiveStreamModal.tsx      # live stream viewer (HLS)
        CameraModals.tsx         # camera add/edit modals
      api/
        client.ts                # API client (fetch wrapper + SSE)
        types.ts                 # TypeScript types
      hooks/
        useVideoPlayer.ts        # video player modal state management
        useSearchHistory.ts      # search history persistence
      i18n/
        i18n.ts                  # i18n setup
        en.json                  # English translations
        pl.json                  # Polish translations
  experiments/                   # previous prototypes
    demo_search.py               # CLI CLIP search tool
    DEMO.md
    desktop/                     # tkinter desktop app
    test_faces/                  # face recognition test images
    test_images/                 # CLIP search test images
    test_images_large/           # larger test dataset
  data/                          # gitignored, runtime data
    videos/                      # downloaded / uploaded MP4s
    frames/                      # extracted JPEGs + manifests
    intelsk.db                   # SQLite database (embeddings, settings, cameras)
    process_history.json         # tracks which camera+date combos are indexed
```

## Configuration

Configuration is split between startup YAML files (infrastructure paths, network
addresses) and runtime SQLite settings (tunable parameters editable via the web UI).

### Startup Config (YAML)

```yaml
# config/app.yaml
app:
  host: 0.0.0.0
  port: 8000
  data_dir: data
  log_level: info

mlservice:
  url: http://localhost:8001

storage:
  db_path: data/intelsk.db

clip:
  batch_size: 32

process:
  history_path: data/process_history.json
```

Extraction settings are in `config/extraction.yaml` — tunable parameters
(interval, quality, dedup) are seeded from here on first run but afterwards
controlled via the SQLite settings.

### Runtime Settings (SQLite)

Editable at runtime through the Settings page or `PUT /api/settings`:

| Setting | Default | Description |
|---------|---------|-------------|
| `general.system_name` | CCTV Intelligence | Display name |
| `search.min_score` | 0.18 | Minimum CLIP similarity score |
| `search.default_limit` | 20 | Default search result count |
| `extraction.time_interval_sec` | 5 | Seconds between extracted frames |
| `extraction.output_quality` | 85 | JPEG quality (1–100) |
| `extraction.dedup_enabled` | true | pHash deduplication |
| `extraction.dedup_phash_threshold` | 8 | Hamming distance threshold (0–64) |
| `clip.batch_size` | 32 | Frames per CLIP encoding batch |
| `clip.model` | mobileclip-s0 | Active CLIP model preset |
| `nvr.ip` | *(empty)* | Hikvision NVR IP address |
| `nvr.rtsp_port` | 554 | NVR RTSP port |
| `nvr.username` | *(empty)* | NVR login |
| `nvr.password` | *(empty)* | NVR password |

Camera-specific config is in [hikvision-downloader.md](hikvision-downloader.md#camera-configuration).
Extraction settings are in [frame-extraction.md](frame-extraction.md#configuration).

## Dependencies

### Go Backend

```
go 1.22+
github.com/go-chi/chi/v5        # HTTP router
github.com/go-chi/cors           # CORS middleware
gopkg.in/yaml.v3                 # YAML config parsing
modernc.org/sqlite               # Pure-Go SQLite driver (no CGO)
```

External tools (must be on PATH):
- `ffmpeg` — frame extraction

### Python ML Sidecar

```
fastapi
uvicorn[standard]
torch
open-clip-torch
face-recognition
numpy
Pillow
scikit-learn                     # agglomerative clustering for face discovery
```

### Frontend

```
react
react-dom
typescript
vite
tailwindcss
@tanstack/react-query
react-i18next             # internationalization (EN / PL)
i18next
date-fns
```

## Implementation Phases

### Phase 1: Design Doc
- This document and linked component docs.

### Phase 2: Frame Extractor (Go)
- Implement ffmpeg-based frame extraction (subprocess)
- Implement pHash de-duplication
- YAML config loading
- CLI entry point for manual testing
- Videos are manually placed in `data/videos/` (Hikvision download deferred to Phase 9)

### Phase 3: Python ML Sidecar + Indexing
- Python FastAPI sidecar with /encode/image, /encode/text, /detect/faces
- Search endpoints in ML sidecar: /search/image, /search/face
- SQLite schema creation (clip_embeddings, face_embeddings tables)
- Go SQLite client for embedding storage
- Go client for ML sidecar (encode + search)
- Batch indexing pipeline in Go

### Phase 4: Go Backend API + Pipeline Orchestration
- Go HTTP server with process + search + camera + face endpoints
- SQLite initialization on startup
- On-demand pipeline: extract → call ML sidecar → store in SQLite
- SSE progress streaming to frontend
- Process history tracking (skip already-indexed camera+date combos)
- Face registry CRUD
- Static file serving for frames/videos

### Phase 5: Web UI
- React app with Vite + Tailwind
- Main page: camera/date selector → progress bar → search
- Camera dashboard with live snapshots
- Face registry management page

### Phase 6: Polish + Hardening
- Authentication (API key or basic auth middleware in Go)
- Error handling and retry logic
- Performance tuning (batch sizes, GPU acceleration)
- Automated data retention cleanup (configurable N-day rolling window)
- Docker Compose for deployment (Go backend + ML sidecar + frontend)

### Phase 9: Hikvision NVR Integration (Done)
- Hikvision ISAPI client in Go (`backend/services/hikvision.go`) with HTTP digest auth
- Recording search and download via ISAPI (POST with XML body)
- Automated NVR download wired into the process pipeline
- Camera snapshot proxy for thumbnails
- Live stream via RTSP → HLS transcoding (`backend/services/streamer.go`)
- NVR connection settings in SQLite (shared across all Hikvision cameras)
- Camera config in SQLite with web UI management (replaced YAML-based config)

## Data Lifecycle

The system implements a rolling-window data retention policy:

- **Retained data** (N-day window, configurable via `storage.retention_days`):
  - Extracted frames (`data/frames/`)
  - Downloaded videos (`data/videos/`)
  - CLIP embeddings (`clip_embeddings` table in SQLite)
  - Face embeddings (`face_embeddings` table in SQLite)
  - Process history entries

- **Kept indefinitely**:
  - Face registry (`data/face_registry.json`) — named persons and their reference encodings
  - Configuration files

- **Cleanup mechanism**:
  - Automatic: runs on backend startup and periodically (e.g., daily)
  - Manual: `POST /api/cleanup` endpoint or CLI command
  - Deletes SQLite rows with `timestamp` older than N days
  - Deletes frame JPEG files and video MP4 files from corresponding date directories
  - Updates process history to remove purged entries

## Open Questions

- **GPU availability**: Will inference run on CPU or is a GPU available? Affects
  batch sizes and whether face detection can use CNN model.
- **Camera count**: How many cameras? Affects storage requirements.
- **Auth**: Should the web UI require login?
- **Notifications**: Should the system alert on specific events (e.g., unknown person detected)?
