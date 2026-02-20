# intelsk — CCTV Video Intelligence System

Search CCTV footage by natural language. Ask "show me when Lukasz arrived" or
"show all deliveries today" and get timestamped results. Combines CLIP text-to-image
search with face recognition over camera footage.

See [doc/DESIGN.md](doc/DESIGN.md) for the full architecture.

## Prerequisites

- **Go 1.22+**
- **Python 3.10+** — for the ML sidecar
- **Node.js 18+** — for the frontend
- **ffmpeg** — for video frame extraction

### Install dependencies

```bash
# macOS
brew install ffmpeg go node python

# Ubuntu/Debian
sudo apt install ffmpeg golang nodejs python3 python3-venv
```

## Quick Start

### 1. Install dependencies

```bash
make setup
```

This creates the Python venv, installs pip/npm dependencies, and fetches Go modules.

### 2. Run everything

```bash
make run
```

This starts three processes:
- **ML sidecar** on `:8001` (CLIP model loading takes a moment on first start)
- **Go backend** on `:8000` (API server)
- **Frontend dev server** on `:5173` (proxies `/api` to the backend)

Open http://localhost:5173 to use the web UI.

### 3. Create a camera, upload footage, and search

1. Go to the **Cameras** page and click **Add Camera**.
   - Give it an ID (e.g. `front-door`) and a name.
   - The "Transcode H.265 to H.264" checkbox is on by default — leave it
     checked if your camera records in HEVC, so videos play in the browser.
2. On the camera card, click **Upload Video** and select one or more `.mp4`
   files (or a whole directory). A progress bar shows upload status.
   Uploaded files are saved under `data/videos/{camera-id}/{today's date}/`.
3. Click the camera card to open its **detail page**, where you can see all
   uploaded videos grouped by date, delete individual videos, or delete all
   data for the camera.
4. Go to the **Process** page. Today's date is pre-filled. Select your
   camera(s) and click **Process**. This extracts frames and indexes them
   with CLIP (wait for the progress bar to finish).
5. Go to the **Search** page. Type a natural-language query — e.g. "person
   carrying a box" — and click **Search**. Results show matched frames with
   scores; click the play button to jump to the moment in the source video.

To start fresh (remove all data including videos, frames, and database):

```bash
make clean
```

## Project Structure

```
intelsk/
  config/                # YAML configuration files
    app.yaml             # app + server + ML + storage settings
    extraction.yaml      # frame extraction settings
  backend/               # Go backend
    main.go              # CLI entry point (extract, process, index, search, serve)
    cmd/server/          # HTTP server setup (Chi router)
    api/                 # HTTP handlers (process, search, cameras, videos, settings)
    config/config.go     # YAML config loader
    models/types.go      # shared types
    services/
      extractor.go       # frame extraction + dedup
      mlclient.go        # HTTP client for ML sidecar
      storage.go         # SQLite storage (embeddings)
      settings.go        # runtime settings (DB-backed, in-memory cached)
      pipeline.go        # indexing pipeline with resume support
      camera.go          # camera CRUD, video management, data cleanup
  mlservice/             # Python ML sidecar
    main.py              # FastAPI app
    clip_encoder.py      # MobileCLIP2 image/text encoding
    searcher.py          # CLIP cosine similarity search
    requirements.txt
    run.sh
  frontend/              # React web UI
    src/
      pages/             # MainPage (search), CamerasPage, CameraDetailPage, ProcessPage, SettingsPage
      components/        # NavBar, ResultCard, VideoPlayerModal, CameraModals
      api/               # API client + TypeScript types
      i18n/              # EN/PL translations
      hooks/             # useVideoPlayer, useSearchHistory
  doc/                   # design documents
  data/                  # runtime data (gitignored)
    videos/              # source MP4 files
    frames/              # extracted JPEGs + manifests
    intelsk.db           # SQLite database (embeddings)
```

## CLI Reference

### `extract` — Extract frames from a single video

