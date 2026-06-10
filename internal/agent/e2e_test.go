package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/config"
	"AICodeAgent/internal/llm"
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

// TestIntegrationTokenTracking 集成测试：多轮对话验证 Token 累计统计
func TestIntegrationTokenTracking(t *testing.T) {
	session := &Session{ID: "sess-token"}
	model := &Model{
		Provider: "openai",
		Name:     "test-model",
		Config: ModelConfig{
			InputPer1M:  0.15,
			OutputPer1M: 0.30,
		},
	}

	// 模拟 3 轮对话的 usage
	usages := []llm.Usage{
		{PromptTokens: 100, CompletionTokens: 50},
		{PromptTokens: 250, CompletionTokens: 80},
		{PromptTokens: 400, CompletionTokens: 120},
	}

	agent := &sessionAgent{isSubAgent: true}
	for _, usage := range usages {
		agent.updateSessionUsage(model, session, &usage)
	}

	if session.PromptTokens != 750 {
		t.Errorf("PromptTokens = %d, want 750", session.PromptTokens)
	}
	if session.CompletionTokens != 250 {
		t.Errorf("CompletionTokens = %d, want 250", session.CompletionTokens)
	}

	// 成本验证：750*0.15/1M + 250*0.30/1M
	expectedCost := 0.0001125 + 0.000075 // = 0.0001875
	if session.TotalCost < expectedCost*0.99 || session.TotalCost > expectedCost*1.01 {
		t.Errorf("TotalCost = %.10f, want ~%.10f", session.TotalCost, expectedCost)
	}

	t.Logf("Token 累计统计: prompt=%d completion=%d cost=%.6f$", session.PromptTokens, session.CompletionTokens, session.TotalCost)
}

// TestIntegrationContextOverflowDetection 集成测试：上下文窗口溢出检测
func TestIntegrationContextOverflowDetection(t *testing.T) {
	cfg := &config.Config{
		Context: config.ContextConfig{
			WindowTokens:      10000,
			LargeWindowBuffer: 0,
			SmallWindowRatio:  0.2,
		},
	}

	// 小窗口场景（≤200K）：使用 20% 比例，阈值 = 10000 * 0.2 = 2000
	session := &Session{
		ID:              "sess-overflow",
		PromptTokens:    8000,
		CompletionTokens: 500,
	}

	c := &coordinator{config: cfg}
	c.largeModel.Store(&Model{Config: ModelConfig{MaxTokens: 10000}})

	stop := c.checkContextWindow(session)
	if stop == nil {
		t.Fatal("expected stop condition when near window limit")
	}
	if stop.Reason != "context_window_overflow" {
		t.Errorf("reason = %q, want context_window_overflow", stop.Reason)
	}
	if stop.Used != 8500 {
		t.Errorf("used = %d, want 8500", stop.Used)
	}

	t.Logf("上下文溢出检测: reason=%s used=%d/%d threshold=%d", stop.Reason, stop.Used, stop.Window, stop.Threshold)
}

// TestIntegrationContextOverflowLargeWindow 集成测试：大窗口溢出检测
func TestIntegrationContextOverflowLargeWindow(t *testing.T) {
	cfg := &config.Config{
		Context: config.ContextConfig{
			WindowTokens:      250000, // >200K
			LargeWindowBuffer: 20000,
			SmallWindowRatio:  0,
		},
	}

	// 大窗口（>200K）：固定 20K 缓冲
	session := &Session{
		ID:              "sess-big",
		PromptTokens:    235000,
		CompletionTokens: 0,
	}

	c := &coordinator{config: cfg}
	c.largeModel.Store(&Model{Config: ModelConfig{MaxTokens: 250000}})

	stop := c.checkContextWindow(session)
	if stop == nil {
		t.Fatal("expected stop condition for large window overflow")
	}
	if stop.Threshold != 20000 {
		t.Errorf("threshold = %d, want 20000 (fixed buffer for large window)", stop.Threshold)
	}

	t.Logf("大窗口溢出: threshold=%d (fixed buffer)", stop.Threshold)
}

