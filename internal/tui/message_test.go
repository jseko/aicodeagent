package tui

import (
	"strings"
	"testing"
)

func TestCachedMessageItem_CacheHit(t *testing.T) {
	c := &cachedMessageItem{}
	c.setCache("hello", 80)

	result, ok := c.getCachedRender(80)
	if !ok {
		t.Error("should hit cache")
	}
	if result != "hello" {
		t.Errorf("cached result = %s, want hello", result)
	}
}

func TestCachedMessageItem_CacheMiss_DifferentWidth(t *testing.T) {
	c := &cachedMessageItem{}
	c.setCache("hello", 80)

	_, ok := c.getCachedRender(40)
	if ok {
		t.Error("should miss cache for different width")
	}
}

func TestCachedMessageItem_CacheMiss_Empty(t *testing.T) {
	c := &cachedMessageItem{}

	_, ok := c.getCachedRender(80)
	if ok {
		t.Error("should miss cache when empty")
	}
}

func TestCachedMessageItem_InvalidateCache(t *testing.T) {
	c := &cachedMessageItem{}
	c.setCache("hello", 80)
	c.invalidateCache()

	_, ok := c.getCachedRender(80)
	if ok {
		t.Error("should miss cache after invalidation")
	}
}

func TestUserMessageItem_Render(t *testing.T) {
	styles := DefaultDarkStyles()
	msg := &Message{ID: "1", Role: "user", Content: "Hello, world!"}
	item := NewUserMessageItem(msg, styles)

	result := item.Render(80)

	if !strings.Contains(result, "Hello, world!") {
		t.Errorf("rendered content should contain message, got: %s", result)
	}
}

func TestAssistantMessageItem_Thinking(t *testing.T) {
	styles := DefaultDarkStyles()
	msg := &Message{ID: "2", Role: "assistant", Content: "I'll help you."}
	item := NewAssistantMessageItem(msg, styles)

	item.SetThinking("Let me think about this...")

	if item.thinkingContent != "Let me think about this..." {
		t.Error("thinking content should be set")
	}

	// 默认折叠
	result := item.Render(80)
	if !strings.Contains(result, "▶") {
		t.Errorf("collapsed thinking should show ▶, got: %s", result)
	}

	// 展开
	item.ToggleThinking()
	result = item.Render(80)
	if !strings.Contains(result, "▼") {
		t.Errorf("expanded thinking should show ▼, got: %s", result)
	}
}

func TestAssistantMessageItem_AppendContent(t *testing.T) {
	styles := DefaultDarkStyles()
	msg := &Message{ID: "3", Role: "assistant", Content: "Hello"}
	item := NewAssistantMessageItem(msg, styles)

	item.AppendContent(", world!")
	if item.message.Content != "Hello, world!" {
		t.Errorf("content = %s, want Hello, world!", item.message.Content)
	}
}

func TestToolMessageItem_StatusIcons(t *testing.T) {
	styles := DefaultDarkStyles()
	msg := &Message{ID: "4", Role: "tool", Content: ""}
	item := NewToolMessageItem(msg, "read_file", styles)

	// Pending
	result := item.Render(80)
	if strings.Contains(result, "✓") || strings.Contains(result, "✗") {
		t.Error("pending should not show success/error icon")
	}

	// Running
	item.SetStatus(ToolStatusRunning)
	result = item.Render(80)
	if !strings.Contains(result, "●") {
		t.Error("running should show ●")
	}

	// Success
	item.SetStatus(ToolStatusSuccess)
	result = item.Render(80)
	if !strings.Contains(result, "✓") {
		t.Errorf("success should show ✓, got: %s", result)
	}

	// Error
	item.SetStatus(ToolStatusError)
	result = item.Render(80)
	if !strings.Contains(result, "✗") {
		t.Errorf("error should show ✗, got: %s", result)
	}
}

func TestToolMessageItem_CollapsibleContent(t *testing.T) {
	styles := DefaultDarkStyles()
	msg := &Message{ID: "5", Role: "tool", Content: ""}
	item := NewToolMessageItem(msg, "bash", styles)

	item.SetResult("some result content")
	result := item.Render(80)
	if !strings.Contains(result, "展开") {
		t.Error("should show expand hint when collapsed")
	}

	item.ToggleContent()
	result = item.Render(80)
	if !strings.Contains(result, "some result content") {
		t.Error("should show result when expanded")
	}
}
