package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverFindsNestedSkillsAndSkipsInvalid(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, filepath.Join(root, "nested"), "alpha", validSkillContent("alpha"))
	writeSkillFile(t, root, "invalid", "---\nname: invalid\n---\n")

	skills := Discover([]string{root})
	if len(skills) != 1 {
		t.Fatalf("Discover() returned %d skills, want 1", len(skills))
	}
	if skills[0].Name != "alpha" {
		t.Fatalf("skill name = %q, want alpha", skills[0].Name)
	}
}

func TestDiscoverAvoidsDuplicateSkillFiles(t *testing.T) {
	root := t.TempDir()
	skillPath := writeSkillFile(t, root, "alpha", validSkillContent("alpha"))

	skills := Discover([]string{root, filepath.Dir(skillPath)})
	if len(skills) != 1 {
		t.Fatalf("Discover() returned %d skills, want 1", len(skills))
	}
}

func TestDiscoverFollowsSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges on windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	writeSkillFile(t, target, "alpha", validSkillContent("alpha"))
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	skills := Discover([]string{link})
	if len(skills) != 1 || skills[0].Name != "alpha" {
		t.Fatalf("Discover() through symlink returned %#v", skills)
	}
}

func TestManagerRegistersAndListsDeterministically(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root)
	for _, name := range []string{"zeta", "alpha"} {
		path := writeSkillFile(t, root, name, validSkillContent(name))
		skill, err := Parse(path)
		if err != nil {
			t.Fatalf("Parse(%s): %v", name, err)
		}
		if err := mgr.Register(skill); err != nil {
			t.Fatalf("Register(%s): %v", name, err)
		}
	}

	if skill, ok := mgr.Get("alpha"); !ok || skill.Name != "alpha" {
		t.Fatalf("Get(alpha) = %#v, %v", skill, ok)
	}
	list := mgr.List()
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("List() order = %#v", list)
	}
}

func TestManagerLoadPrecedenceLaterPathsOverride(t *testing.T) {
	root := t.TempDir()
	userRoot := filepath.Join(root, "user")
	projectRoot := filepath.Join(root, "project")
	writeSkillFile(t, userRoot, "code-review", "---\nname: code-review\ndescription: user\n---\nUser body")
	writeSkillFile(t, projectRoot, "code-review", "---\nname: code-review\ndescription: project\n---\nProject body")

	mgr := NewManager(root)
	if err := mgr.Load([]string{userRoot, projectRoot}); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	skill, ok := mgr.Get("code-review")
	if !ok {
		t.Fatalf("expected code-review skill")
	}
	if skill.Description != "project" {
		t.Fatalf("Description = %q, want project", skill.Description)
	}
}

func TestExpandPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILL_TEST_DIR", "custom")

	got := ExpandPath("$SKILL_TEST_DIR/skills", root)
	want := filepath.Join(root, "custom", "skills")
	if got != want {
		t.Fatalf("ExpandPath env relative = %q, want %q", got, want)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("home dir unavailable")
	}
	got = ExpandPath("~/skills", root)
	want = filepath.Join(home, "skills")
	if got != want {
		t.Fatalf("ExpandPath home = %q, want %q", got, want)
	}
}

func TestResolvePathsSkipsMissingDirectories(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "skills")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths := ResolvePaths([]string{"skills", "missing"}, root)
	if len(paths) != 1 || paths[0] != existing {
		t.Fatalf("ResolvePaths() = %#v", paths)
	}
}
