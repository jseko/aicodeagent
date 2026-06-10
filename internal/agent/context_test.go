package agent

import (
	"testing"
)

func TestBuildContextWithSystemPrompt(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	messages := []Message{
		{Role: RoleUser, Content: "hello"},
	}

	result := agent.buildContext("You are a helpful assistant", nil, messages)

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("first message role = %q, want system", result[0].Role)
	}
	if result[0].Content != "You are a helpful assistant" {
		t.Errorf("system prompt = %q, want %q", result[0].Content, "You are a helpful assistant")
	}
}

func TestBuildContextWithoutSystemPrompt(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	messages := []Message{
		{Role: RoleUser, Content: "hello"},
	}

	result := agent.buildContext("", nil, messages)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("only message role = %q, want user", result[0].Role)
	}
}

func TestBuildContextDeduplicatesSummaryMessages(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	session := &Session{SummaryMessageID: "summary-current"}
	messages := []Message{
		{ID: "summary-old", Role: RoleAssistant, Content: "old summary", IsSummaryMessage: true},
		{ID: "summary-current", Role: RoleAssistant, Content: "current summary", IsSummaryMessage: true},
		{Role: RoleUser, Content: "continue work"},
	}

	result := agent.buildContext("system", session, messages)

	// 应包含: system, current-summary, user message = 3条
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	if result[1].Content != "current summary" {
		t.Errorf("summary content = %q, want current summary", result[1].Content)
	}
}

func TestBuildContextFiltersEmptyMessages(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	messages := []Message{
		{Role: RoleUser, Content: ""},
		{Role: RoleUser, Content: "real message"},
		{Role: RoleAssistant, Content: "   ", Status: MessageStatusCompleted},
	}

	result := agent.buildContext("", nil, messages)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Content != "real message" {
		t.Errorf("content = %q, want real message", result[0].Content)
	}
}

func TestBuildContextFiltersCancelledMessages(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	messages := []Message{
		{Role: RoleUser, Content: "request"},
		{Role: RoleAssistant, Content: "cancelled reply", Status: MessageStatusCancelled},
		{Role: RoleAssistant, Content: "actual reply", Status: MessageStatusCompleted},
	}

	result := agent.buildContext("", nil, messages)

	found := false
	for _, m := range result {
		if m.Content == "cancelled reply" {
			found = true
		}
	}
	if found {
		t.Error("cancelled messages should be filtered out")
	}
}

func TestBuildContextPreservesToolMessages(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	messages := []Message{
		{Role: RoleTool, ToolCallID: "call-1", Content: "tool result"},
	}

	result := agent.buildContext("", nil, messages)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Role != "tool" {
		t.Errorf("role = %q, want tool", result[0].Role)
	}
	if result[0].ToolCallID != "call-1" {
		t.Errorf("ToolCallID = %q, want call-1", result[0].ToolCallID)
	}
}

func TestBuildContextEmptyMessages(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}

	result := agent.buildContext("system", nil, nil)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1 (system only)", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("role = %q, want system", result[0].Role)
	}
}

func TestBuildContextAllSummaryMessageTypes(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	session := &Session{SummaryMessageID: "keep-me"}
	messages := []Message{
		{ID: "discard-1", Role: RoleAssistant, Content: "old summary 1", IsSummaryMessage: true},
		{ID: "keep-me", Role: RoleAssistant, Content: "current summary", IsSummaryMessage: true},
		{ID: "discard-2", Role: RoleAssistant, Content: "old summary 2", IsSummaryMessage: true},
		{Role: RoleUser, Content: "actual work"},
	}

	result := agent.buildContext("system", session, messages)

	// 应有: system, current-summary, user message = 3条
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// 验证只有当前摘要被保留
	summaryCount := 0
	for _, m := range result {
		if m.IsSummaryMessage {
			summaryCount++
			if m.Content != "current summary" {
				t.Errorf("only current summary should be present, got %q", m.Content)
			}
		}
	}
	if summaryCount != 1 {
		t.Errorf("expected 1 summary message, got %d", summaryCount)
	}
}

func TestBuildContextNoSummaryWhenIDEmpty(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	session := &Session{SummaryMessageID: ""}
	messages := []Message{
		{ID: "sum-1", Role: RoleAssistant, Content: "summary", IsSummaryMessage: true},
		{Role: RoleUser, Content: "work"},
	}

	result := agent.buildContext("system", session, messages)

	// 不应该包含摘要消息（SummaryMessageID 为空）
	for _, m := range result {
		if m.IsSummaryMessage {
			t.Error("summary messages should be filtered when SummaryMessageID is empty")
		}
	}
}
