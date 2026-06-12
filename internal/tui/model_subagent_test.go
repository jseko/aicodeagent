package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	"AICodeAgent/internal/subagent"
)

func newSubagentTestModel(names []string) Model {
	registry := subagent.NewRegistry()
	for _, name := range names {
		registry.Register(&subagent.Subagent{
			Config: &subagent.SubagentConfig{
				Name:        name,
				Description: name + " description",
				Model:       "gpt-4o-mini",
				Tools:       map[string]bool{"read_file": true},
				Permissions: &subagent.PermissionConfig{Level: subagent.PermissionLevelReadOnly},
			},
			SystemPrompt: "test prompt",
		})
	}
	model := New(&app.App{
		Broker:    pubsub.NewBroker[events.Event](),
		Subagents: registry,
	})
	model.ready = true
	model.width = 100
	model.height = 30
	return model
}

func TestSubagentNamesPopulatedFromRegistry(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer", "test-writer"})
	if len(model.subagentNames) != 2 {
		t.Fatalf("expected 2 subagent names, got %v", model.subagentNames)
	}
	if model.subagentNames[0] != "code-reviewer" || model.subagentNames[1] != "test-writer" {
		t.Fatalf("unexpected names: %v", model.subagentNames)
	}
}

func TestSubagentNameMatchingFiltersByPrefix(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer", "commit-helper", "test-writer"})

	matches := model.matchingSubagentNames("/subagent co")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches for 'co', got %v", matches)
	}

	matches = model.matchingSubagentNames("/subagent c")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches for 'c', got %v", matches)
	}
}

func TestSubagentNameMatchingRequiresSpaceAfterCommand(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer"})

	if matches := model.matchingSubagentNames("/subagent"); matches != nil {
		t.Fatalf("expected nil when no space after /subagent, got %v", matches)
	}
}

func TestSubagentNameMatchingReturnsNilForOtherInputs(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer"})

	for _, input := range []string{"hello", "/initialize", "/subagents", "review code"} {
		if matches := model.matchingSubagentNames(input); matches != nil {
			t.Fatalf("expected nil for %q, got %v", input, matches)
		}
	}
}

func TestSubagentNameMatchingCaseInsensitive(t *testing.T) {
	model := newSubagentTestModel([]string{"Code-Reviewer"})

	matches := model.matchingSubagentNames("/subagent code")
	if len(matches) != 1 || matches[0] != "Code-Reviewer" {
		t.Fatalf("expected case-insensitive match, got %v", matches)
	}
}

func TestSubagentNameSuggestionRenderedInView(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer", "test-writer"})
	model.input = "/subagent co"

	view := model.View()
	if !strings.Contains(view, "Subagents:") {
		t.Fatalf("expected Subagents section in view:\n%s", view)
	}
	if !strings.Contains(view, "code-reviewer") {
		t.Fatalf("expected code-reviewer in suggestions:\n%s", view)
	}
	if strings.Contains(view, "test-writer") {
		t.Fatalf("test-writer should not match 'co' prefix:\n%s", view)
	}
}

func TestSubagentRoleSwitchEventSetsRole(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})

	updated, _ := model.Update(events.SubagentRoleSwitch{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		ChildSessionID:  "child-1",
	})
	m := updated.(Model)

	if m.subagentRole != "reviewer" {
		t.Fatalf("expected subagentRole 'reviewer', got %q", m.subagentRole)
	}
}

func TestSubagentRoleSwitchAddsSubagentMessage(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})

	updated, _ := model.Update(events.SubagentRoleSwitch{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		ChildSessionID:  "child-1",
	})
	m := updated.(Model)

	if len(m.messageItems) != 1 {
		t.Fatalf("expected 1 subagent message, got %d", len(m.messageItems))
	}
	if _, ok := m.messageItems[0].(*SubagentMessageItem); !ok {
		t.Fatalf("expected subagent message item, got %T", m.messageItems[0])
	}
	view := m.View()
	if !strings.Contains(view, "Subagent: reviewer") {
		t.Fatalf("expected subagent container in view:\n%s", view)
	}
}

func TestSubagentRoleRestoreClearsRole(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})
	model.subagentRole = "reviewer"

	updated, _ := model.Update(events.SubagentRoleRestore{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
	})
	m := updated.(Model)

	if m.subagentRole != "" {
		t.Fatalf("expected subagentRole to be cleared, got %q", m.subagentRole)
	}
}

func TestSubagentRoleRestoreDoesNotAppendExtraMessage(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})
	model.subagentRole = "reviewer"

	updated, _ := model.Update(events.SubagentRoleRestore{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
	})
	m := updated.(Model)

	if len(m.messageItems) != 0 {
		t.Fatalf("expected no extra restore message, got %d", len(m.messageItems))
	}
	if m.subagentRole != "" {
		t.Fatalf("expected role cleared, got %q", m.subagentRole)
	}
}

