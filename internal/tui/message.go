package tui

import (
	"fmt"
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

// SystemMessageItem 系统消息渲染（subagent 生命周期等）
type SubagentToolStep struct {
	ID       string
	Name     string
	Params   string
	Result   string
	Error    string
	Complete bool
}

// SubagentMessageItem 渲染子代理执行树，默认折叠，按键可展开。
type SubagentMessageItem struct {
	*cachedMessageItem
	message    *Message
	styles     *Styles
	name       string
	summary    string
	errorText  string
	expanded   bool
	completed  bool
	tools      []SubagentToolStep
	mdRenderer *MarkdownRenderer
}

func NewSubagentMessageItem(msg *Message, name string, styles *Styles) *SubagentMessageItem {
	return &SubagentMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message:           msg,
		styles:            styles,
		name:              name,
		expanded:          false,
	}
}

func (s *SubagentMessageItem) ID() string { return s.message.ID }

func (s *SubagentMessageItem) Render(width int) string {
	if cached, ok := s.getCachedRender(width); ok {
		return cached
	}
	contentWidth := max(10, width-4)
	toggle := "▶"
	if s.expanded {
		toggle = "▼"
	}
	status := "运行中"
	if s.completed && s.errorText == "" {
		status = "完成"
	} else if s.completed {
		status = "失败"
	}
	header := lipgloss.NewStyle().Bold(true).Render(toggle + " Subagent: " + s.name + " · " + status)
	sections := []string{header}

	if !s.expanded {
		sections = append(sections, s.collapsedPreview())
	} else {
		sections = append(sections, s.expandedBody(contentWidth))
	}

	result := s.styles.Chat.ToolMsg.Width(width).Render(strings.Join(sections, "\n"))
	s.setCache(result, width)
	return result
}

func (s *SubagentMessageItem) collapsedPreview() string {
	lines := make([]string, 0, len(s.tools)+2)
	for _, tool := range s.tools {
		icon := "●"
		if tool.Complete && tool.Error == "" {
			icon = "✓"
		} else if tool.Complete {
			icon = "✗"
		}
		lines = append(lines, "  ├── "+icon+" "+tool.Name)
	}
	if s.summary != "" {
		lines = append(lines, "  ╰── 摘要已生成")
	} else if s.completed && s.errorText != "" {
		lines = append(lines, "  ╰── 执行失败")
	} else {
		lines = append(lines, "  ╰── 等待结果")
	}
	lines = append(lines, "  (按 Ctrl+E 展开/收起；空闲时 Enter/Space 也可切换)")
	return lipgloss.NewStyle().Foreground(s.styles.FgSubtle).Render(strings.Join(lines, "\n"))
}

func (s *SubagentMessageItem) expandedBody(width int) string {
	var sections []string
	for i, tool := range s.tools {
		prefix := "├──"
		if i == len(s.tools)-1 && s.summary == "" && s.errorText == "" {
			prefix = "╰──"
		}
		sections = append(sections, lipgloss.NewStyle().Bold(true).Render("  "+prefix+" "+tool.Name))
		if tool.Params != "" {
			sections = append(sections, indentSubagentBlock("参数: "+tool.Params, 6))
		}
		if tool.Result != "" {
			sections = append(sections, indentSubagentBlock(limitLines(tool.Result, 20), 6))
		}
	}
	if s.errorText != "" {
		sections = append(sections, lipgloss.NewStyle().Foreground(s.styles.Error).Render("  ╰── "+s.errorText))
	} else if s.summary != "" {
		sections = append(sections, lipgloss.NewStyle().Bold(true).Render("  ╰── 最终总结"))
		sections = append(sections, indentSubagentBlock(s.renderMarkdown(s.summary, width-6), 6))
	}
	return strings.Join(sections, "\n")
}

func (s *SubagentMessageItem) renderMarkdown(content string, width int) string {
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

func (s *SubagentMessageItem) Height(width int) int {
	rendered := s.Render(width)
	return strings.Count(rendered, "\n") + 1
}

func (s *SubagentMessageItem) AddToolCall(id, name, params string) {
	for i := range s.tools {
		if s.tools[i].ID == id {
			s.tools[i].Name = name
			s.tools[i].Params = params
			s.invalidateCache()
			return
		}
	}
	s.tools = append(s.tools, SubagentToolStep{ID: id, Name: name, Params: params})
	s.invalidateCache()
}

func (s *SubagentMessageItem) SetToolResult(id, name, result, errorText string) {
	for i := range s.tools {
		if s.tools[i].ID == id {
			s.tools[i].Name = name
			s.tools[i].Result = result
			s.tools[i].Error = errorText
			s.tools[i].Complete = true
			s.invalidateCache()
			return
		}
	}
	s.tools = append(s.tools, SubagentToolStep{ID: id, Name: name, Result: result, Error: errorText, Complete: true})
	s.invalidateCache()
}

func (s *SubagentMessageItem) SetSummary(summary, errorText string) {
	s.summary = summary
	s.errorText = errorText
	s.completed = true
	s.invalidateCache()
}

func (s *SubagentMessageItem) ToggleContent() {
	s.expanded = !s.expanded
	s.invalidateCache()
}

func indentSubagentBlock(content string, spaces int) string {
	padding := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i, line := range lines {
		lines[i] = padding + line
	}
	return strings.Join(lines, "\n")
}

func limitLines(content string, maxLines int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:maxLines], "\n") + fmt.Sprintf("\n... (%d lines hidden)", len(lines)-maxLines)
}

type SystemMessageItem struct {
	*cachedMessageItem
	message *Message
	styles  *Styles
}

func NewSystemMessageItem(msg *Message, styles *Styles) *SystemMessageItem {
	return &SystemMessageItem{
		cachedMessageItem: &cachedMessageItem{},
		message:           msg,
		styles:            styles,
	}
}

func (s *SystemMessageItem) ID() string { return s.message.ID }

func (s *SystemMessageItem) Render(width int) string {
	if cached, ok := s.getCachedRender(width); ok {
		return cached
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true).
		PaddingLeft(2)
	result := style.Render("[系统] " + s.message.Content)
	s.setCache(result, width)
	return result
}

func (s *SystemMessageItem) Height(width int) int {
	rendered := s.Render(width)
	return strings.Count(rendered, "\n") + 1
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
