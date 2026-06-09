package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultDarkStyles(t *testing.T) {
	s := DefaultDarkStyles()

	if s.Primary != lipgloss.Color("#8B5CF6") {
		t.Errorf("Primary = %s, want #8B5CF6", s.Primary)
	}
	if s.Success != lipgloss.Color("#22C55E") {
		t.Errorf("Success = %s, want #22C55E", s.Success)
	}
	if s.Error != lipgloss.Color("#EF4444") {
		t.Errorf("Error = %s, want #EF4444", s.Error)
	}
	if s.BgBase != lipgloss.Color("#171717") {
		t.Errorf("BgBase = %s, want #171717", s.BgBase)
	}
}

func TestDefaultLightStyles(t *testing.T) {
	s := DefaultLightStyles()

	if s.BgBase != lipgloss.Color("#FAFAFA") {
		t.Errorf("BgBase = %s, want #FAFAFA", s.BgBase)
	}
	if s.FgBase != lipgloss.Color("#171717") {
		t.Errorf("FgBase = %s, want #171717", s.FgBase)
	}
}

func TestInitComponentStyles(t *testing.T) {
	s := DefaultDarkStyles()
	s.initComponentStyles()

	// 验证组件样式已初始化（通过渲染验证，lipgloss.Style 包含函数无法直接比较）
	rendered := s.Chat.UserMsg.Render("test")
	if rendered == "" {
		t.Error("UserMsg style should render content")
	}

	rendered = s.Chat.AIMsg.Render("test")
	if rendered == "" {
		t.Error("AIMsg style should render content")
	}

	rendered = s.Chat.ToolMsg.Render("test")
	if rendered == "" {
		t.Error("ToolMsg style should render content")
	}
}

func TestThemeManager_DefaultDark(t *testing.T) {
	tm := NewThemeManager()
	s := tm.GetStyles()

	if s.BgBase != lipgloss.Color("#171717") {
		t.Error("default mode should be dark")
	}
}

func TestThemeManager_SwitchToLight(t *testing.T) {
	tm := NewThemeManager()
	tm.SetMode(ThemeModeLight)
	s := tm.GetStyles()

	if s.BgBase != lipgloss.Color("#FAFAFA") {
		t.Error("should be light after switching")
	}
}

func TestThemeManager_Mode(t *testing.T) {
	tm := NewThemeManager()

	if tm.Mode() != ThemeModeDark {
		t.Error("default mode should be Dark")
	}

	tm.SetMode(ThemeModeLight)
	if tm.Mode() != ThemeModeLight {
		t.Error("mode should be Light")
	}
}

func TestChromaTheme(t *testing.T) {
	s := DefaultDarkStyles()
	theme := s.ChromaTheme()

	if theme == nil {
		t.Fatal("ChromaTheme should not be nil")
	}
	if theme.Name != "aicode" {
		t.Errorf("theme name = %s, want aicode", theme.Name)
	}
}

func TestApplyChromaTheme(t *testing.T) {
	s := DefaultDarkStyles()
	err := s.ApplyChromaTheme()
	if err != nil {
		t.Errorf("ApplyChromaTheme should not error, got: %v", err)
	}
}
