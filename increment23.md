# Increment 23 — Build info at startup

## Что сделано

В `cmd/server/main.go` и `cmd/agent/main.go` добавлены глобальные переменные сборки и вывод их значений при старте приложения.

### Переменные

```go
var (
    buildVersion string
    buildDate    string
    buildCommit  string
)
```

Значения задаются через `-ldflags` при сборке:
```bash
go build -ldflags "-X main.buildVersion=1.2.3 -X main.buildDate=2026-09-12 -X main.buildCommit=abc1234" ./cmd/server/...
```

### Вывод при старте

```
Build version: 1.2.3
Build date: 2026-09-12
Build commit: abc1234
```

Если переменная не задана (пустая строка), выводится `N/A`:
```
Build version: N/A
Build date: N/A
Build commit: N/A
```

### Реализация

Вспомогательная функция `na` возвращает значение или `"N/A"`:
```go
func na(s string) string {
    if s == "" {
        return "N/A"
    }
    return s
}
```

Вызов в `main()` до `run()`:
```go
fmt.Printf("Build version: %s\nBuild date: %s\nBuild commit: %s\n",
    na(buildVersion), na(buildDate), na(buildCommit))
```
