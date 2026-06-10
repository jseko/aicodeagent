package agent

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"AICodeAgent/internal/llm"
)

// ModelType 模型类型
type ModelType int

const (
	LargeModel ModelType = iota
	SmallModel
)

// Rule 行为规则
type Rule struct {
	Title   string
	Content string
}

// Workflow 工作流程定义
type Workflow struct {
	Before []string
	During []string
	After  []string
}

// Tool 工具描述
type Tool struct {
	Name        string
	Description string
	Params      string
}

// EnvInfo 运行时环境信息
type EnvInfo struct {
	WorkingDir string
	IsGitRepo  bool
	GitStatus  string
	Platform   string
	Date       string
}

// SystemPromptBuilder 系统提示词构建器
type SystemPromptBuilder struct {
	identity string
	role     string
	rules    []Rule
	workflow Workflow
	tools    []Tool
	env      EnvInfo
}

// NewSystemPromptBuilder 创建提示词构建器
func NewSystemPromptBuilder(identity, role string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		identity: identity,
		role:     role,
	}
}

// WithRules 设置行为规则
func (b *SystemPromptBuilder) WithRules(rules []Rule) *SystemPromptBuilder {
	b.rules = rules
	return b
}

// WithWorkflow 设置工作流程
func (b *SystemPromptBuilder) WithWorkflow(w Workflow) *SystemPromptBuilder {
	b.workflow = w
	return b
}

// WithTools 设置工具说明
func (b *SystemPromptBuilder) WithTools(tools []Tool) *SystemPromptBuilder {
	b.tools = tools
	return b
}

// WithEnv 设置环境信息
func (b *SystemPromptBuilder) WithEnv(env EnvInfo) *SystemPromptBuilder {
	b.env = env
	return b
}

