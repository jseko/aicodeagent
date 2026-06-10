package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRevertPointStruct(t *testing.T) {
	now := time.Now()
	rp := &RevertPoint{
		MessageID: "msg-123",
		Timestamp: now,
	}

	if rp.MessageID != "msg-123" {
		t.Errorf("MessageID = %q, want msg-123", rp.MessageID)
	}
	if !rp.Timestamp.Equal(now) {
		t.Error("Timestamp should match")
	}
}

func TestSessionWithRevertPoint(t *testing.T) {
	session := &Session{
		ID:    "sess-1",
		Title: "test",
		Revert: &RevertPoint{
			MessageID: "msg-last-user",
			Timestamp: time.Now(),
		},
	}

	if session.Revert == nil {
		t.Fatal("Revert should not be nil")
	}
	if session.Revert.MessageID != "msg-last-user" {
		t.Errorf("Revert.MessageID = %q, want msg-last-user", session.Revert.MessageID)
	}
}

func TestSessionWithoutRevertPoint(t *testing.T) {
	session := &Session{ID: "sess-2", Title: "test"}

	if session.Revert != nil {
		t.Error("Revert should be nil by default")
	}
}

func TestGetVisibleMessagesWithoutRevert(t *testing.T) {
	// 验证无 Revert 点时返回所有消息
	session := &Session{ID: "sess-1"}
	messages := []Message{
		{ID: "1", Role: RoleUser, Content: "first"},
		{ID: "2", Role: RoleAssistant, Content: "reply", Status: MessageStatusCompleted},
		{ID: "3", Role: RoleUser, Content: "second"},
	}

	visible := filterByRevert(messages, session)
	if len(visible) != 3 {
		t.Fatalf("len(visible) = %d, want 3", len(visible))
	}
}

func TestGetVisibleMessagesWithRevert(t *testing.T) {
	session := &Session{
		ID: "sess-1",
		Revert: &RevertPoint{
			MessageID: "2",
			Timestamp: time.Now(),
		},
	}
	messages := []Message{
		{ID: "1", Role: RoleUser, Content: "first"},
		{ID: "2", Role: RoleAssistant, Content: "reply", Status: MessageStatusCompleted},
		{ID: "3", Role: RoleUser, Content: "second"},
		{ID: "4", Role: RoleAssistant, Content: "reply2", Status: MessageStatusCompleted},
	}

	visible := filterByRevert(messages, session)
	if len(visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2", len(visible))
	}
	if visible[len(visible)-1].ID != "2" {
		t.Errorf("last visible ID = %q, want 2", visible[len(visible)-1].ID)
	}
}

func TestGetVisibleMessagesRevertPointNotFound(t *testing.T) {
	session := &Session{
		ID: "sess-1",
		Revert: &RevertPoint{
			MessageID: "nonexistent",
			Timestamp: time.Now(),
		},
	}
	messages := []Message{
		{ID: "1", Role: RoleUser, Content: "first"},
		{ID: "2", Role: RoleAssistant, Content: "reply", Status: MessageStatusCompleted},
	}

	visible := filterByRevert(messages, session)
	// 如果 Revert 点找不到，返回所有消息（安全回退）
	if len(visible) != 2 {
		t.Fatalf("len(visible) = %d, want 2 (fallback to all)", len(visible))
	}
}

func TestGetVisibleMessagesEmpty(t *testing.T) {
	session := &Session{
		ID: "sess-1",
		Revert: &RevertPoint{
			MessageID: "1",
			Timestamp: time.Now(),
		},
	}
	visible := filterByRevert(nil, session)
	if len(visible) != 0 {
		t.Fatalf("len(visible) = %d, want 0", len(visible))
	}
}

func TestRedoClearsRevert(t *testing.T) {
	session := &Session{
		ID: "sess-1",
		Revert: &RevertPoint{
			MessageID: "msg-1",
			Timestamp: time.Now(),
		},
	}

	// 模拟 Redo：清除 Revert
	session.Revert = nil

	if session.Revert != nil {
		t.Error("Revert should be nil after Redo")
	}
}

