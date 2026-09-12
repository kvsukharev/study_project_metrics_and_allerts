// Package storage provides in-memory and PostgreSQL implementations of the
// metric storage backend used by the server.
package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

var (
	// ErrMetricNotFound is returned when the requested metric does not exist.
	ErrMetricNotFound = errors.New("metric not found")
	// ErrInvalidType is returned when an unknown metric type is provided.
	ErrInvalidType = errors.New("invalid metric type")
)

// Storage is the interface implemented by all metric storage backends.
// It is safe for concurrent use from multiple goroutines.
type Storage interface {
	// UpdateGauge sets the gauge metric to value, replacing any previous value.
	UpdateGauge(name string, value float64)
	// UpdateCounter adds delta to the current counter value (starts at 0).
	UpdateCounter(name string, value int64)
	// BatchUpdate atomically applies a slice of metric updates.
	BatchUpdate(ctx context.Context, metrics []model.Metrics) error
	// GetGauge returns the current gauge value or ErrMetricNotFound.
	GetGauge(name string) (float64, error)
	// GetCounter returns the current counter value or ErrMetricNotFound.
	GetCounter(name string) (int64, error)
	// GetAllMetrics returns a snapshot copy of all gauges and counters.
	GetAllMetrics() (map[string]float64, map[string]int64)
	// Ping checks the backend connectivity (always nil for in-memory).
	Ping(ctx context.Context) error
	// Close releases any resources held by the backend.
	Close() error
}

// MetricsStorage is an in-memory Storage implementation backed by two maps
// protected by an RWMutex. Suitable for single-node deployments without
// persistence requirements (use FileStoragePath for durability).
type MetricsStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.RWMutex
}

func (m *MetricsStorage) BatchUpdate(ctx context.Context, metrics []model.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, metric := range metrics {
		switch metric.MType {
		case model.TypeGauge:
			if metric.Value != nil {
				m.gauges[metric.ID] = *metric.Value
			}
		case model.TypeCounter:
			if metric.Delta != nil {
				m.counters[metric.ID] += *metric.Delta
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

// NewMemStorage returns a ready-to-use in-memory MetricsStorage.
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
