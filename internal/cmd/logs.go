package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"AICodeAgent/internal/log"

	"github.com/spf13/cobra"
)

var (
	logFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "查看日志文件",
	Long: `查看 AICodeAgent 日志文件。

默认：显示最近 100 行日志。
使用 -f/--follow 实时跟踪日志输出。

示例：
  AICodeAgent logs
  AICodeAgent logs -f`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "实时跟踪日志输出（类似 tail -f）")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	logDir := log.GetLogDir()
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) == 0 {
		fmt.Println("暂无日志文件。")
		fmt.Printf("日志目录: %s\n", logDir)
		return nil
	}

	// 找到最新的日志文件
	var latest string
	var latestTime int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().Unix()
		if modTime > latestTime {
			latestTime = modTime
			latest = entry.Name()
		}
	}

	if latest == "" {
		fmt.Println("暂无日志文件。")
		return nil
	}

	logFile := logDir + "/" + latest

	switch runtime.GOOS {
	case "windows":
		return runLogsWindows(logFile)
	default:
		return runLogsUnix(logFile)
	}
}

func runLogsUnix(logFile string) error {
	args := []string{"-100", logFile}
	if logFollow {
		args = []string{"-f", logFile}
	}

	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runLogsWindows(logFile string) error {
	var args []string
	if logFollow {
		args = []string{"-Command", fmt.Sprintf("Get-Content '%s' -Wait", logFile)}
	} else {
		args = []string{"-Command", fmt.Sprintf("Get-Content '%s' -Tail 100", logFile)}
	}

	cmd := exec.Command("powershell", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
