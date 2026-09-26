package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
)

func writeJSON(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "cfg-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestFindConfigPath_EnvVar(t *testing.T) {
	want := "/tmp/myconfig.json"
	t.Setenv("CONFIG", want)
	got := config.FindConfigPath()
	if got != want {
		t.Errorf("FindConfigPath() = %q, want %q", got, want)
	}
}

func TestFindConfigPath_FlagOverridesEnv(t *testing.T) {
	t.Setenv("CONFIG", "/tmp/from-env.json")

	flagPath := "/tmp/from-flag.json"
	old := os.Args
	os.Args = []string{"server", "-c=" + flagPath}
	t.Cleanup(func() { os.Args = old })

	got := config.FindConfigPath()
	if got != flagPath {
		t.Errorf("FindConfigPath() = %q, want %q (flag should override env)", got, flagPath)
	}
}

func TestFindConfigPath_LongFlag(t *testing.T) {
	flagPath := "/tmp/cfg.json"
	old := os.Args
	os.Args = []string{"server", "--config", flagPath}
	t.Cleanup(func() { os.Args = old })

	t.Setenv("CONFIG", "")
	got := config.FindConfigPath()
	if got != flagPath {
		t.Errorf("FindConfigPath() = %q, want %q", got, flagPath)
	}
}

func TestLoadJSONConfig_ServerDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "server.json")
	if err := os.WriteFile(cfgFile, []byte(`{
		"address": "10.0.0.1:9090",
		"store_interval": "30s",
		"store_file": "/tmp/metrics.db",
		"restore": true,
		"database_dsn": "postgres://localhost/metrics",
		"crypto_key": "/tmp/private.pem"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	old := os.Args
	os.Args = []string{"server", "-c=" + cfgFile}
	t.Cleanup(func() { os.Args = old })

	// Clear any env vars that might interfere.
	for _, key := range []string{"ADDRESS", "STORE_INTERVAL", "FILE_STORAGE_PATH",
		"RESTORE", "DATABASE_DSN", "CRYPTO_KEY", "CONFIG"} {
		t.Setenv(key, "")
	}

	cfg, err := config.ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	if cfg.Address != "10.0.0.1:9090" {
		t.Errorf("Address = %q, want %q", cfg.Address, "10.0.0.1:9090")
	}
	if cfg.StoreIntervalSec != 30 {
		t.Errorf("StoreIntervalSec = %d, want 30", cfg.StoreIntervalSec)
	}
	if cfg.FileStoragePath != "/tmp/metrics.db" {
		t.Errorf("FileStoragePath = %q, want %q", cfg.FileStoragePath, "/tmp/metrics.db")
	}
	if !cfg.Restore {
		t.Error("Restore should be true")
	}
	if cfg.DatabaseDSN != "postgres://localhost/metrics" {
		t.Errorf("DatabaseDSN = %q", cfg.DatabaseDSN)
	}
	if cfg.CryptoKey != "/tmp/private.pem" {
		t.Errorf("CryptoKey = %q", cfg.CryptoKey)
	}
}
