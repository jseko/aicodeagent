package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"AICodeAgent/internal/hooks"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Theme         string               `yaml:"theme"`
	OpenAI        OpenAI               `yaml:"openai"`
	SkillsPaths   []string             `yaml:"skills_paths"`
	LogLevel      string               `yaml:"log_level"`
	Agent         AgentConfig          `yaml:"agent"`
	Providers     []ProviderConfig     `yaml:"providers"`
	Models        ModelsConfig         `yaml:"models"`
	Context       ContextConfig        `yaml:"context"`
	Session       SessionConfig        `yaml:"session"`
	Permission    PermissionConfig     `yaml:"permission"`
	SummaryPrompt string               `yaml:"summary_prompt"`
	MCP           map[string]MCPConfig `yaml:"mcp"`
	Hooks         hooks.Config         `yaml:"hooks"`
	Subagents     SubagentsConfig      `yaml:"subagents"`
LSP           map[string]LSPConfig `yaml:"lsp"`

}

type OpenAI struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	Key         string
}

type AgentConfig struct {
	LargeModel SelectedModel `yaml:"large_model"`
	SmallModel SelectedModel `yaml:"small_model"`
}

type SelectedModel struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

type ModelsConfig struct {
	Default   string                 `yaml:"default"`
	Scenarios ModelScenarios         `yaml:"scenarios"`
	Items     map[string]ModelConfig `yaml:"items"`
}

type ModelScenarios struct {
	Chat       string `yaml:"chat"`
	Summarize  string `yaml:"summarize"`
	Initialize string `yaml:"initialize"`
}

type ModelConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	TopP        float64 `yaml:"top_p"`
	MaxTokens   int64   `yaml:"max_tokens"`
	InputPer1M  float64 `yaml:"input_per_1m"`
	OutputPer1M float64 `yaml:"output_per_1m"`
}

type ContextConfig struct {
	WindowTokens      int64   `yaml:"window_tokens"`
	LargeWindowBuffer int64   `yaml:"large_window_buffer"`
	SmallWindowRatio  float64 `yaml:"small_window_ratio"`
	SummaryModel      string  `yaml:"summary_model"`
	MemoryMaxBytes    int64   `yaml:"memory_max_bytes"`
}

