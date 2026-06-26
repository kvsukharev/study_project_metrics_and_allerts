# Increment 10 — Подключение PostgreSQL и хендлер /ping

## Что сделано

### internal/storage/postgresql.go
Полная переработка — предыдущая реализация не соответствовала интерфейсу `Storage`.

- `NewPostgresStorage(ctx, dsn)` — создаёт пул соединений, проверяет ping, создаёт таблицы `gauges` и `counters` через `CREATE TABLE IF NOT EXISTS`
- `Ping(ctx)` — делегирует `pool.Ping(ctx)`, используется хендлером `/ping`
- `Close() error` — корректно закрывает пул
- `UpdateGauge` / `UpdateCounter` — соответствуют сигнатуре интерфейса (без ctx, без return), ошибки логируются через `log.Printf`, есть retry 1/3/5 сек на сетевые ошибки
- `GetGauge` / `GetCounter` — реализованы (отсутствовали)
- `GetAllMetrics()` — исправлена сигнатура (убран ctx и error из return)
- `BatchUpdate` — исправлена дублирующаяся логика (транзакция выполнялась дважды), retry 1/3/5 сек

### cmd/server/main.go
- `ctx` перенесён выше инициализации хранилища, чтобы передавать в `NewPostgresStorage`
- Если `cfg.DatabaseDSN != ""` — создаётся `PostgresStorage`, файловая персистентность пропускается
- Без DSN — прежнее поведение (memory + file)
- Переменная `mem` вынесена на уровень функции; периодическое сохранение и финальный снапшот защищены проверкой `mem != nil`

## Уже было готово (не менялось)
- Флаг `-d` и переменная окружения `DATABASE_DSN` в `internal/config/config.go`
- Хендлер `GET /ping` в `internal/handler/handlers.go`
- Метод `Ping(ctx) error` в интерфейсе `Storage`
- Схема таблиц в `migrations/001_init.up.sql`
- Зависимость `github.com/jackc/pgx/v5` в `go.mod`

## Поведение

- `DATABASE_DSN=postgres://... ./server` — сервер использует PostgreSQL
- `./server -d postgres://...` — то же через флаг
- `GET /ping` — возвращает 200 если БД доступна, 500 если нет
- Без DSN — поведение не изменилось
