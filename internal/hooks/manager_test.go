package hooks

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
)

type testIOHook struct {
	name     string
	types    []EventType
	priority Priority
	matcher  string
	err      error
	calls    *atomic.Int64
}

func (h *testIOHook) Name() string       { return h.name }
func (h *testIOHook) Types() []EventType  { return h.types }
func (h *testIOHook) Priority() Priority  { return h.priority }
func (h *testIOHook) MatchTool(tool string) bool {
	return h.matcher == "" || h.matcher == tool
}
func (h *testIOHook) Execute(ctx context.Context, input interface{}, output interface{}) error {
	if h.calls != nil {
		h.calls.Add(1)
	}
	return h.err
}

type testEventHook struct {
	name     string
	types    []EventType
	priority Priority
	err      error
	calls    *atomic.Int64
}

func (h *testEventHook) Name() string      { return h.name }
func (h *testEventHook) Types() []EventType { return h.types }
func (h *testEventHook) Priority() Priority { return h.priority }
func (h *testEventHook) OnEvent(ctx context.Context, event interface{}) error {
	if h.calls != nil {
		h.calls.Add(1)
	}
	return h.err
}

func TestEventTypes(t *testing.T) {
	want := []EventType{EventSessionStart, EventSessionFinish, EventToolExecuteBefore, EventToolExecuteAfter, EventError}
	got := []EventType{"session.start", "session.finish", "tool.execute.before", "tool.execute.after", "event.error"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("event constants mismatch: %#v", want)
	}
}

func TestManagerTriggerNoHooks(t *testing.T) {
	agg, err := NewManager().Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{}, &ToolExecuteOutput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agg.Decision != DecisionNone {
		t.Fatalf("unexpected decision: %s", agg.Decision)
	}
}

func TestManagerPriorityAndRegistrationOrder(t *testing.T) {
	var order []string
	m := NewManager()
	m.Register(callbackHook{name: "observer", priority: PriorityObserver, fn: func() { order = append(order, "observer") }})
	m.Register(callbackHook{name: "blocker", priority: PriorityBlocker, fn: func() { order = append(order, "blocker") }})
	m.Register(callbackHook{name: "normal-a", priority: PriorityNormal, fn: func() { order = append(order, "normal-a") }})
	m.Register(callbackHook{name: "normal-b", priority: PriorityNormal, fn: func() { order = append(order, "normal-b") }})

	_, err := m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"blocker", "normal-a", "normal-b", "observer"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order mismatch: got %v want %v", order, want)
	}
}

func TestManagerBlockerErrorStops(t *testing.T) {
	calls := &atomic.Int64{}
	m := NewManager()
	m.Register(&testIOHook{name: "blocker", types: []EventType{EventToolExecuteBefore}, priority: PriorityBlocker, err: errors.New("blocked")})
	m.Register(&testIOHook{name: "normal", types: []EventType{EventToolExecuteBefore}, priority: PriorityNormal, calls: calls})

	agg, err := m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{})
	if err == nil || agg.Decision != DecisionDeny {
		t.Fatalf("expected deny error, got agg=%#v err=%v", agg, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("normal hook should not run after blocker failure")
	}
}

func TestManagerObserverErrorDoesNotStop(t *testing.T) {
	calls := &atomic.Int64{}
	m := NewManager()
	m.Register(&testEventHook{name: "observer", types: []EventType{EventToolExecuteBefore}, priority: PriorityObserver, err: errors.New("observe failed")})
	m.Register(&testIOHook{name: "normal", types: []EventType{EventToolExecuteBefore}, priority: PriorityNormal, calls: calls})

	_, err := m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{})
	if err != nil {
		t.Fatalf("observer error should not block: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("normal hook should run once")
	}
}

func TestManagerMatcherFiltering(t *testing.T) {
	calls := &atomic.Int64{}
	m := NewManager()
	m.Register(&testIOHook{name: "view-only", types: []EventType{EventToolExecuteBefore}, priority: PriorityNormal, matcher: "view", calls: calls})

	_, err := m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "bash"}, &ToolExecuteOutput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("matcher should skip bash")
	}

	_, err = m.Trigger(context.Background(), EventToolExecuteBefore, &ToolExecuteInput{Tool: "view"}, &ToolExecuteOutput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("matcher should run for view")
	}
}

type callbackHook struct {
	name     string
	priority Priority
	fn       func()
}

func (h callbackHook) Name() string       { return h.name }
func (h callbackHook) Types() []EventType  { return []EventType{EventToolExecuteBefore} }
func (h callbackHook) Priority() Priority  { return h.priority }
func (h callbackHook) Execute(ctx context.Context, input interface{}, output interface{}) error {
	h.fn()
	return nil
}
