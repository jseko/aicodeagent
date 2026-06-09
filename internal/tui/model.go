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
	messageItems     []MessageItem
	currentAssistant *AssistantMessageItem
	msgIDSeq         int

	input      string
	cursor     int
	ready      bool
	isLoading  bool
	statusMsg  string
	width      int
	height     int
	history    []string
	historyIdx int
	styles     *Styles
	theme      *ThemeManager

	// 动画状态
	spinnerFrame int

	// 滚动状态（智能滚动，例9-5）
	scrollPos    int // 0=底部，正数=向上滚动行数
	wasAtBottom  bool
}

// SpinnerTickMsg 旋转动画定时消息
type SpinnerTickMsg struct{}

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

	return Model{
		app:          a,
		pub:          a.Broker,
		sub:          a.Broker,
		sessionID:    "default",
		messageItems: []MessageItem{},
		input:        "",
		ready:        false,
		isLoading:    false,
		statusMsg:    "",
		styles:       s,
		theme:        theme,
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

		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

		if m.isLoading {
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEnter:
			if strings.TrimSpace(m.input) == "" {
				return m, nil
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
			m.statusMsg = "Thinking..."
			m.pub.Publish(events.UserMessage{
				SessionID: m.sessionID,
				Content:   content,
			})
			m.input = ""
			return m, spinnerTick()

		case tea.KeyBackspace, tea.KeyDelete:
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
			return m, nil

		case tea.KeyUp:
			if len(m.history) > 0 && m.historyIdx > 0 {
				m.historyIdx--
				m.input = m.history[m.historyIdx]
			}
			return m, nil

		case tea.KeyDown:
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.input = m.history[m.historyIdx]
			} else {
				m.historyIdx = len(m.history)
				m.input = ""
			}
			return m, nil

		case tea.KeyRunes:
			for _, r := range msg.Runes {
				if r == utf8.RuneError || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
					continue
				}
				m.input += string(r)
			}
			return m, nil

		default:
			switch msg.String() {
			case "backspace", "ctrl+h", "delete":
				runes := []rune(m.input)
				if len(runes) > 0 {
					m.input = string(runes[:len(runes)-1])
				}
			}
			return m, nil
		}

	case events.AgentThink:
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
		m.currentAssistant = nil
		msgID := fmt.Sprintf("msg-%d", m.msgIDSeq)
		m.msgIDSeq++
		paramStr := string(msg.Params)
		item := NewToolMessageItem(&Message{
			ID:      msgID,
			Role:    "tool",
			Content: msg.ToolName + " " + paramStr,
		}, msg.ToolName, m.styles)
		item.SetStatus(ToolStatusRunning)
		m.messageItems = append(m.messageItems, item)
		m.statusMsg = "Running: " + msg.ToolName
		return m, spinnerTick()

	case events.ToolResult:
		item := m.findPendingToolItem(msg.ToolName)
		if item != nil {
			if msg.Error != "" {
				item.SetStatus(ToolStatusError)
				item.SetResult(msg.Error)
				m.statusMsg = "Tool error: " + msg.ToolName
			} else {
				item.SetStatus(ToolStatusSuccess)
				if msg.Result != "" {
					item.SetResult(msg.Result)
				}
				m.statusMsg = "Tool done: " + msg.ToolName
			}
		}
		return m, nil

	case events.ErrorEvent:
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
	}

	return m, nil
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

	// 智能滚动：计算可见范围（例9-5）
	maxVisible := max(1, m.height-8)
	visibleStart := max(0, len(m.messageItems)-maxVisible-m.scrollPos)
	visibleEnd := len(m.messageItems) - m.scrollPos
	if visibleEnd > len(m.messageItems) {
		visibleEnd = len(m.messageItems)
	}
	if visibleStart >= visibleEnd {
		visibleStart = max(0, visibleEnd-1)
	}

	// 向上滚动提示
	if m.scrollPos > 0 {
		scrollHint := lipgloss.NewStyle().
			Foreground(s.FgSubtle).
			PaddingLeft(2).
			Render(fmt.Sprintf("↑ %d messages below (End键回到最新)", m.scrollPos))
		b.WriteString(scrollHint)
		b.WriteString("\n")
	}

	for _, item := range m.messageItems[visibleStart:visibleEnd] {
		b.WriteString(item.Render(contentWidth))
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

	// 输入区
	inputStyle := lipgloss.NewStyle().
		BorderTop(true).
		BorderForeground(s.Primary).
		Padding(0, 1).
		Width(contentWidth)

	inputLine := "> " + m.input
	if !m.isLoading {
		inputLine += "_"
	}
	b.WriteString(inputStyle.Render(inputLine))

	return b.String()
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
func spinnerTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}
