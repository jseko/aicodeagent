package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessContextPathLoadsKnownFilesInPriorityOrder(t *testing.T) {
	root := t.TempDir()
	writeProjectMemoryFile(t, root, "CLAUDE.md", "claude rules")
	writeProjectMemoryFile(t, root, "AGENTS.md", "agent rules")
	writeProjectMemoryFile(t, root, ".github/copilot-instructions.md", "copilot rules")

	files := processContextPath(root, defaultMemoryMaxBytes)

	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	wantSuffixes := []string{"AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"}
	wantContents := []string{"agent rules", "claude rules", "copilot rules"}
	for i := range files {
		if !strings.HasSuffix(files[i].Path, wantSuffixes[i]) {
			t.Fatalf("files[%d].Path = %q, want suffix %q", i, files[i].Path, wantSuffixes[i])
		}
		if files[i].Content != wantContents[i] {
			t.Fatalf("files[%d].Content = %q, want %q", i, files[i].Content, wantContents[i])
		}
	}
}

func TestProcessContextPathSkipsFilesOverLimit(t *testing.T) {
	root := t.TempDir()
	writeProjectMemoryFile(t, root, "AGENTS.md", strings.Repeat("x", 16))
	writeProjectMemoryFile(t, root, "CLAUDE.md", "small")

	files := processContextPath(root, 8)

	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if !strings.HasSuffix(files[0].Path, "CLAUDE.md") {
		t.Fatalf("loaded path = %q, want CLAUDE.md", files[0].Path)
	}
}

func TestBuildSystemPromptMergesBaseProjectContextAndMCPInstructions(t *testing.T) {
	prompt := buildSystemPrompt("base prompt", []ContextFile{
		{Path: "AGENTS.md", Content: "agent rules"},
	}, "mcp rules")

	for _, want := range []string{
		"base prompt",
		"<project_context path=\"AGENTS.md\">",
		"agent rules",
		"<mcp_instructions>",
		"mcp rules",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, prompt)
		}
	}
}

func writeProjectMemoryFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
