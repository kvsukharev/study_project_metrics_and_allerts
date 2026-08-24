package audit

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
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

// Subject holds observers and broadcasts events to all of them.
type Subject struct {
	observers []Observer
}

// NewSubject creates a Subject with the given observers.
func NewSubject(obs ...Observer) *Subject {
	return &Subject{observers: obs}
}

// Notify sends the event to every registered observer.
func (s *Subject) Notify(e Event) {
	for _, o := range s.observers {
		o.Notify(e)
	}
}

// FileObserver appends JSON audit events to a file, one per line.
type FileObserver struct {
	path string
}

// NewFileObserver creates a FileObserver that writes to path.
func NewFileObserver(path string) *FileObserver {
	return &FileObserver{path: path}
}

func (o *FileObserver) Notify(e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("audit file observer: marshal: %v", err)
		return
	}

	f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("audit file observer: open %s: %v", o.path, err)
		return
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
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
