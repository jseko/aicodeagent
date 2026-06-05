package tools

import (
	"context"
	"encoding/json"
)

// Tool 是AI可调用工具的契约
// 实现此接口即可被Coordinator自动发现和调度
type Tool interface {
	// Name返回工具唯一标识，用于LLM识别和调用
	Name() string

	// Description返回工具功能描述，供LLM理解使用场景
	Description() string

	// Parameters返回JSON Schema，描述工具参数结构
	Parameters() json.RawMessage

	// Execute执行工具逻辑，params是LLM提供的参数JSON
	Execute(ctx context.Context, params json.RawMessage) (Result, error)
}

// Result 是工具执行结果的标准格式
type Result struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}
