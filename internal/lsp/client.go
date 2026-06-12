package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"AICodeAgent/internal/config"
	"AICodeAgent/internal/mcp"
)

const initializeTimeout = 30 * time.Second

type Client struct {
	name        string
	fileTypes   []string
	protocol    *mcp.ProtocolHandler
	transport   mcp.Transport
	sem         chan struct{}
	openFiles   sync.Map
	diagnostics *VersionedMap[string, []Diagnostic]

	diagCountsCache   DiagnosticCounts
	diagCountsVersion uint64
	diagCountsMu      sync.Mutex

	onDiagnosticsChanged func(name string, total int)
	cancel               context.CancelFunc
}

type OpenFileInfo struct {
	Version    int32
	URI        string
	LanguageID string
}

type initializeParams struct {
	ProcessID             int               `json:"processId"`
	RootURI               string            `json:"rootUri,omitempty"`
	Capabilities          map[string]any    `json:"capabilities"`
	InitializationOptions map[string]any    `json:"initializationOptions,omitempty"`
	WorkspaceFolders      []workspaceFolder `json:"workspaceFolders,omitempty"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type InitializeResult struct {
	Capabilities json.RawMessage `json:"capabilities"`
	ServerInfo   *ServerInfo     `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

func New(ctx context.Context, name string, cfg config.LSPConfig) (*Client, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("lsp %s command is empty", name)
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = append(os.Environ(), envPairs(cfg.Env)...)
	transport, err := newStdioTransport(cmd)
	if err != nil {
		return nil, err
	}

	maxConcurrent := cfg.MaxConcurrentRequests
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	clientCtx, cancel := context.WithCancel(ctx)
	client := &Client{
		name:        name,
		fileTypes:   cfg.FileTypes,
		transport:   transport,
		sem:         make(chan struct{}, maxConcurrent),
		diagnostics: NewVersionedMap[string, []Diagnostic](),
		cancel:      cancel,
	}
	client.protocol = mcp.NewProtocolHandler(transport)
	client.protocol.SetNotifyHandler(func(method string, params json.RawMessage) {
		if method == "textDocument/publishDiagnostics" {
			HandleDiagnostics(client, params)
		}
	})
	client.protocol.StartReceiveLoop(clientCtx)
	return client, nil
}

func (c *Client) Initialize(ctx context.Context, workspaceDir string) (*InitializeResult, error) {
	initCtx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()

	params := initializeParams{
		ProcessID:    0,
		RootURI:      uriFromPath(workspaceDir),
		Capabilities: defaultCapabilities(),
		WorkspaceFolders: []workspaceFolder{{
			URI:  uriFromPath(workspaceDir),
			Name: workspaceDir,
		}},
	}

	result, err := c.call(initCtx, "initialize", params)
	if err != nil {
		return nil, err
	}
	if err := c.notify(initCtx, "initialized", map[string]any{}); err != nil {
		return nil, err
	}

	var initResult InitializeResult
	if len(result) > 0 {
		if err := json.Unmarshal(result, &initResult); err != nil {
			return nil, err
		}
	}
	return &initResult, nil
}

func (c *Client) Close(ctx context.Context) error {
	_ = c.CloseAllFiles(ctx)
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = c.call(shutdownCtx, "shutdown", nil)
	_ = c.notify(context.Background(), "exit", nil)
	if c.cancel != nil {
		c.cancel()
	}
	return c.transport.Close()
}

func (c *Client) SetDiagnosticsCallback(callback func(name string, total int)) {
	c.onDiagnosticsChanged = callback
}

func (c *Client) HandlesFile(filePath string) bool {
	lang := DetectLanguageID(filePath)
	if lang == "" {
		return false
	}
	if len(c.fileTypes) == 0 {
		return true
	}
	for _, ext := range c.fileTypes {
		if hasFileType(filePath, ext) {
			return true
		}
	}
	return false
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	return c.protocol.Call(ctx, method, params)
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.release()
	return c.protocol.SendNotification(ctx, method, params)
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() {
	<-c.sem
}

func defaultCapabilities() map[string]any {
	return map[string]any{
		"textDocument": map[string]any{
			"publishDiagnostics": map[string]any{"relatedInformation": true},
			"synchronization":    map[string]any{"didSave": true},
		},
	}
}

func envPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}
