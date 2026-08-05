package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/dirigera"
)

type fakeDiscoverer struct {
	results []error
	calls   int
}

func (f *fakeDiscoverer) Discover() error {
	err := f.results[f.calls]
	f.calls++
	return err
}

func withShortBackoff(t *testing.T) {
	t.Helper()
	orig := discoverRetryBackoff
	discoverRetryBackoff = time.Millisecond
	t.Cleanup(func() { discoverRetryBackoff = orig })
}

func TestDiscoverWithRetrySucceedsAfterTimeouts(t *testing.T) {
	withShortBackoff(t)
	d := &fakeDiscoverer{results: []error{dirigera.ErrHubTimeout, dirigera.ErrHubTimeout, nil}}

	if err := discoverWithRetry(d, 5); err != nil {
		t.Fatal(err)
	}
	if d.calls != 3 {
		t.Fatalf("got %d calls, want 3", d.calls)
	}
}

func TestDiscoverWithRetryReturnsFatalErrorImmediately(t *testing.T) {
	boom := errors.New("boom")
	d := &fakeDiscoverer{results: []error{boom}}

	err := discoverWithRetry(d, 5)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
	if d.calls != 1 {
		t.Fatalf("got %d calls, want 1 (should not retry a non-timeout error)", d.calls)
	}
}

func TestDiscoverWithRetryExhaustsRetries(t *testing.T) {
	withShortBackoff(t)
	d := &fakeDiscoverer{results: []error{dirigera.ErrHubTimeout, dirigera.ErrHubTimeout, dirigera.ErrHubTimeout}}

	err := discoverWithRetry(d, 3)
	if !errors.Is(err, dirigera.ErrHubTimeout) {
		t.Fatalf("got %v, want ErrHubTimeout", err)
	}
	if d.calls != 3 {
		t.Fatalf("got %d calls, want 3", d.calls)
	}
}

func TestWaitForShutdownOnSignal(t *testing.T) {
	sig := make(chan os.Signal, 1)
	errs := make(chan error, 1)
	done := make(chan bool, 1)
	sig <- os.Interrupt

	code := waitForShutdown(sig, errs, done)

	if code != 0 {
		t.Fatalf("got exit code %d, want 0", code)
	}
	select {
	case <-done:
	default:
		t.Fatal("expected done to be signaled")
	}
}

func TestWaitForShutdownOnError(t *testing.T) {
	sig := make(chan os.Signal, 1)
	errs := make(chan error, 1)
	done := make(chan bool, 1)
	errs <- errors.New("server exploded")

	code := waitForShutdown(sig, errs, done)

	if code != 1 {
		t.Fatalf("got exit code %d, want 1", code)
	}
	select {
	case <-done:
	default:
		t.Fatal("expected done to be signaled")
	}
}
