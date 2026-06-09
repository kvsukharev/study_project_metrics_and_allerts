package agent_test

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

func decodeGzipJSON(r *http.Request, v interface{}) error {
	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	return json.NewDecoder(gz).Decode(v)
}

func TestNewSender(t *testing.T) {
	sender := agent.NewSender("http://localhost:8080")
	if sender == nil {
		t.Fatal("NewSender() returned nil")
	}
}

func TestSendGauge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", ct)
		}
		if ce := r.Header.Get("Content-Encoding"); ce != "gzip" {
			t.Errorf("Expected Content-Encoding 'gzip', got '%s'", ce)
		}
		if r.URL.Path != "/update" {
			t.Errorf("Expected path '/update', got '%s'", r.URL.Path)
		}
		var m model.Metrics
		if err := decodeGzipJSON(r, &m); err != nil {
			t.Errorf("Failed to decode body: %v", err)
		}
		if m.ID != "testGauge" || m.MType != model.TypeGauge || m.Value == nil || *m.Value != 3.14 {
			t.Errorf("Unexpected metric: %+v", m)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := agent.NewSender(server.URL)
	if err := sender.SendGauge("testGauge", 3.14); err != nil {
		t.Errorf("SendGauge() failed: %v", err)
	}
}

func TestSendCounter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/update" {
			t.Errorf("Expected path '/update', got '%s'", r.URL.Path)
		}
		if ce := r.Header.Get("Content-Encoding"); ce != "gzip" {
			t.Errorf("Expected Content-Encoding 'gzip', got '%s'", ce)
		}
		var m model.Metrics
		if err := decodeGzipJSON(r, &m); err != nil {
			t.Errorf("Failed to decode body: %v", err)
		}
		if m.ID != "testCounter" || m.MType != model.TypeCounter || m.Delta == nil || *m.Delta != 42 {
			t.Errorf("Unexpected metric: %+v", m)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := agent.NewSender(server.URL)
	if err := sender.SendCounter("testCounter", 42); err != nil {
		t.Errorf("SendCounter() failed: %v", err)
	}
}

func TestSendAllMetrics(t *testing.T) {
	received := make(map[string]model.Metrics)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m model.Metrics
		if err := decodeGzipJSON(r, &m); err != nil {
			t.Errorf("Failed to decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received[m.ID] = m
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := agent.NewSender(server.URL)

	gauges := map[string]float64{"gauge1": 1.23, "gauge2": 4.56}
	counters := map[string]int64{"counter1": 10, "counter2": 20}

	if err := sender.SendAllMetrics(gauges, counters); err != nil {
		t.Errorf("SendAllMetrics() failed: %v", err)
	}

	for name, val := range gauges {
		m, ok := received[name]
		if !ok {
			t.Errorf("Metric %s was not sent", name)
			continue
		}
		if m.MType != model.TypeGauge || m.Value == nil || *m.Value != val {
			t.Errorf("Metric %s: unexpected value %+v", name, m)
		}
	}
	for name, val := range counters {
		m, ok := received[name]
		if !ok {
			t.Errorf("Metric %s was not sent", name)
			continue
		}
		if m.MType != model.TypeCounter || m.Delta == nil || *m.Delta != val {
			t.Errorf("Metric %s: unexpected value %+v", name, m)
		}
	}
}

func TestSendMetricServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := agent.NewSender(server.URL)
	err := sender.SendGauge("testGauge", 1.0)

	if err == nil {
		t.Error("Expected error for server error response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain '500', got: %v", err)
	}
}
