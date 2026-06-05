package tui

import (
	"context"
	"strings"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model 存储TUI所有状态
type Model struct {
	app        *app.App
	pub        pubsub.Publisher[events.Event]
	sub        pubsub.Subscriber[events.Event]
	sessionID  string
	messages   []string
	input      string
	cursor     int
	ready      bool
	isLoading  bool
	statusMsg  string
}

// 消息样式
var msgStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#86BBFC")).
	PaddingLeft(2)

func New(a *app.App) Model {
	return Model{
		app:       a,
		pub:       a.Broker,
		sub:       a.Broker,
		sessionID: "default",
		messages:  []string{},
		input:     "",
		ready:     false,
		isLoading: false,
		statusMsg: "",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

// Update 事件处理与状态更新
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		if m.isLoading {
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.input != "" {
				m.messages = append(m.messages, "You: "+m.input)
				m.isLoading = true
				m.statusMsg = "正在思考..."
				m.pub.Publish(events.UserMessage{
					SessionID: m.sessionID,
					Content:   m.input,
				})
				m.input = ""
				return m, nil
			}
			return m, nil
		default:
			m.input += msg.String()
			return m, nil
		}

	case events.AgentThink:
		if msg.Content != "" {
			m.appendStreamContent(msg.Content)
		}
		if msg.IsDone {
			m.isLoading = false
			m.statusMsg = "完成"
		} else {
			m.isLoading = true
		}
		return m, nil

	case events.ErrorEvent:
		m.isLoading = false
		m.statusMsg = "错误: " + msg.Error
		return m, nil

	case events.ToolCall:
		paramStr := string(msg.Params)
		if len(paramStr) > 80 {
			paramStr = paramStr[:80] + "..."
		}
		m.messages = append(m.messages, "[tool] "+msg.ToolName+" "+paramStr)
		m.statusMsg = "执行: " + msg.ToolName
		return m, nil

	case events.ToolResult:
		if msg.Error != "" {
			m.messages = append(m.messages, "[tool error] "+msg.Error)
			m.statusMsg = "工具错误: " + msg.Error
		} else if msg.Result != "" {
			shortResult := msg.Result
			if len(shortResult) > 100 {
				shortResult = shortResult[:100] + "..."
			}
			m.messages = append(m.messages, "[tool result] "+shortResult)
			m.statusMsg = "工具完成: " + msg.ToolName
		} else {
			m.statusMsg = "工具完成: " + msg.ToolName
		}
		return m, nil
	}

	return m, nil
}

// View 渲染界面
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// 长会话性能优化：仅渲染最近 500 条消息
	start := 0
	if len(m.messages) > 500 {
		start = len(m.messages) - 500
	}
	for _, msg := range m.messages[start:] {
		b.WriteString(msgStyle.Render(msg) + "\n")
	}

	if m.isLoading {
		b.WriteString("\n[思考中] " + m.statusMsg + "\n")
		b.WriteString("请稍候...")
	} else {
		b.WriteString("\n> " + m.input + "_")
	}

	return b.String()
}

func (m *Model) appendStreamContent(content string) {
	if len(m.messages) == 0 || !strings.HasPrefix(m.messages[len(m.messages)-1], "AI: ") {
		m.messages = append(m.messages, "AI: "+content)
	} else {
		m.messages[len(m.messages)-1] += content
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

// Start 启动 TUI 循环
func Start(a *app.App) error {
	m := New(a)
	p := tea.NewProgram(m)

	ctx, cancel := context.WithCancel(a.Ctx)
	defer cancel()

	// 启动 App 层事件循环（Coordinator ← Broker）
	a.StartEventLoop(ctx)

	// 启动 TUI 事件监听（Broker → tea.Msg）
	go m.ListenEvents(ctx, p)

	_, err := p.Run()
	return err
}
