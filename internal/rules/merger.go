package rules

import (
	"reflect"
	"sort"
)

type Merger struct {
	sources []RuleSource
}

func (m *Merger) Add(source RuleSource) {
	applySource(&source)
	m.sources = append(m.sources, source)
}

func (m *Merger) HasSources() bool {
	return len(m.sources) > 0
}

func (m *Merger) Merge() (*RuleSet, []RuleConflict) {
	sources := append([]RuleSource(nil), m.sources...)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	merged := &RuleSet{}
	conflicts := make([]RuleConflict, 0)
	constitution := make(map[string]Rule)
	coding := make(map[string]CodingRule)
	workflow := make(map[string]Rule)

	for _, source := range sources {
		for _, rule := range source.Rules.Constitution {
			if existing, ok := constitution[rule.ID]; ok && !reflect.DeepEqual(existing, rule) {
				conflicts = append(conflicts, newConflict(rule.ID, existing.Source, source.Path, existing.Priority, source.Priority))
			}
			constitution[rule.ID] = rule
		}
		for _, rule := range source.Rules.Coding {
			if existing, ok := coding[rule.ID]; ok && !reflect.DeepEqual(existing, rule) {
				conflicts = append(conflicts, newConflict(rule.ID, existing.Source, source.Path, existing.Priority, source.Priority))
			}
			coding[rule.ID] = rule
		}
		for _, rule := range source.Rules.Workflow {
			if existing, ok := workflow[rule.ID]; ok && !reflect.DeepEqual(existing, rule) {
				conflicts = append(conflicts, newConflict(rule.ID, existing.Source, source.Path, existing.Priority, source.Priority))
			}
			workflow[rule.ID] = rule
		}
	}

	for _, rule := range constitution {
		merged.Constitution = append(merged.Constitution, rule)
	}
	for _, rule := range coding {
		merged.Coding = append(merged.Coding, rule)
	}
	for _, rule := range workflow {
		merged.Workflow = append(merged.Workflow, rule)
	}
	sortRules(merged)
	return merged, conflicts
}

func applySource(source *RuleSource) {
	for i := range source.Rules.Constitution {
		if source.Rules.Constitution[i].Source == "" || source.Rules.Constitution[i].Source == "markdown" || source.Rules.Constitution[i].Source == "yaml" {
			source.Rules.Constitution[i].Source = source.Path
		}
		if source.Rules.Constitution[i].Priority == 0 {
			source.Rules.Constitution[i].Priority = source.Priority
		}
	}
	for i := range source.Rules.Coding {
		if source.Rules.Coding[i].Source == "" || source.Rules.Coding[i].Source == "markdown" || source.Rules.Coding[i].Source == "yaml" {
			source.Rules.Coding[i].Source = source.Path
		}
		if source.Rules.Coding[i].Priority == 0 {
			source.Rules.Coding[i].Priority = source.Priority
		}
	}
	for i := range source.Rules.Workflow {
		if source.Rules.Workflow[i].Source == "" || source.Rules.Workflow[i].Source == "markdown" || source.Rules.Workflow[i].Source == "yaml" {
			source.Rules.Workflow[i].Source = source.Path
		}
		if source.Rules.Workflow[i].Priority == 0 {
			source.Rules.Workflow[i].Priority = source.Priority
		}
	}
}

func newConflict(ruleID, low, high string, lowNum, highNum RulePriority) RuleConflict {
	return RuleConflict{RuleID: ruleID, LowPriority: low, HighPriority: high, LowPriorityNum: lowNum, HighPriorityNum: highNum}
}

func sortRules(set *RuleSet) {
	sort.Slice(set.Constitution, func(i, j int) bool { return set.Constitution[i].ID < set.Constitution[j].ID })
	sort.Slice(set.Coding, func(i, j int) bool { return set.Coding[i].ID < set.Coding[j].ID })
	sort.Slice(set.Workflow, func(i, j int) bool { return set.Workflow[i].ID < set.Workflow[j].ID })
}
