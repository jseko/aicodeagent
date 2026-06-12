package subagent

import (
	"fmt"
	"sort"
	"strings"
)

type Depth string

const (
	DepthQuick        Depth = "quick"
	DepthMedium       Depth = "medium"
	DepthVeryThorough Depth = "very_thorough"
)

type ModelTier string

const (
	TierLight ModelTier = "light"
	TierMid   ModelTier = "mid"
	TierHigh  ModelTier = "high"
)

type MatchResult struct {
	Subagent  *Subagent
	Score     int
	Threshold int
	Reasons   []string
}

func BuildDescription(role, focus string, useWhen []string, examples []string, avoid []string) string {
	lines := []string{fmt.Sprintf("%s specializing in %s.", strings.TrimSpace(role), strings.TrimSpace(focus))}
	if len(useWhen) > 0 {
		lines = append(lines, "Use when: "+strings.Join(cleanList(useWhen), ", "))
	}
	if len(examples) > 0 {
		lines = append(lines, "Examples: \""+strings.Join(cleanList(examples), "\", \"")+"\"")
	}
	if len(avoid) > 0 {
		lines = append(lines, "Avoid: "+strings.Join(cleanList(avoid), ", "))
	}
	return strings.Join(lines, "\n")
}

func MatchSubagent(input string, agents []*Subagent, threshold int) MatchResult {
	if threshold <= 0 {
		threshold = 2
	}
	queryTerms := meaningfulTerms(input)
	best := MatchResult{Threshold: threshold}
	for _, agent := range agents {
		if agent == nil || agent.Config == nil {
			continue
		}
		desc := strings.ToLower(agent.Config.Description)
		score := 0
		var reasons []string
		for _, term := range queryTerms {
			if strings.Contains(desc, term) {
				score++
				reasons = append(reasons, "matched term: "+term)
			}
		}
		if strings.Contains(desc, "use when") {
			score++
			reasons = append(reasons, "has Use when")
		}
		if strings.Contains(desc, "examples") && score > 0 {
			score++
			reasons = append(reasons, "has Examples")
		}
		if score > best.Score || (score == best.Score && agent.Config.Name < bestName(best.Subagent)) {
			best = MatchResult{Subagent: agent, Score: score, Threshold: threshold, Reasons: reasons}
		}
	}
	if best.Score < threshold {
		best.Subagent = nil
		best.Reasons = append(best.Reasons, "low confidence fallback")
	}
	return best
}

func SelectDepth(task string) Depth {
	s := strings.ToLower(strings.TrimSpace(task))
	if hasAny(s, "entire", "whole", "full", "all files", "everything", "very thorough") {
		return DepthVeryThorough
	}
	if hasAny(s, "trace", "flow", "call chain", "dependency", "impact") || len(s) > 120 {
		return DepthMedium
	}
	return DepthQuick
}

func SelectModelTier(task string, risk string) ModelTier {
	risk = strings.ToLower(strings.TrimSpace(risk))
	if risk == "security" || risk == "architecture" || risk == "compliance" {
		return TierHigh
	}
	task = strings.ToLower(task)
	if hasAny(task, "optimize", "refactor", "migration", "concurrency", "performance") {
		return TierMid
	}
	return TierLight
}

func RecommendMode(familiar bool, taskClear bool) string {
	if familiar && taskClear {
		return "manual"
	}
	return "agentic"
}

func meaningfulTerms(input string) []string {
	stop := map[string]bool{"the": true, "a": true, "an": true, "to": true, "for": true, "and": true, "or": true, "this": true, "that": true, "please": true, "请": true, "帮我": true}
	seen := make(map[string]bool)
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(strings.TrimSpace(input))) {
		term = strings.Trim(term, " .,;:!?\"'`()[]{}，。；：！？")
		if len([]rune(term)) < 2 || stop[term] || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

func cleanList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func hasAny(s string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

func bestName(agent *Subagent) string {
	if agent == nil || agent.Config == nil {
		return "~"
	}
	return agent.Config.Name
}
