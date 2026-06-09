package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const bashTimeout = 30 * time.Second

// BashParams Bash工具参数
type BashParams struct {
	Command string `json:"command"`
}

// Bash 命令执行工具（受控沙箱）
type Bash struct {
	workingDir string
}

// NewBash 创建Bash工具实例
func NewBash(workingDir string) *Bash {
	return &Bash{workingDir: workingDir}
}

func (b *Bash) Name() string        { return "bash" }
func (b *Bash) Description() string { return "在终端中执行命令（受控沙箱，超时30秒）" }
func (b *Bash) Parameters() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "要执行的Shell命令",
			},
		},
		"required": []string{"command"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (b *Bash) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p BashParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{Success: false, Error: "参数解析失败"}, nil
	}

	if p.Command == "" {
		return Result{Success: false, Error: "命令不能为空"}, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	absDir, _ := filepath.Abs(b.workingDir)
	shell, shellArgs := getPlatformShell()
	args := append(shellArgs, p.Command)
	cmd := exec.CommandContext(execCtx, shell, args...)
	cmd.Dir = absDir
	cmd.Env = prepareShellEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return Result{Success: false, Error: "命令执行超时（30秒）"}, nil
		}
		if len(output) > 0 {
			return Result{Success: false, Output: string(output), Error: err.Error()}, nil
		}
		return Result{Success: false, Error: err.Error()}, nil
	}

	content := strings.TrimSpace(string(output))
	if content == "" {
		content = "(命令执行成功，无输出)"
	}
	return Result{Success: true, Output: content}, nil
}

// getPlatformShell 跨平台 Shell 检测（例9-17）
func getPlatformShell() (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh", []string{"-Command"}
		}
		return "powershell", []string{"-Command"}
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash", []string{"-c"}
	}
	return "sh", []string{"-c"}
}

// prepareShellEnv 准备跨平台环境变量（例9-17）
func prepareShellEnv() []string {
	env := os.Environ()
	env = append(env, "TERM=xterm-256color")
	if runtime.GOOS == "windows" {
		env = append(env, "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	}
	return env
}
