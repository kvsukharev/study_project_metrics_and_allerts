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

	"dario.cat/mergo"
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

// serverConfigFromJSON converts a parsed JSON config into the typed ServerConfig
// representation. Only fields present in the JSON file are non-zero in the result;
// mergo will fill the rest from built-in defaults.
func serverConfigFromJSON(jcfg *serverJSONConfig) (ServerConfig, error) {
	cfg := ServerConfig{
		Address:         jcfg.Address,
		FileStoragePath: jcfg.StoreFile,
		DatabaseDSN:     jcfg.DatabaseDSN,
		CryptoKey:       jcfg.CryptoKey,
	}
	if jcfg.StoreInterval != "" {
		d, err := time.ParseDuration(jcfg.StoreInterval)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("store_interval: invalid duration %q: %w", jcfg.StoreInterval, err)
		}
		cfg.StoreIntervalSec = int(d.Seconds())
	}
	if jcfg.Restore != nil {
		cfg.Restore = *jcfg.Restore
	}
	return cfg, nil
}

// serverDefaults holds the built-in default values for fields that mergo will
// use to fill any zero fields not provided by the JSON config or CLI flags.
var serverDefaults = ServerConfig{
	Address:          "localhost:8080",
	RateLimit:        5,
	StoreIntervalSec: 300,
	FileStoragePath:  "metrics-db.json",
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

// ParseFlags parses CLI flags and overlays environment variables.
// If a JSON config file is found via -c/-config/CONFIG, its values are merged
// with built-in defaults using mergo — adding a new field only requires changes
// in one place (serverJSONConfig + serverConfigFromJSON + flag registration).
func ParseFlags() (*ServerConfig, error) {
	// Step 1: load JSON config (lowest priority).
	jcfg := &serverJSONConfig{}
	if cfgPath := FindConfigPath(); cfgPath != "" {
		if err := loadJSONFile(cfgPath, jcfg); err != nil {
			return nil, err
		}
	}

	// Step 2: convert JSON → typed config (errors on bad values, e.g. invalid duration).
	fromJSON, err := serverConfigFromJSON(jcfg)
	if err != nil {
		return nil, err
	}

	// Step 3: fill zero fields with built-in defaults.
	if err := mergo.Merge(&fromJSON, serverDefaults); err != nil {
		return nil, fmt.Errorf("merge config defaults: %w", err)
	}

	// Step 4: register flags with the merged defaults.
	// Explicit flags override the merged defaults; env vars (step 5) override flags.
	cfg := &ServerConfig{}
	flag.StringVar(&cfg.Address, "a", fromJSON.Address, "HTTP server endpoint address")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC")
	flag.StringVar(&cfg.DatabaseDSN, "d", fromJSON.DatabaseDSN, "Database connection string")
	flag.IntVar(&cfg.RateLimit, "l", fromJSON.RateLimit, "Max concurrent requests")
	flag.IntVar(&cfg.StoreIntervalSec, "i", fromJSON.StoreIntervalSec, "Store interval in seconds (0 = sync write)")
	flag.StringVar(&cfg.FileStoragePath, "f", fromJSON.FileStoragePath, "File storage path")
	flag.BoolVar(&cfg.Restore, "r", fromJSON.Restore, "Restore metrics from file on start")
	flag.StringVar(&cfg.AuditFile, "audit-file", "", "Audit log file path (empty = disabled)")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "Audit remote URL (empty = disabled)")
	flag.StringVar(&cfg.CryptoKey, "crypto-key", fromJSON.CryptoKey, "Path to RSA private key for decrypting agent requests (empty = disabled)")
	flag.String("c", "", "Path to JSON config file")
	flag.String("config", "", "Path to JSON config file")
	flag.Parse()

	// Step 5: env vars override flags.
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if flag.NArg() > 0 {
		return nil, fmt.Errorf("unknown arguments: %v", flag.Args())
	}

	return cfg, nil
}
