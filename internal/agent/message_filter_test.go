package agent

import "testing"

func TestMessageShouldIncludeFiltersEmptyAndCancelledMessages(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
		want bool
	}{
		{name: "nil message", msg: nil, want: false},
		{name: "empty content", msg: &Message{Role: RoleUser}, want: false},
		{name: "whitespace content", msg: &Message{Role: RoleUser, Content: " \n\t "}, want: false},
		{name: "tool call without content", msg: &Message{Role: RoleTool, ToolCallID: "call-1"}, want: true},
		{name: "cancelled assistant", msg: &Message{Role: RoleAssistant, Content: "partial", Status: MessageStatusCancelled}, want: false},
		{name: "completed assistant", msg: &Message{Role: RoleAssistant, Content: "done", Status: MessageStatusCompleted}, want: true},
		{name: "user content", msg: &Message{Role: RoleUser, Content: "hello"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.ShouldInclude(); got != tt.want {
				t.Fatalf("ShouldInclude() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreparePromptFiltersMessagesAndPreservesOrder(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	msgs := []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleUser, Content: "   "},
		{Role: RoleAssistant, Content: "cancelled", Status: MessageStatusCancelled},
		{Role: RoleAssistant, Content: "second", Status: MessageStatusCompleted},
		{Role: RoleTool, ToolCallID: "call-1"},
		{Role: RoleUser, Content: "third"},
	}

	history := agent.preparePrompt(msgs)

	if len(history) != 4 {
		t.Fatalf("len(history) = %d, want 4", len(history))
	}

	wantRoles := []string{"user", "assistant", "tool", "user"}
	wantContents := []string{"first", "second", "", "third"}
	wantToolCallIDs := []string{"", "", "call-1", ""}

	for i := range history {
		if history[i].Role != wantRoles[i] {
			t.Fatalf("history[%d].Role = %q, want %q", i, history[i].Role, wantRoles[i])
		}
		if history[i].Content != wantContents[i] {
			t.Fatalf("history[%d].Content = %q, want %q", i, history[i].Content, wantContents[i])
		}
		if history[i].ToolCallID != wantToolCallIDs[i] {
			t.Fatalf("history[%d].ToolCallID = %q, want %q", i, history[i].ToolCallID, wantToolCallIDs[i])
		}
	}
}

func TestPreparePromptHandlesEmptyHistory(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}

	history := agent.preparePrompt(nil)

	if len(history) != 0 {
		t.Fatalf("len(history) = %d, want 0", len(history))
	}
}

func TestBuildContextKeepsOnlyCurrentSummaryMessage(t *testing.T) {
	agent := &sessionAgent{isSubAgent: true}
	session := &Session{SummaryMessageID: "summary-2"}
	messages := []Message{
		{ID: "summary-1", Role: RoleAssistant, Content: "old summary", IsSummaryMessage: true},
		{ID: "summary-2", Role: RoleAssistant, Content: "current summary", IsSummaryMessage: true},
		{ID: "user-1", Role: RoleUser, Content: "continue"},
	}

	contextMessages := agent.buildContext("system", session, messages)

	if len(contextMessages) != 3 {
		t.Fatalf("len(contextMessages) = %d, want 3", len(contextMessages))
	}
	if contextMessages[0].Role != "system" || contextMessages[0].Content != "system" {
		t.Fatalf("first message = %+v, want system prompt", contextMessages[0])
	}
	if contextMessages[1].Content != "current summary" {
		t.Fatalf("summary content = %q, want current summary", contextMessages[1].Content)
	}
	if contextMessages[2].Content != "continue" {
		t.Fatalf("user content = %q, want continue", contextMessages[2].Content)
	}
}
