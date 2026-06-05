package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBashExecuteSimpleCommand(t *testing.T) {
	b := NewBash(".")
	params, _ := json.Marshal(BashParams{Command: "echo hello"})

	result, err := b.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("expected output to contain 'hello', got: %s", result.Output)
	}
}

func TestBashExecuteEmptyCommand(t *testing.T) {
	b := NewBash(".")
	params, _ := json.Marshal(BashParams{Command: ""})

	result, err := b.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("empty command should be rejected")
	}
}

func TestBashExecuteMultiWordOutput(t *testing.T) {
	b := NewBash(".")
	params, _ := json.Marshal(BashParams{Command: "echo foo bar baz"})

	result, err := b.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if result.Output != "foo bar baz" {
		t.Fatalf("expected 'foo bar baz', got '%s'", result.Output)
	}
}

func TestBashExecutePwd(t *testing.T) {
	b := NewBash(".")
	params, _ := json.Marshal(BashParams{Command: "pwd"})

	result, err := b.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if result.Output == "" {
		t.Fatal("pwd should return a path")
	}
}

func TestBashExecuteCommandWithPipe(t *testing.T) {
	b := NewBash(".")
	params, _ := json.Marshal(BashParams{Command: "echo 'a\nb\nc' | wc -l"})

	result, err := b.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if strings.TrimSpace(result.Output) != "3" {
		t.Fatalf("expected '3', got '%s'", result.Output)
	}
}

func TestBashExecuteInvalidParams(t *testing.T) {
	b := NewBash(".")
	result, err := b.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("invalid params should fail")
	}
}
