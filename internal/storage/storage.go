package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

var (
	ErrMetricNotFound = errors.New("metric not found")
	ErrInvalidType    = errors.New("invalid metric type")
)

type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, value int64)
	BatchUpdate(ctx context.Context, metrics []model.Metrics) error
	GetGauge(name string) (float64, error)
	GetCounter(name string) (int64, error)
	GetAllMetrics() (map[string]float64, map[string]int64)
	Ping(ctx context.Context) error
	Close() error
}

type MetricsStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.RWMutex
}

func (m *MetricsStorage) BatchUpdate(ctx context.Context, metrics []model.Metrics) error {
	for _, metric := range metrics {
		switch metric.MType {
		case model.TypeGauge:
			if metric.Value != nil {
				m.UpdateGauge(metric.ID, *metric.Value)
			}
		case model.TypeCounter:
			if metric.Delta != nil {
				m.UpdateCounter(metric.ID, *metric.Delta)
			}
		default:
			return fmt.Errorf("unknown metric type: %s", metric.MType)
		}
	}
	return nil
}

// Close implements Storage.
func (m *MetricsStorage) Close() error {
	return nil
}

func NewMemStorage() *MetricsStorage {
	return &MetricsStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *MetricsStorage) UpdateGauge(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

func (m *MetricsStorage) UpdateCounter(name string, value int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += value
}

func (m *MetricsStorage) GetGauge(name string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.gauges[name]
	if !exists {
		return 0, ErrMetricNotFound
	}
	return value, nil
}

func (m *MetricsStorage) GetCounter(name string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.counters[name]
	if !exists {
		return 0, ErrMetricNotFound
	}
	return value, nil
}

func (m *MetricsStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gaugesCopy := make(map[string]float64, len(m.gauges))
	countersCopy := make(map[string]int64, len(m.counters))

	for k, v := range m.gauges {
		gaugesCopy[k] = v
	}

	for k, v := range m.counters {
		countersCopy[k] = v
	}

	return gaugesCopy, countersCopy
}

func (m *MetricsStorage) Ping(ctx context.Context) error {
	// Для in-memory хранилища всегда возвращаем успешный ping
	return nil
}
