package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// BashFilter 命令安全过滤器（第12章 工具安全护栏）
type BashFilter struct {
	safeCommands    map[string]bool
	bannedCommands  map[string]bool
	blockedPatterns []*regexp.Regexp
}

// NewBashFilter 创建过滤器，使用内置安全策略
func NewBashFilter() *BashFilter {
	f := &BashFilter{
		safeCommands:   make(map[string]bool),
		bannedCommands: make(map[string]bool),
	}
	for _, cmd := range defaultSafeCommands {
		f.safeCommands[cmd] = true
	}
	for _, cmd := range defaultBannedCommands {
		f.bannedCommands[cmd] = true
	}
	for _, pattern := range defaultBlockedPatterns {
		f.blockedPatterns = append(f.blockedPatterns, regexp.MustCompile(pattern))
	}
	return f
}

// AddSafeCommand 添加安全命令
func (f *BashFilter) AddSafeCommand(cmd string) { f.safeCommands[cmd] = true }

// AddBannedCommand 添加禁止命令
func (f *BashFilter) AddBannedCommand(cmd string) { f.bannedCommands[cmd] = true }

// FilterResult 过滤结果
type FilterResult int

const (
	FilterSafe    FilterResult = iota // 安全，直接执行
	FilterBlocked                     // 已阻止
	FilterGrey                        // 灰区，需确认
)

// Check 检查命令安全性（先封锁后安全后灰区，防止前缀绕过）
func (f *BashFilter) Check(command string) (FilterResult, string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return FilterSafe, ""
	}

	// 1. 检查危险参数模式（最高优先级，防止 git status && rm -rf / 绕过）
	for _, pattern := range f.blockedPatterns {
		if pattern.MatchString(cmd) {
			return FilterBlocked, fmt.Sprintf("命令包含危险模式: %s", pattern.String())
		}
	}

	// 2. 提取第一个命令名并检查
	baseCmd := extractBaseCommand(cmd)
	if f.bannedCommands[baseCmd] {
		return FilterBlocked, fmt.Sprintf("禁止执行的命令: %s", baseCmd)
	}

	// 3. 安全命令白名单
	if f.safeCommands[baseCmd] {
		return FilterSafe, ""
	}

	// 4. 未分类 → 灰区，需用户确认
	return FilterGrey, fmt.Sprintf("命令需要用户确认: %s", baseCmd)
}

func extractBaseCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	base := parts[0]
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	return strings.TrimSpace(base)
}

var defaultSafeCommands = []string{
	"ls", "cat", "head", "tail", "echo", "pwd",
	"find", "grep", "rg", "wc", "sort", "uniq",
	"git", "which", "type", "env", "printenv",
	"date", "true", "false",
}

var defaultBannedCommands = []string{
	"sudo", "su", "reboot", "shutdown", "halt",
	"mkfs", "dd", "fdisk", "parted",
	"mount", "umount", "chown", "chgrp",
	"kill", "killall", "pkill",
	"iptables", "ufw", "ip6tables",
	"passwd", "chpasswd",
}

var defaultBlockedPatterns = []string{
	`rm\s+.*-rf\s+/`,            // rm -rf /
	`>\s*/dev/`,                 // 写入设备文件
	`curl.*\|\s*(ba)?sh`,        // curl 管道执行
	`wget.*\|\s*(ba)?sh`,        // wget 管道执行
	`chmod\s+777\s+/`,           // 全局可写系统路径
	`chmod\s+-R\s+777`,          // 递归全局可写
	`mkfs\.`,                    // 格式化文件系统
	`dd\s+if=`,                  // dd 写入
	`>\s*/etc/`,                 // 写入 /etc
	`>\s*/boot/`,                // 写入 /boot
	`nc\s+.*-e\s`,               // netcat 反弹 shell
	`:\(\)\s*\{\s*:\|:&\s*\};:`, // fork bomb
}
