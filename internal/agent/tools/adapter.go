package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"AICodeAgent/internal/permission"
)

const (
	fileWritePerm       = 0644
	toolExecTimeout     = 10 * time.Second
)

// ToolAdapter 工具适配器接口，统一调用规范
type ToolAdapter interface {
	Execute(ctx context.Context, params map[string]any) (any, error)
}

// FileToolAdapter 文件工具适配器
type FileToolAdapter struct{}

// Execute 执行文件操作
func (a *FileToolAdapter) Execute(ctx context.Context, params map[string]any) (any, error) {
	path, _ := params["path"].(string)
	action, _ := params["action"].(string)

	// 路径安全：清理路径中的 .. 和多余分隔符
	cleanPath := filepath.Clean(path)

	switch action {
	case "read":
		content, err := os.ReadFile(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		return string(content), nil
	case "write":
		content, _ := params["content"].(string)
		if err := os.WriteFile(cleanPath, []byte(content), fileWritePerm); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}
		return "write success", nil
	default:
		return nil, fmt.Errorf("unsupported file action: %s", action)
	}
}

// TerminalToolAdapter 终端工具适配器
type TerminalToolAdapter struct{}

// Execute 执行终端命令
func (a *TerminalToolAdapter) Execute(ctx context.Context, params map[string]any) (any, error) {
	command, _ := params["command"].(string)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\n%s", err, string(output))
	}
	return string(output), nil
}

// ToolCaller 工具调用器，串联匹配→校验→执行全流程
type ToolCaller struct {
	registry    *ToolRegistry
	permService *permission.PermissionService
	adapters    map[ToolType]ToolAdapter
}

// NewToolCaller 创建工具调用器
func NewToolCaller(registry *ToolRegistry, permService *permission.PermissionService) *ToolCaller {
	return &ToolCaller{
		registry:    registry,
		permService: permService,
		adapters: map[ToolType]ToolAdapter{
			ToolTypeFile:     &FileToolAdapter{},
			ToolTypeTerminal: &TerminalToolAdapter{},
		},
	}
}

// CallTool 串联执行全流程：意图匹配→权限校验→超时执行
func (c *ToolCaller) CallTool(ctx context.Context, sessionID, intent string,
	params map[string]any) (any, error) {

	// 1. 意图匹配
	matched := c.registry.MatchToolsByIntent(intent)
	if len(matched) == 0 {
		return nil, fmt.Errorf("no tool matched for intent: %s", intent)
	}
	toolMeta := matched[0]

	// 2. 权限校验
	if err := c.permService.CheckToolPermission(
		sessionID, toolMeta.MinPermLevel,
		string(toolMeta.Type), params,
	); err != nil {
		return nil, fmt.Errorf("permission denied: %w", err)
	}

	// 3. 超时执行
	execCtx, cancel := context.WithTimeout(ctx, toolExecTimeout)
	defer cancel()

	adapter, ok := c.adapters[toolMeta.Type]
	if !ok {
		return nil, fmt.Errorf("no adapter for tool type: %s", toolMeta.Type)
	}
	return adapter.Execute(execCtx, params)
}
