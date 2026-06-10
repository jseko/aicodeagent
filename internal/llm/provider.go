package llm

import (
	"context"
	"strings"
)

// Message 对话消息
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type Message struct {
	Role             string        `json:"role"`
	Content          string        `json:"content,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCallDef `json:"tool_calls,omitempty"`
	ID               string        `json:"id,omitempty"`
	IsSummaryMessage bool          `json:"is_summary_message,omitempty"`
}

// ToolCallDef 工具调用定义（用于消息历史中的tool_calls字段）
type ToolCallDef struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// NewUserMessage 创建用户消息
func NewUserMessage(content string) Message {
	return Message{Role: "user", Content: content}
}

// NewAssistantMessage 创建助手消息
func NewAssistantMessage(content string) Message {
	return Message{Role: "assistant", Content: content}
}

// NewSystemMessage 创建系统消息
func NewSystemMessage(content string) Message {
	return Message{Role: "system", Content: content}
}

// NewFileMessage 创建文件附件消息
func NewFileMessage(content string) Message {
	return Message{Role: "user", Content: content}
}

// AgentStreamCall 统一的LLM调用参数
type AgentStreamCall struct {
	Prompt      string    // 当前用户输入
	Messages    []Message // 历史消息
	Tools       []any     // 工具定义（OpenAI function format）
	MaxTokens   int64     // 最大生成token数
	Temperature float64   // 随机性控制(0-2)
	TopP        float64   // 核采样阈值(0-1)
}

// StreamingChunk 流式响应块
type StreamingChunk struct {
	Content          string          // 文本内容
	ReasoningContent string          // 推理/思考内容（DeepSeek-R1 等推理模型）
	ToolCalls        []ToolCallDelta // 完整的工具调用（流结束后填充）
	Usage            *Usage          // API返回的token用量
	Done             bool            // 流结束标志
	Error            error           // 错误信息
}

// ToolCallDelta 工具调用增量（对应OpenAI streaming tool_calls delta）
type ToolCallDelta struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Provider LLM提供商统一接口
type Provider interface {
	Stream(ctx context.Context, call AgentStreamCall) (<-chan StreamingChunk, error)
	Chat(ctx context.Context, call AgentStreamCall) (string, error)
	Close() error
}

// Chat 便捷同步方法：内部调用Stream，收集所有chunk拼接返回
func Chat(ctx context.Context, p Provider, call AgentStreamCall) (string, error) {
	ch, err := p.Stream(ctx, call)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for chunk := range ch {
		if chunk.Error != nil {
			return sb.String(), chunk.Error
		}
		if chunk.Done {
			break
		}
		sb.WriteString(chunk.Content)
	}
	return sb.String(), nil
}
