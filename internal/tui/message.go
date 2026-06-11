package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// glamourRenderers 按宽度缓存的 Markdown 渲染器（避免重复创建和 OSC 终端探测）
var (
	glamourRenderers   = make(map[int]*glamour.TermRenderer)
	glamourRenderersMu sync.RWMutex
)

func getGlamourRenderer(width int) (*glamour.TermRenderer, error) {
	glamourRenderersMu.RLock()
	if r, ok := glamourRenderers[width]; ok {
		glamourRenderersMu.RUnlock()
		return r, nil
	}
	glamourRenderersMu.RUnlock()

	glamourRenderersMu.Lock()
	defer glamourRenderersMu.Unlock()
	// 双重检查
	if r, ok := glamourRenderers[width]; ok {
		return r, nil
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	glamourRenderers[width] = r
	return r, nil
}

// MessageItem 消息项接口（例9-2）
type MessageItem interface {
	ID() string
	Render(width int) string
	Height(width int) int
}

// cachedMessageItem 缓存机制 - 避免重复渲染
type cachedMessageItem struct {
	renderedCache string
	cacheWidth    int
}

func (c *cachedMessageItem) getCachedRender(width int) (string, bool) {
	if c.cacheWidth == width && c.renderedCache != "" {
		return c.renderedCache, true
	}
	return "", false
}

func (c *cachedMessageItem) setCache(rendered string, width int) {
	c.renderedCache = rendered
	c.cacheWidth = width
}

func (c *cachedMessageItem) invalidateCache() {
	c.renderedCache = ""
	c.cacheWidth = 0
}

// Message 通用消息结构
type Message struct {
	ID      string
	Role    string
	Content string
}

// UserMessageItem 用户消息渲染（主色调圆角边框 + Markdown）
type UserMessageItem struct {
	*cachedMessageItem
	message    *Message
	styles     *Styles
	mdRenderer *MarkdownRenderer
}

func NewUserMessageItem(msg *Message, styles *Styles) *UserMessageItem {
	return &UserMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message:           msg,
		styles:            styles,
	}
}

func (u *UserMessageItem) ID() string { return u.message.ID }

func (u *UserMessageItem) Render(width int) string {
	if cached, ok := u.getCachedRender(width); ok {
		return cached
	}
	contentWidth := max(10, width-6)
	display := "You: " + u.message.Content
	rendered := wrapText(display, contentWidth)
	result := u.styles.Chat.UserMsg.Width(width).Render(rendered)
	u.setCache(result, width)
	return result
}

func (u *UserMessageItem) Height(width int) int {
	rendered := u.Render(width)
	return strings.Count(rendered, "\n") + 1
}

// AssistantMessageItem AI 消息渲染（成功色粗左边框、可折叠思考过程、Markdown 内容）
type AssistantMessageItem struct {
	*cachedMessageItem
	message          *Message
	styles           *Styles
	thinkingExpanded bool
	thinkingContent  string
	isComplete       bool
	mdRenderer       *MarkdownRenderer
}

func NewAssistantMessageItem(msg *Message, styles *Styles) *AssistantMessageItem {
	return &AssistantMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message:           msg,
		styles:            styles,
	}
}

func (a *AssistantMessageItem) ID() string { return a.message.ID }

func (a *AssistantMessageItem) Render(width int) string {
	if cached, ok := a.getCachedRender(width); ok {
		return cached
	}

	var sections []string
	contentWidth := max(10, width-4)

	// 可折叠思考过程 — 使用微妙背景色区分
	if a.thinkingContent != "" {
		thinkingSection := a.renderThinking(contentWidth)
		sections = append(sections, thinkingSection)
	}

	// 主体 Markdown 内容
	if a.message.Content != "" {
		renderedContent := a.renderMarkdown(a.message.Content, contentWidth)
		sections = append(sections, renderedContent)
	}

	// 流式生成中显示光标
	if !a.isComplete && a.message.Content == "" && a.thinkingContent == "" {
		sections = append(sections, lipgloss.NewStyle().
			Foreground(a.styles.FgMuted).
			Render("..."))
	}

	result := a.styles.Chat.AIMsg.Width(width).Render(strings.Join(sections, "\n"))
	a.setCache(result, width)
	return result
}

