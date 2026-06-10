package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"AICodeAgent/internal/llm"
)

// initializeResult /initialize 命令的执行结果
type initializeResult struct {
	Content     string // 生成的 AGENTS.md 内容
	ProjectRoot string // 项目根目录
	FilesFound  int    // 扫描到的项目文件数
	IsUpdate    bool   // 是否为增量更新
}

// scanProjectStructure 扫描项目结构，收集关键信息
func scanProjectStructure(root string) ([]ContextFile, error) {
	var files []ContextFile

	// 扫描已知的项目标识文件
	knownFiles := []string{
		"go.mod", "go.sum", "Makefile", "Dockerfile",
		"package.json", "Cargo.toml", "requirements.txt",
		"README.md", "README.cn.md", "LICENSE",
		".gitignore", ".dockerignore",
	}

	for _, name := range knownFiles {
		path := filepath.Join(root, name)
		content, ok := readContextFile(path, 64*1024) // 64KB 限制
		if !ok {
			continue
		}
		files = append(files, ContextFile{Path: name, Content: content})
	}

	// 扫描目录结构（仅顶层目录）
	entries, err := os.ReadDir(root)
	if err != nil {
		return files, nil // 目录读取失败不阻塞
	}

	var dirs []string
	fileCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			// 跳过隐藏目录和常见非项目目录
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				continue
			}
			dirs = append(dirs, name)
		} else {
			fileCount++
		}
	}

	if len(dirs) > 0 {
		dirSummary := fmt.Sprintf("项目顶层目录（共 %d 个文件，%d 个目录）:\n", fileCount, len(dirs))
		for _, d := range dirs {
			dirSummary += fmt.Sprintf("- %s/\n", d)
		}
		files = append(files, ContextFile{Path: ".structure", Content: dirSummary})
	}

	return files, nil
}

// buildInitPrompt 构造 /initialize 的 LLM 提示词
func buildInitPrompt(projectRoot string, contextFiles []ContextFile, existingContent string) string {
	var sb strings.Builder

	sb.WriteString("你是一个专业的项目文档编写助手。请根据以下项目分析结果，生成 AGENTS.md 文件。\n\n")
	sb.WriteString("## 输出要求\n\n")
	sb.WriteString("生成符合以下规范的 Markdown 文档：\n\n")
	sb.WriteString("1. **项目概述**：项目的用途和技术栈\n")
	sb.WriteString("2. **项目结构**：关键目录和文件说明\n")
	sb.WriteString("3. **常用命令**：构建、测试、运行等命令\n")
	sb.WriteString("4. **编码规范**：项目特定的编码约定\n")
	sb.WriteString("5. **注意事项**：重要的约束和提示\n\n")

	if existingContent != "" {
		sb.WriteString("## 现有 AGENTS.md（请保留用户手动编写的章节）\n\n")
		sb.WriteString("```markdown\n")
		sb.WriteString(existingContent)
		sb.WriteString("\n```\n\n")
		sb.WriteString("请保留上述内容中以 `## [用户]` 开头的章节不变，仅更新或补充其他章节。\n\n")
	}

	sb.WriteString("## 项目分析结果\n\n")
	sb.WriteString(fmt.Sprintf("项目根目录: %s\n\n", projectRoot))

	for _, f := range contextFiles {
		if f.Path == ".structure" {
			sb.WriteString(f.Content + "\n")
			continue
		}
		// 对于大文件，只取前 2000 字符
		content := f.Content
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", f.Path, content))
	}

	sb.WriteString("请直接输出 AGENTS.md 的完整内容，不要包含额外的解释。")
	return sb.String()
}

// extractUserSections 从现有 AGENTS.md 中提取用户手动编写的章节
func extractUserSections(content string) []string {
	if content == "" {
		return nil
	}
	var sections []string
	lines := strings.Split(content, "\n")
	inUserSection := false
	var currentSection strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "## [用户]") {
			if inUserSection && currentSection.Len() > 0 {
				sections = append(sections, currentSection.String())
			}
			inUserSection = true
			currentSection.Reset()
			currentSection.WriteString(line + "\n")
			continue
		}
		if inUserSection && strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "## [用户]") {
			// 遇到新的非用户章节，结束当前用户章节
			sections = append(sections, currentSection.String())
			currentSection.Reset()
			inUserSection = false
			continue
		}
		if inUserSection {
			currentSection.WriteString(line + "\n")
		}
	}
	if inUserSection && currentSection.Len() > 0 {
		sections = append(sections, currentSection.String())
	}
	return sections
}

// mergeAgentMD 合并新生成的内容与用户手动编写的章节
func mergeAgentMD(newContent string, existingContent string) string {
	userSections := extractUserSections(existingContent)
	if len(userSections) == 0 {
		return newContent
	}

	// 在新内容末尾追加用户章节
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(newContent))
	sb.WriteString("\n\n")
	for _, section := range userSections {
		sb.WriteString(strings.TrimSpace(section))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// handleInitialize 处理 /initialize 命令
// 返回生成的 AGENTS.md 内容和操作结果
func (c *coordinator) handleInitialize(ctx context.Context, projectRoot string) (*initializeResult, error) {
	// 1. 检查现有的 AGENTS.md
	existingPath := filepath.Join(projectRoot, "AGENTS.md")
	var existingContent string
	isUpdate := false
	if data, err := os.ReadFile(existingPath); err == nil {
		existingContent = string(data)
		isUpdate = true
	}

	// 2. 扫描项目结构
	contextFiles, err := scanProjectStructure(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("scan project structure: %w", err)
	}

	// 3. 构造 LLM 提示词
	prompt := buildInitPrompt(projectRoot, contextFiles, existingContent)

	// 4. 获取小模型（生成文档不需要大模型）
	model := c.smallModel.Load()
	if model == nil {
		model = c.largeModel.Load()
	}
	if model == nil {
		return nil, fmt.Errorf("no model available for initialization")
	}

	provider, err := BuildProvider(model)
	if err != nil {
		return nil, fmt.Errorf("build provider: %w", err)
	}
	defer provider.Close()

	streamCall := llm.AgentStreamCall{
		Prompt:      prompt,
		Temperature: model.Config.Temperature,
		TopP:        model.Config.TopP,
		MaxTokens:   model.Config.MaxTokens,
	}

	response, err := provider.Chat(ctx, streamCall)
	if err != nil {
		return nil, fmt.Errorf("generate AGENTS.md: %w", err)
	}

	// 5. 如果是更新，合并用户章节
	if isUpdate {
		response = mergeAgentMD(response, existingContent)
	}

	return &initializeResult{
		Content:     response,
		ProjectRoot: projectRoot,
		FilesFound:  len(contextFiles),
		IsUpdate:    isUpdate,
	}, nil
}
