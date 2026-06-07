package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

type Sender struct {
	client  *http.Client
	baseURL string
}

func NewSender(baseURL string) *Sender {
	return &Sender{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

func (s *Sender) SendGauge(name string, value float64) error {
	return s.sendJSON(model.Metrics{
		ID:    name,
		MType: model.TypeGauge,
		Value: &value,
	})
}

func (s *Sender) SendCounter(name string, value int64) error {
	return s.sendJSON(model.Metrics{
		ID:    name,
		MType: model.TypeCounter,
		Delta: &value,
	})
}

func (s *Sender) SendAllMetrics(gauges map[string]float64, counters map[string]int64) error {
	for name, value := range gauges {
		if err := s.SendGauge(name, value); err != nil {
			return fmt.Errorf("failed to send gauge %s: %w", name, err)
		}
	}
	for name, value := range counters {
		if err := s.SendCounter(name, value); err != nil {
			return fmt.Errorf("failed to send counter %s: %w", name, err)
		}
	}
	return nil
}

func (s *Sender) sendJSON(m model.Metrics) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/update", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d for %s/%s", resp.StatusCode, m.MType, m.ID)
	}
	return nil
}

func addHashHeader(req *http.Request, body []byte, key string) {
	if key == "" {
		return
	}
	hash := ComputeHMAC(body, key)
	req.Header.Set("HashSHA256", hash)
}
