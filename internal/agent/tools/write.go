package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// WriteParams Write工具参数
type WriteParams struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// Write 文件写入工具（路径沙箱 + 大小限制）
type Write struct {
	workingDir string
}

// NewWrite 创建Write工具实例
func NewWrite(workingDir string) *Write {
	return &Write{workingDir: workingDir}
}

func (w *Write) Name() string { return "write" }
func (w *Write) Description() string {
	return "写入文件内容（安全检查：路径沙箱 + 大小限制1MB）"
}
func (w *Write) Parameters() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "要写入的文件路径",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要写入的文件内容",
			},
		},
		"required": []string{"file_path", "content"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (w *Write) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p WriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{Success: false, Error: "参数解析失败"}, nil
	}

	// 1. 路径安全检查（与View一致）
	fullPath := p.FilePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(w.workingDir, p.FilePath)
	}
	fullPath = filepath.Clean(fullPath)

	absPath, _ := filepath.Abs(fullPath)
	absWorkDir, _ := filepath.Abs(w.workingDir)

	if !strings.HasPrefix(absPath, absWorkDir+string(filepath.Separator)) &&
		absPath != absWorkDir {
		return Result{Success: false, Error: "路径超出工作目录范围"}, nil
	}

	// 2. 内容大小检查（1MB限制）
	if len(p.Content) > 1*1024*1024 {
		return Result{Success: false, Error: "内容过大（>1MB）"}, nil
	}

	// 3. 创建父目录（如果不存在）
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Result{Success: false, Error: "创建目录失败"}, nil
	}

	// 4. 写入文件
	if err := os.WriteFile(absPath, []byte(p.Content), 0644); err != nil {
		return Result{Success: false, Error: "写入失败"}, nil
	}

	return Result{Success: true, Output: "文件写入成功"}, nil
}
