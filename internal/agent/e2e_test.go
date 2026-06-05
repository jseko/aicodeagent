package agent

import (
	"context"
	"fmt"
	"os"
	"testing"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/permission"
)

// TestEndToEndToolPipeline 端到端仿真：模拟用户提问 → LLM返回工具调用 → handleToolCalls执行 → 返回结果
// 这是当前阶段最接近实际使用场景的测试方式
func TestEndToEndToolPipeline(t *testing.T) {
	// 使用临时目录模拟工作区
	dir := t.TempDir()
	writeFile(t, dir+"/go.mod", "module example\n\ngo 1.21\n")

	// 1. 模拟应用启动时的依赖注入（与 app.go 一致）
	r := tools.NewRegistry()
	r.Register(tools.NewView(dir))

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddBlacklist("rm")
	ps.AddBlacklist("sudo")
	ps.AddBlacklist("/etc/passwd")
	ps.AddWhitelist("/workspace")

	tc := tools.NewToolCaller(r, ps)

	c := &coordinator{
		tools:      r,
		toolCaller: tc,
		confirmFn: func(toolName, arguments string) bool {
			fmt.Printf("[模拟TUI] 确认执行 %s(%s)? [Y/n]: Y\n", toolName, arguments)
			return true
		},
	}

	// 2. 模拟 LLM 返回的工具调用（等价于 function calling 解析结果）
	toolCalls := []ToolCall{
		{
			ID:   "call_001",
			Type: "function",
			Function: FunctionCall{
				Name:      "view",
				Arguments: `{"file_path":"go.mod"}`,
			},
		},
	}

	// 3. Coordinator 处理工具调用
	results, err := c.handleToolCalls(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("handleToolCalls 失败: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("期望1个结果，实际%d个", len(results))
	}

	// 4. 验证结果可被 LLM 消费（符合 OpenAI tool message 格式）
	result := results[0]
	if result.Role != RoleTool {
		t.Errorf("角色应为 tool，实际: %s", result.Role)
	}
	if result.ToolCallID != "call_001" {
		t.Errorf("ToolCallID 应为 call_001，实际: %s", result.ToolCallID)
	}

	t.Logf("端到端管道验证通过：%s", result.Content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSecurityPipeline 安全管道仿真测试
func TestSecurityPipeline(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(tools.NewView("."))
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddBlacklist("sudo")
	ps.AddBlacklist("/etc/passwd")

	tc := tools.NewToolCaller(r, ps)
	c := &coordinator{
		tools:      r,
		toolCaller: tc,
		confirmFn:  func(string, string) bool { return false },
	}

	tests := []struct {
		name     string
		call     ToolCall
		wantText string
	}{
		{
			name:     "黑名单拦截",
			call:     ToolCall{ID: "1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/etc/passwd"}`}},
			wantText: "错误：操作被安全策略阻止",
		},
		{
			name:     "未知工具",
			call:     ToolCall{ID: "2", Function: FunctionCall{Name: "delete_all", Arguments: `{}`}},
			wantText: "错误：未知工具 'delete_all'",
		},
		{
			name:     "用户拒绝未分类操作",
			call:     ToolCall{ID: "3", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/tmp/test"}`}},
			wantText: "操作被用户拒绝",
		},
		{
			name:     "sudo 拦截",
			call:     ToolCall{ID: "4", Function: FunctionCall{Name: "view", Arguments: `{"command":"sudo rm -rf /"}`}},
			wantText: "错误：操作被安全策略阻止",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := c.handleToolCalls(context.Background(), []ToolCall{tt.call})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Content != tt.wantText {
				t.Errorf("期望 '%s'，实际 '%s'", tt.wantText, results[0].Content)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
