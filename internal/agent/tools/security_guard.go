package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/rules"
)

// Operation 工具操作描述
type Operation struct {
	SessionID string
	ToolName  string
	Action    string
	Params    map[string]interface{}
}

// SecurityGuard 多层防御安全护栏（第12章 四层纵深防御）
type SecurityGuard struct {
	constitution []rules.Rule
	bashFilter   *BashFilter
	permissions  *permission.PermissionService
	auditLogger  *AsyncAuditLogger
}

// NewSecurityGuard 创建安全护栏
func NewSecurityGuard(constitution []rules.Rule, bashFilter *BashFilter,
	permissions *permission.PermissionService, auditLogger *AsyncAuditLogger) *SecurityGuard {
	if bashFilter == nil {
		bashFilter = NewBashFilter()
	}
	return &SecurityGuard{
		constitution: constitution,
		bashFilter:   bashFilter,
		permissions:  permissions,
		auditLogger:  auditLogger,
	}
}

// CheckOperation 四层纵深防御检查（任何一层失败立即拒绝）
func (g *SecurityGuard) CheckOperation(ctx context.Context, op *Operation) error {
	// Layer 1: Constitution 检查
	if err := g.checkConstitution(op); err != nil {
		g.auditViolation("constitution", op, err)
		return fmt.Errorf("违反Constitution: %w", err)
	}

	// Layer 2: 命令过滤
	if op.ToolName == "bash" {
		if cmd, ok := op.Params["command"].(string); ok {
			result, reason := g.bashFilter.Check(cmd)
			if result == FilterBlocked {
				err := fmt.Errorf("命令被拒绝: %s", reason)
				g.auditViolation("bash_filter", op, err)
				return err
			}
			if result == FilterGrey {
				err := fmt.Errorf("命令需要用户确认: %s", reason)
				g.auditViolation("bash_filter", op, err)
				return err
			}
		}
	}

	// Layer 3: 权限确认
	if g.permissions != nil {
		paramsJSON, _ := json.Marshal(op.Params)
		allow, _, reason := g.permissions.Check(op.ToolName, paramsJSON)
		if !allow {
			err := fmt.Errorf("权限被拒绝: %s", reason)
			g.auditViolation("permission", op, err)
			return err
		}
	}

	// Layer 4: 审计日志（仅记录成功操作，不阻断）
	g.auditOperation(op)
	return nil
}

func (g *SecurityGuard) checkConstitution(op *Operation) error {
	for _, rule := range g.constitution {
		if !rule.Enabled {
			continue
		}
		switch rule.ID {
		case "no_hardcoded_secrets":
			if err := checkNoSecrets(op); err != nil {
				return fmt.Errorf("%s: %w", rule.Name, err)
			}
		}
	}
	return nil
}

func checkNoSecrets(op *Operation) error {
	if op == nil {
		return nil
	}
	return scanSecretValue("", op.Params)
}

var (
	secretKeyPattern    = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|password|passwd|secret|authorization|credential)`)
	secretValuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bsk-[a-z0-9][a-z0-9_-]{12,}\b`),
		regexp.MustCompile(`(?i)\b(api[_-]?key|token|password|secret)\s*:?=\s*["']?[^"'\s]{8,}`),
		regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._-]{16,}\b`),
	}
)

func scanSecretValue(path string, value interface{}) error {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			if secretKeyPattern.MatchString(key) && hasNonEmptySecretValue(child) {
				return fmt.Errorf("检测到疑似敏感字段: %s", joinSecretPath(path, key))
			}
			if err := scanSecretValue(joinSecretPath(path, key), child); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, child := range v {
			if err := scanSecretValue(fmt.Sprintf("%s[%d]", path, i), child); err != nil {
				return err
			}
		}
	case string:
		for _, pattern := range secretValuePatterns {
			if pattern.MatchString(v) {
				return fmt.Errorf("检测到疑似敏感信息模式: %q", pattern.String())
			}
		}
	}
	return nil
}

func hasNonEmptySecretValue(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case nil:
		return false
	default:
		return true
	}
}

func joinSecretPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func (g *SecurityGuard) auditOperation(op *Operation) {
	if g.auditLogger == nil {
		return
	}
	g.auditLogger.LogOperation(op.SessionID, op.ToolName, op.Action, op.Params)
}

func (g *SecurityGuard) auditViolation(layer string, op *Operation, err error) {
	if g.auditLogger == nil {
		return
	}
	g.auditLogger.LogViolation(layer, op.SessionID, op.ToolName, op.Action, op.Params, err)
}
