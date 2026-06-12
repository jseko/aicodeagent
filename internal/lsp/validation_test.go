package lsp

import (
	"strings"
	"testing"
)

func TestErrorClassifier(t *testing.T) {
	classifier := &ErrorClassifier{}
	tests := map[string]ErrorType{
		"syntax error: unexpected newline":      ErrorTypeSyntax,
		"undefined: fmt":                        ErrorTypeUndefined,
		"cannot use x as string value":          ErrorTypeMismatch,
		"imported and not used":                 ErrorTypeImport,
		"declared and not used":                 ErrorTypeUnused,
		"some other language server diagnostic": ErrorTypeUnknown,
	}

	for msg, want := range tests {
		if got := classifier.Classify(msg); got != want {
			t.Fatalf("Classify(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestBuildContextualPrompt(t *testing.T) {
	prompt := buildContextualPrompt("main.go", 3, "undefined: fmt", "package main")
	if prompt == "" {
		t.Fatal("prompt is empty")
	}
	if !containsAll(prompt, "main.go", "undefined: fmt", "package main") {
		t.Fatalf("prompt missing context: %s", prompt)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
