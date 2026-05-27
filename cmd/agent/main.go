package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/logger"
)

// RootConfig – верхний уровень с вложенным agent_config
type RootConfig struct {
	AgentConfig AgentConfig `yaml:"agent_config"`
}

// AgentConfig с тегами yaml и env
type AgentConfig struct {
	serverAddress string        `yaml:"server_adress" env:"ADDRESS"` // Обращаем внимание: env тег использует точное имя переменной
	PollInterval   time.Duration `yaml:"poll_interval"`               // интервал в time.Duration, парсим отдельно
	ReportInterval time.Duration `yaml:"report_interval"`             // как выше
}

const (
	defaultPollInterval   = 2 * time.Second
	defaultReportInterval = 10 * time.Second
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
		Str("server_address", cfg.serverAddress).
		Dur("poll_interval", cfg.PollInterval).
		Dur("report_interval", cfg.ReportInterval).
		Msg("Starting metrics agent")

	serverURL := cfg.serverAddress
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	collector := agent.NewCollector(100, client, serverURL)
	sender := agent.NewSender(serverURL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Dur("Started metrics collection with interval: %v", cfg.PollInterval)
		ticker := time.NewTicker(cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping metrics collection...")
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
		log.Info().Msgf("Started metrics reporting with interval: %v", cfg.ReportInterval)
		ticker := time.NewTicker(cfg.ReportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping metrics reporting...")
				return
			case <-ticker.C:
				gauges := collector.GetGauges()
				counters := collector.GetCounters()
				if len(gauges) == 0 && len(counters) == 0 {
					log.Info().Msg("No metrics to send")
					continue
				}
				log.Info().Str("Sending metrics to %s", serverURL)
				if err := sender.SendAllMetrics(gauges, counters); err != nil {
					log.Info().Msgf("Failed to send metrics: %v", err)
				} else {
					log.Info().Msg("Successfully sent all metrics")
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
			serverAddress: defaultServerAddress,
			PollInterval:   defaultPollInterval,
			ReportInterval: defaultReportInterval,
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Config file %q not found, using defaults and env variables", path)
	} else {
		if err := yaml.Unmarshal(data, rootCfg); err != nil {
			return nil, fmt.Errorf("unmarshal yaml: %w", err)
		}
	}

	return &rootCfg.AgentConfig, nil
}

// applyEnv проверяет переменные окружения и если они есть — перекрывает параметры
// parseFlags применяет флаги. Приоритет: env var > флаг > дефолт.
func parseFlags(cfg *AgentConfig) error {
	var (
		flagAddress        string
		flagPollInterval   int
		flagReportInterval int
	)

	flag.StringVar(&flagAddress, "a", defaultServerAddress, "HTTP server endpoint address")
	flag.IntVar(&flagPollInterval, "p", int(defaultPollInterval/time.Second), "Poll interval in seconds")
	flag.IntVar(&flagReportInterval, "r", int(defaultReportInterval/time.Second), "Report interval in seconds")

	flag.Parse()

	// env > флаг > дефолт
	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		cfg.serverAddress = envAddr
	} else {
		cfg.serverAddress = flagAddress
	}

	if envPoll := os.Getenv("POLL_INTERVAL"); envPoll != "" {
		sec, err := strconv.Atoi(envPoll)
		if err != nil {
			return fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = time.Duration(sec) * time.Second
	} else {
		cfg.PollInterval = time.Duration(flagPollInterval) * time.Second
	}

	if envReport := os.Getenv("REPORT_INTERVAL"); envReport != "" {
		sec, err := strconv.Atoi(envReport)
		if err != nil {
			return fmt.Errorf("invalid REPORT_INTERVAL: %w", err)
		}
		cfg.ReportInterval = time.Duration(sec) * time.Second
	} else {
		cfg.ReportInterval = time.Duration(flagReportInterval) * time.Second
	}

	return nil
}
