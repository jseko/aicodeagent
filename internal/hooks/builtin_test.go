package hooks

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSensitiveFileBlockerDeniesEnvFile(t *testing.T) {
	blocker := NewSensitiveFileBlocker()
	mgr := NewManager()
	mgr.Register(blocker)

	tests := []struct {
		name     string
		tool     string
		params   string
		blocked  bool
	}{
		{name: ".env file_path", tool: "write_file", params: `{"file_path":".env","content":"foo"}`, blocked: true},
		{name: ".env in view", tool: "view", params: `{"file_path":".env"}`, blocked: true},
		{name: "id_rsa file_path", tool: "write_file", params: `{"file_path":"/home/user/.ssh/id_rsa"}`, blocked: true},
		{name: "cert.pem file_path", tool: "write_file", params: `{"file_path":"server.pem"}`, blocked: true},
		{name: "credentials.json", tool: "view", params: `{"file_path":"credentials.json"}`, blocked: true},
		{name: "safe file", tool: "write_file", params: `{"file_path":"main.go","content":"hello"}`, blocked: false},
		{name: "non-file tool", tool: "grep", params: `{"pattern":"test"}`, blocked: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &ToolExecuteInput{Tool: tt.tool, SessionID: "s1", CallID: "c1"}
			out := &ToolExecuteOutput{Args: json.RawMessage(tt.params)}
			_, err := mgr.Trigger(context.Background(), EventToolExecuteBefore, in, out)
			blocked := err != nil
			if blocked != tt.blocked {
				t.Fatalf("expected blocked=%v, got blocked=%v err=%v", tt.blocked, blocked, err)
			}
		})
	}
}

func TestToolAuditObserverRecordsEntries(t *testing.T) {
	observer := NewToolAuditObserver()
	mgr := NewManager()
	mgr.Register(observer)

	afterInput := &ToolExecuteAfterInput{
		Tool:      "view",
		SessionID: "s1",
		CallID:    "c1",
		Args:      json.RawMessage(`{"file_path":"main.go"}`),
		Error:     "",
	}
	_, err := mgr.Trigger(context.Background(), EventToolExecuteAfter, afterInput, &ToolExecuteOutput{Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("observer should not error: %v", err)
	}
	if len(observer.Entries()) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(observer.Entries()))
	}
	entry := observer.Entries()[0]
	if entry.Tool != "view" || !entry.Success {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestToolAuditObserverDoesNotBlock(t *testing.T) {
	observer := NewToolAuditObserver()
	blocker := NewSensitiveFileBlocker()
	mgr := NewManager()
	mgr.Register(observer)
	mgr.Register(blocker)

	// Observer should not block safe operations, blocker should not block this either
	in := &ToolExecuteInput{Tool: "grep", SessionID: "s1", CallID: "c1"}
	out := &ToolExecuteOutput{Args: json.RawMessage(`{"pattern":"test"}`)}
	_, err := mgr.Trigger(context.Background(), EventToolExecuteBefore, in, out)
	if err != nil {
		t.Fatalf("unexpected block: %v", err)
	}
}
