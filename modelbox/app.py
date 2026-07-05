"""HTTP surface for the model box: /embed, /rerank, /health. Go and the embed
worker are the callers; this process just runs models."""

from fastapi import FastAPI
from pydantic import BaseModel

import models
from config import EMBED_MODEL, RERANK_MODEL

app = FastAPI()


class EmbedRequest(BaseModel):
    texts: list[str]
    is_query: bool = False


class RerankRequest(BaseModel):
    query: str
    passages: list[str]


@app.post("/embed")
def embed(req: EmbedRequest):
    return {"embeddings": models.embed(req.texts, req.is_query)}


@app.post("/rerank")
def rerank(req: RerankRequest):
    return {"scores": models.rerank(req.query, req.passages)}


@app.get("/health")
def health():
    # Loads both models on the first call so later requests are warm.
    models.warm()
    return {"status": "ok", "embed_model": EMBED_MODEL, "rerank_model": RERANK_MODEL}
