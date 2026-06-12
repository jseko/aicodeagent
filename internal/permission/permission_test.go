package permission

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestCheck_TableDriven(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)
	svc.AddBlacklist("/etc/passwd")
	svc.AddBlacklist("rm -rf")
	svc.AddWhitelist("/home/user/project")

	tests := []struct {
		name        string
		tool        string
		params      json.RawMessage
		wantAllow   bool
		wantConfirm bool
	}{
		{
			name:        "BlacklistHit",
			tool:        "read_file",
			params:      json.RawMessage(`{"file_path":"/etc/passwd"}`),
			wantAllow:   false,
			wantConfirm: false,
		},
		{
			name:        "BlacklistHit_DangerousCommand",
			tool:        "bash",
			params:      json.RawMessage(`{"command":"rm -rf /"}`),
			wantAllow:   false,
			wantConfirm: false,
		},
		{
			name:        "WhitelistHit",
			tool:        "read_file",
			params:      json.RawMessage(`{"file_path":"/home/user/project/main.go"}`),
			wantAllow:   true,
			wantConfirm: false,
		},
		{
			name:        "NeedConfirm_UnknownPath",
			tool:        "read_file",
			params:      json.RawMessage(`{"file_path":"/tmp/unknown.txt"}`),
			wantAllow:   false,
			wantConfirm: true,
		},
		{
			name:        "NeedConfirm_UnknownCommand",
			tool:        "bash",
			params:      json.RawMessage(`{"command":"echo hello"}`),
			wantAllow:   false,
			wantConfirm: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow, needConfirm, _ := svc.Check(tt.tool, tt.params)
			if allow != tt.wantAllow {
				t.Errorf("allow = %v, want %v", allow, tt.wantAllow)
			}
			if needConfirm != tt.wantConfirm {
				t.Errorf("needConfirm = %v, want %v", needConfirm, tt.wantConfirm)
			}
		})
	}
}

func TestAddBlacklist_Concurrent(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.AddBlacklist("/tmp/test")
		}()
	}
	wg.Wait()
}

func TestAddWhitelist_Concurrent(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.AddWhitelist("/tmp/safe")
		}()
	}
	wg.Wait()
}

func TestAuditLog(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)
	svc.AddBlacklist("/etc/passwd")

	// Perform checks to generate audit entries
	svc.Check("read_file", json.RawMessage(`{"file_path":"/etc/passwd"}`))
	svc.Check("read_file", json.RawMessage(`{"file_path":"/tmp/ok.txt"}`))

	log := svc.GetAuditLog()
	if len(log) != 2 {
		t.Fatalf("audit log length = %d, want 2", len(log))
	}
	if log[0].Allowed {
		t.Error("first entry should be denied (blacklist)")
	}
	if log[0].Tool != "read_file" {
		t.Errorf("first entry tool = %s, want read_file", log[0].Tool)
	}
}

func TestSetSessionLevel(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)

	tests := []struct {
		name      string
		sessionID string
		level     PermLevel
		want      PermLevel
	}{
		{"SetAdmin", "sess_1", PermLevelAdmin, PermLevelAdmin},
		{"SetGuest", "sess_2", PermLevelGuest, PermLevelGuest},
		{"DefaultLevel", "sess_new", PermLevelNormal, PermLevelNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sessionID != "sess_new" {
				svc.SetSessionLevel(tt.sessionID, tt.level)
			}
			got := svc.GetSessionLevel(tt.sessionID)
			if got != tt.want {
				t.Errorf("GetSessionLevel(%s) = %d, want %d", tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestCheckToolPermission(t *testing.T) {
	svc := NewPermissionService(
		PermLevelNormal,
		[]string{"/home/user/project"},
		[]string{"go", "ls", "cat"},
	)
	svc.SetSessionLevel("sess_admin", PermLevelAdmin)

	tests := []struct {
		name         string
		sessionID    string
		minPermLevel int
		toolType     string
		params       map[string]any
		wantErr      bool
	}{
		{
			name:         "SufficientPermission_Admin",
			sessionID:    "sess_admin",
			minPermLevel: int(PermLevelAdvanced),
			toolType:     "file",
			params:       map[string]any{"path": "/home/user/project/main.go"},
			wantErr:      false,
		},
		{
			name:         "InsufficientPermission",
			sessionID:    "sess_new",
			minPermLevel: int(PermLevelAdmin),
			toolType:     "file",
			params:       map[string]any{"path": "/tmp/test"},
			wantErr:      true,
		},
		{
			name:         "PathNotInWhitelist",
			sessionID:    "sess_admin",
			minPermLevel: int(PermLevelNormal),
			toolType:     "file",
			params:       map[string]any{"path": "/etc/shadow"},
			wantErr:      true,
		},
		{
			name:         "CommandInWhitelist",
			sessionID:    "sess_admin",
			minPermLevel: int(PermLevelNormal),
			toolType:     "terminal",
			params:       map[string]any{"command": "go build ./..."},
			wantErr:      false,
		},
		{
			name:         "CommandNotInWhitelist",
			sessionID:    "sess_admin",
			minPermLevel: int(PermLevelNormal),
			toolType:     "terminal",
			params:       map[string]any{"command": "rm -rf /"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CheckToolPermission(tt.sessionID, tt.minPermLevel, tt.toolType, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckToolPermission error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestApproveOperation(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)

	params := json.RawMessage(`{"file_path":"/tmp/test.go"}`)
	if svc.IsOperationApproved("sess_1", "read_file", params) {
		t.Error("should not be approved before ApproveOperation")
	}

	svc.ApproveOperation("sess_1", "read_file", params)
	if !svc.IsOperationApproved("sess_1", "read_file", params) {
		t.Error("should be approved after ApproveOperation")
	}

	// Different session should not see the approval
	if svc.IsOperationApproved("sess_2", "read_file", params) {
		t.Error("different session should not see approval")
	}
}

func TestRevokeSessionApprovals(t *testing.T) {
	svc := NewPermissionService(PermLevelNormal, nil, nil)

	params := json.RawMessage(`{"file_path":"/tmp/test.go"}`)
	svc.ApproveOperation("sess_1", "read_file", params)
	svc.RevokeSessionApprovals("sess_1")

	if svc.IsOperationApproved("sess_1", "read_file", params) {
		t.Error("should not be approved after revoke")
	}
}
