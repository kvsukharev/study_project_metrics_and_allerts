package config

import (
	"flag"
	"fmt"
	"time"

	"dario.cat/mergo"
	"github.com/caarlos0/env/v6"
)

// AgentConfig holds all tunable parameters for the metrics agent.
type AgentConfig struct {
	Address        string `env:"ADDRESS"`
	PollInterval   int    `env:"POLL_INTERVAL"`   // seconds
	ReportInterval int    `env:"REPORT_INTERVAL"` // seconds
	Key            string `env:"KEY"`
	RateLimit      int    `env:"RATE_LIMIT"`
	CryptoKey      string `env:"CRYPTO_KEY"`
	GRPCAddress    string `env:"GRPC_ADDRESS"` // gRPC server address, e.g. "localhost:3200"; empty = disabled
}

// agentJSONConfig mirrors the JSON config file format for the agent.
// Interval fields accept Go duration strings (e.g. "1s", "10s").
type agentJSONConfig struct {
	Address        string `json:"address"`
	ReportInterval string `json:"report_interval"`
	PollInterval   string `json:"poll_interval"`
	CryptoKey      string `json:"crypto_key"`
	GRPCAddress    string `json:"grpc_address"`
}

// agentDefaults holds the built-in default values for the agent.
var agentDefaults = AgentConfig{
	Address:        "localhost:8080",
	PollInterval:   2,
	ReportInterval: 10,
	RateLimit:      5,
}

// agentConfigFromJSON converts a parsed JSON config into the typed AgentConfig.
// Only fields present in the JSON file are non-zero in the result;
// mergo will fill the rest from agentDefaults.
func agentConfigFromJSON(jcfg *agentJSONConfig) (AgentConfig, error) {
	cfg := AgentConfig{
		Address:     jcfg.Address,
		CryptoKey:   jcfg.CryptoKey,
		GRPCAddress: jcfg.GRPCAddress,
	}
	if jcfg.PollInterval != "" {
		d, err := time.ParseDuration(jcfg.PollInterval)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("poll_interval: invalid duration %q: %w", jcfg.PollInterval, err)
		}
		cfg.PollInterval = int(d.Seconds())
	}
	if jcfg.ReportInterval != "" {
		d, err := time.ParseDuration(jcfg.ReportInterval)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("report_interval: invalid duration %q: %w", jcfg.ReportInterval, err)
		}
		cfg.ReportInterval = int(d.Seconds())
	}
	return cfg, nil
}

// ParseAgentFlags parses CLI flags and overlays environment variables.
// If a JSON config file is found via -c/-config/CONFIG, its values are merged
// with built-in defaults using mergo.
// Priority (highest → lowest): env vars > CLI flags > JSON file > built-in defaults.
func ParseAgentFlags() (*AgentConfig, error) {
	// Step 1: load JSON config (lowest priority).
	jcfg := &agentJSONConfig{}
	if cfgPath := FindConfigPath(); cfgPath != "" {
		if err := loadJSONFile(cfgPath, jcfg); err != nil {
			return nil, err
		}
	}

	// Step 2: convert JSON → typed config (errors on bad values, e.g. invalid duration).
	fromJSON, err := agentConfigFromJSON(jcfg)
	if err != nil {
		return nil, err
	}

	// Step 3: fill zero fields with built-in defaults.
	if err := mergo.Merge(&fromJSON, agentDefaults); err != nil {
		return nil, fmt.Errorf("merge config defaults: %w", err)
	}

	// Step 4: register flags with the merged defaults.
	// Explicit flags override the merged defaults; env vars (step 5) override flags.
	cfg := &AgentConfig{}
	flag.StringVar(&cfg.Address, "a", fromJSON.Address, "HTTP server endpoint address")
	flag.IntVar(&cfg.PollInterval, "p", fromJSON.PollInterval, "Poll interval in seconds")
	flag.IntVar(&cfg.ReportInterval, "r", fromJSON.ReportInterval, "Report interval in seconds")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC SHA256 signing")
	flag.IntVar(&cfg.RateLimit, "l", fromJSON.RateLimit, "Number of concurrent outgoing requests")
	flag.StringVar(&cfg.CryptoKey, "crypto-key", fromJSON.CryptoKey, "Path to RSA public key for encrypting requests (empty = disabled)")
	flag.StringVar(&cfg.GRPCAddress, "grpc", fromJSON.GRPCAddress, "gRPC server address (empty = use HTTP)")
	flag.String("c", "", "Path to JSON config file")
	flag.String("config", "", "Path to JSON config file")
	flag.Parse()

	// Step 5: env vars override flags.
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if cfg.RateLimit <= 0 {
		cfg.RateLimit = agentDefaults.RateLimit
	}

	return cfg, nil
}
