package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"AICodeAgent/internal/agent/tools"
)

// MCPTool 将 MCP 工具包装为 AICodeAgent 内部 tools.Tool 接口
type MCPTool struct {
	mcpName string
	tool    *Tool
	client  *ClientSession
}

// NewMCPTool 创建 MCP 工具包装器
func NewMCPTool(mcpName string, tool *Tool, client *ClientSession) *MCPTool {
	return &MCPTool{
		mcpName: mcpName,
		tool:    tool,
		client:  client,
	}
}

// Name 返回命名空间化工具名：mcp_{server}_{tool}
func (m *MCPTool) Name() string {
	return fmt.Sprintf("mcp_%s_%s", m.mcpName, m.tool.Name)
}

// Description 返回工具描述
func (m *MCPTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", m.mcpName, m.tool.Description)
}

// Parameters 返回工具的 JSON Schema 参数定义
func (m *MCPTool) Parameters() json.RawMessage {
	if m.tool.InputSchema == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	data, _ := json.Marshal(m.tool.InputSchema)
	return data
}

// Execute 执行 MCP 工具调用
func (m *MCPTool) Execute(ctx context.Context, params json.RawMessage) (tools.Result, error) {
	// 1. 解析参数
	var args map[string]any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &args); err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("参数解析失败: %v", err)}, nil
		}
	}

	// 2. 调用 MCP Server 工具
	result, err := m.client.CallTool(ctx, m.tool.Name, args)
	if err != nil {
		log.Printf("[MCPTool] 工具调用失败: %s/%s: %v", m.mcpName, m.tool.Name, err)
		return tools.Result{Success: false, Error: fmt.Sprintf("MCP工具调用失败: %v", err)}, nil
	}

	// 3. 处理不同类型的响应内容
	var output strings.Builder
	for _, item := range result.Content {
		switch item.Type {
		case "text":
			output.WriteString(item.Text)
		default:
			output.WriteString(item.Text) // fallback to text
		}
	}

	if result.IsError {
		return tools.Result{Success: false, Error: output.String()}, nil
	}

	return tools.Result{Success: true, Output: output.String()}, nil
}

// RunTool 调用指定 MCP 服务器的工具（包级函数，供外部直接调用）
func RunTool(ctx context.Context, mcpName, toolName string, input string) (*CallToolResult, error) {
	// 1. 获取或重建会话
	sess, err := getOrRenewClient(ctx, mcpName)
	if err != nil {
		return nil, fmt.Errorf("mcp session unavailable: %w", err)
	}

	// 2. 解析参数
	var args map[string]any
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			return nil, fmt.Errorf("parse tool params: %w", err)
		}
	}

	// 3. 调用工具
	return sess.CallTool(ctx, toolName, args)
}

// GetMCPTools 获取指定 MCP 服务器的所有工具，包装为内部工具接口
func GetMCPTools(mcpName string, rawTools []*Tool, client *ClientSession) []tools.Tool {
	result := make([]tools.Tool, 0, len(rawTools))
	for _, t := range rawTools {
		result = append(result, NewMCPTool(mcpName, t, client))
	}
	return result
}
