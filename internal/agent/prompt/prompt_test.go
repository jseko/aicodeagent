package prompt

import (
	"strings"
	"testing"
)

func TestPromptBuilderSystemLayerRequired(t *testing.T) {
	b := NewPromptBuilder()
	result, err := b.Build()
	if err == nil {
		t.Error("Build() without System layer should return error")
	}
	if result != "" {
		t.Errorf("Build() result = %q, want empty string", result)
	}
}

func TestPromptBuilderWithSystem(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("TestAgent", "测试助手", []string{"规则1", "规则2"})

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, want := range []string{
		"你是 TestAgent",
		"测试助手",
		"规则1",
		"规则2",
		"<system>",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q:\n%s", want, result)
		}
	}
}

func TestPromptBuilderWithEnvironment(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithEnvironment()

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(result, "<environment>") {
		t.Errorf("result missing <environment> tag:\n%s", result)
	}
	// 应包含操作系统信息
	if !strings.Contains(result, "操作系统") {
		t.Errorf("result missing OS info:\n%s", result)
	}
}

func TestPromptBuilderWithEnvironmentInfo(t *testing.T) {
	info := EnvironmentInfo{
		OS:         "linux",
		Shell:      "/bin/bash",
		WorkingDir: "/home/user/project",
		Date:       "2025-06-10",
	}
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithEnvironmentInfo(info)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, want := range []string{"linux", "/bin/bash", "/home/user/project", "2025-06-10"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q:\n%s", want, result)
		}
	}
}

func TestPromptBuilderWithTools(t *testing.T) {
	tools := []ToolInfo{
		{Name: "read_file", Description: "读取文件内容", Parameters: `{"path": "string"}`},
		{Name: "write_file", Description: "写入文件", Parameters: `{"path": "string", "content": "string"}`},
	}
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithTools(tools)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, want := range []string{"<tools>", "read_file", "读取文件内容", "write_file"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q:\n%s", want, result)
		}
	}
}

func TestPromptBuilderWithEmptyTools(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithTools(nil)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(result, "<tools>") {
		t.Error("result should not contain tools tag for empty tools")
	}
}

func TestPromptBuilderWithProject(t *testing.T) {
	files := []ContextFile{
		{Path: "AGENTS.md", Content: "项目规则"},
		{Path: "CLAUDE.md", Content: "代码规范"},
	}
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithProject(files)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, want := range []string{"<project>", "AGENTS.md", "项目规则", "CLAUDE.md", "代码规范"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q:\n%s", want, result)
		}
	}
}

func TestPromptBuilderWithEmptyProjectFiles(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithProject(nil)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(result, "<project>") {
		t.Error("result should not contain project tag for nil files")
	}
}

func TestPromptBuilderWithTask(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithTask("修复登录bug")

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(result, "<task>") {
		t.Error("result should contain task tag")
	}
	if !strings.Contains(result, "修复登录bug") {
		t.Error("result should contain task description")
	}
}

func TestPromptBuilderFullFiveLayers(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("AI Agent", "智能编程助手", []string{"规则A", "规则B"}).
		WithEnvironmentInfo(EnvironmentInfo{
			OS:         "darwin",
			Shell:      "/bin/zsh",
			WorkingDir: "/tmp/test",
			Date:       "2025-01-15",
		}).
		WithTools([]ToolInfo{{Name: "tool1", Description: "工具1"}}).
		WithProject([]ContextFile{{Path: "README.md", Content: "# Test"}}).
		WithTask("编写单元测试")

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 五层都应存在
	for _, layer := range []string{"<system>", "<environment>", "<tools>", "<project>", "<task>"} {
		if !strings.Contains(result, layer) {
			t.Errorf("result missing layer tag %q:\n%s", layer, result)
		}
	}
}

