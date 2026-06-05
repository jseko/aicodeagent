package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"AICodeAgent/internal/config"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/llm"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/pubsub"
	"golang.org/x/sync/errgroup"
)

const (
	modelInitTimeout = 30 * time.Second
)

// Coordinator Agent系统核心协调器接口
type Coordinator interface {
	Run(ctx context.Context, sessionID string, prompt string, attachments ...Attachment) (*AgentResult, error)
	Cancel(sessionID string)
	CancelAll()
	IsSessionBusy(sessionID string) bool
	IsBusy() bool
	Summarize(ctx context.Context, sessionID string) error
	Model() *Model
	UpdateModels(ctx context.Context) error
	HandleUserMessage(msg events.UserMessage)
	PublishToolResult(result events.ToolResult)
}

// coordinator Coordinator接口实现
type coordinator struct {
	config       *config.Config
	sessions     SessionService
	messages     MessageService
	tools        *tools.ToolRegistry
	toolCaller   *tools.ToolCaller
	currentAgent SessionAgent
	largeModel   atomic.Pointer[Model]
	smallModel   atomic.Pointer[Model]
	running      map[string]context.CancelFunc
	mu           sync.RWMutex
	permService  *permission.PermissionService
	broker       pubsub.Publisher[events.Event]
}

// MessageService 消息持久化服务接口
type MessageService interface {
	List(ctx context.Context, sessionID string) ([]Message, error)
	Create(ctx context.Context, sessionID string, params CreateMessageParams) (*Message, error)
}

// CreateMessageParams 创建消息参数
type CreateMessageParams struct {
	Role             MessageRole
	Content          string
	IsSummaryMessage bool
	ToolCalls        []ToolCall
}

// ToolCall 工具调用信息
type ToolCall struct {
	ID       string
	Type     string
	Function FunctionCall
}

// FunctionCall 函数调用详情
type FunctionCall struct {
	Name      string
	Arguments string
}

// NewCoordinator 创建Coordinator实例
func NewCoordinator(cfg *config.Config, sessions SessionService, messages MessageService,
	toolRegistry *tools.ToolRegistry, permService *permission.PermissionService,
	broker pubsub.Publisher[events.Event]) Coordinator {
	c := &coordinator{
		config:      cfg,
		sessions:    sessions,
		messages:    messages,
		tools:       toolRegistry,
		permService: permService,
		broker:      broker,
		running:     make(map[string]context.CancelFunc),
	}
	c.toolCaller = tools.NewToolCaller(toolRegistry, permService)

	// 异步初始化Agent（带超时控制）
	go func() {
		initCtx, cancel := context.WithTimeout(context.Background(), modelInitTimeout)
		defer cancel()
		if err := c.UpdateModels(initCtx); err != nil {
			log.Printf("[Coordinator] 模型初始化失败: %v", err)
		}
	}()

	return c
}

// buildAgent 异步构建Agent实例（使用errgroup并行构建大小模型）
func (c *coordinator) buildAgent(ctx context.Context, cfg config.AgentConfig) (SessionAgent, error) {
	eg, egCtx := errgroup.WithContext(ctx)
	var large, small *Model

	// 1. 并行构建大小模型
	eg.Go(func() error {
		m, err := c.buildModel(egCtx, cfg.LargeModel)
		if err != nil {
			return err
		}
		large = m
		return nil
	})
	eg.Go(func() error {
		m, err := c.buildModel(egCtx, cfg.SmallModel)
		if err != nil {
			return err
		}
		small = m
		return nil
	})

	// 2. 等待并行任务完成
	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("build agent: %w", err)
	}

	result := newSessionAgent(large, small)

	// 3. 同步加载系统提示词（避免数据竞争）
	c.loadSystemPrompt(cfg, result)

	return result, nil
}

// buildModel 构建单个模型实例
func (c *coordinator) buildModel(ctx context.Context, selected config.SelectedModel) (*Model, error) {
	providerCfg, err := llm.FindProviderConfig(c.config.Providers, selected.Provider)
	if err != nil {
		// 兼容旧配置：使用openai配置节
		return &Model{
			Provider: "openai",
			Name:     selected.Model,
			Config: ModelConfig{
				APIKey:  c.config.OpenAI.Key,
				BaseURL: llm.DefaultOpenAIBaseURL,
			},
		}, nil
	}

	apiKey := c.config.ResolveSecret(providerCfg.APIKey)
	return &Model{
		Provider: providerCfg.Type,
		Name:     selected.Model,
		Config: ModelConfig{
			APIKey:  apiKey,
			BaseURL: providerCfg.BaseURL,
		},
	}, nil
}

