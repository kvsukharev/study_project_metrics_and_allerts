# go-musthave-metrics-tpl

## Increment 17 — Benchmarks & Memory Profiling

### Бенчмарки

Бенчмарки добавлены для трёх ключевых пакетов:

| Файл | Что измеряет |
|---|---|
| `internal/storage/storage_bench_test.go` | UpdateGauge, UpdateCounter, BatchUpdate, GetAllMetrics, GetGauge |
| `internal/agent/agent_bench_test.go` | CollectorUpdateMetrics, GetAllMetrics, GetGauges, GetCounters, Compress, ComputeHMAC |
| `internal/handler/handlers_bench_test.go` | BatchUpdate endpoint, UpdateJSON endpoint, GET /, UpdatePlainText endpoint |

Запуск бенчмарков:
```bash
go test -bench=. -benchmem ./internal/storage/ ./internal/agent/ ./internal/handler/
```

### Анализ профиля памяти

Профиль снят во время выполнения бенчмарков агента (`Compress` + `Collector`):

```bash
go test -run='^$' -bench='BenchmarkCompress|BenchmarkCollector' -benchtime=5s \
  -memprofile=profiles/base.pprof ./internal/agent/
```

Топ аллокаторов (`go tool pprof -top profiles/base.pprof`):

```
flat  flat%   sum%        cum   cum%
36414MB 57.42%         compress/flate.NewWriter   ← главная проблема
 8440MB 13.31%         agent.(*Collector).GetAllMetrics
 7575MB 11.95%         compress/flate.(*compressor).initDeflate
 5519MB  8.70%         agent.(*Collector).GetCounters  ← нет capacity hint
 4819MB  7.60%         agent.(*Collector).GetGauges    ← нет capacity hint
```

**Вывод:** `Compress()` создавал `gzip.Writer` (~32 KB внутренних буферов) заново при каждом вызове, что давало **~814 KB аллокаций за один вызов**. `GetGauges`/`GetCounters` не передавали capacity в `make()`, что вызывало rehash при росте map.

### Оптимизации

1. **`internal/agent/util.go` — `sync.Pool` для `gzip.Writer` и `bytes.Buffer`**
   - `gzip.Writer` переиспользуется через `gzipWriterPool` вместо создания нового на каждый вызов
   - `bytes.Buffer` переиспользуется через `compressBufPool`

2. **`internal/agent/collector.go` — capacity hint в `GetGauges()` и `GetCounters()`**
   - `make(map[string]float64, len(c.gauge))` — исключает rehash при копировании

### Результат

```bash
go tool pprof -top -diff_base=profiles/base.pprof profiles/result.pprof
```

```
      flat  flat%   sum%        cum   cum%
  -35.56GB 57.41%         compress/flate.NewWriter
   -7.40GB 11.94%         compress/flate.(*compressor).initDeflate
   -0.85GB  1.37%         agent.(*Collector).GetCounters
   -0.82GB  1.33%         agent.(*Collector).GetGauges
   -0.39GB  0.63%         compress/flate.(*huffmanEncoder).generate
```

Все значения отрицательные — использование памяти снизилось.

| Бенчмарк | До | После | Улучшение |
|---|---|---|---|
| `BenchmarkCompress` | 814 750 B/op, 21 allocs | 432 B/op, 1 alloc | **-99.9% памяти** |
| `BenchmarkCompressSingle` | 814 111 B/op, 20 allocs | 80 B/op, 1 alloc | **-99.9% памяти** |
| `BenchmarkCollectorGetGauges` | 1640 B/op, 7 allocs | 984 B/op, 4 allocs | **-40% памяти** |
| `BenchmarkCollectorGetCounters` | 256 B/op, 2 allocs | 256 B/op, 2 allocs | без изменений |

---



Шаблон репозитория для трека «Сервер сбора метрик и алертинга».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

# Запуск с параметрами по умолчанию
go run cmd/server/main.go

# Запуск на другом порту
go run cmd/server/main.go -a=localhost:9090

# Запуск на всех интерфейсах
go run cmd/server/main.go -a=:8080

# Запуск на конкретном IP
go run cmd/server/main.go -a=192.168.1.100:8080

# Помощь по флагам
go run cmd/server/main.go -h

# Запуск с параметрами по умолчанию (сервер localhost:8080, отправка каждые 10 сек, сбор каждые 2 сек)
go run cmd/agent/main.go

# Запуск с кастомным сервером
go run cmd/agent/main.go -a=localhost:9090

# Запуск с более частой отправкой (каждые 5 секунд)
go run cmd/agent/main.go -r=5

# Запуск с более частым сбором метрик (каждую секунду)
go run cmd/agent/main.go -p=1

