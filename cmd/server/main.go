package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/audit"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	middlewareproj "github.com/kvsukharev/go-musthave-metrics-tpl/internal/middleware_proj"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

var (
	buildVersion = "N/A"
	buildDate    = "N/A"
	buildCommit  = "N/A"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var (
		store storage.Storage
		mem   *storage.MetricsStorage
	)

	if cfg.DatabaseDSN != "" {
		pgStore, err := storage.NewPostgresStorage(ctx, cfg.DatabaseDSN)
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}
		defer pgStore.Close()
		store = pgStore
		log.Printf("Using PostgreSQL storage")
	} else {
		mem = storage.NewMemStorage()

		if cfg.Restore && cfg.FileStoragePath != "" {
			if err := loadMetrics(mem, cfg.FileStoragePath); err != nil {
				log.Printf("Warning: failed to restore metrics from %s: %v", cfg.FileStoragePath, err)
			} else {
				log.Printf("Metrics restored from %s", cfg.FileStoragePath)
			}
		}

		if cfg.FileStoragePath != "" && cfg.StoreIntervalSec == 0 {
			store = &syncStorage{MetricsStorage: mem, path: cfg.FileStoragePath}
		} else {
			store = mem
		}
	}

	// Periodic save goroutine — only for file-based storage.
	if mem != nil && cfg.FileStoragePath != "" && cfg.StoreIntervalSec > 0 {
		interval := time.Duration(cfg.StoreIntervalSec) * time.Second
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := saveMetrics(mem, cfg.FileStoragePath); err != nil {
						log.Printf("Failed to save metrics: %v", err)
					}
				}
			}
		}()
	}

	var auditObservers []audit.Observer
	if cfg.AuditFile != "" {
		fo, err := audit.NewFileObserver(cfg.AuditFile)
		if err != nil {
			return err
		}
		defer fo.Close()
		auditObservers = append(auditObservers, fo)
		log.Printf("Audit file enabled: %s", cfg.AuditFile)
	}
	if cfg.AuditURL != "" {
		auditObservers = append(auditObservers, audit.NewURLObserver(cfg.AuditURL))
		log.Printf("Audit URL enabled: %s", cfg.AuditURL)
	}
	var auditSubject *audit.Subject
	if len(auditObservers) > 0 {
		auditSubject = audit.NewSubject(auditObservers...)
		defer auditSubject.Close()
	}

	h := handlers.NewHandlers(store, cfg.Key, auditSubject)

	r := chi.NewRouter()
	r.Use(middleware.StripSlashes)
	r.Use(middlewareproj.LoggingMiddleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(middlewareproj.GzipMiddleware)
	if cfg.Key != "" {
		r.Use(handlers.NewSHA256CheckMiddleware(cfg.Key))
	}
	h.RegisterRoutes(r)

	srv := &http.Server{Addr: cfg.Address, Handler: r}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("Starting server on %s", cfg.Address)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// Final snapshot on shutdown — only for file-based storage.
	if mem != nil && cfg.FileStoragePath != "" {
		if err := saveMetrics(mem, cfg.FileStoragePath); err != nil {
			log.Printf("Failed to save final snapshot: %v", err)
		} else {
			log.Printf("Final snapshot saved to %s", cfg.FileStoragePath)
		}
	}

	return nil
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

// saveMetrics writes metrics atomically via a temp file + rename to avoid
// corrupt state if the process is killed mid-write.
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

	// Write to a temp file in the same directory, then rename atomically.
	tmp, err := os.CreateTemp(dirOf(path), "metrics-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

func main() {
	buildinfo.Print(buildVersion, buildDate, buildCommit)
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
