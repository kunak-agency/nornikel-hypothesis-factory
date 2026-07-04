"""
pyworker: локальный сервис ingestion + эмбеддингов для Go-сервиса Hypothesis Factory.

- POST /ingest         : парсит PDF/DOCX/XLSX/PPTX/изображения через Docling,
                          возвращает семантические чанки (по секциям: заголовки,
                          параграфы, таблицы), а не окна фиксированного размера.
- POST /ingest-article : парсит PDF научной статьи через GROBID (заголовок/
                          авторы/аннотация/секции), best-effort обогащение
                          метаданными Semantic Scholar (цитирования, venue, год).
- POST /embed           : плотные эмбеддинги BGE-M3 (мультиязычные, до 8192 токенов).
- POST /rerank          : скоринг bge-reranker-v2-m3 (запрос против кандидатов).

Работает полностью офлайн, если модели уже в локальном кэше — кроме шага
обогащения Semantic Scholar в /ingest-article (best-effort сетевой вызов,
пропускается без падения, если недоступен).
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
    # У встроенного в Docling RapidOCR нет модели для кириллицы (только
    # english/latin/chinese) — на русских сканах он тихо откатывается на
    # китайскую. EasyOCR даёт настоящую русскую модель. force_full_page_ocr=True,
    # чтобы Docling не доверял низкокачественному встроенному текстовому слою
    # некоторых сканов.
    global _docling_converter
    if _docling_converter is None:
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import EasyOcrOptions, PdfPipelineOptions
        from docling.document_converter import DocumentConverter, PdfFormatOption
        from docling.models.stages.ocr.easyocr_model import EasyOcrModel

        # EasyOcrModel жёстко фиксирует self.scale = 3 (216 DPI) для растра
        # OCR — не настраивается через EasyOcrOptions. Одного этого разрешения
        # хватало, чтобы упереться в GPU OOM на каждой странице независимо от
        # размера документа. Уменьшение вдвое даёт вчетверо меньше пикселей.
        _original_easyocr_init = EasyOcrModel.__init__

        def _patched_easyocr_init(self, *args, **kwargs):
            _original_easyocr_init(self, *args, **kwargs)
            self.scale = 1.5  # 108 DPI

        EasyOcrModel.__init__ = _patched_easyocr_init

        pipeline_options = PdfPipelineOptions()
        pipeline_options.do_ocr = True
        pipeline_options.ocr_options = EasyOcrOptions(lang=["ru", "en"], force_full_page_ocr=True)
        # ocr_batch_size=4 по умолчанию батчит 4 растра страниц в один тензор,
        # что на плотных сканах может потребовать ~10GB одним выделением.
        pipeline_options.ocr_batch_size = 1
        _docling_converter = DocumentConverter(
            format_options={InputFormat.PDF: PdfFormatOption(pipeline_options=pipeline_options)}
        )
    return _docling_converter


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
        # (из сохранённого ST-конфига), обрезая далеко ниже реального
        # обученного контекста модели в 8192 токена.
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
    """TableFormer принимает однокоронную Word-таблицу (обычный маркированный
    список в рамке таблицы) за таблицу с заголовком в первой строке, и
    сериализатор HybridChunker повторяет "заголовок = строка" для каждой
    последующей строки ("2. X = 3. Y. 2. X = 4. Z.") — непригодно для claim
    extraction. Распознаёт повторяющийся префикс и восстанавливает исходный
    список."""
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
    # Доверяем реконструкции только если восстановилось несколько разных
    # элементов — иначе это не тот паттерн, оставляем текст как есть.
    if len(items) < 2 or len(set(items)) < len(items) - 1:
        return text
    return header + "\n" + "\n".join(items)


_LETTER_SPACING_RE = re.compile(r"(?:\b\w\b[ \-]+){3,}\b\w\b", re.UNICODE)


def fix_letter_spacing(text: str) -> str:
    """Docling/OCR иногда извлекает текст из широко разряженных заголовков/
    таблиц как последовательность однобуквенных "слов" ("Э м и с с и о н н
    о-р а д и о м е т р и ч е с к и е" вместо "Эмиссионно-радиометрические").
    Схлопывает пробегы из 4+ однобуквенных токенов обратно в слово; короткие
    настоящие слова ("и", "о", "в") не трогает — они не встречаются в
    пробегах длиной 4+."""

    def collapse(m: re.Match) -> str:
        return re.sub(r"[ \-]+", "", m.group(0))

    return _LETTER_SPACING_RE.sub(collapse, text)


def _table_markdown(doc, chunk) -> str | None:
    """Сериализатор таблиц по умолчанию в HybridChunker даёт тройки
    "строка = столбец = значение", теряющие структуру строк/столбцов, нужную
    для grounding. TableItem.export_to_markdown сохраняет сетку таблицы.
    Возвращает None, если в чанке нет табличных doc_items (тогда вызывающая
    сторона откатывается на сериализацию HybridChunker)."""
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
    """Сбрасывает закэшированные блоки аллокатора PyTorch после каждого
    документа/батча, чтобы фрагментация не копилась в долгоживущем
    контейнере. Сначала gc.collect(), чтобы Python-ссылки на тензоры реально
    освободились до того, как empty_cache() их заберёт."""
    import gc

    import torch

    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()


def _ingest_sync(tmp_path: str, filename: str) -> list[Chunk]:
    from docling.chunking import HybridChunker

    # Сам convert() (не только цикл чанкинга) должен быть внутри try/finally —
    # именно там работают OCR/layout-модели и происходит OOM.
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
    """Склеивает весь текст под элементом, схлопывая пробелы — TEI оборачивает
    инлайн-элементы (ссылки, формулы) внутри <p>, для claim extraction
    достаточно голого itertext()."""
    return re.sub(r"\s+", " ", "".join(el.itertext())).strip()


_CHUNK_CHAR_BUDGET = 2000
_HARD_PARA_CAP = 4000


def _group_paragraphs(paragraphs: list[str]) -> list[str]:
    """Группирует последовательные параграфы до ~_CHUNK_CHAR_BUDGET символов —
    неограниченный чанк на весь <div> (одна секция GROBID может быть 10k+
    символов в длинной статье) раздувает память attention при эмбеддинге
    (квадратично по длине последовательности) даже в пределах номинального
    окна BGE-M3 в 8192 токена. Никогда не режет параграф посередине
    предложения, кроме редкого случая, когда один параграф сам превышает
    _HARD_PARA_CAP — тогда режется по пробелам как крайняя мера."""
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
    """Превращает TEI XML от GROBID в тот же формат Chunk, что и /ingest, —
    downstream (claim extraction на Go, parent-child контекст соседей) не
    нуждается в отдельной обработке статей. Один чанк на секцию <div>
    (GROBID уже сегментирует по заголовкам) плюс чанк аннотации. Возвращает
    (chunks, header_metadata) — header_metadata (title/authors/year)
    прикрепляется к каждому чанку, чтобы claim extraction могла процитировать
    статью по имени даже из чанка основного текста."""
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
    """Best-effort сигнал авторитетности (цитирования, venue, год) по поиску
    названия статьи — не обязателен для пайплайна, любая неудача (сеть,
    rate limit, нет совпадения) молча даёт пустое обогащение, не роняя
    ingestion."""
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
        # Best-effort: логируем, но не роняем ingestion из-за нестабильного/
        # rate-limited внешнего API.
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
    # asyncio.to_thread — вызов GROBID и парсинг XML в отдельном потоке,
    # чтобы /healthz оставался отзывчивым.
    suffix = Path(file.filename or "upload").suffix or ".pdf"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(await file.read())
        tmp_path = tmp.name

    try:
        chunks = await asyncio.to_thread(_ingest_article_sync, tmp_path, file.filename)
    finally:
        # delete=False выше обязателен (Docling/GROBID нужен реальный путь,
        # не просто fd) — без явной очистки здесь временный файл остаётся
        # в контейнере навсегда.
        os.unlink(tmp_path)
    return IngestResponse(chunks=chunks)


@app.post("/ingest", response_model=IngestResponse)
async def ingest(file: UploadFile = File(...)):
    # Конвертация Docling — синхронная CPU/GPU-работа, может идти десятки
    # минут на большой книге; asyncio.to_thread выносит её в отдельный поток,
    # чтобы не блокировать единственный event loop FastAPI.
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
    # Ретрай с уменьшающимся batch_size при GPU OOM вместо падения всего
    # запроса — крупный батч длинных чанков на полном контексте BGE-M3
    # (8192 токена) может потребовать однократное выделение в несколько GB.
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