// loadSystemPrompt 加载系统提示词
func (c *coordinator) loadSystemPrompt(cfg config.AgentConfig, agent *sessionAgent) {
	envInfo := collectEnvInfo(".")
	builder := NewSystemPromptBuilder("AICodeAgent", "一个运行在终端的AI编程助手").
		WithRules(DefaultRules()).
		WithWorkflow(DefaultWorkflow()).
		WithEnv(envInfo).
		WithTools(c.collectToolDescriptions())

	agent.systemPrompt = builder.Build()
	agent.smallSystemPrompt = builder.BuildForModel(SmallModel)
	agent.temperature = c.config.OpenAI.Temperature
	log.Println("[Coordinator] 系统提示词加载完成")
}

// collectToolDescriptions 从工具注册表收集工具描述
func (c *coordinator) collectToolDescriptions() []Tool {
	metas := c.tools.ListAll()
	tools := make([]Tool, 0, len(metas))
	for _, meta := range metas {
		var params []string
		for _, p := range meta.Params {
			params = append(params, fmt.Sprintf("%s(%s)", p.Name, p.Type))
		}
		tools = append(tools, Tool{
			Name:        meta.Name,
			Description: meta.Description,
			Params:      strings.Join(params, ", "),
		})
	}
	return tools
}

// SelectModel 根据任务复杂度选择模型
func (c *coordinator) selectModel(complexity float64) *Model {
	if complexity < smallModelThreshold {
		return c.smallModel.Load()
	}
	return c.largeModel.Load()
}

// Model 获取当前使用的大模型
func (c *coordinator) Model() *Model {
	return c.largeModel.Load()
}

// UpdateModels 更新模型配置（运行时热更新）
func (c *coordinator) UpdateModels(ctx context.Context) error {
	agent, err := c.buildAgent(ctx, c.config.Agent)
	if err != nil {
		return err
	}

	// 交换新旧模型实例
	if oldAgent, ok := c.currentAgent.(*sessionAgent); ok {
		c.largeModel.Store(oldAgent.LargeModel())
		c.smallModel.Store(oldAgent.SmallModel())
	}

	c.currentAgent = agent
	return nil
}

// Run 执行Agent任务
func (c *coordinator) Run(ctx context.Context, sessionID string, prompt string, attachments ...Attachment) (*AgentResult, error) {
	// 1. 获取或创建会话
	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		session, err = c.sessions.Create(ctx, "New Session")
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}

	// 2. 创建可取消上下文
	runCtx, cancel := context.WithCancel(ctx)

	// 3. 注册cancel函数
	c.mu.Lock()
	c.running[session.ID] = cancel
	c.mu.Unlock()

	// 4. 清理资源（流消费完成后由collectAndClose关闭provider）
	defer func() {
		c.mu.Lock()
		delete(c.running, session.ID)
		c.mu.Unlock()
	}()

	// 5. 确保Agent已初始化
	if c.currentAgent == nil {
		return nil, fmt.Errorf("agent not initialized yet, please wait")
	}

	// 6. 上下文窗口检查，必要时自动总结
	if c.checkContextWindow(session) {
		log.Printf("[Coordinator] 会话 %s 上下文窗口即将耗尽，触发自动总结", session.ID)
		if err := c.Summarize(runCtx, sessionID); err != nil {
			log.Printf("[Coordinator] 自动总结失败: %v", err)
		}
	}

	// 7. 评估任务复杂度
	complexity := estimateComplexity(prompt, 0)

	// 8. 执行Agent调用
	call := SessionAgentCall{
		Prompt:      prompt,
		Session:     session,
		Attachments: attachments,
		Complexity:  complexity,
	}
	return c.currentAgent.Run(runCtx, call)
}

// HandleUserMessage 事件驱动处理用户消息（由外部事件循环调用）
func (c *coordinator) HandleUserMessage(msg events.UserMessage) {
	// 预注册 cancel 占位符，防止同一会话的竞态重复处理
	c.mu.Lock()
	if _, busy := c.running[msg.SessionID]; busy {
		c.mu.Unlock()
		log.Printf("[Coordinator] 会话 %s 正在处理中，跳过重复消息", msg.SessionID)
		return
	}
	c.running[msg.SessionID] = func() {} // 占位，Run() 将替换为实际 cancel
	c.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := c.Run(ctx, msg.SessionID, msg.Content)
		if err != nil {
			c.broker.PublishMustDeliver(ctx,
				events.ErrorEvent{SessionID: msg.SessionID, Error: err.Error(), Time: time.Now()})
			return
		}

		if result.Error != nil {
			c.broker.PublishMustDeliver(ctx,
				events.ErrorEvent{SessionID: msg.SessionID, Error: result.Error.Error(), Time: time.Now()})
			return
		}

		if result.Stream != nil {
			c.consumeStream(ctx, msg.SessionID, result.Stream)
		}

		c.broker.PublishMustDeliver(ctx,
			events.AgentThink{SessionID: msg.SessionID, IsDone: true, Time: time.Now()})
	}()
}

