package skills

import "testing"

func TestNewSkillSuggestionsSortsAndOmitsInstructions(t *testing.T) {
	suggestions := NewSkillSuggestions([]*Skill{
		{Name: "write-tests", Description: "Write tests.", Instructions: "full markdown", Path: "/tmp/write-tests"},
		{Name: "code-review", Description: "Review code.", Instructions: "secret", Path: "/tmp/code-review"},
	})

	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Name != "code-review" || suggestions[0].Command != "/code-review" {
		t.Fatalf("expected code-review first, got %+v", suggestions[0])
	}
	if suggestions[0].Description != "Review code." {
		t.Fatalf("expected description, got %q", suggestions[0].Description)
	}
	if suggestions[0].Location != "/tmp/code-review/SKILL.md" {
		t.Fatalf("expected skill file path, got %q", suggestions[0].Location)
	}
}

func TestNewSkillSuggestionsSkipsNilAndEmptyNames(t *testing.T) {
	suggestions := NewSkillSuggestions([]*Skill{
		nil,
		{Name: ""},
		{Name: "commit", Description: "Commit changes."},
	})

	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Command != "/commit" {
		t.Fatalf("expected /commit, got %q", suggestions[0].Command)
	}
}

func TestFilterSkillSuggestions(t *testing.T) {
	suggestions := []SkillSuggestion{
		{Command: "/code-review", Name: "code-review"},
		{Command: "/commit", Name: "commit"},
		{Command: "/write-tests", Name: "write-tests"},
	}

	all := FilterSkillSuggestions(suggestions, "/")
	if len(all) != 3 {
		t.Fatalf("expected all suggestions, got %d", len(all))
	}

	filtered := FilterSkillSuggestions(suggestions, "/co")
	if len(filtered) != 2 || filtered[0].Name != "code-review" || filtered[1].Name != "commit" {
		t.Fatalf("unexpected filtered suggestions: %+v", filtered)
	}

	none := FilterSkillSuggestions(suggestions, "/missing")
	if len(none) != 0 {
		t.Fatalf("expected no suggestions, got %+v", none)
	}
}

func TestFilterSkillSuggestionsRequiresLeadingSlashToken(t *testing.T) {
	suggestions := []SkillSuggestion{{Command: "/code-review", Name: "code-review"}}

	cases := []string{"please /co", "src/pkg/file", "/co now", "/co\nnext"}
	for _, input := range cases {
		if got := FilterSkillSuggestions(suggestions, input); got != nil {
			t.Fatalf("expected nil for %q, got %+v", input, got)
		}
	}
}

func TestFilterSkillSuggestionsEmptyList(t *testing.T) {
	if got := FilterSkillSuggestions(nil, "/"); len(got) != 0 {
		t.Fatalf("expected empty suggestions, got %+v", got)
	}
}
