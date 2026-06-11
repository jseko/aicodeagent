package rules

type RulePriority int

const (
	PriorityDefault      RulePriority = 10
	PriorityUserGlobal   RulePriority = 50
	PriorityUserCustom   RulePriority = 60
	PriorityProjectRoot  RulePriority = 70
	PriorityProjectDir   RulePriority = 80
	PriorityProjectYAML  RulePriority = 90
	PriorityProjectLocal RulePriority = 100
)

type RuleSet struct {
	Constitution []Rule       `yaml:"constitution"`
	Coding       []CodingRule `yaml:"coding"`
	Workflow     []Rule       `yaml:"workflow"`
}

type Rule struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Category    string       `yaml:"category"`
	Enabled     bool         `yaml:"enabled"`
	Priority    RulePriority `yaml:"priority"`
	Source      string       `yaml:"source"`
}

type CodingRule struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Category    string       `yaml:"category"`
	Enabled     bool         `yaml:"enabled"`
	Priority    RulePriority `yaml:"priority"`
	Source      string       `yaml:"source"`
	Examples    Examples     `yaml:"examples"`
}

type Examples struct {
	Good []string `yaml:"good"`
	Bad  []string `yaml:"bad"`
}

type RuleSource struct {
	Path     string
	Priority RulePriority
	Rules    RuleSet
}

type RuleConflict struct {
	RuleID         string
	LowPriority    string
	HighPriority   string
	LowPriorityNum RulePriority
	HighPriorityNum RulePriority
}

func (r Rule) enabled() bool {
	return r.Enabled
}

func (r CodingRule) enabled() bool {
	return r.Enabled
}
