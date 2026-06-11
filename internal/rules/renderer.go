package rules

import (
	"fmt"
	"strings"
)

func RenderPrompt(set *RuleSet) string {
	if set == nil {
		return ""
	}
	var b strings.Builder
	writeRules(&b, "critical_rules", set.Constitution)
	writeCodingRules(&b, set.Coding)
	writeRules(&b, "workflow", set.Workflow)
	return strings.TrimSpace(b.String())
}

func writeRules(b *strings.Builder, tag string, rules []Rule) {
	if len(rules) == 0 {
		return
	}
	var enabled []Rule
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	if len(enabled) == 0 {
		return
	}

	fmt.Fprintf(b, "<%s>\n", tag)
	for i, rule := range enabled {
		fmt.Fprintf(b, "%d. **%s**: %s\n", i+1, rule.Name, rule.Description)
	}
	fmt.Fprintf(b, "</%s>\n\n", tag)
}

func writeCodingRules(b *strings.Builder, rules []CodingRule) {
	if len(rules) == 0 {
		return
	}
	var enabled []CodingRule
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	if len(enabled) == 0 {
		return
	}

	b.WriteString("<code_conventions>\n")
	for i, rule := range enabled {
		fmt.Fprintf(b, "%d. **%s**: %s\n", i+1, rule.Name, rule.Description)
		for _, good := range rule.Examples.Good {
			fmt.Fprintf(b, "   Good: %s\n", strings.ReplaceAll(good, "\n", "\\n"))
		}
		for _, bad := range rule.Examples.Bad {
			fmt.Fprintf(b, "   Bad: %s\n", strings.ReplaceAll(bad, "\n", "\\n"))
		}
	}
	b.WriteString("</code_conventions>\n\n")
}
