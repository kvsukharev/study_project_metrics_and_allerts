# Increment 16 — Audit via Observer pattern

## Что сделано

### Новый пакет `internal/audit/audit.go`

Реализован паттерн «Наблюдатель» для аудита входящих запросов.

| Тип | Роль |
|---|---|
| `Event` | Структура события аудита: `ts` (unix timestamp), `metrics` (имена метрик), `ip_address` |
| `Observer` | Интерфейс с методом `Notify(Event)` |
| `Subject` | Держит срез наблюдателей, транслирует событие всем: `Notify(Event)` |
| `FileObserver` | Наблюдатель-файл: дописывает JSON-строку в конец файла (`os.O_APPEND`) |
| `URLObserver` | Наблюдатель-URL: отправляет POST с JSON-телом; таймаут 5 секунд |

Ошибки (открытие файла, запись, HTTP) логируются через стандартный `log.Printf`, не паникуют и не прерывают обработку запроса.

### Конфигурация (`internal/config/config.go`)

Добавлены два поля в `ServerConfig`:

```
AuditFile  string  `env:"AUDIT_FILE"`   // флаг --audit-file
AuditURL   string  `env:"AUDIT_URL"`    // флаг --audit-url
```

Флаги зарегистрированы в `ParseFlags()`. Пустое значение = аудит отключён.

### Handlers (`internal/handler/handlers.go`)

- `Handlers` получил поле `audit *audit.Subject` (nil — аудит отключён).
- Сигнатура `NewHandlers` расширена третьим аргументом `*audit.Subject`.
- Добавлен вспомогательный метод `notifyAudit(r, metricNames)` — вызывается только при успехе.
- `updateMetric` теперь возвращает `bool` — позволяет `updateHandlerChi` знать об успехе и отправить событие.
- Вызовы аудита добавлены в три точки:
  - `updateMetricJSONHandler` (POST /update)
  - `BatchUpdateMetrics` (POST /updates)
  - `updateHandlerChi` (POST /update/{type}/{name}/{value})
- `extractIP(r)` — извлекает IP клиента с поддержкой `X-Real-IP`, `X-Forwarded-For` и `net.SplitHostPort(RemoteAddr)`.

### Сборка и wire-up (`cmd/server/main.go`)

После разбора конфига формируется срез `[]audit.Observer`:
- если `cfg.AuditFile != ""` → добавляется `FileObserver`
- если `cfg.AuditURL != ""`  → добавляется `URLObserver`

`Subject` создаётся только если хотя бы один observer активен (иначе `nil`), что исключает лишние аллокации при отключённом аудите.

### Тесты

Скорректирован `handlers_test.go`: `newRouter()` передаёт `nil` в качестве `auditSubject` — аудит отключён в тестах, поведение handlers не изменилось.

Все тесты: `go test ./...` — OK.

## Формат события

```json
{
  "ts": 1724505600,
  "metrics": ["Alloc", "Frees"],
  "ip_address": "192.168.0.42"
}
```

## Использование

```bash
# Аудит только в файл
./server --audit-file=/var/log/metrics-audit.log

# Аудит только на удалённый сервер
./server --audit-url=http://audit.internal/events

# Оба приёмника одновременно
./server --audit-file=audit.log --audit-url=http://audit.internal/events

# Через переменные окружения
AUDIT_FILE=/var/log/audit.log AUDIT_URL=http://audit.internal/events ./server
```
