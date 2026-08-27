// Package model defines the core data types shared between the agent and the server.
package model

// MetricType identifies the kind of a metric: counter or gauge.
type MetricType string

const (
	// TypeCounter is an ever-increasing integer metric (e.g. PollCount).
	TypeCounter MetricType = "counter"
	// TypeGauge is a floating-point snapshot metric (e.g. Alloc, HeapSys).
	TypeGauge MetricType = "gauge"
)

// Metrics is the universal DTO used by both the REST API and the storage layer.
// Delta and Value are pointers so that a zero value can be distinguished from
// an absent value and omitted from JSON output.
type Metrics struct {
	ID    string     `json:"id"`
	MType MetricType `json:"type"`
	Delta *int64     `json:"delta,omitempty"`
	Value *float64   `json:"value,omitempty"`
	Hash  string     `json:"hash,omitempty"`
}
