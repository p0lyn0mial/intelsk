"""CLIP encoder with switchable model presets."""

import torch
import numpy as np
import open_clip
from PIL import Image

MODEL_PRESETS = {
    "mobileclip-s0": {
        "name": "MobileCLIP-S0",
        "model": "MobileCLIP2-S0",
        "pretrained": "dfndr2b",
        "image_mean": (0, 0, 0),
        "image_std": (1, 1, 1),
        "reparameterize": True,
    },
    "vit-b-32": {
        "name": "ViT-B-32 (OpenAI)",
        "model": "ViT-B-32",
        "pretrained": "openai",
    },
    "vit-l-14": {
        "name": "ViT-L-14 (OpenAI)",
        "model": "ViT-L-14",
        "pretrained": "openai",
    },
}


class CLIPEncoder:
    """Encodes images and text into L2-normalized CLIP embeddings."""

    def __init__(self, preset: str = "mobileclip-s0", batch_size: int = 32):
        self.batch_size = batch_size
        self.device = self._detect_device()
        self.preset_key = preset

        preset_cfg = MODEL_PRESETS[preset]

        create_kwargs = {
            "model_name": preset_cfg["model"],
            "pretrained": preset_cfg["pretrained"],
        }
        if "image_mean" in preset_cfg:
            create_kwargs["image_mean"] = preset_cfg["image_mean"]
        if "image_std" in preset_cfg:
            create_kwargs["image_std"] = preset_cfg["image_std"]

        model, _, self.preprocess = open_clip.create_model_and_transforms(**create_kwargs)
        self.tokenizer = open_clip.get_tokenizer(preset_cfg["model"])

        model.eval()

        if preset_cfg.get("reparameterize"):
            from mobileclip.modules.common.mobileone import reparameterize_model
            model = reparameterize_model(model)

        self.model = model.to(self.device)

        # Determine embedding dimension from a dummy forward pass
        with torch.no_grad():
            dummy = torch.zeros(1, 3, 224, 224, device=self.device)
            out = self.model.encode_image(dummy)
            self.embedding_dim = out.shape[-1]

    @staticmethod
    def _detect_device() -> str:
        if torch.cuda.is_available():
            return "cuda"
        if torch.backends.mps.is_available():
            return "mps"
        return "cpu"

    def encode_images(self, paths: list[str]) -> list[np.ndarray]:
        """Batch encode images, returning list of L2-normalized numpy arrays."""
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
        """Encode a single text query, returning L2-normalized numpy array."""
        tokens = self.tokenizer([text]).to(self.device)
        with torch.no_grad(), torch.amp.autocast(
            self.device, enabled=(self.device == "cuda")
        ):
            features = self.model.encode_text(tokens)
            features /= features.norm(dim=-1, keepdim=True)
        return features.cpu().numpy()[0].astype(np.float32)
