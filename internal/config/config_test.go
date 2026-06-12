package config

import (
	"os"
	"testing"
)

func TestConfigLoad_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    func(t *testing.T, cfg *Config)
		wantErr bool
	}{
		{
			name: "SingleConfig",
			content: `theme: dark
openai:
  model: gpt-4
log_level: info`,
			want: func(t *testing.T, cfg *Config) {
				if cfg.Theme != "dark" {
					t.Errorf("Theme = %s, want dark", cfg.Theme)
				}
				if cfg.OpenAI.Model != "gpt-4" {
					t.Errorf("OpenAI.Model = %s, want gpt-4", cfg.OpenAI.Model)
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel = %s, want info", cfg.LogLevel)
				}
			},
		},
		{
			name: "MergedConfig_PartialOverride",
			content: `theme: light
log_level: debug`,
			want: func(t *testing.T, cfg *Config) {
				if cfg.Theme != "light" {
					t.Errorf("Theme = %s, want light", cfg.Theme)
				}
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
				}
				// Defaults should be preserved for fields not in YAML
				if cfg.OpenAI.Model != "gpt-4o" {
					t.Errorf("OpenAI.Model = %s, want default gpt-4o", cfg.OpenAI.Model)
				}
				if cfg.Session.MaxSessions != 10 {
					t.Errorf("MaxSessions = %d, want default 10", cfg.Session.MaxSessions)
				}
			},
		},
		{
			name: "InvalidJSON",
			content: `theme: [invalid: yaml: structure`,
			want: func(t *testing.T, cfg *Config) {
				// Should fall back to defaults on parse error
				if cfg.Theme != "dark" {
					t.Errorf("Theme = %s, want default dark", cfg.Theme)
				}
			},
		},
		{
			name:    "EmptyConfig",
			content: ``,
			want: func(t *testing.T, cfg *Config) {
				if cfg.Theme != "dark" {
					t.Errorf("Theme = %s, want default dark", cfg.Theme)
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel = %s, want default info", cfg.LogLevel)
				}
			},
		},
		{
			name: "ModelConfigOverride",
			content: `models:
  default: "claude-sonnet"
  items:
    claude-sonnet:
      provider: "anthropic"
      model: "claude-sonnet-4-6"
      temperature: 0.5
      max_tokens: 8192`,
			want: func(t *testing.T, cfg *Config) {
				if cfg.Models.Default != "claude-sonnet" {
					t.Errorf("Models.Default = %s, want claude-sonnet", cfg.Models.Default)
				}
				m, ok := cfg.GetModelConfig("claude-sonnet")
				if !ok {
					t.Fatal("GetModelConfig(claude-sonnet) not found")
				}
				if m.Provider != "anthropic" {
					t.Errorf("Provider = %s, want anthropic", m.Provider)
				}
				if m.Temperature != 0.5 {
					t.Errorf("Temperature = %f, want 0.5", m.Temperature)
				}
				if m.MaxTokens != 8192 {
					t.Errorf("MaxTokens = %d, want 8192", m.MaxTokens)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "config-*.yaml")
			if err != nil {
				t.Fatalf("CreateTemp: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("WriteString: %v", err)
			}
			tmpFile.Close()

			cfg, err := Load(tmpFile.Name())
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tt.want != nil {
				tt.want(t, cfg)
			}
		})
	}
}

func TestConfigLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("file not exist should return defaults: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %s, want default dark", cfg.Theme)
	}
}

func TestResolveSecret(t *testing.T) {
	cfg := &Config{}
	os.Setenv("TEST_SECRET_KEY", "secret123")
	defer os.Unsetenv("TEST_SECRET_KEY")

	tests := []struct {
		name    string
		input   string
		want    string
	}{
		{"EnvVarSyntax", "${TEST_SECRET_KEY}", "secret123"},
		{"PlainEnvName", "TEST_SECRET_KEY", "secret123"},
		{"EmptyString", "", ""},
		{"MissingVar", "${NONEXISTENT_VAR}", "${NONEXISTENT_VAR}"},
		{"PlainTextNotEnv", "just-a-string", "just-a-string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.ResolveSecret(tt.input)
			if got != tt.want {
				t.Errorf("ResolveSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetModelConfig(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			Default: "gpt-4o",
			Items: map[string]ModelConfig{
				"gpt-4o": {
					Provider: "openai", Model: "gpt-4o",
					Temperature: 0.7, TopP: 1.0, MaxTokens: 4096,
				},
			},
		},
	}

	tests := []struct {
		name   string
		id     string
		wantOK bool
	}{
		{"ExplicitID", "gpt-4o", true},
		{"EmptyID_FallsBackToDefault", "", true},
		{"MissingID", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := cfg.GetModelConfig(tt.id)
			if ok != tt.wantOK {
				t.Errorf("GetModelConfig(%q) ok = %v, want %v", tt.id, ok, tt.wantOK)
			}
		})
	}
}

func TestModelForScenario(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			Default: "gpt-4o",
			Scenarios: ModelScenarios{
				Chat:       "gpt-4o",
				Summarize:  "gpt-4o-mini",
				Initialize: "gpt-4o-mini",
			},
			Items: map[string]ModelConfig{
				"gpt-4o":      {Provider: "openai", Model: "gpt-4o"},
				"gpt-4o-mini": {Provider: "openai", Model: "gpt-4o-mini"},
			},
		},
	}

	tests := []struct {
		scenario string
		wantID   string
	}{
		{"chat", "gpt-4o"},
		{"summarize", "gpt-4o-mini"},
		{"initialize", "gpt-4o-mini"},
		{"unknown", "gpt-4o"},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			id, _, ok := cfg.ModelForScenario(tt.scenario)
			if !ok {
				t.Errorf("ModelForScenario(%q) not found", tt.scenario)
			}
			if id != tt.wantID {
				t.Errorf("ModelForScenario(%q) = %q, want %q", tt.scenario, id, tt.wantID)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"Valid", &Config{Theme: "dark", LogLevel: "info"}, false},
		{"EmptyTheme", &Config{Theme: "", LogLevel: "info"}, true},
		{"EmptyLogLevel", &Config{Theme: "dark", LogLevel: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