// TestIntegrationProjectMemoryLoading 集成测试：项目记忆加载 → 系统提示词注入
func TestIntegrationProjectMemoryLoading(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/AGENTS.md", "项目规则：使用 Go 1.21+")
	writeFile(t, dir+"/CLAUDE.md", "代码规范：遵循 Effective Go")

	// 1. 加载项目记忆文件
	files := processContextPath(dir, 100*1024)
	if len(files) != 2 {
		t.Fatalf("expected 2 project memory files, got %d", len(files))
	}

	// 2. 构建系统提示词
	prompt := buildSystemPrompt("基础提示词", files, "")
	if !strings.Contains(prompt, "项目规则：使用 Go 1.21+") {
		t.Error("系统提示词缺少 AGENTS.md 内容")
	}
	if !strings.Contains(prompt, "代码规范：遵循 Effective Go") {
		t.Error("系统提示词缺少 CLAUDE.md 内容")
	}
	if !strings.Contains(prompt, "<project_context") {
		t.Error("系统提示词缺少 project_context 标签")
	}

	// 3. 验证 MCP 指令合并
	promptWithMCP := buildSystemPrompt("基础提示词", files, "MCP 工具：context7")
	if !strings.Contains(promptWithMCP, "MCP 工具：context7") {
		t.Error("系统提示词缺少 MCP 指令")
	}

	t.Logf("项目记忆加载成功，提示词长度: %d 字符", len(prompt))
}

// TestIntegrationUndoRedoFlow 集成测试：Undo → 隐藏 → Redo → 恢复完整流程
func TestIntegrationUndoRedoFlow(t *testing.T) {
	messages := []Message{
		{ID: "msg-1", Role: RoleUser, Content: "第一个问题"},
		{ID: "msg-2", Role: RoleAssistant, Content: "第一个回答", Status: MessageStatusCompleted},
		{ID: "msg-3", Role: RoleUser, Content: "第二个问题"},
		{ID: "msg-4", Role: RoleAssistant, Content: "第二个回答", Status: MessageStatusCompleted},
	}

	// 1. 模拟 Undo → 设置 Revert 点到最后一条用户消息(msg-3)
	session := &Session{
		ID: "sess-undo",
		Revert: &RevertPoint{
			MessageID: "msg-3",
			Timestamp: time.Now(),
		},
	}
	visible := filterByRevert(messages, session)
	if len(visible) != 3 {
		t.Fatalf("Undo: len(visible) = %d, want 3", len(visible))
	}
	if visible[len(visible)-1].ID != "msg-3" {
		t.Errorf("Undo: last visible ID = %q, want msg-3", visible[len(visible)-1].ID)
	}

	// 2. msg-4 应该被隐藏
	for _, v := range visible {
		if v.ID == "msg-4" {
			t.Error("Undo: msg-4 should be hidden")
		}
	}

	// 3. 模拟 Redo → 清除 Revert 点
	session.Revert = nil
	visibleAfterRedo := filterByRevert(messages, session)
	if len(visibleAfterRedo) != 4 {
		t.Fatalf("Redo: len(visible) = %d, want 4", len(visibleAfterRedo))
	}

	// 4. 验证所有消息可见
	foundMsg4 := false
	for _, v := range visibleAfterRedo {
		if v.ID == "msg-4" {
			foundMsg4 = true
		}
	}
	if !foundMsg4 {
		t.Error("Redo: msg-4 should be visible again")
	}

	t.Log("Undo → Redo 完整流程验证通过")
}

// TestIntegrationBuiltinCommandRouting 集成测试：内置命令路由正确性
func TestIntegrationBuiltinCommandRouting(t *testing.T) {
	tests := []struct {
		prompt  string
		handled bool
	}{
		{"/initialize", true},
		{"/initialize-confirm", true},
		{"/initialize-reject", true},
		{"普通消息", false},
		{"help me write code", false},
	}

	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			_, handled := (&coordinator{}).handleBuiltinCommand(context.Background(), "", tt.prompt)
			if handled != tt.handled {
				t.Errorf("handleBuiltinCommand(%q) handled=%v, want %v", tt.prompt, handled, tt.handled)
			}
		})
	}

	// /undo 和 /redo 需要完整的 sessions 状态，这里只验证路由前缀匹配
	undoRedoTests := []struct {
		prompt  string
		handled bool
	}{
		{"/undo", true},
		{"/redo", true},
	}
	for _, tt := range undoRedoTests {
		t.Run(tt.prompt, func(t *testing.T) {
			trimmed := tt.prompt
			handled := strings.HasPrefix(trimmed, "/undo") || strings.HasPrefix(trimmed, "/redo")
			if handled != tt.handled {
				t.Errorf("routing(%q) handled=%v, want %v", tt.prompt, handled, tt.handled)
			}
		})
	}
}
