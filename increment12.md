# Increment 12 — Batch API: POST /updates и агент на батчах

## Что сделано

### internal/storage/storage.go
- `MetricsStorage.BatchUpdate` переписан: теперь берёт `mu.Lock()` один раз на весь батч — обновление атомарное, без race condition между метриками.

### internal/agent/collector.go
- Добавлен метод `GetAllMetrics() []model.Metrics` — возвращает все gauge и counter метрики за одну блокировку mutex. Устраняет race condition, который был при раздельных вызовах `GetGauges()` + `GetCounters()` (между ними коллектор мог обновить данные).

### internal/agent/sender.go
- Добавлен метод `SendBatch(metrics []model.Metrics) error`:
  - Не отправляет пустые батчи
  - Сериализует `[]model.Metrics` в JSON
  - Сжимает gzip
  - POST на `/updates`
- `SendAllMetrics` сохранён для обратной совместимости

### cmd/agent/main.go
- Reporting goroutine переключена с `sender.SendAllMetrics(gauges, counters)` на `collector.GetAllMetrics()` + `sender.SendBatch(metrics)` — все метрики отправляются одним запросом.

## Обратная совместимость
- Сервер по-прежнему поддерживает `POST /update` (одиночная метрика) и все остальные старые эндпоинты.
- `StripSlashes` middleware обрабатывает `/updates/` → `/updates`.
- `SendAllMetrics` в sender не удалён.
