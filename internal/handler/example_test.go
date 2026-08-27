package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

// newExampleRouter creates a router wired to an in-memory storage with no auth.
func newExampleRouter() *chi.Mux {
	store := storage.NewMemStorage()
	h := handlers.NewHandlers(store, "", nil)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// ExampleHandlers_RegisterRoutes_updateGaugePlainText demonstrates updating a
// gauge metric via the plain-text URL path endpoint:
//
//	POST /update/{type}/{name}/{value}
func ExampleHandlers_RegisterRoutes_updateGaugePlainText() {
	r := newExampleRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/123.45", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	// Output:
	// 200
}

// ExampleHandlers_RegisterRoutes_updateCounterPlainText demonstrates updating a
// counter metric via the plain-text URL path endpoint:
//
//	POST /update/{type}/{name}/{value}
func ExampleHandlers_RegisterRoutes_updateCounterPlainText() {
	r := newExampleRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	// Output:
	// 200
}

// ExampleHandlers_RegisterRoutes_updateJSON demonstrates updating a single
// metric via JSON body:
//
//	POST /update
func ExampleHandlers_RegisterRoutes_updateJSON() {
	r := newExampleRouter()

	val := 512.0
	m := model.Metrics{ID: "HeapAlloc", MType: model.TypeGauge, Value: &val}
	body, _ := json.Marshal(m)

	req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	// Output:
	// 200
}

// ExampleHandlers_RegisterRoutes_batchUpdate demonstrates sending multiple
// metrics in one request via JSON array:
//
//	POST /updates
func ExampleHandlers_RegisterRoutes_batchUpdate() {
	r := newExampleRouter()

	val := 1024.0
	delta := int64(7)
	metrics := []model.Metrics{
		{ID: "Alloc", MType: model.TypeGauge, Value: &val},
		{ID: "PollCount", MType: model.TypeCounter, Delta: &delta},
	}
	body, _ := json.Marshal(metrics)

	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	// Output:
	// 200
}

// ExampleHandlers_RegisterRoutes_getValuePlainText demonstrates reading a gauge
// value via the plain-text URL path endpoint:
//
//	GET /value/{type}/{name}
func ExampleHandlers_RegisterRoutes_getValuePlainText() {
	r := newExampleRouter()

	// First, store a gauge value.
	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Sys/9000", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	// Then, retrieve it.
	req = httptest.NewRequest(http.MethodGet, "/value/gauge/Sys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body, _ := io.ReadAll(w.Body)
	fmt.Printf("%d %s\n", w.Code, string(body))
	// Output:
	// 200 9000
}

// ExampleHandlers_RegisterRoutes_getValueJSON demonstrates reading a metric
// value via JSON body:
//
//	POST /value
func ExampleHandlers_RegisterRoutes_getValueJSON() {
	r := newExampleRouter()

	// Store a counter first.
	delta := int64(5)
	update := model.Metrics{ID: "PollCount", MType: model.TypeCounter, Delta: &delta}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	// Now query it.
	query := model.Metrics{ID: "PollCount", MType: model.TypeCounter}
	body, _ = json.Marshal(query)
	req = httptest.NewRequest(http.MethodPost, "/value", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var result model.Metrics
	_ = json.NewDecoder(w.Body).Decode(&result)
	fmt.Printf("%d delta=%d\n", w.Code, *result.Delta)
	// Output:
	// 200 delta=5
}

// ExampleHandlers_RegisterRoutes_ping demonstrates the storage liveness probe:
//
//	GET /ping
func ExampleHandlers_RegisterRoutes_ping() {
	r := newExampleRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	// Output:
	// 200
}

// ExampleHandlers_RegisterRoutes_dashboard demonstrates the HTML metrics
// dashboard:
//
//	GET /
func ExampleHandlers_RegisterRoutes_dashboard() {
	r := newExampleRouter()

	// Populate some metrics first.
	val := 42.0
	body, _ := json.Marshal([]model.Metrics{
		{ID: "Alloc", MType: model.TypeGauge, Value: &val},
	})
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	fmt.Println(w.Code)
	fmt.Println(w.Header().Get("Content-Type"))
	// Output:
	// 200
	// text/html; charset=utf-8
}
