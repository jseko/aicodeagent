package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	"AICodeAgent/internal/skills"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model 存储TUI所有状态
type Model struct {
	app       *app.App
	pub       pubsub.Publisher[events.Event]
	sub       pubsub.Subscriber[events.Event]
	sessionID string

	// 消息渲染 — 使用 MessageItem 接口实现富文本渲染
	messageItems      []MessageItem
	currentAssistant  *AssistantMessageItem
	selectedToolIndex int
	msgIDSeq          int

	input                  string
	slashSuggestionIndex   int
	slashSuggestionsHidden bool
	skillSuggestions       []skills.SkillSuggestion
	cursor                 int
	ready                  bool
	isLoading              bool
	cancelledCurrentTask   bool
	statusMsg              string
	tokenStatus            string
	tokenWarning           bool
	width                  int
	height                 int
	history                []string
	historyIdx             int
	styles                 *Styles
	theme                  *ThemeManager

	// 动画状态
	spinnerFrame int

	// 滚动状态（智能滚动，例9-5）
	scrollPos   int // 0=底部，正数=向上滚动行数
	wasAtBottom bool

	// 权限确认状态
	pendingConfirm *ConfirmationState

	// Subagent 状态
	subagentNames         []string
	subagentRole          string
	currentSubagentItem   *SubagentMessageItem
	selectedSubagentIndex int
}

// ConfirmationState 权限确认请求的 TUI 状态
type ConfirmationState struct {
	RequestID string
	SessionID string
	ToolName  string
	Command   string
}

// SpinnerTickMsg 旋转动画定时消息
type SpinnerTickMsg struct{}

const maxVisibleSlashSuggestions = 8

type slashSuggestion struct {
	Command     string
	Description string
	Skill       bool
}

var builtInSlashSuggestions = []slashSuggestion{
	{Command: "/initialize", Description: "扫描项目并生成 AGENTS.md 预览"},
	{Command: "/initialize-confirm", Description: "确认写入上一次生成的 AGENTS.md"},
	{Command: "/initialize-reject", Description: "放弃上一次生成的 AGENTS.md"},
	{Command: "/subagent", Description: "调用指定 Subagent：/subagent <name> <task>"},
	{Command: "/subagents", Description: "查看已加载的 Subagent 列表"},
	{Command: "/skills", Description: "查看已加载的 Skills 列表"},
	{Command: "/rules", Description: "查看已加载的 Rules 列表"},
	{Command: "/undo", Description: "回滚到上一条用户消息"},
	{Command: "/redo", Description: "恢复被回滚的消息"},
}

func (m Model) matchingSlashSuggestions(input string) []slashSuggestion {
	if m.slashSuggestionsHidden {
		return nil
	}
	return m.rawMatchingSlashSuggestions(input)
}

func (m Model) rawMatchingSlashSuggestions(input string) []slashSuggestion {
	prefix, ok := leadingSlashPrefix(input)
	if !ok {
		return nil
	}

	matches := make([]slashSuggestion, 0, len(builtInSlashSuggestions)+len(m.skillSuggestions))
	for _, suggestion := range builtInSlashSuggestions {
		if prefix == "" || strings.HasPrefix(strings.TrimPrefix(suggestion.Command, "/"), prefix) {
			matches = append(matches, suggestion)
		}
	}
	for _, skillSuggestion := range skills.FilterSkillSuggestions(m.skillSuggestions, input) {
		matches = append(matches, slashSuggestion{
			Command:     skillSuggestion.Command,
			Description: skillSuggestion.Description,
			Skill:       true,
		})
	}
	if len(matches) > maxVisibleSlashSuggestions {
		return matches[:maxVisibleSlashSuggestions]
	}
	return matches
}

func leadingSlashPrefix(input string) (string, bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	if strings.ContainsAny(input, " \t\r\n") {
		return "", false
	}
	return strings.TrimPrefix(input, "/"), true
}

func (m Model) matchingSubagentNames(input string) []string {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) != 2 || parts[0] != "/subagent" {
		return nil
	}
	prefix := strings.ToLower(parts[1])
	var matches []string
	for _, name := range m.subagentNames {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			matches = append(matches, name)
		}
	}
	return matches
}

