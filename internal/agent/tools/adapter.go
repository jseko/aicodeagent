package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"AICodeAgent/internal/hooks"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/rules"
)

const toolExecTimeout = 10 * time.Second

// AuditLogger 轻量审计日志接口
type AuditLogger interface {
	LogSuccess(toolName string, params json.RawMessage)
	LogRejection(toolName string, params json.RawMessage, reason string)
}

// ToolCaller 工具调用器，串联查找→鉴权→执行全流程
type ToolCaller struct {
	registry      *Registry
	permService   *permission.PermissionService
	bashFilter    *BashFilter
	securityGuard *SecurityGuard
	auditLogger   AuditLogger
	hookManager   *hooks.Manager
}

// NewToolCaller 创建工具调用器
func NewToolCaller(registry *Registry, permService *permission.PermissionService) *ToolCaller {
	bashFilter := NewBashFilter()
	return &ToolCaller{
		registry:      registry,
		permService:   permService,
		bashFilter:    bashFilter,
		securityGuard: NewSecurityGuard(rules.DefaultConstitutionRules(), bashFilter, nil, nil),
	}
}

// SetAuditLogger 注入审计日志器（可选，失败不阻断操作）
func (c *ToolCaller) SetAuditLogger(logger AuditLogger) { c.auditLogger = logger }

// SetHookManager 注入 Hooks 调度器（可选，未配置时保持原有执行链路）
func (c *ToolCaller) SetHookManager(manager *hooks.Manager) { c.hookManager = manager }

// SetSecurityGuard 注入统一安全护栏，便于测试和扩展安全策略
func (c *ToolCaller) SetSecurityGuard(guard *SecurityGuard) {
	c.securityGuard = guard
	if guard != nil && guard.bashFilter != nil {
		c.bashFilter = guard.bashFilter
	}
}

// ResolveAndCheck 工具查找+权限检查（不含执行），供旧调用方兼容使用
// 返回值：tool（nil 表示未找到）、allow、needConfirm、reason
func (c *ToolCaller) ResolveAndCheck(toolName string, params json.RawMessage) (Tool, bool, bool, string) {
	return c.ResolveAndCheckForSession("", toolName, params)
}

// ResolveAndCheckForSession 工具查找+会话级权限检查
func (c *ToolCaller) ResolveAndCheckForSession(sessionID, toolName string, params json.RawMessage) (Tool, bool, bool, string) {
	tool, ok := c.registry.Get(toolName)
	if !ok {
		return nil, false, false, fmt.Sprintf("未知工具: %s", toolName)
	}
	if isAllowedSkillRead(toolName, tool, params) {
		return tool, true, false, ""
	}
	if sessionID != "" && c.permService != nil && c.permService.IsOperationApproved(sessionID, toolName, params) {
		return tool, true, false, ""
	}

	if err := c.checkConstitution(toolName, params); err != nil {
		return tool, false, false, err.Error()
	}

	// Bash 安全过滤（第12章工具安全护栏）
	if toolName == "bash" && c.bashFilter != nil {
		var bp BashParams
		if err := json.Unmarshal(params, &bp); err == nil {
			result, reason := c.bashFilter.Check(bp.Command)
			if result == FilterBlocked {
				return tool, false, false, reason
			}
			if result == FilterGrey {
				return tool, false, true, reason
			}
		}
	}

	if c.permService == nil {
		return tool, true, false, ""
	}
	allow, needConfirm, reason := c.permService.Check(toolName, params)
	return tool, allow, needConfirm, reason
}

func (c *ToolCaller) checkConstitution(toolName string, params json.RawMessage) error {
	values := map[string]interface{}{}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &values)
	}
	guard := c.securityGuard
	if guard == nil {
		guard = NewSecurityGuard(rules.DefaultConstitutionRules(), c.bashFilter, nil, nil)
		c.securityGuard = guard
	}
	op := &Operation{ToolName: toolName, Action: "execute", Params: values}
	return guard.checkConstitution(op)
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
	tool, allow, needConfirm, reason := c.ResolveAndCheckForSession(sessionID, toolName, params)
	if tool == nil {
		c.auditRejection(toolName, params, reason)
		return Result{Success: false, Error: reason}, nil
	}
	if !allow {
		c.auditRejection(toolName, params, reason)
		if needConfirm {
			return Result{Success: false, Error: "操作需要用户确认: " + reason}, nil
		}
		return Result{Success: false, Error: reason}, nil
	}

	// 2. 执行前 Hook 可拦截或修改参数
	callID := fmt.Sprintf("%s:%d", toolName, time.Now().UnixNano())
	hookInput := hooks.ToolExecuteInput{Tool: toolName, SessionID: sessionID, CallID: callID}
	hookOutput := hooks.ToolExecuteOutput{Args: params}
	if c.hookManager != nil {
		agg, hookErr := c.hookManager.Trigger(ctx, hooks.EventToolExecuteBefore, hookInput, &hookOutput)
		if hookErr != nil {
			reason := hookErr.Error()
			if agg != nil && agg.Reason != "" {
				reason = agg.Reason
			}
			if agg != nil && agg.Halt {
				reason = "turn halted by hook: " + reason
			}
			c.auditRejection(toolName, hookOutput.Args, reason)
			return Result{Success: false, Error: reason}, nil
		}
	}

	// 3. 超时执行
	execCtx, cancel := context.WithTimeout(ctx, toolExecTimeout)
	defer cancel()

	result, err := tool.Execute(execCtx, hookOutput.Args)

	// 审计日志（仅记录，失败不阻断操作）
	c.auditToolResult(toolName, hookOutput.Args, result)

	if c.hookManager != nil {
		afterInput := hooks.ToolExecuteAfterInput{
			Tool:      toolName,
			SessionID: sessionID,
			CallID:    callID,
			Args:      hookOutput.Args,
			Result:    result,
		}
		if err != nil {
			afterInput.Error = err.Error()
		}
		_, _ = c.hookManager.Trigger(ctx, hooks.EventToolExecuteAfter, afterInput, &hooks.ToolExecuteOutput{Args: hookOutput.Args})
	}

	return result, err
}

func (c *ToolCaller) auditRejection(toolName string, params json.RawMessage, reason string) {
	if c.auditLogger == nil {
		return
	}
	c.auditLogger.LogRejection(toolName, params, reason)
}

func (c *ToolCaller) auditToolResult(toolName string, params json.RawMessage, result Result) {
	if c.auditLogger == nil {
		return
	}
	if result.Success {
		c.auditLogger.LogSuccess(toolName, params)
	} else {
		c.auditLogger.LogRejection(toolName, params, result.Error)
	}
}
