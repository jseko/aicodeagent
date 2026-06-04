package cmd

import (
	"fmt"
	"os"

	"AICodeAgent/internal/app"
	"AICodeAgent/internal/config"
	"AICodeAgent/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "AICodeAgent",
	Short: "A hand-rolled AI coding agent",
	Long:  `AICodeAgent: 一个从零手搓的终端 AI 编程智能体。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. 加载配置（三层优先级：默认 < 文件 < 环境变量）
		cfg, err := config.Load("config.yaml")
		if err != nil {
			return fmt.Errorf("配置加载失败: %w", err)
		}

		// 2. 创建依赖注入容器
		myApp := app.New(cmd.Context(), cfg)
		defer myApp.Close()

		// 3. 初始化并启动TUI
		if err := tui.Start(myApp); err != nil {
			return fmt.Errorf("TUI启动失败: %w", err)
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "执行失败:", err)
		os.Exit(1)
	}
}
