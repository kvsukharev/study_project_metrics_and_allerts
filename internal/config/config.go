// Package config parses server configuration from CLI flags, environment
// variables and an optional JSON config file.
// Priority (highest → lowest): env vars > CLI flags > JSON file > built-in defaults.
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v6"
)

// ServerConfig holds all tunable parameters for the metrics server.
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
	CryptoKey        string `env:"CRYPTO_KEY"`
}

// serverJSONConfig mirrors the JSON config file format for the server.
// store_interval accepts Go duration strings (e.g. "1s", "5m").
type serverJSONConfig struct {
	Address       string `json:"address"`
	Restore       *bool  `json:"restore"`        // pointer to distinguish false from absent
	StoreInterval string `json:"store_interval"` // duration string
	StoreFile     string `json:"store_file"`
	DatabaseDSN   string `json:"database_dsn"`
	CryptoKey     string `json:"crypto_key"`
}

// FindConfigPath returns the JSON config file path from the -c/-config flag or
// CONFIG env var. Flag value takes precedence over env var. This pre-scan of
// os.Args is necessary because the path must be known before flags are registered
// so that JSON values can be used as flag defaults.
func FindConfigPath() string {
	path := os.Getenv("CONFIG")
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		for _, name := range []string{"-c", "--c", "-config", "--config"} {
			if args[i] == name {
				if i+1 < len(args) {
					path = args[i+1]
				}
				break
			}
			if strings.HasPrefix(args[i], name+"=") {
				path = args[i][len(name)+1:]
				break
			}
		}
	}
	return path
}

// loadJSONFile reads path and unmarshals JSON into v.
func loadJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}

// orStr returns s if non-empty, otherwise def.
func orStr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// ParseFlags parses CLI flags and overlays environment variables.
// If a JSON config file is found via -c/-config/CONFIG, its values are used as
// flag defaults so that explicit flags and env vars always take precedence.
func ParseFlags() (*ServerConfig, error) {
	// Step 1: find and load the JSON config file (lowest priority).
	jcfg := &serverJSONConfig{}
	if cfgPath := FindConfigPath(); cfgPath != "" {
		if err := loadJSONFile(cfgPath, jcfg); err != nil {
			return nil, err
		}
	}

	// Step 2: derive typed defaults from JSON values.
	storeIntervalSec := 300
	if jcfg.StoreInterval != "" {
		d, err := time.ParseDuration(jcfg.StoreInterval)
		if err != nil {
			return nil, fmt.Errorf("store_interval: %w", err)
		}
		storeIntervalSec = int(d.Seconds())
	}
	restore := false
	if jcfg.Restore != nil {
		restore = *jcfg.Restore
	}

	// Step 3: register flags with JSON-derived defaults.
	// flag.Parse() will override these defaults when an explicit flag is given.
	cfg := &ServerConfig{}
	flag.StringVar(&cfg.Address, "a", orStr(jcfg.Address, "localhost:8080"), "HTTP server endpoint address")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC")
	flag.StringVar(&cfg.DatabaseDSN, "d", jcfg.DatabaseDSN, "Database connection string")
	flag.IntVar(&cfg.RateLimit, "l", 5, "Max concurrent requests")
	flag.IntVar(&cfg.StoreIntervalSec, "i", storeIntervalSec, "Store interval in seconds (0 = sync write)")
	flag.StringVar(&cfg.FileStoragePath, "f", orStr(jcfg.StoreFile, "metrics-db.json"), "File storage path")
	flag.BoolVar(&cfg.Restore, "r", restore, "Restore metrics from file on start")
	flag.StringVar(&cfg.AuditFile, "audit-file", "", "Audit log file path (empty = disabled)")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "Audit remote URL (empty = disabled)")
	flag.StringVar(&cfg.CryptoKey, "crypto-key", jcfg.CryptoKey, "Path to RSA private key for decrypting agent requests (empty = disabled)")
	flag.String("c", "", "Path to JSON config file")
	flag.String("config", "", "Path to JSON config file")
	flag.Parse()

	// Step 4: env vars override flags.
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if flag.NArg() > 0 {
		return nil, fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	return cfg, nil
}
