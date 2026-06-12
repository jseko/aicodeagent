package lsp

import "testing"

func TestDetectLanguageID(t *testing.T) {
	tests := map[string]string{
		"main.go":        "go",
		"app.ts":         "typescript",
		"app.tsx":        "typescriptreact",
		"script.py":      "python",
		"lib.rs":         "rust",
		"unknown.custom": "",
	}

	for filePath, want := range tests {
		if got := DetectLanguageID(filePath); got != want {
			t.Fatalf("DetectLanguageID(%q) = %q, want %q", filePath, got, want)
		}
	}
}
