"""
pyworker: local ingestion + embedding sidecar for the Hypothesis Factory Go service.

- POST /ingest         : parse PDF/DOCX/XLSX/PPTX/images via Docling, return semantic
                          chunks (section-aware: headings, paragraphs, tables), not
                          fixed-size windows.
- POST /ingest-article : parse a scientific-paper PDF via GROBID (structure-aware:
                          title/authors/abstract/sections, not OCR-ish text soup),
                          enrich with Semantic Scholar metadata (citation count, venue,
                          year) best-effort.
- POST /embed          : BGE-M3 dense embeddings (multilingual, up to 8192 tokens).
- POST /rerank          : bge-reranker-v2-m3 cross-encoder scores for a query against candidates.

Runs fully offline once models are cached locally (air-gapped deployment) — except
/ingest-article's Semantic Scholar enrichment step, which is a best-effort network call
that's skipped (not fatal) if unreachable.
"""
from __future__ import annotations

import asyncio
import os
import re
import tempfile
from pathlib import Path
from typing import Literal
from xml.etree import ElementTree as ET

import requests
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


GROBID_URL = os.environ.get("GROBID_URL", "http://grobid:8070")
SEMANTIC_SCHOLAR_URL = "https://api.semanticscholar.org/graph/v1/paper/search"

_TEI_NS = {"t": "http://www.tei-c.org/ns/1.0"}


def _tei_text(el) -> str:
    """Join all text content under an element, collapsing whitespace — TEI
    wraps inline elements (refs, formulas) inside <p>, plain itertext() is
    good enough for claim-extraction purposes (we don't need the markup)."""
    return re.sub(r"\s+", " ", "".join(el.itertext())).strip()


_CHUNK_CHAR_BUDGET = 2000
_HARD_PARA_CAP = 4000


def _group_paragraphs(paragraphs: list[str]) -> list[str]:
    """Groups consecutive paragraphs up to ~_CHUNK_CHAR_BUDGET chars each —
    an unbounded whole-<div>-per-chunk (a single GROBID section can run to
    10k+ chars in a long paper) blows up embedding attention memory
    (quadratic in sequence length) even within BGE-M3's nominal 8192-token
    window. Never splits a paragraph mid-sentence, except the (rare) single
    paragraph that alone exceeds _HARD_PARA_CAP, which is whitespace-sliced
    as a last resort so one runaway paragraph can't OOM the batch either.
    """
    groups: list[str] = []
    current: list[str] = []
    current_len = 0
    for p in paragraphs:
        if len(p) > _HARD_PARA_CAP:
            if current:
                groups.append("\n\n".join(current))
                current, current_len = [], 0
            words = p.split(" ")
            piece = ""
            for w in words:
                if len(piece) + len(w) + 1 > _HARD_PARA_CAP and piece:
                    groups.append(piece)
                    piece = ""
                piece = f"{piece} {w}".strip()
            if piece:
                groups.append(piece)
            continue
        if current and current_len + len(p) > _CHUNK_CHAR_BUDGET:
            groups.append("\n\n".join(current))
            current, current_len = [], 0
        current.append(p)
        current_len += len(p)
    if current:
        groups.append("\n\n".join(current))
    return groups


