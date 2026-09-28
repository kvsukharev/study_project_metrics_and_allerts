package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"encoding/json"

	"dario.cat/mergo"
	"github.com/caarlos0/env/v6"
	"gopkg.in/yaml.v3"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/crypto"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/logger"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

type RootConfig struct {
	AgentConfig AgentConfig `yaml:"agent_config"`
}

type AgentConfig struct {
	Address        string `env:"ADDRESS"`
	PollInterval   int    `env:"POLL_INTERVAL"`   // seconds
	ReportInterval int    `env:"REPORT_INTERVAL"` // seconds
	Key            string `env:"KEY"`
	RateLimit      int    `env:"RATE_LIMIT"`
	CryptoKey      string `env:"CRYPTO_KEY"`
}

var (
	buildVersion = "N/A"
	buildDate    = "N/A"
	buildCommit  = "N/A"
)

const (
	defaultPollInterval   = 2
	defaultReportInterval = 10
	defaultServerAddress  = "localhost:8080"
	defaultRateLimit      = 5
	configPath            = "internal/config/agent.yaml"
)

// agentJSONConfig mirrors the JSON config file format for the agent.
// Interval fields accept Go duration strings (e.g. "1s", "10s").
type agentJSONConfig struct {
	Address        string `json:"address"`
	ReportInterval string `json:"report_interval"`
	PollInterval   string `json:"poll_interval"`
	CryptoKey      string `json:"crypto_key"`
}

func loadAgentJSONConfig(path string) (*agentJSONConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent config file %s: %w", path, err)
	}
	jcfg := &agentJSONConfig{}
	if err := json.Unmarshal(data, jcfg); err != nil {
		return nil, fmt.Errorf("parse agent config file %s: %w", path, err)
	}
	return jcfg, nil
}

