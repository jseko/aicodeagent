package agent

import (
	"strings"
	"testing"
)

func TestBuildSummaryPromptContainsRequiredSections(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "帮我写一个测试"},
		{Role: RoleAssistant, Content: "好的，我来编写测试代码", Status: MessageStatusCompleted},
	}

	prompt := buildSummaryPrompt(messages)

	requiredSections := []string{
		"## Work Completed",
		"## Files Modified",
		"## Current State",
		"## Key Decisions",
		"## Next Steps",
		"## Todo Status",
	}
	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("summary prompt missing section %q:\n%s", section, prompt)
		}
	}
}

func TestBuildSummaryPromptIncludesMessageContent(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "修复登录bug"},
		{Role: RoleAssistant, Content: "已修复，使用了OAuth2方案", Status: MessageStatusCompleted},
	}

	prompt := buildSummaryPrompt(messages)

	if !strings.Contains(prompt, "修复登录bug") {
		t.Error("summary should include user message content")
	}
	if !strings.Contains(prompt, "已修复，使用了OAuth2方案") {
		t.Error("summary should include assistant message content")
	}
}

func TestBuildSummaryPromptFiltersCancelledMessages(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "有用的消息"},
		{Role: RoleAssistant, Content: "被取消的回复", Status: MessageStatusCancelled},
		{Role: RoleAssistant, Content: "实际回复", Status: MessageStatusCompleted},
	}

	prompt := buildSummaryPrompt(messages)

	if strings.Contains(prompt, "被取消的回复") {
		t.Error("summary should not include cancelled messages")
	}
	if !strings.Contains(prompt, "有用的消息") {
		t.Error("summary should include non-cancelled messages")
	}
	if !strings.Contains(prompt, "实际回复") {
		t.Error("summary should include completed messages")
	}
}

func TestBuildSummaryPromptFiltersSummaryMessages(t *testing.T) {
	messages := []Message{
		{ID: "sum-1", Role: RoleAssistant, Content: "previous summary", IsSummaryMessage: true},
		{Role: RoleUser, Content: "新消息"},
	}

	prompt := buildSummaryPrompt(messages)

	if strings.Contains(prompt, "previous summary") {
		t.Error("summary should not include previous summary messages")
	}
	if !strings.Contains(prompt, "新消息") {
		t.Error("summary should include regular messages")
	}
}

func TestBuildSummaryPromptFiltersEmptyMessages(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: ""},
		{Role: RoleAssistant, Content: "actual content", Status: MessageStatusCompleted},
	}

	prompt := buildSummaryPrompt(messages)

	if !strings.Contains(prompt, "actual content") {
		t.Error("summary should include non-empty messages")
	}
}

func TestBuildSummaryPromptEmptyMessages(t *testing.T) {
	prompt := buildSummaryPrompt(nil)

	for _, section := range []string{
		"## Work Completed",
		"## Files Modified",
	} {
		if !strings.Contains(prompt, section) {
			t.Errorf("empty summary prompt should still contain %q", section)
		}
	}
}

func TestValidateSummaryContentRequiresAllSections(t *testing.T) {
	content := strings.Join([]string{
		"## Work Completed",
		"done",
		"## Files Modified",
		"files",
		"## Current State",
		"state",
		"## Key Decisions",
		"decisions",
		"## Next Steps",
		"next",
		"## Todo Status",
		"todos",
	}, "\n")

	if err := validateSummaryContent(content); err != nil {
		t.Fatalf("validateSummaryContent should accept complete summary: %v", err)
	}
}

func TestValidateSummaryContentRejectsMissingSection(t *testing.T) {
	content := "## Work Completed\ndone\n## Files Modified\nfiles"
	if err := validateSummaryContent(content); err == nil {
		t.Fatal("validateSummaryContent should reject incomplete summary")
	}
}

func TestValidateSummaryContentRejectsEmptyContent(t *testing.T) {
	if err := validateSummaryContent("   "); err == nil {
		t.Fatal("validateSummaryContent should reject empty summary")
	}
}

func TestBuildSummaryPromptRoleFormatting(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi", Status: MessageStatusCompleted},
		{Role: RoleTool, Content: "tool result", ToolCallID: "call-1"},
	}

	prompt := buildSummaryPrompt(messages)

	if !strings.Contains(prompt, "USER") {
		t.Error("summary should contain USER role header")
	}
	if !strings.Contains(prompt, "ASSISTANT") {
		t.Error("summary should contain ASSISTANT role header")
	}
	if !strings.Contains(prompt, "TOOL") {
		t.Error("summary should contain TOOL role header")
	}
}
