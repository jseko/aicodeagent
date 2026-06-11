package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"AICodeAgent/internal/mcp"
)

const defaultMemoryMaxBytes int64 = 100 * 1024

var projectMemoryFiles = []string{
	"AGENTS.md",
	"AGENTS.cn.md",
	"CLAUDE.md",
	".claude/CLAUDE.md",
	".cursorrules",
	".github/copilot-instructions.md",
}

var userMemoryFiles = []string{
	"~/.aicode/AGENTS.md",
	"~/.aicode/AGENTS.cn.md",
}

type ContextFile struct {
	Path    string
	Content string
}

func processContextPath(root string, maxBytes int64) []ContextFile {
	if maxBytes <= 0 {
		maxBytes = defaultMemoryMaxBytes
	}
	var files []ContextFile
	for _, name := range projectMemoryFiles {
		path := filepath.Join(root, name)
		content, ok := readContextFile(path, maxBytes)
		if !ok {
			continue
		}
		files = append(files, ContextFile{Path: path, Content: content})
	}
	return files
}

func processUserContextPath(maxBytes int64) []ContextFile {
	if maxBytes <= 0 {
		maxBytes = defaultMemoryMaxBytes
	}
	var files []ContextFile
	for _, name := range userMemoryFiles {
		path := expandHome(name)
		content, ok := readContextFile(path, maxBytes)
		if !ok {
			continue
		}
		files = append(files, ContextFile{Path: path, Content: content})
	}
	return files
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func readContextFile(path string, maxBytes int64) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxBytes {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func formatContextFiles(files []ContextFile) string {
	if len(files) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n<project_context path=%q>\n%s\n</project_context>\n", file.Path, file.Content))
	}
	return strings.TrimSpace(sb.String())
}

func getMCPInstructions(ctx context.Context, timeout time.Duration) string {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		text string
	}
	ch := make(chan result, 1)
	go func() {
		sessions := mcp.ListSessions()
		if len(sessions) == 0 {
			ch <- result{}
			return
		}
		var sb strings.Builder
		for name, session := range sessions {
			if session == nil {
				continue
			}
			tools := session.Tools()
			if len(tools) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("\n<mcp_server name=%q>\n", name))
			for _, tool := range tools {
				if tool == nil {
					continue
				}
				desc := strings.TrimSpace(tool.Description)
				if desc == "" {
					desc = "No description provided."
				}
				sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, desc))
			}
			sb.WriteString("</mcp_server>\n")
		}
		ch <- result{text: strings.TrimSpace(sb.String())}
	}()

	select {
	case <-ctx.Done():
		return ""
	case res := <-ch:
		return res.text
	}
}

func buildSystemPrompt(base string, contextFiles []ContextFile, mcpInstructions string) string {
	parts := []string{strings.TrimSpace(base)}
	if projectContext := formatContextFiles(contextFiles); projectContext != "" {
		parts = append(parts, projectContext)
	}
	if strings.TrimSpace(mcpInstructions) != "" {
		parts = append(parts, "<mcp_instructions>\n"+strings.TrimSpace(mcpInstructions)+"\n</mcp_instructions>")
	}
	return strings.Join(parts, "\n\n")
}

func buildSummaryPrompt(messages []Message) string {
	var sb strings.Builder
	sb.WriteString("请将以下会话压缩为结构化 Markdown 摘要，必须保留技术细节。\n\n")
	sb.WriteString("输出必须包含以下章节：\n")
	sb.WriteString("## Work Completed\n## Files Modified\n## Current State\n## Key Decisions\n## Next Steps\n## Todo Status\n\n")
	sb.WriteString("会话内容：\n")
	for _, msg := range messages {
		if msg.IsSummaryMessage || !msg.ShouldInclude() {
			continue
		}
		role := strings.ToUpper(string(msg.Role))
		sb.WriteString(fmt.Sprintf("\n### %s\n%s\n", role, strings.TrimSpace(msg.Content)))
	}
	return sb.String()
}

func validateSummaryContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("summary content is empty")
	}

	requiredSections := []string{
		"## Work Completed",
		"## Files Modified",
		"## Current State",
		"## Key Decisions",
		"## Next Steps",
		"## Todo Status",
	}
	for _, section := range requiredSections {
		if !strings.Contains(trimmed, section) {
			return fmt.Errorf("summary missing required section %q", section)
		}
	}
	return nil
}
