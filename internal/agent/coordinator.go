package agent

import (
	"context"
	"encoding/json"
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
	tools        *tools.Registry
	toolCaller   *tools.ToolCaller
	currentAgent SessionAgent
	largeModel   atomic.Pointer[Model]
	smallModel   atomic.Pointer[Model]
	running      map[string]context.CancelFunc
	sessionIDs   map[string]string // 请求ID → 实际会话ID 映射
	mu           sync.RWMutex
	permService  *permission.PermissionService
	broker       pubsub.Publisher[events.Event]
	confirmFn    func(toolName, arguments string) bool // 可注入的确认回调，测试用
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
	toolRegistry *tools.Registry, permService *permission.PermissionService,
	broker pubsub.Publisher[events.Event]) Coordinator {
	c := &coordinator{
		config:      cfg,
		sessions:    sessions,
		messages:    messages,
		tools:       toolRegistry,
		permService: permService,
		broker:      broker,
		running:     make(map[string]context.CancelFunc),
		sessionIDs:  make(map[string]string),
	}
	c.toolCaller = tools.NewToolCaller(toolRegistry, permService)
	c.confirmFn = c.requestUserConfirmation

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
	toolList := c.tools.List()
	result := make([]Tool, 0, len(toolList))
	for _, t := range toolList {
		result = append(result, Tool{
			Name:        t.Name(),
			Description: t.Description(),
			Params:      extractParamSignatures(t.Parameters()),
		})
	}
	return result
}

// extractParamSignatures 从 JSON Schema 中提取参数签名
// 输入 {"properties":{"path":{"type":"string"}}}，输出 "path(string)"
func extractParamSignatures(schema json.RawMessage) string {
	if len(schema) == 0 {
		return ""
	}
	var s struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return ""
	}
	var parts []string
	for name, prop := range s.Properties {
		parts = append(parts, fmt.Sprintf("%s(%s)", name, prop.Type))
	}
	return strings.Join(parts, ", ")
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

