# Increment 22 — Generic Pool for Resettable objects

## Что сделано

### Структура

```
internal/pool/
├── pool.go       — generic Pool[T Resettable]
└── pool_test.go  — тесты
```

### Реализация (`internal/pool/pool.go`)

**`Resettable`** — интерфейс-ограничение:
```go
type Resettable interface {
    Reset()
}
```
Совпадает с сигнатурой, которую генерирует `cmd/reset`.

**`Pool[T Resettable]`** — generic-обёртка над `sync.Pool`:

| Элемент | Описание |
|---|---|
| `New[T](newFunc func() T) *Pool[T]` | Конструктор; `newFunc` используется `sync.Pool` при нехватке объектов |
| `Get() T` | Возвращает объект из пула (или создаёт новый через `newFunc`) |
| `Put(v T)` | Вызывает `v.Reset()`, затем возвращает объект в пул |

Вызов `Reset()` в `Put` гарантирует, что следующий вызывающий получит объект в чистом состоянии, без «просочившегося» состояния предыдущего пользователя.

### Пример использования

```go
p := pool.New(func() *audit.Event { return &audit.Event{} })

e := p.Get()
e.TS = time.Now().Unix()
e.Metrics = append(e.Metrics, "gauge/cpu")
// ... use e ...
p.Put(e) // Reset() вызывается автоматически
```
