# Increment 19 — godoc documentation & example_test.go

## Что сделано

### godoc-комментарии

Добавлены doc-комментарии ко всем экспортируемым типам, интерфейсам, функциям и методам:

| Файл | Что задокументировано |
|---|---|
| `internal/model/metrics.go` | package doc, `MetricType`, `TypeCounter`, `TypeGauge`, `Metrics` |
| `internal/storage/storage.go` | package doc, `ErrMetricNotFound`, `ErrInvalidType`, `Storage` (все методы интерфейса), `MetricsStorage`, `NewMemStorage` |
| `internal/handler/handlers.go` | package doc, `Handlers`, `NewHandlers`, `RegisterRoutes`, `PingHandler`, `BatchUpdateMetrics`, `NewSHA256CheckMiddleware` |
| `internal/agent/collector.go` | package doc, `Collector`, `NewCollector`, `UpdateMetrics`, `GetAllMetrics`, `GetGauges`, `GetCounters`, `GetMetricsCount` |
| `internal/agent/sender.go` | `Sender`, `NewSender`, `SendGauge`, `SendCounter`, `SendAllMetrics`, `SendBatch`, `SendMetric` |
| `internal/agent/util.go` | `Compress`, `ComputeHMAC` |
| `internal/audit/audit.go` | `FileObserver.Notify`, `URLObserver.Notify` (остальное уже было задокументировано) |
| `internal/config/config.go` | package doc, `ServerConfig`, `ParseFlags` |

### Примеры (internal/handler/example_test.go)

Покрыты все основные эндпоинты сервера:

| Функция примера | Эндпоинт |
|---|---|
| `ExampleHandlers_RegisterRoutes_updateGaugePlainText` | `POST /update/{type}/{name}/{value}` — обновление gauge |
| `ExampleHandlers_RegisterRoutes_updateCounterPlainText` | `POST /update/{type}/{name}/{value}` — обновление counter |
| `ExampleHandlers_RegisterRoutes_updateJSON` | `POST /update` — JSON-обновление одной метрики |
| `ExampleHandlers_RegisterRoutes_batchUpdate` | `POST /updates` — батчевое обновление |
| `ExampleHandlers_RegisterRoutes_getValuePlainText` | `GET /value/{type}/{name}` — чтение значения |
| `ExampleHandlers_RegisterRoutes_getValueJSON` | `POST /value` — чтение через JSON |
| `ExampleHandlers_RegisterRoutes_ping` | `GET /ping` — liveness probe |
| `ExampleHandlers_RegisterRoutes_dashboard` | `GET /` — HTML dashboard |

Все примеры содержат `// Output:` директиву и проходят `go test`.
