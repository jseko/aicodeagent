package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"AICodeAgent/internal/agent"
	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/llm"
)

type fakeSessionAgent struct {
	call      agent.SessionAgentCall
	calls     []agent.SessionAgentCall
	response  string
	stream    chan llm.StreamingChunk
	responses []*agent.AgentResult
}

func (a *fakeSessionAgent) Run(ctx context.Context, call agent.SessionAgentCall) (*agent.AgentResult, error) {
	a.call = call
	a.calls = append(a.calls, call)
	if len(a.responses) > 0 {
		res := a.responses[0]
		a.responses = a.responses[1:]
		return res, nil
	}
	response := a.response
	if response == "" && a.stream == nil {
		response = "subagent summary"
		ch := make(chan llm.StreamingChunk, 2)
		ch <- llm.StreamingChunk{Content: response}
		ch <- llm.StreamingChunk{Done: true}
		close(ch)
		a.stream = ch
	}
	return &agent.AgentResult{Response: response, Session: call.Session, Stream: a.stream}, nil
}

func (a *fakeSessionAgent) SelectModel(complexity float64) *agent.Model { return nil }

type mockReadTool struct{}

func (mockReadTool) Name() string        { return "view" }
func (mockReadTool) Description() string { return "Read a file" }
func (mockReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`)
}
func (mockReadTool) Execute(ctx context.Context, params json.RawMessage) (tools.Result, error) {
	return tools.Result{Success: true, Output: "package agent\nfunc insecure() {}"}, nil
}

func TestExecutorRunsSessionAgentWithIsolatedMessages(t *testing.T) {
	registry := NewRegistry()
	sub := &Subagent{Config: &SubagentConfig{
		Name:        "reviewer",
		Description: "Use when reviewing code",
		Model:       "gpt-4o-mini",
		Tools:       map[string]bool{"view": true, "write": false},
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly},
	}, SystemPrompt: "review only"}
	if err := registry.Register(sub); err != nil {
		t.Fatalf("register: %v", err)
	}
	coord := NewCoordinator(registry)
	toolRegistry := tools.NewRegistry()
	runner := &fakeSessionAgent{}
	exec := NewExecutor(runner, registry, coord, toolRegistry)

	res, err := exec.Execute(context.Background(), ExecuteRequest{ParentSession: &agent.Session{ID: "parent"}, SubagentName: "reviewer", Input: "check this"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Summary != "subagent summary" || res.SubagentName != "reviewer" {
		t.Fatalf("unexpected result: %#v", res)
	}
	if len(runner.call.Messages) != 2 || runner.call.Messages[0].Role != "system" || runner.call.Messages[0].Content != "review only" {
		t.Fatalf("expected isolated system-first messages: %#v", runner.call.Messages)
	}
	if _, ctx := coord.Current(); ctx != nil {
		t.Fatalf("subagent context should be restored after execution")
	}
	if _, err := coord.ResumeSession(res.SessionID); err == nil {
		t.Fatalf("completed subagent session should not remain running")
	}
}

func TestExecutorReturnsOnlySummaryToParent(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&Subagent{Config: &SubagentConfig{
		Name:        "doc-writer",
		Description: "Use when writing docs",
		Model:       "gpt-4o-mini",
		Tools:       map[string]bool{"view": true},
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly},
	}, SystemPrompt: "write docs"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := &fakeSessionAgent{}
	exec := NewExecutor(runner, registry, NewCoordinator(registry), tools.NewRegistry())

	res, err := exec.Execute(context.Background(), ExecuteRequest{ParentID: "parent", SubagentName: "doc-writer", Input: "summarize"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Summary != "subagent summary" {
		t.Fatalf("expected summary only, got %#v", res)
	}
	if len(runner.call.Messages) == 0 {
		t.Fatalf("expected isolated messages")
	}
}

func TestExecutorUsesResponseWhenStreamIsNil(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&Subagent{Config: &SubagentConfig{
		Name:        "security-auditor",
		Description: "Use when auditing security",
		Model:       "gpt-4o-mini",
		Tools:       map[string]bool{"view": true},
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly},
	}, SystemPrompt: "audit security"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := &fakeSessionAgent{response: "security findings"}
	exec := NewExecutor(runner, registry, NewCoordinator(registry), tools.NewRegistry())

	res, err := exec.Execute(context.Background(), ExecuteRequest{ParentID: "parent", SubagentName: "security-auditor", Input: "audit file"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Summary != "security findings" {
		t.Fatalf("expected response fallback, got %#v", res)
	}
}

func TestExecutorRejectsEmptyResponse(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&Subagent{Config: &SubagentConfig{
		Name:        "security-auditor",
		Description: "Use when auditing security",
		Model:       "gpt-4o-mini",
		Tools:       map[string]bool{"view": true},
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly},
	}, SystemPrompt: "audit security"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	coord := NewCoordinator(registry)
	runner := &fakeSessionAgent{}
	runner.stream = make(chan llm.StreamingChunk, 1)
	runner.stream <- llm.StreamingChunk{Done: true}
	close(runner.stream)
	exec := NewExecutor(runner, registry, coord, tools.NewRegistry())

	_, err := exec.Execute(context.Background(), ExecuteRequest{ParentID: "parent", SubagentName: "security-auditor", Input: "audit file"})
	if err == nil {
		t.Fatalf("expected empty response error")
	}
	if _, ctx := coord.Current(); ctx != nil {
		t.Fatalf("subagent context should be restored after failed execution")
	}
}

func TestExecutorExecutesAllowedToolCalls(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&Subagent{Config: &SubagentConfig{
		Name:        "security-auditor",
		Description: "Use when auditing security",
		Model:       "gpt-4o-mini",
		Tools:       map[string]bool{"view": true},
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly},
	}, SystemPrompt: "audit security"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(mockReadTool{})
	first := &agent.AgentResult{}
	first.ToolCalls = []llm.ToolCallDelta{{ID: "call-1", Type: "function"}}
	first.ToolCalls[0].Function.Name = "view"
	first.ToolCalls[0].Function.Arguments = `{"file_path":"internal/agent/coordinator.go"}`
	runner := &fakeSessionAgent{responses: []*agent.AgentResult{
		first,
		{Response: "发现 1 个低危问题：示例函数缺少上下文校验。"},
	}}
	exec := NewExecutor(runner, registry, NewCoordinator(registry), toolRegistry)

	res, err := exec.Execute(context.Background(), ExecuteRequest{ParentID: "parent", SubagentName: "security-auditor", Input: "audit file"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Summary, "低危问题") {
		t.Fatalf("expected final audit summary, got %#v", res)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected two LLM calls, got %d", len(runner.calls))
	}
	if len(runner.calls[0].Tools) != 1 {
		t.Fatalf("expected view tool definition, got %#v", runner.calls[0].Tools)
	}
	lastMessages := runner.calls[1].Messages
	if len(lastMessages) == 0 || lastMessages[len(lastMessages)-1].Role != "tool" || !strings.Contains(lastMessages[len(lastMessages)-1].Content, "package agent") {
		t.Fatalf("expected tool result in second call messages: %#v", lastMessages)
	}
}

var _ agent.SessionAgent = (*fakeSessionAgent)(nil)
var _ = llm.Message{}
