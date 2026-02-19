# intelsk — CCTV Video Intelligence System

Search CCTV footage by natural language. Ask "show me when Lukasz arrived" or
"show all deliveries today" and get timestamped results. Combines CLIP text-to-image
search with face recognition over camera footage.

See [doc/DESIGN.md](doc/DESIGN.md) for the full architecture.

## Prerequisites

- **Go 1.22+**
- **ffmpeg** — used for video frame extraction

### Install ffmpeg

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Verify
ffmpeg -version
```

## Project Structure

```
intelsk/
  config/              # YAML configuration files
    app.yaml           # app settings (data_dir, log_level)
    extraction.yaml    # frame extraction settings
  backend/             # Go backend
    go.mod
    main.go            # CLI entry point
    config/config.go   # YAML config loader
    models/types.go    # shared types (FrameMetadata)
    services/
      extractor.go     # frame extraction + dedup
  doc/                 # design documents
  data/                # runtime data (gitignored)
    videos/            # source MP4 files
    frames/            # extracted JPEGs + manifests
```

## Getting Started

### 1. Build

```bash
cd backend
go build ./...
```

### 2. Place video files

Videos go in `data/videos/{camera_id}/{date}/{HH}00.mp4`:

```bash
mkdir -p data/videos/front_door/2026-02-18
cp your-video.mp4 data/videos/front_door/2026-02-18/0800.mp4
```

The filename encodes the segment hour (e.g. `0800.mp4` = 08:00, `1400.mp4` = 14:00).

### 3. Extract frames from a single video

```bash
cd backend
go run . extract -root .. -video ../data/videos/front_door/2026-02-18/0800.mp4
```

### 4. Process all videos for a camera + date

```bash
cd backend
go run . process -root .. -camera front_door -date 2026-02-18
```

Both commands will:
1. Extract JPEG frames at the configured interval (default: every 5 seconds)
2. Run pHash de-duplication to remove near-identical frames (if enabled)
3. Write a `manifest.json` with metadata for each retained frame

Output goes to `data/frames/{camera_id}/{date}/`.

## Configuration

### config/app.yaml

```yaml
app:
  data_dir: data       # root directory for videos/frames
  log_level: info
```

### config/extraction.yaml

```yaml
extraction:
  method: time                # time-based extraction (motion deferred to Phase 3)
  time_interval_sec: 5        # extract 1 frame every N seconds
  motion_threshold: 0.01      # reserved for motion-based extraction
  min_gap_sec: 2.0            # reserved for motion-based extraction
  output_format: jpg
  output_quality: 85          # JPEG quality (2 = best in ffmpeg scale)
  dedup_enabled: true         # enable pHash de-duplication
  dedup_phash_threshold: 8    # hamming distance threshold (lower = stricter)
  storage_path: data/frames   # output directory for extracted frames
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

## Development Roadmap

See [doc/roadmap.md](doc/roadmap.md) for the full phase breakdown.

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | Done | Design documents |
| 2 | Done | Frame extractor (Go CLI) |
| 3 | Planned | Python ML sidecar + CLIP indexing |
| 4 | Planned | Go backend API + pipeline |
| 5 | Planned | Web UI (React) |
| 6 | Planned | Polish + hardening + Docker |
| 7 | Planned | Polish language query translation |
| 8 | Planned | Face recognition |
| 9 | Planned | Hikvision camera integration |
