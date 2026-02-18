# MobileCLIP2 Image Search Demo

A CLI tool that ranks images in a directory by similarity to a text query using the MobileCLIP2 model.

## Setup

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -e .
```

## Usage

```bash
python3 demo_search.py --image-dir <path-to-images> --query "<text query>"
```

### Arguments

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--image-dir` | yes | — | Path to directory containing images |
| `--query` | yes | — | Text search query |
| `--model` | no | `MobileCLIP2-S0` | Model variant name |
| `--checkpoint` | no | downloads from HuggingFace | Path to a local `.pt` checkpoint |
| `--batch-size` | no | `32` | Batch size for image encoding |

Supported image formats: `.jpg`, `.jpeg`, `.png`, `.bmp`, `.webp`, `.tiff`.

### Examples

```bash
# Basic search
python3 demo_search.py --image-dir ~/Photos --query "a dog playing in snow"

# Use a local checkpoint instead of downloading
python3 demo_search.py --image-dir ~/Photos --query "sunset at the beach" \
    --checkpoint checkpoints/mobileclip2_s0.pt

# Use a different model variant
python3 demo_search.py --image-dir ~/Photos --query "a cat" --model MobileCLIP2-S2
```

### Output

```
Found 8 images

Results for query: "a cute animal"
Score    File
0.1897   test_images/cat_sleeping.jpg
0.1798   test_images/dog_park.jpg
0.1446   test_images/bird.jpg
0.0830   test_images/car_red.jpg
...
```

Images are ranked by cosine similarity (higher = more relevant).
