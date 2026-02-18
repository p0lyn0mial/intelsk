# Indexing Pipeline & Vector Search

[Back to main design](DESIGN.md)

## Python ML Sidecar API

The ML sidecar is a small FastAPI service that handles all ML inference. The Go
backend calls it over HTTP. It is stateless — it receives file paths or text and
returns embeddings.

### Endpoints

```
POST /encode/image     Batch CLIP image encoding
POST /encode/text      Single CLIP text encoding
POST /detect/faces     Face detection + encoding for a single image
GET  /health           Health check
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

### Implementation

```python
# mlservice/main.py
from fastapi import FastAPI
from clip_encoder import CLIPEncoder
from face_encoder import FaceEncoder

app = FastAPI()
clip = CLIPEncoder()  # loads model on startup
face = FaceEncoder()

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
            // store in ChromaDB...

            for _, framePath := range batch {
                faces, _ := p.mlClient.DetectFaces(framePath)
                // store face embeddings in ChromaDB...
            }
            progress <- ProgressEvent{Stage: "indexing", FramesDone: n, ...}
        }
    }
    progress <- ProgressEvent{Stage: "complete"}
    return nil
}
```

## Vector Database (ChromaDB)

ChromaDB runs as a separate process and is accessed from Go via its HTTP API.

### Collections

Two collections with different embedding dimensions and distance metrics:

- `clip_embeddings` — cosine distance, 512-dim
- `face_embeddings` — L2 (Euclidean) distance, 128-dim

### Schema

**clip_embeddings:**
| Field       | Type      | Description                    |
|-------------|-----------|--------------------------------|
| id          | string    | `{camera_id}_{timestamp}_{frame_num}` |
| embedding   | float[512]| CLIP image embedding           |
| camera_id   | string    | Camera identifier              |
| timestamp   | string    | ISO 8601 datetime              |
| frame_path  | string    | Relative path to JPEG          |
| source_video| string    | Relative path to source MP4    |

**face_embeddings:**
| Field       | Type      | Description                    |
|-------------|-----------|--------------------------------|
| id          | string    | `{camera_id}_{timestamp}_{frame_num}_{face_idx}` |
| embedding   | float[128]| Face encoding (dlib)           |
| camera_id   | string    | Camera identifier              |
| timestamp   | string    | ISO 8601 datetime              |
| frame_path  | string    | Relative path to JPEG          |
| bbox_top    | int       | Bounding box top               |
| bbox_right  | int       | Bounding box right             |
| bbox_bottom | int       | Bounding box bottom            |
| bbox_left   | int       | Bounding box left              |

### Go ChromaDB Client

```go
// backend/services/vectordb.go
type VectorDB struct {
    baseURL        string
    clipCollection string
    faceCollection string
    httpClient     *http.Client
}

func (db *VectorDB) AddClipEmbedding(id string, embedding []float64,
    metadata map[string]any) error {
    // POST to ChromaDB HTTP API
}

func (db *VectorDB) QueryClip(embedding []float64, nResults int,
    where map[string]any) ([]QueryResult, error) {
    // POST query to ChromaDB HTTP API
}
```

### Query Examples

**Text search flow (Go → ML sidecar → ChromaDB):**
1. Go receives `POST /api/search/text` with query `"delivery person"`
2. Go calls ML sidecar `POST /encode/text` → gets 512-dim embedding
3. Go calls ChromaDB query on `clip_embeddings` with the embedding + date filters
4. Go returns ranked results as JSON

**Person search flow (Go → ChromaDB):**
1. Go receives `POST /api/search/person/lukasz`
2. Go reads lukasz's encodings from `data/face_registry.json`
3. Go averages the 128-dim encodings
4. Go calls ChromaDB query on `face_embeddings` with the averaged embedding
5. Go returns ranked results as JSON
