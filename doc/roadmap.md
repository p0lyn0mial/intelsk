# Implementation Roadmap

[Back to main design](DESIGN.md)

## Phase 1: Design Doc

- [x] System architecture and component breakdown
- [x] Hikvision ISAPI integration spec
- [x] Frame extraction strategies
- [x] Indexing pipeline and vector DB schema
- [x] Backend API design
- [x] Web UI wireframes
- [x] Face enrollment spec
- [x] Split into focused sub-documents
- [x] Go backend + Python ML sidecar architecture

---

## Phase 2: Video Downloader + Frame Extractor (Go)

Goal: download video from a Hikvision camera for a given date and extract frames,
runnable from the command line.

### 2.1 Go Project Setup
- [ ] `go mod init` in `backend/`
- [ ] Add dependencies: chi, yaml.v3
- [ ] YAML config loader with `${ENV_VAR}` substitution (`backend/config/config.go`)
- [ ] Create sample `config/cameras.yaml` and `config/extraction.yaml`
- [ ] Define shared types (`backend/models/types.go`)

### 2.2 Hikvision ISAPI Client (`backend/services/downloader.go`)
- [ ] HTTP Digest Auth using Go's `net/http` + digest auth library
- [ ] `SearchRecordings(camera, start, end)` — POST to `/ISAPI/ContentMgmt/search`,
      parse XML response
- [ ] `DownloadClip(camera, start, end, outputPath)` — download via ISAPI or
      RTSP+ffmpeg fallback
- [ ] `GetSnapshot(camera)` — GET single frame from camera
- [ ] Chunked download: split date range into 1-hour segments
- [ ] Skip existing files (resume support)
- [ ] Error handling: connection timeouts, auth failures, no recordings found

### 2.3 Frame Extractor (`backend/services/extractor.go`)
- [ ] `ExtractFramesTime(video, outputDir, intervalSec)` — ffmpeg subprocess
- [ ] `ExtractFramesMotion(video, outputDir, threshold, minGap)` — ffmpeg + Go
      image analysis (or delegate to a small Python/OpenCV helper)
- [ ] pHash de-duplication using a Go perceptual hash library
- [ ] Generate `FrameMetadata` for each extracted frame
- [ ] Write metadata sidecar (JSON manifest per video)

### 2.4 CLI Entry Point (`backend/main.go`)
- [ ] `go run ./backend download --camera front_door --date 2026-02-18`
- [ ] `go run ./backend extract --video data/videos/front_door/2026-02-18/0800.mp4`
- [ ] `go run ./backend process --camera front_door --date 2026-02-18`
      (download + extract in one step)

### 2.5 Verification
- [ ] Download a real clip from a test camera
- [ ] Extract frames with time-based method, verify output
- [ ] Verify de-duplication reduces frame count on static footage

---

## Phase 3: Python ML Sidecar + CLIP Indexing

Goal: Python sidecar for CLIP inference and search, Go client to call it, SQLite storage.

### 3.1 Python ML Sidecar (`mlservice/`)
- [ ] FastAPI app with `/encode/image`, `/encode/text`, `/health`
- [ ] `CLIPEncoder` class — load MobileCLIP2-S0, batch image encoding, text encoding
- [ ] `Searcher` class — load embeddings from SQLite, brute-force NumPy search
- [ ] `POST /search/image` endpoint — text-to-image search (CLIP cosine similarity)
- [ ] `requirements.txt` (fastapi, uvicorn, torch, open-clip-torch, numpy, Pillow, scikit-learn)
- [ ] `run.sh` startup script

### 3.2 Go ML Client (`backend/services/mlclient.go`)
- [ ] HTTP client struct with base URL config
- [ ] `EncodeImages(paths) → [][]float64`
- [ ] `EncodeText(text) → []float64`
- [ ] `SearchByText(dbPath, text, filters, limit) → []SearchResult`
- [ ] Timeout and retry handling
- [ ] Health check on startup (wait for sidecar ready)

