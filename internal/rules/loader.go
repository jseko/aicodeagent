package rules

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultDiscoveryPaths 按优先级降序排列（高优先级在前）
var defaultDiscoveryPaths = []struct {
	Path     string
	Priority RulePriority
	Format   string
}{
	{Path: "AICODE.local.md", Priority: PriorityProjectLocal, Format: "markdown"},
	{Path: ".aicode/rules.yaml", Priority: PriorityProjectYAML, Format: "yaml"},
	{Path: ".aicode/rules/*.md", Priority: PriorityProjectDir, Format: "markdown"},
	{Path: ".aicode/rules/*.mdc", Priority: PriorityProjectDir, Format: "markdown"},
	{Path: "AICODE.md", Priority: PriorityProjectRoot, Format: "markdown"},
	{Path: "AGENTS.md", Priority: PriorityProjectRoot, Format: "markdown"},
	{Path: ".cursorrules", Priority: PriorityProjectRoot, Format: "markdown"},
	{Path: ".github/copilot-instructions.md", Priority: PriorityProjectRoot, Format: "markdown"},
	{Path: "~/.aicode/rules.yaml", Priority: PriorityUserCustom, Format: "yaml"},
	{Path: "~/.aicode/rules/*.md", Priority: PriorityUserCustom, Format: "markdown"},
	{Path: "~/.aicode/rules/*.mdc", Priority: PriorityUserCustom, Format: "markdown"},
	{Path: "~/.aicode/AGENTS.md", Priority: PriorityUserGlobal, Format: "markdown"},
}

type Loader struct {
	workingDir string
	cache      map[string]ruleCacheEntry
}

type ruleCacheEntry struct {
	rules     *RuleSet
	signature string
}

type discoveredRuleFile struct {
	path     string
	priority RulePriority
	format   string
}

func NewLoader(workingDir string) *Loader {
	return &Loader{
		workingDir: workingDir,
		cache:      make(map[string]ruleCacheEntry),
	}
}

func (l *Loader) WorkingDir() string { return l.workingDir }

func (l *Loader) SetWorkingDir(dir string) {
	l.workingDir = dir
}

func (l *Loader) LoadRules() (*RuleSet, error) {
	files := l.discoverRuleFiles()
	signature := buildRuleSignature(files)
	if cached, ok := l.cache[l.workingDir]; ok && cached.signature == signature {
		return cached.rules, nil
	}

	merged := &RuleSet{}
	merger := &Merger{}
	merger.Add(RuleSource{Path: "default", Priority: PriorityDefault, Rules: DefaultRuleSet()})

	for _, file := range files {
		source := RuleSource{Path: file.path, Priority: file.priority}
		if loaded, err := l.loadRuleFile(file.path, file.format); err == nil {
			source.Rules = *loaded
			merger.Add(source)
		}
	}

	raw, _ := merger.Merge()
	merged = raw
	l.cache[l.workingDir] = ruleCacheEntry{rules: merged, signature: signature}
	return merged, nil
}

func (l *Loader) discoverRuleFiles() []discoveredRuleFile {
	var files []discoveredRuleFile
	for _, dp := range defaultDiscoveryPaths {
		expanded := l.expandPath(dp.Path)
		if strings.Contains(expanded, "*") {
			matches, err := filepath.Glob(expanded)
			if err != nil || len(matches) == 0 {
				continue
			}
			for _, m := range matches {
				files = append(files, discoveredRuleFile{path: m, priority: dp.Priority, format: dp.Format})
			}
			continue
		}
		if _, err := os.Stat(expanded); err != nil {
			continue
		}
		files = append(files, discoveredRuleFile{path: expanded, priority: dp.Priority, format: dp.Format})
	}
	return files
}

func buildRuleSignature(files []discoveredRuleFile) string {
	var b strings.Builder
	for _, file := range files {
		info, err := os.Stat(file.path)
		if err != nil {
			continue
		}
		b.WriteString(file.path)
		b.WriteString(":")
		b.WriteString(info.ModTime().UTC().Format("20060102150405.000000000"))
		b.WriteString(":")
		b.WriteString(strconv.FormatInt(info.Size(), 10))
		b.WriteString("\n")
	}
	return b.String()
}

func (l *Loader) loadRuleFile(path, format string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	switch format {
	case "yaml":
		return ParseYAML(data)
	default:
		return ParseMarkdown(data), nil
	}
}

func (l *Loader) expandPath(path string) string {
	path = strings.ReplaceAll(path, "<project>", l.workingDir)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.workingDir, path)
	}
	return path
}

func (l *Loader) InvalidateCache(dir string) {
	delete(l.cache, dir)
}

func (l *Loader) ClearCache() {
	l.cache = make(map[string]ruleCacheEntry)
}
