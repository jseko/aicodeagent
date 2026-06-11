package permission

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PermLevel 权限等级（Deprecated: 使用 Check 三层检查替代）
type PermLevel int

const (
	PermLevelGuest    PermLevel = iota // 0: 访客，仅查询
	PermLevelNormal                    // 1: 普通，可读文件
	PermLevelAdvanced                  // 2: 高级，可写文件
	PermLevelAdmin                     // 3: 管理员，可执行命令
)

// AuditRecord 权限检查审计记录
type AuditRecord struct {
	Timestamp time.Time
	Tool      string
	Params    string
	Allowed   bool
	Reason    string
}

// PermissionRequest 权限请求（事件驱动确认）
type PermissionRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	Action    string `json:"action"`
}

// PermissionService 权限控制服务（三层检查：黑名单→白名单→确认）
type PermissionService struct {
	blacklist        map[string]bool
	whitelist        map[string]bool
	auditLog         []AuditRecord
	auditMu          sync.Mutex
	sessionPermCache map[string]PermLevel
	sessionOpsCache  map[string]map[string]bool // sessionID → cacheKey → approved
	fileWhitelist    []string
	cmdWhitelist     []string
	defaultLevel     PermLevel
	mu               sync.RWMutex
}

// NewPermissionService 创建权限服务实例
func NewPermissionService(defaultLevel PermLevel, fileWhitelist, cmdWhitelist []string) *PermissionService {
	return &PermissionService{
		blacklist:        make(map[string]bool),
		whitelist:        make(map[string]bool),
		auditLog:         make([]AuditRecord, 0),
		sessionPermCache: make(map[string]PermLevel),
		sessionOpsCache:  make(map[string]map[string]bool),
		fileWhitelist:    fileWhitelist,
		cmdWhitelist:     cmdWhitelist,
		defaultLevel:     defaultLevel,
	}
}

// AddBlacklist 添加黑名单模式
func (s *PermissionService) AddBlacklist(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blacklist[pattern] = true
}

// AddWhitelist 添加白名单模式
func (s *PermissionService) AddWhitelist(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.whitelist[pattern] = true
}

// Check 执行三层权限检查（黑名单→白名单→确认）
func (p *PermissionService) Check(tool string, params json.RawMessage) (allow bool, needConfirm bool, reason string) {
	paramsStr := string(params)

	p.mu.RLock()
	defer p.mu.RUnlock()

	// 1. 黑名单检查（最高优先级）
	for pattern := range p.blacklist {
		if strings.Contains(paramsStr, pattern) {
			p.log(tool, paramsStr, false, "命中黑名单: "+pattern)
			return false, false, "操作被安全策略阻止"
		}
	}

	// 2. 白名单检查
	for pattern := range p.whitelist {
		if strings.Contains(paramsStr, pattern) {
			p.log(tool, paramsStr, true, "命中白名单")
			return true, false, ""
		}
	}

	// 3. 未分类操作需用户确认
	p.log(tool, paramsStr, false, "等待用户确认")
	return false, true, "操作需要用户确认"
}

// log 记录审计日志
func (p *PermissionService) log(tool, params string, allowed bool, reason string) {
	p.auditMu.Lock()
	defer p.auditMu.Unlock()

	record := AuditRecord{
		Timestamp: time.Now(),
		Tool:      tool,
		Params:    params,
		Allowed:   allowed,
		Reason:    reason,
	}
	p.auditLog = append(p.auditLog, record)
}

// GetAuditLog 获取审计日志
func (p *PermissionService) GetAuditLog() []AuditRecord {
	return p.auditLog
}

// SetSessionLevel 设置会话权限等级（Deprecated）
func (s *PermissionService) SetSessionLevel(sessionID string, level PermLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionPermCache[sessionID] = level
}

// GetSessionLevel 获取会话权限等级（Deprecated）
func (s *PermissionService) GetSessionLevel(sessionID string) PermLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getSessionLevelLocked(sessionID)
}

// getSessionLevelLocked 无锁版本，供内部已持锁方法调用
func (s *PermissionService) getSessionLevelLocked(sessionID string) PermLevel {
	if level, ok := s.sessionPermCache[sessionID]; ok {
		return level
	}
	return s.defaultLevel
}

// CheckToolPermission 双重校验：权限等级 + 操作范围白名单（Deprecated: 使用 Check 方法替代）
func (s *PermissionService) CheckToolPermission(
	sessionID string,
	minPermLevel int,
	toolType string,
	params map[string]any,
) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. 粗粒度：权限等级校验
	permLevel := s.getSessionLevelLocked(sessionID)
	if permLevel < PermLevel(minPermLevel) {
		return fmt.Errorf("权限不足：会话等级 %d < 工具要求 %d", permLevel, minPermLevel)
	}

	// 2. 细粒度：操作范围白名单校验
	switch toolType {
	case "file":
		if path, ok := params["path"].(string); ok {
			if !s.isInWhitelist(path, s.fileWhitelist) {
				return fmt.Errorf("路径不在白名单：%s", path)
			}
		}
	case "terminal":
		if cmd, ok := params["command"].(string); ok {
			if !s.isInCmdWhitelist(cmd, s.cmdWhitelist) {
				return fmt.Errorf("命令不在白名单：%s", cmd)
			}
		}
	}
	return nil
}

// isInWhitelist 检查路径是否在白名单范围内（目录边界安全匹配）
func (s *PermissionService) isInWhitelist(path string, whitelist []string) bool {
	for _, allowed := range whitelist {
		if strings.HasPrefix(path, allowed) && (len(path) == len(allowed) || path[len(allowed)] == '/') {
			return true
		}
	}
	return false
}

// isInCmdWhitelist 检查命令是否在白名单内（命令边界安全匹配）
// buildOpKey 构造保守的缓存键（工具名 + 核心参数）
func buildOpKey(toolName string, params json.RawMessage) string {
	var sb strings.Builder
	sb.WriteString(toolName)
	if params != nil && len(params) > 0 {
		var m map[string]interface{}
		if err := json.Unmarshal(params, &m); err == nil {
			for _, k := range []string{"file_path", "command", "path"} {
				if v, ok := m[k].(string); ok && v != "" {
					sb.WriteString(":")
					sb.WriteString(v)
					break
				}
			}
		}
	}
	return sb.String()
}

// ApproveOperation 记录会话级操作批准
func (s *PermissionService) ApproveOperation(sessionID string, toolName string, params json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := buildOpKey(toolName, params)
	if s.sessionOpsCache[sessionID] == nil {
		s.sessionOpsCache[sessionID] = make(map[string]bool)
	}
	s.sessionOpsCache[sessionID][key] = true
}

// IsOperationApproved 检查操作是否已被该会话批准
func (s *PermissionService) IsOperationApproved(sessionID string, toolName string, params json.RawMessage) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := buildOpKey(toolName, params)
	if ops, ok := s.sessionOpsCache[sessionID]; ok {
		return ops[key]
	}
	return false
}

// RevokeSessionApprovals 撤销会话所有批准
func (s *PermissionService) RevokeSessionApprovals(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionOpsCache, sessionID)
}

func (s *PermissionService) isInCmdWhitelist(cmd string, whitelist []string) bool {
	for _, allowed := range whitelist {
		if strings.HasPrefix(cmd, allowed) && (len(cmd) == len(allowed) || cmd[len(allowed)] == ' ') {
			return true
		}
	}
	return false
}
