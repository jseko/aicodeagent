package agent

import (
	"strings"

	"AICodeAgent/internal/skills"
)

type TaskFeatures struct {
	IsMultiStep    bool
	RequiresDomain bool
	HasStandard    bool
}

type SkillMatcher struct {
	skills []*skills.Skill
}

var skillMatcherStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "can": {}, "do": {}, "for": {}, "how": {}, "i": {}, "in": {}, "is": {}, "it": {}, "like": {}, "of": {}, "or": {}, "that": {}, "the": {}, "this": {}, "to": {}, "use": {}, "when": {}, "with": {}, "x": {},
}

func NewSkillMatcher(skillList []*skills.Skill) *SkillMatcher {
	return &SkillMatcher{skills: append([]*skills.Skill(nil), skillList...)}
}

func (m *SkillMatcher) Match(task string) (*skills.Skill, float64) {
	if m == nil || len(m.skills) == 0 {
		return nil, 0
	}
	features := analyzeTask(task)
	var matched *skills.Skill
	var confidence float64
	for _, skill := range m.skills {
		score := calculateMatch(skill, task, features)
		if score > confidence {
			matched = skill
			confidence = score
		}
	}
	if confidence < 0.7 {
		return nil, confidence
	}
	return matched, confidence
}

func analyzeTask(task string) TaskFeatures {
	return TaskFeatures{
		IsMultiStep:    containsKeywords(task, []string{"then", "after", "next", "然后", "接着", "下一步"}),
		RequiresDomain: containsKeywords(task, []string{"review", "analyze", "optimize", "审查", "分析", "优化"}),
		HasStandard:    containsKeywords(task, []string{"best practice", "standard", "guideline", "规范", "标准", "最佳实践"}),
	}
}

func calculateMatch(skill *skills.Skill, task string, features TaskFeatures) float64 {
	if skill == nil {
		return 0
	}
	task = strings.ToLower(task)
	description := strings.ToLower(skill.Description)
	name := strings.ToLower(skill.Name)

	score := 0.0
	if strings.Contains(task, name) {
		score += 0.5
	}
	for _, token := range strings.Fields(description) {
		token = strings.Trim(token, " .,;:!?\"'()[]{}<>/\\|`~")
		if _, skip := skillMatcherStopWords[token]; skip {
			continue
		}
		if len([]rune(token)) >= 3 && strings.Contains(task, token) {
			score += 0.25
		}
	}
	if features.IsMultiStep {
		score += 0.1
	}
	if features.RequiresDomain {
		score += 0.2
	}
	if features.HasStandard {
		score += 0.1
	}
	if score > 1 {
		return 1
	}
	return score
}

func containsKeywords(text string, keywords []string) bool {
	text = strings.ToLower(text)
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
