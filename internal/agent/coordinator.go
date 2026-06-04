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
	"AICodeAgent/internal/llm"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/agent/tools"
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
	toolRegistry *tools.ToolRegistry, permService *permission.PermissionService) Coordinator {
	c := &coordinator{
		config:      cfg,
		sessions:    sessions,
		messages:    messages,
		tools:       toolRegistry,
		permService: permService,
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
