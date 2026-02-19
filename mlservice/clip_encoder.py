"""CLIP encoder using MobileCLIP2 for image and text embedding."""

import torch
import numpy as np
import open_clip
from PIL import Image
from mobileclip.modules.common.mobileone import reparameterize_model


class CLIPEncoder:
    """Encodes images and text into 512-dim L2-normalized CLIP embeddings."""

    def __init__(self, batch_size: int = 32):
        self.batch_size = batch_size
        self.device = self._detect_device()

        model, _, self.preprocess = open_clip.create_model_and_transforms(
            "MobileCLIP2-S0",
            pretrained="dfndr2b",
            image_mean=(0, 0, 0),
            image_std=(1, 1, 1),
        )
        self.tokenizer = open_clip.get_tokenizer("MobileCLIP2-S0")

        model.eval()
        model = reparameterize_model(model)
        self.model = model.to(self.device)

    @staticmethod
    def _detect_device() -> str:
        if torch.cuda.is_available():
            return "cuda"
        if torch.backends.mps.is_available():
            return "mps"
        return "cpu"

    def encode_images(self, paths: list[str]) -> list[np.ndarray]:
        """Batch encode images, returning list of L2-normalized 512-dim numpy arrays."""
        all_features = []
        for i in range(0, len(paths), self.batch_size):
            batch_paths = paths[i : i + self.batch_size]
            batch_tensors = torch.stack(
                [self.preprocess(Image.open(p).convert("RGB")) for p in batch_paths]
            ).to(self.device)

            with torch.no_grad(), torch.amp.autocast(
                self.device, enabled=(self.device == "cuda")
            ):
                features = self.model.encode_image(batch_tensors)
                features /= features.norm(dim=-1, keepdim=True)

            all_features.append(features.cpu().numpy())

        combined = np.concatenate(all_features, axis=0)
        return [combined[i].astype(np.float32) for i in range(combined.shape[0])]

    def encode_text(self, text: str) -> np.ndarray:
        """Encode a single text query, returning L2-normalized 512-dim numpy array."""
        tokens = self.tokenizer([text]).to(self.device)
        with torch.no_grad(), torch.amp.autocast(
            self.device, enabled=(self.device == "cuda")
        ):
            features = self.model.encode_text(tokens)
            features /= features.norm(dim=-1, keepdim=True)
        return features.cpu().numpy()[0].astype(np.float32)
