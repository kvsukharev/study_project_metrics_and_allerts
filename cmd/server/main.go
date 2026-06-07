package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	middlewareproj "github.com/kvsukharev/go-musthave-metrics-tpl/internal/middleware_proj"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

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

	store := storage.NewMemStorage()
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

func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