func (m Model) clampSlashSuggestionIndex(input string, index int) int {
	matches := m.matchingSlashSuggestions(input)
	if len(matches) == 0 {
		return 0
	}
	if index < 0 {
		return len(matches) - 1
	}
	if index >= len(matches) {
		return 0
	}
	return index
}

func (m Model) completeSlashSuggestion(input string, index int) string {
	matches := m.matchingSlashSuggestions(input)
	if len(matches) == 0 {
		return input
	}
	command := matches[m.clampSlashSuggestionIndex(input, index)].Command
	if strings.HasPrefix(input, command+" ") {
		return input
	}
	return command + " "
}

func (m Model) inputRunes() []rune {
	return []rune(m.input)
}

func (m Model) clampCursor() Model {
	length := len(m.inputRunes())
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > length {
		m.cursor = length
	}
	return m
}

func (m Model) insertInputText(text string) Model {
	m = m.clampCursor()
	runes := m.inputRunes()
	insert := []rune(text)
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:m.cursor]...)
	next = append(next, insert...)
	next = append(next, runes[m.cursor:]...)
	m.input = string(next)
	m.cursor += len(insert)
	m.slashSuggestionsHidden = false
	m.slashSuggestionIndex = m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex)
	return m
}

func (m Model) deleteInputBeforeCursor() Model {
	m = m.clampCursor()
	if m.cursor == 0 {
		return m
	}
	runes := m.inputRunes()
	next := make([]rune, 0, len(runes)-1)
	next = append(next, runes[:m.cursor-1]...)
	next = append(next, runes[m.cursor:]...)
	m.input = string(next)
	m.cursor--
	m.slashSuggestionsHidden = false
	m.slashSuggestionIndex = m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex)
	return m
}

func (m Model) moveInputCursor(delta int) Model {
	m.cursor += delta
	return m.clampCursor()
}

func (m Model) setInputText(input string) Model {
	m.input = input
	m.cursor = len([]rune(input))
	m.slashSuggestionsHidden = false
	m.slashSuggestionIndex = m.clampSlashSuggestionIndex(m.input, 0)
	return m
}

func toolDisplayName(toolName, skillName string) string {
	if skillName != "" {
		return "skill: " + skillName
	}
	return toolName
}

func (m Model) renderedInputLine() string {
	if m.isLoading {
		return "> " + m.input
	}
	m = m.clampCursor()
	runes := m.inputRunes()
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	if len(runes) == 0 {
		return "> " + cursorStyle.Render(" ")
	}
	if m.cursor >= len(runes) {
		return "> " + string(runes) + cursorStyle.Render(" ")
	}
	before := string(runes[:m.cursor])
	cursor := cursorStyle.Render(string(runes[m.cursor]))
	after := string(runes[m.cursor+1:])
	return "> " + before + cursor + after
}

func formatTokenStatus(promptTokens, completionTokens, windowTokens int64, warning bool) string {
	used := promptTokens + completionTokens
	if windowTokens <= 0 || used <= 0 {
		return ""
	}
	status := fmt.Sprintf("Token: %d / %d", used, windowTokens)
	if warning {
		status += " · 即将自动摘要"
	}
	return status
}

func New(a *app.App) Model {
	theme := NewThemeManager()
	s := theme.GetStyles()
	if a.Config != nil {
		switch a.Config.Theme {
		case "light":
			theme.SetMode(ThemeModeLight)
			s = theme.GetStyles()
		case "auto":
			theme.SetMode(ThemeModeAuto)
			s = theme.GetStyles()
		}
	}

	skillSuggestions := append([]skills.SkillSuggestion(nil), a.SkillSuggestions...)

	subagentNames := make([]string, 0)
	if a.Subagents != nil {
		for _, sub := range a.Subagents.List() {
			if sub.Config != nil {
				subagentNames = append(subagentNames, sub.Config.Name)
			}
		}
	}

	return Model{
		app:                   a,
		pub:                   a.Broker,
		sub:                   a.Broker,
		sessionID:             "default",
		skillSuggestions:      skillSuggestions,
		messageItems:          []MessageItem{},
		selectedToolIndex:     -1,
		selectedSubagentIndex: -1,
		input:                 "",
		ready:                 false,
		isLoading:             false,
		statusMsg:             "",
		subagentNames:         subagentNames,
		styles:                s,
		theme:                 theme,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		spinnerTick(),
	)
}

