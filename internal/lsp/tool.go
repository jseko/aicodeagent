package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"AICodeAgent/internal/agent/tools"
)

type DiagnosticsParams struct {
	FilePath string `json:"file_path"`
}

type ClientStatus struct {
	Name        string
	State       string
	WorkingDir  string
	Command     string
	FileTypes   []string
	RootMarkers []string
	Error       string
}

type DiagnosticsTool struct {
	clients    *sync.Map
	statuses   *sync.Map
	workingDir string
}

func NewDiagnosticsTool(clients *sync.Map, workingDir string, statuses ...*sync.Map) *DiagnosticsTool {
	var statusMap *sync.Map
	if len(statuses) > 0 {
		statusMap = statuses[0]
	}
	return &DiagnosticsTool{clients: clients, statuses: statusMap, workingDir: workingDir}
}

func (t *DiagnosticsTool) Name() string {
	return "lsp_diagnostics"
}

func (t *DiagnosticsTool) Description() string {
	return "获取 LSP 诊断信息，用于检查代码错误、警告和修复效果"
}

func (t *DiagnosticsTool) Parameters() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "要检查的文件路径，相对工作目录或绝对路径",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DiagnosticsTool) Execute(ctx context.Context, params json.RawMessage) (tools.Result, error) {
	var p DiagnosticsParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return tools.Result{Success: false, Error: "参数解析失败"}, nil
		}
	}

	filePath := p.FilePath
	if filePath != "" && !filepath.IsAbs(filePath) {
		filePath = filepath.Join(t.workingDir, filePath)
	}
	if filePath != "" {
		filePath = filepath.Clean(filePath)
	}

	client := t.clientForFile(filePath)
	if client == nil {
		return tools.Result{Success: true, Output: t.formatNoClient(filePath)}, nil
	}

	if filePath != "" {
		beforeVersion := client.diagnostics.Version()
		if err := client.OpenFile(ctx, filePath); err != nil {
			return tools.Result{Success: false, Error: err.Error()}, nil
		}
		if err := client.NotifyChange(ctx, filePath); err != nil {
			return tools.Result{Success: false, Error: err.Error()}, nil
		}
		client.WaitForDiagnostics(ctx, filePath, beforeVersion, 5*time.Second)
		return tools.Result{Success: true, Output: formatDiagnostics(client.DiagnosticsForFile(filePath))}, nil
	}

	counts := client.GetDiagnosticCounts()
	return tools.Result{Success: true, Output: fmt.Sprintf("Errors: %d\nWarnings: %d\nInformation: %d\nHints: %d", counts.Error, counts.Warning, counts.Information, counts.Hint)}, nil
}

func (t *DiagnosticsTool) clientForFile(filePath string) *Client {
	var fallback *Client
	t.clients.Range(func(_, value any) bool {
		client, ok := value.(*Client)
		if !ok {
			return true
		}
		if fallback == nil {
			fallback = client
		}
		if filePath != "" && client.HandlesFile(filePath) {
			fallback = client
			return false
		}
		return true
	})
	return fallback
}

func (t *DiagnosticsTool) formatNoClient(filePath string) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, "未找到可用的 LSP 客户端")
	fmt.Fprintf(&sb, "工作目录: %s\n", t.workingDir)
	if filePath != "" {
		fmt.Fprintf(&sb, "目标文件: %s\n", filePath)
	}

	clientCount := 0
	t.clients.Range(func(_, _ any) bool {
		clientCount++
		return true
	})
	fmt.Fprintf(&sb, "已注册客户端数量: %d\n", clientCount)

	if t.statuses == nil {
		return strings.TrimSpace(sb.String())
	}

	statusCount := 0
	t.statuses.Range(func(_, value any) bool {
		status, ok := value.(ClientStatus)
		if !ok {
			return true
		}
		statusCount++
		fmt.Fprintf(&sb, "- %s: %s\n", status.Name, status.State)
		fmt.Fprintf(&sb, "  command: %s\n", status.Command)
		fmt.Fprintf(&sb, "  working_dir: %s\n", status.WorkingDir)
		fmt.Fprintf(&sb, "  file_types: %s\n", strings.Join(status.FileTypes, ", "))
		fmt.Fprintf(&sb, "  root_markers: %s\n", strings.Join(status.RootMarkers, ", "))
		if status.Error != "" {
			fmt.Fprintf(&sb, "  error: %s\n", status.Error)
		}
		return true
	})
	if statusCount == 0 {
		fmt.Fprintln(&sb, "LSP 初始化状态: 无记录")
	}
	return strings.TrimSpace(sb.String())
}

func formatDiagnostics(diagnostics []Diagnostic) string {
	if len(diagnostics) == 0 {
		return "未发现 LSP 诊断问题"
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Range.Start.Line == diagnostics[j].Range.Start.Line {
			return diagnostics[i].Range.Start.Character < diagnostics[j].Range.Start.Character
		}
		return diagnostics[i].Range.Start.Line < diagnostics[j].Range.Start.Line
	})

	var sb strings.Builder
	for _, diag := range diagnostics {
		severity := severityName(diag.Severity)
		line := diag.Range.Start.Line + 1
		column := diag.Range.Start.Character + 1
		fmt.Fprintf(&sb, "%s:%d:%d [%s] %s\n", diag.FilePath, line, column, severity, diag.Message)
	}
	return strings.TrimSpace(sb.String())
}

func severityName(severity int) string {
	switch severity {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return "error"
	}
}
