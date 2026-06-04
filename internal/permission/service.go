package permission

import (
	"fmt"
	"strings"
	"sync"
)

// PermLevel 权限等级
type PermLevel int

const (
	PermLevelGuest    PermLevel = iota // 0: 访客，仅查询
	PermLevelNormal                     // 1: 普通，可读文件
	PermLevelAdvanced                   // 2: 高级，可写文件
	PermLevelAdmin                      // 3: 管理员，可执行命令
)

// PermissionService 权限控制服务
type PermissionService struct {
	sessionPermCache map[string]PermLevel
	fileWhitelist    []string
	cmdWhitelist     []string
	defaultLevel     PermLevel
	mu               sync.RWMutex
}

// NewPermissionService 创建权限服务实例
func NewPermissionService(defaultLevel PermLevel, fileWhitelist, cmdWhitelist []string) *PermissionService {
	return &PermissionService{
		sessionPermCache: make(map[string]PermLevel),
		fileWhitelist:    fileWhitelist,
		cmdWhitelist:     cmdWhitelist,
		defaultLevel:     defaultLevel,
	}
}

// SetSessionLevel 设置会话权限等级
func (s *PermissionService) SetSessionLevel(sessionID string, level PermLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionPermCache[sessionID] = level
}

// GetSessionLevel 获取会话权限等级
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

// CheckToolPermission 双重校验：权限等级 + 操作范围白名单
func (s *PermissionService) CheckToolPermission(
	sessionID string,
	minPermLevel int,
	toolType string,
	params map[string]any,
) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. 粗粒度：权限等级校验（使用无锁版本避免重入死锁）
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
func (s *PermissionService) isInCmdWhitelist(cmd string, whitelist []string) bool {
	for _, allowed := range whitelist {
		if strings.HasPrefix(cmd, allowed) && (len(cmd) == len(allowed) || cmd[len(allowed)] == ' ') {
			return true
		}
	}
	return false
}
