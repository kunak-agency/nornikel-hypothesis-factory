#!/usr/bin/env bash
# Книги по оборудованию/классификации/измельчению, найденные ресёрч-агентом
# для закрытия систематического пробела в корпусе (система выдавала 0%
# гипотез про оборудование против 100% у реальных экспертов).
set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
DIR="${DIR:-/home/god/Документы/nornikel/Задача 1/Оборудование}"

post_doc() {
  local file="$1" title="$2"
  echo "=== [book] $title ==="
  local t0=$(date +%s)
  curl -sS -X POST "$API/v1/documents" \
    -F "file=@${file}" \
    -F "title=${title}" \
    -F "sourceType=book" \
    -F "domain=flotation" \
    -F "language=ru" | python3 -m json.tool
  local t1=$(date +%s)
  echo "--- took $((t1 - t0))s ---"
}

post_doc "$DIR/andreev_drobleniye.pdf" "Андреев С.Е., Перов В.А., Зверевич В.В. — Дробление, измельчение и грохочение полезных ископаемых, 3-е изд., 1980"
post_doc "$DIR/povarov_gidrociklony.pdf" "Поваров А.И. — Гидроциклоны на обогатительных фабриках, 1978"
post_doc "$DIR/magnitnye_metody.pdf" "Магнитные и электрические методы обогащения руд (учебник для вузов)"
post_doc "$DIR/bogdanov_tom2.pdf" "Богданов О.С. (ред.) — Справочник по обогащению руд, том 2 (Основные процессы), 1982"

echo "=== Done. ==="
