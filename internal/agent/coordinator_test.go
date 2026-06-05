package agent

import (
	"context"
	"encoding/json"
	"testing"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/permission"
)

// setupTestCoordinator 创建用于测试 handleToolCalls 的最小 coordinator
func setupTestCoordinator() *coordinator {
	r := tools.NewRegistry()
	r.Register(&testTool{
		name:        "view",
		description: "read file",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Output: "file content"}, nil
		},
	})
	r.Register(&testTool{
		name:        "failing_tool",
		description: "always fails",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "something went wrong"}, nil
		},
	})

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddBlacklist("rm")
	ps.AddBlacklist("sudo")
	ps.AddWhitelist("/workspace")

	return &coordinator{
		tools:      r,
		toolCaller: tools.NewToolCaller(r, ps),
		confirmFn:  func(toolName, arguments string) bool { return true },
	}
}

// testTool 实现 tools.Tool 接口的测试桩
type testTool struct {
	name        string
	description string
	execute     func(ctx context.Context, params json.RawMessage) (tools.Result, error)
}

func (t *testTool) Name() string                          { return t.name }
func (t *testTool) Description() string                   { return t.description }
func (t *testTool) Parameters() json.RawMessage            { return json.RawMessage(`{}`) }
func (t *testTool) Execute(ctx context.Context, p json.RawMessage) (tools.Result, error) {
	return t.execute(ctx, p)
}

func TestHandleToolCallsUnknownTool(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "nonexistent", Arguments: `{}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Role != RoleTool {
		t.Fatalf("expected RoleTool, got %s", results[0].Role)
	}
	if results[0].Content != "错误：未知工具 'nonexistent'" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsBlacklistDenial(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"command":"sudo rm -rf /"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "错误：操作被安全策略阻止" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsUserRejected(t *testing.T) {
	c := setupTestCoordinator()
	c.confirmFn = func(toolName, arguments string) bool { return false }

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/unknown/file.txt"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "操作被用户拒绝" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsNormalExecution(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/main.go"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "file content" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
	if results[0].ToolCallID != "call_1" {
		t.Fatalf("expected ToolCallID 'call_1', got '%s'", results[0].ToolCallID)
	}
}

func TestHandleToolCallsFailingTool(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "failing_tool", Arguments: `{}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "错误: something went wrong" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsMultipleTools(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/a.go"}`}},
		{ID: "call_2", Function: FunctionCall{Name: "nonexistent", Arguments: `{}`}},
		{ID: "call_3", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/b.go"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Content != "file content" {
		t.Fatalf("result 0: %s", results[0].Content)
	}
	if results[1].Content != "错误：未知工具 'nonexistent'" {
		t.Fatalf("result 1: %s", results[1].Content)
	}
	if results[2].Content != "file content" {
		t.Fatalf("result 2: %s", results[2].Content)
	}
}
