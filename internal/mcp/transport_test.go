package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
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

func TestStreamableHTTPTransportJSONResponse(t *testing.T) {
	var gotAccept, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != contentTypeJSON {
			t.Fatalf("expected content-type %s, got %s", contentTypeJSON, r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, map[string]string{"Authorization": "Bearer secret"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := transport.Send(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	msg, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if string(msg) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatalf("unexpected message: %s", msg)
	}
	if gotAccept != acceptStreamableHTTP {
		t.Fatalf("expected Accept %q, got %q", acceptStreamableHTTP, gotAccept)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected Authorization header to be injected")
	}
}

func TestStreamableHTTPTransportEventStreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeEventStream)
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := transport.Send(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	msg, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if string(msg) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestStreamableHTTPTransportErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret body", http.StatusUnauthorized)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL, map[string]string{"Authorization": "Bearer secret"})
	err := transport.Send(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "streamable http error: status 401" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSETransportReceiveDataEventAndClose(t *testing.T) {
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Header.Get("Accept") != contentTypeEventStream {
			t.Fatalf("expected Accept %q, got %q", contentTypeEventStream, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", contentTypeEventStream)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		close(connected)
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notice\"}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	transport := NewSSETransport(server.URL, map[string]string{"X-Test": "ok"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if string(msg) != `{"jsonrpc":"2.0","method":"notice"}` {
		t.Fatalf("unexpected message: %s", msg)
	}
	<-connected
	if err := transport.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestSSETransportReceiveContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeEventStream)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	transport := NewSSETransport(server.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := transport.Receive(ctx)
	if err == nil {
		t.Fatal("expected context timeout")
	}
	transport.Close()
}
