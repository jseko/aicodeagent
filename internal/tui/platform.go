package tui

import (
	"os"
	"runtime"
)

// ColorDepth 终端颜色深度
type ColorDepth int

const (
	ColorDepthTrueColor ColorDepth = iota
	ColorDepth256
	ColorDepth16
)

// TerminalCapability 终端能力
type TerminalCapability struct {
	ColorDepth      ColorDepth
	SupportsUnicode bool
}

// DetectCapability 运行时检测终端能力（例9-17 终端能力检测）
func DetectCapability() TerminalCapability {
	cap := TerminalCapability{
		SupportsUnicode: detectUnicode(),
	}
	cap.ColorDepth = detectColorDepth()
	return cap
}

func detectColorDepth() ColorDepth {
	if os.Getenv("COLORTERM") == "truecolor" || os.Getenv("COLORTERM") == "24bit" {
		return ColorDepthTrueColor
	}
	term := os.Getenv("TERM")
	if term == "xterm-256color" || term == "screen-256color" {
		return ColorDepth256
	}
	return ColorDepth16
}

func detectUnicode() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	lang := os.Getenv("LANG")
	if lang != "" {
		return true
	}
	return false
}

// UnicodeFallback 渐进增强：Unicode 字符降级为 ASCII（例9-17）
func (cap TerminalCapability) UnicodeFallback(unicode, ascii string) string {
	if cap.SupportsUnicode {
		return unicode
	}
	return ascii
}

// PrepareBashEnvironment 准备跨平台环境变量
func PrepareBashEnvironment() []string {
	env := os.Environ()
	env = append(env, "TERM=xterm-256color")

	if runtime.GOOS == "windows" {
		env = append(env, "LC_ALL=C.UTF-8", "LANG=C.UTF-8")
	}

	return env
}

// GetDefaultShell 获取平台默认 Shell
func GetDefaultShell() (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := execLookPath("pwsh"); err == nil {
			return "pwsh", []string{"-Command"}
		}
		return "powershell", []string{"-Command"}
	}

	if _, err := execLookPath("bash"); err == nil {
		return "bash", []string{"-c"}
	}
	return "sh", []string{"-c"}
}

// execLookPath 包装 os/exec.LookPath 以便测试 mock
var execLookPath = func(file string) (string, error) {
	// 避免 import cycle，直接检查常见路径
	switch file {
	case "pwsh":
		if _, err := os.Stat("C:\\Program Files\\PowerShell\\7\\pwsh.exe"); err == nil {
			return "C:\\Program Files\\PowerShell\\7\\pwsh.exe", nil
		}
	case "bash":
		if _, err := os.Stat("/bin/bash"); err == nil {
			return "/bin/bash", nil
		}
	case "sh":
		if _, err := os.Stat("/bin/sh"); err == nil {
			return "/bin/sh", nil
		}
	}
	return "", os.ErrNotExist
}
