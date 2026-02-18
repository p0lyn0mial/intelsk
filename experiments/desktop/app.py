#!/usr/bin/env python3
"""Desktop GUI app: rank images by similarity to a text query using MobileCLIP2,
plus face-based search using the face_recognition library."""

import json
import os
import threading
import tkinter as tk
from tkinter import filedialog, messagebox, simpledialog, ttk
from pathlib import Path

import numpy as np
import torch
import open_clip
from PIL import Image, ImageTk
from mobileclip.modules.common.mobileone import reparameterize_model

IMAGE_EXTENSIONS = {".jpg", ".jpeg", ".png", ".bmp", ".webp", ".tiff"}
STANDARD_NORM_SUFFIXES = ("S3", "S4", "L-14")
THUMBNAIL_SIZE = (150, 150)
BATCH_SIZE = 32
MODEL_NAME = "MobileCLIP2-S0"
DEFAULT_THRESHOLD_PCT = 50
DEFAULT_FACE_DISTANCE = 0.6
CONFIG_PATH = Path(__file__).resolve().parent / ".app_config.json"
FACE_REGISTRY_PATH = Path(__file__).resolve().parent / ".face_registry.json"
FACE_CACHE_PATH = Path(__file__).resolve().parent / ".face_cache.json"


class App:
    def __init__(self, root: tk.Tk):
        self.root = root
        self.root.title("MobileCLIP2 Image Search")
        self.root.geometry("900x700")

        self.model = None
        self.preprocess = None
        self.tokenizer = None
        self.device = "cuda" if torch.cuda.is_available() else "cpu"
        self._searching = False

        self._face_recognition = None  # lazy import

        self._build_ui()
        self._load_config()
        self._refresh_enrolled_list()

    # ------------------------------------------------------------------ #
    #  UI
    # ------------------------------------------------------------------ #

    def _build_ui(self):
        # -- Directory picker (shared) --
        dir_frame = tk.Frame(self.root)
        dir_frame.pack(fill=tk.X, padx=10, pady=(10, 5))

        tk.Label(dir_frame, text="Image dir:").pack(side=tk.LEFT)
        self.dir_var = tk.StringVar()
        self.dir_label = tk.Label(
            dir_frame, textvariable=self.dir_var, anchor=tk.W, relief=tk.SUNKEN
        )
        self.dir_label.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(5, 5))
        tk.Button(dir_frame, text="Browse", command=self._browse_dir).pack(side=tk.LEFT)

        # -- Notebook with tabs --
        self.notebook = ttk.Notebook(self.root)
        self.notebook.pack(fill=tk.X, padx=10, pady=(0, 5))

        # --- Tab 1: Text Search ---
        text_tab = tk.Frame(self.notebook)
        self.notebook.add(text_tab, text="Text Search")

        query_frame = tk.Frame(text_tab)
        query_frame.pack(fill=tk.X, padx=5, pady=5)
        tk.Label(query_frame, text="Query:").pack(side=tk.LEFT)
        self.query_entry = tk.Entry(query_frame)
        self.query_entry.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(5, 10))
        self.query_entry.bind("<Return>", lambda _: self._on_search())

        thresh_frame = tk.Frame(text_tab)
        thresh_frame.pack(fill=tk.X, padx=5, pady=(0, 5))
        tk.Label(thresh_frame, text="Min similarity %:").pack(side=tk.LEFT)
        self.threshold_var = tk.StringVar(value=str(DEFAULT_THRESHOLD_PCT))
        self.threshold_entry = tk.Entry(thresh_frame, textvariable=self.threshold_var, width=5)
        self.threshold_entry.pack(side=tk.LEFT, padx=(5, 0))

        text_btn_frame = tk.Frame(text_tab)
        text_btn_frame.pack(fill=tk.X, padx=5, pady=(0, 5))
        self.search_btn = tk.Button(
            text_btn_frame, text="Search", command=self._on_search
        )
        self.search_btn.pack(side=tk.LEFT)

        # --- Tab 2: Face Search ---
        face_tab = tk.Frame(self.notebook)
        self.notebook.add(face_tab, text="Face Search")

        # Enrollment section
        enroll_frame = tk.LabelFrame(face_tab, text="Enrolled People")
        enroll_frame.pack(fill=tk.X, padx=5, pady=5)

        enroll_btn_frame = tk.Frame(enroll_frame)
        enroll_btn_frame.pack(fill=tk.X, padx=5, pady=(5, 0))
        tk.Button(enroll_btn_frame, text="Enroll Face...", command=self._on_enroll_face).pack(
            side=tk.LEFT
        )
        tk.Button(enroll_btn_frame, text="Remove Selected", command=self._on_remove_person).pack(
            side=tk.LEFT, padx=(5, 0)
        )

        self.enrolled_listbox = tk.Listbox(enroll_frame, height=4)
        self.enrolled_listbox.pack(fill=tk.X, padx=5, pady=5)

        # Face search controls
        face_search_frame = tk.Frame(face_tab)
        face_search_frame.pack(fill=tk.X, padx=5, pady=(0, 5))

        tk.Label(face_search_frame, text="Person:").pack(side=tk.LEFT)
        self.person_var = tk.StringVar()
        self.person_combo = ttk.Combobox(
            face_search_frame, textvariable=self.person_var, state="readonly", width=20
        )
        self.person_combo.pack(side=tk.LEFT, padx=(5, 10))

        tk.Label(face_search_frame, text="Max distance:").pack(side=tk.LEFT)
        self.face_distance_var = tk.StringVar(value=str(DEFAULT_FACE_DISTANCE))
        tk.Entry(face_search_frame, textvariable=self.face_distance_var, width=5).pack(
            side=tk.LEFT, padx=(5, 0)
        )

        face_btn_frame = tk.Frame(face_tab)
        face_btn_frame.pack(fill=tk.X, padx=5, pady=(0, 5))
        self.face_search_btn = tk.Button(
            face_btn_frame, text="Search Faces", command=self._on_face_search
        )
        self.face_search_btn.pack(side=tk.LEFT)

        # -- Status label (shared) --
        status_frame = tk.Frame(self.root)
        status_frame.pack(fill=tk.X, padx=10, pady=(0, 5))
        self.status_var = tk.StringVar()
        tk.Label(status_frame, textvariable=self.status_var, anchor=tk.W).pack(
            fill=tk.X, expand=True
        )

        # -- Scrollable results area (shared) --
        container = tk.Frame(self.root)
        container.pack(fill=tk.BOTH, expand=True, padx=10, pady=(0, 10))

        self.canvas = tk.Canvas(container)
        scrollbar = tk.Scrollbar(container, orient=tk.VERTICAL, command=self.canvas.yview)
        self.results_frame = tk.Frame(self.canvas)

        self.results_frame.bind(
            "<Configure>",
            lambda _: self.canvas.configure(scrollregion=self.canvas.bbox("all")),
        )
        self.canvas.create_window((0, 0), window=self.results_frame, anchor=tk.NW)
        self.canvas.configure(yscrollcommand=scrollbar.set)

        self.canvas.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

        # Keep references to PhotoImage objects so they aren't garbage-collected
        self._photo_refs: list[ImageTk.PhotoImage] = []

    # ------------------------------------------------------------------ #
    #  Config persistence
    # ------------------------------------------------------------------ #

    def _load_config(self):
        try:
            cfg = json.loads(CONFIG_PATH.read_text())
            saved_dir = cfg.get("image_dir", "")
            if saved_dir and Path(saved_dir).is_dir():
                self.dir_var.set(saved_dir)
            saved_thresh = cfg.get("threshold_pct")
            if saved_thresh is not None:
                self.threshold_var.set(str(saved_thresh))
        except (FileNotFoundError, json.JSONDecodeError):
            pass

    def _save_config(self):
        cfg = {
            "image_dir": self.dir_var.get().strip(),
            "threshold_pct": self.threshold_var.get().strip(),
        }
        CONFIG_PATH.write_text(json.dumps(cfg))

    def _browse_dir(self):
        initial = self.dir_var.get().strip()
        path = filedialog.askdirectory(initialdir=initial or None)
        if path:
            self.dir_var.set(path)
            self._save_config()

    # ------------------------------------------------------------------ #
    #  Face registry persistence
    # ------------------------------------------------------------------ #

    def _load_face_registry(self) -> dict:
        try:
            return json.loads(FACE_REGISTRY_PATH.read_text())
        except (FileNotFoundError, json.JSONDecodeError):
            return {"people": {}}

    def _save_face_registry(self, registry: dict):
        FACE_REGISTRY_PATH.write_text(json.dumps(registry))

    # ------------------------------------------------------------------ #
    #  Face cache persistence
    # ------------------------------------------------------------------ #

    def _load_face_cache(self) -> dict:
        try:
            return json.loads(FACE_CACHE_PATH.read_text())
        except (FileNotFoundError, json.JSONDecodeError):
            return {}

    def _save_face_cache(self, cache: dict):
        FACE_CACHE_PATH.write_text(json.dumps(cache))

    # ------------------------------------------------------------------ #
    #  Lazy face_recognition import
    # ------------------------------------------------------------------ #

    def _ensure_face_recognition(self) -> bool:
        if self._face_recognition is not None:
            return True
        try:
            import face_recognition

            self._face_recognition = face_recognition
            return True
        except ImportError:
            messagebox.showerror(
                "Missing dependency",
                "The 'face_recognition' package is not installed.\n\n"
                "Install it with:\n"
                "  pip install face_recognition\n\n"
                "On macOS you may also need:\n"
                "  xcode-select --install",
            )
            return False

    # ------------------------------------------------------------------ #
    #  Enrollment
    # ------------------------------------------------------------------ #

    def _on_enroll_face(self):
        if not self._ensure_face_recognition():
            return

        file_path = filedialog.askopenfilename(
            title="Select a photo with exactly one face",
            filetypes=[("Image files", "*.jpg *.jpeg *.png *.bmp *.webp *.tiff")],
        )
        if not file_path:
            return

        fr = self._face_recognition
        self._set_status("Detecting face...")
        self.root.update_idletasks()

        try:
            image = fr.load_image_file(file_path)
        except Exception as exc:
            messagebox.showerror("Error", f"Could not load image:\n{exc}")
            self._set_status("")
            return

        locations = fr.face_locations(image, model="hog")

        if len(locations) == 0:
            messagebox.showwarning("No face found", "No face was detected in this photo.")
            self._set_status("")
            return

        if len(locations) > 1:
            messagebox.showwarning(
                "Multiple faces",
                f"{len(locations)} faces detected. Please select a photo with exactly one face.",
            )
            self._set_status("")
            return

        encodings = fr.face_encodings(image, known_face_locations=locations)
        encoding = encodings[0].tolist()

        name = simpledialog.askstring("Person name", "Enter name for this person:")
        if not name or not name.strip():
            self._set_status("")
            return
        name = name.strip()

        registry = self._load_face_registry()
        if name not in registry["people"]:
            registry["people"][name] = {"embeddings": []}

        registry["people"][name]["embeddings"].append(
            {"source": file_path, "encoding": encoding}
        )
        self._save_face_registry(registry)
        self._refresh_enrolled_list()
        self._set_status(f"Enrolled face for '{name}'.")

    def _refresh_enrolled_list(self):
        registry = self._load_face_registry()
        names = sorted(registry["people"].keys())

        self.enrolled_listbox.delete(0, tk.END)
        for name in names:
            count = len(registry["people"][name]["embeddings"])
            self.enrolled_listbox.insert(tk.END, f"{name} ({count} photo{'s' if count != 1 else ''})")

        self.person_combo["values"] = names
        if names and not self.person_var.get():
            self.person_var.set(names[0])

    def _on_remove_person(self):
        sel = self.enrolled_listbox.curselection()
        if not sel:
            messagebox.showinfo("Remove", "Select a person from the list first.")
            return

        registry = self._load_face_registry()
        names = sorted(registry["people"].keys())
        name = names[sel[0]]

        if not messagebox.askyesno("Confirm", f"Remove '{name}' and all enrolled photos?"):
            return

        del registry["people"][name]
        self._save_face_registry(registry)
        self._refresh_enrolled_list()
        self._set_status(f"Removed '{name}'.")

    # ------------------------------------------------------------------ #
    #  Face search
    # ------------------------------------------------------------------ #

    def _on_face_search(self):
        if self._searching:
            return
        if not self._ensure_face_recognition():
            return

        person = self.person_var.get().strip()
        image_dir = self.dir_var.get().strip()

        if not person:
            self._set_status("Select a person to search for.")
            return
        if not image_dir or not Path(image_dir).is_dir():
            self._set_status("Select a valid image directory.")
            return

        try:
            max_distance = float(self.face_distance_var.get().strip())
            if max_distance <= 0:
                self._set_status("Max distance must be positive.")
                return
        except ValueError:
            self._set_status("Max distance must be a number.")
            return

        registry = self._load_face_registry()
        if person not in registry["people"]:
            self._set_status(f"Person '{person}' not found in registry.")
            return

        reference_encodings = [
            np.array(e["encoding"])
            for e in registry["people"][person]["embeddings"]
        ]

        self._searching = True
        self._disable_search_buttons()
        self._clear_results()
        self._set_status("Starting face search...")

        thread = threading.Thread(
            target=self._run_face_search,
            args=(Path(image_dir), person, reference_encodings, max_distance),
            daemon=True,
        )
        thread.start()

    def _run_face_search(
        self,
        image_dir: Path,
        person: str,
        reference_encodings: list,
        max_distance: float,
    ):
        fr = self._face_recognition
        try:
            image_paths = sorted(
                p for p in image_dir.iterdir()
                if p.is_file() and p.suffix.lower() in IMAGE_EXTENSIONS
            )
            if not image_paths:
                self.root.after(0, self._set_status, "No images found in directory.")
                self.root.after(0, self._finish_face_search)
                return

            total = len(image_paths)
            self.root.after(0, self._set_status, f"Scanning {total} images for '{person}'...")

            cache = self._load_face_cache()
            results = []

            for i, path in enumerate(image_paths):
                path_str = str(path)
                try:
                    mtime = os.path.getmtime(path_str)
                except OSError:
                    continue

                # Check cache
                cached = cache.get(path_str)
                if cached and cached.get("mtime") == mtime:
                    face_encodings = [np.array(e) for e in cached["encodings"]]
                else:
                    # Detect and encode faces
                    try:
                        image = fr.load_image_file(path_str)
                        locations = fr.face_locations(image, model="hog")
                        if locations:
                            face_encodings = fr.face_encodings(image, known_face_locations=locations)
                        else:
                            face_encodings = []
                    except Exception:
                        face_encodings = []

                    # Update cache
                    cache[path_str] = {
                        "mtime": mtime,
                        "encodings": [e.tolist() for e in face_encodings],
                    }

                # Compare against reference embeddings
                if face_encodings:
                    best_distance = float("inf")
                    for face_enc in face_encodings:
                        distances = fr.face_distance(reference_encodings, face_enc)
                        min_dist = float(np.min(distances))
                        if min_dist < best_distance:
                            best_distance = min_dist

                    if best_distance <= max_distance:
                        results.append((path, best_distance))

                # Progress update every 20 images
                if (i + 1) % 20 == 0 or i + 1 == total:
                    progress_msg = (
                        f"Scanning {i + 1}/{total}... "
                        f"({len(results)} match{'es' if len(results) != 1 else ''} so far)"
                    )
                    self.root.after(0, self._set_status, progress_msg)

            # Save updated cache
            self._save_face_cache(cache)

            # Sort by distance ascending, display score as 1 - distance
            results.sort(key=lambda x: x[1])
            display_results = [(path, 1.0 - dist) for path, dist in results]

            self.root.after(0, self._show_results, display_results)
            self.root.after(
                0,
                self._set_status,
                f"Done. {len(results)} match{'es' if len(results) != 1 else ''} "
                f"for '{person}' in {total} images (max distance {max_distance}).",
            )

        except Exception as exc:
            self.root.after(0, self._set_status, f"Error: {exc}")

        finally:
            self.root.after(0, self._finish_face_search)

    def _finish_face_search(self):
        self._searching = False
        self._enable_search_buttons()

    # ------------------------------------------------------------------ #
    #  Status / search button locking
    # ------------------------------------------------------------------ #

    def _set_status(self, msg: str):
        self.status_var.set(msg)

    def _disable_search_buttons(self):
        self.search_btn.config(state=tk.DISABLED)
        self.face_search_btn.config(state=tk.DISABLED)

    def _enable_search_buttons(self):
        self.search_btn.config(state=tk.NORMAL)
        self.face_search_btn.config(state=tk.NORMAL)

    # ------------------------------------------------------------------ #
    #  Text search (existing)
    # ------------------------------------------------------------------ #

    def _on_search(self):
        if self._searching:
            return

        query = self.query_entry.get().strip()
        image_dir = self.dir_var.get().strip()

        if not query:
            self._set_status("Enter a query.")
            return
        if not image_dir or not Path(image_dir).is_dir():
            self._set_status("Select a valid image directory.")
            return

        try:
            threshold_pct = float(self.threshold_var.get().strip())
            if not (0 <= threshold_pct <= 100):
                self._set_status("Min similarity % must be between 0 and 100.")
                return
        except ValueError:
            self._set_status("Min similarity % must be a number.")
            return

        self._save_config()
        self._searching = True
        self._disable_search_buttons()
        self._clear_results()
        self._set_status("Starting search...")

        thread = threading.Thread(
            target=self._run_search, args=(query, Path(image_dir), threshold_pct), daemon=True
        )
        thread.start()

    def _run_search(self, query: str, image_dir: Path, threshold_pct: float):
        try:
            # Collect images
            image_paths = sorted(
                p for p in image_dir.iterdir()
                if p.is_file() and p.suffix.lower() in IMAGE_EXTENSIONS
            )
            if not image_paths:
                self.root.after(0, self._set_status, "No images found in directory.")
                self.root.after(0, self._finish_search)
                return

            self.root.after(
                0, self._set_status, f"Found {len(image_paths)} images."
            )

            # Load model (lazily, once)
            if self.model is None:
                self.root.after(0, self._set_status, "Loading model...")
                model_kwargs = {}
                if not any(MODEL_NAME.endswith(s) for s in STANDARD_NORM_SUFFIXES):
                    model_kwargs = {"image_mean": (0, 0, 0), "image_std": (1, 1, 1)}

                model, _, preprocess = open_clip.create_model_and_transforms(
                    MODEL_NAME, pretrained="dfndr2b", **model_kwargs
                )
                tokenizer = open_clip.get_tokenizer(MODEL_NAME)
                model.eval()
                model = reparameterize_model(model)
                model = model.to(self.device)

                self.model = model
                self.preprocess = preprocess
                self.tokenizer = tokenizer

            # Encode images (skip unreadable files)
            self.root.after(0, self._set_status, "Encoding images...")
            valid_paths = []
            all_image_features = []
            for i in range(0, len(image_paths), BATCH_SIZE):
                batch_paths = image_paths[i : i + BATCH_SIZE]
                tensors = []
                kept = []
                for p in batch_paths:
                    try:
                        tensors.append(self.preprocess(Image.open(p).convert("RGB")))
                        kept.append(p)
                    except Exception:
                        continue
                if not tensors:
                    continue
                valid_paths.extend(kept)
                batch_tensors = torch.stack(tensors).to(self.device)

                with torch.no_grad(), torch.amp.autocast(
                    self.device, enabled=(self.device == "cuda")
                ):
                    features = self.model.encode_image(batch_tensors)
                    features /= features.norm(dim=-1, keepdim=True)

                all_image_features.append(features.cpu())

            if not all_image_features:
                self.root.after(0, self._set_status, "No readable images found.")
                self.root.after(0, self._finish_search)
                return

            image_features = torch.cat(all_image_features, dim=0)
            image_paths = valid_paths

            # Encode text
            self.root.after(0, self._set_status, "Encoding query...")
            tokens = self.tokenizer([query]).to(self.device)
            with torch.no_grad(), torch.amp.autocast(
                self.device, enabled=(self.device == "cuda")
            ):
                text_features = self.model.encode_text(tokens)
                text_features /= text_features.norm(dim=-1, keepdim=True)
            text_features = text_features.cpu()

            # Rank
            similarities = (image_features @ text_features.T).squeeze(1)
            ranked_indices = similarities.argsort(descending=True)

            top_score = similarities[ranked_indices[0]].item()
            cutoff = top_score * (threshold_pct / 100.0) if top_score > 0 else 0.0

            results = [
                (image_paths[idx], similarities[idx].item())
                for idx in ranked_indices
                if similarities[idx].item() >= cutoff
            ]

            self.root.after(0, self._show_results, results)
            self.root.after(
                0,
                self._set_status,
                f"Done. {len(results)} of {len(image_paths)} images above "
                f"{threshold_pct:.0f}% of top score ({top_score:.4f}) for \"{query}\".",
            )

        except Exception as exc:
            self.root.after(0, self._set_status, f"Error: {exc}")

        finally:
            self.root.after(0, self._finish_search)

    def _finish_search(self):
        self._searching = False
        self._enable_search_buttons()

    # ------------------------------------------------------------------ #
    #  Results display (shared)
    # ------------------------------------------------------------------ #

    def _clear_results(self):
        for widget in self.results_frame.winfo_children():
            widget.destroy()
        self._photo_refs.clear()

    def _show_results(self, results: list[tuple[Path, float]]):
        self._clear_results()
        for path, score in results:
            row = tk.Frame(self.results_frame, bd=1, relief=tk.RIDGE)
            row.pack(fill=tk.X, pady=2, padx=2)

            try:
                img = Image.open(path).convert("RGB")
                img.thumbnail(THUMBNAIL_SIZE)
                photo = ImageTk.PhotoImage(img)
                self._photo_refs.append(photo)
                tk.Label(row, image=photo).pack(side=tk.LEFT, padx=5, pady=5)
            except Exception:
                tk.Label(row, text="[error]", width=20).pack(side=tk.LEFT, padx=5, pady=5)

            info = tk.Frame(row)
            info.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=5)
            tk.Label(info, text=path.name, anchor=tk.W, font=("TkDefaultFont", 12)).pack(
                anchor=tk.W
            )
            tk.Label(info, text=f"Score: {score:.4f}", anchor=tk.W).pack(anchor=tk.W)

        # Scroll to top
        self.canvas.yview_moveto(0)


def main():
    root = tk.Tk()
    App(root)
    root.mainloop()


if __name__ == "__main__":
    main()