### 3.3 SQLite Storage (`backend/services/storage.go`)
- [ ] SQLite schema creation (`clip_embeddings`, `face_embeddings` tables)
- [ ] `NewStorage(dbPath)` — open DB, run migrations
- [ ] `AddClipEmbedding(id, embedding, metadata)`
- [ ] `Cleanup(olderThan)` — delete rows older than N days
- [ ] Embeddings stored as raw byte blobs (float32 arrays)

### 3.4 Indexing Pipeline (`backend/services/pipeline.go`)
- [ ] Orchestrate: iterate frames → call ML sidecar (batch) → store in SQLite
- [ ] Progress callback (channel) for reporting to caller
- [ ] JSON sidecar to track which frames have been indexed (resume support)

### 3.5 CLI Extension
- [ ] `go run ./backend index --camera front_door --date 2026-02-18`
- [ ] `go run ./backend search --text "delivery person"`

### 3.6 Verification
- [ ] Start ML sidecar, verify `/health` responds
- [ ] Index test images from `experiments/test_images/`
- [ ] Run text search queries, verify results are ranked sensibly

---

## Phase 4: Go Backend API + Pipeline Orchestration

Goal: Go HTTP server exposing the full pipeline and text search.

### 4.1 HTTP Server (`backend/cmd/server/server.go`)
- [ ] Chi router setup with CORS
- [ ] Config loading on startup
- [ ] SQLite initialization on startup
- [ ] ML sidecar health check on startup

### 4.2 Process Endpoint (`backend/api/process.go`)
- [ ] `POST /api/process` — accept camera_ids + date range, start pipeline
- [ ] Run pipeline in a goroutine
- [ ] `GET /api/process/status?job_id=...` — SSE stream with progress events
- [ ] `GET /api/process/history` — list processed camera+date combinations
- [ ] Process history tracking (`data/process_history.json`): skip re-processing

### 4.3 Search Endpoint (`backend/api/search.go`)
- [ ] `POST /api/search/text` — call ML sidecar `/search/image` with DB path + filters

### 4.4 Camera Endpoints (`backend/api/cameras.go`)
- [ ] `GET /api/cameras` — list configured cameras (id, name, status)
- [ ] `GET /api/cameras/{id}/snapshot` — proxy live snapshot from camera

### 4.5 Static Files
- [ ] Serve `data/frames/` and `data/videos/` via `http.FileServer`
- [ ] Path validation to prevent directory traversal

### 4.6 Verification
- [ ] Start Go server + ML sidecar, hit `/api/health`
- [ ] Process a camera+date via API, observe SSE progress
- [ ] Search via API, verify results match CLI output

---

## Phase 5: Web UI

Goal: React frontend for the process + text search workflow.

### 5.1 Project Setup
- [ ] `npm create vite@latest frontend -- --template react-ts`
- [ ] Install Tailwind CSS, React Query, date-fns, react-i18next, i18next
- [ ] API client module (`frontend/src/api/client.ts`)
- [ ] TypeScript types matching Go backend JSON (`frontend/src/api/types.ts`)
- [ ] i18n setup: `frontend/src/i18n/i18n.ts`, `en.json`, `pl.json`
- [ ] Language switcher component (EN/PL toggle in nav bar, persists to localStorage)

### 5.2 Main Page — Process & Search (`MainPage.tsx`)
- [ ] Camera selector (checkboxes from `/api/cameras`)
- [ ] Date range picker
- [ ] "Process" button → POST `/api/process`, connect to SSE for progress
- [ ] Progress bar component with stage labels
- [ ] Search bar (text input), enabled after processing

### 5.3 Results Grid
- [ ] Thumbnail grid with camera name, timestamp, score
- [ ] Play button overlay on thumbnails (`PlayButtonOverlay.tsx`)
- [ ] Click to expand: larger image, metadata
- [ ] Lazy loading / pagination for large result sets

