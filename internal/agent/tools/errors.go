package tools

import "fmt"

// ErrorType 工具错误类型（第12章 结构化反馈）
type ErrorType string

const (
	ErrBannedCommand    ErrorType = "BANNED_COMMAND"
	ErrPermissionDenied ErrorType = "PERMISSION_DENIED"
	ErrInvalidPath      ErrorType = "INVALID_PATH"
	ErrExecutionFailed  ErrorType = "EXECUTION_FAILED"
	ErrNeedConfirmation ErrorType = "NEED_CONFIRMATION"
	ErrTimeout          ErrorType = "TIMEOUT"
)

// ToolError 结构化工具错误
type ToolError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Tool    string    `json:"tool,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

func (e *ToolError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Type, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// NewBannedCommandError 创建禁止命令错误
func NewBannedCommandError(command, reason string) *ToolError {
	return &ToolError{
		Type:    ErrBannedCommand,
		Message: "命令被安全策略阻止",
		Tool:    "bash",
		Detail:  reason,
	}
}

// NewPermissionDeniedError 创建权限拒绝错误
func NewPermissionDeniedError(reason string) *ToolError {
	return &ToolError{
		Type:    ErrPermissionDenied,
		Message: "权限不足",
		Detail:  reason,
	}
}
