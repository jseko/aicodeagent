package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventInterface(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		event     Event
		wantType  EventType
	}{
		{"UserMessage", UserMessage{SessionID: "s1", Content: "hello", Time: now}, TypeUserMessage},
		{"AgentThink chunk", AgentThink{SessionID: "s1", Content: "chunk", Time: now}, TypeAgentThink},
		{"AgentThink done", AgentThink{SessionID: "s1", IsDone: true, Time: now}, TypeAgentThink},
		{"ToolCall", ToolCall{SessionID: "s1", ToolName: "read", Time: now}, TypeToolCall},
		{"ToolResult", ToolResult{SessionID: "s1", ToolName: "read", Result: "ok", Time: now}, TypeToolResult},
		{"ErrorEvent", ErrorEvent{SessionID: "s1", Error: "fail", Time: now}, TypeError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type() != tt.wantType {
				t.Errorf("Type() = %s, want %s", tt.event.Type(), tt.wantType)
			}
			if !tt.event.Timestamp().Equal(now) {
				t.Errorf("Timestamp() = %v, want %v", tt.event.Timestamp(), now)
			}
		})
	}
}

func TestUserMessageJSON(t *testing.T) {
	now := time.Now()
	msg := UserMessage{
		SessionID: "sess_123",
		Content:   "hello world",
		Time:      now,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded UserMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID mismatch: %s != %s", decoded.SessionID, msg.SessionID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content mismatch: %s != %s", decoded.Content, msg.Content)
	}
}

func TestAgentThinkJSON(t *testing.T) {
	think := AgentThink{
		SessionID: "s1",
		Content:   "incremental chunk",
		IsDone:    false,
		Time:      time.Now(),
	}

	data, err := json.Marshal(think)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AgentThink
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.IsDone != think.IsDone {
		t.Errorf("IsDone mismatch: %v != %v", decoded.IsDone, think.IsDone)
	}
}

func TestToolCallJSON(t *testing.T) {
	call := ToolCall{
		SessionID: "s1",
		ToolName:  "read_file",
		Params:    json.RawMessage(`{"path":"/tmp/test.go"}`),
		Time:      time.Now(),
	}

	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ToolCall
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ToolName != "read_file" {
		t.Errorf("ToolName mismatch: %s", decoded.ToolName)
	}
	if string(decoded.Params) != `{"path":"/tmp/test.go"}` {
		t.Errorf("Params mismatch: %s", string(decoded.Params))
	}
}

func TestToolResultJSON(t *testing.T) {
	result := ToolResult{
		SessionID: "s1",
		ToolName:  "read_file",
		Result:    "file content here",
		Time:      time.Now(),
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Result != "file content here" {
		t.Errorf("Result mismatch: %s", decoded.Result)
	}
}

func TestErrorEventJSON(t *testing.T) {
	evt := ErrorEvent{
		SessionID: "s1",
		Error:     "LLM timeout",
		Time:      time.Now(),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ErrorEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Error != "LLM timeout" {
		t.Errorf("Error mismatch: %s", decoded.Error)
	}
}

func TestEventTypeConstants(t *testing.T) {
	// 确保常量值符合预期格式
	if TypeUserMessage != "user.message" {
		t.Errorf("TypeUserMessage = %s", TypeUserMessage)
	}
	if TypeAgentThink != "agent.think" {
		t.Errorf("TypeAgentThink = %s", TypeAgentThink)
	}
	if TypeToolCall != "tool.call" {
		t.Errorf("TypeToolCall = %s", TypeToolCall)
	}
	if TypeToolResult != "tool.result" {
		t.Errorf("TypeToolResult = %s", TypeToolResult)
	}
	if TypeError != "error" {
		t.Errorf("TypeError = %s", TypeError)
	}
}

func TestSubagentEvents(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		event    Event
		wantType EventType
	}{
		{
			"SubagentRoleSwitch",
			SubagentRoleSwitch{ParentSessionID: "p1", SubagentName: "code-reviewer", ChildSessionID: "c1", Time: now},
			TypeSubagentRoleSwitch,
		},
		{
			"SubagentRoleRestore",
			SubagentRoleRestore{ParentSessionID: "p1", SubagentName: "code-reviewer", Time: now},
			TypeSubagentRoleRestore,
		},
		{
			"SubagentToolCall",
			SubagentToolCall{ParentSessionID: "p1", SubagentName: "explore", ToolCallID: "tc1", ToolName: "read_file", Time: now},
			TypeSubagentToolCall,
		},
		{
			"SubagentToolResult",
			SubagentToolResult{ParentSessionID: "p1", SubagentName: "explore", ToolCallID: "tc1", ToolName: "read_file", Result: "ok", Time: now},
			TypeSubagentToolResult,
		},
		{
			"SubagentTaskComplete",
			SubagentTaskComplete{ParentSessionID: "p1", SubagentName: "code-reviewer", Summary: "done", Time: now},
			TypeSubagentTaskComplete,
		},
		{
			"SessionUpdate",
			SessionUpdate{SessionID: "s1", PromptTokens: 1000, Time: now},
			TypeSessionUpdate,
		},
		{
			"CancelTask",
			CancelTask{SessionID: "s1", Time: now},
			TypeCancelTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type() != tt.wantType {
				t.Errorf("Type() = %s, want %s", tt.event.Type(), tt.wantType)
			}
			if !tt.event.Timestamp().Equal(now) {
				t.Errorf("Timestamp() = %v, want %v", tt.event.Timestamp(), now)
			}
		})
	}
}

func TestSubagentEventsJSON(t *testing.T) {
	now := time.Now()

	t.Run("SubagentRoleSwitch", func(t *testing.T) {
		evt := SubagentRoleSwitch{
			ParentSessionID: "parent_1",
			SubagentName:    "code-reviewer",
			ChildSessionID:  "child_1",
			Time:            now,
		}
		data, err := json.Marshal(evt)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var decoded SubagentRoleSwitch
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if decoded.SubagentName != "code-reviewer" {
			t.Errorf("SubagentName = %s", decoded.SubagentName)
		}
	})

	t.Run("SubagentTaskComplete", func(t *testing.T) {
		evt := SubagentTaskComplete{
			ParentSessionID: "parent_1",
			SubagentName:    "explore",
			Summary:         "found 3 files",
			Time:            now,
		}
		data, err := json.Marshal(evt)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var decoded SubagentTaskComplete
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if decoded.Summary != "found 3 files" {
			t.Errorf("Summary = %s", decoded.Summary)
		}
	})
}
