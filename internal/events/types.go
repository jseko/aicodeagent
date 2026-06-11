// Package events 定义业务事件类型体系
package events

import (
	"encoding/json"
	"time"
)

// EventType 事件类型常量，避免硬编码字符串
type EventType string

const (
	TypeUserMessage   EventType = "user.message"
	TypeAgentThink    EventType = "agent.think"
	TypeToolCall      EventType = "tool.call"
	TypeToolResult    EventType = "tool.result"
	TypeSessionUpdate EventType = "session.update"
	TypeCancelTask    EventType = "task.cancel"
	TypeError         EventType = "error"
)

// Event 基础事件接口
type Event interface {
	Type() EventType
	Timestamp() time.Time
}

// UserMessage 用户消息事件
type UserMessage struct {
	SessionID string    `json:"session_id"`
	Content   string    `json:"content"`
	Time      time.Time `json:"time"`
}

func (e UserMessage) Type() EventType      { return TypeUserMessage }
func (e UserMessage) Timestamp() time.Time { return e.Time }

// AgentThink AI 流式响应事件
type AgentThink struct {
	SessionID        string    `json:"session_id"`
	Content          string    `json:"content"`
	ReasoningContent string    `json:"reasoning_content,omitempty"` // 推理/思考内容
	IsDone           bool      `json:"is_done"`
	Time             time.Time `json:"time"`
}

func (e AgentThink) Type() EventType      { return TypeAgentThink }
func (e AgentThink) Timestamp() time.Time { return e.Time }

// ToolCall 工具调用事件
type ToolCall struct {
	SessionID string          `json:"session_id"`
	ToolName  string          `json:"tool_name"`
	Params    json.RawMessage `json:"params"`
	SkillName string          `json:"skill_name,omitempty"`
	Time      time.Time       `json:"time"`
}

func (e ToolCall) Type() EventType      { return TypeToolCall }
func (e ToolCall) Timestamp() time.Time { return e.Time }

// ToolResult 工具执行结果事件
type ToolResult struct {
	SessionID string    `json:"session_id"`
	ToolName  string    `json:"tool_name"`
	SkillName string    `json:"skill_name,omitempty"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	Time      time.Time `json:"time"`
}

func (e ToolResult) Type() EventType      { return TypeToolResult }
func (e ToolResult) Timestamp() time.Time { return e.Time }

// SessionUpdate 会话更新事件（Undo/Redo 等操作触发）
type SessionUpdate struct {
	SessionID        string    `json:"session_id"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	WindowTokens     int64     `json:"window_tokens"`
	ThresholdTokens  int64     `json:"threshold_tokens"`
	Warning          bool      `json:"warning"`
	Time             time.Time `json:"time"`
}

func (e SessionUpdate) Type() EventType      { return TypeSessionUpdate }
func (e SessionUpdate) Timestamp() time.Time { return e.Time }

// CancelTask 取消当前任务事件
type CancelTask struct {
	SessionID string    `json:"session_id"`
	Time      time.Time `json:"time"`
}

func (e CancelTask) Type() EventType      { return TypeCancelTask }
func (e CancelTask) Timestamp() time.Time { return e.Time }

// ErrorEvent 错误事件
type ErrorEvent struct {
	SessionID string    `json:"session_id"`
	Error     string    `json:"error"`
	Time      time.Time `json:"time"`
}

func (e ErrorEvent) Type() EventType      { return TypeError }
func (e ErrorEvent) Timestamp() time.Time { return e.Time }