// consumeStream 消费 LLM 流式响应，发布 AgentThink 和 ToolCall 事件
func (c *coordinator) consumeStream(ctx context.Context, sessionID string, stream <-chan llm.StreamingChunk) {
	for chunk := range stream {
		if chunk.Error != nil {
			c.broker.PublishMustDeliver(ctx,
				events.ErrorEvent{SessionID: sessionID, Error: chunk.Error.Error(), Time: time.Now()})
			return
		}
		if chunk.Done {
			break
		}

		if toolCall := c.detectToolCall(chunk); toolCall != nil {
			c.broker.PublishMustDeliver(ctx, *toolCall)
			continue
		}

		c.broker.Publish(events.AgentThink{
			SessionID: sessionID,
			Content:   chunk.Content,
			Time:      time.Now(),
		})
	}
}

// detectToolCall 检测 LLM chunk 中的工具调用意图
// 当前返回 nil，待 LLM Provider 支持 function calling 后实现
func (c *coordinator) detectToolCall(chunk llm.StreamingChunk) *events.ToolCall {
	_ = chunk
	// TODO: 当 LLM Provider 支持 function calling 时，解析 chunk 中的
	// tool_calls 字段并返回 events.ToolCall{SessionID, ToolName, Params, Time}
	return nil
}

// PublishToolResult 发布工具执行结果（由工具执行层调用）
func (c *coordinator) PublishToolResult(result events.ToolResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.broker.PublishMustDeliver(ctx, result)
}

// Cancel 取消指定会话
func (c *coordinator) Cancel(sessionID string) {
	c.mu.RLock()
	cancel, ok := c.running[sessionID]
	c.mu.RUnlock()
	if ok {
		cancel()
	}
}

// CancelAll 取消所有运行中的会话
func (c *coordinator) CancelAll() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, cancel := range c.running {
		cancel()
	}
}

// IsSessionBusy 检查指定会话是否繁忙
func (c *coordinator) IsSessionBusy(sessionID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.running[sessionID]
	return ok
}

// IsBusy 检查是否有任何会话正在运行
func (c *coordinator) IsBusy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.running) > 0
}

// checkContextWindow 检查是否需要触发自动总结
func (c *coordinator) checkContextWindow(session *Session) bool {
	const (
		largeContextWindowThreshold = 200_000
		largeContextWindowBuffer    = 20_000
		smallContextWindowRatio     = 0.2
	)

	model := c.largeModel.Load()
	if model == nil {
		return false
	}

	cw := model.Config.MaxTokens
	tokens := session.CompletionTokens + session.PromptTokens
	remaining := cw - tokens

	var threshold int64
	if cw > largeContextWindowThreshold {
		threshold = largeContextWindowBuffer
	} else {
		threshold = int64(float64(cw) * smallContextWindowRatio)
	}

	return remaining <= threshold && !session.DisableAutoSummarize
}

// Summarize 生成会话摘要（使用小模型降低90%成本）
func (c *coordinator) Summarize(ctx context.Context, sessionID string) error {
	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session failed: %w", err)
	}

	smallModel := c.smallModel.Load()
	if smallModel == nil {
		return fmt.Errorf("small model not available")
	}

	provider, err := BuildProvider(smallModel)
	if err != nil {
		return fmt.Errorf("build summary provider: %w", err)
	}
	defer provider.Close()

	summaryPrompt := c.config.SummaryPrompt

	streamCall := llm.AgentStreamCall{
		Prompt:      summaryPrompt,
		Temperature: 0.3,
	}

	response, err := provider.Chat(ctx, streamCall)
	if err != nil {
		return fmt.Errorf("summarize failed: %w", err)
	}

	_, err = c.messages.Create(ctx, sessionID, CreateMessageParams{
		Role:             RoleAssistant,
		Content:          response,
		IsSummaryMessage: true,
	})
	if err != nil {
		return fmt.Errorf("save summary message: %w", err)
	}

	return c.sessions.Save(ctx, session)
}
