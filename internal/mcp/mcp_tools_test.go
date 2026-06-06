package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"AICodeAgent/internal/agent/tools"
)

func TestMCPToolName(t *testing.T) {
	tool := &Tool{Name: "list_issues", Description: "List GitHub issues"}
	mcpTool := NewMCPTool("github", tool, nil)

	expected := "mcp_github_list_issues"
	if mcpTool.Name() != expected {
		t.Fatalf("expected name '%s', got '%s'", expected, mcpTool.Name())
	}
}

func TestMCPToolDescription(t *testing.T) {
	tool := &Tool{Name: "search", Description: "Search code"}
	mcpTool := NewMCPTool("filesystem", tool, nil)

	expected := "[MCP:filesystem] Search code"
	if mcpTool.Description() != expected {
		t.Fatalf("expected description '%s', got '%s'", expected, mcpTool.Description())
	}
}

func TestMCPToolParameters(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
		},
		"required": []string{"query"},
	}
	tool := &Tool{
		Name:        "search",
		Description: "Search code",
		InputSchema: schema,
	}
	mcpTool := NewMCPTool("test", tool, nil)

	params := mcpTool.Parameters()
	if len(params) == 0 {
		t.Fatal("expected non-empty parameters")
	}

	var parsed map[string]any
	if err := json.Unmarshal(params, &parsed); err != nil {
		t.Fatalf("parse parameters: %v", err)
	}
	if parsed["type"] != "object" {
		t.Fatalf("expected type 'object', got '%v'", parsed["type"])
	}
}

func TestMCPToolParametersEmpty(t *testing.T) {
	tool := &Tool{Name: "simple", Description: "No params"}
	mcpTool := NewMCPTool("test", tool, nil)

	params := mcpTool.Parameters()
	expected := `{"type":"object","properties":{}}`
	if string(params) != expected {
		t.Fatalf("expected '%s', got '%s'", expected, string(params))
	}
}

func TestMCPToolImplementsToolInterface(t *testing.T) {
	tool := &Tool{Name: "test", Description: "test tool"}
	mcpTool := NewMCPTool("server", tool, nil)

	var _ tools.Tool = mcpTool // 编译期接口验证
}

func TestGetMCPTools(t *testing.T) {
	rawTools := []*Tool{
		{Name: "tool1", Description: "First"},
		{Name: "tool2", Description: "Second"},
		{Name: "tool3", Description: "Third"},
	}

	result := GetMCPTools("test-server", rawTools, nil)

	if len(result) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(result))
	}
	for i, tp := range result {
		expectedName := "mcp_test-server_" + rawTools[i].Name
		if tp.Name() != expectedName {
			t.Fatalf("tool[%d]: expected name '%s', got '%s'", i, expectedName, tp.Name())
		}
	}
}

func TestMCPToolExecuteWithNilClient(t *testing.T) {
	tool := &Tool{Name: "test", Description: "test"}
	mcpTool := NewMCPTool("server", tool, nil)

	ctx := context.Background()
	// nil client should cause a panic/error, verify by recovering
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with nil client")
		} else {
			t.Logf("nil client caused expected panic: %v", r)
		}
	}()
	mcpTool.Execute(ctx, json.RawMessage(`{}`))
}

func TestMCPToolExecuteWithParams(t *testing.T) {
	tool := &Tool{
		Name:        "echo",
		Description: "Echo tool",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
	}
	mcpTool := NewMCPTool("test", tool, nil)

	// nil client causes panic, verify panic recovery
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with nil client")
		} else {
			t.Logf("nil client caused expected panic: %v", r)
		}
	}()

	ctx := context.Background()
	mcpTool.Execute(ctx, json.RawMessage(`{"message":"hello"}`))
}
