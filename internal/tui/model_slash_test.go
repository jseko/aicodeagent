package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/pubsub"
	"AICodeAgent/internal/skills"
)

func newSlashTestModel(skillSuggestions []skills.SkillSuggestion) Model {
	model := New(&app.App{
		Broker:           pubsub.NewBroker[events.Event](),
		SkillSuggestions: skillSuggestions,
	})
	model.ready = true
	model.width = 100
	model.height = 30
	return model
}

func TestModelSlashSuggestionsOpenAndFilterSkills(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
		{Command: "/commit", Name: "commit", Description: "Commit changes."},
		{Command: "/write-tests", Name: "write-tests", Description: "Write tests."},
	})

	all := model.matchingSlashSuggestions("/")
	if len(all) == 0 {
		t.Fatal("expected suggestions for slash")
	}

	matches := model.matchingSlashSuggestions("/co")
	if len(matches) != 2 {
		t.Fatalf("expected 2 skill suggestions for /co, got %+v", matches)
	}
	if matches[0].Command != "/code-review" || matches[1].Command != "/commit" {
		t.Fatalf("unexpected filtered suggestions: %+v", matches)
	}
}

func TestModelSlashSuggestionsIgnoreNonLeadingSlashTokens(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
	})

	cases := []string{"please /co", "src/pkg/file", "/co now"}
	for _, input := range cases {
		if got := model.matchingSlashSuggestions(input); got != nil {
			t.Fatalf("expected no suggestions for %q, got %+v", input, got)
		}
	}
}

func TestModelSlashSuggestionNavigationAndAccept(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
		{Command: "/commit", Name: "commit", Description: "Commit changes."},
	})
	model.input = "/co"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.slashSuggestionIndex != 1 {
		t.Fatalf("expected highlighted index 1, got %d", model.slashSuggestionIndex)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if model.input != "/commit " {
		t.Fatalf("expected accepted command text, got %q", model.input)
	}
	if len(model.matchingSlashSuggestions(model.input)) != 0 {
		t.Fatal("expected accepted suggestion to hide menu")
	}
}

func TestModelSlashSuggestionEnterAcceptsBeforeSubmit(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
	})
	model.input = "/code"

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no submit command while accepting suggestion")
	}
	if model.input != "/code-review " {
		t.Fatalf("expected accepted skill command, got %q", model.input)
	}
	if model.isLoading {
		t.Fatal("expected accepting suggestion not to submit")
	}
}

func TestModelSlashSuggestionEscapeDismisses(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
	})
	model.input = "/co"

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected escape to hide menu without quitting")
	}
	if model.input != "/co" {
		t.Fatalf("expected input preserved, got %q", model.input)
	}
	if got := model.matchingSlashSuggestions(model.input); got != nil {
		t.Fatalf("expected hidden suggestions, got %+v", got)
	}
}

func TestModelSlashSuggestionEnterSubmitsWhenInactive(t *testing.T) {
	model := newSlashTestModel(nil)
	model.input = "hello"

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	if !model.isLoading {
		t.Fatal("expected model to enter loading state")
	}
}

func TestModelSlashSuggestionRenderIncludesCommandDescriptionAndHighlight(t *testing.T) {
	model := newSlashTestModel([]skills.SkillSuggestion{
		{Command: "/code-review", Name: "code-review", Description: "Review code."},
	})
	model.input = "/co"

	view := model.View()
	for _, want := range []string{"Slash suggestions:", "/code-review", "Review code.", "(Tab/Enter)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestModelSlashSuggestionVisibleCountBounded(t *testing.T) {
	suggestions := make([]skills.SkillSuggestion, 0, maxVisibleSlashSuggestions+2)
	for _, command := range []string{"/s0", "/s1", "/s2", "/s3", "/s4", "/s5", "/s6", "/s7", "/s8", "/s9"} {
		suggestions = append(suggestions, skills.SkillSuggestion{Command: command, Name: strings.TrimPrefix(command, "/")})
	}
	model := newSlashTestModel(suggestions)

	matches := model.matchingSlashSuggestions("/s")
	if len(matches) != maxVisibleSlashSuggestions {
		t.Fatalf("expected %d visible suggestions, got %d", maxVisibleSlashSuggestions, len(matches))
	}
}

func TestModelSpaceInput(t *testing.T) {
	model := newSlashTestModel(nil)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t', 'h', 'e', 'r', 'e'}})
	model = updated.(Model)

	if model.input != "hi there" {
		t.Fatalf("expected space in input, got %q", model.input)
	}
}

func TestModelCursorEditing(t *testing.T) {
	model := newSlashTestModel(nil)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("helo")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(Model)

	if model.input != "hello" {
		t.Fatalf("expected middle insertion, got %q", model.input)
	}
	if model.cursor != 4 {
		t.Fatalf("expected cursor after inserted rune, got %d", model.cursor)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.input != "helo" {
		t.Fatalf("expected backspace before cursor, got %q", model.input)
	}
}

func TestModelCursorEditingWithUnicode(t *testing.T) {
	model := newSlashTestModel(nil)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("你我")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'和'}})
	model = updated.(Model)

	if model.input != "你和我" {
		t.Fatalf("expected unicode middle insertion, got %q", model.input)
	}
	if model.cursor != 2 {
		t.Fatalf("expected unicode rune cursor, got %d", model.cursor)
	}
}

func TestModelRenderedInputCursorFollowsMovement(t *testing.T) {
	model := newSlashTestModel(nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abcd")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)

	got := model.renderedInputLine()
	if strings.Contains(got, "_") {
		t.Fatalf("expected rendered cursor not to insert placeholder, got %q", got)
	}
	if !strings.Contains(got, "abcd") {
		t.Fatalf("expected rendered input text to remain intact, got %q", got)
	}
}
