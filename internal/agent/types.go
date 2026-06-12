package agent

import (
	"context"
	"encoding/json"
	"errors"

	"AICodeAgent/internal/llm"
)

// AgentResult Agent执行结果
type AgentResult struct {
	Response  string
	Reasoning string // 推理/思考内容（DeepSeek-R1等）
	Session   *Session
	Error     error
	Stream    <-chan llm.StreamingChunk // 流式响应通道（非nil时表示流式输出）
	ToolCalls []llm.ToolCallDelta       // LLM返回的工具调用（非空时需要执行后继续对话）
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
	Complexity  float64       // 任务复杂度评估(0-1)，0表示允许使用小模型
	Tools       []any         // 工具定义（OpenAI function format），传给LLM
	Messages    []llm.Message // 对话历史（包含system prompt、历史消息、工具结果）
}

// SubagentInfo 子代理基本信息
type SubagentInfo struct {
	Name        string
	Description string
}

type SubagentRunner interface {
	List() []SubagentInfo
	Execute(ctx context.Context, name string, session *Session, input string) (string, error)
	Match(input string) (string, bool)
}

type SubagentExecutionObserver interface {
	OnSubagentToolCall(ctx context.Context, parentSessionID, subagentName, callID, toolName string, params json.RawMessage)
	OnSubagentToolResult(ctx context.Context, parentSessionID, subagentName, callID, toolName, result, errorText string)
}

type ObservableSubagentRunner interface {
	SubagentRunner
	SetObserver(observer SubagentExecutionObserver)
}

// ErrSubagentNotAvailable 子代理不可用
var ErrSubagentNotAvailable = errors.New("subagent not available")

// Model LLM模型配置
type Model struct {
	Provider string
	Name     string
	Config   ModelConfig
}

// ModelConfig 模型配置参数
type ModelConfig struct {
	MaxTokens   int64
	Temperature float64
	TopP        float64
	InputPer1M  float64
	OutputPer1M float64
	APIKey      string
	BaseURL     string
}

type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

type ModelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

type CostConfig struct {
	Pricing map[string]ModelPricing
}

type StopCondition struct {
	Reason    string
	Used      int64
	Window    int64
	Threshold int64
}
