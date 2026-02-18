# Face Enrollment

[Back to main design](DESIGN.md)

## Registry Storage

Follows the format established in the existing desktop app
(`experiments/desktop/.face_registry.json`). The Go backend reads and writes
this file directly.

```json
{
  "people": {
    "lukasz": {
      "embeddings": [
        {
          "source": "data/faces/lukasz_01.jpg",
          "encoding": [-0.097, 0.167, 0.157, ...]
        },
        {
          "source": "data/faces/lukasz_02.jpg",
          "encoding": [-0.095, 0.170, 0.155, ...]
        }
      ]
    }
  }
}
```

Each person has a list of embeddings, each with a `source` photo path and a
128-dimensional `encoding` array from dlib.

## Enrollment Flow

1. User uploads 1+ photos via web UI
2. Go backend saves photo to `data/faces/`, calls ML sidecar `POST /detect/faces`
3. ML sidecar detects face, returns 128-dim encoding
4. Go backend validates exactly one face was found (rejects ambiguous photos)
5. Go backend adds encoding to `data/face_registry.json`
6. For search, Go backend averages all encodings for that person and queries ChromaDB

## Matching

- Multiple encodings per person improve accuracy
- Euclidean distance (L2), matching `face_recognition.face_distance()` behavior
- Accept threshold: distance < 0.6 (default, configurable)
- Score displayed as `1.0 - distance` for UI consistency (higher = better)
- Results sorted by distance ascending (best match first)
