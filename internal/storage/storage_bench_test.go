package storage

import (
	"context"
	"fmt"
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

func BenchmarkUpdateGauge(b *testing.B) {
	s := NewMemStorage()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.UpdateGauge("Alloc", float64(i))
	}
}

func BenchmarkUpdateCounter(b *testing.B) {
	s := NewMemStorage()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.UpdateCounter("PollCount", 1)
	}
}

func BenchmarkBatchUpdate(b *testing.B) {
	s := NewMemStorage()
	ctx := context.Background()

	val := 42.5
	delta := int64(1)
	metrics := makeTestMetrics(val, delta)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.BatchUpdate(ctx, metrics)
	}
}

func BenchmarkGetAllMetrics(b *testing.B) {
	s := NewMemStorage()
	for i := 0; i < 28; i++ {
		s.UpdateGauge(fmt.Sprintf("gauge%d", i), float64(i)*1.1)
	}
	s.UpdateCounter("PollCount", 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetAllMetrics()
	}
}

func BenchmarkGetGauge(b *testing.B) {
	s := NewMemStorage()
	s.UpdateGauge("Alloc", 1234.5)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetGauge("Alloc")
	}
}

func makeTestMetrics(val float64, delta int64) []model.Metrics {
	names := []string{
		"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
		"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
		"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
		"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
		"Sys", "TotalAlloc", "RandomValue",
	}
	metrics := make([]model.Metrics, 0, len(names)+1)
	for _, name := range names {
		v := val
		metrics = append(metrics, model.Metrics{ID: name, MType: model.TypeGauge, Value: &v})
	}
	d := delta
	metrics = append(metrics, model.Metrics{ID: "PollCount", MType: model.TypeCounter, Delta: &d})
	return metrics
}
