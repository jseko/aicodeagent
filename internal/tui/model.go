package tui

import (
	"strings"

	"AICodeAgent/internal/app"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model 存储TUI所有状态
type Model struct {
	app       *app.App
	messages  []string // 消息历史
	input     string   // 当前输入
	cursor    int      // 光标位置
	ready     bool     // 是否初始化完成
	isLoading bool     // 是否正在等待LLM响应
	statusMsg string   // 状态提示信息
}

// 消息样式
var msgStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#86BBFC")).
	PaddingLeft(2)

func New(a *app.App) Model {
	return Model{
		app:       a,
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
		// 加载中时不处理其他按键
		if m.isLoading {
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.input != "" {
				// 发送消息并清空输入
				m.messages = append(m.messages, "You: "+m.input)
				prompt := m.input
				m.input = ""
				m.isLoading = true
				m.statusMsg = "正在思考..."
				// 返回tea.Cmd，异步执行LLM调用
				return m, callLLM(prompt)
			}
			return m, nil
		default:
			m.input += msg.String()
			return m, nil
		}

	case LLMResponseMsg:
		// 异步操作完成后更新UI
		m.isLoading = false
		if msg.Err != nil {
			m.statusMsg = "错误: " + msg.Err.Error()
		} else {
			m.messages = append(m.messages, "AI: "+msg.Content)
			m.statusMsg = "完成"
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

	// 渲染消息历史
	for _, msg := range m.messages {
		b.WriteString(msgStyle.Render(msg) + "\n")
	}

	// 根据状态渲染输入区
	if m.isLoading {
		b.WriteString("\n[思考中] " + m.statusMsg + "\n")
		b.WriteString("请稍候...")
	} else {
		b.WriteString("\n> " + m.input + "_")
	}

	return b.String()
}

// Start 启动 TUI 循环
func Start(a *app.App) error {
	p := tea.NewProgram(New(a))
	_, err := p.Run()
	return err
}
