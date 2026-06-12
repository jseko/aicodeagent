package subagent

import "time"

type ToolCallTrace struct {
	ToolName string
	Duration time.Duration
	Allowed  bool
	Error    string
}

type TraceReport struct {
	SubagentName     string
	ParentSessionID  string
	ChildSessionID   string
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
	ToolCallCount    int
	PromptTokens     int64
	CompletionTokens int64
	Cost             float64
}

type ExecutionTracer struct {
	SubagentName     string
	ParentSessionID  string
	ChildSessionID   string
	StartTime        time.Time
	EndTime          time.Time
	ToolCalls        []ToolCallTrace
	PromptTokens     int64
	CompletionTokens int64
	Cost             float64
}

func NewExecutionTracer(subagentName, parentSessionID, childSessionID string) *ExecutionTracer {
	return &ExecutionTracer{
		SubagentName:    subagentName,
		ParentSessionID: parentSessionID,
		ChildSessionID:  childSessionID,
		StartTime:       time.Now(),
		ToolCalls:       make([]ToolCallTrace, 0),
	}
}

func (t *ExecutionTracer) RecordToolCall(toolName string, duration time.Duration, allowed bool, err error) {
	trace := ToolCallTrace{ToolName: toolName, Duration: duration, Allowed: allowed}
	if err != nil {
		trace.Error = err.Error()
	}
	t.ToolCalls = append(t.ToolCalls, trace)
}

func (t *ExecutionTracer) RecordTokenUsage(promptTokens, completionTokens int64, cost float64) {
	t.PromptTokens += promptTokens
	t.CompletionTokens += completionTokens
	t.Cost += cost
}

func (t *ExecutionTracer) Finish() TraceReport {
	t.EndTime = time.Now()
	return TraceReport{
		SubagentName:     t.SubagentName,
		ParentSessionID:  t.ParentSessionID,
		ChildSessionID:   t.ChildSessionID,
		StartTime:        t.StartTime,
		EndTime:          t.EndTime,
		Duration:         t.EndTime.Sub(t.StartTime),
		ToolCallCount:    len(t.ToolCalls),
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		Cost:             t.Cost,
	}
}
