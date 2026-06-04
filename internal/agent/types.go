package agent

import (
	"context"

	"AICodeAgent/internal/llm"
)

// AgentResult Agent执行结果
type AgentResult struct {
	Response string
	Session  *Session
	Error    error
	Stream   <-chan llm.StreamingChunk // 流式响应通道（非nil时表示流式输出）
}

// SessionAgent 会话代理接口
type SessionAgent interface {
	Run(ctx context.Context, call SessionAgentCall) (*AgentResult, error)
	SelectModel(complexity float64) *Model
}

// SessionAgentCall 会话代理调用参数
type SessionAgentCall struct {
	Prompt      string
	Session     *Session
	Attachments []Attachment
	Complexity  float64 // 任务复杂度评估(0-1)，0表示允许使用小模型
}

// Model LLM模型配置
type Model struct {
	Provider string
	Name     string
	Config   ModelConfig
}

// ModelConfig 模型配置参数
type ModelConfig struct {
	MaxTokens int64
	APIKey    string
	BaseURL   string
}