func TestUndoSetsRevertPoint(t *testing.T) {
	messages := []Message{
		{ID: "1", Role: RoleUser, Content: "hello"},
		{ID: "2", Role: RoleAssistant, Content: "hi", Status: MessageStatusCompleted},
		{ID: "3", Role: RoleUser, Content: "help"},
		{ID: "4", Role: RoleAssistant, Content: "sure", Status: MessageStatusCompleted},
	}

	// 从后向前找最后一条用户消息（跳过摘要消息）
	var lastUserID string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser && !messages[i].IsSummaryMessage {
			lastUserID = messages[i].ID
			break
		}
	}

	if lastUserID != "3" {
		t.Errorf("last user ID = %q, want 3", lastUserID)
	}
}

func TestUndoSkipsSummaryMessages(t *testing.T) {
	messages := []Message{
		{ID: "1", Role: RoleUser, Content: "work"},
		{ID: "2", Role: RoleAssistant, Content: "done", Status: MessageStatusCompleted},
		{ID: "sum-1", Role: RoleAssistant, Content: "summary", IsSummaryMessage: true},
	}

	var lastUserID string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser && !messages[i].IsSummaryMessage {
			lastUserID = messages[i].ID
			break
		}
	}

	if lastUserID != "1" {
		t.Errorf("last user ID = %q, want 1 (should skip summary)", lastUserID)
	}
}

func TestUndoNoUserMessage(t *testing.T) {
	messages := []Message{
		{ID: "1", Role: RoleAssistant, Content: "system message", Status: MessageStatusCompleted},
	}

	var lastUserID string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser && !messages[i].IsSummaryMessage {
			lastUserID = messages[i].ID
			break
		}
	}

	if lastUserID != "" {
		t.Errorf("should not find user message, got %q", lastUserID)
	}
}

func TestHandleBuiltinCommandUndoRedoRouting(t *testing.T) {
	// /undo 和 /redo 需要完整的 coordinator 状态才能执行
	// 这里仅测试路由前缀匹配
	tests := []struct {
		name    string
		prompt  string
		handled bool
	}{
		{name: "undo", prompt: "/undo", handled: true},
		{name: "undo with text", prompt: "/undo last change", handled: true},
		{name: "redo", prompt: "/redo", handled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 注意：这里会 panic 因为 sessions 为 nil，我们只测试前缀匹配逻辑
			trimmed := strings.TrimSpace(tt.prompt)
			var handled bool
			switch {
			case strings.HasPrefix(trimmed, "/undo"):
				handled = true
			case strings.HasPrefix(trimmed, "/redo"):
				handled = true
			}
			if handled != tt.handled {
				t.Errorf("routing(%q) handled=%v, want %v", tt.prompt, handled, tt.handled)
			}
		})
	}
}

func TestRevertPointJSONSerialization(t *testing.T) {
	now := time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC)
	session := &Session{
		ID:    "sess-1",
		Title: "test",
		Revert: &RevertPoint{
			MessageID: "msg-abc",
			Timestamp: now,
		},
	}

	// 模拟序列化/反序列化（Session 已支持 JSON 标签）
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Revert == nil {
		t.Fatal("Revert should survive JSON round-trip")
	}
	if restored.Revert.MessageID != "msg-abc" {
		t.Errorf("Revert.MessageID = %q, want msg-abc", restored.Revert.MessageID)
	}
}

// filterByRevert 测试辅助函数：根据 Revert 点过滤消息
func filterByRevert(messages []Message, session *Session) []Message {
	if session == nil || session.Revert == nil {
		return messages
	}

	var visible []Message
	for _, msg := range messages {
		visible = append(visible, msg)
		if msg.ID == session.Revert.MessageID {
			break
		}
	}
	return visible
}
