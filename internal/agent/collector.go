// Package agent implements the metrics collection and sending logic for the
// agent binary. It reads runtime and system metrics, buffers them, and
// ships them to the server over HTTP.
package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/kvsukharev/go-musthave-metrics-tpl/internal/model"
)

// Collector gathers runtime and gopsutil metrics and makes them available
// for the sender. It is safe for concurrent use from multiple goroutines.
type Collector struct {
	mu        *sync.Mutex
	gauge     map[string]float64
	counter   map[string]int64
	buffer    []model.Metrics
	BatchSize int
	client    *http.Client
	endpoint  string
}

// UpdateMetrics reads the current runtime.MemStats and updates all 28 gauge
// metrics together with the PollCount counter. Safe to call concurrently.
func (c *Collector) UpdateMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.gauge["Alloc"] = float64(m.Alloc)
	c.gauge["BuckHashSys"] = float64(m.BuckHashSys)
	c.gauge["Frees"] = float64(m.Frees)
	c.gauge["GCCPUFraction"] = m.GCCPUFraction
	c.gauge["GCSys"] = float64(m.GCSys)
	c.gauge["HeapAlloc"] = float64(m.HeapAlloc)
	c.gauge["HeapIdle"] = float64(m.HeapIdle)
	c.gauge["HeapInuse"] = float64(m.HeapInuse)
	c.gauge["HeapObjects"] = float64(m.HeapObjects)
	c.gauge["HeapReleased"] = float64(m.HeapReleased)
	c.gauge["HeapSys"] = float64(m.HeapSys)
	c.gauge["LastGC"] = float64(m.LastGC)
	c.gauge["Lookups"] = float64(m.Lookups)
	c.gauge["MCacheInuse"] = float64(m.MCacheInuse)
	c.gauge["MCacheSys"] = float64(m.MCacheSys)
	c.gauge["MSpanInuse"] = float64(m.MSpanInuse)
	c.gauge["MSpanSys"] = float64(m.MSpanSys)
	c.gauge["Mallocs"] = float64(m.Mallocs)
	c.gauge["NextGC"] = float64(m.NextGC)
	c.gauge["NumForcedGC"] = float64(m.NumForcedGC)
	c.gauge["NumGC"] = float64(m.NumGC)
	c.gauge["OtherSys"] = float64(m.OtherSys)
	c.gauge["PauseTotalNs"] = float64(m.PauseTotalNs)
	c.gauge["StackInuse"] = float64(m.StackInuse)
	c.gauge["StackSys"] = float64(m.StackSys)
	c.gauge["Sys"] = float64(m.Sys)
	c.gauge["TotalAlloc"] = float64(m.TotalAlloc)
	c.gauge["RandomValue"] = rand.Float64()

	c.counter["PollCount"]++
}

// NewCollector creates a new Collector with the given batch size, HTTP client
// and server endpoint (e.g. "http://localhost:8080").
func NewCollector(batchSize int, client *http.Client, endpoint string) *Collector {
	return &Collector{
		BatchSize: batchSize,
		mu:        &sync.Mutex{},
		gauge:     make(map[string]float64),
		counter:   make(map[string]int64),
		client:    client,   // Инициализация клиента
		endpoint:  endpoint, // Инициализация адреса сервера
	}
}

// GetGauges returns a copy of all gauge metrics.
func (c *Collector) GetGauges() map[string]float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]float64, len(c.gauge))
	for k, v := range c.gauge {
		result[k] = v
	}
	return result
}

// GetCounters returns a copy of all counter metrics.
func (c *Collector) GetCounters() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]int64, len(c.counter))
	for k, v := range c.counter {
		result[k] = v
	}
	return result
}

// GetAllMetrics returns a snapshot of all gauge and counter metrics in a single
// lock acquisition, preventing a race between the two maps.
func (c *Collector) GetAllMetrics() []model.Metrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics := make([]model.Metrics, 0, len(c.gauge)+len(c.counter))
	for name, value := range c.gauge {
		v := value
		metrics = append(metrics, model.Metrics{
			ID:    name,
			MType: model.TypeGauge,
			Value: &v,
		})
	}
	for name, delta := range c.counter {
		d := delta
		metrics = append(metrics, model.Metrics{
			ID:    name,
			MType: model.TypeCounter,
			Delta: &d,
		})
	}
	return metrics
}

// GetMetricsCount returns the number of gauge and counter metrics currently held.
func (c *Collector) GetMetricsCount() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.gauge), len(c.counter)
}

func (c *Collector) AddMetric(m model.Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buffer = append(c.buffer, m)
	if len(c.buffer) >= c.BatchSize {
		go c.sendBatch()
	}
}

func (c *Collector) sendBatch() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.buffer) == 0 {
		return
	}

	body, err := json.Marshal(c.buffer)
	if err != nil {
		log.Printf("Failed to marshal batch: %v", err)
		return
	}

	compressed := Compress(body)

	req, err := http.NewRequest("POST", c.endpoint+"/updates", bytes.NewReader(compressed))
	if err != nil {
		log.Printf("Failed to create HTTP request: %v", err)
		return
	}

	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Failed to send HTTP request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		c.buffer = c.buffer[:0]
	}

	sendBuffer := make([]model.Metrics, len(c.buffer))
	copy(sendBuffer, c.buffer)

	// Настройка стратегии повторов
	retryBackoff := backoff.WithMaxRetries(
		backoff.NewExponentialBackOff(
			backoff.WithInitialInterval(1*time.Second),
			backoff.WithMaxInterval(5*time.Second),
		),
		3,
	)

	operation := func() error {
		return c.trySendBatch(sendBuffer)
	}

	// Выполнение с повторными попытками
	if err := backoff.Retry(operation, retryBackoff); err == nil {
		c.buffer = c.buffer[:0] // Очищаем только при успехе
	}
}

func (c *Collector) StartFlusher(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			c.mu.Lock()
			if len(c.buffer) > 0 {
				go c.sendBatch()
			}
			c.mu.Unlock()
		}
	}()
}

func (c *Collector) trySendBatch(buffer []model.Metrics) error {
	body, err := json.Marshal(buffer)
	if err != nil {
		return backoff.Permanent(err) // Неповторяемая ошибка
	}

	compressed := Compress(body)
	req, err := http.NewRequest("POST", c.endpoint+"/updates", bytes.NewReader(compressed))
	if err != nil {
		return backoff.Permanent(err)
	}

	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Повторяем для временных ошибок сети
		if isRetriableError(err) {
			return err
		}
		return backoff.Permanent(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}
	return nil
}

func isRetriableError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
