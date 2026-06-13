package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "交互式初始化配置",
	Long: `启动交互式配置向导，引导首次用户设置 LLM 提供商、API Key 和默认模型。

示例：
  AICodeAgent init`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户主目录: %w", err)
	}
	configDir := filepath.Join(home, ".aicode")
	configPath := filepath.Join(configDir, "config.yaml")

	// 检测已有配置
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("⚠️  检测到已有配置文件 %s\n", configPath)
		fmt.Print("是否覆盖？(y/N): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("已取消。")
			return nil
		}
	}

	scanner := bufio.NewScanner(os.Stdin)

	// 1. 选择 LLM 提供商
	fmt.Println("\n=== LLM 提供商选择 ===")
	fmt.Println("1. OpenAI (GPT-4o, GPT-4o-mini)")
	fmt.Println("2. Anthropic (Claude Sonnet, Claude Haiku)")
	fmt.Println("3. OpenRouter (多模型聚合)")
	fmt.Print("\n请选择 (1-3): ")

	providerChoice := "openai"
	if scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "2":
			providerChoice = "anthropic"
		case "3":
			providerChoice = "openrouter"
		}
	}

	// 2. 输入 API Key
	var apiKey string
	switch providerChoice {
	case "openai":
		fmt.Print("\n请输入 OpenAI API Key: ")
	case "anthropic":
		fmt.Print("\n请输入 Anthropic API Key: ")
	case "openrouter":
		fmt.Print("\n请输入 OpenRouter API Key: ")
	}
	if scanner.Scan() {
		apiKey = strings.TrimSpace(scanner.Text())
	}

	// 3. 选择默认模型
	defaultModel := "gpt-4o"
	switch providerChoice {
	case "openai":
		fmt.Println("\n=== 默认模型选择 ===")
		fmt.Println("1. gpt-4o (推荐)")
		fmt.Println("2. gpt-4o-mini (更快)")
		fmt.Print("\n请选择 (1-2): ")
		if scanner.Scan() && strings.TrimSpace(scanner.Text()) == "2" {
			defaultModel = "gpt-4o-mini"
		}
	case "anthropic":
		fmt.Println("\n=== 默认模型选择 ===")
		fmt.Println("1. claude-sonnet-4-6 (推荐)")
		fmt.Println("2. claude-haiku-4-5 (更快)")
		fmt.Print("\n请选择 (1-2): ")
		if scanner.Scan() && strings.TrimSpace(scanner.Text()) == "2" {
			defaultModel = "claude-haiku-4-5"
		} else {
			defaultModel = "claude-sonnet-4-6"
		}
	case "openrouter":
		defaultModel = "openai/gpt-4o"
	}

	// 4. 写入配置文件
	cfg := buildConfig(providerChoice, apiKey, defaultModel)
	if err := writeConfig(configPath, cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	fmt.Printf("\n✅ 配置已保存到 %s\n", configPath)
	fmt.Println("\n=== 下一步 ===")
	fmt.Println("  运行 AICodeAgent 启动 TUI 模式")
	fmt.Println("  运行 AICodeAgent chat \"你的问题\" 开始对话")
	fmt.Println("  运行 AICodeAgent logs 查看日志")
	fmt.Println("  运行 AICodeAgent completion bash 生成补全脚本")

	return nil
}

func buildConfig(provider, apiKey, model string) map[string]interface{} {
	cfg := map[string]interface{}{
		"theme":    "dark",
		"log_level": "info",
		"models": map[string]interface{}{
			"default": model,
			"items": map[string]interface{}{
				model: map[string]interface{}{
					"provider":    provider,
					"model":       model,
					"temperature": 0.7,
					"max_tokens":  4096,
				},
			},
		},
	}

	if apiKey != "" {
		cfg["providers"] = []map[string]interface{}{
			{
				"type":    provider,
				"name":    provider,
				"base_url": getDefaultBaseURL(provider),
				"api_key": apiKey,
			},
		}
	}

	return cfg
}

func getDefaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}

func writeConfig(path string, cfg map[string]interface{}) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