```
Usage: backend extract [flags]
  -video string   Path to video file (required)
  -root string    Project root directory (default: auto-detected)
```

### `process` — Extract frames from all videos for a camera + date

```
Usage: backend process [flags]
  -camera string   Camera ID (required)
  -date string     Date in YYYY-MM-DD format (required)
  -root string     Project root directory (default: auto-detected)
```

### `index` — Index extracted frames via CLIP embeddings

```
Usage: backend index [flags]
  -camera string   Camera ID (required)
  -date string     Date in YYYY-MM-DD format (required)
  -root string     Project root directory (default: auto-detected)
```

Requires the ML sidecar to be running.

### `search` — Search indexed frames by text query

```
Usage: backend search [flags]
  -text string     Text query (required)
  -camera string   Camera ID filter (optional)
  -limit int       Max results (default: 20)
  -root string     Project root directory (default: auto-detected)
```

Requires the ML sidecar to be running.

### `serve` — Start the HTTP API server

```
Usage: backend serve [flags]
  -root string   Project root directory (default: auto-detected)
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check (includes ML sidecar status) |
| POST | `/api/process` | Start extract + index pipeline |
| GET | `/api/process/status?job_id=` | SSE progress stream |
| GET | `/api/process/history` | List processed camera+date combos |
| POST | `/api/search/text` | CLIP text search |
| GET | `/api/settings` | Get all settings (with defaults) |
| PUT | `/api/settings` | Update settings |
| GET | `/api/cameras` | List cameras |
| GET | `/api/cameras/{id}` | Get camera by ID |
| POST | `/api/cameras` | Create camera |
| PUT | `/api/cameras/{id}` | Update camera |
| DELETE | `/api/cameras/{id}` | Delete camera |
| GET | `/api/cameras/{id}/stats` | Per-date video/frame counts |
| GET | `/api/cameras/{id}/videos` | List video files for camera |
| DELETE | `/api/cameras/{id}/videos` | Delete a single video file |
| DELETE | `/api/cameras/{id}/data` | Delete all data (videos, frames, embeddings) |
| POST | `/api/cameras/{id}/upload` | Upload .mp4 files |
| GET | `/api/videos/{video_id}/play` | Stream video with seeking |
| GET | `/api/frames/*` | Serve frame images |

## Configuration

Most settings are stored in **SQLite** and editable at runtime through the
Settings page (`/settings`) or `PUT /api/settings`. Settings auto-save after
changes and take effect immediately without a restart.

| Setting | Default | Range |
|---------|---------|-------|
| `general.system_name` | CCTV Intelligence | — |
| `search.min_score` | 0.18 | 0.0 - 1.0 |
| `search.default_limit` | 20 | 1 - 500 |
| `extraction.time_interval_sec` | 5 | 1 - 3600 |
| `extraction.output_quality` | 85 | 1 - 100 |
| `extraction.dedup_enabled` | true | — |
| `extraction.dedup_phash_threshold` | 8 | 0 - 64 |
| `clip.batch_size` | 32 | 1 - 256 |

### Startup config (YAML)

Two YAML files in `config/` set infrastructure paths and network addresses that
are read once at startup. You typically don't need to change these.

- **`config/app.yaml`** — listen address/port, data directory, ML sidecar URL,
  SQLite path, process history path.
- **`config/extraction.yaml`** — extraction method, output format, frames
  storage path. The tunable parameters (interval, quality, dedup) are seeded
  from here on first run but afterwards controlled via the database.

## Development Roadmap

See [doc/roadmap.md](doc/roadmap.md) for the full phase breakdown.

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | Done | Design documents |
| 2 | Done | Frame extractor (Go CLI) |
| 3 | Done | Python ML sidecar + CLIP indexing |
| 4 | Done | Go backend API + pipeline |
| 5 | Done | Web UI (React) |
| 6 | Planned | Polish + hardening + Docker |
| 7 | Planned | Polish language query translation |
| 8 | Planned | Face recognition |
| 9 | Planned | Hikvision camera integration |
