"""
pyworker: local ingestion + embedding sidecar for the Hypothesis Factory Go service.

- POST /ingest  : parse PDF/DOCX/XLSX/PPTX/images via Docling, return semantic chunks
                   (section-aware: headings, paragraphs, tables), not fixed-size windows.
- POST /embed   : BGE-M3 dense embeddings (multilingual, up to 8192 tokens).
- POST /rerank  : bge-reranker-v2-m3 cross-encoder scores for a query against candidates.

Runs fully offline once models are cached locally (air-gapped deployment).
"""
from __future__ import annotations

import asyncio
import re
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


_LETTER_SPACING_RE = re.compile(r"(?:\b\w\b[ \-]+){3,}\b\w\b", re.UNICODE)


def fix_letter_spacing(text: str) -> str:
    """Docling/OCR occasionally extracts text from wide-tracking table/heading
    layouts as a run of single-letter "words" ("Э м и с с и о н н о - р а д
    и о м е т р и ч е с к и е" instead of "Эмиссионно-радиометрические").
    Collapse any run of 4+ single-character tokens separated by spaces/hyphens
    back into one word; short real words ("и", "о", "в") are untouched since
    they never appear in a run of 4+.
    """

    def collapse(m: re.Match) -> str:
        return re.sub(r"[ \-]+", "", m.group(0))

    return _LETTER_SPACING_RE.sub(collapse, text)


def _table_markdown(doc, chunk) -> str | None:
    """HybridChunker's default table serializer produces triplet text
    ("row = col = value") that severs a table from the row/column semantics a
    reader (or an LLM doing claim extraction) needs. TableItem.export_to_markdown
    keeps the grid structure instead, which grounds claims far better.
    Returns None if the chunk has no table doc_items (caller falls back to the
    HybridChunker serialization).
    """
    from docling_core.types.doc import TableItem

    meta = getattr(chunk, "meta", None)
    if meta is None or not getattr(meta, "doc_items", None):
        return None
    tables = [it for it in meta.doc_items if isinstance(it, TableItem)]
    if not tables:
        return None
    parts = [t.export_to_markdown(doc) for t in tables]
    parts = [p.strip() for p in parts if p and p.strip()]
    return "\n\n".join(parts) if parts else None


def _ingest_sync(tmp_path: str, filename: str) -> list[Chunk]:
    from docling.chunking import HybridChunker

    converter = get_docling_converter()
    result = converter.convert(tmp_path)
    doc = result.document

    chunker = HybridChunker()
    chunks: list[Chunk] = []
    for i, chunk in enumerate(chunker.chunk(doc)):
        heading = None
        meta = getattr(chunk, "meta", None)
        if meta is not None and getattr(meta, "headings", None):
            heading = " / ".join(meta.headings)

        content_type = "text"
        is_table = False
        if meta is not None and getattr(meta, "doc_items", None):
            labels = {getattr(it, "label", "") for it in meta.doc_items}
            is_table = any("table" in str(l).lower() for l in labels)

        if is_table:
            content_type = "table"
            text = _table_markdown(doc, chunk)
            if text is None:
                text = chunker.serialize(chunk)
                text = fix_degenerate_single_column_table(text)
        else:
            text = chunker.serialize(chunk)

        if not text or not text.strip():
            continue
        text = fix_letter_spacing(text.strip())

        chunks.append(
            Chunk(
                ordinal=i,
                section=heading,
                content=text,
                content_type=content_type,
                metadata={"source_file": filename},
            )
        )
    return chunks


@app.post("/ingest", response_model=IngestResponse)
async def ingest(file: UploadFile = File(...)):
    # Docling conversion is synchronous, CPU/GPU-bound work that can run for
    # tens of minutes on a large book. Running it directly in this async def
    # blocks FastAPI's single event loop for the whole duration — even
    # /healthz stops responding, and every other in-flight request queues
    # behind it. asyncio.to_thread offloads it to a worker thread so the
    # event loop stays free to serve concurrent requests.
    suffix = Path(file.filename or "upload").suffix or ".bin"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(await file.read())
        tmp_path = tmp.name

    chunks = await asyncio.to_thread(_ingest_sync, tmp_path, file.filename)
    return IngestResponse(chunks=chunks)


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


def _embed_sync(texts: list[str]) -> list[list[float]]:
    model = get_bge_model()
    vecs = model.encode(texts, batch_size=12, show_progress_bar=False, normalize_embeddings=True)
    return [v.tolist() for v in vecs]


@app.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest):
    embeddings = await asyncio.to_thread(_embed_sync, req.texts)
    return EmbedResponse(embeddings=embeddings)


class RerankRequest(BaseModel):
    query: str
    candidates: list[str]


class RerankResponse(BaseModel):
    scores: list[float]


def _rerank_sync(query: str, candidates: list[str]) -> list[float]:
    model = get_reranker_model()
    pairs = [[query, c] for c in candidates]
    scores = model.predict(pairs, show_progress_bar=False)
    return [float(s) for s in scores]


@app.post("/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest):
    scores = await asyncio.to_thread(_rerank_sync, req.query, req.candidates)
    return RerankResponse(scores=scores)


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}
