package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// EventAdapter 事件适配器 - 将 Agent 事件转换为 Bubble Tea 消息（例9-13）
type EventAdapter struct {
	msgCh chan tea.Msg
}

func NewEventAdapter() *EventAdapter {
	return &EventAdapter{
		msgCh: make(chan tea.Msg, 100),
	}
}

// Subscribe 返回接收 Bubble Tea 消息的 channel
func (ea *EventAdapter) Subscribe() <-chan tea.Msg {
	return ea.msgCh
}

// Emit 发送消息到 TUI（由外部事件源调用）
func (ea *EventAdapter) Emit(msg tea.Msg) {
	select {
	case ea.msgCh <- msg:
	default:
		// channel 满时丢弃，避免阻塞事件源
	}
}

// WaitForMsg 返回一个 tea.Cmd，从事件 channel 读取消息
func (ea *EventAdapter) WaitForMsg() tea.Cmd {
	return func() tea.Msg {
		msg := <-ea.msgCh
		return msg
	}
}

// ListenCmd 创建一个持续监听的命令
func (ea *EventAdapter) ListenCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-ea.msgCh:
			return msg
		case <-ctx.Done():
			return nil
		}
	}
}

// AgentThinkMsg Agent 思考事件消息
type AgentThinkMsg struct {
	SessionID string
	Content   string
}

// ToolCallMsg 工具调用消息
type ToolCallMsg struct {
	SessionID string
	ToolName  string
	Arguments map[string]interface{}
}

// ToolResultMsg 工具结果消息
type ToolResultMsg struct {
	SessionID string
	ToolName  string
	Success   bool
	Output    string
}

// ErrorMsg 错误事件消息
type ErrorMsg struct {
	SessionID string
	Error     error
}
