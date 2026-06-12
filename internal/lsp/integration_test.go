package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/config"
)

func TestDiagnosticsToolLSPPromptFlow(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found")
	}

	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "go.mod"), "module example.com/lspprobe\n\ngo 1.21\n")
	goodFile := filepath.Join(workspace, "good.go")
	badFile := filepath.Join(workspace, "lsp_probe_bad.go")
	writeTestFile(t, goodFile, "package lspprobe\n\nfunc Good() int { return 1 }\n")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := New(ctx, "gopls", config.LSPConfig{
		Command:               "gopls",
		Args:                  []string{"serve"},
		FileTypes:             []string{".go"},
		RootMarkers:           []string{"go.mod"},
		MaxConcurrentRequests: 10,
	})
	if err != nil {
		t.Fatalf("new lsp client: %v", err)
	}
	defer client.Close(context.Background())
	if _, err := client.Initialize(ctx, workspace); err != nil {
		t.Fatalf("initialize lsp client: %v", err)
	}
	// 给 gopls 一点时间完成 workspace 加载
	time.Sleep(1 * time.Second)

	clients := &sync.Map{}
	clients.Store("gopls", client)
	tool := NewDiagnosticsTool(clients, workspace)

	// Prompt 1: 基础可用性
	result1 := executeDiagnostics(t, ctx, tool, "good.go")
	if !result1.Success || strings.Contains(result1.Output, "未找到可用的 LSP 客户端") {
		t.Fatalf("prompt1 unavailable: %+v", result1)
	}
	t.Logf("Prompt1 通过: LSP 可用")

	// 创建错误文件并等待诊断
	writeTestFile(t, badFile, "package lspprobe\n\nfunc Probe() {\n\tfibonaci(5)\n\tfmt.Println(\"done\")\n}\n")

	// 使用工具触发 didOpen/didChange
	executeDiagnostics(t, ctx, tool, "lsp_probe_bad.go")

	t.Logf("等待 gopls 发布诊断（最多 15s）...")
	badURI := uriFromPath(badFile)
	var foundDiags []Diagnostic
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if diags, ok := client.diagnostics.Get(badURI); ok {
			foundDiags = diags
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(foundDiags) == 0 {
		t.Fatal("gopls 未在 15s 内发布诊断")
	}
	diagText := formatDiagnostics(foundDiags)
	if !strings.Contains(diagText, "fibonaci") || !strings.Contains(diagText, "fmt") {
		t.Fatalf("诊断不完整: %q", diagText)
	}
	t.Logf("Prompt2 通过: 诊断正确: %s", diagText)

	// 修复文件
	writeTestFile(t, badFile, "package lspprobe\n\nimport \"fmt\"\n\nfunc Probe() {\n\tfibonacci(5)\n\tfmt.Println(\"done\")\n}\n\nfunc fibonacci(n int) int {\n\tif n < 2 {\n\t\treturn n\n\t}\n\treturn fibonacci(n-1) + fibonacci(n-2)\n}\n")
	before := client.diagnostics.Version()
	client.NotifyChange(ctx, badFile)
	client.WaitForDiagnostics(ctx, badFile, before, 10*time.Second)

	fixedDiags := client.DiagnosticsForFile(badFile)
	diagText2 := formatDiagnostics(fixedDiags)
	if diagText2 != "未发现 LSP 诊断问题" {
		t.Fatalf("Prompt3 失败: 修复后仍有诊断: %q", diagText2)
	}
	t.Logf("Prompt3 通过: 修复后无诊断")

	// Prompt 4: 复查
	before2 := client.diagnostics.Version()
	client.NotifyChange(ctx, badFile)
	client.WaitForDiagnostics(ctx, badFile, before2, 5*time.Second)
	diagText3 := formatDiagnostics(fixedDiags)
	if diagText3 != "未发现 LSP 诊断问题" {
		t.Fatalf("Prompt4 失败: 复查有诊断: %q", diagText3)
	}
	t.Logf("Prompt4 通过: 复查无诊断")
}

func executeDiagnostics(t *testing.T, ctx context.Context, tool *DiagnosticsTool, filePath string) tools.Result {
	t.Helper()
	params, err := json.Marshal(DiagnosticsParams{FilePath: filePath})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("execute diagnostics: %v", err)
	}
	return result
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
