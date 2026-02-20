"""FastAPI ML sidecar for CLIP inference and search."""

from contextlib import asynccontextmanager

from fastapi import FastAPI
from pydantic import BaseModel

from clip_encoder import CLIPEncoder
from searcher import Searcher

encoder: CLIPEncoder | None = None
searcher: Searcher | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global encoder, searcher
    encoder = CLIPEncoder()
    searcher = Searcher(encoder)
    yield


app = FastAPI(title="intelsk ML sidecar", lifespan=lifespan)


# --- Request models ---

class EncodeImageRequest(BaseModel):
    paths: list[str]


class EncodeTextRequest(BaseModel):
    text: str


class SearchImageRequest(BaseModel):
    db_path: str
    text: str
    camera_ids: list[str] | None = None
    start_time: str | None = None
    end_time: str | None = None
    limit: int = 20
    min_score: float = 0.18


# --- Endpoints ---

@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/encode/image")
def encode_image(req: EncodeImageRequest):
    embeddings = encoder.encode_images(req.paths)
    return {"embeddings": [emb.tolist() for emb in embeddings]}


@app.post("/encode/text")
def encode_text(req: EncodeTextRequest):
    embedding = encoder.encode_text(req.text)
    return {"embedding": embedding.tolist()}


@app.post("/search/image")
def search_image(req: SearchImageRequest):
    results = searcher.search_by_text(
        db_path=req.db_path,
        text=req.text,
        camera_ids=req.camera_ids,
        start_time=req.start_time,
        end_time=req.end_time,
        limit=req.limit,
        min_score=req.min_score,
    )
    return {"results": results}
