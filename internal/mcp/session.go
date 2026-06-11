package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// ClientInfo MCP 客户端标识信息
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo MCP 服务器标识信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams Initialize 请求参数
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities 客户端能力声明
type ClientCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability 工具相关能力
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// InitializeResult Initialize 响应结果
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities 服务器能力声明
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// ListToolsResult tools/list 响应结果
type ListToolsResult struct {
	Tools []*Tool `json:"tools"`
}

// ContentItem 工具调用结果内容项
type ContentItem struct {
	Type     string `json:"type"` // "text" / "image" / "resource"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

// CallToolResult tools/call 响应结果
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ClientSession MCP 客户端会话，管理与一个 MCP Server 的完整连接生命周期
type ClientSession struct {
	protocol  *ProtocolHandler
	transport Transport
	Info      ServerInfo
	tools     []*Tool
	cancel    context.CancelFunc // 停止 receive loop
}

// NewClientSession 创建并初始化 MCP 客户端会话
func NewClientSession(ctx context.Context, transport Transport, clientInfo ClientInfo) (*ClientSession, error) {
	protocol := NewProtocolHandler(transport)
	// receive loop 使用独立的后台 context，不受初始化超时影响
	loopCtx, loopCancel := context.WithCancel(context.Background())
	session := &ClientSession{protocol: protocol, transport: transport, cancel: loopCancel}

	protocol.StartReceiveLoop(loopCtx)

	// 1. Initialize 握手
	initParams := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: ClientCapabilities{
			Tools: &ToolsCapability{ListChanged: true},
		},
		ClientInfo: clientInfo,
	}

	result, err := protocol.Call(ctx, "initialize", initParams)
	if err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		transport.Close()
		return nil, fmt.Errorf("parse initialize result: %w", err)
	}
	session.Info = initResult.ServerInfo

	// 2. 发送 Initialized 通知
	if err := protocol.SendNotification(ctx, "notifications/initialized", nil); err != nil {
		transport.Close()
		return nil, fmt.Errorf("send initialized notification: %w", err)
	}

	log.Printf("[MCP Session] 连接成功: server=%s v%s", initResult.ServerInfo.Name, initResult.ServerInfo.Version)
	return session, nil
}

// ListTools 获取可用工具列表
func (s *ClientSession) ListTools(ctx context.Context) ([]*Tool, error) {
	result, err := s.protocol.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	var listResult ListToolsResult
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("parse tools list: %w", err)
	}

	s.tools = listResult.Tools
	return listResult.Tools, nil
}

// CallTool 调用指定工具
func (s *ClientSession) CallTool(ctx context.Context, toolName string, arguments map[string]any) (*CallToolResult, error) {
	params := map[string]any{
		"name":      toolName,
		"arguments": arguments,
	}

	result, err := s.protocol.Call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("call tool %s: %w", toolName, err)
	}

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("parse tool call result: %w", err)
	}
	return &callResult, nil
}

// Ping 健康检查
func (s *ClientSession) Ping(ctx context.Context) error {
	_, err := s.protocol.Call(ctx, "ping", nil)
	return err
}

// Tools 返回缓存的工具列表
func (s *ClientSession) Tools() []*Tool {
	return s.tools
}

// Close 关闭会话
func (s *ClientSession) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.transport.Close()
}

// createSession 创建 MCP 会话（封装 Transport 创建和 Initialize 流程）
func createSession(ctx context.Context, _ string, cfg MCPConfigAdapter) (*ClientSession, error) {
	transport, err := createTransport(ctx, cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15
	}
	sessionCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	clientInfo := ClientInfo{Name: "AICodeAgent", Version: "1.0.0"}
	session, err := NewClientSession(sessionCtx, transport, clientInfo)
	if err != nil {
		transport.Close()
		return nil, err
	}
	return session, nil
}

func createTransport(ctx context.Context, cfg MCPConfigAdapter) (Transport, error) {
	switch MCPTransportType(cfg.Type) {
	case MCPTransportStdio:
		return createStdioTransport(ctx, cfg)
	case MCPTransportHTTP:
		return createHTTPTransport(ctx, cfg)
	case MCPTransportSSE:
		return createSSETransport(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.Type)
	}
}

// MCPConfigAdapter Transport 创建所需的配置适配器
type MCPConfigAdapter struct {
	Type          string
	Command       string
	Args          []string
	URL           string
	Timeout       int
	Disabled      bool
	DisabledTools []string
	Env           map[string]string
	Headers       map[string]string
}

func createStdioTransport(ctx context.Context, cfg MCPConfigAdapter) (Transport, error) {
	cmd := execCmd(ctx, cfg.Command, cfg.Args...)
	// 继承父进程环境变量，再追加自定义变量
	cmd.Env = os.Environ()
	// 解析 env value 中的环境变量引用
	resolvedEnv := make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		resolvedEnv[k] = resolveEnvVars(v)
	}
	cmd.Env = append(cmd.Env, mapToEnvPairs(resolvedEnv)...)
	return NewCommandTransport(cmd)
}

func createHTTPTransport(_ context.Context, cfg MCPConfigAdapter) (Transport, error) {
	resolvedHeaders := resolveHeaders(cfg.Headers)
	return NewHTTPTransport(cfg.URL, resolvedHeaders), nil
}

func createSSETransport(_ context.Context, cfg MCPConfigAdapter) (Transport, error) {
	resolvedHeaders := resolveHeaders(cfg.Headers)
	return NewSSETransport(cfg.URL, resolvedHeaders), nil
}

func resolveHeaders(headers map[string]string) map[string]string {
	resolved := make(map[string]string, len(headers))
	for k, v := range headers {
		resolved[k] = resolveEnvVars(v)
	}
	return resolved
}
