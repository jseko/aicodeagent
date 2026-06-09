package tui

import tea "github.com/charmbracelet/bubbletea"

// Component TUI组件接口 - 核心抽象
type Component interface {
	Init() tea.Cmd
	Update(tea.Msg) (Component, tea.Cmd)
	View() string
	SetSize(width, height int)
	Focus() Component
	Blur() Component
	IsFocused() bool
}

// BaseComponent 基础组件实现
type BaseComponent struct {
	width   int
	height  int
	focused bool
	styles  *Styles
}

func (b *BaseComponent) Init() tea.Cmd                  { return nil }
func (b *BaseComponent) Update(msg tea.Msg) (Component, tea.Cmd) { return b, nil }
func (b *BaseComponent) View() string                   { return "" }
func (b *BaseComponent) SetSize(width, height int)       { b.width = width; b.height = height }
func (b *BaseComponent) Focus() Component                { b.focused = true; return b }
func (b *BaseComponent) Blur() Component                 { b.focused = false; return b }
func (b *BaseComponent) IsFocused() bool                 { return b.focused }
func (b *BaseComponent) Width() int                      { return b.width }
func (b *BaseComponent) Height() int                     { return b.height }