# Запуск со всеми кастомными параметрами
go run cmd/agent/main.go -a=192.168.1.100:8080 -r=30 -p=5

# Быстрый режим для тестирования (сбор каждую секунду, отправка каждые 3 секунды)
go run cmd/agent/main.go -p=1 -r=3

# Медленный режим (сбор каждые 10 секунд, отправка каждую минуту)
go run cmd/agent/main.go -p=10 -r=60

# Помощь по флагам
go run cmd/agent/main.go -h


# 7. Для отладки - тестовый запрос через curl
curl -X POST -H "Content-Type: text/plain" \
  "http://localhost:8080/update/gauge/TestMetric/123.456"


tree ./go-musthave-metrics-trainer
./go-musthave-metrics-trainer
├── README.md
├── api
│   └── api.proto
├── cmd
│   ├── agent
│   │   └── main.go
│   └── server
│       └── main.go
├── internal
│   ├── agent
│   │   └── sender.go
│   ├── config
│   │   ├── config.go
│   │   └── db
│   │       └── db.go
│   ├── handler
│   │   └── router.go
│   ├── model
│   │   └── metrics.go
│   ├── repository
│   │   └── postgres.go
│   └── service
│       └── put.go
├── migrations
│   └── 001-init.sql
└── pkg
    └── client.go


Команда tree ./go-musthave-metrics-trainer используется для визуализации структуры каталогов и файлов в директории ./go-musthave-metrics-trainer. Утилита tree рекурсивно отображает содержимое директории в древовидном формате, что позволяет легко увидеть иерархию файлов и папок.

В приведённом примере структура проекта выглядит следующим образом:
•	Корневая директория: ./go-musthave-metrics-trainer.
•	В корневой директории находится файл README.md, который, скорее всего, содержит описание проекта, инструкции по его использованию или другую важную информацию.
•	Директория api: содержит файл api.proto, который, вероятно, описывает API проекта с использованием Protocol Buffers (protobuf) — формата для сериализации структурированных данных.
•	Директория cmd: содержит поддиректории с кодовой базой для различных компонентов приложения:
o	agent — поддиректория с файлом main.go, который, вероятно, является точкой входа для компонента «агент».
o	server — поддиректория с файлом main.go, который, вероятно, является точкой входа для серверной части приложения.
•	Директория internal: содержит внутренние компоненты приложения:
o	agent — поддиректория с файлом sender.go, который, возможно, отвечает за отправку данных.
o	config — поддиректория с файлами для работы с конфигурацией приложения (config.go) и базой данных (db.go).
o	handler — поддиректория с файлом router.go, который, вероятно, отвечает за маршрутизацию запросов.
o	model — поддиректория с файлом metrics.go, который, возможно, содержит модели данных, связанные с метриками.
o	repository — поддиректория с файлом postgres.go, который, вероятно, реализует взаимодействие с базой данных PostgreSQL.
o	service — поддиректория с файлом put.go, который, возможно, содержит логику для операций записи данных.
•	Директория migrations: содержит SQL-скрипты для миграции базы данных. В данном случае присутствует файл 001-init.sql, который, вероятно, используется для инициализации базы данных.
•	Директория pkg: содержит общие утилиты или библиотеки, которые могут использоваться в проекте. В данном случае присутствует файл client.go, который, вероятно, реализует клиентскую часть для взаимодействия с каким-либо сервисом или API.
Такая структура проекта является типичной для многих Go-приложений и способствует организации кода, упрощая его понимание, поддержку и масштабирование. Например:
•	разделение на cmd, internal и pkg помогает чётко разграничить точки входа в приложение, внутреннюю логику и повторно используемые компоненты;
•	наличие директории api с файлами protobuf позволяет легко описывать и генерировать код для взаимодействия с API;
•	директория migrations упрощает управление изменениями схемы базы данных.
В целом, подобная организация проекта делает код более модульным, читаемым и удобным для командной разработки.


api/
cmd/agent/main.go
cmd/server/main.go
cmd/server/main_test.go
internal/agent/agent.go
internal/agent/collector.go
internal/agent/collector_test.go
internal/agent/sender.go
internal/agent/sender_test.go
internal/agent/util.go
internal/config/config.go
internal/handler/handlers.go
internal/logger/logger.go
internal/logger/logging.go
internal/middleware_proj/gzip.go
internal/model/metrics.go
internal/repository
internal/server/server.go
internal/service
internal/storage/postgresql.go
internal/storage/storage.go
logs/
migrations/001_init.down.sql
migrations/001_init.up.sql
pkg/README.md
go.mod
go.sum
README.md  а для этой схемы