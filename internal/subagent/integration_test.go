package subagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSkillsStub(t *testing.T) {
	registry := NewRegistry()
	agent := testAgent("reviewer", "prompt")
	agent.Config.Skills = []string{"review"}
	if err := registry.Register(agent); err != nil {
		t.Fatalf("register: %v", err)
	}
	loaded, err := LoadSkills(registry, nil, "reviewer")
	if err != nil {
		t.Fatalf("load skills: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil skills from stub")
	}
}

func TestRecordAudit(t *testing.T) {
	recorder := NewAuditRecorder(failAuditStore{})
	if err := recorder.RecordAudit(AuditLog{ID: "1"}); err == nil {
		t.Fatalf("expected audit write failure")
	}

	store := NewMemoryAuditStore()
	recorder = NewAuditRecorder(store)
	if err := recorder.RecordAudit(AuditLog{Subagent: "review", Tool: "view", Action: PermissionDecisionAllowed}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	if len(store.List()) != 1 {
		t.Fatalf("expected one audit log")
	}
}

func TestTracerReport(t *testing.T) {
	tracer := NewExecutionTracer("review", "parent", "child")
	tracer.RecordToolCall("view", time.Millisecond, true, nil)
	tracer.RecordTokenUsage(10, 20, 0.01)
	report := tracer.Finish()
	if report.ToolCallCount != 1 || report.PromptTokens != 10 || report.CompletionTokens != 20 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestToolCacheExpire(t *testing.T) {
	cache := NewToolCache(time.Millisecond, "view")
	if ok := cache.Set("write", "k", "v", time.Second); ok {
		t.Fatalf("write tool should not be cached")
	}
	if ok := cache.Set("view", "k", "v", time.Millisecond); !ok {
		t.Fatalf("read tool should be cached")
	}
	if value, ok := cache.Get("view", "k"); !ok || value != "v" {
		t.Fatalf("expected cache hit")
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := cache.Get("view", "k"); ok {
		t.Fatalf("expected cache expiry")
	}
}

func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	loader := &SubagentLoader{ProjectDir: dir, Registry: NewRegistry()}
	reloader := NewHotReloadableLoader(loader, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- reloader.WatchHotReload(ctx) }()

	writeSubagentFile(t, dir, "hot.md", "hot", "hot prompt")
	waitHotEvent(t, reloader.Events, HotReloadCreated)

	writeSubagentFile(t, dir, "hot.md", "hot", "updated prompt")
	waitHotEvent(t, reloader.Events, HotReloadUpdated)

	if err := os.Remove(filepath.Join(dir, "hot.md")); err != nil {
		t.Fatalf("remove hot file: %v", err)
	}
	waitHotEvent(t, reloader.Events, HotReloadRemoved)

	cancel()
	if err := <-done; err == nil {
		t.Fatalf("expected context cancellation")
	}
}

type failAuditStore struct{}

func (failAuditStore) Save(log AuditLog) error { return fmt.Errorf("fail") }

func waitHotEvent(t *testing.T, events <-chan HotReloadEvent, want HotReloadEventType) {
	t.Helper()
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case event := <-events:
			if event.Type == want {
				return
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s", want)
		}
	}
}
