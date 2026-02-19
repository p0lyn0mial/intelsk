# Indexing Pipeline & Vector Search

[Back to main design](DESIGN.md)

## Python ML Sidecar API

The ML sidecar is a small FastAPI service that handles all ML inference and
vector search. The Go backend calls it over HTTP. It receives file paths, text,
or a database path and returns embeddings, search results, or face clusters.

### Endpoints

```
POST /encode/image       Batch CLIP image encoding
POST /encode/text        Single CLIP text encoding
POST /detect/faces       Face detection + encoding for a single image
POST /search/image       Text-to-image search (CLIP cosine similarity)
POST /search/face        Face search (L2 distance against face embeddings)
POST /cluster/faces      Cluster unassigned face embeddings
GET  /health             Health check
```

### Request/Response Examples

**Encode images (batch):**
```
POST /encode/image
{
  "paths": ["/data/frames/front_door/2026-02-18/frame_000001.jpg",
            "/data/frames/front_door/2026-02-18/frame_000002.jpg"]
}

→ 200
{
  "embeddings": [
    [0.123, -0.456, 0.789, ...],   // 512-dim, one per image
    [0.124, -0.455, 0.790, ...]
  ]
}
```

**Encode text:**
```
POST /encode/text
{
  "text": "a delivery person with a package"
}

→ 200
{
  "embedding": [0.123, -0.456, 0.789, ...]   // 512-dim
}
```

**Detect faces:**
```
POST /detect/faces
{
  "path": "/data/frames/front_door/2026-02-18/frame_000001.jpg"
}

→ 200
{
  "faces": [
    {
      "encoding": [0.12, -0.34, ...],   // 128-dim
      "bbox": {"top": 50, "right": 200, "bottom": 180, "left": 80}
    }
  ]
}
```

**Search by text (CLIP):**
```
POST /search/image
{
  "db_path": "/data/intelsk.db",
  "text": "a delivery person with a package",
  "camera_ids": ["front_door"],        // optional filter
  "start_time": "2026-02-18T00:00:00", // optional filter
  "end_time": "2026-02-18T23:59:59",   // optional filter
  "limit": 20
}

→ 200
{
  "results": [
    {
      "id": "front_door_20260218_143005_000042",
      "frame_path": "frames/front_door/2026-02-18/frame_000042.jpg",
      "camera_id": "front_door",
      "timestamp": "2026-02-18T14:30:05",
      "source_video": "videos/front_door/2026-02-18/1400.mp4",
      "score": 0.82
    }
  ]
}
```

**Search by face:**
```
POST /search/face
{
  "db_path": "/data/intelsk.db",
  "encoding": [-0.097, 0.167, 0.157, ...],  // 128-dim averaged face encoding
  "camera_ids": ["front_door"],              // optional filter
  "start_time": "2026-02-18T00:00:00",       // optional filter
  "end_time": "2026-02-18T23:59:59",         // optional filter
  "threshold": 0.6,                          // L2 distance threshold
  "limit": 20
}

→ 200
{
  "results": [
    {
      "id": "front_door_20260218_143005_000042_0",
      "frame_path": "frames/front_door/2026-02-18/frame_000042.jpg",
      "camera_id": "front_door",
      "timestamp": "2026-02-18T14:30:05",
      "bbox": {"top": 50, "right": 200, "bottom": 180, "left": 80},
      "distance": 0.42,
      "score": 0.58
    }
  ]
}
```

#### Face Search `source_video` Lookup

Note that the face search response above does **not** include a `source_video`
field. The `face_embeddings` table lacks a `source_video` column (it was not
needed during indexing — only `clip_embeddings` stores it). When the Go backend
builds `SearchResult` structs from face search results, it joins against
`clip_embeddings` by `frame_path` to retrieve the `source_video` value:

```sql
SELECT ce.source_video
FROM clip_embeddings ce
WHERE ce.frame_path = ?
LIMIT 1
```

This works because every frame that has face embeddings also has a CLIP embedding
(both are generated during the same indexing pass), so the join always succeeds
for non-purged data.

