package rules

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type yamlRuleSet struct {
	Rules        []Rule       `yaml:"rules"`
	Constitution []Rule       `yaml:"constitution"`
	Coding       []CodingRule `yaml:"coding"`
	Workflow     []Rule       `yaml:"workflow"`
}

func ParseYAML(data []byte) (*RuleSet, error) {
	var raw yamlRuleSet
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse rules yaml: %w", err)
	}

	set := &RuleSet{
		Constitution: raw.Constitution,
		Coding:       raw.Coding,
		Workflow:     raw.Workflow,
	}
	for _, rule := range raw.Rules {
		switch strings.ToLower(rule.Category) {
		case "critical", "constitution":
			set.Constitution = append(set.Constitution, rule)
		case "workflow":
			set.Workflow = append(set.Workflow, rule)
		default:
			set.Coding = append(set.Coding, CodingRule{
				ID:          rule.ID,
				Name:        rule.Name,
				Description: rule.Description,
				Category:    rule.Category,
				Enabled:     rule.Enabled,
				Priority:    rule.Priority,
				Source:      rule.Source,
			})
		}
	}
	normalizeRuleSet(set, "yaml")
	return set, nil
}

func ParseMarkdown(data []byte) *RuleSet {
	content := strings.TrimSpace(string(data))
	if content == "" {
		return &RuleSet{}
	}

	blocks := extractTaggedBlocks(content)
	set := &RuleSet{}
	if critical := strings.TrimSpace(blocks["critical_rules"]); critical != "" {
		set.Constitution = parseListRules(critical, "markdown", "critical")
	}
	if conventions := strings.TrimSpace(blocks["code_conventions"]); conventions != "" {
		for _, rule := range parseListRules(conventions, "markdown", "style") {
			set.Coding = append(set.Coding, CodingRule{
				ID:          rule.ID,
				Name:        rule.Name,
				Description: rule.Description,
				Category:    rule.Category,
				Enabled:     rule.Enabled,
				Priority:    rule.Priority,
				Source:      rule.Source,
			})
		}
	}
	if workflow := strings.TrimSpace(blocks["workflow"]); workflow != "" {
		set.Workflow = parseListRules(workflow, "markdown", "workflow")
	}
	if len(set.Constitution)+len(set.Coding)+len(set.Workflow) == 0 {
		set.Workflow = []Rule{{ID: "markdown_context", Name: "Markdown Context", Description: content, Category: "workflow", Enabled: true}}
	}
	normalizeRuleSet(set, "markdown")
	return set
}

func extractTaggedBlocks(content string) map[string]string {
	blocks := make(map[string]string)
	for _, tag := range []string{"critical_rules", "code_conventions", "workflow"} {
		re := regexp.MustCompile(`(?s)<` + tag + `>\s*(.*?)\s*</` + tag + `>`)
		if match := re.FindStringSubmatch(content); len(match) == 2 {
			blocks[tag] = match[1]
		}
	}
	return blocks
}

func parseListRules(content, source, category string) []Rule {
	lines := strings.Split(content, "\n")
	rules := make([]Rule, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		line = regexp.MustCompile(`^\d+\.\s*`).ReplaceAllString(line, "")
		if line == "" {
			continue
		}

		name, desc := splitRuleLine(line)
		rules = append(rules, Rule{
			ID:          slug(name),
			Name:        name,
			Description: desc,
			Category:    category,
			Enabled:     true,
			Source:      source,
		})
	}
	return rules
}

func splitRuleLine(line string) (string, string) {
	line = strings.ReplaceAll(line, "**", "")
	if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return line, line
}

func normalizeRuleSet(set *RuleSet, source string) {
	for i := range set.Constitution {
		normalizeRule(&set.Constitution[i], source)
	}
	for i := range set.Coding {
		normalizeCodingRule(&set.Coding[i], source)
	}
	for i := range set.Workflow {
		normalizeRule(&set.Workflow[i], source)
	}
}

func normalizeRule(rule *Rule, source string) {
	if rule.ID == "" {
		rule.ID = slug(rule.Name)
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	if rule.Description == "" {
		rule.Description = rule.Name
	}
	if rule.Source == "" {
		rule.Source = source
	}
	if !rule.Enabled {
		return
	}
}

func normalizeCodingRule(rule *CodingRule, source string) {
	if rule.ID == "" {
		rule.ID = slug(rule.Name)
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	if rule.Description == "" {
		rule.Description = rule.Name
	}
	if rule.Source == "" {
		rule.Source = source
	}
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9\p{Han}]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "rule"
	}
	return s
}
