package agent

import (
	"testing"

	"AICodeAgent/internal/skills"
)

func TestSkillMatcherMatchFindsRelevantSkill(t *testing.T) {
	matcher := NewSkillMatcher([]*skills.Skill{
		{Name: "code-review", Description: "Review and analyze code quality standard"},
		{Name: "write-tests", Description: "Write unit tests"},
	})

	matched, confidence := matcher.Match("please review and analyze this code against our standard")
	if matched == nil {
		t.Fatalf("expected matched skill, confidence %.2f", confidence)
	}
	if matched.Name != "code-review" {
		t.Fatalf("expected code-review, got %s", matched.Name)
	}
	if confidence < 0.7 {
		t.Fatalf("expected high confidence, got %.2f", confidence)
	}
}

func TestSkillMatcherMatchReturnsNilForIrrelevantTask(t *testing.T) {
	matcher := NewSkillMatcher([]*skills.Skill{
		{Name: "code-review", Description: "Review and analyze code quality standard"},
	})

	matched, confidence := matcher.Match("read README.md")
	if matched != nil {
		t.Fatalf("expected no match, got %s", matched.Name)
	}
	if confidence >= 0.7 {
		t.Fatalf("expected low confidence, got %.2f", confidence)
	}
}

func TestSkillMatcherMatchEmptySkills(t *testing.T) {
	matched, confidence := NewSkillMatcher(nil).Match("review code")
	if matched != nil || confidence != 0 {
		t.Fatalf("expected empty result, got %+v %.2f", matched, confidence)
	}
}
