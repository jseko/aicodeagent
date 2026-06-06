package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// channelMockTransport uses channels for bi-directional communication in tests
type channelMockTransport struct {
	sendCh    chan []byte
	recvCh    chan []byte
	closed    bool
	mu        sync.Mutex
}

func newChannelMockTransport() *channelMockTransport {
	return &channelMockTransport{
		sendCh: make(chan []byte, 50),
		recvCh: make(chan []byte, 50),
	}
}

func (m *channelMockTransport) Send(_ context.Context, msg []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return context.Canceled
	}
	m.sendCh <- msg
	return nil
}

func (m *channelMockTransport) Receive(_ context.Context) ([]byte, error) {
	msg, ok := <-m.recvCh
	if !ok {
		return nil, context.Canceled
	}
	return msg, nil
}

func (m *channelMockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.recvCh)
	}
	return nil
}

func (m *channelMockTransport) pushResponse(data []byte) {
	m.recvCh <- data
}

func (m *channelMockTransport) lastSent() []byte {
	select {
	case msg := <-m.sendCh:
		return msg
	default:
		return nil
	}
}

func TestProtocolHandlerCallResponse(t *testing.T) {
	transport := newChannelMockTransport()
	handler := NewProtocolHandler(transport)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler.StartReceiveLoop(ctx)

	// 推送模拟响应（在 goroutine 中，因为 Call 会阻塞）
	go func() {
		time.Sleep(10 * time.Millisecond)
		// 读取发送的请求以获取 ID
		reqData := transport.lastSent()
		var req JSONRPCMessage
		json.Unmarshal(reqData, &req)

		// 构造匹配的响应
		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"serverInfo":{"name":"test","version":"1.0"}}`),
		}
		respData, _ := json.Marshal(resp)
		transport.pushResponse(respData)
	}()

	result, err := handler.Call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
	})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if initResult.ServerInfo.Name != "test" {
		t.Fatalf("expected server name 'test', got '%s'", initResult.ServerInfo.Name)
	}
}

func TestProtocolHandlerCallTimeout(t *testing.T) {
	transport := newChannelMockTransport()
	handler := NewProtocolHandler(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	handler.StartReceiveLoop(ctx)

	// 不推送任何响应，应该超时
	_, err := handler.Call(ctx, "ping", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestProtocolHandlerSendNotification(t *testing.T) {
	transport := newChannelMockTransport()
	handler := NewProtocolHandler(transport)
	ctx := context.Background()

	err := handler.SendNotification(ctx, "notifications/initialized", nil)
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}

	sent := transport.lastSent()
	var msg JSONRPCMessage
	if err := json.Unmarshal(sent, &msg); err != nil {
		t.Fatalf("parse sent notification: %v", err)
	}
	if msg.ID != nil {
		t.Fatal("notification should not have an ID")
	}
	if msg.Method != "notifications/initialized" {
		t.Fatalf("expected method 'notifications/initialized', got '%s'", msg.Method)
	}
}

func TestProtocolHandlerConcurrentCalls(t *testing.T) {
	transport := newChannelMockTransport()
	handler := NewProtocolHandler(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handler.StartReceiveLoop(ctx)

	// 自动响应 goroutine：读取请求并回复
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(5 * time.Millisecond)
			reqData := transport.lastSent()
			if reqData == nil {
				continue
			}
			var req JSONRPCMessage
			if err := json.Unmarshal(reqData, &req); err != nil {
				continue
			}
			resp := JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"ok":true}`),
			}
			respData, _ := json.Marshal(resp)
			transport.pushResponse(respData)
		}
	}()

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := handler.Call(ctx, "test", nil)
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call failed: %v", err)
	}
}

func TestProtocolHandlerErrorMessage(t *testing.T) {
	transport := newChannelMockTransport()
	handler := NewProtocolHandler(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	handler.StartReceiveLoop(ctx)

	go func() {
		time.Sleep(10 * time.Millisecond)
		reqData := transport.lastSent()
		var req JSONRPCMessage
		json.Unmarshal(reqData, &req)

		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
		respData, _ := json.Marshal(resp)
		transport.pushResponse(respData)
	}()

	_, err := handler.Call(ctx, "nonexistent_method", nil)
	if err == nil {
		t.Fatal("expected RPC error")
	}
}
