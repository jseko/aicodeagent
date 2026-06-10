package agent

import (
	"testing"

	"AICodeAgent/internal/config"
	"AICodeAgent/internal/llm"
)

func TestUpdateSessionUsageAccumulatesTokensAndCost(t *testing.T) {
	agent := &sessionAgent{}
	model := &Model{Config: ModelConfig{InputPer1M: 30, OutputPer1M: 60}}
	session := &Session{}

	agent.updateSessionUsage(model, session, &llm.Usage{PromptTokens: 1000, CompletionTokens: 200})
	agent.updateSessionUsage(model, session, &llm.Usage{PromptTokens: 500, CompletionTokens: 100})

	if session.PromptTokens != 1500 {
		t.Fatalf("PromptTokens = %d, want 1500", session.PromptTokens)
	}
	if session.CompletionTokens != 300 {
		t.Fatalf("CompletionTokens = %d, want 300", session.CompletionTokens)
	}
	if session.TotalPromptTokens != 1500 {
		t.Fatalf("TotalPromptTokens = %d, want 1500", session.TotalPromptTokens)
	}
	if session.TotalCompletionTokens != 300 {
		t.Fatalf("TotalCompletionTokens = %d, want 300", session.TotalCompletionTokens)
	}
	wantCost := float64(1500)*30/1_000_000 + float64(300)*60/1_000_000
	if session.TotalCost != wantCost {
		t.Fatalf("TotalCost = %f, want %f", session.TotalCost, wantCost)
	}
}

func TestCalculateUsageCostUnknownPricingReturnsZero(t *testing.T) {
	cost := calculateUsageCost(&Model{}, &llm.Usage{PromptTokens: 1000, CompletionTokens: 1000})
	if cost != 0 {
		t.Fatalf("cost = %f, want 0", cost)
	}
}

func TestCheckContextWindowUsesHybridThreshold(t *testing.T) {
	c := &coordinator{config: &config.Config{Context: config.ContextConfig{WindowTokens: 128000, SmallWindowRatio: 0.2}}}
	c.largeModel.Store(&Model{Config: ModelConfig{MaxTokens: 128000}})

	stop := c.checkContextWindow(&Session{PromptTokens: 102401})
	if stop == nil {
		t.Fatal("expected stop condition")
	}
	if stop.Reason != "context_window_overflow" {
		t.Fatalf("reason = %q", stop.Reason)
	}

	noStop := c.checkContextWindow(&Session{PromptTokens: 90000})
	if noStop != nil {
		t.Fatalf("unexpected stop condition: %+v", noStop)
	}
}

func TestCheckContextWindowUsesLargeWindowBuffer(t *testing.T) {
	c := &coordinator{config: &config.Config{Context: config.ContextConfig{WindowTokens: 300000, LargeWindowBuffer: 20000}}}
	c.largeModel.Store(&Model{Config: ModelConfig{MaxTokens: 300000}})

	stop := c.checkContextWindow(&Session{PromptTokens: 280001})
	if stop == nil {
		t.Fatal("expected stop condition")
	}
	if stop.Threshold != 20000 {
		t.Fatalf("threshold = %d, want 20000", stop.Threshold)
	}
}
