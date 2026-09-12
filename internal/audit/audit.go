package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Event is a single audit log entry emitted after a successful metric update.
type Event struct {
	TS        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

// Observer receives audit events.
type Observer interface {
	Notify(e Event)
}

const defaultAuditBufSize = 256

// Subject dispatches audit events to all registered observers asynchronously.
// Events are queued into a buffered channel and delivered by a single background
// goroutine, so Notify never blocks the caller even if an observer is slow.
// Call Close to drain the queue and stop the goroutine on shutdown.
type Subject struct {
	observers []Observer
	ch        chan Event
	wg        sync.WaitGroup
}

// NewSubject creates a Subject and starts its background dispatch goroutine.
// The internal queue holds up to defaultAuditBufSize events; if it is full,
// Notify drops the event and logs a warning instead of blocking.
func NewSubject(obs ...Observer) *Subject {
	s := &Subject{
		observers: obs,
		ch:        make(chan Event, defaultAuditBufSize),
	}
	s.wg.Add(1)
	go s.dispatch()
	return s
}

// dispatch is the background worker that delivers events to all observers.
func (s *Subject) dispatch() {
	defer s.wg.Done()
	for e := range s.ch {
		for _, o := range s.observers {
			o.Notify(e)
		}
	}
}

// Notify enqueues the event for async delivery. If the queue is full the event
// is dropped and a warning is logged; the handler is never blocked.
func (s *Subject) Notify(e Event) {
	select {
	case s.ch <- e:
	default:
		log.Printf("audit: queue full, dropping event for %v", e.Metrics)
	}
}

// Close drains the event queue and waits for the dispatch goroutine to finish.
// Must be called on server shutdown to ensure all in-flight events are delivered.
func (s *Subject) Close() {
	close(s.ch)
	s.wg.Wait()
}

// FileObserver appends JSON audit events to a file, one per line.
// The file is opened once at construction and kept open for the lifetime of
// the observer. Call Close when the observer is no longer needed.
type FileObserver struct {
	mu  sync.Mutex
	enc *json.Encoder
	f   *os.File
}

// NewFileObserver opens path for appending and returns a ready FileObserver.
// The file is created if it does not exist. Returns an error immediately if
// the file cannot be opened, so misconfiguration is visible at startup.
func NewFileObserver(path string) (*FileObserver, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("audit file observer: open %s: %w", path, err)
	}
	return &FileObserver{f: f, enc: json.NewEncoder(f)}, nil
}

// Close releases the underlying file handle.
func (o *FileObserver) Close() error {
	return o.f.Close()
}

// Notify appends the event as a JSON line to the configured file.
// Errors are logged but not propagated.
func (o *FileObserver) Notify(e Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.enc.Encode(e); err != nil {
		log.Printf("audit file observer: write: %v", err)
	}
}

// URLObserver sends audit events via HTTP POST to a remote server.
type URLObserver struct {
	url    string
	client *http.Client
}

// NewURLObserver creates a URLObserver that POSTs to url.
func NewURLObserver(url string) *URLObserver {
	return &URLObserver{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Notify POSTs the event as JSON to the configured URL.
// Errors are logged but not propagated.
func (o *URLObserver) Notify(e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("audit url observer: marshal: %v", err)
		return
	}

	resp, err := o.client.Post(o.url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("audit url observer: post %s: %v", o.url, err)
		return
	}
	resp.Body.Close()
}
