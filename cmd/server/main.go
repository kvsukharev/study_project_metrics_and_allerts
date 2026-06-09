package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	middlewareproj "github.com/kvsukharev/go-musthave-metrics-tpl/internal/middleware_proj"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

// syncStorage wraps MemStorage and saves to file after every write (sync mode).
type syncStorage struct {
	*storage.MetricsStorage
	path string
	mu   sync.Mutex
}

func (s *syncStorage) UpdateGauge(name string, value float64) {
	s.MetricsStorage.UpdateGauge(name, value)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = saveMetrics(s.MetricsStorage, s.path)
}

func (s *syncStorage) UpdateCounter(name string, value int64) {
	s.MetricsStorage.UpdateCounter(name, value)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = saveMetrics(s.MetricsStorage, s.path)
}

func (s *syncStorage) BatchUpdate(ctx context.Context, metrics []model.Metrics) error {
	err := s.MetricsStorage.BatchUpdate(ctx, metrics)
	if err == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = saveMetrics(s.MetricsStorage, s.path)
	}
	return err
}

func run() error {
	cfg, err := config.ParseFlags()
	if err != nil {
		return err
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	defer logger.Sync()

	mem := storage.NewMemStorage()

	if cfg.Restore && cfg.FileStoragePath != "" {
		if err := loadMetrics(mem, cfg.FileStoragePath); err != nil {
			log.Printf("Warning: failed to restore metrics from %s: %v", cfg.FileStoragePath, err)
		} else {
			log.Printf("Metrics restored from %s", cfg.FileStoragePath)
		}
	}

	// Choose storage implementation based on sync mode
	var store storage.Storage
	if cfg.FileStoragePath != "" && cfg.StoreIntervalSec == 0 {
		store = &syncStorage{MetricsStorage: mem, path: cfg.FileStoragePath}
	} else {
		store = mem
	}

	// Periodic save goroutine
	if cfg.FileStoragePath != "" && cfg.StoreIntervalSec > 0 {
		interval := time.Duration(cfg.StoreIntervalSec) * time.Second
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				if err := saveMetrics(mem, cfg.FileStoragePath); err != nil {
					log.Printf("Failed to save metrics: %v", err)
				}
			}
		}()
	}

	h := handlers.NewHandlers(store)

	r := chi.NewRouter()
	r.Use(middleware.StripSlashes)
	r.Use(middlewareproj.LoggingMiddleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(middlewareproj.GzipMiddleware)
	if cfg.Key != "" {
		r.Use(handlers.NewSHA256CheckMiddleware(cfg.Key))
	}
	h.RegisterRoutes(r)

	log.Printf("Starting server on %s", cfg.Address)
	return http.ListenAndServe(cfg.Address, r)
}

func loadMetrics(store *storage.MetricsStorage, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return err
	}

	for _, m := range metrics {
		switch m.MType {
		case model.TypeGauge:
			if m.Value != nil {
				store.UpdateGauge(m.ID, *m.Value)
			}
		case model.TypeCounter:
			if m.Delta != nil {
				store.UpdateCounter(m.ID, *m.Delta)
			}
		}
	}
	return nil
}

func saveMetrics(store *storage.MetricsStorage, path string) error {
	gauges, counters := store.GetAllMetrics()

	metrics := make([]model.Metrics, 0, len(gauges)+len(counters))
	for id, val := range gauges {
		v := val
		metrics = append(metrics, model.Metrics{ID: id, MType: model.TypeGauge, Value: &v})
	}
	for id, delta := range counters {
		d := delta
		metrics = append(metrics, model.Metrics{ID: id, MType: model.TypeCounter, Delta: &d})
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
