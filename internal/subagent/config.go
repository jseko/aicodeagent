package subagent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ModeSubagent = "subagent"
)

type SubagentConfig struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Mode        string            `yaml:"mode"`
	Model       string            `yaml:"model"`
	Tools       map[string]bool   `yaml:"tools"`
	Skills      []string          `yaml:"skills"`
	Permissions *PermissionConfig `yaml:"permissions"`
}

type Subagent struct {
	Config       *SubagentConfig
	SystemPrompt string
	Path         string
}

func (s *Subagent) Name() string {
	if s == nil || s.Config == nil {
		return ""
	}
	return s.Config.Name
}

func (s *Subagent) Description() string {
	if s == nil || s.Config == nil {
		return ""
	}
	return s.Config.Description
}

func LoadSubagentConfig(path string) (*SubagentConfig, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}

	head, body, err := splitFrontMatter(raw)
	if err != nil {
		return nil, "", err
	}

	var cfg SubagentConfig
	if err := yaml.Unmarshal(head, &cfg); err != nil {
		return nil, "", fmt.Errorf("parse yaml: %w", err)
	}

	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	normalizeConfig(&cfg)
	if err := ValidateSubagentConfig(&cfg); err != nil {
		return nil, "", err
	}

	prompt := strings.TrimSpace(string(body))
	if prompt == "" {
		return nil, "", fmt.Errorf("system prompt required")
	}
	return &cfg, prompt, nil
}

func CreateSubagent(mode, name, desc, model string, tpl []byte) ([]byte, error) {
	var cfg SubagentConfig
	switch mode {
	case "interactive":
		cfg = SubagentConfig{
			Name:        name,
			Description: desc,
			Mode:        ModeSubagent,
			Model:       model,
			Tools:       map[string]bool{"view": true},
			Permissions: &PermissionConfig{Level: PermissionLevelStandard},
		}
	case "template":
		if err := yaml.Unmarshal(tpl, &cfg); err != nil {
			return nil, fmt.Errorf("parse template: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	normalizeConfig(&cfg)
	if err := ValidateSubagentConfig(&cfg); err != nil {
		return nil, err
	}

	head, err := yaml.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return bytes.Join([][]byte{[]byte("---\n"), head, []byte("---\n\n# System Prompt\n")}, nil), nil
}

func ValidateSubagentConfig(cfg *SubagentConfig) error {
	if cfg == nil {
		return fmt.Errorf("subagent config required")
	}
	if cfg.Name == "" || cfg.Description == "" || cfg.Model == "" {
		return fmt.Errorf("missing required fields")
	}
	if cfg.Permissions == nil {
		return fmt.Errorf("permissions required")
	}
	if _, err := NormalizePermission(cfg.Permissions); err != nil {
		return err
	}
	return nil
}

func ValidateBestPractice(cfg *SubagentConfig) error {
	if err := ValidateSubagentConfig(cfg); err != nil {
		return err
	}
	perm, err := NormalizePermission(cfg.Permissions)
	if err != nil {
		return err
	}
	for _, tool := range perm.AllowedTools {
		if tool == "write" {
			return fmt.Errorf("write not allowed by default")
		}
	}
	if !strings.Contains(strings.ToLower(cfg.Description), "use when") {
		return fmt.Errorf("description should include Use when")
	}
	return nil
}

func normalizeConfig(cfg *SubagentConfig) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Description = strings.TrimSpace(cfg.Description)
	cfg.Mode = strings.TrimSpace(cfg.Mode)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Mode == "" {
		cfg.Mode = ModeSubagent
	}
}

func splitFrontMatter(raw []byte) ([]byte, []byte, error) {
	raw = bytes.TrimSpace(raw)
	if !bytes.HasPrefix(raw, []byte("---")) {
		return nil, nil, fmt.Errorf("invalid front matter")
	}
	raw = bytes.TrimPrefix(raw, []byte("---"))
	raw = bytes.TrimPrefix(raw, []byte("\r\n"))
	raw = bytes.TrimPrefix(raw, []byte("\n"))
	parts := bytes.SplitN(raw, []byte("\n---"), 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid front matter")
	}
	body := bytes.TrimPrefix(parts[1], []byte("\r\n"))
	body = bytes.TrimPrefix(body, []byte("\n"))
	return parts[0], body, nil
}
