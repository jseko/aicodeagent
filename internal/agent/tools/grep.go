package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const grepTimeout = 15 * time.Second

// GrepParams Grep工具参数
type GrepParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// Grep 内容搜索工具（优先使用ripgrep，回退到grep）
type Grep struct {
	workingDir string
}

// NewGrep 创建Grep工具实例
func NewGrep(workingDir string) *Grep {
	return &Grep{workingDir: workingDir}
}

func (g *Grep) Name() string        { return "grep" }
func (g *Grep) Description() string { return "在文件中搜索指定模式（优先使用ripgrep，回退到grep）" }
func (g *Grep) Parameters() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "搜索模式（正则表达式）",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "搜索路径（相对于工作目录，默认当前目录）",
			},
		},
		"required": []string{"pattern"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (g *Grep) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p GrepParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{Success: false, Error: "参数解析失败"}, nil
	}

	if p.Pattern == "" {
		return Result{Success: false, Error: "搜索模式不能为空"}, nil
	}

	searchPath := p.Path
	if searchPath == "" {
		searchPath = "."
	}

	absWorkDir, _ := filepath.Abs(g.workingDir)
	fullPath := searchPath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(g.workingDir, searchPath)
	}
	fullPath = filepath.Clean(fullPath)
	absPath, _ := filepath.Abs(fullPath)

	if !strings.HasPrefix(absPath, absWorkDir+string(filepath.Separator)) &&
		absPath != absWorkDir {
		return Result{Success: false, Error: "路径超出工作目录范围"}, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, grepTimeout)
	defer cancel()

	// 优先使用 ripgrep，回退到 grep
	cmd := g.buildGrepCmd(execCtx, p.Pattern, absPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			if len(output) == 0 {
				return Result{Success: true, Output: "未找到匹配项"}, nil
			}
		}
		if execCtx.Err() == context.DeadlineExceeded {
			return Result{Success: false, Error: "搜索超时（15秒）"}, nil
		}
	}

	content := strings.TrimSpace(string(output))
	if content == "" {
		content = "未找到匹配项"
	}

	// 限制输出大小（最多5000字符）
	if len(content) > 5000 {
		lines := strings.Split(content, "\n")
		maxLines := len(lines)
		if maxLines > 50 {
			maxLines = 50
		}
		content = strings.Join(lines[:maxLines], "\n")
		content += "\n...(结果已截断)"
	}

	return Result{Success: true, Output: content}, nil
}

func (g *Grep) buildGrepCmd(ctx context.Context, pattern, path string) *exec.Cmd {
	if rg, err := exec.LookPath("rg"); err == nil {
		return exec.CommandContext(ctx, rg, "-n", "--no-heading", pattern, path)
	}
	return exec.CommandContext(ctx, "grep", "-rn", pattern, path)
}