// Update 事件处理与状态更新
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ready = true
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SpinnerTickMsg:
		if m.isLoading {
			m.spinnerFrame++
		}
		if m.isLoading {
			return m, spinnerTick()
		}
		return m, nil

	case tea.MouseMsg:
		switch {
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp:
			m.scrollPos += 3
			m.wasAtBottom = false
			return m, nil
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown:
			m.scrollPos = max(0, m.scrollPos-3)
			if m.scrollPos == 0 {
				m.wasAtBottom = true
			}
			return m, nil
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyPgUp:
			m.scrollPos += m.height / 2
			m.wasAtBottom = false
			return m, nil
		case tea.KeyPgDown:
			m.scrollPos = max(0, m.scrollPos-m.height/2)
			if m.scrollPos == 0 {
				m.wasAtBottom = true
			}
			return m, nil
		case tea.KeyHome:
			m.scrollPos += len(m.messageItems)
			m.wasAtBottom = false
			return m, nil
		case tea.KeyEnd:
			m.scrollPos = 0
			m.wasAtBottom = true
			return m, nil

		case tea.KeyCtrlC:
			if m.isLoading {
				m.pub.Publish(events.CancelTask{SessionID: m.sessionID, Time: time.Now()})
				m.isLoading = false
				m.cancelledCurrentTask = true
				m.currentAssistant = nil
				m.statusMsg = "已取消当前任务"
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if len(m.rawMatchingSlashSuggestions(m.input)) > 0 {
				m.slashSuggestionsHidden = true
				return m, nil
			}
			return m, tea.Quit
		}
		if msg.String() == "ctrl+e" {
			if next, ok := m.toggleSelectedSubagentDetails(); ok {
				return next, nil
			}
			if next, ok := m.toggleSelectedToolDetails(); ok {
				return next, nil
			}
			return m, nil
		}
		if msg.String() == "alt+c" {
			text := m.lastAssistantText()
			if text != "" {
				m.statusMsg = "已复制最后一条 AI 回复"
				return m, copySelectedText(text)
			}
			return m, nil
		}

		if m.pendingConfirm != nil {
			return m.handlePermissionKey(msg)
		}

		if m.isLoading {
			return m, nil
		}

		switch msg.Type {
		case tea.KeyTab:
			m.input = m.completeSlashSuggestion(m.input, m.slashSuggestionIndex)
			m.cursor = len([]rune(m.input))
			m.slashSuggestionIndex = 0
			m.slashSuggestionsHidden = true
			return m, nil

		case tea.KeyEnter:
			if strings.TrimSpace(m.input) == "" {
				if next, ok := m.toggleSelectedSubagentDetails(); ok {
					return next, nil
				}
				if next, ok := m.toggleSelectedToolDetails(); ok {
					return next, nil
				}
				return m, nil
			}
			if matches := m.matchingSlashSuggestions(m.input); len(matches) > 0 {
				selected := matches[m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex)]
				if m.input != selected.Command {
					m.input = m.completeSlashSuggestion(m.input, m.slashSuggestionIndex)
					m.cursor = len([]rune(m.input))
					m.slashSuggestionIndex = 0
					m.slashSuggestionsHidden = true
					return m, nil
				}
			}
			content := m.input
			msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
			m.msgIDSeq++
			userItem := NewUserMessageItem(&Message{
				ID:      msgID,
				Role:    "user",
				Content: content,
			}, m.styles)

			m.messageItems = append(m.messageItems, userItem)
			m.history = append(m.history, content)
			m.historyIdx = len(m.history)
			m.isLoading = true
			m.cancelledCurrentTask = false
			m.statusMsg = "Thinking..."
			m.pub.Publish(events.UserMessage{
				SessionID: m.sessionID,
				Content:   content,
			})
			m.input = ""
			m.cursor = 0
			return m, spinnerTick()

		case tea.KeyBackspace, tea.KeyDelete:
			m = m.deleteInputBeforeCursor()
			return m, nil

		case tea.KeyUp:
			if len(m.matchingSlashSuggestions(m.input)) > 0 {
				m.slashSuggestionIndex = m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex-1)
				return m, nil
			}
			if len(m.history) > 0 && m.historyIdx > 0 {
				m.historyIdx--
				m = m.setInputText(m.history[m.historyIdx])
			}
			return m, nil

		case tea.KeyDown:
			if len(m.matchingSlashSuggestions(m.input)) > 0 {
				m.slashSuggestionIndex = m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex+1)
				return m, nil
			}
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m = m.setInputText(m.history[m.historyIdx])
			} else {
				m.historyIdx = len(m.history)
				m = m.setInputText("")
			}
			return m, nil

		case tea.KeyLeft:
			m = m.moveInputCursor(-1)
			return m, nil

		case tea.KeyRight:
			m = m.moveInputCursor(1)
			return m, nil

		case tea.KeySpace:
			if m.input == "" {
				if next, ok := m.toggleSelectedSubagentDetails(); ok {
					return next, nil
				}
				if next, ok := m.toggleSelectedToolDetails(); ok {
					return next, nil
				}
			}
			m = m.insertInputText(" ")
			return m, nil

		case tea.KeyRunes:
			for _, r := range msg.Runes {
				if r == utf8.RuneError || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
					continue
				}
				m = m.insertInputText(string(r))
			}
			return m, nil

		default:
			switch msg.String() {
			case "backspace", "ctrl+h", "delete":
				m = m.deleteInputBeforeCursor()
			}
			return m, nil
		}

	case events.AgentThink:
		if m.cancelledCurrentTask {
			return m, nil
		}
		// 推理/思考内容：设置到当前助手消息的思考区
		if msg.ReasoningContent != "" {
			if m.currentAssistant == nil {
				msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
				m.msgIDSeq++
				item := NewAssistantMessageItem(&Message{
					ID:      msgID,
					Role:    "ai",
					Content: "",
				}, m.styles)
				m.messageItems = append(m.messageItems, item)
				m.currentAssistant = item
			}
			m.currentAssistant.SetThinking(msg.ReasoningContent)
			m.isLoading = true
			m.statusMsg = "Thinking..."
			return m, spinnerTick()
		}

		// 正文内容
		if m.currentAssistant == nil {
			msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
			m.msgIDSeq++
			item := NewAssistantMessageItem(&Message{
				ID:      msgID,
				Role:    "ai",
				Content: "",
			}, m.styles)
			m.messageItems = append(m.messageItems, item)
			m.currentAssistant = item
		}
		if msg.Content != "" {
			m.currentAssistant.AppendContent(msg.Content)
		}
		if msg.IsDone {
			m.currentAssistant.SetComplete("stop")
			m.currentAssistant = nil
			m.isLoading = false
			m.statusMsg = "完成"
		} else {
			m.isLoading = true
		}
		return m, spinnerTick()

	case events.ToolCall:
		if m.cancelledCurrentTask {
			return m, nil
		}
		m.currentAssistant = nil
		msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
		m.msgIDSeq++
		paramStr := string(msg.Params)
		displayName := toolDisplayName(msg.ToolName, msg.SkillName)
		item := NewToolMessageItem(&Message{
			ID:      msgID,
			Role:    "tool",
			Content: displayName + " " + paramStr,
		}, displayName, m.styles)
		item.SetStatus(ToolStatusRunning)
		m.messageItems = append(m.messageItems, item)
		m.statusMsg = "Running: " + displayName
		return m, spinnerTick()

	case events.SessionUpdate:
		m.tokenStatus = formatTokenStatus(msg.PromptTokens, msg.CompletionTokens, msg.WindowTokens, msg.Warning)
		m.tokenWarning = msg.Warning
		return m, nil

	case events.ToolResult:
		if m.cancelledCurrentTask {
			return m, nil
		}
		displayName := toolDisplayName(msg.ToolName, msg.SkillName)
		item := m.findPendingToolItem(displayName)
		if item != nil {
			if msg.Error != "" {
				item.SetStatus(ToolStatusError)
				item.SetResult(msg.Error)
				m.statusMsg = "Tool error: " + displayName
			} else {
				item.SetStatus(ToolStatusSuccess)
				if msg.Result != "" {
					item.SetResult(msg.Result)
				}
				m.statusMsg = "Tool done: " + displayName
			}
			m.selectedToolIndex = m.latestExpandableToolIndex()
		}
		return m, nil

	case events.ErrorEvent:
		if m.cancelledCurrentTask {
			return m, nil
		}
		m.isLoading = false
		m.currentAssistant = nil
		m.statusMsg = "Error: " + msg.Error
		msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
		m.msgIDSeq++
		errItem := NewUserMessageItem(&Message{
			ID:      msgID,
			Role:    "error",
			Content: msg.Error,
		}, m.styles)
		m.messageItems = append(m.messageItems, errItem)
		return m, nil

	case events.PermissionRequest:
		m.pendingConfirm = &ConfirmationState{
			RequestID: msg.RequestID,
			SessionID: msg.SessionID,
			ToolName:  msg.ToolName,
			Command:   msg.Command,
		}
		m.statusMsg = fmt.Sprintf("确认执行 %s? [y]允许一次 [a]会话允许 [n]拒绝", msg.ToolName)
		return m, nil

	case events.SubagentRoleSwitch:
		m.subagentRole = msg.SubagentName
		msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
		m.msgIDSeq++
		item := NewSubagentMessageItem(&Message{
			ID:      msgID,
			Role:    "subagent",
			Content: msg.SubagentName,
		}, msg.SubagentName, m.styles)
		m.messageItems = append(m.messageItems, item)
		m.currentSubagentItem = item
		m.selectedSubagentIndex = len(m.messageItems) - 1
		m.statusMsg = "Subagent: " + msg.SubagentName
		return m, nil

	case events.SubagentToolCall:
		item := m.subagentItemFor(msg.SubagentName)
		if item != nil {
			item.AddToolCall(msg.ToolCallID, msg.ToolName, string(msg.Params))
			m.selectedSubagentIndex = m.latestExpandableSubagentIndex()
		}
		m.statusMsg = "Subagent tool: " + msg.ToolName
		return m, nil

	case events.SubagentToolResult:
		item := m.subagentItemFor(msg.SubagentName)
		if item != nil {
			item.SetToolResult(msg.ToolCallID, msg.ToolName, msg.Result, msg.Error)
			m.selectedSubagentIndex = m.latestExpandableSubagentIndex()
		}
		m.statusMsg = "Subagent tool done: " + msg.ToolName
		return m, nil

	case events.SubagentTaskComplete:
		item := m.subagentItemFor(msg.SubagentName)
		if item != nil {
			item.SetSummary(msg.Summary, msg.Error)
			m.selectedSubagentIndex = m.latestExpandableSubagentIndex()
		}
		if msg.Error != "" {
			m.statusMsg = msg.Error
		} else {
			m.statusMsg = "Subagent 完成: " + msg.SubagentName
		}
		return m, nil

	case events.SubagentRoleRestore:
		m.subagentRole = ""
		m.currentSubagentItem = nil
		return m, nil
	}

	return m, nil
}

