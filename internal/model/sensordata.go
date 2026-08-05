// Package model holds the sensor reading types shared between reader and writer, including History.
package model

import (
	"encoding/json"
	"sync"
	"time"
)

// SensorData is a single sensor reading at a point in time.
type SensorData struct {
	Name        string
	Timestamp   time.Time
	Temperature int
	RH          int
	PM25        int
	VocIndex    int
}

// History is a concurrency-safe store of sensor readings keyed by device ID.
type History struct {
	mu   sync.RWMutex
	data map[string][]SensorData
}

// NewHistory returns an empty History, ready to use.
func NewHistory() *History {
	return &History{data: make(map[string][]SensorData)}
}

// Append adds entry for id, dropping the oldest once maxPoints is exceeded (0 or less disables trimming).
func (h *History) Append(id string, entry SensorData, maxPoints int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	//nolint:gocritic // intentional: entries is trimmed before being written back to h.data[id]
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

// MarshalJSON allows History to be encoded directly via json.Marshal/Encode.
func (h *History) MarshalJSON() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return json.Marshal(h.data)
}
