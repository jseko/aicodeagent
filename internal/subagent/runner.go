package subagent

import (
	"context"

	"AICodeAgent/internal/agent"
	"AICodeAgent/internal/agent/tools"
)

type RunnerAdapter struct {
	registry  *SubagentRegistry
	coord     *SubagentCoordinator
	toolReg   *tools.Registry
	threshold int
	getRunner func() agent.SessionAgent
	observer  agent.SubagentExecutionObserver
}

func NewRunnerAdapter(registry *SubagentRegistry, coord *SubagentCoordinator, toolReg *tools.Registry, threshold int, getRunner func() agent.SessionAgent) *RunnerAdapter {
	if threshold <= 0 {
		threshold = 2
	}
	return &RunnerAdapter{registry: registry, coord: coord, toolReg: toolReg, threshold: threshold, getRunner: getRunner}
}

func (r *RunnerAdapter) SetObserver(observer agent.SubagentExecutionObserver) {
	if r == nil {
		return
	}
	r.observer = observer
}

func (r *RunnerAdapter) List() []agent.SubagentInfo {
	if r == nil || r.registry == nil {
		return nil
	}
	agents := r.registry.List()
	infos := make([]agent.SubagentInfo, 0, len(agents))
	for _, sub := range agents {
		desc := ""
		if sub.Config != nil {
			desc = sub.Config.Description
		}
		infos = append(infos, agent.SubagentInfo{Name: sub.Config.Name, Description: desc})
	}
	return infos
}

func (r *RunnerAdapter) Execute(ctx context.Context, name string, session *agent.Session, input string) (string, error) {
	if r == nil || r.getRunner == nil {
		return "", agent.ErrSubagentNotAvailable
	}
	runner := r.getRunner()
	if runner == nil {
		return "", agent.ErrSubagentNotAvailable
	}
	exec := NewExecutor(runner, r.registry, r.coord, r.toolReg)
	exec.SetObserver(r.observer)
	parentID := ""
	if session != nil {
		parentID = session.ID
	}
	res, err := exec.Execute(ctx, ExecuteRequest{
		ParentSession: session,
		ParentID:      parentID,
		SubagentName:  name,
		Input:         input,
	})
	if err != nil {
		return "", err
	}
	return res.Summary, nil
}

func (r *RunnerAdapter) Match(input string) (string, bool) {
	if r == nil || r.registry == nil {
		return "", false
	}
	result := MatchSubagent(input, r.registry.List(), r.threshold)
	if result.Subagent == nil || result.Subagent.Config == nil {
		return "", false
	}
	return result.Subagent.Config.Name, true
}
