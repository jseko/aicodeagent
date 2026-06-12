package subagent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type SubagentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*Subagent
}

func NewRegistry() *SubagentRegistry {
	return &SubagentRegistry{agents: make(map[string]*Subagent)}
}

func (r *SubagentRegistry) Register(agent *Subagent) error {
	if err := validateSubagent(agent); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := agent.Config.Name
	if _, exists := r.agents[name]; exists {
		return fmt.Errorf("subagent already registered: %s", name)
	}
	r.agents[name] = agent
	return nil
}

func (r *SubagentRegistry) Override(agent *Subagent) error {
	if err := validateSubagent(agent); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.Config.Name] = agent
	return nil
}

func (r *SubagentRegistry) Get(name string) (*Subagent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("subagent not found: %s", name)
	}
	return agent, nil
}

func (r *SubagentRegistry) List() []*Subagent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	agents := make([]*Subagent, 0, len(names))
	for _, name := range names {
		agents = append(agents, r.agents[name])
	}
	return agents
}

type LoadDiagnostic struct {
	Path string
	Err  error
}

type SubagentLoader struct {
	UserDir     string
	ProjectDir  string
	Registry    *SubagentRegistry
	Logger      *slog.Logger
	Diagnostics []LoadDiagnostic
}

func NewLoader(projectDir string, registry *SubagentRegistry, logger *slog.Logger) *SubagentLoader {
	if registry == nil {
		registry = NewRegistry()
	}
	return &SubagentLoader{
		UserDir:    defaultUserDir(),
		ProjectDir: filepath.Join(projectDir, ".aicode", "subagents"),
		Registry:   registry,
		Logger:     logger,
	}
}

func (l *SubagentLoader) LoadSubagents() error {
	if l.Registry == nil {
		l.Registry = NewRegistry()
	}
	if err := l.loadDir(l.UserDir, false); err != nil {
		return err
	}
	return l.loadDir(l.ProjectDir, true)
}

func (l *SubagentLoader) loadDir(dir string, override bool) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read subagent dir %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := l.loadOne(path, override); err != nil {
			l.Diagnostics = append(l.Diagnostics, LoadDiagnostic{Path: path, Err: err})
			if l.Logger != nil {
				l.Logger.Warn("load subagent failed", "path", path, "error", err)
			}
		}
	}
	return nil
}

func (l *SubagentLoader) loadOne(path string, override bool) error {
	cfg, prompt, err := LoadSubagentConfig(path)
	if err != nil {
		return err
	}
	agent := &Subagent{Config: cfg, SystemPrompt: prompt, Path: path}
	if override {
		return l.Registry.Override(agent)
	}
	return l.Registry.Register(agent)
}

func defaultUserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aicode", "subagents")
}

func validateSubagent(agent *Subagent) error {
	if agent == nil || agent.Config == nil {
		return fmt.Errorf("subagent required")
	}
	if err := ValidateSubagentConfig(agent.Config); err != nil {
		return err
	}
	if strings.TrimSpace(agent.SystemPrompt) == "" {
		return fmt.Errorf("system prompt required")
	}
	return nil
}
