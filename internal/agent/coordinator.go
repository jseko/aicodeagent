package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"AICodeAgent/internal/agent/prompt"
	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/config"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/llm"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/pubsub"
	"AICodeAgent/internal/rules"
	"AICodeAgent/internal/skills"
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
	SessionAgent() SessionAgent
	UpdateModels(ctx context.Context) error
	HandleUserMessage(msg events.UserMessage)
	PublishToolResult(result events.ToolResult)
	SetSubagentRunner(runner SubagentRunner)
	Undo(ctx context.Context, sessionID string) error
	Redo(ctx context.Context, sessionID string) error
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
	skills       *skills.Manager
	rulesLoader  *rules.Loader
	matcher      *SkillMatcher
	subagents    SubagentRunner
	confirmFn    func(toolName, arguments string) bool // 可注入的确认回调，测试用
	pendingInit  *initializeResult                     // 待确认的 /initialize 结果
	sub          pubsub.Subscriber[events.Event]
	pendingConfs map[string]chan bool
	confirmMu    sync.Mutex
}

// MessageService 消息持久化服务接口
type MessageService interface {
	List(ctx context.Context, sessionID string) ([]Message, error)
	Create(ctx context.Context, sessionID string, params CreateMessageParams) (*Message, error)
}

// CreateMessageParams 创建消息参数
type CreateMessageParams struct {
	ID               string
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

type viewToolParams struct {
	FilePath string `json:"file_path"`
}

func skillNameFromToolCall(toolName string, arguments string) string {
	if toolName != "view" {
		return ""
	}
	var params viewToolParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return ""
	}
	return skillNameFromPath(params.FilePath)
}

