package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// MarkdownRenderer Markdown 渲染器 - 封装 Glamour 提供降级策略（例9-7）
type MarkdownRenderer struct {
	glamourR *glamour.TermRenderer
	plainR   *glamour.TermRenderer
	maxWidth int
}

func NewMarkdownRenderer(theme string, maxWidth int) (*MarkdownRenderer, error) {
	glamourR, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(maxWidth),
	)
	if err != nil {
		return nil, err
	}

	plainR, err := glamour.NewTermRenderer(
		glamour.WithStylePath("notty"),
		glamour.WithWordWrap(maxWidth),
	)
	if err != nil {
		plainR = glamourR
	}

	return &MarkdownRenderer{glamourR: glamourR, plainR: plainR, maxWidth: maxWidth}, nil
}

// RenderIncremental 三级降级策略
func (mr *MarkdownRenderer) RenderIncremental(content string, width int) string {
	// Level 1: 检测未闭合代码块，临时添加闭合标记
	if mr.hasUnclosedCodeBlock(content) {
		tempContent := content + "\n```"
		rendered, err := mr.glamourR.Render(tempContent)
		if err == nil {
			return mr.removeTemporaryClosing(rendered)
		}
	}

	// Level 2: 尝试标准 Glamour 渲染
	rendered, err := mr.glamourR.Render(content)
	if err == nil {
		return rendered
	}

	// Level 3: 降级到纯文本 notty 渲染
	if mr.plainR != nil {
		plainRendered, err := mr.plainR.Render(content)
		if err == nil {
			return plainRendered
		}
	}

	// Level 4: 返回原始 Markdown 内容
	return content
}

func (mr *MarkdownRenderer) hasUnclosedCodeBlock(content string) bool {
	return strings.Count(content, "```")%2 != 0
}

func (mr *MarkdownRenderer) removeTemporaryClosing(rendered string) string {
	lines := strings.Split(rendered, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// Render 标准渲染（完整 Markdown）
func (mr *MarkdownRenderer) Render(content string) (string, error) {
	return mr.glamourR.Render(content)
}
