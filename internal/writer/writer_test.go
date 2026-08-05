package writer

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/model"
)

func TestWrite(t *testing.T) {
	history := model.NewHistory()
	history.Append("device-1", model.SensorData{Name: "Kitchen", Temperature: 21}, 0)

	w := New(nil, history, nil, nil)
	dataFile := filepath.Join(t.TempDir(), "data.json")

	if err := w.write(dataFile); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]model.SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 1 || got["device-1"][0].Name != "Kitchen" {
		t.Fatalf("got %+v, want one Kitchen entry", got)
	}
}

func TestWriteCreateFails(t *testing.T) {
	history := model.NewHistory()
	w := New(nil, history, nil, nil)
	dataFile := filepath.Join(t.TempDir(), "no-such-dir", "data.json")

	if err := w.write(dataFile); err == nil {
		t.Fatal("expected an error writing to a nonexistent directory, got nil")
	}
}

// TestStartPropagatesWriteError checks that a failing write is sent to errs
// and stops Start's loop, rather than being silently swallowed.
func TestStartPropagatesWriteError(t *testing.T) {
	history := model.NewHistory()
	update := make(chan bool)
	done := make(chan bool)
	errs := make(chan error, 1)

	w := New(update, history, done, errs)
	dataFile := filepath.Join(t.TempDir(), "no-such-dir", "data.json")
	go w.Start(dataFile)

	update <- true

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("got nil error on errs")
		}
	case <-time.After(time.Second):
		t.Fatal("expected a write failure to be sent to errs")
	}
}

// TestStartWritesOnUpdate exercises Start's goroutine loop rather than write
// directly. Sending on done only rendezvous once Start has looped back to
// its select statement, which can only happen after the prior update case's
// write call has already returned — so a successful done send proves the
// write completed, with no sleep or polling required.
func TestStartWritesOnUpdate(t *testing.T) {
	history := model.NewHistory()
	history.Append("device-1", model.SensorData{Name: "Kitchen", Temperature: 21}, 0)

	update := make(chan bool)
	done := make(chan bool)
	errs := make(chan error, 1)

	w := New(update, history, done, errs)
	dataFile := filepath.Join(t.TempDir(), "data.json")
	go w.Start(dataFile)

	update <- true
	select {
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	case done <- true:
	}

	b, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]model.SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 1 || got["device-1"][0].Name != "Kitchen" {
		t.Fatalf("got %+v, want one Kitchen entry", got)
	}
}

func TestServeData(t *testing.T) {
	history := model.NewHistory()
	history.Append("device-1", model.SensorData{Name: "Kitchen", Temperature: 21}, 0)

	w := New(nil, history, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/data", nil)
	w.ServeData(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var got map[string][]model.SensorData
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 1 || got["device-1"][0].Name != "Kitchen" {
		t.Fatalf("got %+v, want one Kitchen entry", got)
	}
}
