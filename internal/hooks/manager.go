package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type registeredHook struct {
	hook         Hook
	isExternal   bool
	registerTime int64
}

type Manager struct {
	mu    sync.RWMutex
	hooks map[EventType][]registeredHook
}

func NewManager() *Manager {
	return &Manager{hooks: make(map[EventType][]registeredHook)}
}

func (m *Manager) Register(hook Hook) {
	m.register(hook, false)
}

func (m *Manager) RegisterExternal(hook Hook) {
	m.register(hook, true)
}

func (m *Manager) register(hook Hook, external bool) {
	if hook == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UnixNano()
	for _, eventType := range hook.Types() {
		m.hooks[eventType] = append(m.hooks[eventType], registeredHook{
			hook:         hook,
			isExternal:   external || isExternalHook(hook),
			registerTime: now,
		})
		sort.SliceStable(m.hooks[eventType], func(i, j int) bool {
			left := m.hooks[eventType][i]
			right := m.hooks[eventType][j]
			if left.hook.Priority() != right.hook.Priority() {
				return left.hook.Priority() > right.hook.Priority()
			}
			return left.registerTime < right.registerTime
		})
	}
}

func (m *Manager) Trigger(ctx context.Context, eventType EventType, input interface{}, output interface{}) (*AggregateResult, error) {
	if m == nil {
		return &AggregateResult{Decision: DecisionNone}, nil
	}

	hooks := m.snapshot(eventType)
	if len(hooks) == 0 {
		return &AggregateResult{Decision: DecisionNone}, nil
	}

	var external []registeredHook
	for _, reg := range hooks {
		if !matchesTool(reg.hook, input) {
			continue
		}
		if reg.isExternal {
			external = append(external, reg)
			continue
		}
		if err := runInternal(ctx, reg.hook, input, output); err != nil {
			if reg.hook.Priority() >= PriorityBlocker {
				return &AggregateResult{Decision: DecisionDeny, Reason: err.Error(), HookCount: 1}, err
			}
		}
	}

	if len(external) == 0 {
		return &AggregateResult{Decision: DecisionNone}, nil
	}
	return m.runExternal(ctx, external, input, output)
}

func (m *Manager) snapshot(eventType EventType) []registeredHook {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make([]registeredHook, len(m.hooks[eventType]))
	copy(snapshot, m.hooks[eventType])
	return snapshot
}

func runInternal(ctx context.Context, hook Hook, input interface{}, output interface{}) error {
	if ioHook, ok := hook.(InputOutputHook); ok {
		return ioHook.Execute(ctx, input, output)
	}
	if eventHook, ok := hook.(EventHook); ok {
		return eventHook.OnEvent(ctx, input)
	}
	return nil
}

func (m *Manager) runExternal(ctx context.Context, hooks []registeredHook, input interface{}, output interface{}) (*AggregateResult, error) {
	results := make([]HookResult, len(hooks))
	var wg sync.WaitGroup
	for i, reg := range hooks {
		wg.Add(1)
		go func(index int, hook Hook) {
			defer wg.Done()
			result := HookResult{Decision: DecisionNone, HookName: hook.Name()}
			if ioHook, ok := hook.(InputOutputHook); ok {
				localOutput := cloneOutput(output)
				if err := ioHook.Execute(ctx, input, localOutput); err != nil {
					result.Decision = DecisionDeny
					result.Reason = err.Error()
				} else {
					result.UpdatedInput = extractUpdatedArgs(localOutput, output)
				}
			}
			results[index] = result
		}(i, reg.hook)
	}
	wg.Wait()

	agg, err := Aggregate(results)
	if err != nil {
		return agg, err
	}
	if agg.Decision == DecisionDeny {
		return agg, fmt.Errorf("external hook blocked: %s", agg.Reason)
	}
	if len(agg.UpdatedInput) > 0 {
		if err := applyUpdatedArgs(output, agg.UpdatedInput); err != nil {
			return agg, err
		}
	}
	return agg, nil
}

func Aggregate(results []HookResult) (*AggregateResult, error) {
	agg := &AggregateResult{Decision: DecisionNone, HookCount: len(results)}
	var updates []json.RawMessage
	for _, result := range results {
		if result.Reason != "" && agg.Reason == "" {
			agg.Reason = result.Reason
		}
		if result.Halt {
			agg.Decision = DecisionDeny
			agg.Halt = true
			if agg.Reason == "" {
				agg.Reason = result.Reason
			}
			continue
		}
		if result.Decision == DecisionDeny && !agg.Halt {
			agg.Decision = DecisionDeny
			if agg.Reason == "" {
				agg.Reason = result.Reason
			}
		}
		if len(result.UpdatedInput) > 0 {
			updates = append(updates, result.UpdatedInput)
		}
	}
	if agg.Decision == DecisionDeny {
		return agg, nil
	}
	updated, err := mergeJSONObjects(updates)
	if err != nil {
		return agg, err
	}
	agg.UpdatedInput = updated
	return agg, nil
}

func matchesTool(hook Hook, input interface{}) bool {
	matcher, ok := hook.(MatcherHook)
	if !ok {
		return true
	}
	toolName := toolNameFromInput(input)
	if toolName == "" {
		return true
	}
	return matcher.MatchTool(toolName)
}

func toolNameFromInput(input interface{}) string {
	switch typed := input.(type) {
	case ToolExecuteInput:
		return typed.Tool
	case *ToolExecuteInput:
		return typed.Tool
	case ToolExecuteAfterInput:
		return typed.Tool
	case *ToolExecuteAfterInput:
		return typed.Tool
	default:
		return ""
	}
}

func isExternalHook(hook Hook) bool {
	external, ok := hook.(ExternalHook)
	return ok && external.IsExternal()
}
