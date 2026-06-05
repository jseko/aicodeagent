package tools

import (
	"context"
	"encoding/json"
	"testing"

	"AICodeAgent/internal/permission"
)

func TestToolCallerExactNameLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name:        "view",
		description: "read file",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			return Result{Success: true, Output: "content"}, nil
		},
	})

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	caller := NewToolCaller(r, ps)

	result, err := caller.CallTool(context.Background(), "session1", "view", json.RawMessage(`{"file_path":"/test/file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
}

func TestToolCallerUnknownTool(t *testing.T) {
	r := NewRegistry()
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	caller := NewToolCaller(r, ps)

	result, err := caller.CallTool(context.Background(), "session1", "nonexistent", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("unknown tool should fail")
	}
}

func TestToolCallerBlacklistDenial(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "bash",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			return Result{Success: true}, nil
		},
	})

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddBlacklist("rm")
	caller := NewToolCaller(r, ps)

	result, err := caller.CallTool(context.Background(), "session1", "bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("blacklisted command should be denied")
	}
}
