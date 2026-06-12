package subagent

import (
	"strings"
	"testing"
)

func TestSubagentContextBuildPromptAndMessages(t *testing.T) {
	sub := testAgent("review", "system prompt")
	sub.Config.Tools = map[string]bool{"view": true, "write": false}
	ctx := NewSubagentContext("parent", sub)
	ctx.AddMessage("assistant", "prior child message")

	prompt := ctx.BuildPrompt("user input")
	if !strings.HasPrefix(prompt, "system prompt\n\n") {
		t.Fatalf("system prompt should be first: %q", prompt)
	}
	if len(ctx.History) != 1 {
		t.Fatalf("expected isolated child history")
	}
	if len(ctx.Tools) != 1 || ctx.Tools[0] != "view" {
		t.Fatalf("expected enabled tools only: %#v", ctx.Tools)
	}

	messages := ctx.BuildMessages("user input")
	if messages[0].Role != "system" || messages[0].Content != "system prompt" {
		t.Fatalf("expected system message first: %#v", messages)
	}
}

func TestSubagentCoordinatorSwitchRestoreAndCurrent(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testAgent("review", "prompt")); err != nil {
		t.Fatalf("register: %v", err)
	}
	coord := NewCoordinator(registry)
	ctx, err := coord.SwitchRole("parent", "review")
	if err != nil {
		t.Fatalf("switch role: %v", err)
	}
	current, currentCtx := coord.Current()
	if current == nil || current.Config.Name != "review" || currentCtx.SessionID != ctx.SessionID {
		t.Fatalf("unexpected current state")
	}
	if err := coord.RestoreMainAgent(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	current, currentCtx = coord.Current()
	if current != nil || currentCtx != nil {
		t.Fatalf("expected main agent restored")
	}
}

func TestResumeSessionRules(t *testing.T) {
	coord := NewCoordinator(NewRegistry())
	coord.sessions["running"] = &SubagentSession{ID: "running", Status: SessionStatusRunning}
	coord.sessions["done"] = &SubagentSession{ID: "done", Status: SessionStatusDone}
	coord.sessions["failed"] = &SubagentSession{ID: "failed", Status: SessionStatusFailed}

	if _, err := coord.ResumeSession("running"); err != nil {
		t.Fatalf("running should resume: %v", err)
	}
	if _, err := coord.ResumeSession("done"); err == nil {
		t.Fatalf("done should not resume")
	}
	if _, err := coord.ResumeSession("failed"); err == nil {
		t.Fatalf("failed should not resume")
	}
}

func TestRecordTokenUsage(t *testing.T) {
	session := &SubagentSession{}
	session.RecordTokenUsage("model-a", 1000, 2000)
	if session.PromptTokens != 1000 || session.CompletionTokens != 2000 {
		t.Fatalf("unexpected token usage: %#v", session)
	}
}
