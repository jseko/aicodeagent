package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilePicker_New(t *testing.T) {
	styles := DefaultDarkStyles()
	fp := NewFilePicker("/tmp", styles)

	if fp.currentDir != "/tmp" {
		t.Errorf("currentDir = %s, want /tmp", fp.currentDir)
	}
	if fp.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0", fp.selectedIdx)
	}
}

func TestFilePicker_KeyNavigation(t *testing.T) {
	styles := DefaultDarkStyles()
	fp := NewFilePicker("/tmp", styles)

	// Up at boundary - should stay at 0
	_, _ = fp.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fp.selectedIdx != 0 {
		t.Error("up at top should stay at 0")
	}

	// Escape calls onSelect
	selected := ""
	fp.OnSelect(func(path string) tea.Cmd {
		selected = path
		return nil
	})
	_, _ = fp.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if selected != "" {
		t.Errorf("escape should select empty, got: %s", selected)
	}
}

func TestFilePicker_View(t *testing.T) {
	styles := DefaultDarkStyles()
	fp := NewFilePicker("/tmp", styles)

	result := fp.View()
	if !strings.Contains(result, "/tmp") {
		t.Error("view should contain current dir")
	}
}

func TestMultiLineEditor_New(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	if len(e.lines) != 1 || e.lines[0] != "" {
		t.Error("new editor should have one empty line")
	}
}

func TestMultiLineEditor_InsertText(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	e.insertText("hello")
	if e.lines[0] != "hello" {
		t.Errorf("line = %s, want hello", e.lines[0])
	}
}

func TestMultiLineEditor_InsertSpace(t *testing.T) {
	editor := NewMultiLineEditor(DefaultDarkStyles())

	updated, _ := editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	editor = updated.(*MultiLineEditor)
	updated, _ = editor.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	editor = updated.(*MultiLineEditor)
	updated, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t', 'h', 'e', 'r', 'e'}})
	editor = updated.(*MultiLineEditor)

	if got := editor.Value(); got != "hi there" {
		t.Fatalf("expected space in editor value, got %q", got)
	}
}

func TestMultiLineEditor_InsertNewLine(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	e.insertText("line1")
	e.insertNewLine()
	e.insertText("line2")

	if len(e.lines) != 2 {
		t.Errorf("lines count = %d, want 2", len(e.lines))
	}
	if e.lines[0] != "line1" || e.lines[1] != "line2" {
		t.Errorf("lines mismatch: %v", e.lines)
	}
}

func TestMultiLineEditor_DeleteBefore(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	e.insertText("ab")
	e.cursorCol = 1
	e.deleteBefore()

	if e.lines[0] != "b" {
		t.Errorf("line = %s, want b", e.lines[0])
	}
}

func TestMultiLineEditor_Submit(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	e.insertText("test message")
	cmd := e.submit()

	if cmd == nil {
		t.Fatal("submit should return a command")
	}
	msg := cmd()
	submitMsg, ok := msg.(EditorSubmitMsg)
	if !ok {
		t.Fatal("should return EditorSubmitMsg")
	}
	if submitMsg.Content != "test message" {
		t.Errorf("content = %s, want test message", submitMsg.Content)
	}
}

func TestMultiLineEditor_History(t *testing.T) {
	styles := DefaultDarkStyles()
	e := NewMultiLineEditor(styles)

	// Submit first message
	e.insertText("msg1")
	e.submit()
	// Clear for next message
	e.lines = []string{""}
	e.cursorLine = 0
	e.cursorCol = 0

	// Submit second message
	e.insertText("msg2")
	e.submit()

	if len(e.history) != 2 {
		t.Fatalf("history count = %d, want 2", len(e.history))
	}

	// Navigate to previous
	e.prevHistory()
	if e.lines[0] != "msg2" {
		t.Errorf("first prevHistory should load msg2, got: %s", e.lines[0])
	}

	// Navigate further back
	e.prevHistory()
	if e.lines[0] != "msg1" {
		t.Errorf("second prevHistory should load msg1, got: %s", e.lines[0])
	}
}

func TestPermissionDialog_RiskLevels(t *testing.T) {
	tests := []struct {
		name     string
		req      PermissionRequest
		expected RiskLevel
	}{
		{
			name:     "rm -rf is high risk",
			req:      PermissionRequest{Type: PermissionTypeBash, Command: "rm -rf /"},
			expected: RiskHigh,
		},
		{
			name:     "sudo is medium risk",
			req:      PermissionRequest{Type: PermissionTypeBash, Command: "sudo apt update"},
			expected: RiskMedium,
		},
		{
			name:     "ls is low risk",
			req:      PermissionRequest{Type: PermissionTypeBash, Command: "ls -la"},
			expected: RiskLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateRisk(tt.req)
			if result != tt.expected {
				t.Errorf("risk = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestPermissionDialog_Respond(t *testing.T) {
	styles := DefaultDarkStyles()
	req := PermissionRequest{ID: "test-1", Type: PermissionTypeBash, Command: "ls"}
	d := NewPermissionDialog(req, styles)

	cmd := d.respond(ActionAllow)
	if cmd == nil {
		t.Fatal("respond should return a command")
	}

	msg := cmd()
	resp, ok := msg.(PermissionResponseMsg)
	if !ok {
		t.Fatal("should return PermissionResponseMsg")
	}
	if resp.RequestID != "test-1" || resp.Action != ActionAllow {
		t.Error("response mismatch")
	}
}

func TestDialogManager_Stack(t *testing.T) {
	dm := NewDialogManager()

	if dm.HasDialogs() {
		t.Error("should be empty initially")
	}

	styles := DefaultDarkStyles()
	d := NewPermissionDialog(PermissionRequest{ID: "1"}, styles)
	dm.OpenDialog(d)

	if !dm.HasDialogs() {
		t.Error("should have dialog after Open")
	}

	if dm.Top() == nil {
		t.Error("top should return dialog")
	}

	dm.CloseFront()
	if dm.HasDialogs() {
		t.Error("should be empty after CloseFront")
	}
}

func TestStreamingManager_Basics(t *testing.T) {
	sm := NewStreamingManager()
	sm.CreateMessage("msg-1")
	sm.AppendDelta("msg-1", "hello")
	sm.CompleteMessage("msg-1", "done")

	buf := sm.GetBuffer("msg-1")
	if buf == nil {
		t.Fatal("buffer should not be nil")
	}
	if buf.Content != "hello" {
		t.Errorf("content = %s, want hello", buf.Content)
	}
	if !sm.IsComplete("msg-1") {
		t.Error("message should be complete")
	}
}
