// Package handlers implements the HTTP handlers for the metrics server.
// Routes are registered via Handlers.RegisterRoutes onto a chi.Router.
package handlers

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/audit"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"

	"github.com/go-chi/chi/v5"
)

// Handlers groups all HTTP handler methods for the metrics server.
// Create one with NewHandlers and register its routes via RegisterRoutes.
type Handlers struct {
	storage storage.Storage
	key     string
	audit   *audit.Subject // nil when audit is disabled
}

// NewHandlers creates a Handlers bound to the given storage backend.
// key is the HMAC-SHA256 signing key; pass an empty string to disable signing.
// auditSubject may be nil to disable audit logging.
func NewHandlers(storage storage.Storage, key string, auditSubject *audit.Subject) *Handlers {
	return &Handlers{storage: storage, key: key, audit: auditSubject}
}

// extractIP returns the client IP from the request, preferring X-Real-IP and
// X-Forwarded-For headers over RemoteAddr.
func extractIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			return fwd[:i]
		}
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handlers) notifyAudit(r *http.Request, metricNames []string) {
	if h.audit == nil {
		return
	}
	h.audit.Notify(audit.Event{
		TS:        time.Now().Unix(),
		Metrics:   metricNames,
		IPAddress: extractIP(r),
	})
}

// RegisterRoutes mounts all metric endpoints onto r:
//
//	POST /update              – update a single metric via JSON body
//	POST /updates             – batch-update metrics via JSON array
//	POST /update/{type}/{name}/{value} – update a metric via URL path
//	POST /value               – retrieve a metric value via JSON body
//	GET  /value/{type}/{name} – retrieve a metric value via URL path
//	GET  /                    – HTML dashboard with all current metrics
//	GET  /ping                – liveness probe for the storage backend
func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Post("/update", h.updateMetricJSONHandler)
	r.Post("/value", h.valueMetricJSONHandler)
	r.Post("/updates", h.BatchUpdateMetrics)
	r.Post("/update/{type}/{name}/{value}", h.updateHandlerChi)
	r.Post("/update/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "metric value is required", http.StatusNotFound)
	})
	r.Post("/update/{type}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "metric name is required", http.StatusNotFound)
	})
	r.Get("/value/{type}/{name}", h.valueHandler)
	r.Get("/", h.rootHandler)
	r.Get("/ping", h.PingHandler())
}

