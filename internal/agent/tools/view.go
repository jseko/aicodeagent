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

const (
	maxViewFileSize  = 5 * 1024 * 1024
	maxSkillFileSize = 1024 * 1024
)

// View 文件查看工具（路径沙箱 + 大小限制）
type View struct {
	workingDir string
	skillRoots []string
}

// NewView 创建View工具实例
func NewView(workingDir string) *View {
	return &View{workingDir: workingDir}
}

func (v *View) WithSkillRoots(paths []string) *View {
	v.skillRoots = normalizeRoots(paths)
	return v
}

func (v *View) AllowsSkillRead(filePath string) bool {
	if filePath == "" {
		return false
	}
	absPath, err := v.resolvePath(filePath)
	if err != nil {
		return false
	}
	return v.isInsideSkillRoot(absPath)
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
	absPath, err := v.resolvePath(p.FilePath)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}
	absWorkDir, _ := filepath.Abs(v.workingDir)
	if realWorkDir, err := filepath.EvalSymlinks(absWorkDir); err == nil {
		absWorkDir = realWorkDir
	}

	log.Printf("[View] 解析路径 abs=%q workdir=%q", absPath, absWorkDir)

	isSkillFile := v.isInsideSkillRoot(absPath)
	if !isSkillFile && !isInside(absPath, absWorkDir) {
		return Result{Success: false, Error: "路径超出工作目录范围"}, nil
	}

	// 2. 文件大小检查
	info, err := os.Stat(absPath)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("文件不存在: %s", absPath)}, nil
	}
	if info.IsDir() {
		return Result{Success: false, Error: fmt.Sprintf("路径是目录: %s", absPath)}, nil
	}
	limit := int64(maxViewFileSize)
	if isSkillFile {
		limit = maxSkillFileSize
	}
	if info.Size() > limit {
		if isSkillFile {
			return Result{Success: false, Error: "技能文件过大（>1MB）"}, nil
		}
		return Result{Success: false, Error: "文件过大（>5MB）"}, nil
	}

	// 3. 读取内容
	content, err := os.ReadFile(absPath)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("读取失败: %s", absPath)}, nil
	}

	return Result{Success: true, Output: string(content)}, nil
}

func (v *View) resolvePath(filePath string) (string, error) {
	fullPath := filePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(v.workingDir, filePath)
	}
	fullPath = filepath.Clean(fullPath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("路径解析失败: %s", filePath)
	}
	if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = realPath
	}
	return absPath, nil
}

func (v *View) isInsideSkillRoot(absPath string) bool {
	for _, root := range v.skillRoots {
		if isInside(absPath, root) {
			return true
		}
	}
	return false
}

func normalizeRoots(paths []string) []string {
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		absPath, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			continue
		}
		if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
			absPath = realPath
		}
		roots = append(roots, absPath)
	}
	return roots
}

func isInside(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
