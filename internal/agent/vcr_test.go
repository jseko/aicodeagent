package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/permission"
)

// TestCoderAgent_ReadFile 集成测试：验证 CoderAgent 通过 view 工具读取文件
// VCR 模式：从 testdata 读取预录制的工具调用参数，在测试中重放
func TestCoderAgent_ReadFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 模拟应用启动
	r := tools.NewRegistry()
	r.Register(tools.NewView(dir))

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist(dir)

	c := &coordinator{
		tools:       r,
		permService: ps,
		toolCaller:  tools.NewToolCaller(r, ps),
		confirmFn:   func(toolName, arguments string) bool { return true },
	}

	// 模拟 VCR 录制的工具调用（等价于 LLM function calling 返回）
	toolCalls := []ToolCall{
		{
			ID:   "call_read_001",
			Type: "function",
			Function: FunctionCall{
				Name:      "view",
				Arguments: `{"file_path":"main.go"}`,
			},
		},
	}

	results, err := c.handleToolCalls(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("handleToolCalls: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Role != RoleTool {
		t.Errorf("role = %s, want tool", result.Role)
	}
	if result.ToolCallID != "call_read_001" {
		t.Errorf("tool_call_id = %s, want call_read_001", result.ToolCallID)
	}

	// 验证文件内容出现在结果中
	if result.Content == "" {
		t.Error("expected file content in result")
	}
	t.Logf("ReadFile result: %s", result.Content)
}

// TestCoderAgent_WriteFile 集成测试：验证 CoderAgent 通过 write 工具写入文件
func TestCoderAgent_WriteFile(t *testing.T) {
	dir := t.TempDir()
	targetFile := filepath.Join(dir, "output.go")

	r := tools.NewRegistry()
	r.Register(tools.NewWrite(dir))

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist(dir)

	c := &coordinator{
		tools:       r,
		permService: ps,
		toolCaller:  tools.NewToolCaller(r, ps),
		confirmFn:   func(toolName, arguments string) bool { return true },
	}

	// 模拟 LLM 返回的 write_file 工具调用
	content := "package output\n\nfunc Greet() string { return \"hi\" }\n"
	params, _ := json.Marshal(map[string]string{
		"file_path": targetFile,
		"content":   content,
	})

	toolCalls := []ToolCall{
		{
			ID:   "call_write_001",
			Type: "function",
			Function: FunctionCall{
				Name:      "write",
				Arguments: string(params),
			},
		},
	}

	results, err := c.handleToolCalls(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("handleToolCalls: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Role != RoleTool {
		t.Errorf("role = %s, want tool", result.Role)
	}

	// 验证文件已写入
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content mismatch:\nwant: %s\ngot:  %s", content, string(data))
	}

	t.Logf("WriteFile success, file written: %s", targetFile)
}

// TestCoderAgent_ToolCallAssertion 验证工具调用的参数传递和结果格式正确性
func TestCoderAgent_ToolCallAssertion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Project\n"), 0o644)

	r := tools.NewRegistry()
	r.Register(tools.NewView(dir))
	r.Register(tools.NewGrep(dir))

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist(dir)

	c := &coordinator{
		tools:       r,
		permService: ps,
		toolCaller:  tools.NewToolCaller(r, ps),
		confirmFn:   func(toolName, arguments string) bool { return true },
	}

	// 多工具调用场景（模拟 LLM 同时请求 read + grep）
	toolCalls := []ToolCall{
		{
			ID:   "call_v_001",
			Type: "function",
			Function: FunctionCall{
				Name:      "view",
				Arguments: `{"file_path":"readme.md"}`,
			},
		},
		{
			ID:   "call_g_001",
			Type: "function",
			Function: FunctionCall{
				Name:      "grep",
				Arguments: `{"pattern":"Project","path":"."}`,
			},
		},
	}

	results, err := c.handleToolCalls(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("handleToolCalls: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// 验证每条结果格式正确
	for i, result := range results {
		if result.Role != RoleTool {
			t.Errorf("result[%d].Role = %s, want tool", i, result.Role)
		}
		if result.ToolCallID == "" {
			t.Errorf("result[%d].ToolCallID is empty", i)
		}
	}

	// 验证 view 返回了文件内容
	if !contains(results[0].Content, "Project") {
		t.Error("view result should contain file content")
	}

	t.Logf("Multi-tool call assertion passed: %d results", len(results))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
