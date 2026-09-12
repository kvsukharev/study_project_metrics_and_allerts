package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

func newBenchRouter() *chi.Mux {
	store := storage.NewMemStorage()
	h := handlers.NewHandlers(store, "", nil)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

var benchMetrics = func() []model.Metrics {
	names := []string{
		"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
		"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
		"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
		"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
		"Sys", "TotalAlloc", "RandomValue",
	}
	ms := make([]model.Metrics, 0, len(names)+1)
	for _, name := range names {
		v := 42.5
		ms = append(ms, model.Metrics{ID: name, MType: model.TypeGauge, Value: &v})
	}
	d := int64(1)
	ms = append(ms, model.Metrics{ID: "PollCount", MType: model.TypeCounter, Delta: &d})
	return ms
}()

func BenchmarkHandlerBatchUpdate(b *testing.B) {
	r := newBenchRouter()
	body, _ := json.Marshal(benchMetrics)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkHandlerUpdateJSON(b *testing.B) {
	r := newBenchRouter()
	val := 42.5
	m := model.Metrics{ID: "Alloc", MType: model.TypeGauge, Value: &val}
	body, _ := json.Marshal(m)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkHandlerGetAllRoot(b *testing.B) {
	r := newBenchRouter()
	// pre-populate
	body, _ := json.Marshal(benchMetrics)
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkHandlerUpdatePlainText(b *testing.B) {
	r := newBenchRouter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/123.45", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}
