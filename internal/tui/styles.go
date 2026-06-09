package tui

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// ChatStyles 聊天消息样式（crush-style left border system）
type ChatStyles struct {
	UserMsg     lipgloss.Style
	AIMsg       lipgloss.Style
	ToolMsg     lipgloss.Style
	UserBlur    lipgloss.Style
	AIBlur      lipgloss.Style
	ToolBlur    lipgloss.Style
	ThinkingBox lipgloss.Style
}

// ToolStyles 工具状态样式
type ToolStyles struct {
	IconSuccess lipgloss.Style
	IconError   lipgloss.Style
	IconRunning lipgloss.Style
	IconPending lipgloss.Style
}

// Styles 语义化样式配置（用途而非具体颜色）
type Styles struct {
	Primary   lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Info      lipgloss.Color
	BgBase    lipgloss.Color
	BgSurface lipgloss.Color
	BgOverlay lipgloss.Color
	FgBase    lipgloss.Color
	FgMuted   lipgloss.Color
	FgSubtle  lipgloss.Color

	Chat ChatStyles
	Tool ToolStyles
}

// ThemeMode 主题模式
type ThemeMode int

const (
	ThemeModeDark ThemeMode = iota
	ThemeModeLight
	ThemeModeAuto
)

// DefaultDarkStyles 暗色主题（Charmtone 配色）
func DefaultDarkStyles() *Styles {
	s := &Styles{
		Primary:   lipgloss.Color("#8B5CF6"),
		Success:   lipgloss.Color("#22C55E"),
		Warning:   lipgloss.Color("#F59E0B"),
		Error:     lipgloss.Color("#EF4444"),
		Info:      lipgloss.Color("#3B82F6"),
		BgBase:    lipgloss.Color("#171717"),
		BgSurface: lipgloss.Color("#262626"),
		BgOverlay: lipgloss.Color("#1F1F1F"),
		FgBase:    lipgloss.Color("#D4D4D4"),
		FgMuted:   lipgloss.Color("#A3A3A3"),
		FgSubtle:  lipgloss.Color("#737373"),
	}
	s.initComponentStyles()
	return s
}

// DefaultLightStyles 亮色主题（Charmtone 配色）
func DefaultLightStyles() *Styles {
	s := &Styles{
		Primary:   lipgloss.Color("#7C3AED"),
		Success:   lipgloss.Color("#16A34A"),
		Warning:   lipgloss.Color("#D97706"),
		Error:     lipgloss.Color("#DC2626"),
		Info:      lipgloss.Color("#2563EB"),
		BgBase:    lipgloss.Color("#FAFAFA"),
		BgSurface: lipgloss.Color("#FFFFFF"),
		BgOverlay: lipgloss.Color("#F5F5F5"),
		FgBase:    lipgloss.Color("#171717"),
		FgMuted:   lipgloss.Color("#525252"),
		FgSubtle:  lipgloss.Color("#A3A3A3"),
	}
	s.initComponentStyles()
	return s
}

// initComponentStyles 基于语义色初始化组件样式（crush-style left border system）
func (s *Styles) initComponentStyles() {
	// 用户消息：主色圆角边框
	s.Chat.UserMsg = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(s.Primary).
		Padding(0, 1).
		MarginBottom(1)

	// AI 消息：成功色粗体左边框（crush focused style）
	s.Chat.AIMsg = lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(s.Success).
		PaddingLeft(1).
		MarginBottom(1)

	// 工具消息：警告色左边框
	s.Chat.ToolMsg = lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(s.Warning).
		PaddingLeft(1).
		MarginBottom(1)

	// 思考过程框：微妙背景色
	s.Chat.ThinkingBox = lipgloss.NewStyle().
		Background(s.BgSurface).
		Padding(0, 1).
		MarginBottom(1)

	s.Chat.UserBlur = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(s.FgSubtle).
		Padding(0, 1)

	s.Chat.AIBlur = lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(s.FgSubtle).
		PaddingLeft(1)

	s.Chat.ToolBlur = lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(s.FgSubtle).
		PaddingLeft(1)

	s.Tool.IconSuccess = lipgloss.NewStyle().Foreground(s.Success).Bold(true)
	s.Tool.IconError = lipgloss.NewStyle().Foreground(s.Error).Bold(true)
	s.Tool.IconRunning = lipgloss.NewStyle().Foreground(s.Primary)
	s.Tool.IconPending = lipgloss.NewStyle().Foreground(s.FgMuted)
}

// ChromaTheme 自定义语法高亮样式（例9-8）
func (s *Styles) ChromaTheme() *chroma.Style {
	return chroma.MustNewStyle("aicode", chroma.StyleEntries{
		chroma.Text:            string(s.Primary),
		chroma.Comment:         "italic " + string(s.FgSubtle),
		chroma.Keyword:         "bold " + string(s.Primary),
		chroma.KeywordType:     string(s.Primary),
		chroma.NameFunction:    string(s.Success),
		chroma.NameClass:       "bold " + string(s.Warning),
		chroma.LiteralString:   string(s.Error),
		chroma.LiteralNumber:   string(s.Info),
		chroma.GenericDeleted:  string(s.Error),
		chroma.GenericInserted: string(s.Success),
	})
}

// ApplyChromaTheme 应用自定义主题到全局
func (s *Styles) ApplyChromaTheme() error {
	styles.Register(s.ChromaTheme())
	return nil
}

// ThemeManager 主题管理器
type ThemeManager struct {
	dark  *Styles
	light *Styles
	mode  ThemeMode
}

// NewThemeManager 创建主题管理器
func NewThemeManager() *ThemeManager {
	return &ThemeManager{
		dark:  DefaultDarkStyles(),
		light: DefaultLightStyles(),
		mode:  ThemeModeDark,
	}
}

// GetStyles 获取当前主题样式
func (tm *ThemeManager) GetStyles() *Styles {
	switch tm.mode {
	case ThemeModeLight:
		return tm.light
	case ThemeModeAuto:
		if tm.isTerminalLight() {
			return tm.light
		}
		return tm.dark
	default:
		return tm.dark
	}
}

// SetMode 设置主题模式
func (tm *ThemeManager) SetMode(mode ThemeMode) {
	tm.mode = mode
}

// Mode 获取当前模式
func (tm *ThemeManager) Mode() ThemeMode {
	return tm.mode
}

// isTerminalLight 检测终端是否为亮色背景
func (tm *ThemeManager) isTerminalLight() bool {
	return false
}
