"""
pyworker: runtime embedding/reranking sidecar for the Hypothesis Factory Go
service — the part of the pipeline that runs on every POST /v1/runs query.

- POST /embed   : BGE-M3 dense embeddings (multilingual, up to 8192 tokens).
- POST /rerank  : bge-reranker-v2-m3 cross-encoder scores for a query against candidates.

Deliberately does NOT include Docling/ingestion — that's a heavy, one-off
offline job with its own GPU-accelerated tool (tools/ingestion), never part
of the submitted solution's runtime footprint. The knowledge base ships as a
pre-built Postgres dump; this service only serves live queries against it.
"""
from __future__ import annotations

import asyncio

from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="hypothesis-factory-pyworker")

_bge_model = None
_reranker_model = None


def get_bge_model():
    # sentence-transformers, not FlagEmbedding: FlagEmbedding 1.3.4 eagerly
    # imports decoder-only (Gemma-based) reranker code at import time that
    # breaks against current transformers internals, even though we only
    # need the encoder-only BGE-M3/bge-reranker-v2-m3 models.
    global _bge_model
    if _bge_model is None:
        from sentence_transformers import SentenceTransformer
        _bge_model = SentenceTransformer("BAAI/bge-m3")
        # sentence-transformers defaults BGE-M3 to a 512-token max_seq_length
        # (from its saved ST config), silently truncating well below the
        # model's actual trained 8192-token context — must match the
        # ingestion tool's setting or query/document vectors diverge.
        _bge_model.max_seq_length = 8192
    return _bge_model


def get_reranker_model():
    global _reranker_model
    if _reranker_model is None:
        from sentence_transformers import CrossEncoder
        _reranker_model = CrossEncoder("BAAI/bge-reranker-v2-m3")
    return _reranker_model


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


def _embed_sync(texts: list[str]) -> list[list[float]]:
    # Retry with a shrinking batch size on GPU OOM instead of failing the
    # request outright — see tools/ingestion/main.py for the same fix and
    # why (long chunks at the model's 8192-token max_seq_length can attempt
    # a multi-GiB single allocation).
    import torch

    for batch_size in (12, 4, 1):
        try:
            model = get_bge_model()
            vecs = model.encode(texts, batch_size=batch_size, show_progress_bar=False, normalize_embeddings=True)
            return [v.tolist() for v in vecs]
        except torch.OutOfMemoryError:
            torch.cuda.empty_cache()
            if batch_size == 1:
                raise
        finally:
            # A caching allocator doesn't return freed blocks to the OS/driver
            # by default; across many queries with different batch/sequence
            # shapes those blocks can fragment until an allocation that would
            # easily fit fails anyway. Cheap relative to a whole embed call.
            torch.cuda.empty_cache()
    raise AssertionError("unreachable")


@app.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest):
    # asyncio.to_thread keeps the event loop free (e.g. /healthz responsive)
    # while the model runs — a single query embedding is fast, but this
    # avoids reintroducing the blocking-event-loop bug under concurrent load.
    embeddings = await asyncio.to_thread(_embed_sync, req.texts)
    return EmbedResponse(embeddings=embeddings)


class RerankRequest(BaseModel):
    query: str
    candidates: list[str]


class RerankResponse(BaseModel):
    scores: list[float]


def _rerank_sync(query: str, candidates: list[str]) -> list[float]:
    import torch

    pairs = [[query, c] for c in candidates]
    for batch_size in (32, 8, 1):
        try:
            model = get_reranker_model()
            scores = model.predict(pairs, show_progress_bar=False, batch_size=batch_size)
            return [float(s) for s in scores]
        except torch.OutOfMemoryError:
            torch.cuda.empty_cache()
            if batch_size == 1:
                raise
        finally:
            torch.cuda.empty_cache()
    raise AssertionError("unreachable")


@app.post("/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest):
    scores = await asyncio.to_thread(_rerank_sync, req.query, req.candidates)
    return RerankResponse(scores=scores)


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}