func (m Model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action, allowed, ok := permissionKeyAction(msg.String())
	if !ok {
		return m, nil
	}
	pending := m.pendingConfirm
	m.pendingConfirm = nil
	m.statusMsg = "已处理权限请求: " + action
	m.pub.Publish(events.PermissionResponse{
		RequestID: pending.RequestID,
		SessionID: m.sessionID,
		Allowed:   allowed,
		Action:    action,
		Time:      time.Now(),
	})
	return m, nil
}

func permissionKeyAction(key string) (string, bool, bool) {
	switch key {
	case "y", "Y":
		return "allow", true, true
	case "a", "A":
		return "allow_session", true, true
	case "n", "N", "esc":
		return "deny", false, true
	default:
		return "", false, false
	}
}

// findPendingToolItem 从末尾查找匹配的待处理工具项
func (m *Model) findPendingToolItem(toolName string) *ToolMessageItem {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if ti, ok := m.messageItems[i].(*ToolMessageItem); ok {
			if ti.toolName == toolName && ti.status == ToolStatusRunning {
				return ti
			}
		}
	}
	return nil
}

func (m Model) latestExpandableToolIndex() int {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if item, ok := m.messageItems[i].(*ToolMessageItem); ok && item.resultContent != "" {
			return i
		}
	}
	return -1
}

