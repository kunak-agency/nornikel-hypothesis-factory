FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
# CPU-only: this service only embeds one query + reranks top-K candidates per
# request — no GPU needed, and it must build/run reliably on any judge's
# machine without ROCm/CUDA setup. Heavy ingestion runs separately (tools/ingestion).
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir -r requirements.txt

# Bake the models into the image at build time so runtime never downloads from
# HuggingFace. Without this, BGE-M3 + bge-reranker-v2-m3 (~4.5 GB total) are
# fetched lazily on the first /embed | /rerank — which on Cloud Run's ephemeral
# FS re-downloads on every cold start (the classic "прогрев" / slow first run).
# Here the download happens once, locally, during `docker build`; the weights
# ship inside the image layer.
ENV HF_HOME=/opt/hf-cache
RUN python -c "from sentence_transformers import SentenceTransformer, CrossEncoder; \
    SentenceTransformer('BAAI/bge-m3'); \
    CrossEncoder('BAAI/bge-reranker-v2-m3')"

# Models are baked above — forbid any network call to HuggingFace at runtime so
# startup is deterministic and fails fast if the cache is ever missing, rather
# than silently hanging on a download.
ENV HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1

COPY main.py .

EXPOSE 8090
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8090"]
