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
    # Docling's bundled RapidOCR integration (this Docling version, 2.108.0)
    # only ships "english"/"latin"/"chinese" model sets — there is no
    # Cyrillic set at all, so passing lang=["ru"] to RapidOcrOptions still
    # silently falls back to the default (Chinese) recognizer on Russian
    # scans, which is what this codebase did for months: every scanned book
    # ingested through the default DocumentConverter() got Chinese OCR run
    # against Cyrillic glyphs, producing near-total noise (digits/punctuation/
    # stray Latin letters instead of words) — confirmed by inspecting ingested
    # chunk content for Поваров/Андреев/Богданов/Генкин/Мещеряков/Комлев.
    # EasyOCR does ship a Russian recognition model, so it replaces RapidOCR
    # here. force_full_page_ocr=True additionally means Docling always runs
    # this OCR pass itself rather than trusting any embedded text layer the
    # source PDF might already carry (geokniga.org/DJVU-converted scans can
    # ship their own low-quality, non-Cyrillic-aware embedded text layer,
    # which Docling would otherwise prefer over its own OCR when present).
    global _docling_converter
    if _docling_converter is None:
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import EasyOcrOptions, PdfPipelineOptions
        from docling.document_converter import DocumentConverter, PdfFormatOption
        from docling.models.stages.ocr.easyocr_model import EasyOcrModel

        # EasyOcrModel hardcodes self.scale = 3 (216 DPI) for the page bitmap
        # it feeds to EasyOCR's CRAFT detector — not exposed via
        # EasyOcrOptions at all. With force_full_page_ocr=True every page
        # goes through this at full page size (~1787x2526px for A4), and
        # that resolution is what was hitting the same ~11.6GB ceiling
        # regardless of how many pages were in a batch (even a single fresh
        # 20-page sub-document OOM'd identically) — the memory pressure is
        # per-page-bitmap-size, not cumulative across pages. Halving the
        # linear scale quarters the pixel count the detector has to process.
        _original_easyocr_init = EasyOcrModel.__init__

        def _patched_easyocr_init(self, *args, **kwargs):
            _original_easyocr_init(self, *args, **kwargs)
            self.scale = 1.5  # 108 DPI — still well above what OCR needs for printed text

        EasyOcrModel.__init__ = _patched_easyocr_init

        pipeline_options = PdfPipelineOptions()
        pipeline_options.do_ocr = True
        pipeline_options.ocr_options = EasyOcrOptions(lang=["ru", "en"], force_full_page_ocr=True)
        # Docling's default ocr_batch_size=4 feeds 4 page images through
        # EasyOCR's CRAFT detector in one tensor batch; on a scan with larger/
        # denser pages that single batch can demand ~10GB in one allocation
        # (observed: "HIP out of memory. Tried to allocate 9.72 GiB" on a book
        # that otherwise fits the 12GB card fine page-by-page). One page per
        # OCR call trades some throughput for actually fitting in memory.
        pipeline_options.ocr_batch_size = 1
        _docling_converter = DocumentConverter(
            format_options={InputFormat.PDF: PdfFormatOption(pipeline_options=pipeline_options)}
        )
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


def _release_gpu_memory() -> None:
    """Docling's per-document conversion (layout/table/OCR models running
    over every page) and BGE-M3/reranker batches leave PyTorch's caching
    allocator holding variable-sized blocks that often can't be reused for
    the next call's different tensor shapes — across a long-running
    container ingesting many large books back to back (thousands of pages
    in a single session), this fragmentation accumulates until a request
    that would easily fit in the 12GB card OOMs anyway ("9.81 GiB allocated
    ... tried to allocate 1.12 GiB"). gc.collect() first so any Python-side
    references to page images/tensors from the just-finished call are
    actually dropped before empty_cache() hands the freed blocks back to
    the allocator's free pool.
    """
    import gc

    import torch

    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()


def _ingest_sync(tmp_path: str, filename: str) -> list[Chunk]:
    from docling.chunking import HybridChunker

    # converter.convert() itself — not just the chunking loop below — is
    # where OCR/layout models actually run and where OOM has been observed
    # ("HIP out of memory ... Tried to allocate 9.72 GiB" inside convert()).
    # The whole function used to only wrap the chunking loop in try/finally,
    # so a convert() failure skipped _release_gpu_memory() entirely and left
    # that GPU memory unrecoverable for every subsequent document in the
    # same long-running container — exactly the scenario this fix targets.
    result = None
    doc = None
    try:
        converter = get_docling_converter()
        result = converter.convert(tmp_path)
        doc = result.document

        chunker = HybridChunker()
        chunks: list[Chunk] = []
        _ingest_sync_body(doc, chunker, chunks, filename)
        return chunks
    finally:
        del result, doc
        _release_gpu_memory()


def _ingest_sync_body(doc, chunker, chunks: list[Chunk], filename: str) -> None:
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

    try:
        chunks = await asyncio.to_thread(_ingest_article_sync, tmp_path, file.filename)
    finally:
        # delete=False above is required (Docling/GROBID need a real path to
        # open, not just an fd) — without an explicit cleanup here, every
        # ingested article leaves its upload behind in the container's temp
        # dir forever, eventually filling the disk on a long ingestion run.
        os.unlink(tmp_path)
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

    try:
        chunks = await asyncio.to_thread(_ingest_sync, tmp_path, file.filename)
    finally:
        os.unlink(tmp_path)
    return IngestResponse(chunks=chunks)


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


def _embed_sync(texts: list[str]) -> list[list[float]]:
    # batch_size=12 at the model's full 8192-token max_seq_length repeatedly
    # OOM'd on large books (11.98 GiB GPU shared with Docling/RapidOCR, which
    # alone hold ~8.3 GiB resident): a batch of long chunks can attempt a
    # single ~5 GiB allocation. Retry with a shrinking batch size on OOM
    # instead of failing the whole document — worst case (batch_size=1) is
    # slower but always fits, and empty_cache() between attempts releases
    # whatever fragmented memory the previous attempt's partial allocation left.
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
            # Same accumulating-fragmentation reasoning as _release_gpu_memory
            # (see there) — embed runs on every /runs query, not just
            # ingestion, so this keeps the allocator tidy for the long-running
            # runtime container too, not just the one-off ingestion tool.
            torch.cuda.empty_cache()
    raise AssertionError("unreachable")


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
