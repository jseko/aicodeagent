package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetDefaultBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{name: "OpenAI", provider: "openai", want: "https://api.openai.com/v1"},
		{name: "Anthropic", provider: "anthropic", want: "https://api.anthropic.com/v1"},
		{name: "OpenRouter", provider: "openrouter", want: "https://openrouter.ai/api/v1"},
		{name: "Unknown", provider: "unknown", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDefaultBaseURL(tt.provider); got != tt.want {
				t.Errorf("getDefaultBaseURL(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		apiKey   string
		model    string
	}{
		{name: "OpenAI", provider: "openai", apiKey: "sk-test", model: "gpt-4o"},
		{name: "Anthropic", provider: "anthropic", apiKey: "sk-ant-test", model: "claude-sonnet-4-6"},
		{name: "OpenRouter", provider: "openrouter", apiKey: "or-test", model: "openai/gpt-4o"},
		{name: "EmptyAPIKey", provider: "openai", apiKey: "", model: "gpt-4o-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildConfig(tt.provider, tt.apiKey, tt.model)

			if cfg["theme"] != "dark" {
				t.Errorf("theme = %v, want dark", cfg["theme"])
			}
			if cfg["log_level"] != "info" {
				t.Errorf("log_level = %v, want info", cfg["log_level"])
			}

			models, ok := cfg["models"].(map[string]interface{})
			if !ok {
				t.Fatal("models is not a map")
			}
			if models["default"] != tt.model {
				t.Errorf("default model = %v, want %v", models["default"], tt.model)
			}

			if tt.apiKey != "" {
				providers, ok := cfg["providers"].([]map[string]interface{})
				if !ok || len(providers) != 1 {
					t.Fatal("providers should have 1 entry")
				}
				if providers[0]["api_key"] != tt.apiKey {
					t.Errorf("api_key = %v, want %v", providers[0]["api_key"], tt.apiKey)
				}
				if providers[0]["type"] != tt.provider {
					t.Errorf("provider type = %v, want %v", providers[0]["type"], tt.provider)
				}
			} else {
				if _, ok := cfg["providers"]; ok {
					t.Error("providers should not exist when apiKey is empty")
				}
			}
		})
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")

	cfg := map[string]interface{}{
		"theme":     "dark",
		"log_level": "info",
	}

	if err := writeConfig(path, cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}

	if parsed["theme"] != "dark" {
		t.Errorf("theme = %v, want dark", parsed["theme"])
	}
	if parsed["log_level"] != "info" {
		t.Errorf("log_level = %v, want info", parsed["log_level"])
	}
}

func TestBuildConfig_ModelItems(t *testing.T) {
	cfg := buildConfig("openai", "sk-test", "gpt-4o")

	models := cfg["models"].(map[string]interface{})
	items := models["items"].(map[string]interface{})
	modelCfg := items["gpt-4o"].(map[string]interface{})

	if modelCfg["provider"] != "openai" {
		t.Errorf("provider = %v, want openai", modelCfg["provider"])
	}
	if modelCfg["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", modelCfg["temperature"])
	}
	if modelCfg["max_tokens"] != 4096 {
		t.Errorf("max_tokens = %v, want 4096", modelCfg["max_tokens"])
	}
}
