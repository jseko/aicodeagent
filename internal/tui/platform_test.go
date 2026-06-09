package tui

import (
	"runtime"
	"testing"
)

func TestDetectCapability(t *testing.T) {
	cap := DetectCapability()

	// 验证返回有效的 ColorDepth 值
	if cap.ColorDepth < ColorDepthTrueColor || cap.ColorDepth > ColorDepth16 {
		t.Errorf("invalid color depth: %d", cap.ColorDepth)
	}
}

func TestUnicodeFallback_Supported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	cap := TerminalCapability{SupportsUnicode: true}

	result := cap.UnicodeFallback("✓", "[OK]")
	if result != "✓" {
		t.Errorf("should return unicode when supported, got: %s", result)
	}
}

func TestUnicodeFallback_Unsupported(t *testing.T) {
	cap := TerminalCapability{SupportsUnicode: false}

	result := cap.UnicodeFallback("✓", "[OK]")
	if result != "[OK]" {
		t.Errorf("should return ASCII fallback, got: %s", result)
	}

	result = cap.UnicodeFallback("✗", "[ERROR]")
	if result != "[ERROR]" {
		t.Errorf("should return ASCII fallback for error, got: %s", result)
	}
}

func TestPrepareBashEnvironment(t *testing.T) {
	env := PrepareBashEnvironment()

	foundTerm := false
	for _, e := range env {
		if e == "TERM=xterm-256color" {
			foundTerm = true
			break
		}
	}

	if !foundTerm {
		t.Error("TERM=xterm-256color should be set")
	}
}

func TestGetDefaultShell_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}

	shell, args := GetDefaultShell()
	if shell == "" {
		t.Error("shell should not be empty")
	}
	if len(args) == 0 {
		t.Error("args should not be empty")
	}
}

func TestColorDepth_Values(t *testing.T) {
	if int(ColorDepthTrueColor) != 0 {
		t.Error("TrueColor should be 0")
	}
	if int(ColorDepth256) != 1 {
		t.Error("256 should be 1")
	}
	if int(ColorDepth16) != 2 {
		t.Error("16 should be 2")
	}
}
