"""
pyworker: local ingestion + embedding sidecar for the Hypothesis Factory Go service.

- POST /ingest  : parse PDF/DOCX/XLSX/PPTX/images via Docling, return semantic chunks
                   (section-aware: headings, paragraphs, tables), not fixed-size windows.
- POST /embed   : BGE-M3 dense embeddings (multilingual, up to 8192 tokens).
- POST /rerank  : bge-reranker-v2-m3 cross-encoder scores for a query against candidates.

Runs fully offline once models are cached locally (air-gapped deployment).
"""
from __future__ import annotations

import tempfile
from pathlib import Path
from typing import Literal

from fastapi import FastAPI, File, UploadFile
from pydantic import BaseModel

app = FastAPI(title="hypothesis-factory-pyworker")

_docling_converter = None
_bge_model = None
_reranker_model = None


def get_docling_converter():
    global _docling_converter
    if _docling_converter is None:
        from docling.document_converter import DocumentConverter
        _docling_converter = DocumentConverter()
    return _docling_converter


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
        # model's actual trained 8192-token context — a real loss for long
        # regulation/textbook chunks. Use the full context it supports.
        _bge_model.max_seq_length = 8192
    return _bge_model


def get_reranker_model():
    global _reranker_model
    if _reranker_model is None:
        from sentence_transformers import CrossEncoder
        _reranker_model = CrossEncoder("BAAI/bge-reranker-v2-m3")
    return _reranker_model


class Chunk(BaseModel):
    ordinal: int
    section: str | None = None
    content: str
    content_type: Literal["text", "table", "figure_caption"] = "text"
    metadata: dict = {}


class IngestResponse(BaseModel):
    chunks: list[Chunk]


def fix_degenerate_single_column_table(text: str) -> str:
    """Docling's TableFormer treats a single-column Word table (a plain
    bulleted list rendered as a bordered table, e.g. brainstormed hypothesis
    lists) as having row 0 as a header, then HybridChunker's table serializer
    repeats "header = row" for every subsequent row: "2. X = 3. Y. 2. X = 4. Z."
    That's unusable for claim extraction. Detect the repeating-prefix pattern
    and reconstruct the original list instead.
    """
    if text.count(" = ") < 2:
        return text
    parts = text.split(" = ")
    header = parts[0].strip()
    if not header:
        return text
    items: list[str] = []
    for p in parts[1:]:
        p = p.strip()
        if p.endswith(header):
            item = p[: -len(header)].rstrip(" .")
        elif header and header in p:
            item = p.split(header)[0].rstrip(" .")
        else:
            item = p.rstrip(" .")
        if item:
            items.append(item)
    # Only trust the reconstruction if it actually recovered multiple distinct
    # items — otherwise this wasn't the degenerate pattern, leave text as-is.
    if len(items) < 2 or len(set(items)) < len(items) - 1:
        return text
    return header + "\n" + "\n".join(items)


@app.post("/ingest", response_model=IngestResponse)
async def ingest(file: UploadFile = File(...)):
    from docling.chunking import HybridChunker

    suffix = Path(file.filename or "upload").suffix or ".bin"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(await file.read())
        tmp_path = tmp.name

    converter = get_docling_converter()
    result = converter.convert(tmp_path)
    doc = result.document

    chunker = HybridChunker()
    chunks: list[Chunk] = []
    for i, chunk in enumerate(chunker.chunk(doc)):
        text = chunker.serialize(chunk)
        if not text or not text.strip():
            continue
        heading = None
        meta = getattr(chunk, "meta", None)
        if meta is not None and getattr(meta, "headings", None):
            heading = " / ".join(meta.headings)
        content_type = "text"
        if meta is not None and getattr(meta, "doc_items", None):
            labels = {getattr(it, "label", "") for it in meta.doc_items}
            if any("table" in str(l).lower() for l in labels):
                content_type = "table"
                text = fix_degenerate_single_column_table(text)
        chunks.append(
            Chunk(
                ordinal=i,
                section=heading,
                content=text.strip(),
                content_type=content_type,
                metadata={"source_file": file.filename},
            )
        )

    return IngestResponse(chunks=chunks)


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


@app.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest):
    model = get_bge_model()
    vecs = model.encode(req.texts, batch_size=12, show_progress_bar=False, normalize_embeddings=True)
    return EmbedResponse(embeddings=[v.tolist() for v in vecs])


class RerankRequest(BaseModel):
    query: str
    candidates: list[str]


class RerankResponse(BaseModel):
    scores: list[float]


@app.post("/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest):
    model = get_reranker_model()
    pairs = [[req.query, c] for c in req.candidates]
    scores = model.predict(pairs, show_progress_bar=False)
    return RerankResponse(scores=[float(s) for s in scores])


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}
