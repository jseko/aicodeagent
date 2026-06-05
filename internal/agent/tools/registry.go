package tools

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Registry 工具注册表，管理所有已注册的Tool实例
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具，重复注册会panic
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("工具 %s 已注册", name))
	}
	r.tools[name] = tool
}

// Get 按名称获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List 返回所有已注册的工具
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	return list
}

// FunctionDefinition OpenAI函数定义格式
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToOpenAIFunctions 将所有工具转换为OpenAI Function格式
func (r *Registry) ToOpenAIFunctions() []FunctionDefinition {
	tools := r.List()
	functions := make([]FunctionDefinition, len(tools))

	for i, tool := range tools {
		functions[i] = FunctionDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		}
	}
	return functions
}