func (m Model) expandedToolIndex() int {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if item, ok := m.messageItems[i].(*ToolMessageItem); ok && item.expandedContent {
			return i
		}
	}
	return -1
}

func (m Model) toggleSelectedToolDetails() (Model, bool) {
	index := m.expandedToolIndex()
	if index < 0 {
		index = m.selectedToolIndex
	}
	if index < 0 || index >= len(m.messageItems) {
		index = m.latestExpandableToolIndex()
	}
	if index < 0 {
		return m, false
	}
	item, ok := m.messageItems[index].(*ToolMessageItem)
	if !ok || item.resultContent == "" {
		return m, false
	}
	item.ToggleContent()
	m.selectedToolIndex = index
	if item.expandedContent {
		m.statusMsg = "已展开工具详情: " + item.toolName
	} else {
		m.statusMsg = "已收起工具详情: " + item.toolName
	}
	return m, true
}

func (m Model) latestExpandableSubagentIndex() int {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if item, ok := m.messageItems[i].(*SubagentMessageItem); ok && (item.summary != "" || item.errorText != "" || len(item.tools) > 0) {
			return i
		}
	}
	return -1
}

func (m Model) expandedSubagentIndex() int {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if item, ok := m.messageItems[i].(*SubagentMessageItem); ok && item.expanded {
			return i
		}
	}
	return -1
}

