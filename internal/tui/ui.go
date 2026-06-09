package tui

import (
	"AICodeAgent/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// UI 主模型 - 组合所有子组件，完整 MVU 架构（例9-13）
type UI struct {
	Model
	components   []Component
	focusedIdx   int
	dialogs      *DialogManager
	eventAdapter *EventAdapter
}

func NewUI(a *app.App) *UI {
	return &UI{
		Model:        New(a),
		components:   make([]Component, 0),
		dialogs:      NewDialogManager(),
		eventAdapter: NewEventAdapter(),
	}
}

func (u *UI) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, u.Model.Init())
	for _, c := range u.components {
		cmds = append(cmds, c.Init())
	}
	return tea.Batch(cmds...)
}

func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Dialog 优先路由
	if u.dialogs.HasDialogs() {
		top := u.dialogs.Top()
		newComp, cmd := top.Update(msg)
		if newComp != nil {
			// 在 DialogManager 中更新 dialog
			if d, ok := newComp.(Dialog); ok {
				u.dialogs.CloseFront()
				u.dialogs.OpenDialog(d)
			}
		}
		return u, cmd
	}

	// 焦点组件路由
	if u.focusedIdx >= 0 && u.focusedIdx < len(u.components) {
		newComp, cmd := u.components[u.focusedIdx].Update(msg)
		if newComp != nil {
			u.components[u.focusedIdx] = newComp
		}
		return u, cmd
	}

	// 默认: 转发到 Model
	newModel, cmd := u.Model.Update(msg)
	if m, ok := newModel.(Model); ok {
		u.Model = m
	}
	return u, cmd
}

func (u *UI) View() string {
	if u.dialogs.HasDialogs() {
		base := u.Model.View()
		overlay := u.dialogs.Top().View()
		return base + "\n" + overlay
	}
	return u.Model.View()
}

func (u *UI) AddComponent(c Component) {
	u.components = append(u.components, c)
}

func (u *UI) FocusComponent(idx int) {
	if idx >= 0 && idx < len(u.components) {
		if u.focusedIdx >= 0 && u.focusedIdx < len(u.components) {
			u.components[u.focusedIdx].Blur()
		}
		u.focusedIdx = idx
		u.components[idx].Focus()
	}
}

func (u *UI) DialogManager() *DialogManager {
	return u.dialogs
}

func (u *UI) EventAdapter() *EventAdapter {
	return u.eventAdapter
}
