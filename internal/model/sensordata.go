package model

import (
	"encoding/json"
	"sync"
	"time"
)

type SensorData struct {
	Name        string
	Timestamp   time.Time
	Temperature int
	RH          int
	PM25        int
	VocIndex    int
}

// History is a concurrency-safe store of sensor readings keyed by device ID.
// It is shared between the reader goroutine (which appends) and the writer
// goroutine and HTTP handler (which read it), so all access goes through mu.
type History struct {
	mu   sync.RWMutex
	data map[string][]SensorData
}

func NewHistory() *History {
	return &History{data: make(map[string][]SensorData)}
}

// Append adds entry for id, dropping the oldest entries once maxPoints is exceeded.
// A maxPoints of 0 or less disables trimming.
func (h *History) Append(id string, entry SensorData, maxPoints int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entries := append(h.data[id], entry)
	if maxPoints > 0 && len(entries) > maxPoints {
		entries = entries[len(entries)-maxPoints:]
	}
	h.data[id] = entries
}

// Load replaces the history's contents with data decoded from dec.
func (h *History) Load(dec *json.Decoder) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return dec.Decode(&h.data)
}

// MarshalJSON allows History to be encoded directly, e.g. via
// json.NewEncoder(w).Encode(history).
func (h *History) MarshalJSON() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return json.Marshal(h.data)
}
