# Increment 20 — Static analyzer (linter)

## Что сделано

### Структура

```
cmd/linter/
├── main.go                          — точка входа multichecker
└── analyzer/
    ├── panic_check.go               — анализатор paniccheck
    ├── exit_check.go                — анализатор exitcheck
    ├── analyzer_test.go             — тесты через analysistest
    └── testdata/src/
        ├── paniccheck/paniccheck.go — тест-кейс для paniccheck
        └── exitcheck/exitcheck.go   — тест-кейс для exitcheck
```

### Анализаторы

**`paniccheck`** (`analyzer/panic_check.go`)
- Обходит AST через `inspect.Analyzer`
- Находит все `ast.CallExpr`, где `Fun` — `ast.Ident` с именем `"panic"`
- Сообщает: `use of built-in panic is forbidden`

**`exitcheck`** (`analyzer/exit_check.go`)
- Работает только в пакете `main`
- Запрещает вызовы: `os.Exit`, `log.Fatal`, `log.Fatalf`, `log.Fatalln`
- Исключение: вызовы внутри функции `main()` допустимы
- Определяет тело `main()` по `ast.BlockStmt`, проверяет позицию вызова через `nodeInBlock`
- Сообщает: `call to <pkg>.<func> is forbidden outside of main.main`

### Запуск

```bash
go run ./cmd/linter/... ./...
```

### Тесты

```bash
go test ./cmd/linter/analyzer/ -v
```

Используется `analysistest.Run` с тест-файлами в `testdata/src/`.
Комментарии `// want` задают ожидаемые диагностики.

### Проверка проекта

Весь проект проходит оба анализатора без замечаний:
- `log.Fatalf` в `cmd/server/main.go` и `os.Exit` в `cmd/agent/main.go` находятся строго внутри функции `main()` — анализатор их пропускает.
