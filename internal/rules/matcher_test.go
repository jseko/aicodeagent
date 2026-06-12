package rules

import (
	"testing"
)

// matcherTest 定义匹配测试用例结构
type matcherTest struct {
	name     string
	rules    []Rule
	matchID  string
	wantName string
	wantOK   bool
}

func TestMatcher_ExactMatch(t *testing.T) {
	tests := []matcherTest{
		{
			name:     "ExactMatch_ByID",
			rules:    []Rule{{ID: "no_secret", Name: "禁止硬编码密钥", Enabled: true}},
			matchID:  "no_secret",
			wantName: "禁止硬编码密钥",
			wantOK:   true,
		},
		{
			name:     "ExactMatch_DifferentID",
			rules:    []Rule{{ID: "no_secret", Name: "禁止硬编码密钥", Enabled: true}},
			matchID:  "use_tabs",
			wantOK:   false,
		},
		{
			name: "ExactMatch_AmongMany",
			rules: []Rule{
				{ID: "r1", Name: "Rule 1", Enabled: true},
				{ID: "r2", Name: "Rule 2", Enabled: true},
				{ID: "r3", Name: "Rule 3", Enabled: true},
			},
			matchID:  "r2",
			wantName: "Rule 2",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findRuleByID(tt.rules, tt.matchID)
			if tt.wantOK && got == nil {
				t.Fatalf("expected to find rule %q", tt.matchID)
			}
			if !tt.wantOK && got != nil {
				t.Fatalf("expected no match for %q, got %+v", tt.matchID, got)
			}
			if tt.wantOK && got.Name != tt.wantName {
				t.Errorf("rule name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestMatcher_RegexMatch(t *testing.T) {
	tests := []struct {
		name    string
		rules   []Rule
		pattern string
		want    []string
	}{
		{
			name: "RegexMatch_ByPrefix",
			rules: []Rule{
				{ID: "security_no_secret", Name: "No Secret", Enabled: true},
				{ID: "security_no_upload", Name: "No Upload", Enabled: true},
				{ID: "style_tabs", Name: "Tabs", Enabled: true},
			},
			pattern: "security_",
			want:    []string{"security_no_secret", "security_no_upload"},
		},
		{
			name: "RegexMatch_NoMatch",
			rules: []Rule{
				{ID: "style_tabs", Name: "Tabs", Enabled: true},
			},
			pattern: "security_",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findRulesByPrefix(tt.rules, tt.pattern)
			if len(got) != len(tt.want) {
				t.Fatalf("found %d rules, want %d", len(got), len(tt.want))
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("rule[%d].ID = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

func TestMatcher_DisabledRuleNotMatched(t *testing.T) {
	rules := []Rule{
		{ID: "enabled_rule", Name: "Enabled", Enabled: true},
		{ID: "disabled_rule", Name: "Disabled", Enabled: false},
	}

	enabled := filterEnabled(rules)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled rule, got %d", len(enabled))
	}
	if enabled[0].ID != "enabled_rule" {
		t.Errorf("got %q, want enabled_rule", enabled[0].ID)
	}
}

func TestMatcher_EmptyRules(t *testing.T) {
	tests := []struct {
		name  string
		rules []Rule
	}{
		{"NilSlice", nil},
		{"EmptySlice", []Rule{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findRuleByID(tt.rules, "any")
			if got != nil {
				t.Errorf("expected nil for %s, got %+v", tt.name, got)
			}
			filtered := filterEnabled(tt.rules)
			if len(filtered) != 0 {
				t.Errorf("expected 0 filtered rules for %s, got %d", tt.name, len(filtered))
			}
		})
	}
}

// findRuleByID 按 ID 精确查找规则
func findRuleByID(rules []Rule, id string) *Rule {
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i]
		}
	}
	return nil
}

// findRulesByPrefix 按 ID 前缀查找规则
func findRulesByPrefix(rules []Rule, prefix string) []Rule {
	var result []Rule
	for _, r := range rules {
		if len(r.ID) >= len(prefix) && r.ID[:len(prefix)] == prefix {
			result = append(result, r)
		}
	}
	return result
}

// filterEnabled 过滤启用的规则
func filterEnabled(rules []Rule) []Rule {
	var result []Rule
	for _, r := range rules {
		if r.Enabled {
			result = append(result, r)
		}
	}
	return result
}
