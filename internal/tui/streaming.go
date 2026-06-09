package tui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// StreamCreateMsg 创建新的流式消息
type StreamCreateMsg struct {
	MessageID string
}

// StreamDeltaMsg 追加增量内容
type StreamDeltaMsg struct {
	MessageID string
	Delta     string
}

// StreamCompleteMsg 标记流式完成
type StreamCompleteMsg struct {
	MessageID  string
	FinishInfo string
}

// StreamTickMsg 定时刷新消息
type StreamTickMsg struct {
	Time time.Time
}

// MessageBuffer 单条消息的内容缓冲
type MessageBuffer struct {
	MessageID  string
	Content    string
	IsComplete bool
	FinishInfo string
}

// StreamingManager 流式消息管理器（例9-5）
type StreamingManager struct {
	mu      sync.RWMutex
	buffers map[string]*MessageBuffer
}

func NewStreamingManager() *StreamingManager {
	return &StreamingManager{
		buffers: make(map[string]*MessageBuffer),
	}
}

func (sm *StreamingManager) CreateMessage(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.buffers[id] = &MessageBuffer{
		MessageID: id,
		Content:   "",
	}
}

func (sm *StreamingManager) AppendDelta(id, delta string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if buf, ok := sm.buffers[id]; ok {
		buf.Content += delta
	}
}

func (sm *StreamingManager) CompleteMessage(id, info string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if buf, ok := sm.buffers[id]; ok {
		buf.IsComplete = true
		buf.FinishInfo = info
	}
}

func (sm *StreamingManager) GetBuffer(id string) *MessageBuffer {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.buffers[id]
}

func (sm *StreamingManager) IsComplete(id string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if buf, ok := sm.buffers[id]; ok {
		return buf.IsComplete
	}
	return false
}

// StreamTick 创建 50ms 定时刷新命令
func StreamTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return StreamTickMsg{Time: t}
	})
}
