package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSubagentConfigValid(t *testing.T) {
	path := writeTempSubagent(t, "reviewer.md", `---
description: |
  Reviewer.
  Use when: reviewing code
model: gpt-x
tools:
  view: true
permissions:
  level: standard
---

You review code.
`)

	cfg, prompt, err := LoadSubagentConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Name != "reviewer" {
		t.Fatalf("expected derived name reviewer, got %q", cfg.Name)
	}
	if !strings.Contains(prompt, "review code") {
		t.Fatalf("expected prompt body, got %q", prompt)
	}
}

func TestLoadSubagentConfigInvalidFrontMatter(t *testing.T) {
	path := writeTempSubagent(t, "broken.md", `description: missing markers`)
	if _, _, err := LoadSubagentConfig(path); err == nil {
		t.Fatalf("expected invalid front matter error")
	}
}

func TestLoadSubagentConfigMissingFields(t *testing.T) {
	path := writeTempSubagent(t, "missing.md", `---
name: missing
permissions:
  level: standard
---

Prompt.
`)
	if _, _, err := LoadSubagentConfig(path); err == nil {
		t.Fatalf("expected missing fields error")
	}
}

func TestCreateSubagent(t *testing.T) {
	body, err := CreateSubagent("interactive", "review", "Use when: reviewing code", "gpt-x", nil)
	if err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	if !strings.Contains(string(body), "name: review") || !strings.Contains(string(body), "# System Prompt") {
		t.Fatalf("unexpected generated body: %s", body)
	}
}

func TestValidateBestPractice(t *testing.T) {
	cfg := &SubagentConfig{
		Name:        "review",
		Description: "Reviewer. Use when: reviewing code",
		Mode:        ModeSubagent,
		Model:       "gpt-x",
		Permissions: &PermissionConfig{Level: PermissionLevelStandard, AllowedTools: []string{"view"}},
	}
	if err := ValidateBestPractice(cfg); err != nil {
		t.Fatalf("validate best practice: %v", err)
	}

	cfg.Permissions.AllowedTools = []string{"write"}
	if err := ValidateBestPractice(cfg); err == nil {
		t.Fatalf("expected write best-practice error")
	}
}

func writeTempSubagent(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp subagent: %v", err)
	}
	return path
}
