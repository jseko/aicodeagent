package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SkillStep 技能步骤
type SkillStep struct {
	Name   string
	Status StepStatus
	Detail string
}

// StepStatus 步骤状态
type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepCompleted
	StepFailed
)

// SkillPlanMsg 技能计划消息
type SkillPlanMsg struct {
	SkillName string
	Steps     []SkillStep
}

// SkillStepStartMsg 步骤开始消息
type SkillStepStartMsg struct {
	StepIndex int
	Detail    string
}

// SkillStepCompleteMsg 步骤完成消息
type SkillStepCompleteMsg struct {
	StepIndex int
	Result    string
	Error     string
}

// SkillExecutionView 技能执行可视化组件（例9-14）
type SkillExecutionView struct {
	*BaseComponent
	skillName     string
	plan          []SkillStep
	currentStep   int
	progress      float64
	currentDetail string
	results       []string
}

func NewSkillExecutionView(styles *Styles) *SkillExecutionView {
	return &SkillExecutionView{
		BaseComponent: &BaseComponent{styles: styles},
		plan:          make([]SkillStep, 0),
		currentStep:   -1,
		results:       make([]string, 0),
	}
}

func (v *SkillExecutionView) Init() tea.Cmd { return nil }

func (v *SkillExecutionView) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case SkillPlanMsg:
		v.skillName = msg.SkillName
		v.plan = msg.Steps
		v.currentStep = -1
		v.progress = 0
		v.results = make([]string, 0)
		return v, nil

	case SkillStepStartMsg:
		v.currentStep = msg.StepIndex
		v.currentDetail = msg.Detail
		if msg.StepIndex >= 0 && msg.StepIndex < len(v.plan) {
			v.plan[msg.StepIndex].Status = StepRunning
			v.plan[msg.StepIndex].Detail = msg.Detail
		}
		return v, nil

	case SkillStepCompleteMsg:
		v.results = append(v.results, msg.Result)
		if len(v.plan) > 0 {
			v.progress = float64(len(v.results)) / float64(len(v.plan)) * 100
		}
		if msg.StepIndex >= 0 && msg.StepIndex < len(v.plan) {
			if msg.Error != "" {
				v.plan[msg.StepIndex].Status = StepFailed
				v.plan[msg.StepIndex].Detail = msg.Error
			} else {
				v.plan[msg.StepIndex].Status = StepCompleted
				v.plan[msg.StepIndex].Detail = msg.Result
			}
		}
		return v, nil
	}
	return v, nil
}

func (v *SkillExecutionView) View() string {
	var b strings.Builder

	// 标题
	title := "🔧 Skill Execution"
	if v.skillName != "" {
		title += ": " + v.skillName
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	b.WriteString("\n\n")

	// 步骤列表
	for i, step := range v.plan {
		icon := v.stepIcon(step.Status)
		style := v.stepStyle(step.Status)

		line := fmt.Sprintf("%s %d. %s", icon, i+1, step.Name)
		if step.Detail != "" {
			line += fmt.Sprintf(" — %s", step.Detail)
		}

		if i == v.currentStep {
			line = lipgloss.NewStyle().Foreground(v.styles.Primary).Render(line)
		}

		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	// 当前步骤详情
	if v.currentDetail != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(v.styles.FgMuted).Render("Current: " + v.currentDetail))
		b.WriteString("\n")
	}

	// 进度条
	if v.progress > 0 {
		b.WriteString("\n")
		b.WriteString(v.renderProgressBar())
	}

	return b.String()
}

func (v *SkillExecutionView) stepIcon(status StepStatus) string {
	switch status {
	case StepCompleted:
		return v.styles.Tool.IconSuccess.Render("[✓]")
	case StepRunning:
		return v.styles.Tool.IconRunning.Render("[>]")
	case StepFailed:
		return v.styles.Tool.IconError.Render("[✗]")
	default:
		return "[ ]"
	}
}

func (v *SkillExecutionView) stepStyle(status StepStatus) lipgloss.Style {
	switch status {
	case StepCompleted:
		return lipgloss.NewStyle().Foreground(v.styles.Success)
	case StepRunning:
		return lipgloss.NewStyle().Foreground(v.styles.Primary)
	case StepFailed:
		return lipgloss.NewStyle().Foreground(v.styles.Error)
	default:
		return lipgloss.NewStyle().Foreground(v.styles.FgMuted)
	}
}

func (v *SkillExecutionView) renderProgressBar() string {
	barWidth := 30
	filled := int(v.progress * float64(barWidth) / 100.0)
	if filled > barWidth {
		filled = barWidth
	}

	filledBar := strings.Repeat("█", filled)
	emptyBar := strings.Repeat("░", barWidth-filled)
	percent := fmt.Sprintf(" %.0f%%", v.progress)

	return lipgloss.NewStyle().Foreground(v.styles.Primary).Render(filledBar) +
		lipgloss.NewStyle().Foreground(v.styles.FgSubtle).Render(emptyBar+percent)
}

func (v *SkillExecutionView) SetPlan(plan []SkillStep) {
	v.plan = plan
}

func (v *SkillExecutionView) SetProgress(step int, progress float64) {
	v.currentStep = step
	v.progress = progress
	if step >= 0 && step < len(v.plan) {
		v.plan[step].Status = StepRunning
	}
}

func (v *SkillExecutionView) CompleteStep(step int) {
	if step >= 0 && step < len(v.plan) {
		v.plan[step].Status = StepCompleted
	}
}

func (v *SkillExecutionView) FailStep(step int, detail string) {
	if step >= 0 && step < len(v.plan) {
		v.plan[step].Status = StepFailed
		v.plan[step].Detail = detail
	}
}

func (v *SkillExecutionView) SetSize(width, height int) {
	v.width = width
	v.height = height
}
