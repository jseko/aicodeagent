package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"
)

// Transport MCP 传输层抽象接口，定义统一的数据收发契约
type Transport interface {
	Send(ctx context.Context, message []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

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
	once        sync.Once
}

// NewHTTPTransport 创建 HTTP Transport 实例
func NewHTTPTransport(endpoint string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		Endpoint:    endpoint,
		HTTPClient:  &http.Client{},
		headers:     headers,
		messageChan: make(chan []byte, 100),
	}
}

func (t *HTTPTransport) Send(ctx context.Context, msg []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", t.Endpoint, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http error: %d, body: %s", resp.StatusCode, string(body))
	}

	// 将响应读入接收通道
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		select {
		case t.messageChan <- data:
		default:
		}
	}
	return nil
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
	t.once.Do(func() {
		close(t.messageChan)
	})
	return nil
}

// dropCR 去除行尾的 \r（兼容 Windows）
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[:len(data)-1]
	}
	return data
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