### 5.4 Video Playback
- [ ] Backend: `GET /api/videos/{video_id}/play` handler using `http.ServeFile` (supports Range requests)
- [ ] Backend: path validation against directory traversal on video ID
- [ ] Backend: compute `seek_offset_sec` in search result mapping (frame timestamp minus segment start hour)
- [ ] Backend: construct `source_video_url` from ML sidecar `source_video` field (encode as video ID)
- [ ] Backend: face search join — retrieve `source_video` from `clip_embeddings` by `frame_path`
- [ ] Frontend: `PlayButtonOverlay.tsx` — semi-transparent play icon on `ResultCard`, hover reveal
- [ ] Frontend: `VideoPlayerModal.tsx` — HTML5 `<video>` in modal, dark backdrop, close on Escape/backdrop
- [ ] Frontend: on `loadedmetadata`, set `video.currentTime = seekOffsetSec`; autoplay muted
- [ ] Frontend: `useVideoPlayer.ts` hook — manages modal open/close state + video props
- [ ] Frontend: update `ResultCard.tsx` to render `PlayButtonOverlay`
- [ ] Frontend: add i18n keys (`video.play`, `video.close`, `video.loading`, `video.error`, `video.camera`, `video.timestamp`, `video.seek_hint`) in EN + PL
- [ ] Frontend: add `seek_offset_sec` and `source_video_url` to TypeScript `SearchResult` type
- [ ] Verification: end-to-end test — search → click play → video opens seeked to correct time
- [ ] Verification: Range request seeking works (seek forward/backward in player)
- [ ] Verification: 404 handled gracefully when video purged by retention cleanup

### 5.5 Camera Dashboard (`CamerasPage.tsx`)
- [ ] Grid of snapshot thumbnails, auto-refresh every 10s
- [ ] Online/offline indicator per camera
- [ ] Click camera → navigate to main page with that camera pre-selected

### 5.6 Navigation
- [ ] Top nav bar: CCTV Intelligence | Cameras | [EN|PL]
- [ ] React Router for page navigation
- [ ] All UI strings via `useTranslation()` hook (no hardcoded text)

### 5.7 Verification
- [ ] Full end-to-end: select camera + date → process → search → view results
- [ ] Camera dashboard shows live snapshots

---

## Phase 6: Polish + Hardening

Goal: production readiness.

### 6.1 Authentication
- [ ] API key or basic auth middleware in Go
- [ ] Login page in frontend (if basic auth)

### 6.2 Error Handling
- [ ] Retry logic for camera connections (3 attempts with backoff)
- [ ] Graceful error display in UI (camera offline, download failed, etc.)
- [ ] Pipeline error recovery (partial indexing, resume from failure)
- [ ] ML sidecar reconnection if it restarts

### 6.3 Performance
- [ ] GPU acceleration toggle (cuda/mps) tested and documented
- [ ] Batch size tuning for different hardware
- [ ] Concurrent camera downloads (goroutines)

### 6.4 Deployment
- [ ] `Dockerfile` for Go backend (multi-stage: build + runtime with ffmpeg)
- [ ] `Dockerfile` for ML sidecar (Python + torch)
- [ ] `Dockerfile` for frontend (Node build + nginx)
- [ ] `docker-compose.yaml` (Go backend + ML sidecar + frontend)
- [ ] Volume mounts for `data/` and `config/`
- [ ] Environment variable documentation

### 6.5 Data Retention + Cleanup
- [ ] `backend/services/cleanup.go` — delete old embeddings, frames, videos
- [ ] Automatic cleanup on backend startup (based on `storage.retention_days`)
- [ ] Periodic cleanup (configurable interval, e.g., daily)
- [ ] `POST /api/cleanup` endpoint for manual trigger
- [ ] CLI command: `go run ./backend cleanup --older-than 30d`
- [ ] Cleanup skips face registry and assigned face embeddings
- [ ] Disk usage display in UI

---

## Phase 7: Polish Language Query Translation

Goal: users type queries in Polish, which are translated to English before
being sent to CLIP. CLIP only understands English well, so translation is
required for accurate results.

### 7.1 Translation Approach
- [ ] Evaluate options:
  - **Local model**: Helsinki-NLP/opus-mt-pl-en (lightweight, no API key, runs
    in the ML sidecar)
  - **LLM API**: OpenAI / Anthropic API call for translation (higher quality,
    requires API key, adds latency + cost)
  - **Google Translate API**: reliable, requires API key
