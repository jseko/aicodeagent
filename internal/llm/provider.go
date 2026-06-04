package llm

import (
	"context"
	"strings"
)

// Message 对话消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	MaxTokens   int64     // 最大生成token数
	Temperature float64   // 随机性控制(0-2)
}

// StreamingChunk 流式响应块
type StreamingChunk struct {
	Content string // 文本内容
	Done    bool   // 流结束标志
	Error   error  // 错误信息
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
