package model

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestHistoryAppend(t *testing.T) {
	h := NewHistory()
	h.Append("device-1", SensorData{Name: "Kitchen", Temperature: 21}, 0)
	h.Append("device-1", SensorData{Name: "Kitchen", Temperature: 22}, 0)
	h.Append("device-2", SensorData{Name: "Bedroom", Temperature: 19}, 0)

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string][]SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if len(got["device-1"]) != 2 {
		t.Fatalf("device-1: got %d entries, want 2", len(got["device-1"]))
	}
	if len(got["device-2"]) != 1 {
		t.Fatalf("device-2: got %d entries, want 1", len(got["device-2"]))
	}
	if got["device-1"][1].Temperature != 22 {
		t.Fatalf("device-1[1].Temperature = %d, want 22", got["device-1"][1].Temperature)
	}
}

func TestHistoryAppendTrimsToMaxPoints(t *testing.T) {
	h := NewHistory()
	const maxPoints = 3
	for i := range 5 {
		h.Append("device-1", SensorData{Temperature: i}, maxPoints)
	}

	b, _ := json.Marshal(h)
	var got map[string][]SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	entries := got["device-1"]
	if len(entries) != maxPoints {
		t.Fatalf("got %d entries, want %d", len(entries), maxPoints)
	}
	// The oldest two (Temperature 0, 1) should have been dropped, keeping 2, 3, 4.
	want := []int{2, 3, 4}
	for i, e := range entries {
		if e.Temperature != want[i] {
			t.Fatalf("entries[%d].Temperature = %d, want %d", i, e.Temperature, want[i])
		}
	}
}

func TestHistoryAppendMaxPointsZeroDisablesTrim(t *testing.T) {
	h := NewHistory()
	for i := range 10 {
		h.Append("device-1", SensorData{Temperature: i}, 0)
	}

	b, _ := json.Marshal(h)
	var got map[string][]SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 10 {
		t.Fatalf("got %d entries, want 10", len(got["device-1"]))
	}
}

func TestHistoryLoad(t *testing.T) {
	h := NewHistory()
	input := `{"device-1":[{"Name":"Kitchen","Temperature":21}]}`

	if err := h.Load(json.NewDecoder(bytes.NewBufferString(input))); err != nil {
		t.Fatal(err)
	}

	b, _ := json.Marshal(h)
	var got map[string][]SensorData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["device-1"]) != 1 || got["device-1"][0].Name != "Kitchen" {
		t.Fatalf("got %+v, want one Kitchen entry", got)
	}
}

func TestHistoryLoadInvalidJSON(t *testing.T) {
	h := NewHistory()
	err := h.Load(json.NewDecoder(bytes.NewBufferString("not json")))
	if err == nil {
		t.Fatal("expected an error decoding invalid JSON, got nil")
	}
}

func TestHistoryMarshalJSONOmitsInternalFields(t *testing.T) {
	h := NewHistory()
	h.Append("device-1", SensorData{Name: "Kitchen"}, 0)

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}

	// A default struct marshal would include no keys at all, since mu and
	// data are unexported — confirms MarshalJSON is what actually ran.
	if !bytes.Contains(b, []byte(`"device-1"`)) {
		t.Fatalf("marshaled output missing device-1 key: %s", b)
	}
	if bytes.Contains(b, []byte("mu")) {
		t.Fatalf("marshaled output leaked internal field: %s", b)
	}
}

// TestHistoryConcurrentAccess exercises Append and MarshalJSON from many
// goroutines at once. Run with -race: this is the regression test for the
// concurrent map read/write bug the History type was introduced to fix.
func TestHistoryConcurrentAccess(t *testing.T) {
	h := NewHistory()
	const goroutines = 20
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range perGoroutine {
				h.Append("device", SensorData{
					Timestamp:   time.Now(),
					Temperature: id*perGoroutine + i,
				}, 1000)
			}
		}(g)

		go func() {
			defer wg.Done()
			for range perGoroutine {
				if _, err := json.Marshal(h); err != nil {
					t.Error(err)
				}
			}
		}()
	}

	wg.Wait()
}
