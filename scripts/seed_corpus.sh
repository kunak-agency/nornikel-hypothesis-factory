#!/usr/bin/env bash
# Ingests the hackathon-provided knowledge base into the running API:
# textbooks (books), equipment/regulation sheets (regulation), flotation
# schemes (scheme), and the four worked examples as historical_case pairs
# (tailings profile + brainstormed hypotheses) — these are the strongest
# few-shot evidence for claim extraction since they show real analyst output.
set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
CASE_DIR="${CASE_DIR:-/home/god/Документы/nornikel/Задача 1}"

post_doc() {
  local file="$1" title="$2" source_type="$3" domain="${4:-flotation}"
  echo "Ingesting [$source_type] $title ..."
  curl -sS -X POST "$API/v1/documents" \
    -F "file=@${file}" \
    -F "title=${title}" \
    -F "sourceType=${source_type}" \
    -F "domain=${domain}" \
    -F "language=ru" | python3 -m json.tool
}

# --- Textbooks ---
shopt -s nullglob
for f in "$CASE_DIR/Дополнительные материалы/"*.pdf; do
  post_doc "$f" "$(basename "$f" .pdf)" "book"
done

# --- Regulations / equipment lists ---
post_doc "$CASE_DIR/Как читать отчет института по хвостам.docx" "Как читать отчет по хвостам" "regulation"
for f in "$CASE_DIR/Регламенты/"*.png; do
  post_doc "$f" "$(basename "$f")" "regulation"
done

# --- Flotation schemes ---
for f in "$CASE_DIR/Схемы флотации/"*; do
  post_doc "$f" "$(basename "$f")" "scheme"
done

# --- Historical cases: tailings profile + brainstormed hypotheses ---
for dir in "$CASE_DIR"/Пример*/; do
  name=$(basename "$dir")
  for f in "$dir"*; do
    post_doc "$f" "$name — $(basename "$f")" "historical_case"
  done
done

echo "Done."
