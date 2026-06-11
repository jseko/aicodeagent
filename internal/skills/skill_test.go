package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillFile(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	path := filepath.Join(dir, SkillFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
	return path
}

func validSkillContent(name string) string {
	return "---\n" +
		"name: " + name + "\n" +
		"description: Test skill\n" +
		"license: MIT\n" +
		"compatibility: AICodeAgent\n" +
		"metadata:\n" +
		"  version: 1.0.0\n" +
		"  author: tester\n" +
		"---\n" +
		"# Instructions\n\nDo the thing.\n\n---\n\nKeep this markdown separator."
}

func TestParseValidSkill(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFile(t, root, "code-review", validSkillContent("code-review"))

	skill, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if skill.Name != "code-review" {
		t.Fatalf("Name = %q", skill.Name)
	}
	if skill.Path != filepath.Dir(path) {
		t.Fatalf("Path = %q, want %q", skill.Path, filepath.Dir(path))
	}
	if !strings.Contains(skill.Instructions, "Keep this markdown separator") {
		t.Fatalf("Instructions should preserve markdown after frontmatter, got %q", skill.Instructions)
	}
	if skill.GetVersion() != "1.0.0" || skill.GetAuthor() != "tester" {
		t.Fatalf("metadata helpers returned version=%q author=%q", skill.GetVersion(), skill.GetAuthor())
	}
	if err := skill.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestParseRejectsMissingFrontmatter(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFile(t, root, "bad-skill", "name: bad-skill\n---\nbody")

	_, err := Parse(path)
	if err == nil || !strings.Contains(err.Error(), "missing yaml frontmatter") {
		t.Fatalf("Parse() error = %v, want missing frontmatter", err)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFile(t, root, "bad-skill", "---\nname: bad-skill\ndescription: Test\nunknown: value\n---\nBody")

	_, err := Parse(path)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("Parse() error = %v, want unknown field error", err)
	}
}

func TestParseRejectsInvalidMetadataType(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFile(t, root, "bad-skill", "---\nname: bad-skill\ndescription: Test\nmetadata:\n  version:\n    nested: value\n---\nBody")

	_, err := Parse(path)
	if err == nil {
		t.Fatalf("Parse() expected metadata type error")
	}
}

func TestValidateRejectsInvalidSkills(t *testing.T) {
	longName := strings.Repeat("a", MaxNameLength+1)
	longDescription := strings.Repeat("d", MaxDescriptionLength+1)
	longCompatibility := strings.Repeat("c", MaxCompatibilityLength+1)

	tests := []struct {
		name  string
		skill *Skill
	}{
		{"missing name", &Skill{Description: "desc", Instructions: "body", Path: "/tmp/foo"}},
		{"long name", &Skill{Name: longName, Description: "desc", Instructions: "body", Path: "/tmp/" + longName}},
		{"invalid name", &Skill{Name: "bad_name", Description: "desc", Instructions: "body", Path: "/tmp/bad_name"}},
		{"missing description", &Skill{Name: "foo", Instructions: "body", Path: "/tmp/foo"}},
		{"long description", &Skill{Name: "foo", Description: longDescription, Instructions: "body", Path: "/tmp/foo"}},
		{"long compatibility", &Skill{Name: "foo", Description: "desc", Compatibility: longCompatibility, Instructions: "body", Path: "/tmp/foo"}},
		{"empty instructions", &Skill{Name: "foo", Description: "desc", Path: "/tmp/foo"}},
		{"directory mismatch", &Skill{Name: "foo", Description: "desc", Instructions: "body", Path: "/tmp/bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.skill.Validate(); err == nil {
				t.Fatalf("Validate() expected error")
			}
		})
	}
}

func TestMetadataHelpersReturnEmptyString(t *testing.T) {
	skill := &Skill{}
	if skill.GetVersion() != "" || skill.GetAuthor() != "" {
		t.Fatalf("metadata helpers should return empty strings")
	}
}
