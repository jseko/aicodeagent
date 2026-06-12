package subagent

import (
	"strings"
	"testing"
)

func TestBuildDescription(t *testing.T) {
	desc := BuildDescription("Reviewer", "security", []string{"review code"}, []string{"Review this"}, []string{"write files"})
	for _, want := range []string{"Reviewer specializing in security", "Use when:", "Examples:", "Avoid:"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing %q: %s", want, desc)
		}
	}
}

func TestMatchSubagentHighConfidence(t *testing.T) {
	agents := []*Subagent{testAgentWithDescription("review", BuildDescription(
		"Reviewer",
		"security",
		[]string{"review security code"},
		[]string{"review security"},
		nil,
	))}
	result := MatchSubagent("review security code", agents, 2)
	if result.Subagent == nil || result.Subagent.Config.Name != "review" {
		t.Fatalf("expected review match, got %#v", result)
	}
	if len(result.Reasons) == 0 {
		t.Fatalf("expected explainable reasons")
	}
}

func TestMatchSubagentLowConfidenceFallback(t *testing.T) {
	agents := []*Subagent{testAgentWithDescription("review", "Use when: review code")}
	result := MatchSubagent("hello world", agents, 3)
	if result.Subagent != nil {
		t.Fatalf("expected fallback, got %#v", result.Subagent)
	}
}

func TestSelectDepth(t *testing.T) {
	if got := SelectDepth("find this function"); got != DepthQuick {
		t.Fatalf("expected quick, got %s", got)
	}
	if got := SelectDepth("trace call flow"); got != DepthMedium {
		t.Fatalf("expected medium, got %s", got)
	}
	if got := SelectDepth("review all files in entire project"); got != DepthVeryThorough {
		t.Fatalf("expected very thorough, got %s", got)
	}
}

func TestSelectModelTier(t *testing.T) {
	if got := SelectModelTier("read file", ""); got != TierLight {
		t.Fatalf("expected light, got %s", got)
	}
	if got := SelectModelTier("refactor package", ""); got != TierMid {
		t.Fatalf("expected mid, got %s", got)
	}
	if got := SelectModelTier("small auth check", "security"); got != TierHigh {
		t.Fatalf("expected high, got %s", got)
	}
}

func TestRecommendMode(t *testing.T) {
	if got := RecommendMode(true, true); got != "manual" {
		t.Fatalf("expected manual, got %s", got)
	}
	if got := RecommendMode(false, true); got != "agentic" {
		t.Fatalf("expected agentic, got %s", got)
	}
	if got := RecommendMode(true, false); got != "agentic" {
		t.Fatalf("expected agentic, got %s", got)
	}
}

func testAgentWithDescription(name, desc string) *Subagent {
	agent := testAgent(name, "prompt")
	agent.Config.Description = desc
	return agent
}
