package subagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryDuplicateAndOverride(t *testing.T) {
	registry := NewRegistry()
	agent := testAgent("review", "prompt")
	if err := registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Register(agent); err == nil {
		t.Fatalf("expected duplicate error")
	}
	replacement := testAgent("review", "replacement")
	if err := registry.Override(replacement); err != nil {
		t.Fatalf("override: %v", err)
	}
	got, err := registry.Get("review")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SystemPrompt != "replacement" {
		t.Fatalf("expected replacement prompt, got %q", got.SystemPrompt)
	}
}

func TestRegistryListStableOrdering(t *testing.T) {
	registry := NewRegistry()
	for _, name := range []string{"zeta", "alpha", "beta"} {
		if err := registry.Register(testAgent(name, "prompt")); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	list := registry.List()
	if list[0].Config.Name != "alpha" || list[1].Config.Name != "beta" || list[2].Config.Name != "zeta" {
		t.Fatalf("list not sorted: %#v", list)
	}
}

func TestLoaderProjectOverridesUser(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	projectDir := filepath.Join(root, "project")
	mustMkdir(t, userDir)
	mustMkdir(t, projectDir)
	writeSubagentFile(t, userDir, "review.md", "review", "user prompt")
	writeSubagentFile(t, projectDir, "review.md", "review", "project prompt")

	loader := &SubagentLoader{UserDir: userDir, ProjectDir: projectDir, Registry: NewRegistry()}
	if err := loader.LoadSubagents(); err != nil {
		t.Fatalf("load: %v", err)
	}
	agent, err := loader.Registry.Get("review")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if agent.SystemPrompt != "project prompt" {
		t.Fatalf("expected project override, got %q", agent.SystemPrompt)
	}
}

func TestLoaderContinuesAfterSingleFileFailure(t *testing.T) {
	dir := t.TempDir()
	writeSubagentFile(t, dir, "valid.md", "valid", "valid prompt")
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("not front matter"), 0o600); err != nil {
		t.Fatalf("write broken: %v", err)
	}

	loader := &SubagentLoader{UserDir: dir, Registry: NewRegistry()}
	if err := loader.LoadSubagents(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := loader.Registry.Get("valid"); err != nil {
		t.Fatalf("expected valid subagent loaded: %v", err)
	}
	if len(loader.Diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(loader.Diagnostics))
	}
}

func testAgent(name, prompt string) *Subagent {
	return &Subagent{
		Config: &SubagentConfig{
			Name:        name,
			Description: "Agent. Use when: testing",
			Mode:        ModeSubagent,
			Model:       "gpt-x",
			Permissions: &PermissionConfig{Level: PermissionLevelStandard},
		},
		SystemPrompt: prompt,
	}
}

func writeSubagentFile(t *testing.T, dir, fileName, name, prompt string) {
	t.Helper()
	content := `---
name: ` + name + `
description: |
  Agent.
  Use when: testing
model: gpt-x
permissions:
  level: standard
---

` + prompt + `
`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write subagent file: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}
