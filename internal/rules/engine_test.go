package rules

import (
	"strings"
	"testing"
)

func TestEngine_ActionChainOrdered(t *testing.T) {
	// 验证 Merger 按优先级顺序处理规则（高优先级覆盖低优先级）
	m := &Merger{}

	sources := []RuleSource{
		{Priority: PriorityDefault, Rules: RuleSet{Constitution: []Rule{{ID: "r1", Name: "Default", Description: "desc1", Enabled: true}}}},
		{Priority: PriorityUserGlobal, Rules: RuleSet{Constitution: []Rule{{ID: "r1", Name: "UserGlobal", Description: "desc2", Enabled: true}}}},
		{Priority: PriorityProjectLocal, Rules: RuleSet{Constitution: []Rule{{ID: "r1", Name: "ProjectLocal", Description: "desc3", Enabled: true}}}},
	}

	for _, src := range sources {
		m.Add(src)
	}

	set, conflicts := m.Merge()

	// 最高优先级（ProjectLocal）应该生效
	if len(conflicts) == 0 {
		t.Error("expected conflicts when multiple rules share same ID")
	}
	if set.Constitution[0].Name != "ProjectLocal" {
		t.Errorf("highest priority rule should win, got Name=%q", set.Constitution[0].Name)
	}
}

func TestEngine_StopsOnFirstDisabled(t *testing.T) {
	// 验证规则启用/禁用控制：禁用的规则不出现在渲染输出中
	ruleSet := &RuleSet{
		Constitution: []Rule{
			{ID: "on1", Name: "ON1", Description: "enabled1", Enabled: true},
			{ID: "off1", Name: "OFF1", Description: "disabled1", Enabled: false},
			{ID: "on2", Name: "ON2", Description: "enabled2", Enabled: true},
		},
	}

	prompt := RenderPrompt(ruleSet)

	if !strings.Contains(prompt, "ON1") {
		t.Error("enabled rule ON1 should be in prompt")
	}
	if strings.Contains(prompt, "OFF1") {
		t.Error("disabled rule OFF1 should not be in prompt")
	}
	if !strings.Contains(prompt, "ON2") {
		t.Error("enabled rule ON2 should be in prompt")
	}
}

func TestEngine_PriorityConflictResolution(t *testing.T) {
	// 相同优先级时，后注册的胜出
	m := &Merger{}

	m.Add(RuleSource{
		Path:     "first",
		Priority: PriorityDefault,
		Rules:    RuleSet{Constitution: []Rule{{ID: "dup", Name: "First", Description: "first registered", Enabled: true}}},
	})
	m.Add(RuleSource{
		Path:     "second",
		Priority: PriorityDefault,
		Rules:    RuleSet{Constitution: []Rule{{ID: "dup", Name: "Second", Description: "second registered", Enabled: true}}},
	})

	set, conflicts := m.Merge()

	if len(conflicts) == 0 {
		t.Error("expected conflicts for duplicate ID at same priority")
	}
	if set.Constitution[0].Name != "Second" {
		t.Errorf("later registration should win at same priority, got %q", set.Constitution[0].Name)
	}
}

func TestEngine_MultipleRulesDifferentIDs(t *testing.T) {
	m := &Merger{}
	m.Add(RuleSource{
		Priority: PriorityDefault,
		Rules: RuleSet{
			Constitution: []Rule{
				{ID: "r1", Name: "Rule1", Description: "first", Enabled: true},
				{ID: "r2", Name: "Rule2", Description: "second", Enabled: true},
			},
		},
	})

	set, conflicts := m.Merge()

	if len(conflicts) != 0 {
		t.Errorf("no conflicts expected for different IDs, got %d", len(conflicts))
	}
	if len(set.Constitution) != 2 {
		t.Errorf("expected 2 rules, got %d", len(set.Constitution))
	}
}
