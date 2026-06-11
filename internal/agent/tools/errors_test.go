package tools

import (
	"strings"
	"testing"
)

func TestToolErrorFormatting(t *testing.T) {
	err := NewBannedCommandError("rm -rf /", "rm 是危险命令")
	if err.Type != ErrBannedCommand {
		t.Fatalf("expected BANNED_COMMAND, got %s", err.Type)
	}
	msg := err.Error()
	if !strings.Contains(msg, "[BANNED_COMMAND]") {
		t.Fatalf("expected [BANNED_COMMAND] prefix: %s", msg)
	}
	if !strings.Contains(msg, "rm 是危险命令") {
		t.Fatalf("expected detail: %s", msg)
	}
}

func TestPermissionDeniedError(t *testing.T) {
	err := NewPermissionDeniedError("不允许执行此操作")
	if err.Type != ErrPermissionDenied {
		t.Fatalf("expected PERMISSION_DENIED, got %s", err.Type)
	}
	msg := err.Error()
	if !strings.Contains(msg, "[PERMISSION_DENIED]") {
		t.Fatalf("expected [PERMISSION_DENIED] prefix: %s", msg)
	}
}

func TestBashFilterReturnsStructuredErrorType(t *testing.T) {
	b := NewBash(".")
	result, err := b.Execute(nil, jsonRaw(`{"command":"sudo rm -rf /"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("banned command should be rejected")
	}
	// 结构化的 ToolError 文本应包含类型标识
	if !strings.Contains(result.Error, string(ErrBannedCommand)) {
		t.Fatalf("expected structured error type in result: %s", result.Error)
	}
}

func TestBashFilterGreyZoneReturnsStructuredError(t *testing.T) {
	b := NewBash(".")
	result, err := b.Execute(nil, jsonRaw(`{"command":"docker ps"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("grey zone should need confirmation")
	}
	if !strings.Contains(result.Error, string(ErrNeedConfirmation)) {
		t.Fatalf("expected NEED_CONFIRMATION type: %s", result.Error)
	}
}
