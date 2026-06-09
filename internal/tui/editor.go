package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EditorSubmitMsg 编辑器提交消息
type EditorSubmitMsg struct {
	Content string
}

// MultiLineEditor 多行编辑器组件（例9-10）
type MultiLineEditor struct {
	*BaseComponent
	lines      []string
	cursorLine int
	cursorCol  int
	scrollLine int

	history    []string
	historyIdx int

	placeholder string
}

func NewMultiLineEditor(styles *Styles) *MultiLineEditor {
	return &MultiLineEditor{
		BaseComponent: &BaseComponent{styles: styles},
		lines:         []string{""},
		history:       make([]string, 0),
		historyIdx:    -1,
	}
}

func (e *MultiLineEditor) Init() tea.Cmd { return nil }

func (e *MultiLineEditor) Update(msg tea.Msg) (Component, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return e, nil
	}

	switch {
	case k.Type == tea.KeyRunes:
		e.insertText(string(k.Runes))
	case k.Type == tea.KeyEnter && k.Alt:
		e.insertNewLine()
	case k.Type == tea.KeyEnter:
		return e, e.submit()
	case k.Type == tea.KeyBackspace:
		e.deleteBefore()
	case k.Type == tea.KeyUp:
		e.moveCursor(0, -1)
	case k.Type == tea.KeyDown:
		e.moveCursor(0, 1)
	case k.Type == tea.KeyLeft:
		e.moveCursor(-1, 0)
	case k.Type == tea.KeyRight:
		e.moveCursor(1, 0)
	case k.Type == tea.KeyCtrlP:
		e.prevHistory()
	case k.Type == tea.KeyCtrlN:
		e.nextHistory()
	case k.Type == tea.KeyCtrlA:
		e.cursorCol = 0
	case k.Type == tea.KeyCtrlE:
		e.cursorCol = len(e.currentLine())
	case k.Type == tea.KeyCtrlK:
		e.killLine()
	}

	e.adjustScroll()
	return e, nil
}

func (e *MultiLineEditor) currentLine() string {
	if e.cursorLine >= len(e.lines) {
		return ""
	}
	return e.lines[e.cursorLine]
}

func (e *MultiLineEditor) insertText(s string) {
	line := e.currentLine()
	e.lines[e.cursorLine] = line[:e.cursorCol] + s + line[e.cursorCol:]
	e.cursorCol += len(s)
}

func (e *MultiLineEditor) insertNewLine() {
	line := e.currentLine()
	left := line[:e.cursorCol]
	right := line[e.cursorCol:]

	e.lines[e.cursorLine] = left
	e.lines = append(e.lines[:e.cursorLine+1], append([]string{right}, e.lines[e.cursorLine+1:]...)...)
	e.cursorLine++
	e.cursorCol = 0
}

func (e *MultiLineEditor) deleteBefore() {
	if e.cursorCol > 0 {
		line := e.currentLine()
		e.lines[e.cursorLine] = line[:e.cursorCol-1] + line[e.cursorCol:]
		e.cursorCol--
	} else if e.cursorLine > 0 {
		// 合并到上一行
		prevLine := e.lines[e.cursorLine-1]
		curLine := e.lines[e.cursorLine]
		e.cursorCol = len(prevLine)
		e.lines[e.cursorLine-1] = prevLine + curLine
		e.lines = append(e.lines[:e.cursorLine], e.lines[e.cursorLine+1:]...)
		e.cursorLine--
	}
}

func (e *MultiLineEditor) moveCursor(dCol, dLine int) {
	e.cursorLine += dLine
	e.cursorCol += dCol

	if e.cursorLine < 0 {
		e.cursorLine = 0
	}
	if e.cursorLine >= len(e.lines) {
		e.cursorLine = len(e.lines) - 1
	}
	if e.cursorCol < 0 {
		if e.cursorLine > 0 {
			e.cursorLine--
			e.cursorCol = len(e.currentLine())
		} else {
			e.cursorCol = 0
		}
	}
	if e.cursorCol > len(e.currentLine()) {
		e.cursorCol = len(e.currentLine())
	}
}

func (e *MultiLineEditor) killLine() {
	line := e.currentLine()
	e.lines[e.cursorLine] = line[:e.cursorCol]
}

func (e *MultiLineEditor) submit() tea.Cmd {
	content := strings.Join(e.lines, "\n")
	if strings.TrimSpace(content) != "" {
		e.history = append(e.history, content)
		e.historyIdx = len(e.history)
	}
	return func() tea.Msg { return EditorSubmitMsg{Content: content} }
}

func (e *MultiLineEditor) prevHistory() {
	if len(e.history) == 0 {
		return
	}
	if e.historyIdx == -1 {
		e.historyIdx = len(e.history) - 1
	} else if e.historyIdx > 0 {
		e.historyIdx--
	}
	e.loadHistory()
}

func (e *MultiLineEditor) nextHistory() {
	if e.historyIdx == -1 || e.historyIdx >= len(e.history)-1 {
		e.historyIdx = -1
		e.lines = []string{""}
		e.cursorLine = 0
		e.cursorCol = 0
		return
	}
	e.historyIdx++
	e.loadHistory()
}

func (e *MultiLineEditor) loadHistory() {
	if e.historyIdx >= 0 && e.historyIdx < len(e.history) {
		e.lines = strings.Split(e.history[e.historyIdx], "\n")
		e.cursorLine = len(e.lines) - 1
		e.cursorCol = len(e.lines[e.cursorLine])
	}
}

func (e *MultiLineEditor) adjustScroll() {
	if e.cursorLine < e.scrollLine {
		e.scrollLine = e.cursorLine
	}
	if e.cursorLine >= e.scrollLine+e.height-1 && e.height > 0 {
		e.scrollLine = e.cursorLine - e.height + 2
	}
}

func (e *MultiLineEditor) View() string {
	visibleLines := e.lines
	if e.height > 0 && len(visibleLines) > e.height {
		visibleLines = visibleLines[e.scrollLine : e.scrollLine+e.height]
	}

	var b strings.Builder
	for i, line := range visibleLines {
		actualLineIdx := i + e.scrollLine
		if actualLineIdx == e.cursorLine {
			// 显示光标
			if e.cursorCol < len(line) {
				b.WriteString(line[:e.cursorCol])
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(string(line[e.cursorCol])))
				b.WriteString(line[e.cursorCol+1:])
			} else {
				b.WriteString(line)
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
			}
		} else {
			b.WriteString(line)
		}
		if i < len(visibleLines)-1 {
			b.WriteString("\n")
		}
	}

	if len(b.String()) == 0 && e.placeholder != "" {
		return lipgloss.NewStyle().Foreground(e.styles.FgSubtle).Render(e.placeholder)
	}

	return b.String()
}

func (e *MultiLineEditor) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e *MultiLineEditor) SetPlaceholder(p string) {
	e.placeholder = p
}

// Value 获取当前编辑器内容
func (e *MultiLineEditor) Value() string {
	return strings.Join(e.lines, "\n")
}

// SetValue 设置编辑器内容
func (e *MultiLineEditor) SetValue(v string) {
	if v == "" {
		e.lines = []string{""}
	} else {
		e.lines = strings.Split(v, "\n")
	}
	e.cursorLine = len(e.lines) - 1
	e.cursorCol = len(e.lines[e.cursorLine])
}
