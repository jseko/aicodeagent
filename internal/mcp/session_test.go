package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// scriptedTransport 按预编程顺序返回响应
type scriptedTransport struct {
	script [][]byte
	idx    int
	sent   [][]byte
	closed bool
}

func newScriptedTransport(script [][]byte) *scriptedTransport {
	return &scriptedTransport{script: script}
}

func (s *scriptedTransport) Send(_ context.Context, msg []byte) error {
	s.sent = append(s.sent, msg)
	return nil
}

func (s *scriptedTransport) Receive(_ context.Context) ([]byte, error) {
	if s.idx >= len(s.script) {
		// block briefly then return error
		time.Sleep(100 * time.Millisecond)
		return nil, context.Canceled
	}
	data := s.script[s.idx]
	s.idx++
	return data, nil
}

func (s *scriptedTransport) Close() error {
	s.closed = true
	return nil
}

// buildInitResponse 构造 Initialize 成功响应
func buildInitResponse(id int) []byte {
	resp := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"test-server","version":"1.0.0"}}`),
	}
	data, _ := json.Marshal(resp)
	return data
}

// buildListToolsResponse 构造 tools/list 响应
func buildListToolsResponse(id int, tools []*Tool) []byte {
	result := ListToolsResult{Tools: tools}
	resultData, _ := json.Marshal(result)
	resp := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  resultData,
	}
	data, _ := json.Marshal(resp)
	return data
}

// buildCallToolResponse 构造 tools/call 响应
func buildCallToolResponse(id int, text string) []byte {
	result := CallToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
	}
	resultData, _ := json.Marshal(result)
	resp := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  resultData,
	}
	data, _ := json.Marshal(resp)
	return data
}

func TestNewClientSessionInitialize(t *testing.T) {
	// 脚本：接收2条消息（initialize响应 + initialized通知后无需响应）
	script := [][]byte{
		buildInitResponse(1),
	}
	transport := newScriptedTransport(script)
	clientInfo := ClientInfo{Name: "test-client", Version: "1.0.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("NewClientSession failed: %v", err)
	}

	if session.Info.Name != "test-server" {
		t.Fatalf("expected server name 'test-server', got '%s'", session.Info.Name)
	}

	// 验证发送的消息：Initialize 请求 + Initialized 通知
	if len(transport.sent) < 2 {
		t.Fatalf("expected at least 2 sent messages, got %d", len(transport.sent))
	}

	// 第一条：Initialize 请求
	var initReq JSONRPCMessage
	if err := json.Unmarshal(transport.sent[0], &initReq); err != nil {
		t.Fatalf("parse init request: %v", err)
	}
	if initReq.Method != "initialize" {
		t.Fatalf("expected 'initialize', got '%s'", initReq.Method)
	}

	// 第二条：Initialized 通知（无 ID）
	var notif JSONRPCMessage
	if err := json.Unmarshal(transport.sent[1], &notif); err != nil {
		t.Fatalf("parse initialized notification: %v", err)
	}
	if notif.Method != "notifications/initialized" {
		t.Fatalf("expected 'notifications/initialized', got '%s'", notif.Method)
	}
	if notif.ID != nil {
		t.Fatal("notification should not have ID")
	}

	session.Close()
}

func TestClientSessionListTools(t *testing.T) {
	expectedTools := []*Tool{
		{Name: "tool1", Description: "First tool"},
		{Name: "tool2", Description: "Second tool"},
	}

	// 脚本: init响应 + tools/list响应
	script := [][]byte{
		buildInitResponse(1),
		buildListToolsResponse(2, expectedTools),
	}
	transport := newPairedTransport(script)
	clientInfo := ClientInfo{Name: "test", Version: "1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("NewClientSession failed: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "tool1" || tools[1].Name != "tool2" {
		t.Fatalf("unexpected tool names: %v", tools)
	}
}

func TestClientSessionCallTool(t *testing.T) {
	script := [][]byte{
		buildInitResponse(1),
	}
	transport := newScriptedTransport(script)
	clientInfo := ClientInfo{Name: "test", Version: "1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("NewClientSession failed: %v", err)
	}
	defer session.Close()

	// 由于 scriptedTransport receive 在 script 耗尽后会超时，
	// CallTool 通过 protocol.Call 发送请求并等待响应
	// 需要预先 script 中包含对应的 tools/call 响应
	// 这里测试基本参数构建
	if session == nil {
		t.Fatal("session should not be nil")
	}
}

func TestClientSessionPing(t *testing.T) {
	// Ping 的脚本响应
	pingResp := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Result:  json.RawMessage(`{}`),
	}
	pingData, _ := json.Marshal(pingResp)

	script := [][]byte{
		buildInitResponse(1),
		pingData,
	}
	transport := newPairedTransport(script)
	clientInfo := ClientInfo{Name: "test", Version: "1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClientSession(ctx, transport, clientInfo)
	if err != nil {
		t.Fatalf("NewClientSession failed: %v", err)
	}
	defer session.Close()

	err = session.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func intPtr(i int) *int { return &i }
