package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// execCmd 创建带上下文控制的命令
func execCmd(ctx context.Context, command string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd
}

// mapToEnvPairs 将 map 转换为环境变量格式（KEY=VALUE）
func mapToEnvPairs(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// resolveEnvVars 解析字符串中的环境变量引用（$VAR 或 ${VAR}）
func resolveEnvVars(s string) string {
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		s = strings.ReplaceAll(s, "$"+pair[0], pair[1])
		s = strings.ReplaceAll(s, "${"+pair[0]+"}", pair[1])
	}
	return s
}