def _parse_tei_to_chunks(tei_xml: str, filename: str) -> tuple[list[Chunk], dict]:
    """Turns GROBID's TEI XML into the same Chunk shape /ingest produces, so
    downstream (Go-side claim extraction, parent-child neighbor context) needs
    no article-specific handling. One chunk per <div> section (GROBID already
    segments by heading — no need for Docling's HybridChunker here) plus one
    abstract chunk. Returns (chunks, header_metadata) — header_metadata (title/
    authors/year) is attached to every chunk so claim extraction can cite the
    paper by name even from a body chunk.
    """
    root = ET.fromstring(tei_xml)

    title_el = root.find(".//t:teiHeader//t:titleStmt/t:title", _TEI_NS)
    title = _tei_text(title_el) if title_el is not None else filename

    authors = []
    for pers in root.findall(".//t:sourceDesc//t:biblStruct//t:author/t:persName", _TEI_NS):
        forename = pers.find("t:forename", _TEI_NS)
        surname = pers.find("t:surname", _TEI_NS)
        name = " ".join(
            p.text for p in (forename, surname) if p is not None and p.text
        )
        if name:
            authors.append(name)

    date_el = root.find(".//t:sourceDesc//t:biblStruct//t:date", _TEI_NS)
    year = date_el.get("when", "")[:4] if date_el is not None else ""

    header_meta = {"source_file": filename, "article_title": title}
    if authors:
        header_meta["article_authors"] = ", ".join(authors)
    if year:
        header_meta["article_year"] = year

    chunks: list[Chunk] = []
    ordinal = 0

    abstract_paras = [_tei_text(p) for p in root.findall(".//t:profileDesc/t:abstract//t:p", _TEI_NS)]
    abstract_paras = [p for p in abstract_paras if p]
    for group in _group_paragraphs(abstract_paras):
        chunks.append(Chunk(
            ordinal=ordinal, section="Abstract", content=fix_letter_spacing(group),
            content_type="text", metadata=header_meta,
        ))
        ordinal += 1

    for div in root.findall(".//t:text/t:body/t:div", _TEI_NS):
        head_el = div.find("t:head", _TEI_NS)
        heading = _tei_text(head_el) if head_el is not None else None
        paras = [_tei_text(p) for p in div.findall("t:p", _TEI_NS)]
        paras = [p for p in paras if p]
        for group in _group_paragraphs(paras):
            chunks.append(Chunk(
                ordinal=ordinal, section=heading, content=fix_letter_spacing(group),
                content_type="text", metadata=header_meta,
            ))
            ordinal += 1

    return chunks, header_meta


def _semantic_scholar_enrich(title: str) -> dict:
    """Best-effort authority signal (citation count, venue, year) for a paper
    by title search — not required for the pipeline to work, so any failure
    (network, rate limit, no match) is swallowed and just yields no enrichment
    rather than failing the whole ingest.
    """
    if not title:
        return {}
    try:
        resp = requests.get(
            SEMANTIC_SCHOLAR_URL,
            params={"query": title, "fields": "title,year,citationCount,venue,externalIds", "limit": 1},
            timeout=10,
        )
        resp.raise_for_status()
        data = resp.json().get("data") or []
        if not data:
            return {}
        paper = data[0]
        enrichment = {}
        if paper.get("citationCount") is not None:
            enrichment["s2_citation_count"] = str(paper["citationCount"])
        if paper.get("year") is not None:
            enrichment["s2_year"] = str(paper["year"])
        if paper.get("venue"):
            enrichment["s2_venue"] = paper["venue"]
        doi = (paper.get("externalIds") or {}).get("DOI")
        if doi:
            enrichment["s2_doi"] = doi
        return enrichment
    except Exception as e:
        # Best-effort: don't fail ingestion over a flaky/rate-limited external
        # API, but do log it — a silent except here previously made a 429 from
        # Semantic Scholar's tight anonymous rate limit indistinguishable from
        # "no matching paper found".
        print(f"[semantic_scholar_enrich] skipped for {title!r}: {e}")
        return {}


def _ingest_article_sync(tmp_path: str, filename: str) -> list[Chunk]:
    with open(tmp_path, "rb") as f:
        resp = requests.post(
            f"{GROBID_URL}/api/processFulltextDocument",
            files={"input": (filename, f, "application/pdf")},
            data={"consolidateHeader": "1", "consolidateCitations": "0"},
            timeout=300,
        )
    resp.raise_for_status()

    chunks, header_meta = _parse_tei_to_chunks(resp.text, filename)
    if not chunks:
        return chunks

    enrichment = _semantic_scholar_enrich(header_meta.get("article_title", ""))
    if enrichment:
        for c in chunks:
            c.metadata.update(enrichment)
    return chunks


@app.post("/ingest-article", response_model=IngestResponse)
async def ingest_article(file: UploadFile = File(...)):
    # Same event-loop-blocking concern as /ingest: the GROBID HTTP call plus
    # XML parsing runs in a worker thread so /healthz stays responsive.
    suffix = Path(file.filename or "upload").suffix or ".pdf"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(await file.read())
        tmp_path = tmp.name

    chunks = await asyncio.to_thread(_ingest_article_sync, tmp_path, file.filename)
    return IngestResponse(chunks=chunks)


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