- [ ] Pick approach and implement

### 7.2 ML Sidecar — `/translate` Endpoint
- [ ] Add `POST /translate` endpoint to ML sidecar
  ```
  POST /translate
  {"text": "dostawa paczki", "source": "pl", "target": "en"}
  → {"translated": "package delivery"}
  ```
- [ ] If using local model: load opus-mt-pl-en on startup, add `transformers`
      and `sentencepiece` to `requirements.txt`
- [ ] If using external API: HTTP client call with API key from config

### 7.3 Go Backend Integration
- [ ] Add `Translate(text, sourceLang, targetLang) → string` to Go ML client
- [ ] Update `POST /api/search/text` handler: detect Polish input (or always
      translate), call `/translate`, then call `/encode/text` with English result
- [ ] Return both original query and translated query in search response so the
      user can see what CLIP actually searched for

### 7.4 Web UI Update
- [ ] Show translated query below search bar (e.g., "Searching for: package delivery")
- [ ] Optional: language toggle if users sometimes want to type in English directly

### 7.5 Verification
- [ ] Search "samochód na podjeździe" → should match car/vehicle frames
- [ ] Search "osoba z paczką" → should match delivery frames
- [ ] Compare result quality: Polish direct vs. translated English

---

## Phase 8: Face Recognition (Future)

Goal: add person identification alongside text search, using discovery-based enrollment.

### 8.1 ML Sidecar Extension
- [ ] Add `/detect/faces` endpoint to ML sidecar
- [ ] `FaceEncoder` class — face_recognition HOG detection + 128-dim encoding
- [ ] Add `face-recognition` to `requirements.txt`
- [ ] Add `/search/face` endpoint — L2 distance search over SQLite face embeddings
- [ ] Add `/cluster/faces` endpoint — agglomerative clustering on unassigned faces
- [ ] Add `scikit-learn` to `requirements.txt`

### 8.2 SQLite Face Storage
- [ ] `AddFaceEmbedding(id, embedding, metadata)` in Go SQLite client
- [ ] `face_embeddings` table with `person_name` column (NULL = unassigned)
- [ ] Index on `person_name` for filtering unassigned faces

### 8.3 Go ML Client Extension
- [ ] `DetectFaces(path) → []Face`
- [ ] `SearchByFace(dbPath, encoding, filters, threshold, limit) → []SearchResult`
- [ ] `ClusterFaces(dbPath, distanceThreshold) → []FaceCluster`

### 8.4 Indexing Pipeline Update
- [ ] Run face detection alongside CLIP encoding during indexing
- [ ] Store face embeddings in SQLite `face_embeddings` table (person_name = NULL)

### 8.5 Face Discovery + Registry
- [ ] `POST /api/faces/discover` — trigger face clustering via ML sidecar
- [ ] `GET /api/faces/clusters` — return discovered clusters to UI
- [ ] `POST /api/faces/clusters/{id}/assign` — assign name, update registry + SQLite
- [ ] Face registry management in Go (`backend/services/faceregistry.go`)
- [ ] `POST /api/search/person/{name}` — average encodings, search via ML sidecar
- [ ] `DELETE /api/faces/registry/{name}` — remove person from registry

### 8.6 Web UI — Face Features
- [ ] Face Registry page with "Discover Faces" button
- [ ] Cluster review grid: representative thumbnails, name input, assign button
- [ ] Enrolled persons list with face count and delete option
- [ ] Person search toggle on main page
- [ ] Person dropdown from `/api/faces/registry`
- [ ] Add "Faces" to navigation bar
- [ ] i18n keys for face discovery (EN + PL)

### 8.7 Verification
- [ ] Process footage with faces, verify face embeddings stored in SQLite
- [ ] Trigger face discovery, verify clusters make sense
- [ ] Assign a name to a cluster, verify registry updated
- [ ] Search for an enrolled person, verify results
- [ ] Index test faces from `experiments/test_faces/`
