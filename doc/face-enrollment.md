# Face Enrollment

[Back to main design](DESIGN.md)

## Overview

Face enrollment is **discovery-based only** — there is no manual photo upload.
The system finds faces in processed frames, clusters all unassigned faces across
the entire database (all cameras, all dates), and presents groups in the UI for
the user to assign names.

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
          "source": "frames/front_door/2026-02-18/frame_000042.jpg",
          "encoding": [-0.097, 0.167, 0.157, ...]
        },
        {
          "source": "frames/driveway/2026-02-19/frame_000108.jpg",
          "encoding": [-0.095, 0.170, 0.155, ...]
        }
      ]
    }
  }
}
```

Each person has a list of embeddings, each with a `source` frame path and a
128-dimensional `encoding` array from dlib. Source paths point to the frames
where the face was discovered (not uploaded photos).

## Discovery-Based Enrollment Flow

1. **During processing**: face embeddings are detected by the ML sidecar and stored
   in the SQLite `face_embeddings` table with `person_name = NULL` (unassigned).
   Each embedding is linked to its source frame and bounding box.

2. **User triggers "Discover Faces"** in the web UI (Faces page).

3. **Go backend calls ML sidecar** `POST /cluster/faces` with the path to the
   SQLite database. The sidecar loads **all unassigned face embeddings** across
   the entire database (all cameras, all dates) and clusters them.

4. **ML sidecar clusters faces** using agglomerative clustering (scikit-learn):
   - Distance metric: Euclidean (L2), matching dlib's face_distance behavior
   - Linkage: average
   - Distance threshold: 0.6 (same as match threshold, configurable)
   - Each cluster represents a distinct person

5. **Go returns clusters** to the UI as groups of face thumbnails. Each cluster
   includes up to 5 representative face crops and total count.

6. **User assigns names** to clusters in the UI — types a name and clicks "Assign".

7. **Named clusters become face registry entries**:
   - The average encoding of the cluster is computed
   - Added to `data/face_registry.json` with source frame references
   - SQLite `face_embeddings` rows are updated: `person_name` is set to the
     assigned name (so they are excluded from future clustering)

## API Endpoints

```
POST /api/faces/discover              Trigger face clustering on all unassigned faces
GET  /api/faces/clusters              Get discovered face clusters (cached from last discovery)
POST /api/faces/clusters/{id}/assign  Assign a name to a cluster
GET  /api/faces/registry              List enrolled persons
DELETE /api/faces/registry/{name}     Remove enrolled person
```

See [backend-api.md](backend-api.md) for full request/response details.

## Matching

- Multiple encodings per person improve accuracy
- Euclidean distance (L2), matching `face_recognition.face_distance()` behavior
- Accept threshold: distance < 0.6 (default, configurable)
- Score displayed as `1.0 - distance` for UI consistency (higher = better)
- Results sorted by distance ascending (best match first)

## Data Lifecycle

- **Unassigned face embeddings**: purged after N days along with their source
  frames (controlled by `storage.retention_days`)
- **Assigned face embeddings**: kept as long as the person exists in the registry
  (not subject to retention cleanup)
- **Face registry** (`face_registry.json`): kept indefinitely