func TestSubagentRoleDisplayInStatusBar(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer"})
	model.subagentRole = "code-reviewer"
	model.statusMsg = ""

	view := model.View()
	if !strings.Contains(view, "Subagent: code-reviewer") {
		t.Fatalf("expected subagent role in status bar:\n%s", view)
	}
}

func TestSubagentToolEventsRenderInExpandableContainer(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})

	updated, _ := model.Update(events.SubagentRoleSwitch{ParentSessionID: "parent-1", SubagentName: "reviewer"})
	m := updated.(Model)
	updated, _ = m.Update(events.SubagentToolCall{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		ToolCallID:      "call-1",
		ToolName:        "view",
		Params:          []byte(`{"file_path":"main.go"}`),
	})
	m = updated.(Model)
	updated, _ = m.Update(events.SubagentToolResult{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		ToolCallID:      "call-1",
		ToolName:        "view",
		Result:          "package main",
	})
	m = updated.(Model)
	updated, _ = m.Update(events.SubagentTaskComplete{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		Summary:         "review done",
	})
	m = updated.(Model)

	collapsed := m.View()
	if !strings.Contains(collapsed, "├──") || !strings.Contains(collapsed, "view") {
		t.Fatalf("expected collapsed tool tree, got:\n%s", collapsed)
	}
	if strings.Contains(collapsed, "package main") {
		t.Fatalf("collapsed view should hide tool result:\n%s", collapsed)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	expanded := m.View()
	if !strings.Contains(expanded, "package main") || !strings.Contains(expanded, "最终总结") {
		t.Fatalf("expected expanded details, got:\n%s", expanded)
	}
}

func TestSubagentRoleNotShownWhenEmpty(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer"})

	view := model.View()
	if strings.Contains(view, "Subagent:") {
		t.Fatalf("unexpected subagent role in status bar when empty:\n%s", view)
	}
}

func TestPermissionConfirmationUnaffectedBySubagentRole(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})
	model.subagentRole = "reviewer"

	updated, _ := model.Update(events.PermissionRequest{
		RequestID: "req-1",
		SessionID: "s1",
		ToolName:  "bash",
		Command:   "rm file.txt",
	})
	m := updated.(Model)

	if m.pendingConfirm == nil {
		t.Fatal("expected pending confirmation")
	}
	if m.pendingConfirm.ToolName != "bash" {
		t.Fatalf("expected bash tool in confirmation, got %q", m.pendingConfirm.ToolName)
	}
	// 角色不应影响权限确认状态
	if m.subagentRole != "reviewer" {
		t.Fatalf("subagent role should not be affected by permission request")
	}
}

func TestSubagentViewRendersSuggestionsOnlyWhenNotLoading(t *testing.T) {
	model := newSubagentTestModel([]string{"code-reviewer"})
	model.input = "/subagent co"
	model.isLoading = true

	view := model.View()
	if strings.Contains(view, "Subagents:") {
		t.Fatalf("subagent suggestions should not render while loading:\n%s", view)
	}
}

func TestMessageStreamDoesNotMixSubagentHistory(t *testing.T) {
	model := newSubagentTestModel(nil)

	// 模拟完整 Subagent 生命周期
	updated, _ := model.Update(events.SubagentRoleSwitch{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
		ChildSessionID:  "child-1",
	})
	m := updated.(Model)

	// 模拟 AgentThink 事件（来自子 Agent 或主 Agent）
	updated, _ = m.Update(events.AgentThink{
		SessionID: "parent-1",
		Content:   "[Subagent reviewer] review done",
		IsDone:    true,
	})
	m = updated.(Model)

	updated, _ = m.Update(events.SubagentRoleRestore{
		ParentSessionID: "parent-1",
		SubagentName:    "reviewer",
	})
	m = updated.(Model)

	items := m.messageItems
	if len(items) != 2 {
		t.Fatalf("expected 2 messages (subagent container, ai), got %d", len(items))
	}
	if _, ok := items[0].(*SubagentMessageItem); !ok {
		t.Fatal("expected first message to be subagent container")
	}
	if _, ok := items[1].(*AssistantMessageItem); !ok {
		t.Fatal("expected second message to be assistant")
	}
}

func TestHandlePermissionKeyIntegrationWithSubagentRole(t *testing.T) {
	model := newSubagentTestModel([]string{"reviewer"})
	model.subagentRole = "reviewer"
	model.pendingConfirm = &ConfirmationState{
		RequestID: "req-1",
		ToolName:  "write_file",
		Command:   "write file.txt",
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m := updated.(Model)

	if m.pendingConfirm != nil {
		t.Fatal("expected pending confirm to be cleared after keypress")
	}
	if m.subagentRole != "reviewer" {
		t.Fatalf("subagent role should persist after permission handling: %q", m.subagentRole)
	}
}
