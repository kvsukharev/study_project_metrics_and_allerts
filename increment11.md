# Increment 11 — Миграции PostgreSQL через goose

## Что сделано

### internal/storage/migrations/001_init.sql
Новый файл миграции в формате `pressly/goose/v3`:
- `-- +goose Up` — создаёт таблицы `gauges` (`DOUBLE PRECISION`) и `counters` (`BIGINT`)
- `-- +goose Down` — удаляет таблицы

### internal/storage/postgresql.go
- Добавлен `//go:embed migrations` — SQL файлы встроены в бинарник
- Убран `createTables()`, добавлен `runMigrations(pool *pgxpool.Pool)`:
  - `stdlib.OpenDBFromPool(pool)` — получает `*sql.DB` из pgx-пула без нового соединения
  - `goose.SetBaseFS(migrationsFS)` — использует встроенные файлы
  - `goose.Up(db, "migrations")` — накатывает все непримененные миграции
  - `goose.NopLogger()` — подавляет вывод goose
- `NewPostgresStorage` — порядок инициализации: ping → миграции → возврат хранилища

### go.mod
Добавлена зависимость `github.com/pressly/goose/v3 v3.27.1`

## Поведение fallback (приоритет хранилища)
1. `DATABASE_DSN` задан → PostgreSQL (миграции применяются автоматически)
2. `DATABASE_DSN` пуст, `FILE_STORAGE_PATH` задан → файловое хранилище
3. Оба пусты → хранение в памяти
