package skills

import (
	"strings"
	"testing"
)

func TestToPromptXML(t *testing.T) {
	skills := []*Skill{
		{
			Name:        "code-review",
			Description: "Review & check <code> \"safely\"",
			Path:        "/tmp/code-review",
		},
	}

	xml := ToPromptXML(skills)
	for _, want := range []string{
		"<available_skills>",
		"<name>code-review</name>",
		"Review &amp; check &lt;code&gt; &#34;safely&#34;",
		"<location>/tmp/code-review/SKILL.md</location>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("ToPromptXML() missing %q in:\n%s", want, xml)
		}
	}
}

func TestToPromptXMLEmpty(t *testing.T) {
	if got := ToPromptXML(nil); got != "" {
		t.Fatalf("ToPromptXML(nil) = %q, want empty", got)
	}
}
