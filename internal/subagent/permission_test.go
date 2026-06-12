package subagent

import (
	"errors"
	"testing"
)

func TestCheckToolPermissionOrder(t *testing.T) {
	p := &PermissionSet{
		Allowed: []string{"view", "bash"},
		Denied:  []string{"bash"},
		Confirm: []string{"write"},
	}
	if ok, err := p.CheckToolPermission("bash"); ok || err == nil {
		t.Fatalf("denied tool should win over allowed")
	}
	if ok, err := p.CheckToolPermission("view"); !ok || err != nil {
		t.Fatalf("view should be allowed")
	}
	if ok, err := p.CheckToolPermission("write"); ok || !errors.Is(err, ErrNeedConfirm) {
		t.Fatalf("write should require confirmation, got ok=%v err=%v", ok, err)
	}
	if ok, err := p.CheckToolPermission("unknown"); ok || err == nil {
		t.Fatalf("unknown should default deny")
	}
}

func TestNormalizePermission(t *testing.T) {
	p, err := NormalizePermission(nil)
	if err != nil {
		t.Fatalf("normalize nil: %v", err)
	}
	if p.Level != PermissionLevelReadOnly || !containsString(p.AllowedTools, "view") {
		t.Fatalf("expected read-only defaults: %#v", p)
	}

	p, err = NormalizePermission(&PermissionConfig{})
	if err != nil {
		t.Fatalf("normalize empty: %v", err)
	}
	if p.Level != PermissionLevelStandard || !containsString(p.ConfirmTools, "bash") {
		t.Fatalf("expected standard defaults: %#v", p)
	}

	if _, err := NormalizePermission(&PermissionConfig{Level: "invalid"}); err == nil {
		t.Fatalf("expected invalid level error")
	}
}

func TestDefaultPermissionsLeastPrivilege(t *testing.T) {
	for _, role := range []string{"code-reviewer", "architect", "test-writer", "doc-writer"} {
		if p := DefaultPermissions(role); containsString(p.Allowed, "write") {
			t.Fatalf("%s should not allow write", role)
		}
	}
	if p := DefaultPermissions("doc-writer"); containsString(p.Allowed, "bash") {
		t.Fatalf("doc-writer should not allow bash")
	}
}

func TestBuiltinRoles(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	reviewer, err := registry.Get("code-reviewer")
	if err != nil {
		t.Fatalf("get reviewer: %v", err)
	}
	perm, err := PermissionSetFromConfig(reviewer.Config.Permissions)
	if err != nil {
		t.Fatalf("permission set: %v", err)
	}
	if ok, _ := perm.CheckToolPermission("write"); ok {
		t.Fatalf("reviewer should not write files")
	}
	if ok, _ := perm.CheckToolPermission("bash"); ok {
		t.Fatalf("reviewer should not invoke bash")
	}
}

func TestRoleToolPermissionFilterPassesThroughWithoutActiveSubagent(t *testing.T) {
	filter := NewRoleToolPermissionFilter(NewCoordinator(NewRegistry()))
	allow, needConfirm, reason := filter.CheckRoleToolPermission("s1", "write", nil)
	if !allow || needConfirm || reason != "" {
		t.Fatalf("expected main agent pass-through, allow=%v needConfirm=%v reason=%q", allow, needConfirm, reason)
	}
}

func TestRoleToolPermissionFilterUsesCurrentSubagentPermissions(t *testing.T) {
	registry := NewRegistry()
	agent := &Subagent{Config: &SubagentConfig{
		Name:        "reviewer",
		Description: "Use when reviewing code",
		Model:       "gpt-4o-mini",
		Permissions: &PermissionConfig{Level: PermissionLevelReadOnly, AllowedTools: []string{"view"}, DeniedTools: []string{"write"}},
	}, SystemPrompt: "review code"}
	if err := registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}
	coord := NewCoordinator(registry)
	if _, err := coord.SwitchRole("parent", "reviewer"); err != nil {
		t.Fatalf("switch role: %v", err)
	}
	filter := NewRoleToolPermissionFilter(coord)

	allow, needConfirm, reason := filter.CheckRoleToolPermission("s1", "write", nil)
	if allow || needConfirm || reason == "" {
		t.Fatalf("expected role denial, allow=%v needConfirm=%v reason=%q", allow, needConfirm, reason)
	}
}

func TestRoleToolPermissionFilterReturnsConfirmation(t *testing.T) {
	registry := NewRegistry()
	agent := &Subagent{Config: &SubagentConfig{
		Name:        "architect",
		Description: "Use when designing architecture",
		Model:       "gpt-4o-mini",
		Permissions: &PermissionConfig{Level: PermissionLevelStandard, AllowedTools: []string{"view"}, ConfirmTools: []string{"bash"}},
	}, SystemPrompt: "design architecture"}
	if err := registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}
	coord := NewCoordinator(registry)
	if _, err := coord.SwitchRole("parent", "architect"); err != nil {
		t.Fatalf("switch role: %v", err)
	}
	filter := NewRoleToolPermissionFilter(coord)

	allow, needConfirm, reason := filter.CheckRoleToolPermission("s1", "bash", nil)
	if allow || !needConfirm || reason == "" {
		t.Fatalf("expected role confirmation, allow=%v needConfirm=%v reason=%q", allow, needConfirm, reason)
	}
}
