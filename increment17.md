# Increment 17 — Benchmarks & Memory Profiling

## Что сделано

### Бенчмарки (3 файла)

**`internal/storage/storage_bench_test.go`**
- `BenchmarkUpdateGauge` — одиночное обновление gauge
- `BenchmarkUpdateCounter` — одиночное обновление counter
- `BenchmarkBatchUpdate` — пакетное обновление 29 метрик (реалистичный набор как у агента)
- `BenchmarkGetAllMetrics` — копирование всех метрик (28 gauges + 1 counter)
- `BenchmarkGetGauge` — чтение одной gauge метрики

**`internal/agent/agent_bench_test.go`**
- `BenchmarkCollectorUpdateMetrics` — сбор runtime.MemStats + 28 gauge + 1 counter
- `BenchmarkCollectorGetAllMetrics` — снапшот всех метрик в один срез
- `BenchmarkCollectorGetGauges` / `BenchmarkCollectorGetCounters` — раздельное копирование
- `BenchmarkCompress` — сжатие реалистичного JSON-батча (~1.5 KB)
- `BenchmarkCompressSingle` — сжатие одной метрики
- `BenchmarkComputeHMAC` — подпись сообщения HMAC-SHA256

**`internal/handler/handlers_bench_test.go`**
- `BenchmarkHandlerBatchUpdate` — полный HTTP-цикл POST /updates с 29 метриками
- `BenchmarkHandlerUpdateJSON` — полный HTTP-цикл POST /update одной метрики
- `BenchmarkHandlerGetAllRoot` — GET / (HTML dashboard со всеми метриками)
- `BenchmarkHandlerUpdatePlainText` — POST /update/{type}/{name}/{value}

### Профилирование памяти

Директория `profiles/` в корне проекта:
- `profiles/base.pprof` — снят до оптимизации
- `profiles/result.pprof` — снят после оптимизации

Профиль снимался во время бенчмарков агента (`-benchtime=5s`):
```
go test -run='^$' -bench='BenchmarkCompress|BenchmarkCollector' -benchtime=5s \
  -memprofile=profiles/base.pprof ./internal/agent/
```

### Выявленные проблемы (pprof top)

| Функция | Аллокаций | Причина |
|---|---|---|
| `compress/flate.NewWriter` | 57.42% | `gzip.Writer` создаётся заново при каждом вызове `Compress()` |
| `Collector.GetGauges` | 7.60% | `make(map)` без capacity → rehash при заполнении |
| `Collector.GetCounters` | 8.70% | то же самое |

### Оптимизации

**`internal/agent/util.go`** — `sync.Pool` для `gzip.Writer` и `bytes.Buffer`:
- До: `gzip.NewWriter(&buf)` — выделяет ~32 KB буферов flate каждый вызов
- После: `gzip.Writer` берётся из пула и сбрасывается через `gz.Reset(buf)`, `bytes.Buffer` тоже пулится

**`internal/agent/collector.go`** — capacity hint:
- `make(map[string]float64)` → `make(map[string]float64, len(c.gauge))`
- `make(map[string]int64)` → `make(map[string]int64, len(c.counter))`

### Результат (pprof diff)

```
go tool pprof -top -diff_base=profiles/base.pprof profiles/result.pprof

      flat  flat%        cum
  -35.56GB 57.41%  -43.16GB   compress/flate.NewWriter
   -7.40GB 11.94%   -7.40GB   compress/flate.(*compressor).initDeflate
   -0.85GB  1.37%   -0.85GB   agent.(*Collector).GetCounters
   -0.82GB  1.33%   -0.82GB   agent.(*Collector).GetGauges
```

Все значения отрицательные — цель достигнута.

| Бенчмарк | B/op до | B/op после | allocs до | allocs после |
|---|---|---|---|---|
| BenchmarkCompress | 814 750 | 432 | 21 | 1 |
| BenchmarkCompressSingle | 814 111 | 80 | 20 | 1 |
| BenchmarkCollectorGetGauges | 1 640 | 984 | 7 | 4 |
