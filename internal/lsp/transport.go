package lsp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
	stderr bytes.Buffer
}

func newStdioTransport(cmd *exec.Cmd) (*stdioTransport, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	transport := &stdioTransport{cmd: cmd, stdin: stdin, stdout: stdout, reader: bufio.NewReader(stdout)}
	cmd.Stderr = &transport.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}
	return transport, nil
}

func (t *stdioTransport) Send(ctx context.Context, message []byte) error {
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(message))
	if _, err := t.stdin.Write([]byte(frame)); err != nil {
		return err
	}
	_, err := t.stdin.Write(message)
	return err
}

func (t *stdioTransport) Receive(ctx context.Context) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := t.readFrame()
		ch <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result.data, result.err
	}
}

func (t *stdioTransport) Close() error {
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	if t.cmd == nil {
		return nil
	}
	if err := t.cmd.Wait(); err != nil {
		if t.stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(t.stderr.String()))
		}
		return err
	}
	return nil
}

func (t *stdioTransport) readFrame() ([]byte, error) {
	contentLength := 0
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		length, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length: %w", err)
		}
		contentLength = length
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	data := make([]byte, contentLength)
	_, err := io.ReadFull(t.reader, data)
	return data, err
}
