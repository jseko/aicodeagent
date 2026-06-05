package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGrepDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc helper() {\n\tprintln(\"world\")\n}\n"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("hello world\nfoo bar\n"), 0644)
	return dir
}

func TestGrepExecuteFindMatch(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: "hello", Path: "."})
	result, err := g.Execute(context.Background(), params)
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

func TestGrepExecuteNoMatch(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: "zzz_nonexistent_zzz", Path: "."})
	result, err := g.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("no match should still succeed, got: %s", result.Error)
	}
}

func TestGrepExecuteAbsolutePath(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: "hello", Path: dir})
	result, err := g.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("absolute path should work, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("expected output to contain 'hello', got: %s", result.Output)
	}
}

func TestGrepExecutePathTraversal(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: ".*", Path: "/etc"})
	result, err := g.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("searching /etc from sandbox should be rejected")
	}
}

func TestGrepExecuteDefaultPath(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: "println"})
	result, err := g.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("default path should work, got: %s", result.Error)
	}
}

func TestGrepExecuteEmptyPattern(t *testing.T) {
	dir := setupGrepDir(t)
	g := NewGrep(dir)

	params, _ := json.Marshal(GrepParams{Pattern: ""})
	result, err := g.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("empty pattern should be rejected")
	}
}
