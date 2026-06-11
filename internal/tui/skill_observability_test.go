package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"AICodeAgent/internal/events"
)

func TestModelToolCallDisplaysSkillActivation(t *testing.T) {
	model := newSlashTestModel(nil)
	params := json.RawMessage(`{"file_path":"/tmp/find-skills/SKILL.md"}`)

	updated, _ := model.Update(events.ToolCall{
		SessionID: "s1",
		ToolName:  "view",
		Params:    params,
		SkillName: "find-skills",
	})
	model = updated.(Model)

	item := model.findPendingToolItem("skill: find-skills")
	if item == nil {
		t.Fatal("expected pending skill tool item")
	}
	if item.toolName != "skill: find-skills" {
		t.Fatalf("expected skill display name, got %q", item.toolName)
	}

	updated, _ = model.Update(events.ToolResult{
		SessionID: "s1",
		ToolName:  "view",
		SkillName: "find-skills",
		Result:    "ok",
	})
	model = updated.(Model)

	if item.status != ToolStatusSuccess {
		t.Fatalf("expected skill tool item success, got %v", item.status)
	}
}

func TestModelEnterTogglesLatestToolDetails(t *testing.T) {
	model := newSlashTestModel(nil)
	params := json.RawMessage(`{"file_path":"/tmp/find-skills/SKILL.md"}`)

	updated, _ := model.Update(events.ToolCall{
		SessionID: "s1",
		ToolName:  "view",
		Params:    params,
		SkillName: "find-skills",
	})
	model = updated.(Model)

	updated, _ = model.Update(events.ToolResult{
		SessionID: "s1",
		ToolName:  "view",
		SkillName: "find-skills",
		Result:    "browser automation details",
	})
	model = updated.(Model)

	if !strings.Contains(model.View(), "展开详情") {
		t.Fatal("expected collapsed tool details hint")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !strings.Contains(model.View(), "browser automation details") {
		t.Fatal("expected Enter to expand latest tool details")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	view := model.View()
	if strings.Contains(view, "browser automation details") || !strings.Contains(view, "展开详情") {
		t.Fatal("expected second Enter to collapse tool details")
	}
}

func TestModelCtrlETogglesToolDetailsWhileLoading(t *testing.T) {
	model := newSlashTestModel(nil)
	params := json.RawMessage(`{"file_path":"/tmp/find-skills/SKILL.md"}`)

	updated, _ := model.Update(events.ToolCall{
		SessionID: "s1",
		ToolName:  "view",
		Params:    params,
		SkillName: "find-skills",
	})
	model = updated.(Model)

	updated, _ = model.Update(events.ToolResult{
		SessionID: "s1",
		ToolName:  "view",
		SkillName: "find-skills",
		Result:    "details available while agent is still running",
	})
	model = updated.(Model)
	model.isLoading = true

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	if !strings.Contains(model.View(), "details available while agent is still running") {
		t.Fatal("expected Ctrl+E to expand tool details while loading")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = updated.(Model)
	view := model.View()
	if strings.Contains(view, "details available while agent is still running") || !strings.Contains(view, "展开详情") {
		t.Fatal("expected second Ctrl+E to collapse tool details while loading")
	}
}
