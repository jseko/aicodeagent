package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Manager struct {
	mu         sync.RWMutex
	skills     map[string]*Skill
	discoverer *Discoverer
	projectDir string
}

func NewManager(projectDir string) *Manager {
	return &Manager{
		skills:     make(map[string]*Skill),
		discoverer: NewDiscoverer(),
		projectDir: projectDir,
	}
}

func (m *Manager) Register(skill *Skill) error {
	if err := skill.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.skills[skill.Name] = skill
	return nil
}

func (m *Manager) Get(name string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	skill, ok := m.skills[name]
	return skill, ok
}

func (m *Manager) List() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.skills))
	for name := range m.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	skills := make([]*Skill, 0, len(names))
	for _, name := range names {
		skills = append(skills, m.skills[name])
	}
	return skills
}

func (m *Manager) Load(paths []string) error {
	for _, skill := range m.discoverer.Discover(paths) {
		if err := m.Register(skill); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) LoadDefault(additionalPaths ...string) error {
	paths := DefaultPaths(m.projectDir)
	paths = append(paths, additionalPaths...)
	return m.Load(paths)
}

func DefaultPaths(projectDir string) []string {
	var paths []string
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = appendIfDir(paths, filepath.Join(homeDir, ".aicode", "skills"))
	}
	if projectDir != "" {
		paths = appendIfDir(paths, filepath.Join(projectDir, ".aicode", "skills"))
	}
	if envPath := strings.TrimSpace(os.Getenv("AICODE_SKILLS_DIR")); envPath != "" {
		paths = appendIfDir(paths, ExpandPath(envPath, projectDir))
	}
	return paths
}

func ResolvePaths(paths []string, projectDir string) []string {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		expanded := ExpandPath(path, projectDir)
		resolved = appendIfDir(resolved, expanded)
	}
	return resolved
}

func ExpandPath(path, projectDir string) string {
	path = os.ExpandEnv(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if path == "~" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return homeDir
		}
	}
	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	if !filepath.IsAbs(path) && projectDir != "" {
		return filepath.Join(projectDir, path)
	}
	return path
}

func appendIfDir(paths []string, path string) []string {
	if path == "" {
		return paths
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return append(paths, path)
	}
	return paths
}
