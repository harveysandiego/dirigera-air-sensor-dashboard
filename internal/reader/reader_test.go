package reader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
