package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"AICodeAgent/internal/agent"
	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/llm"
)

type Executor struct {
	runner   agent.SessionAgent
	tools    *tools.Registry
	registry *SubagentRegistry
	coord    *SubagentCoordinator
	observer agent.SubagentExecutionObserver
}

type ExecuteRequest struct {
	ParentSession *agent.Session
	ParentID      string
	SubagentName  string
	Input         string
}

type ExecuteResult struct {
	SubagentName string
	SessionID    string
	Summary      string
}

func NewExecutor(runner agent.SessionAgent, registry *SubagentRegistry, coord *SubagentCoordinator, toolRegistry *tools.Registry) *Executor {
	return &Executor{runner: runner, registry: registry, coord: coord, tools: toolRegistry}
}

func (e *Executor) SetObserver(observer agent.SubagentExecutionObserver) {
	if e == nil {
		return
	}
	e.observer = observer
}

func (e *Executor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error) {
	if e == nil || e.runner == nil || e.coord == nil {
		return nil, fmt.Errorf("subagent executor not configured")
	}
	parentID := req.ParentID
	if parentID == "" && req.ParentSession != nil {
		parentID = req.ParentSession.ID
	}
	subCtx, err := e.coord.SwitchRole(parentID, req.SubagentName)
	if err != nil {
		return nil, err
	}
	completed := false
	defer func() {
		if completed {
			e.coord.MarkDone(subCtx.SessionID)
		} else {
			e.coord.MarkFailed(subCtx.SessionID)
		}
		e.coord.RestoreMainAgent()
	}()

	messages := subCtx.BuildMessages(req.Input)
	llmTools := e.allowedTools(subCtx.Tools)
	var summary string
	const maxIterations = 5

	for i := 0; i < maxIterations; i++ {
		result, err := e.runner.Run(ctx, agent.SessionAgentCall{
			Prompt:   "",
			Session:  req.ParentSession,
			Tools:    llmTools,
			Messages: messages,
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("subagent returned nil result")
		}

		summary, err = collectSubagentSummary(result)
		if err != nil {
			return nil, err
		}
		if len(result.ToolCalls) == 0 {
			break
		}

		messages = append(messages, buildSubagentToolCallMessage(result.ToolCalls, summary))
		toolMessages := e.executeToolCalls(ctx, parentID, req.SubagentName, result.ToolCalls)
		messages = append(messages, toolMessages...)
		summary = fallbackToolSummary(toolMessages)
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, fmt.Errorf("subagent returned empty response")
	}

	subCtx.AddMessage("user", req.Input)
	subCtx.AddMessage("assistant", summary)
	completed = true
	return &ExecuteResult{SubagentName: req.SubagentName, SessionID: subCtx.SessionID, Summary: summary}, nil
}

func collectSubagentSummary(result *agent.AgentResult) (string, error) {
	summary := strings.TrimSpace(result.Response)
	if result.Stream == nil {
		return summary, nil
	}

	var response strings.Builder
	for chunk := range result.Stream {
		if chunk.Error != nil {
			return "", chunk.Error
		}
		if chunk.Done {
			break
		}
		response.WriteString(chunk.Content)
	}
	return strings.TrimSpace(response.String()), nil
}

func (e *Executor) allowedTools(names []string) []any {
	if e.tools == nil || len(names) == 0 {
		return nil
	}
	defs := make([]any, 0, len(names))
	for _, name := range names {
		name = normalizeToolName(name)
		tool, ok := e.tools.Get(name)
		if !ok {
			continue
		}
		defs = append(defs, openAIToolDefinition{
			Type: "function",
			Function: tools.FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}
	return defs
}

func (e *Executor) executeToolCalls(ctx context.Context, parentID, subagentName string, calls []llm.ToolCallDelta) []llm.Message {
	messages := make([]llm.Message, 0, len(calls))
	for _, call := range calls {
		name := normalizeToolName(call.Function.Name)
		params := json.RawMessage(call.Function.Arguments)
		if e.observer != nil {
			e.observer.OnSubagentToolCall(ctx, parentID, subagentName, call.ID, name, params)
		}
		content := e.executeTool(ctx, name, params)
		errorText := ""
		if strings.HasPrefix(content, "错误") || strings.HasPrefix(content, "执行失败") {
			errorText = content
		}
		if e.observer != nil {
			e.observer.OnSubagentToolResult(ctx, parentID, subagentName, call.ID, name, content, errorText)
		}
		messages = append(messages, llm.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    content,
		})
	}
	return messages
}

func (e *Executor) executeTool(ctx context.Context, name string, params json.RawMessage) string {
	name = normalizeToolName(name)
	if e.tools == nil {
		return "错误：Subagent 工具注册表未初始化"
	}
	tool, ok := e.tools.Get(name)
	if !ok {
		return fmt.Sprintf("错误：未知工具 '%s'", name)
	}
	result, err := tool.Execute(ctx, params)
	if err != nil {
		return fmt.Sprintf("执行失败: %v", err)
	}
	if result.Success {
		return result.Output
	}
	return fmt.Sprintf("错误: %s", result.Error)
}

func buildSubagentToolCallMessage(calls []llm.ToolCallDelta, content string) llm.Message {
	defs := make([]llm.ToolCallDef, len(calls))
	for i, call := range calls {
		defs[i] = llm.ToolCallDef{ID: call.ID, Type: call.Type}
		defs[i].Function.Name = call.Function.Name
		defs[i].Function.Arguments = call.Function.Arguments
	}
	return llm.Message{Role: "assistant", Content: content, ToolCalls: defs}
}

func fallbackToolSummary(messages []llm.Message) string {
	if len(messages) == 0 {
		return ""
	}
	content := strings.TrimSpace(messages[len(messages)-1].Content)
	if content == "" {
		return ""
	}
	return "Subagent 已读取工具结果，但未生成最终总结。最后一次工具结果：\n\n" + content
}

func normalizeToolName(name string) string {
	switch name {
	case "read_file":
		return "view"
	case "write_file":
		return "write"
	default:
		return name
	}
}

type openAIToolDefinition struct {
	Type     string                   `json:"type"`
	Function tools.FunctionDefinition `json:"function"`
}
