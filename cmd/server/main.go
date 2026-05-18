package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	handlers "github.com/kvsukharev/go-musthave-metrics-tpl/internal/handler"
	middlewareproj "github.com/kvsukharev/go-musthave-metrics-tpl/internal/middleware_proj"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/server"
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

	// Применение middleware
	r.Use(
		middleware.Logger,
		middleware.Recoverer,
		middlewareproj.GzipMiddleware,
	)

	// Регистрация маршрутов
	h.RegisterRoutes(r)

	// Инициализация хранилища
	var dbStorage *storage.PostgresStorage

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
