# CCTV Video Intelligence System — Design Document

## Problem Statement

Searching through CCTV footage is manual and time-consuming. A user should be able to ask
"show me when Lukasz arrived" or "show all deliveries today" and get timestamped results
instantly. This system combines text-based scene search (CLIP) with person identification
(face recognition) over Hikvision camera footage, exposed through a web UI.

## Architecture

Two-process design: a **Go backend** handles HTTP serving, video downloading, frame
extraction, and vector DB queries. A **Python ML sidecar** handles CLIP and face
inference (the only parts that need PyTorch and dlib).

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
      | MP4 clips               | JPEG frames     +--------+----------+
      v                         v                           |
+-----+------+        +---------+--------+         embeddings (JSON)
| data/      |        | data/            |                  |
| videos/    |        | frames/          |                  v
+------------+        +------------------+        +---------+--------+
                                                  |    ChromaDB      |
                                                  | clip_embeddings  |
                          Processing complete,    | face_embeddings  |
                          user can now search     +--------+---------+
                                                           ^
                                                           |
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
- Pipeline orchestration (download → extract → call ML sidecar → store in ChromaDB)
- ChromaDB queries (via HTTP client)
- SSE progress streaming
- Static file serving (frames, videos)
- Face registry CRUD (JSON file)
- YAML config loading

### Python ML Sidecar Responsibilities
- CLIP image encoding (MobileCLIP2-S0 via open_clip)
- CLIP text encoding
- Face detection + encoding (face_recognition / dlib)
- Stateless — receives file paths or text, returns embeddings

## User Workflow

1. User opens the web UI, selects one or more cameras and a date (or date range)
2. Clicks "Process" — backend downloads video from the selected cameras for that period
3. UI shows a progress bar: downloading → extracting frames → indexing
4. Once complete, the search bar becomes active for that dataset
5. Previously processed camera+date combinations are cached — re-selecting them
   skips straight to search

## Component Design Docs

| Component | Document | Description |
|-----------|----------|-------------|
| Video Downloader | [hikvision-downloader.md](hikvision-downloader.md) | Hikvision ISAPI integration, camera config, on-demand download |
| Frame Extractor | [frame-extraction.md](frame-extraction.md) | Time-based and motion-based extraction, de-duplication |
| Indexing + Search | [indexing-and-search.md](indexing-and-search.md) | Python ML sidecar API, CLIP encoding, face detection, ChromaDB |
| Backend API | [backend-api.md](backend-api.md) | Go HTTP endpoints, request/response types, SSE progress |
| Web UI | [web-ui.md](web-ui.md) | React frontend, pages, layout wireframe |
| Face Enrollment | [face-enrollment.md](face-enrollment.md) | Face registry format, enrollment flow, matching logic |
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
    cameras.yaml                 # camera definitions
    extraction.yaml              # frame extraction settings
    download.yaml                # download settings
    app.yaml                     # app + model + DB config
  backend/                       # Go backend
    go.mod
    go.sum
    main.go                      # entrypoint
    cmd/
      server/server.go           # HTTP server setup
    api/
      process.go                 # process pipeline endpoint + SSE progress
      search.go                  # search endpoints
      cameras.go                 # camera endpoints
      faces.go                   # face registry endpoints
    services/
      downloader.go              # Hikvision ISAPI client
      extractor.go               # frame extraction (ffmpeg subprocess)
      mlclient.go                # HTTP client for Python ML sidecar
      pipeline.go                # orchestrates download → extract → index
      faceregistry.go            # face enrollment logic (JSON file)
      vectordb.go                # ChromaDB HTTP client
    config/
      config.go                  # YAML config loader
    models/
      types.go                   # shared types / structs
  mlservice/                     # Python ML sidecar
    main.py                      # FastAPI app (3 endpoints)
    clip_encoder.py              # CLIP model loading + encoding
    face_encoder.py              # face detection + encoding
    requirements.txt
  frontend/
    package.json
    vite.config.ts
    src/
      App.tsx
      pages/
        MainPage.tsx             # process + search (two-phase)
        CamerasPage.tsx
        FacesPage.tsx
      components/
        CameraSelector.tsx
        DatePicker.tsx
        ProcessProgress.tsx
        SearchBar.tsx
        ResultsGrid.tsx
        ResultCard.tsx
        FrameDetail.tsx
        CameraGrid.tsx
        FaceRegistry.tsx
      api/
        client.ts                # API client (fetch wrapper)
        types.ts                 # TypeScript types
      hooks/
        useProcess.ts
        useSearch.ts
        useCameras.ts
  experiments/                   # previous prototypes
    demo_search.py               # CLI CLIP search tool
    DEMO.md
    desktop/                     # tkinter desktop app
    test_faces/                  # face recognition test images
    test_images/                 # CLIP search test images
    test_images_large/           # larger test dataset
  data/                          # gitignored, runtime data
    videos/                      # downloaded MP4s
    frames/                      # extracted JPEGs
    faces/                       # enrolled face photos
    chromadb/                    # vector DB persistence
    face_registry.json           # enrolled persons
    process_history.json         # tracks which camera+date combos are indexed
