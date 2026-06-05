package permission

import (
	"encoding/json"
	"testing"
)

func TestCheckBlacklistDenial(t *testing.T) {
	s := NewPermissionService(PermLevelNormal, nil, nil)
	s.AddBlacklist("rm")
	s.AddBlacklist("sudo")

	allow, needConfirm, reason := s.Check("bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if allow {
		t.Fatal("blacklisted 'rm' command should be denied")
	}
	if needConfirm {
		t.Fatal("blacklisted commands should not need confirmation")
	}
	if reason != "操作被安全策略阻止" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestCheckWhitelistAllow(t *testing.T) {
	s := NewPermissionService(PermLevelNormal, nil, nil)
	s.AddWhitelist("/workspace")

	allow, needConfirm, reason := s.Check("view", json.RawMessage(`{"file_path":"/workspace/main.go"}`))
	if !allow {
		t.Fatal("whitelisted path should be allowed")
	}
	if needConfirm {
		t.Fatal("whitelisted operations should not need confirmation")
	}
	if reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestCheckBlacklistPriority(t *testing.T) {
	s := NewPermissionService(PermLevelNormal, nil, nil)
	s.AddBlacklist("rm")
	s.AddWhitelist("bash")

	// Blacklist takes priority even if whitelist would match
	allow, _, _ := s.Check("bash", json.RawMessage(`{"command":"rm file.txt"}`))
	if allow {
		t.Fatal("blacklist should take priority over whitelist")
	}
}

func TestCheckNeedsConfirmation(t *testing.T) {
	s := NewPermissionService(PermLevelNormal, nil, nil)

	allow, needConfirm, reason := s.Check("unknown_tool", json.RawMessage(`{"key":"value"}`))
	if allow {
		t.Fatal("unclassified tool should not be auto-allowed")
	}
	if !needConfirm {
		t.Fatal("unclassified tool should need confirmation")
	}
	if reason != "操作需要用户确认" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestAuditLogRecorded(t *testing.T) {
	s := NewPermissionService(PermLevelNormal, nil, nil)
	s.AddBlacklist("rm")

	s.Check("bash", json.RawMessage(`{"command":"rm file"}`))
	s.Check("view", json.RawMessage(`{"file_path":"/test"}`))

	logs := s.GetAuditLog()
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(logs))
	}

	if logs[0].Allowed {
		t.Fatal("first audit record should be denied")
	}
}
