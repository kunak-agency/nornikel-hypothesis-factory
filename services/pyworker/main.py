"""
pyworker: runtime-сервис эмбеддингов/реранкинга для Go-сервиса Hypothesis
Factory — часть пайплайна, которая выполняется на каждый запрос POST /v1/runs.

- POST /embed   : плотные эмбеддинги BGE-M3 (мультиязычные, до 8192 токенов).
- POST /rerank  : скоринг bge-reranker-v2-m3 (запрос против кандидатов).

Намеренно не включает Docling/ingestion — это тяжёлая, разовая офлайн-задача
со своим GPU-инструментом (tools/ingestion), не часть runtime-профиля сдаваемого
решения. База знаний поставляется как готовый дамп Postgres; этот сервис
только обслуживает живые запросы к ней.
"""
from __future__ import annotations

import asyncio

from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="hypothesis-factory-pyworker")

_bge_model = None
_reranker_model = None


def get_bge_model():
    # sentence-transformers, не FlagEmbedding: FlagEmbedding 1.3.4 при импорте
    # эагерно подтягивает decoder-only (Gemma) код реранкера, который ломается
    # об текущий transformers — хотя нужны только encoder-only BGE-M3/
    # bge-reranker-v2-m3.
    global _bge_model
    if _bge_model is None:
        from sentence_transformers import SentenceTransformer
        _bge_model = SentenceTransformer("BAAI/bge-m3")
        # sentence-transformers по умолчанию ставит BGE-M3 max_seq_length=512
        # (из сохранённого ST-конфига) — должно совпадать с настройкой
        # ingestion-инструмента, иначе векторы запроса/документа разъезжаются.
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
    # Ретрай с уменьшающимся batch_size при GPU OOM вместо падения запроса —
    # см. tools/ingestion/main.py, тот же фикс и причина.
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
            torch.cuda.empty_cache()
    raise AssertionError("unreachable")


@app.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest):
    # asyncio.to_thread — держит event loop свободным (/healthz отзывчив),
    # пока модель работает.
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
