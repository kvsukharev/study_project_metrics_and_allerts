package config

import (
	"flag"
	"fmt"

	"github.com/caarlos0/env/v6"
)

type ServerConfig struct {
	Key              string `env:"KEY"`
	Address          string `env:"ADDRESS"`
	StoreIntervalSec int    `env:"STORE_INTERVAL"`    // seconds; 0 = sync write
	FileStoragePath  string `env:"FILE_STORAGE_PATH"` // path to persistence file
	Restore          bool   `env:"RESTORE"`
	DatabaseDSN      string `env:"DATABASE_DSN"`
	RateLimit        int    `env:"RATE_LIMIT"`
	AuditFile        string `env:"AUDIT_FILE"`
	AuditURL         string `env:"AUDIT_URL"`
}

func ParseFlags() (*ServerConfig, error) {
	cfg := &ServerConfig{
		Address:          "localhost:8080",
		RateLimit:        5,
		StoreIntervalSec: 300,
		FileStoragePath:  "metrics-db.json",
		Restore:          false,
	}

	flag.StringVar(&cfg.Address, "a", "localhost:8080", "HTTP server endpoint address")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC")
	flag.StringVar(&cfg.DatabaseDSN, "d", "", "Database connection string")
	flag.IntVar(&cfg.RateLimit, "l", 5, "Max concurrent requests")
	flag.IntVar(&cfg.StoreIntervalSec, "i", 300, "Store interval in seconds (0 = sync write)")
	flag.StringVar(&cfg.FileStoragePath, "f", "metrics-db.json", "File storage path")
	flag.BoolVar(&cfg.Restore, "r", false, "Restore metrics from file on start")
	flag.StringVar(&cfg.AuditFile, "audit-file", "", "Audit log file path (empty = disabled)")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "Audit remote URL (empty = disabled)")
	flag.Parse()

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if flag.NArg() > 0 {
		return nil, fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	return cfg, nil
}