type ProviderConfig struct {
	Type    string `yaml:"type"`
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

type SessionConfig struct {
	IdleTimeout string `yaml:"idle_timeout"`
	MaxSessions int    `yaml:"max_sessions"`
}

type PermissionConfig struct {
	DefaultLevel  int      `yaml:"default_level"`
	FileWhitelist []string `yaml:"file_whitelist"`
	CmdWhitelist  []string `yaml:"cmd_whitelist"`
}

// MCPConfig MCP 服务器连接配置
type MCPConfig struct {
	Type          string            `yaml:"type"`           // stdio / http / sse
	Command       string            `yaml:"command"`        // stdio 模式：可执行命令
	Args          []string          `yaml:"args"`           // stdio 模式：命令参数
	URL           string            `yaml:"url"`            // http/sse 模式：服务端点
	Timeout       int               `yaml:"timeout"`        // 超时时间（秒），默认 15
	Disabled      bool              `yaml:"disabled"`       // 是否禁用
	DisabledTools []string          `yaml:"disabled_tools"` // 禁用的工具列表
	Env           map[string]string `yaml:"env"`            // 环境变量
	Headers       map[string]string `yaml:"headers"`        // HTTP 头（http/sse 模式）
}

type SubagentsConfig struct {
	Enabled        bool `yaml:"enabled"`
	AgenticEnabled bool `yaml:"agentic_enabled"`
	}

type LSPConfig struct {
	Command               string            `yaml:"command"`
	Args                  []string          `yaml:"args"`
	FileTypes             []string          `yaml:"file_types"`
	RootMarkers           []string          `yaml:"root_markers"`
	Disabled              bool              `yaml:"disabled"`
	Env                   map[string]string `yaml:"env"`
	MaxConcurrentRequests int               `yaml:"max_concurrent_requests"`

}

func (c *Config) ResolveSecret(secretRef string) string {
	if secretRef == "" {
		return ""
	}
	// 解析 ${VAR} 语法：去掉 ${ 和 } 后从环境变量取值
	if strings.HasPrefix(secretRef, "${") && strings.HasSuffix(secretRef, "}") {
		varName := secretRef[2 : len(secretRef)-1]
		if key := os.Getenv(varName); key != "" {
			return key
		}
		return secretRef
	}
	// 兜底：直接作为环境变量名查找
	if key := os.Getenv(secretRef); key != "" {
		return key
	}
	return secretRef
}

// Load 加载配置：默认 → 文件 → 环境变量
func Load(path string) (*Config, error) {
	// 1. 初始化默认配置
	cfg := &Config{
		Theme:    "dark",
		LogLevel: "info",
		OpenAI:   OpenAI{Model: "gpt-4o", Temperature: 0.7},
		Agent: AgentConfig{
			LargeModel: SelectedModel{Provider: "openai", Model: "gpt-4o"},
			SmallModel: SelectedModel{Provider: "openai", Model: "gpt-4o-mini"},
		},
		Models: ModelsConfig{
			Default: "gpt-4o",
			Scenarios: ModelScenarios{
				Chat:       "gpt-4o",
				Summarize:  "gpt-4o-mini",
				Initialize: "gpt-4o-mini",
			},
			Items: map[string]ModelConfig{
				"gpt-4o": {
					Provider: "openai", Model: "gpt-4o",
					Temperature: 0.7, TopP: 1.0, MaxTokens: 4096,
				},
				"gpt-4o-mini": {
					Provider: "openai", Model: "gpt-4o-mini",
					Temperature: 0.3, TopP: 1.0, MaxTokens: 2048,
				},
			},
		},
		Context: ContextConfig{
			WindowTokens:      128000,
			LargeWindowBuffer: 20000,
			SmallWindowRatio:  0.2,
			SummaryModel:      "gpt-4o-mini",
			MemoryMaxBytes:    100 * 1024,
		},
		Session:       SessionConfig{IdleTimeout: "30m", MaxSessions: 10},
		Permission:    PermissionConfig{DefaultLevel: 1},
		SummaryPrompt: "请用简洁的语言总结以下对话的关键要点，保留所有技术细节。",
	}

	// 2. 读取配置文件（如果不存在则使用默认）
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			log.Printf("[Config] 配置文件解析失败，使用默认配置: %v", err)
		}
	}

	// 3. 环境变量覆盖敏感字段
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAI.Key = key
	}
	// 解析 Provider API Key 中的环境变量引用（如 ${DEEPSEEK_API_KEY}）
	for i := range cfg.Providers {
		cfg.Providers[i].APIKey = cfg.ResolveSecret(cfg.Providers[i].APIKey)
	}

	return cfg, cfg.Validate()
}

// Validate 校验配置有效性
func (c *Config) Validate() error {
	if c.Theme == "" {
		return fmt.Errorf("配置校验失败: theme 不能为空")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("配置校验失败: log_level 不能为空")
	}
	if c.Hooks.Enabled {
		if _, err := hooks.ValidateExternalHooks(c.Hooks.Hooks); err != nil {
			return fmt.Errorf("hooks 配置校验失败: %w", err)
		}
	}
	return nil
}

func (c *Config) GetModelConfig(id string) (ModelConfig, bool) {
	if id == "" {
		id = c.Models.Default
	}
	if c.Models.Items == nil {
		return ModelConfig{}, false
	}
	model, ok := c.Models.Items[id]
	if !ok {
		return ModelConfig{}, false
	}
	return c.normalizeModelConfig(id, model), true
}

func (c *Config) normalizeModelConfig(id string, model ModelConfig) ModelConfig {
	if model.Model == "" {
		model.Model = id
	}
	if model.Provider == "" {
		model.Provider = "openai"
	}
	if model.Temperature == 0 {
		model.Temperature = c.OpenAI.Temperature
	}
	if model.TopP == 0 {
		model.TopP = 1.0
	}
	if model.MaxTokens == 0 {
		model.MaxTokens = 4096
	}
	return model
}

func (c *Config) ModelForScenario(scenario string) (string, ModelConfig, bool) {
	id := c.Models.Default
	switch scenario {
	case "chat":
		if c.Models.Scenarios.Chat != "" {
			id = c.Models.Scenarios.Chat
		}
	case "summarize":
		if c.Models.Scenarios.Summarize != "" {
			id = c.Models.Scenarios.Summarize
		}
	case "initialize":
		if c.Models.Scenarios.Initialize != "" {
			id = c.Models.Scenarios.Initialize
		}
	}
	model, ok := c.GetModelConfig(id)
	return id, model, ok
}
