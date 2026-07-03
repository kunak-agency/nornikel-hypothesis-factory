#!/usr/bin/env bash
# One-shot full corpus re-ingest through the GPU pipeline (tools/ingestion,
# ROCm) after the parent-child / table-markdown / letter-spacing fixes.
# Reconstructed from the document list that was live in the DB before the
# clean wipe — same titles/sourceType/domain, so the KB ends up equivalent
# but re-chunked with the fixed pipeline. Run AFTER truncating the DB and
# AFTER docker-compose.ingestion.yml pyworker is up.
set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
CASE_DIR="${CASE_DIR:-/home/god/Документы/nornikel/Задача 1}"
VISION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../tools/ingestion/vision_extract" && pwd)"
TELEGRAM_DIR="/home/god/Загрузки/Telegram Desktop"

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

post_doc "$CASE_DIR/Как читать отчет института по хвостам.docx" "Как читать отчет по хвостам" "regulation"

post_doc "$CASE_DIR/Дополнительные материалы/geokniga-flotacionnye-metody-obogashcheniya_0.pdf" "Флотационные методы обогащения" "book"
post_doc "$CASE_DIR/Дополнительные материалы/geokniga-tehnologiyaobogashcheniyapoleznyhiskopaemyh.pdf" "Технология обогащения полезных ископаемых" "book"

post_doc "$CASE_DIR/Пример 2/Гипотезы НОФ вкр.docx" "Пример 2 — Гипотезы НОФ вкр.docx" "historical_case"
post_doc "$CASE_DIR/Пример 3/Гипотезы НОФ мед.docx" "Пример 3 — Гипотезы НОФ мед.docx" "historical_case"
post_doc "$CASE_DIR/Пример 4/Гипотезы ТОФ.docx" "Пример 4 — Гипотезы ТОФ.docx" "historical_case"

post_doc "$VISION_DIR/equipment_list.md" "Типичный список оборудования обогатительной фабрики" "scheme"
post_doc "$VISION_DIR/scheme_concept_gravity_flash_flotation.md" "Методика: гравитация + скоростная флотация (концепт)" "scheme"
post_doc "$VISION_DIR/scheme_concept_inert_grinding.md" "Методика: измельчение в инертной среде (концепт)" "scheme"
post_doc "$VISION_DIR/scheme_concept_polymetallic_reagent_regime.md" "Методика: реагентный режим полиметаллической руды (концепт)" "scheme"
post_doc "$VISION_DIR/scheme_concept_standard_stages.md" "Методика: типовая схема основная/перечистная/контрольная флотация (концепт)" "scheme"
post_doc "$VISION_DIR/scheme_crushing_circuit_1.md" "Регламент 1 — дробильно-сортировочный комплекс (карьер/шахта)" "scheme"
post_doc "$VISION_DIR/scheme_crushing_circuit_2.md" "scheme_crushing_circuit_2.md" "scheme"
post_doc "$VISION_DIR/scheme_cu_flotation_cell_bank.md" "scheme_cu_flotation_cell_bank.md" "scheme"
post_doc "$VISION_DIR/scheme_cu_ni_flotation_material_balance.md" "scheme_cu_ni_flotation_material_balance.md" "scheme"
post_doc "$VISION_DIR/scheme_full_grinding_flotation_chain.md" "scheme_full_grinding_flotation_chain.md" "scheme"
post_doc "$VISION_DIR/scheme_grinding_classification_layout.md" "scheme_grinding_classification_layout.md" "scheme"
post_doc "$VISION_DIR/scheme_malomyr_gold_plant_equipment_spec.md" "scheme_malomyr_gold_plant_equipment_spec.md" "scheme"
post_doc "$VISION_DIR/scheme_reagent_regime_collective_flotation.md" "scheme_reagent_regime_collective_flotation.md" "scheme"

post_doc "$CASE_DIR/Найденные переиздания/statya_ikkijelon.pdf" "Рахманов и др. — Технология извлечения золота и серебра из упорного мышьяковистого флотоконцентрата Иккижелон" "report"
post_doc "$CASE_DIR/Найденные переиздания/avdokhin_tom1.pdf" "Авдохин В.М. — Основы обогащения полезных ископаемых, том 1 (Обогатительные процессы)" "book"
post_doc "$TELEGRAM_DIR/geokniga-flotacionnyemetodyobogashcheniya.pdf" "Абрамов А.А. — Флотационные методы обогащения, 4-е изд., 2016" "book"
post_doc "$CASE_DIR/Найденные переиздания/avdokhin_tom2.pdf" "Авдохин В.М. — Основы обогащения полезных ископаемых, том 2 (Обогатительные технологии)" "book"

echo "=== Done. ==="
