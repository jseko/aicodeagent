package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ViewParams View工具参数
type ViewParams struct {
	FilePath string `json:"file_path"`
}

// View 文件查看工具（路径沙箱 + 大小限制）
type View struct {
	workingDir string
}

// NewView 创建View工具实例
func NewView(workingDir string) *View {
	return &View{workingDir: workingDir}
}

// Name 返回工具名称
func (v *View) Name() string {
	return "view"
}

// Description 返回工具描述
func (v *View) Description() string {
	return "读取文件内容（安全检查：路径沙箱 + 大小限制5MB）"
}

// Parameters 返回参数的JSON Schema
func (v *View) Parameters() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "要读取的文件路径（相对于工作目录或绝对路径）",
			},
		},
		"required": []string{"file_path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

// Execute 执行文件读取（安全检查：路径沙箱 + 大小限制）
func (v *View) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p ViewParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{Success: false, Error: "参数解析失败"}, nil
	}

	if p.FilePath == "" {
		return Result{Success: false, Error: "file_path 参数不能为空"}, nil
	}

	log.Printf("[View] 接收参数 file_path=%q workingDir=%q", p.FilePath, v.workingDir)

	// 1. 路径规范化与安全检查
	fullPath := p.FilePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(v.workingDir, p.FilePath)
	}
	fullPath = filepath.Clean(fullPath)

	absPath, _ := filepath.Abs(fullPath)
	absWorkDir, _ := filepath.Abs(v.workingDir)

	log.Printf("[View] 解析路径 abs=%q workdir=%q", absPath, absWorkDir)

	// 严格检查：路径必须在工作目录内
	if !strings.HasPrefix(absPath, absWorkDir+string(filepath.Separator)) &&
		absPath != absWorkDir {
		return Result{Success: false, Error: "路径超出工作目录范围"}, nil
	}

	// 2. 文件大小检查（5MB限制）
	info, err := os.Stat(absPath)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("文件不存在: %s", absPath)}, nil
	}
	if info.IsDir() {
		return Result{Success: false, Error: fmt.Sprintf("路径是目录: %s", absPath)}, nil
	}
	if info.Size() > 5*1024*1024 {
		return Result{Success: false, Error: "文件过大（>5MB）"}, nil
	}

	// 3. 读取内容
	content, err := os.ReadFile(absPath)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("读取失败: %s", absPath)}, nil
	}

	return Result{Success: true, Output: string(content)}, nil
}
