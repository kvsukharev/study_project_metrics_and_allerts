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
)

// RootConfig – верхний уровень с вложенным agent_config
type RootConfig struct {
	AgentConfig AgentConfig `yaml:"agent_config"`
}

type AgentConfig struct {
	Address        string `env:"ADDRESS"`
	PollInterval   int    `env:"POLL_INTERVAL"`   // seconds
	ReportInterval int    `env:"REPORT_INTERVAL"` // seconds
}

const (
	defaultPollInterval   = 2
	defaultReportInterval = 10
	defaultServerAddress  = "localhost:8080"
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

	log.Info().
		Str("address", cfg.Address).
		Int("poll_interval", cfg.PollInterval).
		Int("report_interval", cfg.ReportInterval).
		Msg("Starting metrics agent")

	serverURL := cfg.Address
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	pollInterval := time.Duration(cfg.PollInterval) * time.Second
	reportInterval := time.Duration(cfg.ReportInterval) * time.Second

	client := &http.Client{Timeout: 10 * time.Second}
	collector := agent.NewCollector(100, client, serverURL)
	sender := agent.NewSender(serverURL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

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
				gCount, cCount := collector.GetMetricsCount()
				log.Info().Msgf("Collected metrics: %d gauges, %d counters", gCount, cCount)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
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
				log.Info().Str("server_url", serverURL).Int("count", len(metrics)).Msg("Sending metrics batch")
				if err := sender.SendBatch(ctx, metrics); err != nil {
					log.Info().Msgf("Failed to send metrics batch: %v", err)
				} else {
					log.Info().Msg("Successfully sent metrics batch")
				}
			}
		}
	}()

	log.Info().Msg("Agent is running. Press Ctrl+C to stop.")

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

// loadConfig читает YAML, задаёт дефолты, парсит в структуру
func loadConfig(path string) (*AgentConfig, error) {
	rootCfg := &RootConfig{
		AgentConfig: AgentConfig{
			Address:        defaultServerAddress,
			PollInterval:   defaultPollInterval,
			ReportInterval: defaultReportInterval,
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

// parseFlags применяет флаги и env vars. Приоритет: env var > флаг > дефолт.
func parseFlags(cfg *AgentConfig) error {
	flag.StringVar(&cfg.Address, "a", defaultServerAddress, "HTTP server endpoint address")
	flag.IntVar(&cfg.PollInterval, "p", defaultPollInterval, "Poll interval in seconds")
	flag.IntVar(&cfg.ReportInterval, "r", defaultReportInterval, "Report interval in seconds")

	flag.Parse()

	// env.Parse перезапишет значения из env vars, давая приоритет env > флаг
	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("parse env: %w", err)
	}

	return nil
}
