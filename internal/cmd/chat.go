package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"AICodeAgent/internal/agent"
	"AICodeAgent/internal/app"
	"AICodeAgent/internal/config"

	"github.com/spf13/cobra"
)

const (
	defaultSessionID      = "default"
	interactiveSessionID  = "interactive"
)

var (
	sessionID string
)

var chatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "与AI对话",
	Long: `与AICodeAgent进行对话。

单条消息模式：
  AICodeAgent chat "帮我优化这段代码"

交互式模式：
  AICodeAgent chat

使用--session指定会话ID以保持对话连续性：
  AICodeAgent chat --session abc123 "继续我们刚才的讨论"`,
	RunE: runChat,
}

func init() {
	chatCmd.Flags().StringVarP(&sessionID, "session", "s", "", "会话ID（用于多轮对话连续性）")
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("配置加载失败: %w", err)
	}

	myApp := app.New(cmd.Context(), cfg)
	defer myApp.Close()

	// 等待Agent初始化完成
	if myApp.Coordinator == nil {
		return fmt.Errorf("Coordinator未初始化，请检查配置")
	}

	// 单条消息模式
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		return singleMessage(cmd.Context(), myApp, prompt)
	}

	// 交互式模式
	return interactiveMode(cmd.Context(), myApp)
}

func singleMessage(ctx context.Context, myApp *app.App, prompt string) error {
	if sessionID == "" {
		sessionID = defaultSessionID
	}

	result, err := myApp.Coordinator.Run(ctx, sessionID, prompt)
	if err != nil {
		return fmt.Errorf("对话失败: %w", err)
	}

	// 流式输出并收集完整响应
	printAndCollect(result)

	if result.Error != nil {
		return fmt.Errorf("对话失败: %w", result.Error)
	}
	return nil
}

func interactiveMode(ctx context.Context, myApp *app.App) error {
	if sessionID == "" {
		sessionID = interactiveSessionID
	}

	fmt.Println("🤖 AICodeAgent 交互式对话模式（输入 /quit 退出）")
	fmt.Println("-------------------------------------------")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			fmt.Println("再见！")
			break
		}

		result, err := myApp.Coordinator.Run(ctx, sessionID, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			continue
		}

		printAndCollect(result)

		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", result.Error)
		}
	}

	return nil
}

// printAndCollect 流式打印并收集完整响应到result.Response
func printAndCollect(result *agent.AgentResult) {
	if result.Stream == nil {
		fmt.Println(result.Response)
		return
	}

	var sb strings.Builder
	for chunk := range result.Stream {
		if chunk.Error != nil {
			result.Error = chunk.Error
			fmt.Fprintf(os.Stderr, "\n流式错误: %v\n", chunk.Error)
			break
		}
		if chunk.Done {
			break
		}
		fmt.Print(chunk.Content)
		sb.WriteString(chunk.Content)
	}
	fmt.Println()
	result.Response = sb.String()
}