**Cluster unassigned faces:**
```
POST /cluster/faces
{
  "db_path": "/data/intelsk.db",
  "distance_threshold": 0.6
}

→ 200
{
  "clusters": [
    {
      "cluster_id": 0,
      "size": 15,
      "face_ids": ["front_door_20260218_143005_000042_0", ...],
      "representative_faces": [
        {
          "id": "front_door_20260218_143005_000042_0",
          "frame_path": "frames/front_door/2026-02-18/frame_000042.jpg",
          "bbox": {"top": 50, "right": 200, "bottom": 180, "left": 80},
          "camera_id": "front_door",
          "timestamp": "2026-02-18T14:30:05"
        }
      ]
    }
  ]
}
```

### Implementation

```python
# mlservice/main.py
from fastapi import FastAPI
from clip_encoder import CLIPEncoder
from face_encoder import FaceEncoder
from searcher import Searcher

app = FastAPI()
clip = CLIPEncoder()  # loads model on startup
face = FaceEncoder()
searcher = Searcher(clip)

@app.post("/encode/image")
def encode_image(req: ImageEncodeRequest):
    embeddings = clip.encode_images(req.paths)
    return {"embeddings": [e.tolist() for e in embeddings]}

@app.post("/encode/text")
def encode_text(req: TextEncodeRequest):
    embedding = clip.encode_text(req.text)
    return {"embedding": embedding.tolist()}

@app.post("/detect/faces")
def detect_faces(req: FaceDetectRequest):
    faces = face.detect_and_encode(req.path)
    return {"faces": faces}

@app.post("/search/image")
def search_image(req: ImageSearchRequest):
    results = searcher.search_by_text(req.db_path, req.text,
        camera_ids=req.camera_ids, start_time=req.start_time,
        end_time=req.end_time, limit=req.limit)
    return {"results": results}

@app.post("/search/face")
def search_face(req: FaceSearchRequest):
    results = searcher.search_by_face(req.db_path, req.encoding,
        camera_ids=req.camera_ids, start_time=req.start_time,
        end_time=req.end_time, threshold=req.threshold, limit=req.limit)
    return {"results": results}

@app.post("/cluster/faces")
def cluster_faces(req: ClusterRequest):
    clusters = searcher.cluster_unassigned_faces(req.db_path,
        distance_threshold=req.distance_threshold)
    return {"clusters": clusters}
```

### Searcher

Loads embeddings from SQLite into NumPy arrays and performs brute-force search.
This is viable because the dataset is bounded by the N-day retention window.

```python
# mlservice/searcher.py
import sqlite3
import numpy as np
from sklearn.cluster import AgglomerativeClustering

class Searcher:
    def __init__(self, clip_encoder):
        self.clip = clip_encoder

    def search_by_text(self, db_path, text, camera_ids=None,
                       start_time=None, end_time=None, limit=20):
        query_emb = self.clip.encode_text(text)
        conn = sqlite3.connect(db_path)
        rows = self._load_clip_embeddings(conn, camera_ids, start_time, end_time)
        conn.close()

        if not rows:
            return []

        ids, embeddings, metadata = zip(*rows)
        embeddings = np.array(embeddings)
        # cosine similarity (embeddings are L2-normalized)
        scores = embeddings @ query_emb
        top_k = np.argsort(scores)[::-1][:limit]
        return [{"id": ids[i], "score": float(scores[i]), **metadata[i]}
                for i in top_k]

    def search_by_face(self, db_path, encoding, camera_ids=None,
                       start_time=None, end_time=None, threshold=0.6,
                       limit=20):
        query_emb = np.array(encoding)
        conn = sqlite3.connect(db_path)
        rows = self._load_face_embeddings(conn, camera_ids, start_time, end_time)
        conn.close()

        if not rows:
            return []

        ids, embeddings, metadata = zip(*rows)
        embeddings = np.array(embeddings)
        distances = np.linalg.norm(embeddings - query_emb, axis=1)
        mask = distances < threshold
        filtered = np.where(mask)[0]
        sorted_idx = filtered[np.argsort(distances[filtered])][:limit]
        return [{"id": ids[i], "distance": float(distances[i]),
                 "score": float(1.0 - distances[i]), **metadata[i]}
                for i in sorted_idx]

    def cluster_unassigned_faces(self, db_path, distance_threshold=0.6):
        conn = sqlite3.connect(db_path)
        rows = self._load_unassigned_face_embeddings(conn)
        conn.close()

        if not rows:
            return []

        ids, embeddings, metadata = zip(*rows)
        embeddings = np.array(embeddings)

        clustering = AgglomerativeClustering(
            n_clusters=None,
            distance_threshold=distance_threshold,
            metric="euclidean",
            linkage="average",
        )
        labels = clustering.fit_predict(embeddings)

        clusters = {}
        for i, label in enumerate(labels):
            label = int(label)
            if label not in clusters:
                clusters[label] = {"cluster_id": label, "face_ids": [],
                                   "representative_faces": [], "size": 0}
            clusters[label]["face_ids"].append(ids[i])
            clusters[label]["size"] += 1
            # keep up to 5 representative faces per cluster
            if len(clusters[label]["representative_faces"]) < 5:
                clusters[label]["representative_faces"].append(
                    {"id": ids[i], **metadata[i]})

        return list(clusters.values())
```

