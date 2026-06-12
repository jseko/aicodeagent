package hooks

import (
	"context"
	"fmt"
)

type Bridge struct {
	manager  *Manager
	executor *Executor
}

func NewBridge(manager *Manager, executor *Executor) *Bridge {
	if executor == nil {
		executor = NewExecutor()
	}
	return &Bridge{manager: manager, executor: executor}
}

func (b *Bridge) LoadAndRegister(configs []ExternalHookConfig) error {
	if b == nil || b.manager == nil {
		return fmt.Errorf("hook bridge requires manager")
	}
	validated, err := ValidateExternalHooks(configs)
	if err != nil {
		return err
	}
	for i := range validated {
		cfg := validated[i]
		b.manager.RegisterExternal(&externalHookWrapper{external: cfg, executor: b.executor})
	}
	return nil
}

func (b *Bridge) LoadFilesAndRegister(paths []string) error {
	configs, err := LoadConfigFiles(paths)
	if err != nil {
		return err
	}
	return b.LoadAndRegister(configs)
}

type externalHookWrapper struct {
	external ExternalHookConfig
	executor *Executor
}

func (w *externalHookWrapper) Name() string      { return w.external.Name }
func (w *externalHookWrapper) Types() []EventType { return w.external.Types }
func (w *externalHookWrapper) Priority() Priority { return w.external.Priority }
func (w *externalHookWrapper) IsExternal() bool   { return true }
func (w *externalHookWrapper) MatchTool(toolName string) bool {
	return w.external.MatchTool(toolName)
}

func (w *externalHookWrapper) Execute(ctx context.Context, input interface{}, output interface{}) error {
	eventType := firstEventType(w.external.Types)
	result, err := w.executor.ExecuteExternal(ctx, &w.external, eventType, input, output)
	if err != nil {
		return err
	}
	if len(result.UpdatedInput) > 0 {
		if out, ok := output.(*ToolExecuteOutput); ok {
			out.Args = cloneRawMessage(result.UpdatedInput)
		}
	}
	if result.Decision == DecisionDeny {
		if result.Reason == "" {
			result.Reason = "external hook rejected"
		}
		return fmt.Errorf("%s", result.Reason)
	}
	return nil
}

func firstEventType(types []EventType) EventType {
	if len(types) == 0 {
		return ""
	}
	return types[0]
}
