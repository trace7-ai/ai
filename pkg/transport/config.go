package transport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	miraBaseURL      = "https://mira.byteintl.net"
	defaultModelKey  = "opus4.6"
	defaultModelName = "Cloud-O-4.6"
)

type modelSpec struct {
	Name string
	ID   string
	Mode string
}

var models = map[string]modelSpec{
	"opus4.6":   {Name: "Cloud-O-4.6", ID: "re-o-46", Mode: "quick"},
	"opus4.6t":  {Name: "Cloud-O-4.6 Think", ID: "re-o-46", Mode: "deep"},
	"opus4.5":   {Name: "Cloud-O-4.5", ID: "re-o-45", Mode: "quick"},
	"sonnet4.6": {Name: "Cloud-S-4.6", ID: "re-s-46", Mode: "quick"},
	"sonnet4":   {Name: "Cloud-S-4", ID: "claude-sonnet-4-20250514", Mode: "quick"},
	"sonnet3.7": {Name: "Cloud-S-3.7", ID: "claude-3-7-sonnet-20250219", Mode: "quick"},
	"sonnet3.5": {Name: "Cloud-S-3.5", ID: "claude-3-5-sonnet-20241022", Mode: "quick"},
	"haiku3.5":  {Name: "Cloud-H-3.5", ID: "claude-3-5-haiku-20241022", Mode: "quick"},
	"gpt5.4":    {Name: "GPT-5.4", ID: "gpt-5.4", Mode: "quick"},
	"gemini3.1": {Name: "Gemini 3.1 Pro", ID: "gemini-3.1-pro-preview", Mode: "quick"},
	"glm5":      {Name: "Glm-5", ID: "glm-5", Mode: "quick"},
}

type Config struct {
	Cookies   string
	Username  string
	DeviceID  string
	ModelKey  string
	BaseURL   string
	DebugLogs bool
}

func LoadConfig() (Config, error) {
	config := Config{
		ModelKey:  defaultModelKey,
		BaseURL:   miraBaseURL,
		DebugLogs: os.Getenv("MIRA_DEBUG") == "1",
	}
	configPath := filepath.Join(miraHome(), "config.json")
	if body, err := os.ReadFile(configPath); err == nil {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return Config{}, fmt.Errorf("config file must contain a JSON object: %s", configPath)
		}
		if cookies, ok := payload["cookies"].(string); ok {
			config.Cookies = cookies
		}
		if config.Cookies == "" {
			if session, ok := payload["session"].(string); ok && session != "" {
				config.Cookies = "session=" + session
			}
		}
		if username, ok := payload["username"].(string); ok {
			config.Username = username
		}
		if deviceID, ok := payload["device_id"].(string); ok {
			config.DeviceID = deviceID
		}
	}
	modelPath := filepath.Join(miraHome(), "model")
	if body, err := os.ReadFile(modelPath); err == nil {
		if key := string(bytesTrimSpace(body)); key != "" {
			if _, ok := models[key]; ok {
				config.ModelKey = key
			}
		}
	}
	return config, nil
}

func (config Config) HasAuth() bool {
	return config.Cookies != ""
}

func (config Config) ModelName() *string {
	spec := config.modelSpec()
	return &spec.Name
}

func (config Config) ModelID() string {
	return config.modelSpec().ID
}

func (config Config) ModelMode() string {
	return config.modelSpec().Mode
}

func (config Config) modelSpec() modelSpec {
	if spec, ok := models[config.ModelKey]; ok {
		return spec
	}
	return modelSpec{Name: defaultModelName, ID: "re-o-46", Mode: "quick"}
}

func miraHome() string {
	if root := os.Getenv("MIRA_HOME"); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".mira")
}

func bytesTrimSpace(body []byte) []byte {
	start := 0
	end := len(body)
	for start < end && (body[start] == ' ' || body[start] == '\n' || body[start] == '\t' || body[start] == '\r') {
		start++
	}
	for end > start && (body[end-1] == ' ' || body[end-1] == '\n' || body[end-1] == '\t' || body[end-1] == '\r') {
		end--
	}
	return body[start:end]
}
