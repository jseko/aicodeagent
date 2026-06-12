package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"AICodeAgent/internal/hooks"
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

type countingAuditLogger struct {
	successes  int
	rejections int
}

func (l *countingAuditLogger) LogSuccess(toolName string, params json.RawMessage) { l.successes++ }
func (l *countingAuditLogger) LogRejection(toolName string, params json.RawMessage, reason string) {
	l.rejections++
}

func TestToolCallerAuditLogsSuccess(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "view",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			return Result{Success: true, Output: "ok"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	caller := NewToolCaller(r, ps)
	logger := &countingAuditLogger{}
	caller.SetAuditLogger(logger)

	_, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/test/f"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger.successes != 1 || logger.rejections != 0 {
		t.Fatalf("expected 1 success, got successes=%d rejections=%d", logger.successes, logger.rejections)
	}
}

func TestToolCallerAuditLogsRejection(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "bash",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			return Result{Success: false, Error: "blocked"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("safe")
	caller := NewToolCaller(r, ps)
	logger := &countingAuditLogger{}
	caller.SetAuditLogger(logger)

	caller.CallTool(context.Background(), "s1", "bash", json.RawMessage(`{"command":"safe"}`))
	if logger.rejections != 1 || logger.successes != 0 {
		t.Fatalf("expected 1 rejection, got successes=%d rejections=%d", logger.successes, logger.rejections)
	}
}

func TestToolCallerAuditFailureDoesNotBlock(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "view",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			return Result{Success: true, Output: "ok"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	caller := NewToolCaller(r, ps)
	// 不设置 audit logger (nil)，操作仍应成功
	caller.SetAuditLogger(nil)

	result, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/test/f"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success without audit logger, got: %s", result.Error)
	}
}

func TestToolCallerApprovedOperationBypassesConfirmation(t *testing.T) {
	r := NewRegistry()
	executed := false
	r.Register(&mockTool{
		name: "bash",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			executed = true
			return Result{Success: true, Output: "ok"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	caller := NewToolCaller(r, ps)
	params := json.RawMessage(`{"command":"docker ps"}`)

	_, allow, needConfirm, _ := caller.ResolveAndCheckForSession("s1", "bash", params)
	if allow || !needConfirm {
		t.Fatalf("expected first grey-zone call to require confirmation")
	}
	ps.ApproveOperation("s1", "bash", params)

	result, err := caller.CallTool(context.Background(), "s1", "bash", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || !executed {
		t.Fatalf("expected approved operation to execute, result=%+v executed=%v", result, executed)
	}
}

func TestToolCallerBlocksHardcodedSecretBeforeExecute(t *testing.T) {
	r := NewRegistry()
	executed := false
	r.Register(&mockTool{
		name: "write_file",
		execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			executed = true
			return Result{Success: true, Output: "written"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("main.go")
	caller := NewToolCaller(r, ps)

	result, err := caller.CallTool(context.Background(), "s1", "write_file", json.RawMessage(`{"file_path":"main.go","content":"apiKey := \"sk-1234567890abcdef\""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || executed {
		t.Fatalf("expected hardcoded secret to be blocked before execution, result=%+v executed=%v", result, executed)
	}
}

func TestToolCallerBeforeHookBlocksExecution(t *testing.T) {
	r := NewRegistry()
	executed := false
	r.Register(&mockTool{name: "view", execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
		executed = true
		return Result{Success: true, Output: "ok"}, nil
	}})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	caller := NewToolCaller(r, ps)
	manager := hooks.NewManager()
	manager.Register(&toolIOHook{name: "block", priority: hooks.PriorityBlocker, err: errors.New("blocked by hook")})
	caller.SetHookManager(manager)

	result, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/test/file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || executed {
		t.Fatalf("expected hook to block execution, result=%+v executed=%v", result, executed)
	}
}

func TestToolCallerBeforeHookModifiesArgs(t *testing.T) {
	r := NewRegistry()
	var got ViewParams
	r.Register(&mockTool{name: "view", execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
		if err := json.Unmarshal(params, &got); err != nil {
			return Result{Success: false, Error: err.Error()}, nil
		}
		return Result{Success: true, Output: "ok"}, nil
	}})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	ps.AddWhitelist("/safe")
	caller := NewToolCaller(r, ps)
	manager := hooks.NewManager()
	manager.Register(&toolIOHook{name: "rewrite", priority: hooks.PriorityNormal, update: json.RawMessage(`{"file_path":"/safe/file.txt"}`)})
	caller.SetHookManager(manager)

	result, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/test/file.txt"}`))
	if err != nil || !result.Success {
		t.Fatalf("unexpected result=%+v err=%v", result, err)
	}
	if got.FilePath != "/safe/file.txt" {
		t.Fatalf("hook-modified args not used: %+v", got)
	}
}

func TestToolCallerAfterHookObservesResult(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "view", execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
		return Result{Success: true, Output: "ok"}, nil
	}})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddWhitelist("/test")
	caller := NewToolCaller(r, ps)
	manager := hooks.NewManager()
	after := &toolEventHook{name: "after", types: []hooks.EventType{hooks.EventToolExecuteAfter}}
	manager.Register(after)
	caller.SetHookManager(manager)

	result, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/test/file.txt"}`))
	if err != nil || !result.Success {
		t.Fatalf("unexpected result=%+v err=%v", result, err)
	}
	if after.calls != 1 {
		t.Fatalf("after hook should observe once, got %d", after.calls)
	}
}

func TestToolCallerPermissionFailureBeforeHook(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "view", execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
		return Result{Success: true, Output: "ok"}, nil
	}})
	ps := permission.NewPermissionService(permission.PermLevelGuest, nil, nil)
	caller := NewToolCaller(r, ps)
	manager := hooks.NewManager()
	before := &toolIOHook{name: "before", priority: hooks.PriorityNormal}
	manager.Register(before)
	caller.SetHookManager(manager)

	result, err := caller.CallTool(context.Background(), "s1", "view", json.RawMessage(`{"file_path":"/blocked/file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || before.calls != 0 {
		t.Fatalf("permission failure should happen before hook, result=%+v calls=%d", result, before.calls)
	}
}

type toolIOHook struct {
	name     string
	priority hooks.Priority
	err      error
	update   json.RawMessage
	calls    int
}

func (h *toolIOHook) Name() string            { return h.name }
func (h *toolIOHook) Types() []hooks.EventType { return []hooks.EventType{hooks.EventToolExecuteBefore} }
func (h *toolIOHook) Priority() hooks.Priority { return h.priority }
func (h *toolIOHook) Execute(ctx context.Context, input interface{}, output interface{}) error {
	h.calls++
	if h.err != nil {
		return h.err
	}
	if len(h.update) > 0 {
		output.(*hooks.ToolExecuteOutput).Args = h.update
	}
	return nil
}

type toolEventHook struct {
	name  string
	types []hooks.EventType
	calls int
}

func (h *toolEventHook) Name() string            { return h.name }
func (h *toolEventHook) Types() []hooks.EventType { return h.types }
func (h *toolEventHook) Priority() hooks.Priority { return hooks.PriorityObserver }
func (h *toolEventHook) OnEvent(ctx context.Context, event interface{}) error {
	h.calls++
	return nil
}
