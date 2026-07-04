# Дамп базы знаний

`hypothesis_factory.dump.sql.gz` (~230MB: 51 документ, ~46.5k чанков с
эмбеддингами) не хранится в git — он поставляется вместе с материалами
сдачи. Положите файл в этот каталог перед первым `docker compose up`:
postgres выполнит его автоматически при инициализации пустого volume.

Без дампа стек поднимается с пустой базой — корпус можно собрать заново
через `POST /v1/documents` (см. scripts/seed_corpus.sh) с ingestion-профилем
`docker-compose.ingestion.yml`.