// Run 执行Agent任务（ReAct循环：LLM调用→工具执行→结果反馈→循环）
func (c *coordinator) Run(ctx context.Context, sessionID string, prompt string, attachments ...Attachment) (*AgentResult, error) {
	// 1. 获取或创建会话（优先使用已映射的实际会话ID）
	c.mu.RLock()
	actualID, hasMapping := c.sessionIDs[sessionID]
	c.mu.RUnlock()

	var session *Session
	var err error
	if hasMapping {
		session, err = c.sessions.Get(ctx, actualID)
	}
	if session == nil && err == nil {
		session, err = c.sessions.Get(ctx, sessionID)
	}
	if err != nil {
		session, err = c.sessions.Create(ctx, "New Session")
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
		c.mu.Lock()
		c.sessionIDs[sessionID] = session.ID
		c.mu.Unlock()
	}

	// 2. 创建可取消上下文
	runCtx, cancel := context.WithCancel(ctx)

	// 3. 注册cancel函数
	c.mu.Lock()
	c.running[session.ID] = cancel
	c.mu.Unlock()

	// 4. 清理资源
	defer func() {
		c.mu.Lock()
		delete(c.running, session.ID)
		c.mu.Unlock()
	}()

	// 5. 确保Agent已初始化
	if c.currentAgent == nil {
		return nil, fmt.Errorf("agent not initialized yet, please wait")
	}

	// 6. 上下文窗口检查
	if c.checkContextWindow(session) {
		log.Printf("[Coordinator] 会话 %s 上下文窗口即将耗尽，触发自动总结", session.ID)
		if err := c.Summarize(runCtx, sessionID); err != nil {
			log.Printf("[Coordinator] 自动总结失败: %v", err)
		}
	}

	// 7. 获取工具定义（OpenAI Function格式）
	toolDefs := c.tools.ToOpenAIFunctions()
	llmTools := make([]any, len(toolDefs))
	for i, td := range toolDefs {
		type openaiTool struct {
			Type     string `json:"type"`
			Function any    `json:"function"`
		}
		llmTools[i] = openaiTool{Type: "function", Function: td}
	}

	// 8. 构建初始消息（系统提示词 + 用户输入）
	messages := c.buildInitialMessages(prompt)

	// 9. ReAct循环：最多10轮迭代，每轮最多2分钟
	const (
		maxReActIterations  = 10
		perIterationTimeout = 2 * time.Minute
	)
	for i := 0; i < maxReActIterations; i++ {
		log.Printf("[Coordinator] ReAct迭代 %d/%d（消息数=%d）", i+1, maxReActIterations, len(messages))

		// 每次LLM调用设置独立超时，防止单次调用永久阻塞
		iterCtx, iterCancel := context.WithTimeout(runCtx, perIterationTimeout)
		call := SessionAgentCall{
			Prompt:     "", // 用户消息已在 Messages 中，避免重复
			Session:    session,
			Complexity: estimateComplexity(prompt, len(messages)),
			Tools:      llmTools,
			Messages:   messages,
		}
		result, err := c.currentAgent.Run(iterCtx, call)
		iterCancel()
		if err != nil {
			return nil, err
		}

		// 如果有工具调用，执行并继续循环
		if len(result.ToolCalls) > 0 {
			log.Printf("[Coordinator] 执行%d个工具调用", len(result.ToolCalls))
			// 发布工具调用事件到UI
			for _, tc := range result.ToolCalls {
				log.Printf("[Coordinator] 工具: %s(%s)", tc.Function.Name, tc.Function.Arguments)
				c.broker.PublishMustDeliver(runCtx, events.ToolCall{
					SessionID: session.ID,
					ToolName:  tc.Function.Name,
					Params:    json.RawMessage(tc.Function.Arguments),
					Time:      time.Now(),
				})
			}

			// 转换 ToolCallDelta → ToolCall 并执行
			agentToolCalls := convertToolCallDeltas(result.ToolCalls)
			toolResults, _ := c.handleToolCalls(runCtx, agentToolCalls)

			// 发布工具结果事件（关联工具名）
			for idx, tr := range toolResults {
				toolName := ""
				if idx < len(agentToolCalls) {
					toolName = agentToolCalls[idx].Function.Name
				}
				c.broker.PublishMustDeliver(runCtx, events.ToolResult{
					SessionID: session.ID,
					ToolName:  toolName,
					Result:    tr.Content,
					Time:      time.Now(),
				})
			}

			// 将助手消息（含tool_calls）和工具结果追加到消息历史
			messages = append(messages, buildAssistantToolCallsMsg(result.ToolCalls, result.Response))
			for _, tr := range toolResults {
				messages = append(messages, llm.Message{
					Role:       string(tr.Role),
					Content:    tr.Content,
					ToolCallID: tr.ToolCallID,
				})
			}
			continue
		}

		// 无工具调用：发布最终响应并返回
		log.Printf("[Coordinator] 最终响应（%d字符, 推理=%d字符）", len(result.Response), len(result.Reasoning))
		// 先发布推理内容（思考过程）
		if result.Reasoning != "" {
			c.broker.PublishMustDeliver(runCtx, events.AgentThink{
				SessionID:        session.ID,
				ReasoningContent: result.Reasoning,
				Time:             time.Now(),
			})
		}
		// 再发布正文内容
		c.broker.PublishMustDeliver(runCtx, events.AgentThink{
			SessionID: session.ID,
			Content:   result.Response,
			IsDone:    true,
			Time:      time.Now(),
		})
		return result, nil
	}

	return nil, fmt.Errorf("exceeded max ReAct iterations (%d)", maxReActIterations)
}

// buildInitialMessages 构建初始消息列表（系统提示词 + 用户输入）
func (c *coordinator) buildInitialMessages(prompt string) []llm.Message {
	var msgs []llm.Message
	if sa, ok := c.currentAgent.(*sessionAgent); ok {
		if sa.systemPrompt != "" {
			msgs = append(msgs, llm.NewSystemMessage(sa.systemPrompt))
		}
	}
	msgs = append(msgs, llm.NewUserMessage(prompt))
	return msgs
}

// convertToolCallDeltas 将LLM返回的ToolCallDelta转换为内部的ToolCall格式
func convertToolCallDeltas(deltas []llm.ToolCallDelta) []ToolCall {
	result := make([]ToolCall, len(deltas))
	for i, d := range deltas {
		result[i] = ToolCall{
			ID:   d.ID,
			Type: d.Type,
			Function: FunctionCall{
				Name:      d.Function.Name,
				Arguments: d.Function.Arguments,
			},
		}
	}
	return result
}

