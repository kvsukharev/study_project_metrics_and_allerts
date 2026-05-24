package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

func newRouter() *chi.Mux {
	store := storage.NewMemStorage()
	h := handlers.NewHandlers(store)
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestUpdateGauge(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Alloc/123.45", nil)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestUpdateCounter(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/1", nil)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestUpdateInvalidType(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/unknown/foo/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestUpdateGaugeInvalidValue(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/foo/notanumber", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestUpdateCounterInvalidValue(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/counter/foo/3.14", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestUpdateMissingValue(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/foo", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetGauge(t *testing.T) {
	r := newRouter()

	// Сначала записываем
	req := httptest.NewRequest(http.MethodPost, "/update/gauge/Sys/999.0", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	// Потом читаем
	req = httptest.NewRequest(http.MethodGet, "/value/gauge/Sys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "999") {
		t.Errorf("expected body to contain 999, got: %s", w.Body.String())
	}
}

func TestGetCounter(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodGet, "/value/counter/PollCount", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "5") {
		t.Errorf("expected body to contain 5, got: %s", w.Body.String())
	}
}

func TestGetNotFound(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/value/gauge/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetUnknownType(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/value/unknown/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for unknown type, got %d", w.Code)
	}
}

func TestGetGaugeValue(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodPost, "/update/gauge/HeapSys/512.5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodGet, "/value/gauge/HeapSys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %s", ct)
	}
	if body := strings.TrimSpace(w.Body.String()); body == "" {
		t.Error("expected non-empty body")
	}
}

func TestCounterAccumulatesOnServer(t *testing.T) {
	r := newRouter()

	// Агент шлёт PollCount=1 несколько раз — сервер должен накапливать
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/1", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/value/counter/PollCount", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "5") {
		t.Errorf("expected accumulated counter=5, got: %s", w.Body.String())
	}
}

func TestCounterAccumulates(t *testing.T) {
	r := newRouter()

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/update/counter/PollCount/10", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/value/counter/PollCount", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "30") {
		t.Errorf("expected counter to be 30, got: %s", w.Body.String())
	}
}

func TestUpdateJSONGauge(t *testing.T) {
	r := newRouter()

	val := 42.5
	body, _ := json.Marshal(map[string]interface{}{
		"id":    "HeapAlloc",
		"type":  "gauge",
		"value": val,
	})

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestUpdateJSONCounter(t *testing.T) {
	r := newRouter()

	delta := int64(7)
	body, _ := json.Marshal(map[string]interface{}{
		"id":    "PollCount",
		"type":  "counter",
		"delta": delta,
	})

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRootHandler(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %s", ct)
	}
}

func TestBatchUpdate(t *testing.T) {
	r := newRouter()

	val := 1.23
	delta := int64(5)
	metrics := []map[string]interface{}{
		{"id": "Alloc", "type": "gauge", "value": val},
		{"id": "PollCount", "type": "counter", "delta": delta},
	}
	body, _ := json.Marshal(metrics)

	req := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPingHandler(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAllRuntimeGaugesUpdate(t *testing.T) {
	r := newRouter()

	gaugeNames := []string{
		"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
		"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
		"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
		"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
		"Sys", "TotalAlloc", "RandomValue",
	}

	for i, name := range gaugeNames {
		url := fmt.Sprintf("/update/gauge/%s/%d", name, i)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("gauge %s: expected status 200, got %d", name, w.Code)
		}
	}
}