// Build 构建完整版系统提示词
func (b *SystemPromptBuilder) Build() string {
	var sb strings.Builder

	// 1. 身份定义
	sb.WriteString(fmt.Sprintf("你是 %s，%s\n\n", b.identity, b.role))

	// 2. 关键规则
	if len(b.rules) > 0 {
		sb.WriteString("<关键规则>\n")
		sb.WriteString("这些规则优先级最高，请严格遵守：\n\n")
		for i, rule := range b.rules {
			sb.WriteString(fmt.Sprintf("%d. **%s**：%s\n", i+1, rule.Title, rule.Content))
		}
		sb.WriteString("</关键规则>\n\n")
	}

	// 3. 工作流程
	if len(b.workflow.Before) > 0 || len(b.workflow.During) > 0 {
		sb.WriteString("<工作流程>\n")
		if len(b.workflow.Before) > 0 {
			sb.WriteString("**行动之前**：\n")
			for _, step := range b.workflow.Before {
				sb.WriteString(fmt.Sprintf("- %s\n", step))
			}
			sb.WriteString("\n")
		}
		if len(b.workflow.During) > 0 {
			sb.WriteString("**行动期间**：\n")
			for _, step := range b.workflow.During {
				sb.WriteString(fmt.Sprintf("- %s\n", step))
			}
			sb.WriteString("\n")
		}
		if len(b.workflow.After) > 0 {
			sb.WriteString("**完成之前**：\n")
			for _, step := range b.workflow.After {
				sb.WriteString(fmt.Sprintf("- %s\n", step))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("</工作流程>\n\n")
	}

	// 4. 环境信息
	sb.WriteString(b.buildEnvSection())

	return sb.String()
}

// BuildForModel 根据模型类型构建对应版本的提示词
func (b *SystemPromptBuilder) BuildForModel(modelType ModelType) string {
	switch modelType {
	case LargeModel:
		return b.Build()
	case SmallModel:
		return fmt.Sprintf(`你是 %s 的轻量级助手。

<规则>
1. 保持简洁，直接回答
2. 相关时分享文件名和代码片段
3. 使用绝对路径
</规则>

<环境>
工作目录：%s
平台：%s
</环境>`, b.identity, b.env.WorkingDir, b.env.Platform)
	default:
		return b.Build()
	}
}

// buildEnvSection 构建环境信息段落
func (b *SystemPromptBuilder) buildEnvSection() string {
	var sb strings.Builder
	sb.WriteString("<环境信息>\n")
	if b.env.WorkingDir != "" {
		sb.WriteString(fmt.Sprintf("工作目录：%s\n", b.env.WorkingDir))
	}
	sb.WriteString(fmt.Sprintf("是否为 Git 仓库：%s\n", map[bool]string{true: "是", false: "否"}[b.env.IsGitRepo]))
	sb.WriteString(fmt.Sprintf("平台：%s\n", b.env.Platform))
	sb.WriteString(fmt.Sprintf("今天的日期：%s\n", b.env.Date))

	if b.env.IsGitRepo && b.env.GitStatus != "" {
		sb.WriteString(fmt.Sprintf("\nGit 状态：\n%s\n", b.env.GitStatus))
	}
	sb.WriteString("</环境信息>")
	return sb.String()
}

// collectEnvInfo 收集运行时环境信息
func collectEnvInfo(workingDir string) EnvInfo {
	info := EnvInfo{
		WorkingDir: workingDir,
		Platform:   runtime.GOOS,
		Date:       "",
	}

	// 检查是否为 Git 仓库
	if _, err := os.Stat(workingDir + "/.git"); err == nil {
		info.IsGitRepo = true
	}

	return info
}

// DefaultRules 返回AICodeAgent的默认行为规则
func DefaultRules() []Rule {
	return []Rule{
		{Title: "编辑前必读", Content: "绝不要编辑未读取过的文件"},
		{Title: "保持自主性", Content: "自主决策，不频繁询问用户"},
		{Title: "修改即测试", Content: "每次修改后立即运行测试"},
		{Title: "保持简洁", Content: "输出控制在4行以内"},
		{Title: "精确匹配", Content: "编辑时精确匹配所有空格和缩进"},
	}
}

// DefaultWorkflow 返回默认工作流程
func DefaultWorkflow() Workflow {
	return Workflow{
		Before: []string{
			"使用 view 工具读取相关文件",
			"使用 grep 工具搜索代码",
			"分析问题根源，规划修改方案",
		},
		During: []string{
			"使用 write 工具进行精确修改",
			"使用 bash 工具执行命令和运行测试",
			"一次只修改一个文件",
		},
		After: []string{
			"运行测试验证修改",
			"检查是否有语法错误",
			"确认功能正常",
		},
	}
}

// preparePrompt 三层上下文构建：系统级→会话级→附件级
func (a *sessionAgent) preparePrompt(msgs []Message) []llm.Message {
	history := make([]llm.Message, 0, len(msgs)+len(a.attachments)+1)
	filtered := 0

	if !a.isSubAgent {
		reminder := a.buildTodoReminder()
		if reminder != "" {
			history = append(history, llm.NewUserMessage(
				fmt.Sprintf("<reminder>%s</reminder>", reminder),
			))
		}
	}

	for i := range msgs {
		msg := &msgs[i]
		if !msg.ShouldInclude() {
			filtered++
		}
	}
	history = append(history, a.buildContext("", nil, msgs)...)
	if len(msgs) > 0 && filtered > 0 {
		saved := float64(filtered) / float64(len(msgs)) * 100
		log.Printf("[SessionAgent] filtered %d/%d messages, saved ~%.0f%% tokens", filtered, len(msgs), saved)
	}

	for _, att := range a.attachments {
		history = append(history, llm.NewFileMessage(att.Content))
	}

	return history
}

func (a *sessionAgent) buildContext(systemPrompt string, session *Session, messages []Message) []llm.Message {
	contextMessages := make([]llm.Message, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		contextMessages = append(contextMessages, llm.NewSystemMessage(systemPrompt))
	}

	summaryID := ""
	if session != nil {
		summaryID = session.SummaryMessageID
	}
	for i := range messages {
		msg := &messages[i]
		if msg.IsSummaryMessage {
			if msg.ID == summaryID {
				contextMessages = append(contextMessages, msg.ToLLMMessage())
			}
			continue
		}
		if msg.ShouldInclude() {
			contextMessages = append(contextMessages, msg.ToLLMMessage())
		}
	}
	return contextMessages
}

// buildTodoReminder 构建待办提醒
func (a *sessionAgent) buildTodoReminder() string {
	if len(a.todos) == 0 {
		return ""
	}
	var items []string
	for i, todo := range a.todos {
		items = append(items, fmt.Sprintf("%d. %s", i+1, todo))
	}
	return "当前待办事项：\n" + strings.Join(items, "\n")
}

// Attachment 附件
type Attachment struct {
	Name    string
	Content string
}

// Message 代理层消息类型（与llm.Message对应）
type Message struct {
	ID               string
	Role             MessageRole
	Content          string
	ToolCallID       string // 工具调用ID，用于关联tool消息与请求
	Status           MessageStatus
	IsSummaryMessage bool
}

// MessageRole 消息角色
type MessageRole string

// MessageStatus 消息状态
type MessageStatus string

const (
	MessageStatusCompleted MessageStatus = "completed"
	MessageStatusCancelled MessageStatus = "cancelled"
)

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// ShouldInclude 判断消息是否应包含在上下文中
func (m *Message) ShouldInclude() bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.Content) == "" && m.ToolCallID == "" {
		return false
	}
	if m.Role == RoleAssistant && m.Status == MessageStatusCancelled {
		return false
	}
	return true
}

func (m *Message) ToLLMMessage() llm.Message {
	return llm.Message{
		Role:             string(m.Role),
		Content:          m.Content,
		ToolCallID:       m.ToolCallID,
		ID:               m.ID,
		IsSummaryMessage: m.IsSummaryMessage,
	}
}
