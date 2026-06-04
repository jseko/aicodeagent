package agent

import (
	"context"
	"fmt"
	"sync/atomic"

	"AICodeAgent/internal/llm"
)

// 复杂度估算阈值常量
const (
	smallModelThreshold  = 0.3  // 小模型选择阈值
	maxComplexity        = 1.0  // 复杂度上限
	shortPromptLen       = 50   // 短prompt字符数
	mediumPromptLen      = 200  // 中等prompt字符数
	shortPromptScore     = 0.0  // 短prompt分数
	mediumPromptScore    = 0.2  // 中等prompt分数
	longPromptScore      = 0.4  // 长prompt分数
	historyLenThreshold  = 10   // 历史消息数量阈值
	longHistoryScore     = 0.3  // 长历史分数加成
	defaultTemperature   = 0.7  // 默认温度参数
)

// sessionAgent 双模型会话代理（实现SessionAgent接口）
type sessionAgent struct {
	largeModel        atomic.Pointer[Model]
	smallModel        atomic.Pointer[Model]
	systemPrompt      string
	smallSystemPrompt string
	isSubAgent        bool
	todos             []string
	attachments       []Attachment
	temperature       float64
}

// newSessionAgent 创建会话代理实例
func newSessionAgent(large, small *Model) *sessionAgent {
	a := &sessionAgent{}
	if large != nil {
		a.largeModel.Store(large)
	}
	if small != nil {
		a.smallModel.Store(small)
	}
	return a
}

// LargeModel 获取大模型
func (a *sessionAgent) LargeModel() *Model {
	return a.largeModel.Load()
}

// SmallModel 获取小模型
func (a *sessionAgent) SmallModel() *Model {
	return a.smallModel.Load()
}

// SelectModel 根据复杂度选择模型
func (a *sessionAgent) SelectModel(complexity float64) *Model {
	if complexity < smallModelThreshold {
		if m := a.smallModel.Load(); m != nil {
			return m
		}
	}
	return a.largeModel.Load()
}

// estimateComplexity 根据输入简单估算任务复杂度
func estimateComplexity(prompt string, historyLen int) float64 {
	score := 0.0
	if len(prompt) < shortPromptLen {
		score += shortPromptScore
	} else if len(prompt) < mediumPromptLen {
		score += mediumPromptScore
	} else {
		score += longPromptScore
	}
	if historyLen > historyLenThreshold {
		score += longHistoryScore
	}
	if score > maxComplexity {
		score = maxComplexity
	}
	return score
}

// Run 执行会话代理调用（流式输出）
func (a *sessionAgent) Run(ctx context.Context, call SessionAgentCall) (*AgentResult, error) {
	// 1. 根据复杂度选择模型
	complexity := call.Complexity
	if complexity == 0 {
		complexity = estimateComplexity(call.Prompt, 0)
	}
	model := a.SelectModel(complexity)
	if model == nil {
		return nil, fmt.Errorf("no model available")
	}

	// 2. 构建Provider
	provider, err := a.buildProvider(model)
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}

	// 3. 选择对应模型类型的系统提示词
	prompt := a.systemPrompt
	if model == a.smallModel.Load() {
		prompt = a.smallSystemPrompt
	}

	// 4. 构建消息
	messages := a.buildMessages(prompt, call)

	// 5. 流式调用LLM
	streamCall := llm.AgentStreamCall{
		Prompt:      call.Prompt,
		Messages:    messages,
		Temperature: a.temperature,
	}
	ch, err := provider.Stream(ctx, streamCall)
	if err != nil {
		provider.Close()
		return &AgentResult{Session: call.Session, Error: err}, err
	}

	// 6. 返回流式通道，Provider在readSSE完成后自动关闭
	result := &AgentResult{
		Session: call.Session,
		Stream:  ch,
	}

	return result, nil
}

// buildProvider 根据模型配置构建对应的Provider实例
func (a *sessionAgent) buildProvider(model *Model) (llm.Provider, error) {
	return BuildProvider(model)
}

// BuildProvider 根据模型配置构建Provider（导出供外部使用）
func BuildProvider(model *Model) (llm.Provider, error) {
	switch model.Provider {
	case "openai", "":
		return llm.NewOpenAIProvider(model.Config.BaseURL, model.Config.APIKey, model.Name), nil
	case "anthropic":
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "google":
		return nil, fmt.Errorf("google provider not yet implemented")
	default:
		// 兼容以OpenAI兼容模式运行的第三方服务(如DeepSeek)
		return llm.NewOpenAIProvider(model.Config.BaseURL, model.Config.APIKey, model.Name), nil
	}
}

// buildMessages 构建发送给LLM的消息列表
func (a *sessionAgent) buildMessages(systemPrompt string, call SessionAgentCall) []llm.Message {
	var msgs []llm.Message

	if systemPrompt != "" {
		msgs = append(msgs, llm.NewSystemMessage(systemPrompt))
	}

	return msgs
}
