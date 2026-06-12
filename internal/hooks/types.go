package hooks

import (
	"context"
	"encoding/json"
	"time"
)

type EventType string

const (
	EventSessionStart      EventType = "session.start"
	EventSessionFinish     EventType = "session.finish"
	EventToolExecuteBefore EventType = "tool.execute.before"
	EventToolExecuteAfter  EventType = "tool.execute.after"
	EventError             EventType = "event.error"
)

type Priority int

const (
	PriorityBlocker  Priority = 100
	PriorityNormal   Priority = 50
	PriorityObserver Priority = 0
)

type Decision string

const (
	DecisionNone  Decision = "none"
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type HookContext struct {
	SessionID string
	StartTime time.Time
	Metadata  map[string]interface{}
}

type ToolExecuteInput struct {
	Tool      string `json:"tool"`
	SessionID string `json:"session_id"`
	CallID    string `json:"call_id"`
}

type ToolExecuteOutput struct {
	Args json.RawMessage `json:"args"`
}

type ToolExecuteAfterInput struct {
	Tool      string          `json:"tool"`
	SessionID string          `json:"session_id"`
	CallID    string          `json:"call_id"`
	Args      json.RawMessage `json:"args"`
	Result    interface{}     `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type SessionStartInput struct {
	SessionID string `json:"session_id"`
	UserRole  string `json:"user_role"`
}

type SessionFinishInput struct {
	DurationMs  int64 `json:"duration_ms"`
	TotalTokens int   `json:"total_tokens"`
	ToolCount   int   `json:"tool_count"`
}

type ErrorInput struct {
	Event     EventType `json:"event"`
	SessionID string    `json:"session_id,omitempty"`
	Hook      string    `json:"hook,omitempty"`
	Reason    string    `json:"reason"`
}

type AggregateResult struct {
	Decision     Decision        `json:"decision"`
	Halt         bool            `json:"halt"`
	HookCount    int             `json:"hook_count"`
	Reason       string          `json:"reason,omitempty"`
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"`
}

type HookResult struct {
	Decision     Decision        `json:"decision"`
	Halt         bool            `json:"halt,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"`
	HookName     string          `json:"hook_name,omitempty"`
}

type Hook interface {
	Name() string
	Types() []EventType
	Priority() Priority
}

type InputOutputHook interface {
	Hook
	Execute(ctx context.Context, input interface{}, output interface{}) error
}

type EventHook interface {
	Hook
	OnEvent(ctx context.Context, event interface{}) error
}

type MatcherHook interface {
	MatchTool(toolName string) bool
}

type ExternalHook interface {
	Hook
	IsExternal() bool
}