```

## Configuration

All configuration lives in `config/` as YAML files. Sensitive values (passwords)
are referenced via environment variables using `${VAR_NAME}` syntax, resolved at startup.

```yaml
# config/app.yaml
app:
  host: 0.0.0.0
  port: 8000
  data_dir: data
  log_level: info

mlservice:
  url: http://localhost:8001     # Python ML sidecar address

clip:
  model: MobileCLIP2-S0          # open_clip model name
  pretrained: dfndr2b            # pretrained weights tag
  image_mean: [0, 0, 0]         # custom normalization for S0/S2
  image_std: [1, 1, 1]
  device: cpu                    # cpu | cuda | mps
  batch_size: 32

face:
  detection_model: hog           # hog (CPU) | cnn (GPU)
  match_threshold: 0.6           # Euclidean distance threshold
  registry_path: data/face_registry.json

chromadb:
  url: http://localhost:8002     # ChromaDB server (or embedded path)
  clip_collection: clip_embeddings
  face_collection: face_embeddings

process:
  history_path: data/process_history.json
```

Camera-specific config is in [hikvision-downloader.md](hikvision-downloader.md#camera-configuration).
Extraction settings are in [frame-extraction.md](frame-extraction.md#configuration).

## Dependencies

### Go Backend

```
go 1.22+
github.com/go-chi/chi/v5        # HTTP router
github.com/go-chi/cors           # CORS middleware
gopkg.in/yaml.v3                 # YAML config parsing
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
```

### ChromaDB

Run as a separate process or Docker container:
```
chromadb                         # pip install chromadb
# or: docker run -p 8002:8000 chromadb/chroma
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

### Phase 2: Video Downloader + Frame Extractor (Go)
- Implement Hikvision ISAPI client in Go (HTTP digest auth)
- Implement ffmpeg-based frame extraction (subprocess)
- Implement pHash de-duplication
- YAML config loading
- CLI entry point for manual testing

### Phase 3: Python ML Sidecar + Indexing
- Python FastAPI sidecar with /encode/image, /encode/text, /detect/faces
- ChromaDB setup with two collections
- Go client for ML sidecar
- Go client for ChromaDB HTTP API
- Batch indexing pipeline in Go

### Phase 4: Go Backend API + Pipeline Orchestration
- Go HTTP server with process + search + camera + face endpoints
- On-demand pipeline: download → extract → call ML sidecar → store in ChromaDB
- SSE progress streaming to frontend
- Process history tracking (skip already-indexed camera+date combos)
- Camera snapshot proxy
- Face registry CRUD
- Static file serving for frames/videos

### Phase 5: Web UI
- React app with Vite + Tailwind
- Main page: camera/date selector → progress bar → search
- Camera dashboard with live snapshots
- Face registry management page

### Phase 6: Polish + Hardening
- Authentication (API key or basic auth middleware in Go)
- Error handling and retry logic for camera connections
- Performance tuning (batch sizes, GPU acceleration)
- Docker Compose for deployment (Go backend + ML sidecar + ChromaDB + frontend)

## Open Questions

- **GPU availability**: Will inference run on CPU or is a GPU available? Affects
  batch sizes and whether face detection can use CNN model.
- **Camera count**: How many cameras? Affects storage requirements.
- **Auth**: Should the web UI require login?
- **Cleanup**: Should there be a way to delete previously processed data from the UI
  (free disk space), or is that a manual operation?
- **Notifications**: Should the system alert on specific events (e.g., unknown person detected)?
