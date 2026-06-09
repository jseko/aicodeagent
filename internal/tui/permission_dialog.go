package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RiskLevel 风险等级
type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
)

// PermissionAction 权限操作
type PermissionAction int

const (
	ActionAllow PermissionAction = iota
	ActionAllowSession
	ActionDeny
)

// PermissionType 权限类型
type PermissionType int

const (
	PermissionTypeBash PermissionType = iota
	PermissionTypeFileWrite
	PermissionTypeFileDelete
)

// PermissionRequest 权限请求
type PermissionRequest struct {
	ID      string
	Type    PermissionType
	Command string
	Target  string
	Reason  string
}

// PermissionResponseMsg 权限响应消息
type PermissionResponseMsg struct {
	RequestID string
	Action    PermissionAction
}

// DialogOption 对话框选项
type DialogOption struct {
	Key    string
	Label  string
	Action PermissionAction
}

// PermissionDialog 权限确认对话框（例9-11/例9-12）
type PermissionDialog struct {
	*BaseComponent
	req      PermissionRequest
	risk     RiskLevel
	options  []DialogOption
	selected int
}

func NewPermissionDialog(req PermissionRequest, styles *Styles) *PermissionDialog {
	return &PermissionDialog{
		BaseComponent: &BaseComponent{styles: styles},
		req:           req,
		risk:          calculateRisk(req),
		options: []DialogOption{
			{Key: "y", Label: "Allow Once", Action: ActionAllow},
			{Key: "a", Label: "Allow Session", Action: ActionAllowSession},
			{Key: "n", Label: "Deny", Action: ActionDeny},
		},
	}
}

func (d *PermissionDialog) ID() string { return "perm-" + d.req.ID }

func (d *PermissionDialog) Init() tea.Cmd { return nil }

func (d *PermissionDialog) Update(msg tea.Msg) (Component, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	switch k.String() {
	case "y", "Y":
		return d, d.respond(ActionAllow)
	case "a", "A":
		return d, d.respond(ActionAllowSession)
	case "n", "N", "esc":
		return d, d.respond(ActionDeny)
	case "left":
		if d.selected > 0 {
			d.selected--
		}
	case "right":
		if d.selected < len(d.options)-1 {
			d.selected++
		}
	}

	return d, nil
}

func (d *PermissionDialog) respond(a PermissionAction) tea.Cmd {
	return func() tea.Msg {
		return PermissionResponseMsg{RequestID: d.req.ID, Action: a}
	}
}

func (d *PermissionDialog) View() string {
	riskStyle := d.riskStyle()
	riskLabel := d.riskLabel()

	var b strings.Builder
	b.WriteString(riskStyle.Render(fmt.Sprintf("⚠ %s Risk: %s", riskLabel, d.req.Command)))
	b.WriteString("\n\n")

	if d.req.Reason != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(d.styles.FgMuted).Render("Reason: " + d.req.Reason))
		b.WriteString("\n\n")
	}

	// 选项
	for i, opt := range d.options {
		prefix := "  "
		if i == d.selected {
			prefix = "▸ "
		}
		line := fmt.Sprintf("%s[%s] %s", prefix, opt.Key, opt.Label)
		if i == d.selected {
			line = lipgloss.NewStyle().Foreground(d.styles.Primary).Render(line)
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

func (d *PermissionDialog) riskStyle() lipgloss.Style {
	switch d.risk {
	case RiskHigh:
		return lipgloss.NewStyle().Foreground(d.styles.Error).Bold(true)
	case RiskMedium:
		return lipgloss.NewStyle().Foreground(d.styles.Warning)
	default:
		return lipgloss.NewStyle().Foreground(d.styles.Success)
	}
}

func (d *PermissionDialog) riskLabel() string {
	switch d.risk {
	case RiskHigh:
		return "HIGH"
	case RiskMedium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func calculateRisk(req PermissionRequest) RiskLevel {
	if req.Type == PermissionTypeBash {
		cmd := strings.ToLower(req.Command)
		if strings.Contains(cmd, "rm -rf") {
			return RiskHigh
		}
		if strings.Contains(cmd, "sudo") {
			return RiskMedium
		}
	}
	if req.Type == PermissionTypeFileDelete {
		return RiskHigh
	}
	if req.Type == PermissionTypeFileWrite {
		return RiskMedium
	}
	return RiskLow
}
