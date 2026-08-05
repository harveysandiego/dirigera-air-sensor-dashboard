// Package dirigera talks to an IKEA DIRIGERA smart home hub: discovering it
// via mDNS, pairing with it over OAuth/PKCE, and querying its devices.
package dirigera

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/hashicorp/mdns"
)

// httpClient is used for all requests to the hub, which presents a
// self-signed certificate, instead of mutating http.DefaultTransport globally.
var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type (
	// Dirigera is a client for a single paired DIRIGERA hub.
	Dirigera struct {
		configFile string
		hubUrl     string
		Interface  string
		AuthToken  string
	}

	authResponse struct {
		Code        string `json:"code"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Message     string `json:"message"`
	}

	// Device is a hub-managed device, as returned by the hub's /v1/devices endpoint.
	Device struct {
		ID         string    `json:"id"`
		Type       string    `json:"type"`
		DeviceType string    `json:"deviceType"`
		CreatedAt  time.Time `json:"createdAt"`
		LastSeen   time.Time `json:"lastSeen"`
		Attributes struct {
			CustomName         string `json:"customName"`
			FirmwareVersion    string `json:"firmwareVersion"`
			HardwareVersion    string `json:"hardwareVersion"`
			Manufacturer       string `json:"manufacturer"`
			Model              string `json:"model"`
			ProductCode        string `json:"productCode"`
			SerialNumber       string `json:"serialNumber"`
			CurrentTemperature int    `json:"currentTemperature"`
			CurrentRH          int    `json:"currentRH"`
			CurrentPM25        int    `json:"currentPM25"`
			VocIndex           int    `json:"vocIndex"`
		} `json:"attributes"`
	}
)

// ErrHubTimeout is returned by Discover when no hub responds within the mDNS query window.
var ErrHubTimeout = errors.New("timeout discovering hub")

// New loads a Dirigera client from filename, or returns an empty, unpaired
// client if the file doesn't exist yet.
func New(filename string) (*Dirigera, error) {
	diregera := &Dirigera{
		configFile: filename,
	}

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No config")
			return diregera, nil
		}

		return nil, err
	}
	defer func() { _ = file.Close() }()

	log.Debug("Reading config")
	decoder := json.NewDecoder(file)
	err = decoder.Decode(diregera)
	if err != nil {
		return nil, err
	}
	log.Debug(*diregera)

	return diregera, nil
}

// Discover finds a hub on the local network via mDNS and records its URL.
func (d *Dirigera) Discover() error {
	var ifi *net.Interface
	var err error
	if d.Interface == "" {
		log.Debug("No interface name given")
		ifi = nil
	} else {
		ifi, err = net.InterfaceByName(d.Interface)
		if err != nil {
			return err
		}
	}

	entries := make(chan *mdns.ServiceEntry, 4)
	timeout := 60 * time.Second
	params := &mdns.QueryParam{
		Service:             "_ihsp._tcp",
		Domain:              "local",
		Timeout:             timeout,
		Interface:           ifi,
		Entries:             entries,
		WantUnicastResponse: false,
		DisableIPv4:         false,
		DisableIPv6:         false,
	}
	err = mdns.Query(params)
	if err != nil {
		return err
	}

	for {
		select {
		case entry := <-entries:
			log.Debug(*entry)
			if strings.Contains(entry.Info, "DIRIGERA") {
				log.Info("Found hub")
				var ip string
				if entry.AddrV4 != nil {
					ip = entry.AddrV4.String()
				} else {
					for _, field := range entry.InfoFields {
						if strings.HasPrefix(field, "ipv4address=") {
							ip = strings.TrimPrefix(field, "ipv4address=")
						}
					}
				}
				if ip == "" {
					return errors.New("mDNS reply is missing IP info")
				}
				d.hubUrl = fmt.Sprintf("https://%s:%d", ip, entry.Port)
				log.Debug(d.hubUrl)
				close(entries)
				return nil
			}
		case <-time.After(timeout):
			return ErrHubTimeout
		}
	}
}

// Auth performs the PKCE pairing handshake against a discovered hub,
// prompting for a physical button press on the hub, and persists the
// resulting token to configFile. It's a no-op if already paired.
func (d *Dirigera) Auth() error {
	if d.hubUrl == "" {
		return errors.New("hub URL not set")
	}

	if d.AuthToken != "" {
		log.Info("Auth token already set, skipping auth")
		return nil
	}

	log.Info("Starting auth")

	authUrl, err := url.Parse(d.hubUrl + "/v1/oauth/authorize")
	if err != nil {
		return err
	}

	codeVerifier := createCodeVerifier()

	authParams := url.Values{}
	authParams.Add("audience", "homesmart.local")
	authParams.Add("response_type", "code")
	authParams.Add("code_challenge", createCodeChallenge(codeVerifier))
	authParams.Add("code_challenge_method", "S256")
	authUrl.RawQuery = authParams.Encode()

	log.Debug("Get auth code")
	resp, err := httpClient.Get(authUrl.String())
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response authResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}

	if response.Error != "" {
		return errors.New(response.Error + response.Message)
	}

	log.Info("Press button on hub (sleeping 20s)")
	time.Sleep(20 * time.Second)

	token, err := d.generateToken(response.Code, codeVerifier)
	if err != nil {
		return err
	}

	d.AuthToken = token
	log.Debug(d.AuthToken)

	err = d.saveConfig()
	if err != nil {
		return err
	}

	return nil
}

func (d Dirigera) generateToken(code string, codeVerifier string) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	tokenParams := url.Values{}
	tokenParams.Add("code", code)
	tokenParams.Add("code_verifier", codeVerifier)
	tokenParams.Add("name", hostname)
	tokenParams.Add("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", d.hubUrl+"/v1/oauth/token", strings.NewReader(tokenParams.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Debug("Post auth code")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response authResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}

	if response.Error != "" {
		return "", errors.New(response.Error + response.Message)
	}

	return response.AccessToken, nil
}

func (d Dirigera) saveConfig() error {
	file, err := os.Create(d.configFile)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	err = json.NewEncoder(file).Encode(d)
	if err != nil {
		return err
	}

	return nil
}

// ListEnvironmentSensors returns the hub's paired environmentSensor devices
// (e.g. VINDSTYRKA air quality sensors), filtered out of its full device list.
func (d Dirigera) ListEnvironmentSensors() (*[]Device, error) {
	req, err := http.NewRequest("GET", d.hubUrl+"/v1/devices", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+d.AuthToken)

	log.Debug("Get list of devices")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var devices []Device
	err = json.Unmarshal(body, &devices)
	if err != nil {
		return nil, err
	}

	var filteredDevices []Device
	for _, device := range devices {
		if device.DeviceType == "environmentSensor" {
			filteredDevices = append(filteredDevices, device)
		}
	}

	return &filteredDevices, nil
}

func createCodeVerifier() string {
	charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	codeLength := 128
	b := make([]byte, codeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Fatal(err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func createCodeChallenge(codeVerifier string) string {
	hash := sha256.New()
	hash.Write([]byte(codeVerifier))
	digest := hash.Sum(nil)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(digest)
}
