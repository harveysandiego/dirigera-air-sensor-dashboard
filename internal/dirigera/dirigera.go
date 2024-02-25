package dirigera

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

type (
	Dirigera struct {
		configFile string
		HubUrl     string
		AuthToken  string
	}

	authResponse struct {
		Code        string `json:"code"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Message     string `json:"message"`
	}

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

func New(filename string) (*Dirigera, error) {
	diregera := &Dirigera{
		configFile: filename,
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	log.Debug("Reading history")
	decoder := json.NewDecoder(file)
	err = decoder.Decode(diregera)
	if err != nil {
		return nil, err
	}
	log.Debug(*diregera)

	return diregera, nil
}

func (d *Dirigera) Auth() error {
	if d.AuthToken != "" {
		log.Info("Auth token already set, skipping auth")
		return nil
	}

	if d.HubUrl == "" {
		return errors.New("hub URL not set")
	}

	log.Info("Starting auth")

	authUrl, err := url.Parse(d.HubUrl + "/v1/oauth/authorize")
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
	resp, err := http.Get(authUrl.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

	req, err := http.NewRequest("POST", d.HubUrl+"/v1/oauth/token", strings.NewReader(tokenParams.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Debug("Post auth code")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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
	defer file.Close()

	err = json.NewEncoder(file).Encode(d)
	if err != nil {
		return err
	}

	return nil
}

func (d Dirigera) ListEnvironmentSensors() (*[]Device, error) {
	req, err := http.NewRequest("GET", d.HubUrl+"/v1/devices", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+d.AuthToken)

	client := &http.Client{}
	log.Debug("Get list of devices")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var devices []Device
	err = json.Unmarshal([]byte(body), &devices)
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
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, codeLength)
	for i := range b {
		b[i] = charset[rnd.Intn(len(charset))]
	}
	return string(b)
}

func createCodeChallenge(codeVerifier string) string {
	hash := sha256.New()
	hash.Write([]byte(codeVerifier))
	digest := hash.Sum(nil)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(digest)
}
