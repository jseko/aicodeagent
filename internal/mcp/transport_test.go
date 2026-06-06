package mcp

import (
	"context"
	"sync"
	"testing"
)

// mockTransport 用于测试的可控 Transport 实现
type mockTransport struct {
	sendCh    chan []byte
	recvQueue [][]byte
	recvIdx   int
	closed    bool
	mu        sync.Mutex
}

func newMockTransport(responses ...[]byte) *mockTransport {
	return &mockTransport{
		sendCh:    make(chan []byte, 10),
		recvQueue: responses,
	}
}

func (m *mockTransport) Send(_ context.Context, msg []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return context.Canceled
	}
	m.sendCh <- msg
	return nil
}

func (m *mockTransport) Receive(_ context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvIdx >= len(m.recvQueue) {
		// Block until closed or more data
		return nil, context.Canceled
	}
	data := m.recvQueue[m.recvIdx]
	m.recvIdx++
	return data, nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	close(m.sendCh)
	return nil
}

func (m *mockTransport) sentMessages() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	var msgs [][]byte
	for {
		select {
		case msg := <-m.sendCh:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

func TestTransportInterface(t *testing.T) {
	transport := newMockTransport()
	var _ Transport = transport // 编译期接口验证

	ctx := context.Background()

	if err := transport.Send(ctx, []byte("hello")); err != nil {
		t.Fatal("Send failed:", err)
	}

	msgs := transport.sentMessages()
	if len(msgs) != 1 || string(msgs[0]) != "hello" {
		t.Fatalf("expected 'hello', got %v", msgs)
	}

	if err := transport.Close(); err != nil {
		t.Fatal("Close failed:", err)
	}

	if err := transport.Send(ctx, []byte("after close")); err == nil {
		t.Fatal("expected error after close")
	}
}

func TestMockTransportReceiveQueue(t *testing.T) {
	responses := [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`),
		[]byte(`{"jsonrpc":"2.0","id":2,"result":{}}`),
	}
	transport := newMockTransport(responses...)
	ctx := context.Background()

	for i, expected := range responses {
		data, err := transport.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive[%d] failed: %v", i, err)
		}
		if string(data) != string(expected) {
			t.Fatalf("Receive[%d]: expected '%s', got '%s'", i, expected, data)
		}
	}
}
