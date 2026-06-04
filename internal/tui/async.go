package tui

import tea "github.com/charmbracelet/bubbletea"

// LLMResponseMsg 异步操作完成后的消息类型
type LLMResponseMsg struct {
	Content string
	Err     error
}

// callLLM 创建tea.Cmd：在goroutine中执行耗时操作
func callLLM(prompt string) tea.Cmd {
	return func() tea.Msg {
		// 这个函数在后台goroutine中执行
		// 实际实现将在第4章通过Provider接口完成
		return LLMResponseMsg{
			Content: "LLM 响应: " + prompt,
			Err:     nil,
		}
	}
}
