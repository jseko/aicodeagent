package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool 用于测试的简单工具实现
type mockTool struct {
	name        string
	description string
	params      json.RawMessage
	execute     func(ctx context.Context, params json.RawMessage) (Result, error)
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return m.description }
func (m *mockTool) Parameters() json.RawMessage { return m.params }
func (m *mockTool) Execute(ctx context.Context, p json.RawMessage) (Result, error) {
	return m.execute(ctx, p)
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test", description: "a test tool"}

	r.Register(tool)

	if _, ok := r.Get("test"); !ok {
		t.Fatal("Register: tool not found after registration")
	}
}

func TestRegistryRegisterDuplicatePanic(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "test"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Register: expected panic on duplicate registration")
		}
	}()
	r.Register(&mockTool{name: "test"})
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get: found non-existent tool")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "tool1", description: "first"})
	r.Register(&mockTool{name: "tool2", description: "second"})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List: expected 2 tools, got %d", len(list))
	}
}

func TestRegistryToOpenAIFunctions(t *testing.T) {
	r := NewRegistry()
	params := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	r.Register(&mockTool{
		name:        "view",
		description: "read file",
		params:      params,
	})

	funcs := r.ToOpenAIFunctions()
	if len(funcs) != 1 {
		t.Fatalf("ToOpenAIFunctions: expected 1, got %d", len(funcs))
	}
	if funcs[0].Name != "view" {
		t.Fatalf("ToOpenAIFunctions: expected name 'view', got '%s'", funcs[0].Name)
	}
}
