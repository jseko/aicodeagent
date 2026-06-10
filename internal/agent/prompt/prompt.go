package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Prompt 系统提示词接口
type Prompt interface {
	String() string
}

// ToolInfo 工具描述信息
type ToolInfo struct {
	Name        string
	Description string
	Parameters  string // JSON Schema 字符串
}

// EnvironmentInfo 运行时环境信息
type EnvironmentInfo struct {
	OS         string
	Shell      string
	WorkingDir string
	Date       string
}

// ContextFile 上下文文件
type ContextFile struct {
	Path    string
	Content string
}

// PromptData 五层提示词数据
type PromptData struct {
	System      string        // 第1层：系统身份与核心规则
	Environment string        // 第2层：运行时环境信息
	Tools       string        // 第3层：可用工具描述
	Project     string        // 第4层：项目上下文
	Task        string        // 第5层：当前任务
}

func (p *PromptData) String() string {
	var sb strings.Builder
	layers := []struct {
		name  string
		value string
	}{
		{"system", p.System},
		{"environment", p.Environment},
		{"tools", p.Tools},
		{"project", p.Project},
		{"task", p.Task},
	}
	for _, layer := range layers {
		if strings.TrimSpace(layer.value) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("<%s>\n%s\n</%s>\n\n", layer.name, strings.TrimSpace(layer.value), layer.name))
	}
	return strings.TrimSpace(sb.String())
}

// PromptBuilder 五层提示词链式构建器
// WithSystem() 必须首先调用，否则 Build() 返回错误
type PromptBuilder struct {
	data  PromptData
	err   error
	cache atomic.Value // *promptCache
}

type promptCache struct {
	hash   string
	result string
}

// NewPromptBuilder 创建提示词构建器
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// WithSystem 设置系统层（必须首先调用）
func (b *PromptBuilder) WithSystem(identity, role string, rules []string) *PromptBuilder {
	if b.err != nil {
		return b
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，%s\n\n", identity, role))
	if len(rules) > 0 {
		sb.WriteString("## 核心规则\n\n")
		for i, rule := range rules {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule))
		}
		sb.WriteString("\n")
	}
	b.data.System = sb.String()
	return b
}

// WithEnvironment 设置环境层（自动采集）
func (b *PromptBuilder) WithEnvironment() *PromptBuilder {
	if b.err != nil {
		return b
	}
	info := collectEnvironment()
	b.data.Environment = fmt.Sprintf(
		"操作系统: %s\nShell: %s\n工作目录: %s\n日期: %s",
		info.OS, info.Shell, info.WorkingDir, info.Date,
	)
	return b
}

// WithEnvironmentInfo 使用指定环境信息设置环境层
func (b *PromptBuilder) WithEnvironmentInfo(info EnvironmentInfo) *PromptBuilder {
	if b.err != nil {
		return b
	}
	b.data.Environment = fmt.Sprintf(
		"操作系统: %s\nShell: %s\n工作目录: %s\n日期: %s",
		info.OS, info.Shell, info.WorkingDir, info.Date,
	)
	return b
}

// WithTools 设置工具层
func (b *PromptBuilder) WithTools(tools []ToolInfo) *PromptBuilder {
	if b.err != nil {
		return b
	}
	if len(tools) == 0 {
		return b
	}
	var sb strings.Builder
	sb.WriteString("## 可用工具\n\n")
	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s", tool.Name, tool.Description))
		if tool.Parameters != "" {
			sb.WriteString(fmt.Sprintf("\n  参数: %s", tool.Parameters))
		}
		sb.WriteString("\n")
	}
	b.data.Tools = sb.String()
	return b
}

// WithProject 设置项目上下文层
func (b *PromptBuilder) WithProject(files []ContextFile) *PromptBuilder {
	if b.err != nil {
		return b
	}
	if len(files) == 0 {
		return b
	}
	var sb strings.Builder
	sb.WriteString("## 项目上下文\n\n")
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", file.Path, file.Content))
	}
	b.data.Project = sb.String()
	return b
}

// WithProjectContext 直接设置项目上下文内容
func (b *PromptBuilder) WithProjectContext(content string) *PromptBuilder {
	if b.err != nil {
		return b
	}
	b.data.Project = content
	return b
}

// WithTask 设置任务层
func (b *PromptBuilder) WithTask(description string) *PromptBuilder {
	if b.err != nil {
		return b
	}
	b.data.Task = fmt.Sprintf("## 当前任务\n\n%s", description)
	return b
}

// Build 构建最终的系统提示词
// 返回值: (提示词文本, 错误)。当 System 层缺失时返回错误
func (b *PromptBuilder) Build() (string, error) {
	if b.err != nil {
		return "", b.err
	}
	if strings.TrimSpace(b.data.System) == "" {
		return "", fmt.Errorf("prompt: System layer is required, call WithSystem() first")
	}

	result := b.data.String()

	// 写入缓存
	hash := hashString(result)
	cached := &promptCache{hash: hash, result: result}
	b.cache.Store(cached)

	return result, nil
}

// BuildCached 构建提示词，若环境未变则返回缓存
func (b *PromptBuilder) BuildCached(envHash string) (string, error) {
	if b.err != nil {
		return "", b.err
	}

	// 检查缓存
	if cached := b.cache.Load(); cached != nil {
		c := cached.(*promptCache)
		if c.hash != "" && envHash != "" && c.hash == envHash {
			return c.result, nil
		}
	}

	result, err := b.Build()
	if err != nil {
		return "", err
	}

	// 用环境哈希更新缓存
	if envHash != "" {
		cached := &promptCache{hash: envHash, result: result}
		b.cache.Store(cached)
	}

	return result, nil
}

// Cached 获取缓存的提示词（如果有）
func (b *PromptBuilder) Cached() (string, bool) {
	if cached := b.cache.Load(); cached != nil {
		c := cached.(*promptCache)
		if c.result != "" {
			return c.result, true
		}
	}
	return "", false
}

// collectEnvironment 采集当前运行时环境信息
func collectEnvironment() EnvironmentInfo {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "unknown"
	}
	wd, err := os.Getwd()
	if err != nil {
		wd = "unknown"
	}
	return EnvironmentInfo{
		OS:         runtime.GOOS,
		Shell:      shell,
		WorkingDir: wd,
		Date:       time.Now().Format("2006-01-02"),
	}
}

// ComputeEnvironmentHash 计算环境哈希，用于检测环境变化
func ComputeEnvironmentHash(info EnvironmentInfo) string {
	payload := fmt.Sprintf("%s|%s|%s|%s", info.OS, info.Shell, info.WorkingDir, info.Date)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])[:16]
}

// CurrentEnvironmentHash 计算当前环境的哈希
func CurrentEnvironmentHash() string {
	return ComputeEnvironmentHash(collectEnvironment())
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}
