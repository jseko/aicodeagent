package subagent

import (
	"encoding/json"
	"fmt"
)

type RoleToolPermissionFilter struct {
	coordinator *SubagentCoordinator
}

func NewRoleToolPermissionFilter(coordinator *SubagentCoordinator) *RoleToolPermissionFilter {
	return &RoleToolPermissionFilter{coordinator: coordinator}
}

func (f *RoleToolPermissionFilter) CheckRoleToolPermission(sessionID, toolName string, params json.RawMessage) (bool, bool, string) {
	if f == nil || f.coordinator == nil {
		return true, false, ""
	}
	agent, _ := f.coordinator.Current()
	if agent == nil {
		return true, false, ""
	}
	perm, err := PermissionSetFromConfig(agent.Config.Permissions)
	if err != nil {
		return false, false, fmt.Sprintf("subagent permission invalid: %v", err)
	}
	decision := perm.Decide(toolName)
	if decision.Allowed {
		return true, false, ""
	}
	return false, decision.Confirm, fmt.Sprintf("subagent %s: %s", agent.Config.Name, decision.Reason)
}
