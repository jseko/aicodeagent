package tui

import (
	"testing"
	"time"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlCPreservesCancellationStatusAgainstLateDone(t *testing.T) {
	model := New(&app.App{Broker: pubsub.NewBroker[events.Event]()})
	model.isLoading = true

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(Model)

	if model.statusMsg != "已取消当前任务" {
		t.Fatalf("status after cancel = %q", model.statusMsg)
	}

	updated, _ = model.Update(events.AgentThink{
		SessionID: "default",
		Content:   "late response",
		IsDone:    true,
		Time:      time.Now(),
	})
	model = updated.(Model)

	if model.statusMsg != "已取消当前任务" {
		t.Fatalf("late done overwrote cancellation status: %q", model.statusMsg)
	}
	if model.isLoading {
		t.Fatal("model should remain non-loading after cancellation")
	}
}
