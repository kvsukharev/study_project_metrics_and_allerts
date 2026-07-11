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

	"github.com/caarlos0/env/v6"
	"gopkg.in/yaml.v3"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
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
}

const (
	defaultPollInterval   = 2
	defaultReportInterval = 10
	defaultServerAddress  = "localhost:8080"
	defaultRateLimit      = 5
	configPath            = "internal/config/agent.yaml"
)

func main() {
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

	if err := parseFlags(cfg); err != nil {
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
	sender := agent.NewSender(serverURL, cfg.Key)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
				log.Info().Msg("Stopping metrics reporting...")
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

	// Worker pool: cfg.RateLimit параллельных горутин-отправителей
	for i := 0; i < cfg.RateLimit; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case m, ok := <-jobs:
					if !ok {
						return
					}
					if err := sender.SendMetric(ctx, m); err != nil {
						log.Info().Err(err).Str("metric", m.ID).Msg("Failed to send metric")
					}
				case <-ctx.Done():
					// вычитываем оставшиеся задания до закрытия канала
					for m := range jobs {
						_ = m
					}
					return
				}
			}
		}(i)
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
	case <-time.After(5 * time.Second):
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

func parseFlags(cfg *AgentConfig) error {
	flag.StringVar(&cfg.Address, "a", defaultServerAddress, "HTTP server endpoint address")
	flag.IntVar(&cfg.PollInterval, "p", defaultPollInterval, "Poll interval in seconds")
	flag.IntVar(&cfg.ReportInterval, "r", defaultReportInterval, "Report interval in seconds")
	flag.StringVar(&cfg.Key, "k", "", "Secret key for HMAC SHA256 signing")
	flag.IntVar(&cfg.RateLimit, "l", defaultRateLimit, "Number of concurrent outgoing requests")

	flag.Parse()

	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("parse env: %w", err)
	}

	return nil
}
