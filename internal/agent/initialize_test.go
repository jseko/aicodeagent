package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanProjectStructureFindsKnownFiles(t *testing.T) {
	root := t.TempDir()

	writeProjectMemoryFile(t, root, "go.mod", "module example.com/myapp\n\ngo 1.21")
	writeProjectMemoryFile(t, root, "Makefile", "build:\n\tgo build ./...")
	writeProjectMemoryFile(t, root, "README.md", "# My Project")

	files, err := scanProjectStructure(root)
	if err != nil {
		t.Fatalf("scanProjectStructure: %v", err)
	}

	found := make(map[string]bool)
	for _, f := range files {
		found[f.Path] = true
	}
	for _, name := range []string{"go.mod", "Makefile", "README.md"} {
		if !found[name] {
			t.Errorf("expected %q to be found", name)
		}
	}
}

func TestScanProjectStructureSkipsMissingFiles(t *testing.T) {
	root := t.TempDir()
	writeProjectMemoryFile(t, root, "go.mod", "module x")

	files, err := scanProjectStructure(root)
	if err != nil {
		t.Fatalf("scanProjectStructure: %v", err)
	}

	if len(files) < 1 {
		t.Fatal("expected at least go.mod to be found")
	}
}

func TestScanProjectStructureHandlesEmptyProject(t *testing.T) {
	root := t.TempDir()

	files, err := scanProjectStructure(root)
	if err != nil {
		t.Fatalf("scanProjectStructure on empty dir: %v", err)
	}

	for _, f := range files {
		if f.Path == ".structure" {
			return // 至少有目录结构说明
		}
	}
}

func TestBuildInitPromptContainsProjectInfo(t *testing.T) {
	contextFiles := []ContextFile{
		{Path: "go.mod", Content: "module example.com/app"},
		{Path: "README.md", Content: "# App"},
	}

	prompt := buildInitPrompt("/tmp/test", contextFiles, "")

	for _, want := range []string{
		"/tmp/test",
		"go.mod",
		"module example.com/app",
		"README.md",
		"输出要求",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildInitPromptIncludesExistingContent(t *testing.T) {
	existing := "## [用户] 自定义规则\n不要删除我"

	prompt := buildInitPrompt("/tmp/test", nil, existing)

	if !strings.Contains(prompt, "## [用户] 自定义规则") {
		t.Error("prompt should include existing user sections")
	}
	if !strings.Contains(prompt, "保留上述内容中以 `## [用户]` 开头的章节") {
		t.Error("prompt should instruct to preserve user sections")
	}
}

func TestExtractUserSections(t *testing.T) {
	content := `# AGENTS.md

## 项目概述
auto-generated

## [用户] 自定义规则1
规则内容A

## 中间章节
auto again

## [用户] 自定义规则2
规则内容B
`

	sections := extractUserSections(content)

	if len(sections) != 2 {
		t.Fatalf("extractUserSections count = %d, want 2", len(sections))
	}
	if !strings.Contains(sections[0], "自定义规则1") {
		t.Errorf("section 0 = %q, should contain 自定义规则1", sections[0])
	}
	if !strings.Contains(sections[1], "自定义规则2") {
		t.Errorf("section 1 = %q, should contain 自定义规则2", sections[1])
	}
}

func TestExtractUserSectionsEmptyContent(t *testing.T) {
	if sections := extractUserSections(""); len(sections) != 0 {
		t.Errorf("expected 0 sections for empty content, got %d", len(sections))
	}
}

func TestExtractUserSectionsNoUserSections(t *testing.T) {
	content := `## 项目概述
auto

## 常用命令
build
`
	if sections := extractUserSections(content); len(sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(sections))
	}
}

func TestMergeAgentMDPreservesUserSections(t *testing.T) {
	existing := `## 项目概述
old content

## [用户] 我的规则
不要删除
`

	newContent := `## 项目概述
new content

## 常用命令
go build
`

	merged := mergeAgentMD(newContent, existing)

	if !strings.Contains(merged, "new content") {
		t.Error("merged should contain new content")
	}
	if !strings.Contains(merged, "## [用户] 我的规则") {
		t.Error("merged should preserve user sections")
	}
	if !strings.Contains(merged, "不要删除") {
		t.Error("merged should preserve user section content")
	}
}

func TestMergeAgentMDNoExistingContent(t *testing.T) {
	result := mergeAgentMD("new content", "")
	if result != "new content" {
		t.Errorf("mergeAgentMD without existing = %q, want %q", result, "new content")
	}
}

func TestHandleBuiltinCommandRoutesCorrectly(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		handled bool
	}{
		{name: "initialize", prompt: "/initialize", handled: true},
		{name: "initialize with args", prompt: "/initialize my project", handled: true},
		{name: "confirm", prompt: "/initialize-confirm", handled: true},
		{name: "reject", prompt: "/initialize-reject", handled: true},
		{name: "normal message", prompt: "hello", handled: false},
		{name: "not a command", prompt: "just some /text", handled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, handled := (&coordinator{}).handleBuiltinCommand(nil, "", tt.prompt)
			if handled != tt.handled {
				t.Errorf("handleBuiltinCommand(%q) handled=%v, want %v", tt.prompt, handled, tt.handled)
			}
		})
	}
}

func TestRejectInitClearsPending(t *testing.T) {
	c := &coordinator{pendingInit: &initializeResult{Content: "test"}}
	result, handled := c.rejectInit()

	if !handled {
		t.Error("rejectInit should be handled")
	}
	if !strings.Contains(result.Response, "放弃") {
		t.Errorf("rejectInit response = %q, should indicate rejection", result.Response)
	}
	if c.pendingInit != nil {
		t.Error("rejectInit should clear pendingInit")
	}
}

func TestConfirmInitWithoutPending(t *testing.T) {
	c := &coordinator{}
	result, handled := c.confirmInit(nil)

	if !handled {
		t.Error("confirmInit without pending should still be handled")
	}
	if !strings.Contains(result.Response, "没有待确认") {
		t.Errorf("response = %q, should say no pending content", result.Response)
	}
}

func TestConfirmInitWritesAgentsMDToProjectRoot(t *testing.T) {
	root := t.TempDir()
	content := "# AGENTS.md\n\n项目规则"
	c := &coordinator{pendingInit: &initializeResult{Content: content, ProjectRoot: root}}

	result, handled := c.confirmInit(nil)
	if !handled {
		t.Fatal("confirmInit should be handled")
	}

	path := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected AGENTS.md to be written at project root: %v", err)
	}
	if string(data) != content {
		t.Errorf("AGENTS.md content = %q, want %q", string(data), content)
	}
	if c.pendingInit != nil {
		t.Error("confirmInit should clear pendingInit after writing")
	}
	if !strings.Contains(result.Response, path) {
		t.Errorf("response = %q, should include target path %q", result.Response, path)
	}
}

func TestBuildInitPromptWithTruncation(t *testing.T) {
	// 构造一个超过2000字符的文件内容
	bigContent := strings.Repeat("x", 3000)
	contextFiles := []ContextFile{
		{Path: "bigfile.txt", Content: bigContent},
	}

	prompt := buildInitPrompt("/tmp/test", contextFiles, "")

	if strings.Contains(prompt, bigContent) {
		t.Error("prompt should not contain full large file content")
	}
	if !strings.Contains(prompt, "truncated") {
		t.Error("prompt should indicate truncation")
	}
}
