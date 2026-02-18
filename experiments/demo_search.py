#!/usr/bin/env python3
"""Demo CLI app: rank images by similarity to a text query using MobileCLIP2."""

import argparse
import sys
from pathlib import Path

import torch
import open_clip
from PIL import Image
from mobileclip.modules.common.mobileone import reparameterize_model

IMAGE_EXTENSIONS = {".jpg", ".jpeg", ".png", ".bmp", ".webp", ".tiff"}

# Models that do NOT need custom image_mean/image_std
STANDARD_NORM_SUFFIXES = ("S3", "S4", "L-14")


def parse_args():
    parser = argparse.ArgumentParser(
        description="Rank images by similarity to a text query using MobileCLIP2."
    )
    parser.add_argument(
        "--image-dir", required=True, type=Path, help="Directory containing images"
    )
    parser.add_argument("--query", required=True, help="Text query string")
    parser.add_argument(
        "--model", default="MobileCLIP2-S0", help="Model variant name (default: MobileCLIP2-S0)"
    )
    parser.add_argument(
        "--checkpoint",
        default=None,
        help="Path to local .pt checkpoint. If omitted, downloads from HuggingFace.",
    )
    parser.add_argument(
        "--batch-size", type=int, default=32, help="Batch size for image encoding (default: 32)"
    )
    return parser.parse_args()


def collect_images(image_dir: Path) -> list[Path]:
    images = sorted(
        p for p in image_dir.iterdir()
        if p.is_file() and p.suffix.lower() in IMAGE_EXTENSIONS
    )
    return images


def main():
    args = parse_args()

    if not args.image_dir.is_dir():
        print(f"Error: {args.image_dir} is not a directory", file=sys.stderr)
        sys.exit(1)

    # -- Collect images --
    image_paths = collect_images(args.image_dir)
    if not image_paths:
        print(f"No images found in {args.image_dir}", file=sys.stderr)
        sys.exit(1)
    print(f"Found {len(image_paths)} images")

    # -- Load model --
    pretrained = args.checkpoint if args.checkpoint else "dfndr2b"
    model_kwargs = {}
    if not any(args.model.endswith(s) for s in STANDARD_NORM_SUFFIXES):
        model_kwargs = {"image_mean": (0, 0, 0), "image_std": (1, 1, 1)}

    model, _, preprocess = open_clip.create_model_and_transforms(
        args.model, pretrained=pretrained, **model_kwargs
    )
    tokenizer = open_clip.get_tokenizer(args.model)

    model.eval()
    model = reparameterize_model(model)

    device = "cuda" if torch.cuda.is_available() else "cpu"
    model = model.to(device)

    # -- Encode images --
    all_image_features = []
    for i in range(0, len(image_paths), args.batch_size):
        batch_paths = image_paths[i : i + args.batch_size]
        batch_tensors = torch.stack(
            [preprocess(Image.open(p).convert("RGB")) for p in batch_paths]
        ).to(device)

        with torch.no_grad(), torch.amp.autocast(device, enabled=(device == "cuda")):
            features = model.encode_image(batch_tensors)
            features /= features.norm(dim=-1, keepdim=True)

        all_image_features.append(features.cpu())

    image_features = torch.cat(all_image_features, dim=0)

    # -- Encode text --
    tokens = tokenizer([args.query]).to(device)
    with torch.no_grad(), torch.amp.autocast(device, enabled=(device == "cuda")):
        text_features = model.encode_text(tokens)
        text_features /= text_features.norm(dim=-1, keepdim=True)
    text_features = text_features.cpu()

    # -- Rank by cosine similarity --
    similarities = (image_features @ text_features.T).squeeze(1)
    ranked_indices = similarities.argsort(descending=True)

    print(f'\nResults for query: "{args.query}"')
    print(f"{'Score':<9}File")
    for idx in ranked_indices:
        score = similarities[idx].item()
        print(f"{score:<9.4f}{image_paths[idx]}")


if __name__ == "__main__":
    main()