// PingHandler returns an http.HandlerFunc that checks storage connectivity.
// Responds 200 OK when the backend is reachable, 500 otherwise.
func (h *Handlers) PingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.storage.Ping(r.Context()); err != nil {
			http.Error(w, "Database unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handlers) valueMetricJSONHandler(w http.ResponseWriter, r *http.Request) {
	var req model.Metrics
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	switch req.MType {
	case model.TypeGauge:
		value, err := h.storage.GetGauge(req.ID)
		if err != nil {
			http.Error(w, "Metric not found", http.StatusNotFound)
			return
		}
		req.Value = &value

	case model.TypeCounter:
		value, err := h.storage.GetCounter(req.ID)
		if err != nil {
			http.Error(w, "Metric not found", http.StatusNotFound)
			return
		}
		req.Delta = &value

	default:
		http.Error(w, "Invalid metric type", http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "marshal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeSignedResponse(w, body, h.key)
}

func decodeBody(r *http.Request, v interface{}) error {
	// Обработка gzip
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return err
		}
		defer gz.Close()
		return json.NewDecoder(gz).Decode(v)
	}
	return json.NewDecoder(r.Body).Decode(v)

}

// BatchUpdateMetrics handles POST /updates.
// It accepts a JSON array of Metrics objects and applies them atomically.
// On success it responds 200 OK and emits an audit event with all metric names.
func (h *Handlers) BatchUpdateMetrics(w http.ResponseWriter, r *http.Request) {
	var metrics []model.Metrics

	// Декодируем тело запроса
	if err := decodeBody(r, &metrics); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Проверяем наличие метрик
	if len(metrics) == 0 {
		http.Error(w, "empty metrics batch", http.StatusBadRequest)
		return
	}

	// Выполняем пакетное обновление
	if err := h.storage.BatchUpdate(r.Context(), metrics); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	names := make([]string, 0, len(metrics))
	for _, m := range metrics {
		names = append(names, m.ID)
	}
	h.notifyAudit(r, names)

	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) updateMetricJSONHandler(w http.ResponseWriter, r *http.Request) {
	var metric model.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	switch metric.MType {
	case model.TypeGauge:
		if metric.Value == nil {
			http.Error(w, "Missing value for gauge", http.StatusBadRequest)
			return
		}
		h.storage.UpdateGauge(metric.ID, *metric.Value)
		val, _ := h.storage.GetGauge(metric.ID)
		metric.Value = &val

	case model.TypeCounter:
		if metric.Delta == nil {
			http.Error(w, "Missing delta for counter", http.StatusBadRequest)
			return
		}
		h.storage.UpdateCounter(metric.ID, *metric.Delta)
		delta, _ := h.storage.GetCounter(metric.ID)
		metric.Delta = &delta

	default:
		http.Error(w, "Invalid metric type", http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(metric)
	if err != nil {
		http.Error(w, "marshal error", http.StatusInternalServerError)
		return
	}

	h.notifyAudit(r, []string{metric.ID})

	w.Header().Set("Content-Type", "application/json")
	writeSignedResponse(w, body, h.key)
}

func (h *Handlers) updateHandlerChi(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricName := chi.URLParam(r, "name")
	metricValue := chi.URLParam(r, "value")
	if h.updateMetric(w, metricType, metricName, metricValue) {
		h.notifyAudit(r, []string{metricName})
	}
}

// updateMetric applies the update and returns true on success.
func (h *Handlers) updateMetric(w http.ResponseWriter, metricType, metricName, metricValue string) bool {
	switch metricType {
	case "gauge":
		value, err := strconv.ParseFloat(metricValue, 64)
		if err != nil {
			http.Error(w, "Invalid gauge value", http.StatusBadRequest)
			return false
		}
		h.storage.UpdateGauge(metricName, value)
		log.Printf("Updated gauge %s = %.6f", metricName, value)

	case "counter":
		value, err := strconv.ParseInt(metricValue, 10, 64)
		if err != nil {
			http.Error(w, "Invalid counter value", http.StatusBadRequest)
			return false
		}
		h.storage.UpdateCounter(metricName, value)
		log.Printf("Updated counter %s (added %d)", metricName, value)

	default:
		http.Error(w, "Unknown metric type. Use 'gauge' or 'counter'", http.StatusBadRequest)
		return false
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK\n")
	return true
}

func (h *Handlers) valueHandler(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricName := chi.URLParam(r, "name")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	switch metricType {
	case "gauge":
		value, err := h.storage.GetGauge(metricName)
		if err != nil {
			http.Error(w, "Metric not found", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, "%g", value)

	case "counter":
		value, err := h.storage.GetCounter(metricName)
		if err != nil {
			http.Error(w, "Metric not found", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, "%d", value)

	default:
		http.Error(w, "Unknown metric type. Use 'gauge' or 'counter'", http.StatusNotFound)
	}
}

func (h *Handlers) rootHandler(w http.ResponseWriter, r *http.Request) {
	gauges, counters := h.storage.GetAllMetrics()

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Metrics Server</title>
    <style>
        /* ... (ваши стили остаются без изменений) ... */
    </style>
</head>
<body>
    <div class="container">
        <h1>Metrics Server Dashboard</h1>
        
        <h2>Gauges <span class="count">({{len .Gauges}})</span></h2>
        <table>
            <tr><th>Name</th><th>Value</th></tr>
            {{range $name, $value := .Gauges}}
            <tr><td><strong>{{$name}}</strong></td><td>{{printf "%.6f" $value}}</td></tr>
            {{else}}
            <tr><td colspan="2" style="text-align: center; color: #666;">No gauges available</td></tr>
            {{end}}
        </table>
        
        <h2>Counters <span class="count">({{len .Counters}})</span></h2>
        <table>
            <tr><th>Name</th><th>Value</th></tr>
            {{range $name, $value := .Counters}}
            <tr><td><strong>{{$name}}</strong></td><td>{{$value}}</td></tr>
            {{else}}
            <tr><td colspan="2" style="text-align: center; color: #666;">No counters available</td></tr>
            {{end}}
        </table>
        
        <div style="margin-top: 30px; padding: 15px; background-color: #e7f3ff; border-left: 4px solid #2196F3;">
            <h3>API Endpoints:</h3>
            <ul>
                <li><code>POST /update/{type}/{name}/{value}</code> - Update metric</li>
                <li><code>GET /value/{type}/{name}</code> - Get metric value</li>
                <li><code>GET /</code> - This dashboard</li>
                <li><code>GET /ping</code> - Check database connection</li>
            </ul>
        </div>
    </div>
</body>
</html>`

	t, err := template.New("metrics").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		log.Printf("Template parse error: %v", err)
		return
	}

	data := struct {
		Gauges   map[string]float64
		Counters map[string]int64
	}{
		Gauges:   gauges,
		Counters: counters,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if err := t.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// NewSHA256CheckMiddleware returns middleware that verifies the HashSHA256
// request header against an HMAC-SHA256 of the request body using key.
// If key is empty the middleware is a no-op and passes every request through.
func NewSHA256CheckMiddleware(key string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "cannot read body", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			gotHash := r.Header.Get("HashSHA256")
			expectedHash := agent.ComputeHMAC(bodyBytes, key)
			if !hmac.Equal([]byte(expectedHash), []byte(gotHash)) {
				http.Error(w, "invalid hash", http.StatusBadRequest)
				return
			}

			// Вернуть тело для следующего обработчика
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			next.ServeHTTP(w, r)
		})
	}
}

func writeSignedResponse(w http.ResponseWriter, body []byte, key string) {
	if key != "" {
		w.Header().Set("HashSHA256", agent.ComputeHMAC(body, key))
	}
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