### CLIP Encoder

Uses MobileCLIP2 via `open_clip`, matching the pattern from `experiments/demo_search.py`:

```python
# mlservice/clip_encoder.py
import torch
import open_clip
from mobileclip.modules.common.mobileone import reparameterize_model

class CLIPEncoder:
    def __init__(self, model_name="MobileCLIP2-S0", pretrained="dfndr2b",
                 device=None):
        self.device = device or ("cuda" if torch.cuda.is_available() else "cpu")
        model, _, self.preprocess = open_clip.create_model_and_transforms(
            model_name, pretrained=pretrained,
            image_mean=(0, 0, 0), image_std=(1, 1, 1),
        )
        self.model = reparameterize_model(model).to(self.device).eval()
        self.tokenizer = open_clip.get_tokenizer(model_name)

    def encode_images(self, paths: list[str]) -> list[np.ndarray]:
        # batch encode, 32 at a time
        ...

    def encode_text(self, text: str) -> np.ndarray:
        tokens = self.tokenizer([text]).to(self.device)
        with torch.no_grad():
            emb = self.model.encode_text(tokens)
            emb /= emb.norm(dim=-1, keepdim=True)
        return emb.squeeze().cpu().numpy()
```

### Face Encoder

```python
# mlservice/face_encoder.py
import face_recognition as fr

class FaceEncoder:
    def detect_and_encode(self, path: str) -> list[dict]:
        image = fr.load_image_file(path)
        locations = fr.face_locations(image, model="hog")
        encodings = fr.face_encodings(image, locations)
        return [
            {
                "encoding": enc.tolist(),
                "bbox": {"top": t, "right": r, "bottom": b, "left": l},
            }
            for (t, r, b, l), enc in zip(locations, encodings)
        ]
```

## SQLite Storage

Embeddings and metadata are stored in a single SQLite database file
(`data/intelsk.db`). This replaces ChromaDB — the bounded dataset (N-day
rolling window) makes in-memory brute-force search viable, and SQLite
provides persistence without an extra process.

### Schema

```sql
CREATE TABLE IF NOT EXISTS clip_embeddings (
    id           TEXT PRIMARY KEY,
    embedding    BLOB NOT NULL,          -- 512 x float32, stored as raw bytes
    camera_id    TEXT NOT NULL,
    timestamp    TEXT NOT NULL,          -- ISO 8601
    frame_path   TEXT NOT NULL,
    source_video TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_clip_camera_ts ON clip_embeddings(camera_id, timestamp);
CREATE INDEX idx_clip_created ON clip_embeddings(created_at);

CREATE TABLE IF NOT EXISTS face_embeddings (
    id           TEXT PRIMARY KEY,
    embedding    BLOB NOT NULL,          -- 128 x float32, stored as raw bytes
    camera_id    TEXT NOT NULL,
    timestamp    TEXT NOT NULL,          -- ISO 8601
    frame_path   TEXT NOT NULL,
    bbox_top     INTEGER NOT NULL,
    bbox_right   INTEGER NOT NULL,
    bbox_bottom  INTEGER NOT NULL,
    bbox_left    INTEGER NOT NULL,
    person_name  TEXT,                   -- NULL = unassigned, set after discovery
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_face_camera_ts ON face_embeddings(camera_id, timestamp);
CREATE INDEX idx_face_person ON face_embeddings(person_name);
CREATE INDEX idx_face_created ON face_embeddings(created_at);
```

