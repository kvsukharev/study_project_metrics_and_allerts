package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

type Sender struct {
	client  *http.Client
	baseURL string
	key     string
}

func NewSender(baseURL, key string) *Sender {
	return &Sender{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
		key:     key,
	}
}

func (s *Sender) SendGauge(ctx context.Context, name string, value float64) error {
	return s.sendJSON(ctx, model.Metrics{
		ID:    name,
		MType: model.TypeGauge,
		Value: &value,
	})
}

func (s *Sender) SendCounter(ctx context.Context, name string, value int64) error {
	return s.sendJSON(ctx, model.Metrics{
		ID:    name,
		MType: model.TypeCounter,
		Delta: &value,
	})
}

func (s *Sender) SendAllMetrics(ctx context.Context, gauges map[string]float64, counters map[string]int64) error {
	for name, value := range gauges {
		if err := s.SendGauge(ctx, name, value); err != nil {
			return fmt.Errorf("failed to send gauge %s: %w", name, err)
		}
	}
	for name, value := range counters {
		if err := s.SendCounter(ctx, name, value); err != nil {
			return fmt.Errorf("failed to send counter %s: %w", name, err)
		}
	}
	return nil
}

func (s *Sender) SendBatch(ctx context.Context, metrics []model.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}
	body, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	compressed := Compress(body)
	return retryOnConnErr(ctx, func() error {
		return s.postCompressed("/updates", body, compressed)
	})
}

func (s *Sender) sendJSON(ctx context.Context, m model.Metrics) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	compressed := Compress(body)
	return retryOnConnErr(ctx, func() error {
		return s.postCompressed("/update", body, compressed)
	})
}

// postCompressed отправляет gzip-сжатый JSON на указанный путь.
// body — исходные (несжатые) байты для вычисления хеша.
// compressed — сжатые байты для тела запроса.
func (s *Sender) postCompressed(path string, body, compressed []byte) error {
	req, err := http.NewRequest("POST", s.baseURL+path, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	addHashHeader(req, body, s.key)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func addHashHeader(req *http.Request, body []byte, key string) {
	if key == "" {
		return
	}
	req.Header.Set("HashSHA256", ComputeHMAC(body, key))
}
