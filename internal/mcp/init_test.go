package mcp

import (
	"context"
	"sync"
	"testing"
	"time"
)

// resetGlobalState 在测试前重置全局状态
func resetGlobalState() {
	sessions = sync.Map{}
	states = sync.Map{}
	configs = sync.Map{}
}

func TestUpdateState(t *testing.T) {
	resetGlobalState()

	updateState("test-server", StateStarting, nil, nil, Counts{})

	state, ok := GetState("test-server")
	if !ok {
		t.Fatal("state not found after updateState")
	}
	if state.State != StateStarting {
		t.Fatalf("expected StateStarting, got '%s'", state.State)
	}
	if state.Name != "test-server" {
		t.Fatalf("expected name 'test-server', got '%s'", state.Name)
	}
}

func TestStateTransitions(t *testing.T) {
	resetGlobalState()

	// Disabled → Starting → Connected
	updateState("server1", StateDisabled, nil, nil, Counts{})
	state, _ := GetState("server1")
	if state.State != StateDisabled {
		t.Fatalf("expected StateDisabled, got '%s'", state.State)
	}

	updateState("server1", StateStarting, nil, nil, Counts{Tools: 0})
	state, _ = GetState("server1")
	if state.State != StateStarting {
		t.Fatalf("expected StateStarting, got '%s'", state.State)
	}

	updateState("server1", StateConnected, nil, nil, Counts{Tools: 5})
	state, _ = GetState("server1")
	if state.State != StateConnected {
		t.Fatalf("expected StateConnected, got '%s'", state.State)
	}
	if state.Counts.Tools != 5 {
		t.Fatalf("expected 5 tools, got %d", state.Counts.Tools)
	}

	// Connected → Error
	updateState("server1", StateError, context.DeadlineExceeded, nil, Counts{Tools: 5, Retries: 1})
	state, _ = GetState("server1")
	if state.State != StateError {
		t.Fatalf("expected StateError, got '%s'", state.State)
	}
	if state.Counts.Retries != 1 {
		t.Fatalf("expected 1 retry, got %d", state.Counts.Retries)
	}
}

func TestGetStateNotFound(t *testing.T) {
	resetGlobalState()

	_, ok := GetState("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestInitializeDisabledServer(t *testing.T) {
	resetGlobalState()

	cfgs := map[string]MCPConfigAdapter{
		"disabled-server": {
			Type:     "stdio",
			Command:  "nonexistent",
			Disabled: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	Initialize(ctx, cfgs)

	state, ok := GetState("disabled-server")
	if !ok {
		t.Fatal("state not found for disabled server")
	}
	if state.State != StateDisabled {
		t.Fatalf("expected StateDisabled, got '%s'", state.State)
	}
}

func TestInitializeInvalidCommand(t *testing.T) {
	resetGlobalState()

	cfgs := map[string]MCPConfigAdapter{
		"bad-server": {
			Type:    "stdio",
			Command: "/nonexistent/path/to/binary",
			Timeout: 1,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	Initialize(ctx, cfgs)

	state, ok := GetState("bad-server")
	if !ok {
		t.Fatal("state not found")
	}
	if state.State != StateError {
		t.Fatalf("expected StateError for invalid command, got '%s'", state.State)
	}
}

func TestCloseNoSessions(t *testing.T) {
	resetGlobalState()

	// 关闭无 session 时不应 panic
	Close()
}

func TestGetOrRenewClientNotFound(t *testing.T) {
	resetGlobalState()

	ctx := context.Background()
	_, err := getOrRenewClient(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

func TestListSessionsEmpty(t *testing.T) {
	resetGlobalState()

	result := ListSessions()
	if len(result) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(result))
	}
}

func TestGetSessionNotFound(t *testing.T) {
	resetGlobalState()

	_, ok := GetSession("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestMaxReconnectRetriesConstant(t *testing.T) {
	if maxReconnectRetries != 3 {
		t.Fatalf("expected maxReconnectRetries=3, got %d", maxReconnectRetries)
	}
}

func TestStateConstants(t *testing.T) {
	if StateDisabled != "disabled" {
		t.Fatalf("expected StateDisabled='disabled', got '%s'", StateDisabled)
	}
	if StateStarting != "starting" {
		t.Fatalf("expected StateStarting='starting', got '%s'", StateStarting)
	}
	if StateConnected != "connected" {
		t.Fatalf("expected StateConnected='connected', got '%s'", StateConnected)
	}
	if StateError != "error" {
		t.Fatalf("expected StateError='error', got '%s'", StateError)
	}
}