func (m Model) toggleSelectedSubagentDetails() (Model, bool) {
	index := m.expandedSubagentIndex()
	if index < 0 {
		index = m.selectedSubagentIndex
	}
	if index < 0 || index >= len(m.messageItems) {
		index = m.latestExpandableSubagentIndex()
	}
	if index < 0 {
		return m, false
	}
	item, ok := m.messageItems[index].(*SubagentMessageItem)
	if !ok {
		return m, false
	}
	item.ToggleContent()
	m.selectedSubagentIndex = index
	if item.expanded {
		m.statusMsg = "已展开 Subagent 执行详情: " + item.name
	} else {
		m.statusMsg = "已收起 Subagent 执行详情: " + item.name
	}
	return m, true
}

// View 渲染界面 — 使用 MessageItem 接口委托渲染
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	s := m.styles
	var b strings.Builder

	// 标题栏
	titleStyle := lipgloss.NewStyle().
		Background(s.Primary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 2).
		Width(m.width)
	b.WriteString(titleStyle.Render("AICodeAgent — Terminal AI Coding Assistant"))
	b.WriteString("\n")

	// 消息区 — 使用 MessageItem.Render()
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	messageView := renderVisibleMessageLines(m.messageItems, contentWidth, max(1, m.height-8), m.scrollPos)
	if messageView.hiddenBelow > 0 {
		scrollHint := lipgloss.NewStyle().
			Foreground(s.FgSubtle).
			PaddingLeft(2).
			Render(fmt.Sprintf("↑ %d lines below (End键回到最新)", messageView.hiddenBelow))
		b.WriteString(scrollHint)
		b.WriteString("\n")
	}

	if messageView.content != "" {
			b.WriteString(messageView.content)
			b.WriteString("\n")
		}

	if m.pendingConfirm != nil {
		confirmStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(s.Warning).
			Foreground(s.Warning).
			Padding(0, 1).
			Width(contentWidth)
		b.WriteString("\n")
		b.WriteString(confirmStyle.Render(fmt.Sprintf(
			"权限确认: %s\n参数: %s\n[y] 允许一次   [a] 本会话允许   [n] 拒绝",
			m.pendingConfirm.ToolName,
			m.pendingConfirm.Command,
		)))
		b.WriteString("\n")
	}

	// 状态栏 — 带旋转动画
	if m.isLoading {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		statusStyle := lipgloss.NewStyle().
			Foreground(s.Primary).
			PaddingLeft(2)
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(frame + " " + m.statusMsg))
		b.WriteString("\n")
	} else if m.statusMsg != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(s.FgMuted).
			PaddingLeft(2)
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.statusMsg))
		b.WriteString("\n")
	}
	if m.tokenStatus != "" {
		tokenStyle := lipgloss.NewStyle().
			Foreground(s.FgMuted).
			PaddingLeft(2)
		if m.tokenWarning {
			tokenStyle = tokenStyle.Foreground(s.Primary).Bold(true)
		}
		b.WriteString(tokenStyle.Render(m.tokenStatus))
		b.WriteString("\n")
	}

	if m.subagentRole != "" {
		roleStyle := lipgloss.NewStyle().
			Foreground(s.Primary).
			Bold(true).
			PaddingLeft(2)
		b.WriteString(roleStyle.Render("Subagent: " + m.subagentRole))
		b.WriteString("\n")
	}

	// 命令提示区
	if !m.isLoading {
		matches := m.matchingSlashSuggestions(m.input)
		if len(matches) > 0 {
			hintStyle := lipgloss.NewStyle().
				Foreground(s.FgMuted).
				PaddingLeft(2)
			selectedStyle := lipgloss.NewStyle().
				Foreground(s.Primary).
				Bold(true)

			b.WriteString("\n")
			b.WriteString(hintStyle.Render("Slash suggestions:"))
			b.WriteString("\n")
			selected := m.clampSlashSuggestionIndex(m.input, m.slashSuggestionIndex)
			for i, command := range matches {
				line := fmt.Sprintf("  %s  %s", command.Command, command.Description)
				if i == selected {
					line += "  (Tab/Enter)"
					b.WriteString(selectedStyle.Render(line))
				} else {
					b.WriteString(hintStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}

		subagentMatches := m.matchingSubagentNames(m.input)
		if len(subagentMatches) > 0 {
			hintStyle := lipgloss.NewStyle().
				Foreground(s.FgMuted).
				PaddingLeft(2)

			b.WriteString("\n")
			b.WriteString(hintStyle.Render("Subagents:"))
			b.WriteString("\n")
			for _, name := range subagentMatches {
				b.WriteString(hintStyle.Render("  " + name))
				b.WriteString("\n")
			}
		}
	}

	// 输入区
	inputStyle := lipgloss.NewStyle().
		BorderTop(true).
		BorderForeground(s.Primary).
		Padding(0, 1).
		Width(contentWidth)

	b.WriteString(inputStyle.Render(m.renderedInputLine()))

	return b.String()
}

func (m *Model) subagentItemFor(name string) *SubagentMessageItem {
	if m.currentSubagentItem != nil && m.currentSubagentItem.name == name {
		return m.currentSubagentItem
	}
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		item, ok := m.messageItems[i].(*SubagentMessageItem)
		if ok && item.name == name {
			return item
		}
	}
	return nil
}

type renderedMessageView struct {
	content     string
	hiddenBelow int
}

func renderVisibleMessageLines(items []MessageItem, width, maxVisible, scrollPos int) renderedMessageView {
	if len(items) == 0 || maxVisible <= 0 {
		return renderedMessageView{}
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		rendered := strings.TrimRight(item.Render(width), "\n")
		if rendered == "" {
			continue
		}
		lines = append(lines, strings.Split(rendered, "\n")...)
	}
	if len(lines) == 0 {
		return renderedMessageView{}
	}

	scrollPos = max(0, scrollPos)
	maxScroll := max(0, len(lines)-maxVisible)
	if scrollPos > maxScroll {
		scrollPos = maxScroll
	}

	end := len(lines) - scrollPos
	start := max(0, end-maxVisible)
	return renderedMessageView{
		content:     strings.Join(lines[start:end], "\n"),
		hiddenBelow: scrollPos,
	}
}

// ListenEvents 在独立 goroutine 中订阅事件，转换为 tea.Msg
func (m *Model) ListenEvents(ctx context.Context, program *tea.Program) {
	ch := m.sub.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			program.Send(event)
		}
	}
}

// Start 启动 TUI 循环（使用 UI 组件架构）
func Start(a *app.App) error {
	logFile, logErr := os.OpenFile("aicodeagent.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logErr == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	ui := NewUI(a)
	p := tea.NewProgram(ui)

	ctx, cancel := context.WithCancel(a.Ctx)
	defer cancel()

	a.StartEventLoop(ctx)

	go ui.ListenEvents(ctx, p)

	_, err := p.Run()
	return err
}

// spinnerTick 50ms 刷新定时器（20fps，例9-5）
// lastAssistantText 返回最后一条 AI 助手的消息文本
func (m *Model) lastAssistantText() string {
	for i := len(m.messageItems) - 1; i >= 0; i-- {
		if a, ok := m.messageItems[i].(*AssistantMessageItem); ok && a.message.Content != "" {
			return a.message.Content
		}
	}
	return ""
}

func spinnerTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}
