package app

import (
	"context"
	"path/filepath"
	"testing"

	"AICodeAgent/internal/config"
)

func TestAppInitializesSubagentsWithoutAgenticDelegation(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	app := New(context.Background(), cfg)
	defer app.Close()

	if app.Subagents == nil || app.SubagentCoord == nil {
		t.Fatalf("expected subagent registry and coordinator initialized")
	}
	if cfg.Subagents.AgenticEnabled {
		t.Fatalf("agentic delegation should remain disabled by default")
	}
	if app.Coordinator == nil {
		t.Fatalf("main coordinator should remain initialized")
	}
	if _, err := app.Subagents.Get("security-auditor"); err != nil {
		t.Fatalf("expected builtin security-auditor loaded")
	}
}

func TestAppWorksWhenSubagentsDisabled(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Subagents.Enabled = false
	app := New(context.Background(), cfg)
	defer app.Close()

	if app.Coordinator == nil {
		t.Fatalf("main coordinator should initialize when subagents are disabled")
	}
	if app.Subagents == nil || app.SubagentCoord == nil {
		t.Fatalf("subagent components should exist for dependency stability")
	}
	if _, err := app.Subagents.Get("security-auditor"); err != nil {
		t.Fatalf("builtin subagents should remain available when file loading is disabled")
	}
}
