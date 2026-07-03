#!/usr/bin/env bash
# Ingests the 14 "deep research" markdown write-ups the team collected
# (LLM-synthesized topic overviews on flotation/gold extraction/refractory
# ore processing, not raw academic PDFs with citations) — plain text, so
# regular Docling ingestion (sourceType=report) is correct; GROBID is for
# real paper PDFs, not applicable here.
set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
SRC_DIR="${SRC_DIR:-/home/god/Загрузки/Telegram Desktop}"

post_doc() {
  local file="$1" title="$2"
  echo "=== [report] $title ==="
  local t0=$(date +%s)
  curl -sS -X POST "$API/v1/documents" \
    -F "file=@${file}" \
    -F "title=${title}" \
    -F "sourceType=report" \
    -F "domain=flotation" \
    -F "language=ru" | python3 -m json.tool
  local t1=$(date +%s)
  echo "--- took $((t1 - t0))s ---"
}

FILES=(
  "1_флотация_и_флотационное_обогащение.md"
  "2_металлургия_золота_серебра_и_благородных_металлов.md"
  "3_альтернативное_выщелачивание_золота_экология_и_вторсырьё.md"
  "4_флотация_обогащение_и_металлургия_золота_и_серебра.md"
  "5_общая_технология_обогащения_полезных_ископаемых.md"
  "6_цианирование_извлечение_золота_и_переработка_упорного_сырья.md"
  "7_флотация_обогащение_и_металлургия_золота_и_серебра.md"
  "8_извлечение_золота_и_металлов_из_электронных_отходов.md"
  "10_переработка_золотосодержащих_руд_и_концентратов.md"
  "1_переработка_упорного_золотосульфидного_сырья.md"
  "2_устойчивое_извлечение_золота_бесцианидные_технологии.md"
  "3_флотация_предконцентрация_и_бесцианидное_извлечение_золота.md"
  "4_бесцианидное_и_цианидное_выщелачивание_упорных_руд.md"
  "7_флотация_упорные_руды_и_бесцианидное_выщелачивание.md"
)

for f in "${FILES[@]}"; do
  title="Ресёрч: ${f%.md}"
  title="$(echo "$title" | tr '_' ' ')"
  post_doc "$SRC_DIR/$f" "$title"
done

echo "=== Done. ==="
