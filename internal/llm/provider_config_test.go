package llm_test

import (
	"os"
	"path/filepath"
	"testing"

	"AICodeAgent/internal/config"
)

func TestLoadModelsConfigFromYAML(t *testing.T) {
	path := writeTempConfig(t, `
theme: dark
log_level: info
models:
  default: chat-fast
  scenarios:
    chat: chat-fast
    summarize: summary-cheap
    initialize: summary-cheap
  items:
    chat-fast:
      provider: deepseek
      model: deepseek-v4-pro
      temperature: 0.2
      top_p: 0.9
      max_tokens: 8192
      input_per_1m: 0.14
      output_per_1m: 0.28
    summary-cheap:
      provider: deepseek
      model: deepseek-v4-pro
      temperature: 0.1
      top_p: 1.0
      max_tokens: 2048
providers:
  - type: openai
    name: deepseek
    base_url: https://api.deepseek.com/v1
    api_key: DEEPSEEK_API_KEY
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	model, ok := cfg.GetModelConfig("chat-fast")
	if !ok {
		t.Fatal("expected chat-fast model config")
	}
	if model.Provider != "deepseek" {
		t.Fatalf("Provider = %q, want deepseek", model.Provider)
	}
	if model.Model != "deepseek-v4-pro" {
		t.Fatalf("Model = %q, want deepseek-v4-pro", model.Model)
	}
	if model.Temperature != 0.2 {
		t.Fatalf("Temperature = %v, want 0.2", model.Temperature)
	}
	if model.TopP != 0.9 {
		t.Fatalf("TopP = %v, want 0.9", model.TopP)
	}
	if model.MaxTokens != 8192 {
		t.Fatalf("MaxTokens = %d, want 8192", model.MaxTokens)
	}
	if model.InputPer1M != 0.14 || model.OutputPer1M != 0.28 {
		t.Fatalf("pricing = %v/%v, want 0.14/0.28", model.InputPer1M, model.OutputPer1M)
	}
}

func TestModelConfigDefaults(t *testing.T) {
	path := writeTempConfig(t, `
theme: dark
log_level: info
openai:
  temperature: 0.6
models:
  default: local-model
  items:
    local-model: {}
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	model, ok := cfg.GetModelConfig("")
	if !ok {
		t.Fatal("expected default model config")
	}
	if model.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", model.Provider)
	}
	if model.Model != "local-model" {
		t.Fatalf("Model = %q, want local-model", model.Model)
	}
	if model.Temperature != 0.6 {
		t.Fatalf("Temperature = %v, want 0.6", model.Temperature)
	}
	if model.TopP != 1.0 {
		t.Fatalf("TopP = %v, want 1.0", model.TopP)
	}
	if model.MaxTokens != 4096 {
		t.Fatalf("MaxTokens = %d, want 4096", model.MaxTokens)
	}
}

func TestModelForScenarioFallsBackToDefault(t *testing.T) {
	path := writeTempConfig(t, `
theme: dark
log_level: info
models:
  default: chat-fast
  scenarios:
    chat: chat-fast
    summarize: summary-cheap
  items:
    chat-fast:
      provider: openai
      model: gpt-4o
    summary-cheap:
      provider: openai
      model: gpt-4o-mini
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	id, model, ok := cfg.ModelForScenario("summarize")
	if !ok {
		t.Fatal("expected summarize model")
	}
	if id != "summary-cheap" || model.Model != "gpt-4o-mini" {
		t.Fatalf("summarize = %q/%q, want summary-cheap/gpt-4o-mini", id, model.Model)
	}

	id, model, ok = cfg.ModelForScenario("unknown")
	if !ok {
		t.Fatal("expected default model for unknown scenario")
	}
	if id != "chat-fast" || model.Model != "gpt-4o" {
		t.Fatalf("unknown = %q/%q, want chat-fast/gpt-4o", id, model.Model)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
