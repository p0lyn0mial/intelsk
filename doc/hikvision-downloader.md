# Video Downloader — Hikvision Integration

[Back to main design](DESIGN.md)

## Hikvision ISAPI Integration

All Hikvision cameras expose an HTTP API (ISAPI) using Digest Authentication.

### Search Recordings

```
POST /ISAPI/ContentMgmt/search
Content-Type: application/xml

<?xml version="1.0" encoding="UTF-8"?>
<CMSearchDescription>
  <searchID>unique-uuid</searchID>
  <trackList>
    <trackID>101</trackID>
  </trackList>
  <timeSpanList>
    <timeSpan>
      <startTime>2026-02-18T00:00:00Z</startTime>
      <endTime>2026-02-18T23:59:59Z</endTime>
    </timeSpan>
  </timeSpanList>
  <maxResults>100</maxResults>
  <searchResultPosition>0</searchResultPosition>
  <metadataList>
    <metadataDescriptor>//recordType.meta.std-cgi.com</metadataDescriptor>
  </metadataList>
</CMSearchDescription>
```

### Download a Clip

```
GET /ISAPI/ContentMgmt/download
Content-Type: application/xml

<?xml version="1.0" encoding="UTF-8"?>
<downloadRequest>
  <playbackURI>rtsp://ip/Streaming/tracks/101?starttime=...&endtime=...</playbackURI>
</downloadRequest>
```

Alternative — direct RTSP recording via ffmpeg:
```bash
ffmpeg -rtsp_transport tcp \
  -i "rtsp://user:pass@192.168.1.64:554/Streaming/Channels/101" \
  -c copy -t 3600 output.mp4
```

### Snapshot (Single Frame)

```
GET /ISAPI/Streaming/channels/101/picture
Authorization: Digest ...
```

## Camera Configuration

```yaml
# config/cameras.yaml
cameras:
  - id: front_door
    name: "Front Door"
    ip: 192.168.1.64
    port: 80
    rtsp_port: 554
    username: admin
    password: "${CAMERA_FRONT_DOOR_PASSWORD}"  # env var reference
    channels:
      main: 101    # full resolution
      sub: 102     # lower resolution (for motion detection)
    timezone: "Europe/Warsaw"

  - id: driveway
    name: "Driveway"
    ip: 192.168.1.65
    port: 80
    rtsp_port: 554
    username: admin
    password: "${CAMERA_DRIVEWAY_PASSWORD}"
    channels:
      main: 101
      sub: 102
    timezone: "Europe/Warsaw"
```

## On-Demand Download

Downloads are triggered by the user, not scheduled. The user selects cameras and a
date range in the web UI, which calls the backend to start the pipeline.

- Backend queries the camera for recordings in the requested time window
- Downloads clips sequentially per camera, in 1-hour chunks to avoid timeouts
- Skips download if clips for that camera+hour already exist on disk
- Stores raw clips in `data/videos/{camera_id}/{date}/{HH}00.mp4`
- Reports download progress back to the UI via SSE (Server-Sent Events)

```yaml
# config/download.yaml
download:
  method: isapi          # isapi | rtsp
  format: mp4
  chunk_hours: 1         # download in 1-hour segments
  storage_path: data/videos
```
