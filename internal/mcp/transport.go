package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os/exec"
	"strings"
	"sync"
)

// Transport MCP 传输层抽象接口，定义统一的数据收发契约
type Transport interface {
	Send(ctx context.Context, message []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

// MCPTransportType 明确区分 MCP 传输形态
type MCPTransportType string

const (
	MCPTransportStdio MCPTransportType = "stdio"
	MCPTransportHTTP  MCPTransportType = "http"
	MCPTransportSSE   MCPTransportType = "sse"
)

const (
	contentTypeJSON        = "application/json"
	contentTypeEventStream = "text/event-stream"
	acceptStreamableHTTP   = "application/json, text/event-stream"
)

// CommandTransport 基于 STDIO 的传输实现（通过子进程 stdin/stdout 通信）
type CommandTransport struct {
	Command *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	reader  *bufio.Reader
}

// NewCommandTransport 创建 STDIO Transport 实例并启动子进程
func NewCommandTransport(cmd *exec.Cmd) (*CommandTransport, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// 捕获子进程 stderr，防止污染终端（TUI 模式下尤其重要）
	cmd.Stderr = &stderrCapture{buf: make([]byte, 0, 4096)}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}
	return &CommandTransport{
		Command: cmd,
		stdin:   stdin,
		stdout:  stdout,
		reader:  bufio.NewReader(stdout),
	}, nil
}

func (t *CommandTransport) Send(ctx context.Context, msg []byte) error {
	_, err := t.stdin.Write(append(msg, '\n'))
	return err
}

func (t *CommandTransport) Receive(ctx context.Context) ([]byte, error) {
	// 使用 goroutine + channel 实现 context 可取消的阻塞读取
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := t.reader.ReadBytes('\n')
		ch <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.data, r.err
	}
}

func (t *CommandTransport) Close() error {
	if t.Command != nil && t.Command.Process != nil {
		t.Command.Process.Kill()
	}
	err := t.Command.Wait()
	// 输出捕获的子进程 stderr 用于调试
	if cap, ok := t.Command.Stderr.(*stderrCapture); ok && len(cap.buf) > 0 {
		log.Printf("[MCP Transport] 子进程 stderr: %s", cap.String())
	}
	return err
}

// HTTPTransport 基于 HTTP POST 的传输实现（远程 MCP Server）
type HTTPTransport struct {
	Endpoint    string
	HTTPClient  *http.Client
	headers     map[string]string
	messageChan chan []byte
}

type SSETransport struct {
	Endpoint    string
	HTTPClient  *http.Client
	headers     map[string]string
	messageChan chan []byte
	ctx         context.Context
	cancel      context.CancelFunc
	body        io.Closer
	mu          sync.Mutex
	once        sync.Once
}

func NewSSETransport(endpoint string, headers map[string]string) *SSETransport {
	return NewLegacySSETransport(endpoint, &http.Client{}, headers)
}

func NewLegacySSETransport(endpoint string, client *http.Client, headers map[string]string) *SSETransport {
	if client == nil {
		client = &http.Client{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SSETransport{
		Endpoint:    endpoint,
		HTTPClient:  client,
		headers:     headers,
		messageChan: make(chan []byte, 100),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (t *SSETransport) Send(ctx context.Context, msg []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	setHeaders(req, t.headers)

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("sse send error: status %d", resp.StatusCode)
	}
	return nil
}

func (t *SSETransport) Receive(ctx context.Context) ([]byte, error) {
	if err := t.start(ctx); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.messageChan:
		return msg, nil
	}
}

func (t *SSETransport) Close() error {
	t.cancel()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.body != nil {
		return t.body.Close()
	}
	return nil
}

func (t *SSETransport) start(ctx context.Context) error {
	var startErr error
	t.once.Do(func() {
		streamCtx, cancel := context.WithCancel(t.ctx)
		go func() {
			<-ctx.Done()
			cancel()
		}()
		startErr = t.openStream(streamCtx)
	})
	return startErr
}

func (t *SSETransport) openStream(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", contentTypeEventStream)
	setHeaders(req, t.headers)

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		return fmt.Errorf("sse connect error: status %d", resp.StatusCode)
	}
	t.mu.Lock()
	t.body = resp.Body
	t.mu.Unlock()
	go readSSE(ctx, resp.Body, t.messageChan)
	return nil
}

func NewHTTPTransport(endpoint string, headers map[string]string) *HTTPTransport {
	return NewStreamableHTTPTransport(endpoint, &http.Client{}, headers)
}

func NewStreamableHTTPTransport(endpoint string, client *http.Client, headers map[string]string) *HTTPTransport {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPTransport{
		Endpoint:    endpoint,
		HTTPClient:  client,
		headers:     headers,
		messageChan: make(chan []byte, 100),
	}
}

func (t *HTTPTransport) Send(ctx context.Context, msg []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Accept", acceptStreamableHTTP)
	setHeaders(req, t.headers)

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("streamable http error: status %d", resp.StatusCode)
	}
	return enqueueHTTPResponse(ctx, resp, t.messageChan)
}

func (t *HTTPTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.messageChan:
		return msg, nil
	}
}

func (t *HTTPTransport) Close() error {
	return nil
}

// dropCR 去除行尾的 \r（兼容 Windows）
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
}

func setHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

func enqueueHTTPResponse(ctx context.Context, resp *http.Response, ch chan<- []byte) error {
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == contentTypeEventStream {
		return readSSE(ctx, resp.Body, ch)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return enqueueMessage(ctx, ch, data)
}

func readSSE(ctx context.Context, r io.Reader, ch chan<- []byte) error {
	scanner := bufio.NewScanner(r)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushSSEData(ctx, ch, data); err != nil {
				return err
			}
			data = nil
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := flushSSEData(ctx, ch, data); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		select {
		case <-ctx.Done():
			return nil
		default:
			return err
		}
	}
	return nil
}

func flushSSEData(ctx context.Context, ch chan<- []byte, data []string) error {
	payload := strings.TrimSpace(strings.Join(data, "\n"))
	if payload == "" {
		return nil
	}
	return enqueueMessage(ctx, ch, []byte(payload))
}

func enqueueMessage(ctx context.Context, ch chan<- []byte, msg []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- msg:
		return nil
	}
}

// stderrCapture 捕获子进程 stderr 输出，防止污染终端
type stderrCapture struct {
	buf []byte
}

func (s *stderrCapture) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	return len(p), nil
}

// StderrString 返回捕获的 stderr 内容
func (s *stderrCapture) String() string {
	return string(s.buf)
}
