package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteExecuteNormalWrite(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	content := "new file content"
	params, _ := json.Marshal(WriteParams{FilePath: "output.txt", Content: content})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	// 验证文件实际写入
	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected '%s', got '%s'", content, string(data))
	}
}

func TestWriteExecuteAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	content := "absolute write test"
	absPath := filepath.Join(dir, "abs_output.txt")
	params, _ := json.Marshal(WriteParams{FilePath: absPath, Content: content})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("absolute path should work, got: %s", result.Error)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected '%s', got '%s'", content, string(data))
	}
}

func TestWriteExecutePathTraversal(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	params, _ := json.Marshal(WriteParams{
		FilePath: "../../etc/malicious.txt",
		Content:  "evil",
	})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("path traversal should be rejected")
	}
}

func TestWriteExecuteAbsolutePathOutsideWorkdir(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	params, _ := json.Marshal(WriteParams{
		FilePath: "/tmp/should_not_write.txt",
		Content:  "nope",
	})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("absolute path outside workdir should be rejected")
	}
}

func TestWriteExecuteCreateParentDir(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	content := "nested file"
	params, _ := json.Marshal(WriteParams{
		FilePath: "a/b/c/nested.txt",
		Content:  content,
	})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success with auto dir creation, got: %s", result.Error)
	}

	data, err := os.ReadFile(filepath.Join(dir, "a/b/c/nested.txt"))
	if err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected '%s', got '%s'", content, string(data))
	}
}

func TestWriteExecuteInvalidParams(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	result, err := w.Execute(context.Background(), json.RawMessage(`{invalid}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("invalid params should fail")
	}
}

func TestWriteExecuteEmptyContent(t *testing.T) {
	dir := t.TempDir()
	w := NewWrite(dir)

	params, _ := json.Marshal(WriteParams{FilePath: "empty.txt", Content: ""})

	result, err := w.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("empty content should be allowed, got: %s", result.Error)
	}
}
