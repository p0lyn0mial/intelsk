# Video Downloader — Hikvision NVR Integration

[Back to main design](DESIGN.md)

## Hikvision ISAPI Integration

The system communicates with a Hikvision NVR over HTTPS using the ISAPI protocol
with HTTP Digest Authentication. The Go client is in `backend/services/hikvision.go`.

### Search Recordings

Search for recordings on a specific channel within a time range.

**Important format requirements** (discovered through testing with DS-7616NXI firmware V4.84):
- `version="1.0"` attribute is required on `<CMSearchDescription>`
- `searchID` must be a UUID in curly braces (e.g., `{CA77BA52-0780-0001-34B2-6120F2501D36}`)
- Use `<trackList>` (not `<trackIDList>`)
- Do **not** include `<?xml?>` declaration or `xmlns` namespace — the NVR's parser rejects both
- Do **not** include `<metadataList>` — not supported on all firmware versions
- The misspelling `<searchResultPostion>` is intentional (matches Hikvision's own ISAPI spec)

```
POST /ISAPI/ContentMgmt/search
Content-Type: application/xml

<CMSearchDescription version="1.0"><searchID>{CA77BA52-0780-0001-34B2-6120F2501D36}</searchID><trackList><trackID>501</trackID></trackList><timeSpanList><timeSpan><startTime>2026-02-21T00:00:00Z</startTime><endTime>2026-02-21T23:59:59Z</endTime></timeSpan></timeSpanList><maxResults>500</maxResults><searchResultPostion>0</searchResultPostion></CMSearchDescription>
```

Track IDs encode the channel: channel 1 = 101, channel 2 = 201, ..., channel 5 = 501.

Response contains `<playbackURI>` for each recording, which is an RTSP URL used
for the download step.

### Download a Clip

Download a recording using the `playbackURI` from search results.

**Important**: Must use `POST` with an XML body. `GET` with query parameters does not
work on this firmware. The `version="1.0"` attribute is required on `<downloadRequest>`.

```
POST /ISAPI/ContentMgmt/download
Content-Type: application/xml

<downloadRequest version="1.0"><playbackURI>rtsp://192.168.31.139/Streaming/tracks/501/?starttime=20260221T014727Z&amp;endtime=20260221T014847Z&amp;name=00000000955000913&amp;size=47656708</playbackURI></downloadRequest>
```

Note: `&` characters in the playbackURI must be XML-escaped as `&amp;`.

Downloads are written to a `.tmp` file first, then renamed to the final path
on success. This prevents partial downloads from poisoning retries.

### Snapshot (Single Frame)

```
GET /ISAPI/Streaming/channels/{channel}01/picture
Authorization: Digest ...
```

### Live Stream

Live streams use RTSP via ffmpeg to transcode to HLS:

```
rtsp://{username}:{password}@{nvr_ip}:{rtsp_port}/Streaming/Channels/{channel}0{stream_type}
```

Stream types: 1 = main stream (high res), 2 = sub stream (low res).

## Camera Configuration

Cameras are stored in SQLite (managed via the web UI), not YAML files.

Each Hikvision camera has type `"hikvision"` and the following config fields:

| Field | Description |
|-------|-------------|
| `nvr_channel` | NVR channel number (e.g., 5 for channel 5 → track 501) |
| `process_on_upload` | Auto-process after upload (bool) |
| `transcode` | Transcode HEVC to H.264 on upload (bool) |

NVR connection settings are global (shared across all Hikvision cameras) and
stored in the `settings` table:

| Setting | Description |
|---------|-------------|
| `nvr.ip` | NVR IP address (HTTPS, port 443) |
| `nvr.rtsp_port` | RTSP port (default: 554) |
| `nvr.username` | NVR username |
| `nvr.password` | NVR password |

These can be configured on the Settings page or via `PUT /api/settings`.

## On-Demand Download Pipeline

Downloads are triggered by the user clicking "Process" in the web UI. The
pipeline for Hikvision cameras is:

1. **Search** — query NVR for recordings in the requested date range
2. **Download** — download each recording clip via ISAPI
   - Filenames use `{HHMM}.mp4` format based on recording start time
   - Filename collisions (same minute) handled with `_1`, `_2` suffixes
   - Stale `.tmp` files from previous failed attempts are cleaned up first
   - Progress events show `"Downloading {camera} {time range} (3/12)..."`
3. **Extract** — extract frames from downloaded videos using ffmpeg
4. **Index** — encode frames with CLIP and store embeddings in SQLite

Files are stored in `data/videos/{camera_id}/{date}/{HHMM}.mp4`.

The frontend shows a scrollable event log with color-coded messages for each
stage (downloading, extracting, indexing, errors).
