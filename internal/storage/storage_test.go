package storage_test

import (
	"context"
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

func TestUpdateAndGetGauge(t *testing.T) {
	s := storage.NewMemStorage()

	s.UpdateGauge("Alloc", 123.45)

	val, err := s.GetGauge("Alloc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 123.45 {
		t.Errorf("expected 123.45, got %f", val)
	}
}

func TestUpdateAndGetCounter(t *testing.T) {
	s := storage.NewMemStorage()

	s.UpdateCounter("PollCount", 1)
	s.UpdateCounter("PollCount", 1)
	s.UpdateCounter("PollCount", 1)

	val, err := s.GetCounter("PollCount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestGetGaugeNotFound(t *testing.T) {
	s := storage.NewMemStorage()

	_, err := s.GetGauge("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent gauge, got nil")
	}
}

func TestGetCounterNotFound(t *testing.T) {
	s := storage.NewMemStorage()

	_, err := s.GetCounter("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent counter, got nil")
	}
}

func TestGetAllMetrics(t *testing.T) {
	s := storage.NewMemStorage()

	s.UpdateGauge("Alloc", 1.0)
	s.UpdateGauge("Sys", 2.0)
	s.UpdateCounter("PollCount", 5)

	gauges, counters := s.GetAllMetrics()

	if len(gauges) != 2 {
		t.Errorf("expected 2 gauges, got %d", len(gauges))
	}
	if len(counters) != 1 {
		t.Errorf("expected 1 counter, got %d", len(counters))
	}
	if gauges["Alloc"] != 1.0 {
		t.Errorf("expected Alloc=1.0, got %f", gauges["Alloc"])
	}
	if counters["PollCount"] != 5 {
		t.Errorf("expected PollCount=5, got %d", counters["PollCount"])
	}
}

func TestGaugeOverwrite(t *testing.T) {
	s := storage.NewMemStorage()

	s.UpdateGauge("Alloc", 1.0)
	s.UpdateGauge("Alloc", 99.9)

	val, err := s.GetGauge("Alloc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 99.9 {
		t.Errorf("expected 99.9, got %f", val)
	}
}

func TestBatchUpdate(t *testing.T) {
	s := storage.NewMemStorage()

	v := 42.0
	d := int64(10)
	metrics := []model.Metrics{
		{ID: "HeapAlloc", MType: model.TypeGauge, Value: &v},
		{ID: "PollCount", MType: model.TypeCounter, Delta: &d},
	}

	err := s.BatchUpdate(context.Background(), metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gauge, err := s.GetGauge("HeapAlloc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gauge != 42.0 {
		t.Errorf("expected 42.0, got %f", gauge)
	}

	counter, err := s.GetCounter("PollCount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter != 10 {
		t.Errorf("expected 10, got %d", counter)
	}
}

func TestBatchUpdateInvalidType(t *testing.T) {
	s := storage.NewMemStorage()

	metrics := []model.Metrics{
		{ID: "foo", MType: "unknown"},
	}

	err := s.BatchUpdate(context.Background(), metrics)
	if err == nil {
		t.Error("expected error for unknown metric type, got nil")
	}
}

func TestPing(t *testing.T) {
	s := storage.NewMemStorage()

	if err := s.Ping(context.Background()); err != nil {
		t.Errorf("Ping() returned error: %v", err)
	}
}

func TestGetAllMetricsReturnsCopy(t *testing.T) {
	s := storage.NewMemStorage()
	s.UpdateGauge("X", 1.0)

	gauges, _ := s.GetAllMetrics()
	gauges["X"] = 999.0

	// Оригинал не должен измениться
	val, _ := s.GetGauge("X")
	if val == 999.0 {
		t.Error("GetAllMetrics() returned a reference, not a copy")
	}
}
