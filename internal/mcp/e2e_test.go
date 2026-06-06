package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// pairedTransport 请求-响应对应的 mock transport
// 只对带 ID 的请求返回响应，通知不计入响应计数
type pairedTransport struct {
	responses    [][]byte
	respIdx      int
	requestCount int
	sent         [][]byte
	closed       bool
	mu           sync.Mutex
	cond         *sync.Cond
}

func newPairedTransport(responses [][]byte) *pairedTransport {
	p := &pairedTransport{
		responses: responses,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *pairedTransport) Send(_ context.Context, msg []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return context.Canceled
	}
	p.sent = append(p.sent, msg)

	// 检查是否为请求（带 ID）而非通知
	var check JSONRPCMessage
	if json.Unmarshal(msg, &check) == nil && check.ID != nil {
		p.requestCount++
		p.cond.Signal()
	}
	return nil
}

func (p *pairedTransport) Receive(ctx context.Context) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 等待有请求到来，且还有对应响应未消费
	for p.requestCount <= p.respIdx && !p.closed {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			p.mu.Lock()
		}
	}

	if p.respIdx >= len(p.responses) {
		return nil, context.DeadlineExceeded
	}

	data := p.responses[p.respIdx]
	p.respIdx++
	return data, nil
}

func (p *pairedTransport) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.cond.Broadcast()
	return nil
}

// TestE2EFullFlow 端到端测试：完整 MCP 客户端流程
func TestE2EFullFlow(t *testing.T) {
	resetGlobalState()

	script := [][]byte{
		buildInitResponse(1),
		buildListToolsResponse(2, []*Tool{
			{Name: "get_weather", Description: "Get weather for a city", InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
			}},
		}),
		buildCallToolResponse(3, `{"temperature":25,"city":"Beijing","condition":"Sunny"}`),
	}

	transport := newPairedTransport(script)
	clientInfo := ClientInfo{Name: "AICodeAgent", Version: "1.0.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Phase 1: 创建会话
	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("Phase 1 - Initialize failed: %v", err)
	}
	defer session.Close()

	if session.Info.Name != "test-server" {
		t.Fatalf("expected server name 'test-server', got '%s'", session.Info.Name)
	}
	t.Logf("Phase 1 ✓ Initialize complete: server=%s v%s", session.Info.Name, session.Info.Version)

	// Phase 2: 工具发现
	tools, err := session.ListTools(ctx)
	if err != nil {
		t.Fatalf("Phase 2 - ListTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "get_weather" {
		t.Fatalf("expected tool 'get_weather', got '%s'", tools[0].Name)
	}
	t.Logf("Phase 2 ✓ Tool discovery: found %d tools", len(tools))

	// Phase 3: 工具调用
	result, err := session.CallTool(ctx, "get_weather", map[string]any{"city": "Beijing"})
	if err != nil {
		t.Fatalf("Phase 3 - CallTool failed: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text == "" {
		t.Fatal("Phase 3 - expected non-empty result content")
	}
	t.Logf("Phase 3 ✓ Tool call result: %s", result.Content[0].Text)

	// Phase 4: MCPTool 包装器
	mcpTool := NewMCPTool("weather", tools[0], session)
	if mcpTool.Name() != "mcp_weather_get_weather" {
		t.Fatalf("expected name 'mcp_weather_get_weather', got '%s'", mcpTool.Name())
	}
	t.Logf("Phase 4 ✓ MCPTool adapter: name=%s", mcpTool.Name())
}

// TestE2EErrorHandling 端到端测试：错误处理路径
func TestE2EErrorHandling(t *testing.T) {
	resetGlobalState()

	// 模拟 Server 返回错误
	initResp := buildInitResponse(1)

	errResp := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
	}
	errData, _ := json.Marshal(errResp)

	script := [][]byte{initResp, errData}
	transport := newPairedTransport(script)
	clientInfo := ClientInfo{Name: "test", Version: "1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer session.Close()

	// tools/list 应该收到 RPC 错误
	_, err = session.ListTools(ctx)
	if err == nil {
		t.Fatal("expected error from ListTools when server returns RPC error")
	}
	t.Logf("Error handling ✓ RPC error: %v", err)
}

// TestE2EConfigFlow 端到端测试：配置驱动的初始化流程
func TestE2EConfigFlow(t *testing.T) {
	resetGlobalState()

	cfgs := map[string]MCPConfigAdapter{
		"github": {
			Type:     "stdio",
			Command:  "echo",
			Args:     []string{"test"},
			Timeout:  1,
			Disabled: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	Initialize(ctx, cfgs)

	state, ok := GetState("github")
	if !ok {
		t.Fatal("state not found for github")
	}
	if state.State != StateDisabled {
		t.Fatalf("expected StateDisabled, got '%s'", state.State)
	}
	t.Logf("Config flow ✓ disabled server state: %s", state.State)

	// 验证 ListSessions 在 non-started 时不包含 disabled server
	sessions := ListSessions()
	if _, exists := sessions["github"]; exists {
		t.Fatal("disabled server should not have a session")
	}

	// 验证 Close 不会 panic
	Close()
	t.Log("Config flow ✓ graceful close without panic")
}

// TestE2EProtocolVersion 验证协议版本协商
func TestE2EProtocolVersion(t *testing.T) {
	script := [][]byte{buildInitResponse(1)}
	transport := newPairedTransport(script)
	clientInfo := ClientInfo{Name: "test", Version: "1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer session.Close()

	// 验证 Initialize 请求中包含正确的协议版本
	var initReq JSONRPCMessage
	json.Unmarshal(transport.sent[0], &initReq)

	var params InitializeParams
	json.Unmarshal(initReq.Params, &params)

	if params.ProtocolVersion != "2024-11-05" {
		t.Fatalf("expected protocol version '2024-11-05', got '%s'", params.ProtocolVersion)
	}
	t.Logf("Protocol version ✓ %s", params.ProtocolVersion)
}