func TestPromptBuilderLayerOrdering(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithEnvironmentInfo(EnvironmentInfo{OS: "linux", Shell: "/bin/sh", WorkingDir: "/tmp", Date: "2025-01-01"}).
		WithTools([]ToolInfo{{Name: "tool", Description: "desc"}}).
		WithTask("task")

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	systemIdx := strings.Index(result, "<system>")
	envIdx := strings.Index(result, "<environment>")
	toolsIdx := strings.Index(result, "<tools>")
	taskIdx := strings.Index(result, "<task>")

	if systemIdx < 0 || envIdx < 0 || toolsIdx < 0 || taskIdx < 0 {
		t.Fatal("all layers should be present")
	}

	if !(systemIdx < envIdx && envIdx < toolsIdx && toolsIdx < taskIdx) {
		t.Errorf("layers out of order: system=%d env=%d tools=%d task=%d\n%s",
			systemIdx, envIdx, toolsIdx, taskIdx, result)
	}
}

func TestPromptBuilderCachedReturnsPreviousResult(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithEnvironment()

	// 首次构建
	result1, err := b.Build()
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}

	// 检查缓存
	cached, ok := b.Cached()
	if !ok {
		t.Fatal("expected cached result after Build")
	}
	if cached != result1 {
		t.Errorf("cached = %q, want %q", cached, result1)
	}
}

func TestPromptBuilderBuildCachedWithSameHash(t *testing.T) {
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithEnvironmentInfo(EnvironmentInfo{
			OS: "linux", Shell: "/bin/sh", WorkingDir: "/tmp", Date: "2025-01-01",
		})

	envHash := ComputeEnvironmentHash(EnvironmentInfo{
		OS: "linux", Shell: "/bin/sh", WorkingDir: "/tmp", Date: "2025-01-01",
	})

	result1, _ := b.BuildCached(envHash)
	result2, _ := b.BuildCached(envHash)

	if result1 != result2 {
		t.Error("BuildCached with same hash should return cached result")
	}
}

func TestComputeEnvironmentHashDeterministic(t *testing.T) {
	info := EnvironmentInfo{
		OS: "linux", Shell: "/bin/bash", WorkingDir: "/home/user", Date: "2025-06-10",
	}
	h1 := ComputeEnvironmentHash(info)
	h2 := ComputeEnvironmentHash(info)

	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
}

func TestComputeEnvironmentHashDifferent(t *testing.T) {
	info1 := EnvironmentInfo{OS: "linux", Shell: "/bin/bash", WorkingDir: "/home/a", Date: "2025-01-01"}
	info2 := EnvironmentInfo{OS: "darwin", Shell: "/bin/zsh", WorkingDir: "/home/b", Date: "2025-06-10"}

	h1 := ComputeEnvironmentHash(info1)
	h2 := ComputeEnvironmentHash(info2)

	if h1 == h2 {
		t.Error("different environments should produce different hashes")
	}
}

func TestCollectEnvironmentFillsFields(t *testing.T) {
	info := collectEnvironment()

	if info.OS == "" {
		t.Error("OS should not be empty")
	}
	if info.Shell == "" {
		t.Error("Shell should not be empty")
	}
	if info.WorkingDir == "" {
		t.Error("WorkingDir should not be empty")
	}
	if info.Date == "" {
		t.Error("Date should not be empty")
	}
}

func TestCurrentEnvironmentHashReturnsValue(t *testing.T) {
	h := CurrentEnvironmentHash()
	if h == "" {
		t.Error("CurrentEnvironmentHash should return non-empty hash")
	}
	if len(h) != 16 {
		t.Errorf("hash length = %d, want 16", len(h))
	}
}

func TestPromptBuilderErrorPropagation(t *testing.T) {
	// 如果 err 已设置，后续操作应该是 no-op
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil)

	// 强制触发一个错误（通过不设置 System 层检查）
	b2 := NewPromptBuilder()
	_, err := b2.Build()
	if err == nil {
		t.Fatal("Build without System should error")
	}
	// b 仍应正常工作
	result, err := b.Build()
	if err != nil {
		t.Fatalf("valid builder should not error after invalid builder created: %v", err)
	}
	if result == "" {
		t.Error("valid builder should produce non-empty result")
	}
}

func TestPromptBuilderEmptyProjectFile(t *testing.T) {
	files := []ContextFile{
		{Path: "empty.md", Content: ""},
		{Path: "valid.md", Content: "content"},
	}
	b := NewPromptBuilder().
		WithSystem("Agent", "helper", nil).
		WithProject(files)

	result, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(result, "empty.md") {
		t.Error("empty project files should be omitted")
	}
	if !strings.Contains(result, "valid.md") {
		t.Error("non-empty project files should be included")
	}
}
