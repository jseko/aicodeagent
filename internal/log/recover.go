package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

// RecoverPanic 在每个 goroutine 中通过 defer 调用，捕获 panic 并记录堆栈
// name: 组件名称（如 "coordinator", "eventloop"）
// cleanup: 可选的清理函数，在 panic 记录后执行
func RecoverPanic(name string, cleanup func()) {
	if r := recover(); r != nil {
		stack := string(debug.Stack())

		slog.Error("Panic recovered",
			"component", name,
			"panic", fmt.Sprintf("%v", r),
			"stack", stack,
		)

		// 写入独立 panic 日志文件
		writePanicLog(name, r, stack)

		if cleanup != nil {
			cleanup()
		}
	}
}

// writePanicLog 写入带时间戳的 panic 日志文件
func writePanicLog(name string, recovered any, stack string) {
	dir := getLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("aicodeagent-panic-%s-%s.log", name, timestamp)
	path := filepath.Join(dir, filename)

	content := fmt.Sprintf(
		"Panic Time: %s\nComponent: %s\nOS: %s\nArch: %s\nPanic: %v\n\nStack Trace:\n%s\n",
		time.Now().Format(time.RFC3339),
		name,
		runtime.GOOS,
		runtime.GOARCH,
		recovered,
		stack,
	)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		slog.Error("写入 panic 日志失败", "path", path, "error", err)
	}
}

// getLogDir 返回跨平台日志目录
// Unix: $XDG_DATA_HOME/aicodeagent/logs （回退到 ~/.local/share/aicodeagent/logs）
// Windows: %APPDATA%/AICodeAgent/logs
func getLogDir() string {
	if dir := logDir; dir != "" {
		return dir
	}

	var base string
	if runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		base = filepath.Join(base, "AICodeAgent")
	} else {
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				base = filepath.Join("/tmp", "aicodeagent")
			} else {
				base = filepath.Join(home, ".local", "share", "aicodeagent")
			}
		}
	}

	return filepath.Join(base, "logs")
}

// GetLogDir 公开 getLogDir 供外部使用
func GetLogDir() string {
	return getLogDir()
}

// SetLogDir 设置自定义日志目录（用于测试）
func SetLogDir(dir string) {
	logDir = dir
}
