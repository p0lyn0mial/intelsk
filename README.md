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

### 2. Place video files

Videos go in `data/videos/{camera_id}/{date}/{HH}00.mp4`:

```bash
mkdir -p data/videos/front_door/2026-02-18
cp your-video.mp4 data/videos/front_door/2026-02-18/0800.mp4
```

The filename encodes the segment hour (e.g. `0800.mp4` = 08:00, `1400.mp4` = 14:00).

### 3. Run everything

```bash
make run
```

This starts three processes:
- **ML sidecar** on `:8001` (CLIP model loading takes a moment on first start)
- **Go backend** on `:8000` (API server)
- **Frontend dev server** on `:5173` (proxies `/api` to the backend)

Open http://localhost:5173 to use the web UI.

You can add new videos at any time — just drop them in the right directory and
click Process again. The system detects new files and only processes what's new.

To start fresh (remove extracted frames, database, and process history):

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
  mlservice/             # Python ML sidecar
    main.py              # FastAPI app
    clip_encoder.py      # MobileCLIP2 image/text encoding
    searcher.py          # CLIP cosine similarity search
    requirements.txt
    run.sh
  frontend/              # React web UI
    src/
      pages/             # MainPage (process + search), CamerasPage, SettingsPage
      components/        # NavBar, ResultCard, VideoPlayerModal, PlayButtonOverlay
      api/               # API client + TypeScript types
      i18n/              # EN/PL translations
      hooks/             # useVideoPlayer
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
| GET | `/api/cameras` | List discovered cameras |
| GET | `/api/videos/{video_id}/play` | Stream video with seeking |
| GET | `/api/frames/*` | Serve frame images |

## Configuration

YAML files in `config/` provide startup defaults for infrastructure settings.
Extraction, search, and CLIP parameters are **configurable at runtime** via the
Settings page (`/settings`) or the `PUT /api/settings` API — changes are
persisted in SQLite and take effect immediately without restarting.

### config/app.yaml

```yaml
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
  batch_size: 32          # runtime-configurable

process:
  history_path: data/process_history.json
```

### config/extraction.yaml

```yaml
extraction:
  method: time
  time_interval_sec: 5    # runtime-configurable
  output_format: jpg
  output_quality: 85      # runtime-configurable
  dedup_enabled: true      # runtime-configurable
  dedup_phash_threshold: 8 # runtime-configurable
  storage_path: data/frames
```

### Runtime settings

These settings can be changed in the UI at `/settings`:

| Setting | Default | Range |
|---------|---------|-------|
| `search.min_score` | 0.18 | 0.0 - 1.0 |
| `search.default_limit` | 20 | 1 - 500 |
| `extraction.time_interval_sec` | 5 | 1 - 3600 |
| `extraction.output_quality` | 85 | 1 - 100 |
| `extraction.dedup_enabled` | true | — |
| `extraction.dedup_phash_threshold` | 8 | 0 - 64 |
| `clip.batch_size` | 32 | 1 - 256 |

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
