package dirigera

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateCodeVerifier(t *testing.T) {
	v := createCodeVerifier()
	if len(v) != 128 {
		t.Fatalf("got length %d, want 128", len(v))
	}

	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	for _, c := range v {
		if !strings.ContainsRune(charset, c) {
			t.Fatalf("verifier contains character %q outside the allowed charset", c)
		}
	}

	a, b := createCodeVerifier(), createCodeVerifier()
	if a == b {
		t.Fatal("two calls produced the same verifier, expected randomness")
	}
}

// TestCreateCodeChallenge checks against RFC 7636 Appendix B's own worked
// example, rather than re-deriving the hash the same way the code does.
func TestCreateCodeChallenge(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got := createCodeChallenge(verifier)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNewNoConfigFile(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("got nil Dirigera")
	}
	if d.AuthToken != "" {
		t.Fatalf("AuthToken = %q, want empty", d.AuthToken)
	}
}

func TestNewLoadsExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"Interface":"eth0","AuthToken":"tok-123"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Interface != "eth0" {
		t.Fatalf("Interface = %q, want eth0", d.Interface)
	}
	if d.AuthToken != "tok-123" {
		t.Fatalf("AuthToken = %q, want tok-123", d.AuthToken)
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	d := &Dirigera{configFile: path, Interface: "eth0", AuthToken: "tok-123"}

	if err := d.saveConfig(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Interface != "eth0" || reloaded.AuthToken != "tok-123" {
		t.Fatalf("got %+v, want Interface=eth0 AuthToken=tok-123", reloaded)
	}
}

func TestGenerateToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authResponse{AccessToken: "hub-token-abc"})
	}))
	defer srv.Close()

	d := Dirigera{hubUrl: srv.URL}
	token, err := d.generateToken("some-code", "some-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if token != "hub-token-abc" {
		t.Fatalf("got token %q, want hub-token-abc", token)
	}
}

func TestGenerateTokenErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authResponse{Error: "invalid_grant", Message: ": code expired"})
	}))
	defer srv.Close()

	d := Dirigera{hubUrl: srv.URL}
	_, err := d.generateToken("some-code", "some-verifier")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error %q does not mention invalid_grant", err)
	}
}

func TestListEnvironmentSensors(t *testing.T) {
	const body = `[
		{"id":"1","deviceType":"environmentSensor","attributes":{"customName":"Kitchen","currentTemperature":21}},
		{"id":"2","deviceType":"light","attributes":{"customName":"Lamp"}},
		{"id":"3","deviceType":"environmentSensor","attributes":{"customName":"Bedroom","currentTemperature":19}}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			t.Fatalf("Authorization header = %q, want Bearer tok-123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	d := Dirigera{hubUrl: srv.URL, AuthToken: "tok-123"}
	devices, err := d.ListEnvironmentSensors()
	if err != nil {
		t.Fatal(err)
	}
	if len(*devices) != 2 {
		t.Fatalf("got %d devices, want 2 (non-sensor device should be filtered out)", len(*devices))
	}
	for _, dev := range *devices {
		if dev.DeviceType != "environmentSensor" {
			t.Fatalf("got device with DeviceType %q, want only environmentSensor", dev.DeviceType)
		}
	}
}

func TestListEnvironmentSensorsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	d := Dirigera{hubUrl: srv.URL}
	if _, err := d.ListEnvironmentSensors(); err == nil {
		t.Fatal("expected an error decoding invalid JSON, got nil")
	}
}
