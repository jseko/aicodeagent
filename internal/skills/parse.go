package skills

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func Parse(path string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	yamlContent, markdownContent, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("splitting frontmatter: %w", err)
	}

	var skill Skill
	decoder := yaml.NewDecoder(bytes.NewReader(yamlContent))
	decoder.KnownFields(true)
	if err := decoder.Decode(&skill); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	skill.Instructions = strings.TrimSpace(string(markdownContent))
	skill.Path = filepath.Dir(path)
	return &skill, nil
}

func splitFrontmatter(content []byte) (yamlContent, markdownContent []byte, err error) {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return nil, nil, errors.New("missing yaml frontmatter")
	}

	rest := content[len("---\n"):]
	endIndex := bytes.Index(rest, []byte("\n---\n"))
	if endIndex == -1 {
		return nil, nil, errors.New("unclosed yaml frontmatter")
	}

	yamlContent = rest[:endIndex]
	markdownContent = rest[endIndex+len("\n---\n"):]
	return yamlContent, markdownContent, nil
}
