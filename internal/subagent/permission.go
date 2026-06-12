package subagent

import (
	"errors"
	"fmt"
)

const (
	PermissionLevelReadOnly = "read-only"
	PermissionLevelStandard = "standard"
	PermissionLevelAdvanced = "advanced"

	PermissionDecisionAllowed = "allowed"
	PermissionDecisionDenied  = "denied"
	PermissionDecisionConfirm = "confirm"
)

var ErrNeedConfirm = errors.New("tool requires confirmation")

type PermissionConfig struct {
	Level        string   `yaml:"level"`
	AllowedTools []string `yaml:"allowedTools"`
	DeniedTools  []string `yaml:"deniedTools"`
	ConfirmTools []string `yaml:"confirmTools"`
}

type PermissionSet struct {
	Allowed []string
	Denied  []string
	Confirm []string
}

type PermissionDecision struct {
	Allowed bool
	Confirm bool
	Reason  string
}

func (p *PermissionSet) CheckToolPermission(tool string) (bool, error) {
	decision := p.Decide(tool)
	if decision.Allowed {
		return true, nil
	}
	if decision.Confirm {
		return false, ErrNeedConfirm
	}
	return false, errors.New(decision.Reason)
}

func (p *PermissionSet) Decide(tool string) PermissionDecision {
	if p == nil {
		return PermissionDecision{Reason: "permission set required"}
	}
	if containsString(p.Denied, tool) {
		return PermissionDecision{Reason: "tool denied"}
	}
	if containsString(p.Allowed, tool) {
		return PermissionDecision{Allowed: true}
	}
	if containsString(p.Confirm, tool) {
		return PermissionDecision{Confirm: true, Reason: "tool requires confirmation"}
	}
	return PermissionDecision{Reason: "tool not permitted"}
}

func NormalizePermission(cfg *PermissionConfig) (*PermissionConfig, error) {
	if cfg == nil {
		cfg = &PermissionConfig{Level: PermissionLevelReadOnly}
	} else {
		clone := *cfg
		cfg = &clone
	}
	if cfg.Level == "" {
		cfg.Level = PermissionLevelStandard
	}

	defaults, err := permissionDefaultsForLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	if len(cfg.AllowedTools) == 0 {
		cfg.AllowedTools = append([]string(nil), defaults.Allowed...)
	}
	if len(cfg.DeniedTools) == 0 {
		cfg.DeniedTools = append([]string(nil), defaults.Denied...)
	}
	if len(cfg.ConfirmTools) == 0 {
		cfg.ConfirmTools = append([]string(nil), defaults.Confirm...)
	}
	return cfg, nil
}

func PermissionSetFromConfig(cfg *PermissionConfig) (*PermissionSet, error) {
	normalized, err := NormalizePermission(cfg)
	if err != nil {
		return nil, err
	}
	return &PermissionSet{
		Allowed: append([]string(nil), normalized.AllowedTools...),
		Denied:  append([]string(nil), normalized.DeniedTools...),
		Confirm: append([]string(nil), normalized.ConfirmTools...),
	}, nil
}

func DefaultPermissions(role string) *PermissionSet {
	switch role {
	case "code-reviewer", "security-auditor":
		return &PermissionSet{Allowed: []string{"view", "grep"}, Denied: []string{"write", "bash"}}
	case "architect":
		return &PermissionSet{Allowed: []string{"view", "grep", "bash"}, Denied: []string{"write"}}
	case "test-engineer", "test-writer":
		return &PermissionSet{Allowed: []string{"view", "bash"}, Denied: []string{"write"}}
	case "doc-writer":
		return &PermissionSet{Allowed: []string{"view", "grep"}, Denied: []string{"write", "bash"}}
	default:
		return &PermissionSet{Allowed: []string{"view"}, Denied: []string{"write"}}
	}
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func permissionDefaultsForLevel(level string) (*PermissionSet, error) {
	switch level {
	case PermissionLevelReadOnly:
		return &PermissionSet{Allowed: []string{"view", "grep"}, Denied: []string{"write", "bash"}}, nil
	case PermissionLevelStandard:
		return &PermissionSet{Allowed: []string{"view", "grep"}, Denied: []string{"write"}, Confirm: []string{"bash"}}, nil
	case PermissionLevelAdvanced:
		return &PermissionSet{Allowed: []string{"view", "grep"}, Denied: []string{"write"}, Confirm: []string{"bash"}}, nil
	default:
		return nil, fmt.Errorf("unknown permission level: %s", level)
	}
}