// buildAssistantToolCallsMsg 构建包含tool_calls的助手消息（用于对话历史）
func buildAssistantToolCallsMsg(deltas []llm.ToolCallDelta, textContent string) llm.Message {
	toolCalls := make([]llm.ToolCallDef, len(deltas))
	for i, d := range deltas {
		toolCalls[i] = llm.ToolCallDef{
			ID:   d.ID,
			Type: d.Type,
		}
		toolCalls[i].Function.Name = d.Function.Name
		toolCalls[i].Function.Arguments = d.Function.Arguments
	}
	return llm.Message{
		Role:      "assistant",
		Content:   textContent,
		ToolCalls: toolCalls,
	}
}

// HandleUserMessage 事件驱动处理用户消息（由外部事件循环调用）
func (c *coordinator) HandleUserMessage(msg events.UserMessage) {
	c.mu.Lock()
	if _, busy := c.running[msg.SessionID]; busy {
		c.mu.Unlock()
		log.Printf("[Coordinator] 会话 %s 正在处理中，跳过重复消息", msg.SessionID)
		return
	}
	c.running[msg.SessionID] = func() {} // 占位，Run() 将替换为实际 cancel
	c.mu.Unlock()

	go func() {
		// 确保占位符被清理
		defer func() {
			c.mu.Lock()
			delete(c.running, msg.SessionID)
			c.mu.Unlock()
		}()

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
		}
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
func (c *coordinator) detectToolCall(chunk llm.StreamingChunk) *events.ToolCall {
	if len(chunk.ToolCalls) == 0 {
		return nil
	}
	tc := chunk.ToolCalls[0]
	return &events.ToolCall{
		ToolName: tc.Function.Name,
		Params:   json.RawMessage(tc.Function.Arguments),
		Time:     time.Now(),
	}
}

// PublishToolResult 发布工具执行结果（由工具执行层调用）
func (c *coordinator) PublishToolResult(result events.ToolResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.broker.PublishMustDeliver(ctx, result)
}

// handleToolCalls 工具调用完整处理流程（查找→鉴权→确认→执行→结果）
// 对应 ch6.md 例6-5
func (c *coordinator) handleToolCalls(ctx context.Context, toolCalls []ToolCall) ([]Message, error) {
	var results []Message

	for _, call := range toolCalls {
		params := json.RawMessage(call.Function.Arguments)

		// 1-2. 工具查找+权限检查（通过 ToolCaller 共享逻辑）
		tool, allow, needConfirm, reason := c.toolCaller.ResolveAndCheck(call.Function.Name, params)
		if tool == nil {
			results = append(results, Message{
				Role:       RoleTool,
				Content:    fmt.Sprintf("错误：未知工具 '%s'", call.Function.Name),
				ToolCallID: call.ID,
			})
			continue
		}
		if !allow {
			if needConfirm {
				confirmed := c.confirmFn(call.Function.Name, call.Function.Arguments)
				if !confirmed {
					results = append(results, Message{
						Role:       RoleTool,
						Content:    "操作被用户拒绝",
						ToolCallID: call.ID,
					})
					continue
				}
			} else {
				results = append(results, Message{
					Role:       RoleTool,
					Content:    "错误：" + reason,
					ToolCallID: call.ID,
				})
				continue
			}
		}

		// 3. 执行工具
		result, err := tool.Execute(ctx, params)

		var content string
		if err != nil {
			content = fmt.Sprintf("执行失败: %v", err)
		} else if result.Success {
			content = result.Output
		} else {
			content = fmt.Sprintf("错误: %s", result.Error)
		}

		results = append(results, Message{
			Role:       RoleTool,
			Content:    content,
			ToolCallID: call.ID,
		})
	}

	return results, nil
}

// requestUserConfirmation 请求用户确认危险操作
func (c *coordinator) requestUserConfirmation(toolName, arguments string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.broker.PublishMustDeliver(ctx, events.ToolCall{
		SessionID: "",
		ToolName:  toolName,
		Params:    json.RawMessage(arguments),
		Time:      time.Now(),
	})

	return true
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
