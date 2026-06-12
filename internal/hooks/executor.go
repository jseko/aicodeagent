package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const HaltExitCode = 49

type Executor struct{}

type ExternalRequest struct {
	Event  EventType              `json:"event"`
	Input  interface{}            `json:"input"`
	Output interface{}            `json:"output"`
	Config map[string]interface{} `json:"config,omitempty"`
}

type hookResponse struct {
	Decision           Decision             `json:"decision"`
	Reason             string               `json:"reason"`
	Halt               bool                 `json:"halt"`
	UpdatedInput        json.RawMessage      `json:"updated_input"`
	Output              json.RawMessage      `json:"output"`
	HookSpecificOutput  hookSpecificResponse `json:"hookSpecificOutput"`
}

type hookSpecificResponse struct {
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func NewExecutor() *Executor { return &Executor{} }

func (e *Executor) ExecuteExternal(ctx context.Context, hook *ExternalHookConfig, event EventType, input interface{}, output interface{}) (HookResult, error) {
	if hook == nil {
		return HookResult{Decision: DecisionNone}, nil
	}
	payload, err := json.Marshal(ExternalRequest{Event: event, Input: input, Output: output, Config: hook.Config})
	if err != nil {
		return HookResult{Decision: DecisionNone, HookName: hook.Name}, err
	}

	timeout := hook.Timeout
	if timeout <= 0 {
		timeout = DefaultExternalTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, hook.Command, hook.Args...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		return HookResult{Decision: DecisionNone, HookName: hook.Name}, fmt.Errorf("external hook timeout: %s", hook.Name)
	}
	if err != nil {
		return resultFromExitError(hook.Name, err, stderr.String())
	}

	result, err := ParseResponse(stdout.Bytes())
	result.HookName = hook.Name
	if err != nil {
		return result, err
	}
	return result, nil
}

func resultFromExitError(name string, err error, stderr string) (HookResult, error) {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return HookResult{Decision: DecisionNone, HookName: name}, err
	}
	reason := strings.TrimSpace(stderr)
	if reason == "" {
		reason = "blocked by external hook"
	}
	switch exitErr.ExitCode() {
	case 2:
		return HookResult{Decision: DecisionDeny, Reason: reason, HookName: name}, nil
	case HaltExitCode:
		return HookResult{Decision: DecisionDeny, Halt: true, Reason: reason, HookName: name}, nil
	default:
		return HookResult{Decision: DecisionNone, Reason: reason, HookName: name}, fmt.Errorf("external hook %s exited with code %d: %s", name, exitErr.ExitCode(), reason)
	}
}

func ParseResponse(data []byte) (HookResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return HookResult{Decision: DecisionNone}, nil
	}
	var resp hookResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return HookResult{Decision: DecisionNone}, fmt.Errorf("parse external hook response: %w", err)
	}
	result := HookResult{Decision: normalizeDecision(resp.Decision), Reason: resp.Reason, Halt: resp.Halt}
	if len(resp.UpdatedInput) > 0 {
		result.UpdatedInput = cloneRawMessage(resp.UpdatedInput)
	} else if len(resp.Output) > 0 {
		result.UpdatedInput = extractArgsFromOutput(resp.Output)
	}
	if resp.HookSpecificOutput.PermissionDecision != "" {
		result.Decision = normalizeDecision(Decision(resp.HookSpecificOutput.PermissionDecision))
		result.Reason = resp.HookSpecificOutput.PermissionDecisionReason
	}
	return result, nil
}

func normalizeDecision(decision Decision) Decision {
	switch strings.ToLower(string(decision)) {
	case string(DecisionAllow):
		return DecisionAllow
	case string(DecisionDeny):
		return DecisionDeny
	default:
		return DecisionNone
	}
}

func extractArgsFromOutput(output json.RawMessage) json.RawMessage {
	var payload struct {
		Args json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(output, &payload); err != nil || len(payload.Args) == 0 {
		return nil
	}
	return cloneRawMessage(payload.Args)
}