func (a *AssistantMessageItem) renderThinking(width int) string {
	toggle := "▼"
	if !a.thinkingExpanded {
		toggle = "▶"
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(a.styles.FgMuted).
		Bold(true)

	var builder strings.Builder
	builder.WriteString(headerStyle.Render(toggle + " 思考过程"))
	builder.WriteString("\n")

	if a.thinkingExpanded {
		thinkingRendered := a.renderMarkdown(a.thinkingContent, width-2)
		boxed := a.styles.Chat.ThinkingBox.Width(width).Render(thinkingRendered)
		builder.WriteString(boxed)
	} else {
		preview := a.thinkingContent
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		builder.WriteString(lipgloss.NewStyle().
			Foreground(a.styles.FgSubtle).
			PaddingLeft(2).
			Render(preview))
	}

	return builder.String()
}

func (a *AssistantMessageItem) renderMarkdown(content string, width int) string {
	r, err := getGlamourRenderer(width)
	if err != nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return rendered
}

func (a *AssistantMessageItem) Height(width int) int {
	rendered := a.Render(width)
	return strings.Count(rendered, "\n") + 1
}

func (a *AssistantMessageItem) SetThinking(content string) {
	a.thinkingContent = content
	a.invalidateCache()
}

func (a *AssistantMessageItem) ToggleThinking() {
	a.thinkingExpanded = !a.thinkingExpanded
	a.invalidateCache()
}

func (a *AssistantMessageItem) AppendContent(delta string) {
	a.message.Content += delta
	a.invalidateCache()
}

func (a *AssistantMessageItem) SetComplete(info string) {
	a.isComplete = true
	a.invalidateCache()
}

// ToolMessageItem 工具消息渲染（警告色左边框、状态图标、可折叠内容）
type ToolMessageItem struct {
	*cachedMessageItem
	message         *Message
	styles          *Styles
	toolName        string
	status          ToolStatus
	expandedContent bool
	resultContent   string
}

type ToolStatus int

const (
	ToolStatusPending ToolStatus = iota
	ToolStatusRunning
	ToolStatusSuccess
	ToolStatusError
)

func NewToolMessageItem(msg *Message, toolName string, styles *Styles) *ToolMessageItem {
	return &ToolMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message:           msg,
		styles:            styles,
		toolName:          toolName,
		status:            ToolStatusPending,
	}
}

func (t *ToolMessageItem) ID() string { return t.message.ID }

func (t *ToolMessageItem) Render(width int) string {
	if cached, ok := t.getCachedRender(width); ok {
		return cached
	}

	icon := t.statusIcon()
	headerStyle := lipgloss.NewStyle().Bold(true)
	header := headerStyle.Render(icon + " " + t.toolName)

	var sections []string
	sections = append(sections, header)

	if t.status == ToolStatusError && t.resultContent != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(t.styles.Error).
			PaddingLeft(2)
		errText := t.resultContent
		if len(errText) > 200 {
			errText = errText[:200] + "..."
		}
		sections = append(sections, errStyle.Render(errText))
	} else if t.expandedContent && t.resultContent != "" {
		resultText := t.resultContent
		if len(resultText) > 500 {
			resultText = resultText[:500] + "..."
		}
		resultStyle := lipgloss.NewStyle().
			Foreground(t.styles.FgMuted).
			PaddingLeft(2)
		sections = append(sections, resultStyle.Render(resultText))
	} else if t.resultContent != "" && !t.expandedContent {
		sections = append(sections, lipgloss.NewStyle().
			Foreground(t.styles.FgSubtle).
			PaddingLeft(2).
			Render("(按 Ctrl+E 展开详情；空闲时 Enter/Space 也可展开)"))
	}

	result := t.styles.Chat.ToolMsg.Width(width).Render(strings.Join(sections, "\n"))
	t.setCache(result, width)
	return result
}

func (t *ToolMessageItem) statusIcon() string {
	switch t.status {
	case ToolStatusSuccess:
		return t.styles.Tool.IconSuccess.Render("✓")
	case ToolStatusError:
		return t.styles.Tool.IconError.Render("✗")
	case ToolStatusRunning:
		return t.styles.Tool.IconRunning.Render("●")
	default:
		return t.styles.Tool.IconPending.Render(" ")
	}
}

func (t *ToolMessageItem) Height(width int) int {
	rendered := t.Render(width)
	return strings.Count(rendered, "\n") + 1
}

func (t *ToolMessageItem) SetStatus(status ToolStatus) {
	t.status = status
	t.invalidateCache()
}

func (t *ToolMessageItem) SetResult(content string) {
	t.resultContent = content
	t.invalidateCache()
}

func (t *ToolMessageItem) ToggleContent() {
	t.expandedContent = !t.expandedContent
	t.invalidateCache()
}

// wrapText 简单文本换行（用于用户消息等纯文本场景）
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var result strings.Builder
	remaining := text
	for len(remaining) > width {
		result.WriteString(remaining[:width])
		result.WriteString("\n")
		remaining = remaining[width:]
	}
	if len(remaining) > 0 {
		result.WriteString(remaining)
	}
	return result.String()
}
