package tui

// Dialog 对话框接口（ch9.md 9.3 intro）
type Dialog interface {
	Component
	ID() string
}

// DialogManager Overlay 栈管理器
type DialogManager struct {
	dialogs []Dialog
}

func NewDialogManager() *DialogManager {
	return &DialogManager{dialogs: make([]Dialog, 0)}
}

func (dm *DialogManager) OpenDialog(d Dialog) {
	dm.dialogs = append(dm.dialogs, d)
}

func (dm *DialogManager) CloseFront() {
	if len(dm.dialogs) > 0 {
		dm.dialogs = dm.dialogs[:len(dm.dialogs)-1]
	}
}

func (dm *DialogManager) CloseDialog(id string) {
	for i, d := range dm.dialogs {
		if d.ID() == id {
			dm.dialogs = append(dm.dialogs[:i], dm.dialogs[i+1:]...)
			return
		}
	}
}

func (dm *DialogManager) Top() Dialog {
	if len(dm.dialogs) == 0 {
		return nil
	}
	return dm.dialogs[len(dm.dialogs)-1]
}

func (dm *DialogManager) HasDialogs() bool {
	return len(dm.dialogs) > 0
}
