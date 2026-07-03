#!/usr/bin/env bash
# Additional books found in "Дополнительные материалы" that were downloaded
# but never actually ingested (gap discovered while rebuilding the corpus
# list for the clean GPU re-ingest). Same API, same domain.
set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
CASE_DIR="${CASE_DIR:-/home/god/Документы/nornikel/Задача 1}"

post_doc() {
  local file="$1" title="$2" source_type="$3"
  echo "=== [$source_type] $title ==="
  local t0=$(date +%s)
  curl -sS -X POST "$API/v1/documents" \
    -F "file=@${file}" \
    -F "title=${title}" \
    -F "sourceType=${source_type}" \
    -F "domain=flotation" \
    -F "language=ru" | python3 -m json.tool
  local t1=$(date +%s)
  echo "--- took $((t1 - t0))s ---"
}

post_doc "$CASE_DIR/Дополнительные материалы/geokniga_lodeyshchikovvvtehnologiyaizvlecheniyazolotaiserebraizupornyh1.pdf" \
  "Лодейщиков В.В. — Технология извлечения золота и серебра из упорных руд" "book"
post_doc "$CASE_DIR/Дополнительные материалы/geokniga-metallurgiya-blagorodnyh-metallov_0.pdf" \
  "Металлургия благородных металлов" "book"
post_doc "$CASE_DIR/Дополнительные материалы/tehnologiya_izvlecheniya_zolota_i_serebra_iz_upornogo_zolotosoderzhaschego.pdf" \
  "Технология извлечения золота и серебра из упорного золотосодержащего сырья" "book"

echo "=== Extra corpus done. ==="
