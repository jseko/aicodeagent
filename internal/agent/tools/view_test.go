package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestViewExecuteNormalRead(t *testing.T) {
	dir := t.TempDir()
	content := "hello world"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(dir)
	params, _ := json.Marshal(ViewParams{FilePath: "test.txt"})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected '%s', got '%s'", content, result.Output)
	}
}

func TestViewExecutePathTraversal(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	params, _ := json.Marshal(ViewParams{FilePath: "../../../etc/passwd"})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("path traversal should be rejected")
	}
}

func TestViewExecuteBoundaryConfusion(t *testing.T) {
	dir := t.TempDir()
	// Create adjacent directory to test boundary
	adjacent := dir + "extra"
	os.Mkdir(adjacent, 0755)
	defer os.RemoveAll(adjacent)

	if err := os.WriteFile(filepath.Join(adjacent, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(dir)
	params, _ := json.Marshal(ViewParams{FilePath: "../" + filepath.Base(adjacent) + "/secret.txt"})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("boundary confusion should be rejected")
	}
}

func TestViewExecuteDirectoryRejection(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	params, _ := json.Marshal(ViewParams{FilePath: "."})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("directory should be rejected")
	}
}

func TestViewExecuteInvalidParams(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	result, err := v.Execute(context.Background(), json.RawMessage(`{invalid}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("invalid params should fail")
	}
}

func TestViewExecuteFileNotFound(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	params, _ := json.Marshal(ViewParams{FilePath: "nonexistent.txt"})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("non-existent file should fail")
	}
}

func TestViewExecuteAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	content := "absolute path test"
	filePath := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(dir)
	params, _ := json.Marshal(ViewParams{FilePath: filePath})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("absolute path should work, got: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected '%s', got '%s'", content, result.Output)
	}
}

func TestViewExecuteAbsolutePathOutsideWorkdir(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	// 尝试读取 /etc/hosts（绝对路径但不在工作目录内）
	params, _ := json.Marshal(ViewParams{FilePath: "/etc/hosts"})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("absolute path outside workdir should be rejected")
	}
}

func TestViewExecuteEmptyPath(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	params, _ := json.Marshal(ViewParams{FilePath: ""})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("empty path should be rejected")
	}
}

func TestViewExecuteLargeFile(t *testing.T) {
	dir := t.TempDir()
	v := NewView(dir)

	// 创建大于 5MB 的文件（稀疏文件，不占磁盘空间）
	largeFile := filepath.Join(dir, "large.bin")
	f, err := os.Create(largeFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(6 * 1024 * 1024); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	params, _ := json.Marshal(ViewParams{FilePath: "large.bin"})
	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("file >5MB should be rejected")
	}
	if result.Error != "文件过大（>5MB）" {
		t.Fatalf("expected '文件过大（>5MB）', got '%s'", result.Error)
	}
}

func TestViewExecuteUnicodePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "测试目录")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "unicode path content"
	if err := os.WriteFile(filepath.Join(subDir, "文件.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(dir)
	params, _ := json.Marshal(ViewParams{FilePath: filepath.Join(subDir, "文件.md")})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("unicode path should work, got: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected '%s', got '%s'", content, result.Output)
	}
}

func TestViewExecuteSkillRootReadOutsideWorkdir(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	content := "# Skill instructions"
	path := filepath.Join(skillRoot, "code-review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	params, _ := json.Marshal(ViewParams{FilePath: path})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("skill file should be readable, got: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected %q, got %q", content, result.Output)
	}
}

func TestViewExecuteSkillAssetReadOutsideWorkdir(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	content := "asset content"
	path := filepath.Join(skillRoot, "code-review", "assets", "guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	params, _ := json.Marshal(ViewParams{FilePath: path})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("skill asset should be readable, got: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected %q, got %q", content, result.Output)
	}
}

func TestViewExecuteRejectsOutsideSkillRoot(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(path, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	params, _ := json.Marshal(ViewParams{FilePath: path})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("outside skill root should be rejected")
	}
}

func TestViewExecuteRejectsSkillSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(skillRoot, "code-review", "secret.md")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	params, _ := json.Marshal(ViewParams{FilePath: linkPath})

	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("skill symlink escaping root should be rejected")
	}
}

func TestViewExecuteRejectsOversizedSkillFile(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	path := filepath.Join(skillRoot, "code-review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSkillFileSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	params, _ := json.Marshal(ViewParams{FilePath: path})
	result, err := v.Execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("oversized skill file should be rejected")
	}
	if result.Error != "技能文件过大（>1MB）" {
		t.Fatalf("expected skill size error, got %q", result.Error)
	}
}

func TestViewAllowsSkillRead(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	inside := filepath.Join(skillRoot, "code-review", "SKILL.md")
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(inside), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("# Instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("# Outside"), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewView(workDir).WithSkillRoots([]string{skillRoot})
	if !v.AllowsSkillRead(inside) {
		t.Fatal("skill path should be allowed")
	}
	if v.AllowsSkillRead(outside) {
		t.Fatal("outside path should not be allowed")
	}
}
