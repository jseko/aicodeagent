package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"AICodeAgent/internal/config"
	"AICodeAgent/internal/skills"
)

func TestNewLoadsSkillSuggestions(t *testing.T) {
	skillRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(skillRoot, "home"))
	t.Setenv("AICODE_SKILLS_DIR", "")
	skillDir := filepath.Join(skillRoot, "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillFile := filepath.Join(skillDir, skills.SkillFileName)
	content := "---\nname: code-review\ndescription: Review code.\n---\nFull instructions."
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
	expectedSkillFile, err := filepath.EvalSymlinks(skillFile)
	if err != nil {
		expectedSkillFile = skillFile
	}

	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.SkillsPaths = []string{skillRoot}

	app := New(context.Background(), cfg)
	defer app.Close()

	if len(app.SkillSuggestions) != 1 {
		t.Fatalf("expected 1 skill suggestion, got %+v", app.SkillSuggestions)
	}
	suggestion := app.SkillSuggestions[0]
	if suggestion.Command != "/code-review" || suggestion.Description != "Review code." || suggestion.Location != expectedSkillFile {
		t.Fatalf("unexpected suggestion: %+v", suggestion)
	}
}
