package tui

import (
	"strings"
	"testing"
	"time"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSessionUpdateRendersTokenWarning(t *testing.T) {
	model := New(&app.App{Broker: pubsub.NewBroker[events.Event]()})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(Model)

	updated, _ = model.Update(events.SessionUpdate{
		SessionID:        "default",
		PromptTokens:     8000,
		CompletionTokens: 500,
		WindowTokens:     10000,
		ThresholdTokens:  2000,
		Warning:          true,
		Time:             time.Now(),
	})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "Token: 8500 / 10000") {
		t.Fatalf("view should render token usage, got:\n%s", view)
	}
	if !strings.Contains(view, "即将自动摘要") {
		t.Fatalf("view should render token warning, got:\n%s", view)
	}
}