func skillNameFromPath(path string) string {
	if path == "" || filepath.Base(path) != skills.SkillFileName {
		return ""
	}
	name := filepath.Base(filepath.Dir(path))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
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
		skills:      skills.NewManager("."),
		rulesLoader: rules.NewLoader("."),
		running:     make(map[string]context.CancelFunc),
		sessionIDs:  make(map[string]string),
	}
	c.toolCaller = tools.NewToolCaller(toolRegistry, permService)
	c.confirmFn = c.requestUserConfirmation
	c.pendingConfs = make(map[string]chan bool)
	if sub, ok := broker.(pubsub.Subscriber[events.Event]); ok {
		c.sub = sub
		go c.listenConfirmations()
	}

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

	largeSelected := cfg.LargeModel
	if id, modelCfg, ok := c.config.ModelForScenario("chat"); ok {
		largeSelected = config.SelectedModel{Provider: modelCfg.Provider, Model: id}
	}
	smallSelected := cfg.SmallModel
	if id, modelCfg, ok := c.config.ModelForScenario("summarize"); ok {
		smallSelected = config.SelectedModel{Provider: modelCfg.Provider, Model: id}
	}

	// 1. 并行构建大小模型
	eg.Go(func() error {
		m, err := c.buildModel(egCtx, largeSelected)
		if err != nil {
			return err
		}
		large = m
		return nil
	})
	eg.Go(func() error {
		m, err := c.buildModel(egCtx, smallSelected)
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
	modelCfg, hasModelCfg := c.config.GetModelConfig(selected.Model)
	if hasModelCfg {
		if selected.Provider == "" {
			selected.Provider = modelCfg.Provider
		}
		selected.Model = modelCfg.Model
	}

	providerCfg, err := llm.FindProviderConfig(c.config.Providers, selected.Provider)
	if err != nil {
		return &Model{
			Provider: "openai",
			Name:     selected.Model,
			Config: ModelConfig{
				MaxTokens:   modelCfg.MaxTokens,
				Temperature: modelCfg.Temperature,
				TopP:        modelCfg.TopP,
				InputPer1M:  modelCfg.InputPer1M,
				OutputPer1M: modelCfg.OutputPer1M,
				APIKey:      c.config.OpenAI.Key,
				BaseURL:     llm.DefaultOpenAIBaseURL,
			},
		}, nil
	}

	apiKey := c.config.ResolveSecret(providerCfg.APIKey)
	return &Model{
		Provider: providerCfg.Type,
		Name:     selected.Model,
		Config: ModelConfig{
			MaxTokens:   modelCfg.MaxTokens,
			Temperature: modelCfg.Temperature,
			TopP:        modelCfg.TopP,
			InputPer1M:  modelCfg.InputPer1M,
			OutputPer1M: modelCfg.OutputPer1M,
			APIKey:      apiKey,
			BaseURL:     providerCfg.BaseURL,
		},
	}, nil
}

// loadSystemPrompt 加载系统提示词（使用五层 PromptBuilder）
func (c *coordinator) loadSystemPrompt(cfg config.AgentConfig, agent *sessionAgent) {
	envInfo := collectEnvInfo(".")
	contextFiles := append(
		processUserContextPath(c.config.Context.MemoryMaxBytes),
		processContextPath(envInfo.WorkingDir, c.config.Context.MemoryMaxBytes)...,
	)
	mcpInstr := getMCPInstructions(context.Background(), 3*time.Second)

	projectContent := formatContextFiles(contextFiles)
	if mcpInstr != "" {
		if projectContent != "" {
			projectContent += "\n\n" + mcpInstr
		} else {
			projectContent = mcpInstr
		}
	}

	availableSkillsXML := c.loadAvailableSkills(envInfo.WorkingDir)
	renderedRules := c.loadRenderedRules(envInfo.WorkingDir)

	builder := prompt.NewPromptBuilder().
		WithSystem("AICodeAgent", "一个运行在终端的AI编程助手"+renderedRules, ruleStrings(DefaultRules())).
		WithEnvironmentInfo(prompt.EnvironmentInfo{
			OS:         runtime.GOOS,
			Shell:      os.Getenv("SHELL"),
			WorkingDir: envInfo.WorkingDir,
			Date:       envInfo.Date,
		}).
		WithTools(convertToPromptTools(c.collectToolDescriptions())).
		WithSkills(availableSkillsXML).
		WithProjectContext(projectContent)

	systemPrompt, err := builder.Build()
	if err != nil {
		log.Printf("[Coordinator] 系统提示词构建失败: %v", err)
		return
	}

	agent.systemPrompt = systemPrompt
	smallPrompt, _ := prompt.NewPromptBuilder().
		WithSystem("AICodeAgent", "轻量级助手", []string{"保持简洁", "直接回答"}).
		WithEnvironmentInfo(prompt.EnvironmentInfo{
			OS:         runtime.GOOS,
			Shell:      os.Getenv("SHELL"),
			WorkingDir: envInfo.WorkingDir,
			Date:       envInfo.Date,
		}).Build()
	agent.smallSystemPrompt = smallPrompt
	agent.temperature = c.config.OpenAI.Temperature
	log.Println("[Coordinator] 系统提示词加载完成")
}

func (c *coordinator) loadRenderedRules(projectDir string) string {
	if c.rulesLoader == nil {
		c.rulesLoader = rules.NewLoader(projectDir)
	}
	c.rulesLoader.SetWorkingDir(projectDir)
	set, err := c.rulesLoader.LoadRules()
	if err != nil {
		log.Printf("[Coordinator] 规则加载失败: %v", err)
		return ""
	}
	rendered := rules.RenderPrompt(set)
	if strings.TrimSpace(rendered) == "" {
		return ""
	}
	return "\n\n" + rendered
}

func (c *coordinator) loadAvailableSkills(projectDir string) string {
	if c.skills == nil {
		return ""
	}

	c.skills = skills.NewManager(projectDir)
	paths := skills.DefaultPaths(projectDir)
	paths = append(paths, skills.ResolvePaths(c.config.SkillsPaths, projectDir)...)
	if len(paths) == 0 {
		c.matcher = nil
		return ""
	}
	if err := c.skills.Load(paths); err != nil {
		log.Printf("[Coordinator] 技能加载失败: %v", err)
		c.matcher = nil
		return ""
	}
	skillList := c.skills.List()
	c.matcher = NewSkillMatcher(skillList)
	return skills.ToPromptXML(skillList)
}

// ruleStrings 将 Rule 切片转换为字符串切片
func ruleStrings(rules []Rule) []string {
	result := make([]string, len(rules))
	for i, r := range rules {
		result[i] = r.Title + "：" + r.Content
	}
	return result
}

// convertToPromptTools 将 agent.Tool 转换为 prompt.ToolInfo
func convertToPromptTools(tools []Tool) []prompt.ToolInfo {
	result := make([]prompt.ToolInfo, len(tools))
	for i, t := range tools {
		result[i] = prompt.ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Params,
		}
	}
	return result
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

func (c *coordinator) SessionAgent() SessionAgent {
	return c.currentAgent
}

// UpdateModels 更新模型配置（运行时热更新）
func (c *coordinator) UpdateModels(ctx context.Context) error {
	agent, err := c.buildAgent(ctx, c.config.Agent)
	if err != nil {
		return err
	}

	if newAgent, ok := agent.(*sessionAgent); ok {
		c.largeModel.Store(newAgent.LargeModel())
		c.smallModel.Store(newAgent.SmallModel())
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

	// 2. 处理内置命令（/initialize 等）
	if result, handled := c.handleBuiltinCommand(ctx, sessionID, prompt); handled {
		return result, nil
	}

	// 3. 创建可取消上下文
	runCtx, cancel := context.WithCancel(ctx)

	// 4. 注册cancel函数
	c.mu.Lock()
	c.running[session.ID] = cancel
	c.mu.Unlock()

	// 5. 清理资源
	defer func() {
		c.mu.Lock()
		delete(c.running, session.ID)
		c.mu.Unlock()
	}()

	// 6. 确保Agent已初始化
	if c.currentAgent == nil {
		return nil, fmt.Errorf("agent not initialized yet, please wait")
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
	messages := c.buildInitialMessages(session.ID, prompt)

	// 9. ReAct循环：最多10轮迭代，每轮最多2分钟
	const (
		maxReActIterations  = 10
		perIterationTimeout = 2 * time.Minute
	)
	for i := 0; i < maxReActIterations; i++ {
		log.Printf("[Coordinator] ReAct迭代 %d/%d（消息数=%d）", i+1, maxReActIterations, len(messages))

		if stopped, err := c.handleStopCondition(runCtx, sessionID, session, c.checkContextWindow(session)); stopped != nil || err != nil {
			return stopped, err
		}

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
				skillName := skillNameFromToolCall(tc.Function.Name, tc.Function.Arguments)
				c.broker.PublishMustDeliver(runCtx, events.ToolCall{
					SessionID: session.ID,
					ToolName:  tc.Function.Name,
					Params:    json.RawMessage(tc.Function.Arguments),
					SkillName: skillName,
					Time:      time.Now(),
				})
			}

			// 转换 ToolCallDelta → ToolCall 并执行
			agentToolCalls := convertToolCallDeltas(result.ToolCalls)
			toolResults, _ := c.handleToolCallsForSession(runCtx, session.ID, agentToolCalls)

			// 发布工具结果事件（关联工具名）
			for idx, tr := range toolResults {
				toolName := ""
				skillName := ""
				if idx < len(agentToolCalls) {
					toolName = agentToolCalls[idx].Function.Name
					skillName = skillNameFromToolCall(toolName, agentToolCalls[idx].Function.Arguments)
				}
				c.broker.PublishMustDeliver(runCtx, events.ToolResult{
					SessionID: session.ID,
					ToolName:  toolName,
					SkillName: skillName,
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

// buildInitialMessages 构建初始消息列表（系统提示词 + 用户输入，尊重 Revert 回滚点）
func (c *coordinator) buildInitialMessages(sessionID string, prompt string) []llm.Message {
	var msgs []llm.Message

	if sa, ok := c.currentAgent.(*sessionAgent); ok {
		if sa.systemPrompt != "" {
			msgs = append(msgs, llm.NewSystemMessage(sa.systemPrompt))
		}
	}

	agentMsgs, _ := c.messages.List(context.Background(), sessionID)
	session, _ := c.sessions.Get(context.Background(), sessionID)

	// 如果存在 Revert 点，只加载回滚点之前的消息
	if session != nil && session.Revert != nil {
		for _, am := range agentMsgs {
			if !am.ShouldInclude() {
				continue
			}
			msgs = append(msgs, llm.Message{
				Role:             string(am.Role),
				Content:          am.Content,
				ToolCallID:       am.ToolCallID,
				ID:               am.ID,
				IsSummaryMessage: am.IsSummaryMessage,
			})
			if am.ID == session.Revert.MessageID {
				break
			}
		}
	} else {
		for _, am := range agentMsgs {
			if !am.ShouldInclude() {
				continue
			}
			msgs = append(msgs, llm.Message{
				Role:             string(am.Role),
				Content:          am.Content,
				ToolCallID:       am.ToolCallID,
				ID:               am.ID,
				IsSummaryMessage: am.IsSummaryMessage,
			})
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	c.mu.Lock()
	if _, busy := c.running[msg.SessionID]; busy {
		c.mu.Unlock()
		cancel()
		log.Printf("[Coordinator] 会话 %s 正在处理中，跳过重复消息", msg.SessionID)
		return
	}
	c.running[msg.SessionID] = cancel
	c.mu.Unlock()

	go func() {
		defer func() {
			c.publishSessionUpdate(context.Background(), msg.SessionID)
			cancel()
			c.mu.Lock()
			delete(c.running, msg.SessionID)
			c.mu.Unlock()
		}()

		result, err := c.Run(ctx, msg.SessionID, msg.Content)
		if err != nil {
			content := err.Error()
			if strings.Contains(err.Error(), "canceled") {
				content = "任务已取消"
			}
			c.broker.PublishMustDeliver(context.Background(), events.AgentThink{
				SessionID: msg.SessionID,
				Content:   content,
				IsDone:    true,
				Time:      time.Now(),
			})
			return
		}

		if result.Error != nil {
			c.broker.PublishMustDeliver(context.Background(), events.AgentThink{
				SessionID: msg.SessionID,
				Content:   result.Error.Error(),
				IsDone:    true,
				Time:      time.Now(),
			})
			return
		}

		// 内置命令需要主动向 TUI 发布结束事件
		trimmed := strings.TrimSpace(msg.Content)
		if result.Response != "" && isBuiltinCommand(trimmed) {
			c.broker.PublishMustDeliver(context.Background(), events.AgentThink{
				SessionID: msg.SessionID,
				Content:   result.Response,
				IsDone:    true,
				Time:      time.Now(),
			})
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
		ToolName:  tc.Function.Name,
		Params:    json.RawMessage(tc.Function.Arguments),
		SkillName: skillNameFromToolCall(tc.Function.Name, tc.Function.Arguments),
		Time:      time.Now(),
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
	return c.handleToolCallsForSession(ctx, "default", toolCalls)
}

func (c *coordinator) handleToolCallsForSession(ctx context.Context, sessionID string, toolCalls []ToolCall) ([]Message, error) {
	var results []Message

	for _, call := range toolCalls {
		params := json.RawMessage(call.Function.Arguments)

		// 1. 工具查找 + 权限检查
		tool, allow, needConfirm, reason := c.toolCaller.ResolveAndCheckForSession(sessionID, call.Function.Name, params)
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
				if sessionID != "" && c.permService != nil {
					c.permService.ApproveOperation(sessionID, call.Function.Name, params)
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

		// 2. 统一通过 ToolCaller 执行，确保审计和防护链路一致
		result, err := c.toolCaller.CallTool(ctx, sessionID, call.Function.Name, params)

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

// requestUserConfirmation 请求用户确认危险操作（通过 Broker 事件往返）
func (c *coordinator) requestUserConfirmation(toolName, arguments string) bool {
	if c.sub == nil {
		return false
	}

	requestID := fmt.Sprintf("perm-%d", time.Now().UnixNano())
	respCh := make(chan bool, 1)

	c.confirmMu.Lock()
	c.pendingConfs[requestID] = respCh
	c.confirmMu.Unlock()

	defer func() {
		c.confirmMu.Lock()
		delete(c.pendingConfs, requestID)
		c.confirmMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.broker.PublishMustDeliver(ctx, events.PermissionRequest{
		RequestID: requestID,
		ToolName:  toolName,
		Command:   arguments,
		Time:      time.Now(),
	})

	select {
	case allowed := <-respCh:
		return allowed
	case <-ctx.Done():
		return false
	}
}

// listenConfirmations 订阅 PermissionResponse 事件，路由到等待的确认通道
func (c *coordinator) listenConfirmations() {
	ch := c.sub.Subscribe(context.Background())
	for {
		event, ok := <-ch
		if !ok {
			return
		}
		resp, ok := event.(events.PermissionResponse)
		if !ok {
			continue
		}
		c.confirmMu.Lock()
		respCh, ok := c.pendingConfs[resp.RequestID]
		c.confirmMu.Unlock()
		if ok {
			select {
			case respCh <- resp.Allowed:
			default:
			}
		}
	}
}

// Cancel 取消指定会话
func (c *coordinator) SetSubagentRunner(runner SubagentRunner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if observable, ok := runner.(ObservableSubagentRunner); ok {
		observable.SetObserver(c)
	}
	c.subagents = runner
}

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

type contextWindowStatus struct {
	Used      int64
	Window    int64
	Threshold int64
	Warning   bool
}

func (c *coordinator) contextWindowStatus(session *Session) *contextWindowStatus {
	if session == nil || session.DisableAutoSummarize {
		return nil
	}

	model := c.largeModel.Load()
	if model == nil {
		return nil
	}

	window := c.config.Context.WindowTokens
	if window == 0 {
		window = model.Config.MaxTokens
	}
	if window == 0 {
		return nil
	}

	buffer := c.config.Context.LargeWindowBuffer
	if buffer == 0 {
		buffer = 20_000
	}
	ratio := c.config.Context.SmallWindowRatio
	if ratio == 0 {
		ratio = 0.2
	}

	used := session.PromptTokens + session.CompletionTokens
	threshold := int64(float64(window) * ratio)
	if window > 200_000 {
		threshold = buffer
	}

	return &contextWindowStatus{
		Used:      used,
		Window:    window,
		Threshold: threshold,
		Warning:   window-used <= threshold,
	}
}

// checkContextWindow 检查是否需要触发自动总结
func (c *coordinator) checkContextWindow(session *Session) *StopCondition {
	status := c.contextWindowStatus(session)
	if status == nil || !status.Warning {
		return nil
	}

	return &StopCondition{
		Reason:    "context_window_overflow",
		Used:      status.Used,
		Window:    status.Window,
		Threshold: status.Threshold,
	}
}

func (c *coordinator) handleStopCondition(ctx context.Context, sessionID string, session *Session, stop *StopCondition) (*AgentResult, error) {
	if stop == nil {
		return nil, nil
	}
	log.Printf("[Coordinator] 停止当前ReAct循环，原因=%s used=%d window=%d threshold=%d",
		stop.Reason, stop.Used, stop.Window, stop.Threshold)
	if err := c.Summarize(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("summarize after stop condition: %w", err)
	}
	return &AgentResult{
		Session:  session,
		Response: "上下文窗口接近上限，已自动生成会话摘要。请继续输入下一步任务。",
	}, nil
}

func isBuiltinCommand(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "/initialize-confirm") ||
		strings.HasPrefix(trimmed, "/initialize-reject") ||
		strings.HasPrefix(trimmed, "/initialize") ||
		strings.HasPrefix(trimmed, "/subagents") ||
		strings.HasPrefix(trimmed, "/skills") ||
		strings.HasPrefix(trimmed, "/rules") ||
		strings.HasPrefix(trimmed, "/subagent ") ||
		trimmed == "/subagent" ||
		strings.HasPrefix(trimmed, "/undo") ||
		strings.HasPrefix(trimmed, "/redo")
}

// handleBuiltinCommand 处理内置斜杠命令
// 返回 (result, handled)。handled=true 时调用方应直接返回 result
func (c *coordinator) handleBuiltinCommand(ctx context.Context, sessionID string, prompt string) (*AgentResult, bool) {
	trimmed := strings.TrimSpace(prompt)

	switch {
	case strings.HasPrefix(trimmed, "/initialize-confirm"):
		return c.confirmInit(ctx)
	case strings.HasPrefix(trimmed, "/initialize-reject"):
		return c.rejectInit()
	case strings.HasPrefix(trimmed, "/initialize"):
		return c.runInit(ctx, sessionID)
	case strings.HasPrefix(trimmed, "/subagents"):
		return c.listSubagentsCommand()
	case strings.HasPrefix(trimmed, "/skills"):
		return c.listSkillsCommand()
	case strings.HasPrefix(trimmed, "/rules"):
		return c.listRulesCommand()
	case strings.HasPrefix(trimmed, "/subagent"):
		return c.runSubagentCommand(ctx, sessionID, trimmed)
	case strings.HasPrefix(trimmed, "/undo"):
		return c.undoCommand(ctx, sessionID)
	case strings.HasPrefix(trimmed, "/redo"):
		return c.redoCommand(ctx, sessionID)
	default:
		return nil, false
	}
}

func (c *coordinator) listSubagentsCommand() (*AgentResult, bool) {
	c.mu.RLock()
	runner := c.subagents
	c.mu.RUnlock()
	if runner == nil {
		return &AgentResult{Response: "Subagent 尚未初始化。"}, true
	}
	items := runner.List()
	if len(items) == 0 {
		return &AgentResult{Response: "当前没有加载任何 Subagent。"}, true
	}
	var b strings.Builder
	b.WriteString("已加载 Subagent：\n")
	for _, item := range items {
		if item.Description == "" {
			b.WriteString(fmt.Sprintf("- `%s`\n", item.Name))
			continue
		}
		b.WriteString(fmt.Sprintf("- `%s`：%s\n", item.Name, item.Description))
	}
	return &AgentResult{Response: strings.TrimSpace(b.String())}, true
}

func (c *coordinator) listSkillsCommand() (*AgentResult, bool) {
	if c.skills == nil {
		return &AgentResult{Response: "Skills 尚未初始化。"}, true
	}
	items := c.skills.List()
	if len(items) == 0 {
		return &AgentResult{Response: "当前没有加载任何 Skill。"}, true
	}
	var b strings.Builder
	b.WriteString("已加载 Skills：\n")
	for _, item := range items {
		if item.Description == "" {
			b.WriteString(fmt.Sprintf("- `/%s`\n", item.Name))
			continue
		}
		b.WriteString(fmt.Sprintf("- `/%s`：%s\n", item.Name, item.Description))
	}
	return &AgentResult{Response: strings.TrimSpace(b.String())}, true
}

func (c *coordinator) listRulesCommand() (*AgentResult, bool) {
	projectRoot, err := os.Getwd()
	if err != nil {
		projectRoot = "."
	}
	if c.rulesLoader == nil {
		c.rulesLoader = rules.NewLoader(projectRoot)
	}
	c.rulesLoader.SetWorkingDir(projectRoot)
	set, err := c.rulesLoader.LoadRules()
	if err != nil {
		return &AgentResult{Response: fmt.Sprintf("Rules 加载失败: %v", err)}, true
	}
	var b strings.Builder
	b.WriteString("已加载 Rules：\n")
	appendRules := func(title string, count int) {
		b.WriteString(fmt.Sprintf("- %s：%d 条\n", title, count))
	}
	appendRules("constitution", len(set.Constitution))
	appendRules("workflow", len(set.Workflow))
	appendRules("coding", len(set.Coding))
	return &AgentResult{Response: strings.TrimSpace(b.String())}, true
}

func (c *coordinator) runSubagentCommand(ctx context.Context, sessionID string, input string) (*AgentResult, bool) {
	parts := strings.Fields(input)
	if len(parts) < 3 {
		return &AgentResult{Response: "用法：`/subagent <name> <task>`。可先输入 `/subagents` 查看可用 Subagent。"}, true
	}
	c.mu.RLock()
	runner := c.subagents
	c.mu.RUnlock()
	if runner == nil {
		return &AgentResult{Response: "Subagent 尚未初始化。"}, true
	}
	actualID := sessionID
	if mappedID, ok := c.sessionIDs[sessionID]; ok {
		actualID = mappedID
	}
	session, err := c.sessions.Get(ctx, actualID)
	if err != nil {
		session, err = c.sessions.Create(ctx, "New Session")
		if err != nil {
			return &AgentResult{Response: fmt.Sprintf("创建会话失败: %v", err)}, true
		}
		c.mu.Lock()
		c.sessionIDs[sessionID] = session.ID
		c.mu.Unlock()
	}
	name := parts[1]
	task := normalizeSubagentTaskPath(strings.TrimSpace(strings.TrimPrefix(input, strings.Join(parts[:2], " "))))

	c.publishSubagentRoleSwitch(ctx, session.ID, name)
	defer c.publishSubagentRoleRestore(context.Background(), session.ID, name)

	result, err := runner.Execute(ctx, name, session, task)
	if err != nil {
		errorText := fmt.Sprintf("Subagent 执行失败: %v", err)
		c.publishSubagentTaskComplete(ctx, session.ID, name, "", errorText)
		return &AgentResult{Response: errorText}, true
	}
	c.publishSubagentTaskComplete(ctx, session.ID, name, result, "")
	c.persistSubagentExchange(ctx, session.ID, input, name, result)
	return &AgentResult{Response: result}, true
}

func (c *coordinator) publishSubagentRoleSwitch(ctx context.Context, parentSessionID, name string) {
	if c.broker == nil {
		return
	}
	c.broker.PublishMustDeliver(ctx, events.SubagentRoleSwitch{
		ParentSessionID: parentSessionID,
		SubagentName:    name,
		Time:            time.Now(),
	})
}

func (c *coordinator) publishSubagentRoleRestore(ctx context.Context, parentSessionID, name string) {
	if c.broker == nil {
		return
	}
	c.broker.PublishMustDeliver(ctx, events.SubagentRoleRestore{
		ParentSessionID: parentSessionID,
		SubagentName:    name,
		Time:            time.Now(),
	})
}

func (c *coordinator) OnSubagentToolCall(ctx context.Context, parentSessionID, subagentName, callID, toolName string, params json.RawMessage) {
	if c == nil || c.broker == nil {
		return
	}
	c.broker.PublishMustDeliver(ctx, events.SubagentToolCall{
		ParentSessionID: parentSessionID,
		SubagentName:    subagentName,
		ToolCallID:      callID,
		ToolName:        toolName,
		Params:          params,
		Time:            time.Now(),
	})
}

func (c *coordinator) OnSubagentToolResult(ctx context.Context, parentSessionID, subagentName, callID, toolName, result, errorText string) {
	if c == nil || c.broker == nil {
		return
	}
	c.broker.PublishMustDeliver(ctx, events.SubagentToolResult{
		ParentSessionID: parentSessionID,
		SubagentName:    subagentName,
		ToolCallID:      callID,
		ToolName:        toolName,
		Result:          result,
		Error:           errorText,
		Time:            time.Now(),
	})
}

func (c *coordinator) publishSubagentTaskComplete(ctx context.Context, parentSessionID, name, summary, errorText string) {
	if c.broker == nil {
		return
	}
	c.broker.PublishMustDeliver(ctx, events.SubagentTaskComplete{
		ParentSessionID: parentSessionID,
		SubagentName:    name,
		Summary:         summary,
		Error:           errorText,
		Time:            time.Now(),
	})
}

func (c *coordinator) persistSubagentExchange(ctx context.Context, sessionID, input, name, result string) {
	if c.messages == nil {
		return
	}
	_, _ = c.messages.Create(ctx, sessionID, CreateMessageParams{
		ID:      generateID(),
		Role:    RoleUser,
		Content: input,
	})
	_, _ = c.messages.Create(ctx, sessionID, CreateMessageParams{
		ID:      generateID(),
		Role:    RoleAssistant,
		Content: fmt.Sprintf("Subagent `%s` 结果：\n\n%s", name, result),
	})
}

func normalizeSubagentTaskPath(task string) string {
	if task == "" {
		return task
	}
	wd, err := os.Getwd()
	if err != nil {
		return task
	}
	base := filepath.Base(wd)
	parts := strings.Fields(task)
	changed := false
	for i, part := range parts {
		if !strings.Contains(part, string(filepath.Separator)) || filepath.IsAbs(part) {
			continue
		}
		clean := filepath.Clean(part)
		segments := strings.Split(clean, string(filepath.Separator))
		for idx, segment := range segments {
			if segment != base || idx == len(segments)-1 {
				continue
			}
			candidate := filepath.Join(segments[idx+1:]...)
			if _, err := os.Stat(filepath.Join(wd, candidate)); err == nil {
				parts[i] = candidate
				changed = true
			}
			break
		}
	}
	if !changed {
		return task
	}
	return strings.Join(parts, " ")
}

// runInit 执行 /initialize 命令，生成 AGENTS.md 内容
func (c *coordinator) runInit(ctx context.Context, sessionID string) (*AgentResult, bool) {
	projectRoot, err := os.Getwd()
	if err != nil {
		projectRoot = "."
	}

	result, err := c.handleInitialize(ctx, projectRoot)
	if err != nil {
		return &AgentResult{
			Response: fmt.Sprintf("初始化失败: %v", err),
		}, true
	}

	c.mu.Lock()
	c.pendingInit = result
	c.mu.Unlock()

	var action string
	if result.IsUpdate {
		action = "更新"
	} else {
		action = "生成"
	}

	targetPath := filepath.Join(result.ProjectRoot, "AGENTS.md")
	return &AgentResult{
		Response: fmt.Sprintf("已%s AGENTS.md 模板（扫描到 %d 个项目文件）。\n\n目标路径：`%s`\n\n```markdown\n%s\n```\n\n"+
			"当前仅为预览，尚未写入文件。请输入 /initialize-confirm 写入文件，或 /initialize-reject 放弃。", action, result.FilesFound, targetPath, result.Content),
	}, true
}

// confirmInit 确认写入 AGENTS.md
func (c *coordinator) confirmInit(ctx context.Context) (*AgentResult, bool) {
	c.mu.Lock()
	result := c.pendingInit
	c.pendingInit = nil
	c.mu.Unlock()

	if result == nil {
		return &AgentResult{
			Response: "没有待确认的 AGENTS.md 内容。请先执行 /initialize。",
		}, true
	}

	path := filepath.Join(result.ProjectRoot, "AGENTS.md")
	if err := os.WriteFile(path, []byte(result.Content), 0o644); err != nil {
		return &AgentResult{
			Response: fmt.Sprintf("写入 AGENTS.md 失败: %v", err),
		}, true
	}

	// 写入成功后，更新系统提示词（如果 agent 已初始化）
	c.updateSystemPromptWithProjectMemory(result.ProjectRoot)

	return &AgentResult{
		Response: fmt.Sprintf("AGENTS.md 已写入 %s。系统提示词已更新，下次对话将自动加载。", path),
	}, true
}

// rejectInit 放弃 AGENTS.md 内容
func (c *coordinator) rejectInit() (*AgentResult, bool) {
	c.mu.Lock()
	c.pendingInit = nil
	c.mu.Unlock()

	return &AgentResult{
		Response: "已放弃 AGENTS.md 生成。",
	}, true
}

// undoCommand 处理 /undo 命令
func (c *coordinator) undoCommand(ctx context.Context, sessionID string) (*AgentResult, bool) {
	if err := c.Undo(ctx, sessionID); err != nil {
		return &AgentResult{Response: fmt.Sprintf("Undo 失败: %v", err)}, true
	}
	return &AgentResult{Response: "已回滚到上一条用户消息。输入 /redo 可恢复。"}, true
}

// redoCommand 处理 /redo 命令
func (c *coordinator) redoCommand(ctx context.Context, sessionID string) (*AgentResult, bool) {
	if err := c.Redo(ctx, sessionID); err != nil {
		return &AgentResult{Response: fmt.Sprintf("Redo 失败: %v", err)}, true
	}
	return &AgentResult{Response: "已恢复完整对话历史。"}, true
}

// Undo 回滚到上一条用户消息（不删除消息，仅设置 Revert 隐藏点）
func (c *coordinator) Undo(ctx context.Context, sessionID string) error {
	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	messages, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	// 从后向前找最后一条用户消息
	var revertMsgID string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser && !messages[i].IsSummaryMessage {
			revertMsgID = messages[i].ID
			break
		}
	}

	if revertMsgID == "" {
		return fmt.Errorf("no user message found to undo")
	}

	session.Revert = &RevertPoint{
		MessageID: revertMsgID,
		Timestamp: time.Now(),
	}

	if err := c.sessions.Save(ctx, session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	c.publishSessionUpdate(ctx, sessionID)
	return nil
}

// Redo 清除回滚点，恢复完整可见历史
func (c *coordinator) Redo(ctx context.Context, sessionID string) error {
	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if session.Revert == nil {
		return fmt.Errorf("nothing to redo")
	}

	session.Revert = nil
	if err := c.sessions.Save(ctx, session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	c.publishSessionUpdate(ctx, sessionID)
	return nil
}

// GetVisibleMessages 获取当前可见的消息（受 Revert 点影响）
func (c *coordinator) GetVisibleMessages(ctx context.Context, sessionID string) ([]Message, error) {
	messages, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	if session.Revert == nil {
		return messages, nil
	}

	// 找到 Revert 点的位置，只返回之前（含）的消息
	var visible []Message
	for _, msg := range messages {
		visible = append(visible, msg)
		if msg.ID == session.Revert.MessageID {
			break
		}
	}
	return visible, nil
}

// publishSessionUpdate 发布会话更新事件
func (c *coordinator) publishSessionUpdate(ctx context.Context, sessionID string) {
	update := events.SessionUpdate{SessionID: sessionID, Time: time.Now()}
	session, err := c.sessions.Get(ctx, sessionID)
	if err == nil {
		update.PromptTokens = session.PromptTokens
		update.CompletionTokens = session.CompletionTokens
		if status := c.contextWindowStatus(session); status != nil {
			update.WindowTokens = status.Window
			update.ThresholdTokens = status.Threshold
			update.Warning = status.Warning
		}
	}
	c.broker.Publish(update)
}

// updateSystemPromptWithProjectMemory 用项目记忆文件更新系统提示词
func (c *coordinator) updateSystemPromptWithProjectMemory(projectRoot string) {
	sa, ok := c.currentAgent.(*sessionAgent)
	if !ok || sa == nil {
		return
	}

	contextFiles := processContextPath(projectRoot, defaultMemoryMaxBytes)
	if len(contextFiles) == 0 {
		return
	}

	basePrompt := sa.systemPrompt
	mcpInstructions := getMCPInstructions(context.Background(), 3*time.Second)
	sa.systemPrompt = buildSystemPrompt(basePrompt, contextFiles, mcpInstructions)
}

// Summarize 生成会话摘要（使用小模型降低90%成本）
func (c *coordinator) Summarize(ctx context.Context, sessionID string) error {
	session, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session failed: %w", err)
	}

	summaryModel := c.smallModel.Load()
	if summaryModel == nil {
		return fmt.Errorf("summary model not available")
	}

	messages, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list messages failed: %w", err)
	}

	provider, err := BuildProvider(summaryModel)
	if err != nil {
		return fmt.Errorf("build summary provider: %w", err)
	}
	defer provider.Close()

	streamCall := llm.AgentStreamCall{
		Prompt:      buildSummaryPrompt(messages),
		Temperature: summaryModel.Config.Temperature,
		TopP:        summaryModel.Config.TopP,
		MaxTokens:   summaryModel.Config.MaxTokens,
	}

	response, err := provider.Chat(ctx, streamCall)
	if err != nil {
		return fmt.Errorf("summarize failed: %w", err)
	}
	if err := validateSummaryContent(response); err != nil {
		return fmt.Errorf("invalid summary: %w", err)
	}

	summaryMessageID := generateID()
	_, err = c.messages.Create(ctx, sessionID, CreateMessageParams{
		ID:               summaryMessageID,
		Role:             RoleAssistant,
		Content:          response,
		IsSummaryMessage: true,
	})
	if err != nil {
		return fmt.Errorf("save summary message: %w", err)
	}

	session.SummaryMessageID = summaryMessageID
	session.PromptTokens = 0
	session.CompletionTokens = 0
	return c.sessions.Save(ctx, session)
}
