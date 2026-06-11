package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRuleSet(t *testing.T) {
	set := DefaultRuleSet()
	if len(set.Constitution) == 0 || len(set.Coding) == 0 || len(set.Workflow) == 0 {
		t.Fatalf("default rules should include all categories: %+v", set)
	}
}

func TestParseYAML(t *testing.T) {
	data := []byte(`rules:
  - id: no_secret
    name: 禁止硬编码密钥
    description: 使用环境变量
    category: critical
    enabled: true
coding:
  - id: use_tabs
    name: 使用 Tab
    description: gofmt
    enabled: false
`)
	set, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML() error = %v", err)
	}
	if len(set.Constitution) != 1 {
		t.Fatalf("expected 1 constitution rule, got %d", len(set.Constitution))
	}
	if len(set.Coding) != 1 || set.Coding[0].Enabled {
		t.Fatalf("expected disabled coding rule, got %+v", set.Coding)
	}
}

func TestParseMarkdownTaggedBlocks(t *testing.T) {
	set := ParseMarkdown([]byte(`<critical_rules>
1. **READ FIRST**: read before edit
</critical_rules>
<code_conventions>
- Use tabs
</code_conventions>
<workflow>
- Run tests
</workflow>`))
	if len(set.Constitution) != 1 || len(set.Coding) != 1 || len(set.Workflow) != 1 {
		t.Fatalf("unexpected parsed set: %+v", set)
	}
}

func TestMergerPriorityConflict(t *testing.T) {
	m := &Merger{}
	m.Add(RuleSource{Path: "default", Priority: PriorityDefault, Rules: RuleSet{Constitution: []Rule{{ID: "x", Name: "X", Description: "low", Enabled: true}}}})
	m.Add(RuleSource{Path: "local", Priority: PriorityProjectLocal, Rules: RuleSet{Constitution: []Rule{{ID: "x", Name: "X", Description: "high", Enabled: true}}}})

	set, conflicts := m.Merge()
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if got := set.Constitution[0].Description; got != "high" {
		t.Fatalf("expected high priority override, got %q", got)
	}
}

func TestRenderPromptOmitsDisabledRules(t *testing.T) {
	prompt := RenderPrompt(&RuleSet{
		Constitution: []Rule{{ID: "on", Name: "ON", Description: "enabled", Enabled: true}, {ID: "off", Name: "OFF", Description: "disabled", Enabled: false}},
		Coding:       []CodingRule{{ID: "tabs", Name: "Tabs", Description: "use tabs", Enabled: true, Examples: Examples{Good: []string{"go fmt"}, Bad: []string{"spaces"}}}},
	})
	if !strings.Contains(prompt, "<critical_rules>") || !strings.Contains(prompt, "<code_conventions>") {
		t.Fatalf("expected rendered tags, got %s", prompt)
	}
	if strings.Contains(prompt, "disabled") {
		t.Fatalf("disabled rule should be omitted: %s", prompt)
	}
	if !strings.Contains(prompt, "Good:") || !strings.Contains(prompt, "Bad:") {
		t.Fatalf("examples should be rendered: %s", prompt)
	}
}

func TestLoaderDiscoversProjectRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AICODE.md"), []byte(`<critical_rules>
1. **PROJECT RULE**: project description
</critical_rules>`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(dir)
	set, err := loader.LoadRules()
	if err != nil {
		t.Fatalf("LoadRules() error = %v", err)
	}
	found := false
	for _, rule := range set.Constitution {
		if rule.Name == "PROJECT RULE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected project rule, got %+v", set.Constitution)
	}
}

func TestLoaderReloadsWhenRuleFileChanges(t *testing.T) {
	projectDir := t.TempDir()
	writeRuleFile(t, projectDir, ".aicode/rules.yaml", `workflow:
  - id: hot_reload_rule
    name: BEFORE CHANGE
    description: before
    enabled: true
`)

	loader := NewLoader(projectDir)
	set, err := loader.LoadRules()
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}
	assertWorkflowRuleName(t, set.Workflow, "BEFORE CHANGE")

	writeRuleFile(t, projectDir, ".aicode/rules.yaml", `workflow:
  - id: hot_reload_rule
    name: AFTER CHANGE
    description: after
    enabled: true
`)

	set, err = loader.LoadRules()
	if err != nil {
		t.Fatalf("reload rules: %v", err)
	}
	assertWorkflowRuleName(t, set.Workflow, "AFTER CHANGE")
}

func TestLoaderDiscoversUserRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeRuleFile(t, home, ".aicode/rules.yaml", `constitution:
  - id: user_yaml_rule
    name: USER YAML RULE
    description: from user yaml
    category: workflow
    enabled: true
`)
	writeRuleFile(t, home, ".aicode/rules/user.md", `<workflow>
1. **USER MD RULE**: from user markdown
</workflow>`)
	writeRuleFile(t, home, ".aicode/rules/user.mdc", `<workflow>
1. **USER MDC RULE**: from user mdc
</workflow>`)
	writeRuleFile(t, home, ".aicode/AGENTS.md", `<critical_rules>
1. **USER AGENTS RULE**: from user agents
</critical_rules>`)

	loader := NewLoader(t.TempDir())
	set, err := loader.LoadRules()
	if err != nil {
		t.Fatalf("LoadRules() error = %v", err)
	}

	assertRuleName(t, set.Constitution, "USER AGENTS RULE")
	assertRuleName(t, set.Constitution, "USER YAML RULE")
	assertWorkflowRuleName(t, set.Workflow, "USER MD RULE")
	assertWorkflowRuleName(t, set.Workflow, "USER MDC RULE")
}

func TestLoaderDiscoversProjectAgentsAndMdcRules(t *testing.T) {
	dir := t.TempDir()
	writeRuleFile(t, dir, "AGENTS.md", `<critical_rules>
1. **PROJECT AGENTS RULE**: from project agents
</critical_rules>`)
	writeRuleFile(t, dir, ".aicode/rules/project.mdc", `<workflow>
1. **PROJECT MDC RULE**: from project mdc
</workflow>`)

	loader := NewLoader(dir)
	set, err := loader.LoadRules()
	if err != nil {
		t.Fatalf("LoadRules() error = %v", err)
	}

	assertRuleName(t, set.Constitution, "PROJECT AGENTS RULE")
	assertWorkflowRuleName(t, set.Workflow, "PROJECT MDC RULE")
}

func writeRuleFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertRuleName(t *testing.T, rules []Rule, name string) {
	t.Helper()
	for _, rule := range rules {
		if rule.Name == name {
			return
		}
	}
	t.Fatalf("expected rule %q, got %+v", name, rules)
}

func assertWorkflowRuleName(t *testing.T, rules []Rule, name string) {
	t.Helper()
	for _, rule := range rules {
		if rule.Name == name {
			return
		}
	}
	t.Fatalf("expected workflow rule %q, got %+v", name, rules)
}
