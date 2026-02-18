# Frame Extraction

[Back to main design](DESIGN.md)

## Time-Based Extraction

Simple, predictable. Extract 1 frame every N seconds using ffmpeg:

```bash
ffmpeg -i input.mp4 -vf "fps=1/5" -q:v 2 frames/frame_%06d.jpg
```

Python wrapper:
```python
def extract_frames_time(video_path: Path, output_dir: Path, interval_sec: int = 5):
    cmd = [
        "ffmpeg", "-i", str(video_path),
        "-vf", f"fps=1/{interval_sec}",
        "-q:v", "2",
        str(output_dir / "frame_%06d.jpg")
    ]
    subprocess.run(cmd, check=True)
```

## Motion-Based Extraction

Reduces frame count significantly for static scenes. Uses OpenCV MOG2 background
subtractor:

```python
def extract_frames_motion(
    video_path: Path,
    output_dir: Path,
    motion_threshold: float = 0.01,    # fraction of pixels changed
    min_gap_sec: float = 2.0,          # minimum time between saved frames
):
    cap = cv2.VideoCapture(str(video_path))
    bg_sub = cv2.createBackgroundSubtractorMOG2(
        history=500, varThreshold=50, detectShadows=False
    )
    fps = cap.get(cv2.CAP_PROP_FPS)
    last_saved_frame = -999

    while True:
        ret, frame = cap.read()
        if not ret:
            break
        frame_num = int(cap.get(cv2.CAP_PROP_POS_FRAMES))
        mask = bg_sub.apply(frame)
        motion_ratio = np.count_nonzero(mask) / mask.size

        elapsed = (frame_num - last_saved_frame) / fps
        if motion_ratio > motion_threshold and elapsed >= min_gap_sec:
            cv2.imwrite(str(output_dir / f"frame_{frame_num:06d}.jpg"), frame)
            last_saved_frame = frame_num

    cap.release()
```

## De-Duplication

Perceptual hashing (pHash) to skip near-identical frames across extraction runs:

```python
from imagehash import phash
from PIL import Image

def is_duplicate(frame_path: Path, seen_hashes: set, threshold: int = 8) -> bool:
    h = phash(Image.open(frame_path))
    for seen in seen_hashes:
        if h - seen < threshold:
            return True
    seen_hashes.add(h)
    return False
```

## Frame Metadata

Each extracted frame is stored with metadata:

```python
@dataclass
class FrameMetadata:
    frame_path: str          # relative path to JPEG
    camera_id: str           # from config
    timestamp: datetime      # calculated from video start + frame offset
    source_video: str        # path to source MP4
    frame_number: int
    extraction_method: str   # "time" | "motion"
    motion_score: float | None
```

## Configuration

```yaml
# config/extraction.yaml
extraction:
  method: motion           # time | motion | both
  time_interval_sec: 5     # for time-based
  motion_threshold: 0.01   # for motion-based
  min_gap_sec: 2.0         # for motion-based
  output_format: jpg
  output_quality: 85
  dedup_enabled: true
  dedup_phash_threshold: 8
  storage_path: data/frames
  retention_days: 90
```
