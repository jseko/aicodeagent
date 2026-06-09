package tui

import (
	"context"
	"testing"
	"time"

	"AICodeAgent/internal/events"
	"AICodeAgent/internal/tui/testutil"
)

// TestEventAdapter_EmitAndReceive 验证事件适配器发送和接收
func TestEventAdapter_EmitAndReceive(t *testing.T) {
	adapter := NewEventAdapter()

	// 发送消息
	go func() {
		adapter.Emit(AgentThinkMsg{SessionID: "s1", Content: "hello"})
	}()

	// 接收消息
	cmd := adapter.WaitForMsg()
	msg := cmd()

	think, ok := msg.(AgentThinkMsg)
	if !ok {
		t.Fatalf("expected AgentThinkMsg, got %T", msg)
	}
	if think.SessionID != "s1" || think.Content != "hello" {
		t.Errorf("message mismatch: %+v", think)
	}
}

// TestEventAdapter_NonBlocking 验证 channel 满时不阻塞
func TestEventAdapter_NonBlocking(t *testing.T) {
	adapter := NewEventAdapter()

	// 填满 channel (容量 100)
	for i := 0; i < 200; i++ {
		adapter.Emit(AgentThinkMsg{SessionID: "s1", Content: "test"})
	}

	// 不应 panic 或死锁
	adapter.Emit(AgentThinkMsg{SessionID: "s1", Content: "overflow"})
}

// TestEventAdapter_ChannelSubscription 验证 Subscribe 返回有效 channel
func TestEventAdapter_ChannelSubscription(t *testing.T) {
	adapter := NewEventAdapter()
	ch := adapter.Subscribe()

	if ch == nil {
		t.Fatal("Subscribe should return non-nil channel")
	}

	// 发送并验证接收
	go adapter.Emit(ToolCallMsg{SessionID: "s1", ToolName: "read_file"})

	select {
	case msg := <-ch:
		toolCall, ok := msg.(ToolCallMsg)
		if !ok {
			t.Fatalf("expected ToolCallMsg, got %T", msg)
		}
		if toolCall.ToolName != "read_file" {
			t.Errorf("tool name = %s, want read_file", toolCall.ToolName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// TestUI_DialogRouting 验证 Dialog 优先路由
func TestUI_DialogRouting(t *testing.T) {
	styles := DefaultDarkStyles()
	ui := &UI{
		Model:        Model{},
		dialogs:      NewDialogManager(),
		eventAdapter: NewEventAdapter(),
	}

	// 打开一个权限对话框
	req := PermissionRequest{ID: "test-1", Type: PermissionTypeBash, Command: "ls"}
	dialog := NewPermissionDialog(req, styles)
	ui.dialogs.OpenDialog(dialog)

	if !ui.dialogs.HasDialogs() {
		t.Fatal("should have dialog")
	}

	// View 应包含 dialog 内容
	view := ui.View()
	if view == "" {
		t.Error("view should not be empty with dialog open")
	}
}

// TestUI_ComponentFocusManagement 验证组件焦点管理
func TestUI_ComponentFocusManagement(t *testing.T) {
	styles := DefaultDarkStyles()
	ui := &UI{
		Model:        Model{},
		dialogs:      NewDialogManager(),
		eventAdapter: NewEventAdapter(),
	}

	editor1 := NewMultiLineEditor(styles)
	editor2 := NewMultiLineEditor(styles)

	ui.AddComponent(editor1)
	ui.AddComponent(editor2)

	ui.FocusComponent(0)
	if !editor1.IsFocused() {
		t.Error("editor1 should be focused")
	}
	if editor2.IsFocused() {
		t.Error("editor2 should not be focused")
	}

	ui.FocusComponent(1)
	if editor1.IsFocused() {
		t.Error("editor1 should be blurred after switching")
	}
	if !editor2.IsFocused() {
		t.Error("editor2 should be focused")
	}
}

// TestEventAdapterWithBroker_Integration 验证 EventAdapter 与 Broker 的集成
func TestEventAdapterWithBroker_Integration(t *testing.T) {
	fixtures := testutil.NewTestFixtures(t)
	defer fixtures.Cleanup(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	adapter := NewEventAdapter()

	// 先订阅 Broker
	brokerCh := fixtures.Broker.Subscribe(ctx)

	// 再发布事件（确保 subscriber 已就绪）
	time.Sleep(10 * time.Millisecond)
	fixtures.Broker.Publish(events.AgentThink{
		SessionID: "s1",
		Content:   "thinking...",
		Time:      time.Now(),
	})

	// 转发 Broker 事件到 EventAdapter
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-brokerCh:
				if !ok {
					return
				}
				if think, ok := evt.(events.AgentThink); ok {
					adapter.Emit(AgentThinkMsg{
						SessionID: think.SessionID,
						Content:   think.Content,
					})
					return
				}
			}
		}
	}()

	// 通过 EventAdapter 接收
	ch := adapter.Subscribe()
	select {
	case msg := <-ch:
		if _, ok := msg.(AgentThinkMsg); !ok {
			t.Errorf("expected AgentThinkMsg, got %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event adapter message")
	}
}

// TestStreamingManager_BrokerIntegration 验证 StreamingManager 与 EventAdapter 协作
func TestStreamingManager_BrokerIntegration(t *testing.T) {
	fixtures := testutil.NewTestFixtures(t)
	defer fixtures.Cleanup(t)

	sm := NewStreamingManager()
	adapter := NewEventAdapter()

	// 创建消息缓冲区
	sm.CreateMessage("msg-1")

	// 模拟流式 chunk
	chunks := []string{"Hello", " ", "World", "!", "\n"}
	for i, chunk := range chunks {
		sm.AppendDelta("msg-1", chunk)
		if i == len(chunks)-1 {
			sm.CompleteMessage("msg-1", "stop")
		}
	}

	buf := sm.GetBuffer("msg-1")
	if buf == nil {
		t.Fatal("buffer should exist")
	}
	if buf.Content != "Hello World!\n" {
		t.Errorf("content = %q, want %q", buf.Content, "Hello World!\n")
	}
	if !buf.IsComplete {
		t.Error("message should be complete")
	}

	// 通过 EventAdapter 发送完成消息到 TUI
	adapter.Emit(StreamCompleteMsg{MessageID: "msg-1", FinishInfo: "stop"})

	// 验证 TUI 可以接收
	ch := adapter.Subscribe()
	select {
	case msg := <-ch:
		complete, ok := msg.(StreamCompleteMsg)
		if !ok {
			t.Fatalf("expected StreamCompleteMsg, got %T", msg)
		}
		if complete.MessageID != "msg-1" {
			t.Errorf("message id = %s, want msg-1", complete.MessageID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
