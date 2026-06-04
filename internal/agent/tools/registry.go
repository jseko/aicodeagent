package tools

import (
	"strings"
	"sync"
)

// ToolType 工具类型分类
type ToolType string

const (
	ToolTypeFile     ToolType = "file"
	ToolTypeTerminal ToolType = "terminal"
	ToolTypeAPI      ToolType = "api"
)

// ParamMeta 参数元信息
type ParamMeta struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// ToolMeta 工具元数据
type ToolMeta struct {
	ID           string
	Name         string
	Type         ToolType
	Description  string
	Params       []ParamMeta
	MinPermLevel int
	Enabled      bool
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools     map[string]*ToolMeta
	typeIndex map[ToolType][]string
	mu        sync.RWMutex
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:     make(map[string]*ToolMeta),
		typeIndex: make(map[ToolType][]string),
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(meta *ToolMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[meta.ID] = meta
	r.typeIndex[meta.Type] = append(r.typeIndex[meta.Type], meta.ID)
}

// Get 按ID获取工具元数据
func (r *ToolRegistry) Get(id string) (*ToolMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.tools[id]
	return meta, ok
}

// MatchToolsByIntent 根据用户意图匹配工具（关键词匹配）
func (r *ToolRegistry) MatchToolsByIntent(intent string) []*ToolMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*ToolMeta
	switch {
	case strings.Contains(intent, "文件") || strings.Contains(intent, "读取") ||
		strings.Contains(intent, "写入") || strings.Contains(intent, "编辑"):
		for _, id := range r.typeIndex[ToolTypeFile] {
			matched = append(matched, r.tools[id])
		}
	case strings.Contains(intent, "命令") || strings.Contains(intent, "执行") ||
		strings.Contains(intent, "运行") || strings.Contains(intent, "测试"):
		for _, id := range r.typeIndex[ToolTypeTerminal] {
			matched = append(matched, r.tools[id])
		}
	case strings.Contains(intent, "API") || strings.Contains(intent, "请求") ||
		strings.Contains(intent, "HTTP"):
		for _, id := range r.typeIndex[ToolTypeAPI] {
			matched = append(matched, r.tools[id])
		}
	}
	return matched
}

// ListAll 列出所有已注册的工具
func (r *ToolRegistry) ListAll() []*ToolMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []*ToolMeta
	for _, meta := range r.tools {
		all = append(all, meta)
	}
	return all
}
