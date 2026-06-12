package hooks

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// SensitiveFileBlocker blocks tool operations on sensitive files (.env, *.pem, id_rsa).
type SensitiveFileBlocker struct {
	patterns []string
}

// NewSensitiveFileBlocker creates a blocker with default sensitive patterns.
func NewSensitiveFileBlocker() *SensitiveFileBlocker {
	return &SensitiveFileBlocker{
		patterns: []string{".env", ".pem", "id_rsa", "id_ed25519", "credentials.json", "secrets.yaml"},
	}
}

func (h *SensitiveFileBlocker) Name() string     { return "builtin.sensitive_file_blocker" }
func (h *SensitiveFileBlocker) Types() []EventType { return []EventType{EventToolExecuteBefore} }
func (h *SensitiveFileBlocker) Priority() Priority  { return PriorityBlocker }

func (h *SensitiveFileBlocker) Execute(ctx context.Context, input interface{}, output interface{}) error {
	in, ok := input.(*ToolExecuteInput)
	if !ok || in.Tool == "" {
		return nil
	}
	// Only check file-modifying tools
	if in.Tool != "write_file" && in.Tool != "view" && in.Tool != "bash" {
		return nil
	}
	out, ok := output.(*ToolExecuteOutput)
	if !ok || len(out.Args) == 0 {
		return nil
	}

	var params map[string]interface{}
	if err := json.Unmarshal(out.Args, &params); err != nil {
		return nil
	}

	// Check file_path and content for sensitive patterns
	paths := extractFilePaths(params)
	for _, p := range paths {
		base := filepath.Base(p)
		for _, pattern := range h.patterns {
			if strings.EqualFold(base, pattern) || strings.HasSuffix(base, pattern) {
				return &HookError{Reason: "sensitive file blocked: " + base}
			}
		}
	}
	return nil
}

// HookError signals a hook denial with a reason.
type HookError struct {
	Reason string
}

func (e *HookError) Error() string { return e.Reason }

func extractFilePaths(params map[string]interface{}) []string {
	var paths []string
	if fp, ok := params["file_path"]; ok {
		if s, ok := fp.(string); ok && s != "" {
			paths = append(paths, s)
		}
	}
	if content, ok := params["content"]; ok {
		if s, ok := content.(string); ok && s != "" {
			// Scan content for sensitive path-like patterns
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, ".env") ||
					strings.Contains(line, ".pem") ||
					strings.Contains(line, "id_rsa") {
					paths = append(paths, line)
				}
			}
		}
	}
	return paths
}

// ToolAuditObserver records tool execution metadata without blocking.
type ToolAuditObserver struct {
	mu   sync.Mutex
	logs []AuditEntry
}

// AuditEntry holds metadata for a single tool execution.
type AuditEntry struct {
	Tool      string `json:"tool"`
	SessionID string `json:"session_id"`
	CallID    string `json:"call_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func NewToolAuditObserver() *ToolAuditObserver { return &ToolAuditObserver{} }

func (h *ToolAuditObserver) Name() string     { return "builtin.tool_audit_observer" }
func (h *ToolAuditObserver) Types() []EventType { return []EventType{EventToolExecuteAfter} }
func (h *ToolAuditObserver) Priority() Priority  { return PriorityObserver }

func (h *ToolAuditObserver) OnEvent(ctx context.Context, event interface{}) error {
	in, ok := event.(*ToolExecuteAfterInput)
	if !ok {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, AuditEntry{
		Tool:      in.Tool,
		SessionID: in.SessionID,
		CallID:    in.CallID,
		Success:   in.Error == "",
		Error:     in.Error,
	})
	return nil
}

func (h *ToolAuditObserver) Entries() []AuditEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]AuditEntry, len(h.logs))
	copy(out, h.logs)
	return out
}
