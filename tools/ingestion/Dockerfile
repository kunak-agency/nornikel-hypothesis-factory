FROM rocm/dev-ubuntu-24.04:6.4-complete

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/opt/venv/bin:$PATH"

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3-pip python3-dev python3-venv build-essential libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m venv /opt/venv

WORKDIR /app

# PyTorch с ROCm 6.4 — тот же пин torch==2.8.*, что и в NewMindShorts (проверено
# рабочим на этой же RX 6700 XT). Без пина rocm6.4-индекс может отдать более
# свежий torch, несовместимый с версией sentence-transformers/docling ниже.
RUN pip install --no-cache-dir \
    "torch==2.8.*" "torchvision==0.23.*" \
    --index-url https://download.pytorch.org/whl/rocm6.4

# Constraint guard: docling/sentence-transformers транзитивно требуют torch, и
# без пина pip может притянуть CUDA-сборку torch с PyPI поверх уже
# установленного ROCm-билда (тот же failure mode, что в NewMindShorts).
COPY requirements.txt .
RUN pip freeze | grep -E "^(torch|torchvision)==" > /tmp/torch-constraint.txt \
    && pip install --no-cache-dir -r requirements.txt -c /tmp/torch-constraint.txt

COPY main.py .

EXPOSE 8090
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8090"]
