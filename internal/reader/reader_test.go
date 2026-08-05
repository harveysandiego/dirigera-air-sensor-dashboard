package reader

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/dirigera"
)

// fakeLister is a test double for sensorLister.
type fakeLister struct {
	devices []dirigera.Device
	err     error
}

func (f fakeLister) ListEnvironmentSensors() (*[]dirigera.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.devices, nil
}

func unmarshalDevices(t *testing.T, jsonStr string) []dirigera.Device {
	t.Helper()
	var devices []dirigera.Device
	if err := json.Unmarshal([]byte(jsonStr), &devices); err != nil {
		t.Fatal(err)
	}
	return devices
}

func TestGetHistoryMissingFile(t *testing.T) {
	errs := make(chan error, 1)
	r := New(nil, nil, nil, errs)

	r.GetHistory(filepath.Join(t.TempDir(), "does-not-exist.json"))

	select {
	case err := <-errs:
		t.Fatalf("unexpected error for a missing history file: %v", err)
	default:
	}

	b, err := json.Marshal(r.History)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("got %s, want an empty history", b)
	}
}

func TestGetHistoryLoadsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"device-1":[{"Name":"Kitchen","Temperature":21}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 1)
	r := New(nil, nil, nil, errs)
	r.GetHistory(path)

	select {
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	default:
	}

	b, err := json.Marshal(r.History)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]struct {
		Name        string
		Temperature int
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 1 || got["device-1"][0].Name != "Kitchen" {
		t.Fatalf("got %+v, want one Kitchen entry", got)
	}
}

func TestRead(t *testing.T) {
	devices := unmarshalDevices(t, `[{"id":"1","deviceType":"environmentSensor","attributes":{"customName":"Kitchen","currentTemperature":21}}]`)

	update := make(chan bool, 1)
	errs := make(chan error, 1)
	r := New(fakeLister{devices: devices}, update, nil, errs)

	r.read()

	select {
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
	select {
	case <-update:
	default:
		t.Fatal("expected update to be signaled after a successful read")
	}

	b, err := json.Marshal(r.History)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string][]struct {
		Name        string
		Temperature int
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["1"]) != 1 || got["1"][0].Name != "Kitchen" || got["1"][0].Temperature != 21 {
		t.Fatalf("got %+v, want one Kitchen/21 entry keyed by device id 1", got)
	}
}

func TestReadError(t *testing.T) {
	boom := errors.New("boom")
	update := make(chan bool, 1)
	errs := make(chan error, 1)
	r := New(fakeLister{err: boom}, update, nil, errs)

	r.read()

	select {
	case err := <-errs:
		if !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	default:
		t.Fatal("expected an error on errs when ListEnvironmentSensors fails")
	}
	select {
	case <-update:
		t.Fatal("update should not fire when the read failed")
	default:
	}
}

func TestStartReadsImmediatelyThenStopsOnDone(t *testing.T) {
	devices := unmarshalDevices(t, `[{"id":"1","deviceType":"environmentSensor","attributes":{"customName":"Kitchen"}}]`)

	update := make(chan bool, 1)
	done := make(chan bool)
	errs := make(chan error, 1)
	r := New(fakeLister{devices: devices}, update, done, errs)

	startReturned := make(chan struct{})
	go func() {
		r.Start()
		close(startReturned)
	}()

	select {
	case <-update:
	case err := <-errs:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Start did not perform an immediate read")
	}

	close(done)

	select {
	case <-startReturned:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after done was closed")
	}
}

func TestGetHistoryOpenPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks are bypassed")
	}

	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 1)
	r := New(nil, nil, nil, errs)
	r.GetHistory(path)

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("got nil error for a permission-denied file")
		}
	default:
		t.Fatal("expected an error on errs for a permission-denied file")
	}
}

func withShortPollInterval(t *testing.T) {
	t.Helper()
	orig := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = orig })
}

func TestStartPollsAgainOnTicker(t *testing.T) {
	withShortPollInterval(t)
	devices := unmarshalDevices(t, `[{"id":"1","deviceType":"environmentSensor","attributes":{"customName":"Kitchen"}}]`)

	// Generous buffer: must never block once we stop draining below.
	update := make(chan bool, 100)
	done := make(chan bool)
	errs := make(chan error, 1)
	r := New(fakeLister{devices: devices}, update, done, errs)

	go r.Start()
	defer close(done)

	// First read is immediate; the second can only come from the ticker firing.
	for i := range 2 {
		select {
		case <-update:
		case err := <-errs:
			t.Fatalf("unexpected error: %v", err)
		case <-time.After(time.Second):
			t.Fatalf("did not receive update #%d", i+1)
		}
	}
}

func TestGetHistoryInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 1)
	r := New(nil, nil, nil, errs)
	r.GetHistory(path)

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("got nil error for invalid JSON")
		}
	default:
		t.Fatal("expected an error on the errs channel for invalid JSON, got none")
	}
}
