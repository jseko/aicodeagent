package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultExternalTimeout = 10 * time.Second

type Config struct {
	Enabled bool                 `yaml:"enabled" json:"enabled"`
	Files   []string             `yaml:"files" json:"files"`
	Hooks   []ExternalHookConfig `yaml:"hooks" json:"hooks"`
}

type ExternalHookConfig struct {
	Name     string                 `yaml:"name" json:"name"`
	Types    []EventType            `yaml:"types" json:"types"`
	Command  string                 `yaml:"command" json:"command"`
	Args     []string               `yaml:"args" json:"args"`
	Matcher  string                 `yaml:"matcher" json:"matcher"`
	Timeout  time.Duration          `yaml:"timeout" json:"timeout"`
	Priority Priority               `yaml:"priority" json:"priority"`
	Config   map[string]interface{} `yaml:"config" json:"config"`
	matcher  *regexp.Regexp
}

func (c *ExternalHookConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("external hook name is required")
	}
	if len(c.Types) == 0 {
		return fmt.Errorf("external hook %s requires event types", c.Name)
	}
	if c.Command == "" {
		return fmt.Errorf("external hook %s command is required", c.Name)
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultExternalTimeout
	}
	if c.Matcher != "" {
		matcher, err := regexp.Compile(c.Matcher)
		if err != nil {
			return fmt.Errorf("external hook %s invalid matcher: %w", c.Name, err)
		}
		c.matcher = matcher
	}
	return nil
}

func (c *ExternalHookConfig) MatchTool(toolName string) bool {
	if c.Matcher == "" {
		return true
	}
	if c.matcher == nil {
		matcher, err := regexp.Compile(c.Matcher)
		if err != nil {
			return false
		}
		c.matcher = matcher
	}
	return c.matcher.MatchString(toolName)
}

func LoadConfigFiles(paths []string) ([]ExternalHookConfig, error) {
	var configs []ExternalHookConfig
	for _, path := range paths {
		loaded, err := LoadConfigFile(path)
		if err != nil {
			return nil, err
		}
		configs = append(configs, loaded...)
	}
	return configs, nil
}

func LoadConfigFile(path string) ([]ExternalHookConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Hooks) == 0 {
		var hook ExternalHookConfig
		if err := yaml.Unmarshal(data, &hook); err != nil {
			return nil, err
		}
		if hook.Name != "" {
			cfg.Hooks = []ExternalHookConfig{hook}
		}
	}
	return ValidateExternalHooks(cfg.Hooks)
}

func ValidateExternalHooks(configs []ExternalHookConfig) ([]ExternalHookConfig, error) {
	validated := make([]ExternalHookConfig, 0, len(configs))
	for _, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		validated = append(validated, cfg)
	}
	return validated, nil
}

func (c ExternalHookConfig) MarshalJSON() ([]byte, error) {
	type alias ExternalHookConfig
	return json.Marshal(struct {
		Timeout string `json:"timeout"`
		alias
	}{Timeout: c.Timeout.String(), alias: alias(c)})
}
