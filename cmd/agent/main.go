package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/agent"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/config"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/crypto"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/logger"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

var (
	buildVersion = "N/A"
	buildDate    = "N/A"
	buildCommit  = "N/A"
)

const (
	// drainTimeout is how long workers keep delivering queued metrics after
	// the shutdown signal is received.
	drainTimeout = 30 * time.Second
	// shutdownTimeout is the maximum time to wait for all goroutines to exit
	// before forcing a hard exit.
	shutdownTimeout = 30 * time.Second
)

func main() {
	buildinfo.Print(buildVersion, buildDate, buildCommit)
	if err := run(); err != nil {
		log.Printf("Application error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	log := logger.GetLogger()

	cfg, err := config.ParseAgentFlags()
	if err != nil {
		return err
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
			return fmt.Errorf("load public key %s: %w", cfg.CryptoKey, err)
		}
		sender = agent.NewSender(serverURL, cfg.Key, pk)
		log.Info().Str("key", cfg.CryptoKey).Msg("Asymmetric encryption enabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// drainCtx is used by workers after the main ctx is cancelled so that
	// in-flight and queued metrics can still be delivered during graceful shutdown.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), drainTimeout)
	defer drainCancel()

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

	if cfg.GRPCAddress != "" {
		// gRPC mode: reporter sends metric batches directly via gRPC.
		grpcSender, err := agent.NewGRPCSender(cfg.GRPCAddress)
		if err != nil {
			return fmt.Errorf("create grpc sender: %w", err)
		}
		defer grpcSender.Close()
		log.Info().Str("grpc_address", cfg.GRPCAddress).Msg("gRPC transport enabled")

		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info().Dur("report_interval", reportInterval).Msg("Started gRPC batch reporting")
			ticker := time.NewTicker(reportInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					log.Info().Msg("Stopping gRPC reporting, flushing remaining metrics...")
					flushCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
					defer cancel()
					if err := grpcSender.SendBatch(flushCtx, collector.GetAllMetrics()); err != nil {
						log.Info().Err(err).Msg("Failed to flush metrics via gRPC")
					}
					return
				case <-ticker.C:
					metrics := collector.GetAllMetrics()
					if len(metrics) == 0 {
						continue
					}
					log.Info().Int("count", len(metrics)).Msg("Sending gRPC batch")
					if err := grpcSender.SendBatch(ctx, metrics); err != nil {
						log.Info().Err(err).Msg("Failed to send gRPC batch")
					}
				}
			}
		}()
	} else {
		// HTTP mode: reporter dispatches metrics to the worker pool via jobs channel.
		jobs := make(chan model.Metrics, cfg.RateLimit*2)

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
	case <-time.After(shutdownTimeout):
		log.Info().Msg("Shutdown timeout, forcing exit")
	}

	return nil
}