Embeddings are stored as raw byte blobs (`numpy.ndarray.tobytes()`) for
compact storage and fast loading. The ML sidecar reads them back with
`np.frombuffer(blob, dtype=np.float32)`.

### Go SQLite Client

```go
// backend/services/storage.go
type Storage struct {
    db *sql.DB
}

func NewStorage(dbPath string) (*Storage, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, err
    }
    // run schema migrations
    if err := runMigrations(db); err != nil {
        return nil, err
    }
    return &Storage{db: db}, nil
}

func (s *Storage) AddClipEmbedding(id string, embedding []byte,
    cameraID, timestamp, framePath, sourceVideo string) error {
    _, err := s.db.Exec(`INSERT OR REPLACE INTO clip_embeddings
        (id, embedding, camera_id, timestamp, frame_path, source_video)
        VALUES (?, ?, ?, ?, ?, ?)`,
        id, embedding, cameraID, timestamp, framePath, sourceVideo)
    return err
}

func (s *Storage) AddFaceEmbedding(id string, embedding []byte,
    cameraID, timestamp, framePath string,
    bboxTop, bboxRight, bboxBottom, bboxLeft int) error {
    _, err := s.db.Exec(`INSERT OR REPLACE INTO face_embeddings
        (id, embedding, camera_id, timestamp, frame_path,
         bbox_top, bbox_right, bbox_bottom, bbox_left)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        id, embedding, cameraID, timestamp, framePath,
        bboxTop, bboxRight, bboxBottom, bboxLeft)
    return err
}

func (s *Storage) Cleanup(olderThan time.Time) (int64, error) {
    ts := olderThan.Format(time.RFC3339)
    var total int64

    res, err := s.db.Exec("DELETE FROM clip_embeddings WHERE created_at < ?", ts)
    if err != nil {
        return 0, err
    }
    n, _ := res.RowsAffected()
    total += n

    res, err = s.db.Exec(
        "DELETE FROM face_embeddings WHERE created_at < ? AND person_name IS NULL",
        ts)
    if err != nil {
        return total, err
    }
    n, _ = res.RowsAffected()
    total += n

    return total, nil
}
```

## Go ML Client

The Go backend calls the ML sidecar via HTTP:

```go
// backend/services/mlclient.go
type MLClient struct {
    baseURL    string
    httpClient *http.Client
}

func (c *MLClient) EncodeImages(paths []string) ([][]float64, error) {
    body, _ := json.Marshal(map[string]any{"paths": paths})
    resp, err := c.httpClient.Post(c.baseURL+"/encode/image",
        "application/json", bytes.NewReader(body))
    // parse response...
}

func (c *MLClient) EncodeText(text string) ([]float64, error) {
    body, _ := json.Marshal(map[string]any{"text": text})
    resp, err := c.httpClient.Post(c.baseURL+"/encode/text",
        "application/json", bytes.NewReader(body))
    // parse response...
}

func (c *MLClient) DetectFaces(path string) ([]Face, error) {
    body, _ := json.Marshal(map[string]any{"path": path})
    resp, err := c.httpClient.Post(c.baseURL+"/detect/faces",
        "application/json", bytes.NewReader(body))
    // parse response...
}

func (c *MLClient) SearchByText(dbPath, text string, cameraIDs []string,
    startTime, endTime string, limit int) ([]SearchResult, error) {
    body, _ := json.Marshal(map[string]any{
        "db_path": dbPath, "text": text, "camera_ids": cameraIDs,
        "start_time": startTime, "end_time": endTime, "limit": limit,
    })
    resp, err := c.httpClient.Post(c.baseURL+"/search/image",
        "application/json", bytes.NewReader(body))
    // parse response...
}

