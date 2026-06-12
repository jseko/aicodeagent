package tui

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// copySelectedText 将文本写入剪贴板（OSC 52 + 原生命令双保险）
func copySelectedText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		fmt.Printf("\x1b]52;c;%s\x07", encoded)
		writeNativeClipboard(text)
		return nil
	}
}

// writeNativeClipboard 使用操作系统原生命令写入剪贴板
func writeNativeClipboard(text string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	}
	if cmd == nil {
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	go func() {
		defer stdin.Close()
		stdin.Write([]byte(text))
	}()
	cmd.Run()
}
