package config

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Theme       string         `yaml:"theme"`
	OpenAI      OpenAI         `yaml:"openai"`
	SkillsPaths []string       `yaml:"skills_paths"`
	LogLevel    string         `yaml:"log_level"`
	Agent       AgentConfig    `yaml:"agent"`
	Providers   []ProviderConfig `yaml:"providers"`
	Session     SessionConfig  `yaml:"session"`
	Permission  PermissionConfig `yaml:"permission"`
	SummaryPrompt string       `yaml:"summary_prompt"`
}

type OpenAI struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	Key         string
}

type AgentConfig struct {
	LargeModel  SelectedModel `yaml:"large_model"`
	SmallModel  SelectedModel `yaml:"small_model"`
}

type SelectedModel struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
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
	FileWhitelist  []string `yaml:"file_whitelist"`
	CmdWhitelist   []string `yaml:"cmd_whitelist"`
}

func (c *Config) ResolveSecret(secretRef string) string {
	if secretRef == "" {
		return ""
	}
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
		Session:    SessionConfig{IdleTimeout: "30m", MaxSessions: 10},
		Permission: PermissionConfig{DefaultLevel: 1},
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
	return nil
}
