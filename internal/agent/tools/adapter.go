package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"AICodeAgent/internal/permission"
)

const toolExecTimeout = 10 * time.Second

// ToolCaller 工具调用器，串联查找→鉴权→执行全流程
type ToolCaller struct {
	registry    *Registry
	permService *permission.PermissionService
}

// NewToolCaller 创建工具调用器
func NewToolCaller(registry *Registry, permService *permission.PermissionService) *ToolCaller {
	return &ToolCaller{
		registry:    registry,
		permService: permService,
	}
}

// ResolveAndCheck 工具查找+权限检查（不含执行），供 handleToolCalls 和 CallTool 共享
// 返回值：tool（nil 表示未找到）、allow、needConfirm、reason
func (c *ToolCaller) ResolveAndCheck(toolName string, params json.RawMessage) (Tool, bool, bool, string) {
	tool, ok := c.registry.Get(toolName)
	if !ok {
		return nil, false, false, fmt.Sprintf("未知工具: %s", toolName)
	}
	if isAllowedSkillRead(toolName, tool, params) {
		return tool, true, false, ""
	}
	allow, needConfirm, reason := c.permService.Check(toolName, params)
	return tool, allow, needConfirm, reason
}

type skillReadAuthorizer interface {
	AllowsSkillRead(filePath string) bool
}

func isAllowedSkillRead(toolName string, tool Tool, params json.RawMessage) bool {
	if toolName != "view" {
		return false
	}
	authorizer, ok := tool.(skillReadAuthorizer)
	if !ok {
		return false
	}
	var viewParams ViewParams
	if err := json.Unmarshal(params, &viewParams); err != nil {
		return false
	}
	return authorizer.AllowsSkillRead(viewParams.FilePath)
}

// CallTool 串联执行全流程：精确查找→权限检查→超时执行
func (c *ToolCaller) CallTool(ctx context.Context, sessionID, toolName string,
	params json.RawMessage) (Result, error) {

	// 1. 精确查找+权限检查（共享逻辑）
	tool, allow, needConfirm, reason := c.ResolveAndCheck(toolName, params)
	if tool == nil {
		return Result{Success: false, Error: reason}, nil
	}
	if !allow {
		if needConfirm {
			return Result{Success: false, Error: "操作需要用户确认: " + reason}, nil
		}
		return Result{Success: false, Error: reason}, nil
	}

	// 2. 超时执行
	execCtx, cancel := context.WithTimeout(ctx, toolExecTimeout)
	defer cancel()

	return tool.Execute(execCtx, params)
}
