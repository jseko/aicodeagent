package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PreviewLoadedMsg 预览加载完成消息
type PreviewLoadedMsg struct {
	Path    string
	Content string
}

// FilePicker 文件选择器组件（例9-9）
type FilePicker struct {
	*BaseComponent
	currentDir  string
	selectedIdx int
	entries     []fs.DirEntry

	showPreview    bool
	previewPath    string
	previewContent string

	onSelect func(path string) tea.Cmd
}

func NewFilePicker(dir string, styles *Styles) *FilePicker {
	fp := &FilePicker{
		BaseComponent: &BaseComponent{styles: styles},
		currentDir:    dir,
		showPreview:   false,
	}
	fp.loadDir()
	return fp
}

func (fp *FilePicker) loadDir() {
	entries, err := os.ReadDir(fp.currentDir)
	if err != nil {
		fp.entries = nil
		return
	}
	fp.entries = entries
	fp.selectedIdx = 0
}

func (fp *FilePicker) Init() tea.Cmd { return nil }

func (fp *FilePicker) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return fp.handleKey(msg)
	case PreviewLoadedMsg:
		if msg.Path == fp.previewPath {
			fp.previewContent = msg.Content
		}
		return fp, nil
	}
	return fp, nil
}

func (fp *FilePicker) handleKey(k tea.KeyMsg) (Component, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if fp.selectedIdx > 0 {
			fp.selectedIdx--
		}
	case "down", "j":
		if fp.selectedIdx < len(fp.entries)-1 {
			fp.selectedIdx++
		}
	case "enter":
		return fp, fp.selectCurrent()
	case "space":
		fp.showPreview = !fp.showPreview
		if fp.showPreview && fp.selectedIdx < len(fp.entries) {
			return fp, fp.loadPreview(fp.currentPath())
		}
	case "esc":
		if fp.onSelect != nil {
			return fp, fp.onSelect("")
		}
	}
	return fp, nil
}

func (fp *FilePicker) selectCurrent() tea.Cmd {
	if fp.selectedIdx >= len(fp.entries) {
		return nil
	}
	entry := fp.entries[fp.selectedIdx]
	path := filepath.Join(fp.currentDir, entry.Name())

	if entry.IsDir() {
		fp.currentDir = path
		fp.loadDir()
		return nil
	}

	if fp.onSelect != nil {
		return fp.onSelect(path)
	}
	return nil
}

func (fp *FilePicker) currentPath() string {
	if fp.selectedIdx >= len(fp.entries) {
		return fp.currentDir
	}
	return filepath.Join(fp.currentDir, fp.entries[fp.selectedIdx].Name())
}

func (fp *FilePicker) loadPreview(path string) tea.Cmd {
	const maxPreviewSize = 100 * 1024
	return func() tea.Msg {
		st, err := os.Stat(path)
		if err != nil || st.Size() > maxPreviewSize {
			return PreviewLoadedMsg{Path: path, Content: "[preview skipped]"}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return PreviewLoadedMsg{Path: path, Content: "[read error]"}
		}
		return PreviewLoadedMsg{Path: path, Content: string(b)}
	}
}

func (fp *FilePicker) View() string {
	var b strings.Builder

	// 当前目录
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("📁 " + fp.currentDir))
	b.WriteString("\n")

	// 文件列表
	for i, entry := range fp.entries {
		prefix := "  "
		if i == fp.selectedIdx {
			prefix = "▸ "
		}

		name := entry.Name()
		if entry.IsDir() {
			name = lipgloss.NewStyle().Foreground(fp.styles.Info).Render(name + "/")
		}

		line := prefix + name
		if i == fp.selectedIdx {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		b.WriteString(line + "\n")
	}

	// 预览区
	if fp.showPreview && fp.previewContent != "" {
		b.WriteString("\n── 预览 ──\n")
		b.WriteString(fp.previewContent)
	}

	return b.String()
}

func (fp *FilePicker) SetSize(width, height int) {
	fp.width = width
	fp.height = height
}

func (fp *FilePicker) OnSelect(fn func(path string) tea.Cmd) {
	fp.onSelect = fn
}
