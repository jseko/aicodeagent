package skills

import (
	"sort"
	"strings"
)

type SkillSuggestion struct {
	Command     string
	Name        string
	Description string
	Location    string
}

func NewSkillSuggestions(skillList []*Skill) []SkillSuggestion {
	suggestions := make([]SkillSuggestion, 0, len(skillList))
	for _, skill := range skillList {
		if skill == nil || skill.Name == "" {
			continue
		}
		suggestions = append(suggestions, SkillSuggestion{
			Command:     "/" + skill.Name,
			Name:        skill.Name,
			Description: skill.Description,
			Location:    skill.SkillFilePath(),
		})
	}

	sort.SliceStable(suggestions, func(i, j int) bool {
		return suggestions[i].Name < suggestions[j].Name
	})
	return suggestions
}

func FilterSkillSuggestions(suggestions []SkillSuggestion, input string) []SkillSuggestion {
	prefix, ok := slashPrefix(input)
	if !ok {
		return nil
	}
	if prefix == "" {
		return append([]SkillSuggestion(nil), suggestions...)
	}

	filtered := make([]SkillSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if strings.HasPrefix(strings.TrimPrefix(suggestion.Command, "/"), prefix) || strings.HasPrefix(suggestion.Name, prefix) {
			filtered = append(filtered, suggestion)
		}
	}
	return filtered
}

func slashPrefix(input string) (string, bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	if strings.ContainsAny(input, " \t\r\n") {
		return "", false
	}
	return strings.TrimPrefix(input, "/"), true
}
