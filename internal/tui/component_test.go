package tui

import (
	"testing"
)

func TestBaseComponent_SetSize(t *testing.T) {
	b := &BaseComponent{}
	b.SetSize(80, 24)

	if b.Width() != 80 {
		t.Errorf("width = %d, want 80", b.Width())
	}
	if b.Height() != 24 {
		t.Errorf("height = %d, want 24", b.Height())
	}
}

func TestBaseComponent_Focus(t *testing.T) {
	b := &BaseComponent{}
	if b.IsFocused() {
		t.Error("should not be focused initially")
	}

	b.Focus()
	if !b.IsFocused() {
		t.Error("should be focused after Focus()")
	}
}

func TestBaseComponent_Blur(t *testing.T) {
	b := &BaseComponent{}
	b.Focus()
	b.Blur()

	if b.IsFocused() {
		t.Error("should not be focused after Blur()")
	}
}

func TestBaseComponent_Init(t *testing.T) {
	b := &BaseComponent{}
	cmd := b.Init()
	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestBaseComponent_ImplementsComponent(t *testing.T) {
	var c Component = &BaseComponent{}
	_ = c
}
