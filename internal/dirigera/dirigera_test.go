package dirigera

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
)

// withFakeMDNS swaps mdnsQuery/discoverTimeout for the test's duration, so Discover skips real mDNS.
func withFakeMDNS(t *testing.T, timeout time.Duration, query func(*mdns.QueryParam) error) {
	t.Helper()
	origQuery, origTimeout := mdnsQuery, discoverTimeout
	mdnsQuery, discoverTimeout = query, timeout
	t.Cleanup(func() { mdnsQuery, discoverTimeout = origQuery, origTimeout })
}

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

// TestCreateCodeChallenge checks against RFC 7636 Appendix B's own worked example.
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

func TestNewInvalidConfigJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(path); err == nil {
		t.Fatal("expected an error decoding invalid JSON, got nil")
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

func TestSaveConfigCreateFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "config.json")
	d := &Dirigera{configFile: path}

	if err := d.saveConfig(); err == nil {
		t.Fatal("expected an error creating a config file in a nonexistent directory, got nil")
	}
}

func TestDiscoverFindsHub(t *testing.T) {
	withFakeMDNS(t, time.Second, func(params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{
			Info:   "DIRIGERA-abc123",
			AddrV4: net.ParseIP("192.168.1.50"),
			Port:   8443,
		}
		return nil
	})

	d := &Dirigera{}
	if err := d.Discover(); err != nil {
		t.Fatal(err)
	}
	if d.hubUrl != "https://192.168.1.50:8443" {
		t.Fatalf("hubUrl = %q, want https://192.168.1.50:8443", d.hubUrl)
	}
}

func TestDiscoverFallsBackToInfoFieldsIP(t *testing.T) {
	withFakeMDNS(t, time.Second, func(params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{
			Info:       "DIRIGERA-abc123",
			InfoFields: []string{"ipv4address=192.168.1.51"},
			Port:       8443,
		}
		return nil
	})

	d := &Dirigera{}
	if err := d.Discover(); err != nil {
		t.Fatal(err)
	}
	if d.hubUrl != "https://192.168.1.51:8443" {
		t.Fatalf("hubUrl = %q, want https://192.168.1.51:8443", d.hubUrl)
	}
}

func TestDiscoverMissingIPInfo(t *testing.T) {
	withFakeMDNS(t, time.Second, func(params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{Info: "DIRIGERA-abc123", Port: 8443}
		return nil
	})

	d := &Dirigera{}
	err := d.Discover()
	if err == nil || !strings.Contains(err.Error(), "missing IP info") {
		t.Fatalf("got %v, want a missing IP info error", err)
	}
}

func TestDiscoverIgnoresNonHubEntries(t *testing.T) {
	withFakeMDNS(t, 50*time.Millisecond, func(params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{Info: "some-other-service", AddrV4: net.ParseIP("10.0.0.1")}
		return nil
	})

	d := &Dirigera{}
	err := d.Discover()
	if !errors.Is(err, ErrHubTimeout) {
		t.Fatalf("got %v, want ErrHubTimeout", err)
	}
}

func TestDiscoverTimeout(t *testing.T) {
	withFakeMDNS(t, 50*time.Millisecond, func(params *mdns.QueryParam) error {
		return nil // no entries ever sent
	})

	d := &Dirigera{}
	if err := d.Discover(); !errors.Is(err, ErrHubTimeout) {
		t.Fatalf("got %v, want ErrHubTimeout", err)
	}
}

func TestDiscoverInvalidInterface(t *testing.T) {
	d := &Dirigera{Interface: "not-a-real-interface"}
	if err := d.Discover(); err == nil {
		t.Fatal("expected an error for a nonexistent network interface, got nil")
	}
}

func TestDiscoverMDNSQueryError(t *testing.T) {
	boom := errors.New("boom")
	withFakeMDNS(t, time.Second, func(params *mdns.QueryParam) error {
		return boom
	})

	d := &Dirigera{}
	if err := d.Discover(); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
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

func TestGenerateTokenInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	d := Dirigera{hubUrl: srv.URL}
	if _, err := d.generateToken("some-code", "some-verifier"); err == nil {
		t.Fatal("expected an error decoding invalid JSON, got nil")
	}
}

func TestGenerateTokenHubUnreachable(t *testing.T) {
	d := Dirigera{hubUrl: "http://127.0.0.1:1"}
	if _, err := d.generateToken("some-code", "some-verifier"); err == nil {
		t.Fatal("expected an error for an unreachable hub, got nil")
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

func TestListEnvironmentSensorsHubUnreachable(t *testing.T) {
	d := Dirigera{hubUrl: "http://127.0.0.1:1"}
	if _, err := d.ListEnvironmentSensors(); err == nil {
		t.Fatal("expected an error for an unreachable hub, got nil")
	}
}

func withShortPairingDelay(t *testing.T) {
	t.Helper()
	orig := pairingDelay
	pairingDelay = time.Millisecond
	t.Cleanup(func() { pairingDelay = orig })
}

func TestAuthFullFlow(t *testing.T) {
	withShortPairingDelay(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("code_challenge_method") != "S256" {
			t.Fatalf("missing/wrong code_challenge_method: %s", r.URL.Query().Get("code_challenge_method"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authResponse{Code: "auth-code-xyz"})
	})
	mux.HandleFunc("/v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("code") != "auth-code-xyz" {
			t.Fatalf("token request code = %q, want auth-code-xyz", r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authResponse{AccessToken: "final-token-abc"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	d := &Dirigera{hubUrl: srv.URL, configFile: configPath}

	if err := d.Auth(); err != nil {
		t.Fatal(err)
	}
	if d.AuthToken != "final-token-abc" {
		t.Fatalf("AuthToken = %q, want final-token-abc", d.AuthToken)
	}

	reloaded, err := New(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AuthToken != "final-token-abc" {
		t.Fatalf("persisted AuthToken = %q, want final-token-abc", reloaded.AuthToken)
	}
}

func TestAuthNoHubURL(t *testing.T) {
	d := &Dirigera{}
	if err := d.Auth(); err == nil {
		t.Fatal("expected an error when hubUrl is unset, got nil")
	}
}

func TestAuthAlreadyPaired(t *testing.T) {
	// hubUrl deliberately invalid: Auth must return before ever dialing it.
	d := &Dirigera{hubUrl: "http://invalid.invalid", AuthToken: "already-set"}
	if err := d.Auth(); err != nil {
		t.Fatalf("unexpected error for an already-paired client: %v", err)
	}
}

func TestAuthAuthorizeErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authResponse{Error: "access_denied", Message: ": user declined"})
	}))
	defer srv.Close()

	d := &Dirigera{hubUrl: srv.URL}
	err := d.Auth()
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("got %v, want an error mentioning access_denied", err)
	}
}

func TestAuthAuthorizeInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	d := &Dirigera{hubUrl: srv.URL}
	if err := d.Auth(); err == nil {
		t.Fatal("expected an error decoding invalid JSON, got nil")
	}
}

func TestAuthHubUnreachable(t *testing.T) {
	d := &Dirigera{hubUrl: "http://127.0.0.1:1"}
	if err := d.Auth(); err == nil {
		t.Fatal("expected an error for an unreachable hub, got nil")
	}
}
