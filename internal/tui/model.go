package tui

import (
	"context"
	"log"
	"os"
	"strings"
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
	width      int
	height     int
	history    []string
	historyIdx int
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
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.isLoading {
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if strings.TrimSpace(m.input) == "" {
				return m, nil
			}
			content := m.input
			m.messages = append(m.messages, "You: "+content)
			m.history = append(m.history, content)
			m.historyIdx = len(m.history)
			m.isLoading = true
			m.statusMsg = "正在思考..."
			m.pub.Publish(events.UserMessage{
				SessionID: m.sessionID,
				Content:   content,
			})
			m.input = ""
			return m, nil

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

	// 根据终端宽度动态设置消息样式，自动换行
	styled := msgStyle
	if m.width > 0 {
		styled = styled.Width(m.width - 4) // 留出内边距和滚动条空间
	}

	// 长会话性能优化：仅渲染最近 500 条消息
	start := 0
	if len(m.messages) > 500 {
		start = len(m.messages) - 500
	}
	for _, msg := range m.messages[start:] {
		b.WriteString(styled.Render(msg) + "\n")
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
	// 将 log 输出重定向到文件，防止干扰 TUI 终端渲染
	logFile, logErr := os.OpenFile("aicodeagent.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logErr == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

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
