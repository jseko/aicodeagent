package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorDecisions(t *testing.T) {
	script := filepath.Join("testdata", "external_hook.sh")
	tests := []struct {
		name     string
		mode     string
		decision Decision
		halt     bool
		reason   string
		err      bool
	}{
		{name: "allow", mode: "allow", decision: DecisionAllow},
		{name: "json deny", mode: "deny", decision: DecisionDeny, reason: "json blocked"},
		{name: "exit2", mode: "exit2", decision: DecisionDeny, reason: "exit blocked"},
		{name: "halt", mode: "halt", decision: DecisionDeny, halt: true, reason: "halt turn"},
		{name: "unknown exit", mode: "fail", decision: DecisionNone, err: true},
		{name: "invalid stdout", mode: "invalid", decision: DecisionNone, err: true},
		{name: "claude", mode: "claude", decision: DecisionDeny, reason: "claude blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ExternalHookConfig{Name: tt.name, Types: []EventType{EventToolExecuteBefore}, Command: script, Args: []string{tt.mode}, Timeout: time.Second}
			result, err := NewExecutor().ExecuteExternal(context.Background(), cfg, EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{Args: json.RawMessage(`{"file_path":".env"}`)})
			if tt.err && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Decision != tt.decision || result.Halt != tt.halt {
				t.Fatalf("unexpected result: %#v", result)
			}
			if tt.reason != "" && result.Reason != tt.reason {
				t.Fatalf("reason mismatch: %q", result.Reason)
			}
		})
	}
}

func TestExecutorTimeout(t *testing.T) {
	cfg := &ExternalHookConfig{Name: "sleep", Types: []EventType{EventToolExecuteBefore}, Command: filepath.Join("testdata", "external_hook.sh"), Args: []string{"sleep"}, Timeout: 20 * time.Millisecond}
	_, err := NewExecutor().ExecuteExternal(context.Background(), cfg, EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestExternalHookConfigValidation(t *testing.T) {
	valid := ExternalHookConfig{Name: "env", Types: []EventType{EventToolExecuteBefore}, Command: "python3", Matcher: "^view$"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config failed: %v", err)
	}
	if !valid.MatchTool("view") || valid.MatchTool("bash") {
		t.Fatalf("matcher behavior invalid")
	}

	invalid := ExternalHookConfig{Name: "bad", Types: []EventType{EventToolExecuteBefore}, Command: "python3", Matcher: "["}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid matcher error")
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.yaml")
	content := []byte("hooks:\n  - name: env\n    types: [tool.execute.before]\n    command: python3\n    matcher: '^view$'\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	configs, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(configs) != 1 || configs[0].Name != "env" {
		t.Fatalf("unexpected configs: %#v", configs)
	}
}

func TestBridgeRegistersExternalHook(t *testing.T) {
	m := NewManager()
	bridge := NewBridge(m, NewExecutor())
	cfg := ExternalHookConfig{Name: "deny", Types: []EventType{EventToolExecuteBefore}, Command: filepath.Join("testdata", "external_hook.sh"), Args: []string{"deny"}, Timeout: time.Second, Priority: PriorityBlocker, Matcher: "^view$"}
	if err := bridge.LoadAndRegister([]ExternalHookConfig{cfg}); err != nil {
		t.Fatalf("load bridge: %v", err)
	}
	_, err := m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{Args: json.RawMessage(`{"file_path":".env"}`)})
	if err == nil {
		t.Fatalf("expected external hook rejection")
	}
}
