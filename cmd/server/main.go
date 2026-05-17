package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/middleware_proj"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/storage"
)

func run() error {
	cfg, err := config.ParseFlags()
	if err != nil {
		return err
	}

	store := storage.NewMemStorage()
	h := handlers.NewHandlers(store)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware_proj.GzipMiddleware)
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
