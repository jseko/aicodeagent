package skills

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	SkillFileName          = "SKILL.md"
	MaxNameLength          = 64
	MaxDescriptionLength   = 1024
	MaxCompatibilityLength = 500
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)

type Skill struct {
	Name                   string            `yaml:"name"`
	Description            string            `yaml:"description"`
	License                string            `yaml:"license,omitempty"`
	Compatibility          string            `yaml:"compatibility,omitempty"`
	Metadata               map[string]string `yaml:"metadata,omitempty"`
	Builtin                bool              `yaml:"builtin,omitempty"`
	UserInvocable          bool              `yaml:"user_invocable,omitempty"`
	DisableModelInvocation bool              `yaml:"disable_model_invocation,omitempty"`
	Instructions           string            `yaml:"-"`
	Path                   string            `yaml:"-"`
}

func (s *Skill) Validate() error {
	if s == nil {
		return fmt.Errorf("skill is nil")
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(s.Name) > MaxNameLength {
		return fmt.Errorf("name exceeds %d characters", MaxNameLength)
	}
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("name must be alphanumeric with hyphens")
	}
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if len(s.Description) > MaxDescriptionLength {
		return fmt.Errorf("description exceeds %d characters", MaxDescriptionLength)
	}
	if len(s.Compatibility) > MaxCompatibilityLength {
		return fmt.Errorf("compatibility exceeds %d characters", MaxCompatibilityLength)
	}
	if strings.TrimSpace(s.Instructions) == "" {
		return fmt.Errorf("instructions are required")
	}
	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if dirName := filepath.Base(s.Path); !strings.EqualFold(dirName, s.Name) {
		return fmt.Errorf("name %q must match directory name %q", s.Name, dirName)
	}
	return nil
}

func (s *Skill) SkillFilePath() string {
	if s == nil || s.Path == "" {
		return ""
	}
	return filepath.Join(s.Path, SkillFileName)
}

func (s *Skill) GetVersion() string {
	if s == nil || s.Metadata == nil {
		return ""
	}
	return s.Metadata["version"]
}

func (s *Skill) GetAuthor() string {
	if s == nil || s.Metadata == nil {
		return ""
	}
	return s.Metadata["author"]
}
