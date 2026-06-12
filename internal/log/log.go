package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	initialized atomic.Bool
	initOnce    sync.Once
	logDir      string
)

// Setup 初始化结构化日志（slog JSON handler + lumberjack 轮转）
// logFile: 日志文件路径，debug: 是否启用 debug 级别
func Setup(logFile string, debug bool) {
	initOnce.Do(func() {
		logDir = filepath.Dir(logFile)

		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}

		writer := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    10,  // MB
			MaxBackups: 3,
			MaxAge:     30,  // days
			Compress:   true,
		}

		handler := slog.NewJSONHandler(io.MultiWriter(os.Stderr, writer), &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		})

		slog.SetDefault(slog.New(handler))
		initialized.Store(true)

		slog.Info("日志系统初始化完成",
			"file", logFile,
			"level", level.String(),
			"os", runtime.GOOS,
			"arch", runtime.GOARCH,
		)
	})
}

// Initialized 返回日志是否已初始化
func Initialized() bool {
	return initialized.Load()
}

// LogDir 返回日志目录路径
func LogDir() string {
	return logDir
}