// agentConfigFromJSON converts parsed JSON config into a typed AgentConfig.
// Only fields present in the JSON file are non-zero; mergo fills the rest
// from agentDefaults.
func agentConfigFromJSON(jcfg *agentJSONConfig) (AgentConfig, error) {
	cfg := AgentConfig{
		Address:   jcfg.Address,
		CryptoKey: jcfg.CryptoKey,
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

var agentDefaults = AgentConfig{
	Address:        defaultServerAddress,
	PollInterval:   defaultPollInterval,
	ReportInterval: defaultReportInterval,
	RateLimit:      defaultRateLimit,
}

func main() {
	buildinfo.Print(buildVersion, buildDate, buildCommit)
	if err := run(); err != nil {
		log.Printf("Application error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	log := logger.GetLogger()

	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	jcfg := &agentJSONConfig{}
	if cfgPath := config.FindConfigPath(); cfgPath != "" {
		loaded, err := loadAgentJSONConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("load agent json config: %w", err)
		}
		jcfg = loaded
	}

	if err := parseFlags(cfg, jcfg); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if cfg.RateLimit <= 0 {
		cfg.RateLimit = defaultRateLimit
	}

	log.Info().
		Str("address", cfg.Address).
		Int("poll_interval", cfg.PollInterval).
		Int("report_interval", cfg.ReportInterval).
		Int("rate_limit", cfg.RateLimit).
		Msg("Starting metrics agent")

	serverURL := cfg.Address
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	pollInterval := time.Duration(cfg.PollInterval) * time.Second
	reportInterval := time.Duration(cfg.ReportInterval) * time.Second

	client := &http.Client{Timeout: 10 * time.Second}
	collector := agent.NewCollector(100, client, serverURL)

	sender := agent.NewSender(serverURL, cfg.Key, nil)
	if cfg.CryptoKey != "" {
		pk, err := crypto.LoadPublicKey(cfg.CryptoKey)
		if err != nil {
			return fmt.Errorf("load public key: %w", err)
		}
		sender = agent.NewSender(serverURL, cfg.Key, pk)
		log.Info().Str("key", cfg.CryptoKey).Msg("Asymmetric encryption enabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// drainCtx is used by workers after the main ctx is cancelled so that
	// in-flight and queued metrics can still be delivered during graceful shutdown.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer drainCancel()

	// jobs — канал заданий для worker pool
	jobs := make(chan model.Metrics, cfg.RateLimit*2)

	var wg sync.WaitGroup

	// Горутина 1: сбор runtime-метрик
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Dur("poll_interval", pollInterval).Msg("Started metrics collection")
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping metrics collection...")
				return
			case <-ticker.C:
				collector.UpdateMetrics()
			}
		}
	}()

	// Горутина 2: сбор gopsutil-метрик (TotalMemory, FreeMemory, CPUutilizationN)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Dur("poll_interval", pollInterval).Msg("Started gopsutil collection")
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping gopsutil collection...")
				return
			case <-ticker.C:
				collector.UpdateGopsutilMetrics()
			}
		}
	}()

	// Горутина 3: reporter — снимает снапшот метрик и кладёт задания в канал
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(jobs)
		log.Info().Dur("report_interval", reportInterval).Msg("Started metrics reporting")
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping metrics reporting, flushing remaining metrics...")
				// Final collection: push accumulated metrics into jobs.
				// Use select so the write is interrupted if drainCtx expires
				// before all items are enqueued (prevents goroutine leak).
				for _, m := range collector.GetAllMetrics() {
					select {
					case jobs <- m:
					case <-drainCtx.Done():
						return
					}
				}
				return
			case <-ticker.C:
				metrics := collector.GetAllMetrics()
				if len(metrics) == 0 {
					log.Info().Msg("No metrics to send")
					continue
				}
				log.Info().Int("count", len(metrics)).Msg("Dispatching metrics to workers")
				for _, m := range metrics {
					select {
					case jobs <- m:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	// Worker pool: cfg.RateLimit параллельных горутин-отправителей.
	// During normal operation they use ctx; after cancellation they switch to
	// drainCtx so queued and in-flight metrics are still delivered gracefully.
	for i := 0; i < cfg.RateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				sendCtx := ctx
				if ctx.Err() != nil {
					sendCtx = drainCtx
				}
				if err := sender.SendMetric(sendCtx, m); err != nil {
					log.Info().Err(err).Str("metric", m.ID).Msg("Failed to send metric")
				}
			}
		}()
	}

	log.Info().Int("workers", cfg.RateLimit).Msg("Agent is running. Press Ctrl+C to stop.")

	<-ctx.Done()
	log.Info().Msg("Received shutdown signal...")

	stop()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("Agent stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Info().Msg("Shutdown timeout, forcing exit")
	}

	return nil
}

func loadConfig(path string) (*AgentConfig, error) {
	rootCfg := &RootConfig{
		AgentConfig: AgentConfig{
			Address:        defaultServerAddress,
			PollInterval:   defaultPollInterval,
			ReportInterval: defaultReportInterval,
			RateLimit:      defaultRateLimit,
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Config file %q not found, using defaults", path)
	} else {
		if err := yaml.Unmarshal(data, rootCfg); err != nil {
			return nil, fmt.Errorf("unmarshal yaml: %w", err)
		}
	}

	return &rootCfg.AgentConfig, nil
}

func parseFlags(cfg *AgentConfig, jcfg *agentJSONConfig) error {
	fromJSON, err := agentConfigFromJSON(jcfg)
	if err != nil {
		return err
	}
	if err := mergo.Merge(&fromJSON, agentDefaults); err != nil {
		return fmt.Errorf("merge config defaults: %w", err)
	}

	flag.StringVar(&cfg.Address, "a", fromJSON.Address, "HTTP server endpoint address")
	flag.IntVar(&cfg.PollInterval, "p", fromJSON.PollInterval, "Poll interval in seconds")
	flag.IntVar(&cfg.ReportInterval, "r", fromJSON.ReportInterval, "Report interval in seconds")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC SHA256 signing")
	flag.IntVar(&cfg.RateLimit, "l", fromJSON.RateLimit, "Number of concurrent outgoing requests")
	flag.StringVar(&cfg.CryptoKey, "crypto-key", fromJSON.CryptoKey, "Path to RSA public key for encrypting requests (empty = disabled)")
	flag.String("c", "", "Path to JSON config file")
	flag.String("config", "", "Path to JSON config file")

	flag.Parse()

	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("parse env: %w", err)
	}

	return nil
}