func (c *MLClient) SearchByFace(dbPath string, encoding []float64,
    cameraIDs []string, startTime, endTime string,
    threshold float64, limit int) ([]SearchResult, error) {
    body, _ := json.Marshal(map[string]any{
        "db_path": dbPath, "encoding": encoding, "camera_ids": cameraIDs,
        "start_time": startTime, "end_time": endTime,
        "threshold": threshold, "limit": limit,
    })
    resp, err := c.httpClient.Post(c.baseURL+"/search/face",
        "application/json", bytes.NewReader(body))
    // parse response...
}

func (c *MLClient) ClusterFaces(dbPath string,
    distanceThreshold float64) ([]FaceCluster, error) {
    body, _ := json.Marshal(map[string]any{
        "db_path": dbPath, "distance_threshold": distanceThreshold,
    })
    resp, err := c.httpClient.Post(c.baseURL+"/cluster/faces",
        "application/json", bytes.NewReader(body))
    // parse response...
}
```

## Pipeline Orchestration (Go)

The Go backend orchestrates the full indexing pipeline:

```go
// backend/services/pipeline.go
func (p *Pipeline) Process(cameras []string, startDate, endDate string,
    progress chan<- ProgressEvent) error {
    for _, camID := range cameras {
        // 1. Download video clips
        clips, err := p.downloader.Download(camID, startDate, endDate)
        progress <- ProgressEvent{Stage: "downloading", CameraID: camID, ...}

        // 2. Extract frames
        frames, err := p.extractor.Extract(clips)
        progress <- ProgressEvent{Stage: "extracting", CameraID: camID, ...}

        // 3. Encode frames via ML sidecar (batched)
        for batch := range batches(frames, 32) {
            clipEmbeddings, _ := p.mlClient.EncodeImages(batch)
            // store CLIP embeddings in SQLite
            for i, framePath := range batch {
                embBytes := float64sToBytes(clipEmbeddings[i])
                p.storage.AddClipEmbedding(frameID, embBytes,
                    camID, timestamp, framePath, sourceVideo)
            }

            // detect and store face embeddings
            for _, framePath := range batch {
                faces, _ := p.mlClient.DetectFaces(framePath)
                for j, face := range faces {
                    embBytes := float64sToBytes(face.Encoding)
                    p.storage.AddFaceEmbedding(faceID, embBytes,
                        camID, timestamp, framePath,
                        face.BBox.Top, face.BBox.Right,
                        face.BBox.Bottom, face.BBox.Left)
                }
            }
            progress <- ProgressEvent{Stage: "indexing", FramesDone: n, ...}
        }
    }
    progress <- ProgressEvent{Stage: "complete"}
    return nil
}
```

## Search Flows

**Text search flow (Go → ML sidecar → SQLite):**
1. Go receives `POST /api/search/text` with query `"delivery person"`
2. Go calls ML sidecar `POST /search/image` with query text + DB path + filters
3. ML sidecar encodes text via CLIP, loads CLIP embeddings from SQLite,
   computes cosine similarity (brute-force NumPy), returns ranked results
4. Go returns results as JSON

**Person search flow (Go → ML sidecar → SQLite):**
1. Go receives `POST /api/search/person/lukasz`
2. Go reads lukasz's encodings from `data/face_registry.json`
3. Go averages the 128-dim encodings
4. Go calls ML sidecar `POST /search/face` with the averaged encoding + DB path
5. ML sidecar loads face embeddings from SQLite, computes L2 distance,
   filters by threshold, returns ranked results
6. Go returns results as JSON

## Data Cleanup

The Go backend runs cleanup to enforce the N-day retention window:

1. Delete SQLite rows from `clip_embeddings` where `created_at` is older than N days
2. Delete SQLite rows from `face_embeddings` where `created_at` is older than N days
   **and** `person_name IS NULL` (assigned faces are kept for the registry)
3. Delete frame JPEG files from `data/frames/` for purged dates
4. Delete video MP4 files from `data/videos/` for purged dates
5. Update `data/process_history.json` to remove purged entries

Cleanup runs:
- Automatically on backend startup
- Periodically (configurable, e.g., daily)
- On demand via `POST /api/cleanup`
