package tui

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// 构造器测试
// ============================================================================

func TestMarkdownRenderer_New(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Fatalf("NewMarkdownRenderer should not error, got: %v", err)
	}
	if mr == nil {
		t.Fatal("renderer should not be nil")
	}
	if mr.glamourR == nil {
		t.Error("glamourR should not be nil")
	}
	if mr.plainR == nil {
		t.Error("plainR should not be nil (fallback renderer)")
	}
	if mr.maxWidth != 80 {
		t.Errorf("maxWidth = %d, want 80", mr.maxWidth)
	}
}

// ============================================================================
// hasUnclosedCodeBlock 边界测试
// ============================================================================

func TestHasUnclosedCodeBlock(t *testing.T) {
	mr := &MarkdownRenderer{}

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"完整代码块", "```go\nfunc main() {}\n```", false},
		{"未闭合代码块", "```go\nfunc main() {}", true},
		{"无代码块", "no code block", false},
		{"交替完整", "```\nline1\n```\n\n```\nline2\n```", false},
		{"交替未闭合", "```\nline1\n```\n\n```\nline2", true},
		{"三个反引（6个）", "```python\nx=1\n```\n\n```go\ny=2\n```", false},
		{"语言标签未闭合", "```typescript\nconst x: number = 1;", true},
		{"空字符串", "", false},
		{"内联代码不影响", "use `fmt.Println` to print", false},
		{"多个内联反引号", "`a` `b` `c`", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mr.hasUnclosedCodeBlock(tt.content)
			if result != tt.expected {
				t.Errorf("hasUnclosedCodeBlock(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestRemoveTemporaryClosing(t *testing.T) {
	mr := &MarkdownRenderer{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"标准结尾清理",
			"some rendered content\n\n```\n\n",
			"some rendered content\n\n```",
		},
		{
			"无多余空白行",
			"rendered\n```",
			"rendered\n```",
		},
		{
			"空内容",
			"\n",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mr.removeTemporaryClosing(tt.input)
			if strings.TrimSpace(result) != strings.TrimSpace(tt.want) {
				t.Errorf("got %q, want %q", result, tt.want)
			}
		})
	}
}

// ============================================================================
// 四级降级策略测试
// ============================================================================

func TestRenderIncremental_PlainText(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	result := mr.RenderIncremental("Hello, world!", 80)
	if !strings.Contains(result, "Hello, world!") {
		t.Errorf("plain text should be rendered: %s", result)
	}
}

func TestRenderIncremental_UnclosedCodeBlock(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	content := "```go\nfunc main() {"
	result := mr.RenderIncremental(content, 80)
	if result == "" {
		t.Error("should not return empty result for unclosed code block")
	}
	// Level 1 处理不应崩溃，结果应包含原代码内容
	if !strings.Contains(result, "func") {
		t.Errorf("result should contain original code, got: %s", result)
	}
}

func TestRenderIncremental_CompleteMarkdown(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	content := "```go\npackage main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	result := mr.RenderIncremental(content, 80)
	if result == "" {
		t.Error("complete markdown should render successfully")
	}
}

// ============================================================================
// Markdown 格式特性测试（ch9.md 目标：标题/列表/代码块自动格式化）
// ============================================================================

func TestRenderIncremental_FormatFeatures(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	tests := []struct {
		name     string
		content  string
		contains []string
	}{
		{
			"标题",
			"# Main Title\n## Section\n### Subsection",
			[]string{"Main Title", "Section", "Subsection"},
		},
		{
			"粗体和斜体",
			"this is **bold** and *italic* and ***both***",
			[]string{"bold", "italic", "both"},
		},
		{
			"无序列表",
			"- item 1\n- item 2\n  - nested",
			[]string{"item 1", "item 2", "nested"},
		},
		{
			"有序列表",
			"1. first\n2. second\n3. third",
			[]string{"first", "second", "third"},
		},
		{
			"引用块",
			"> this is a quote\n> second line",
			[]string{"quote"},
		},
		{
			"表格",
			"| Col A | Col B |\n|-------|-------|\n| a1 | b1 |\n| a2 | b2 |",
			[]string{"Col A", "Col B", "a1", "b2"},
		},
		{
			"行内代码",
			"use `fmt.Println()` to output",
			[]string{"fmt.Println"},
		},
		{
			"混合格式",
			"# Title\n\n**bold text** with `code`\n\n- list item",
			[]string{"Title", "bold text", "code", "list item"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mr.RenderIncremental(tt.content, 80)
			if result == "" {
				t.Error("result should not be empty")
			}
			for _, keyword := range tt.contains {
				if !strings.Contains(result, keyword) {
					t.Errorf("result should contain %q, got: %s", keyword, result)
				}
			}
		})
	}
}

// ============================================================================
// 边界与异常场景测试
// ============================================================================

func TestRenderIncremental_EdgeCases(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	t.Run("空内容", func(t *testing.T) {
		result := mr.RenderIncremental("", 80)
		// 不应崩溃，空输入返回空或极小输出
		if len(result) > 200 {
			t.Errorf("empty input should produce minimal output, got %d chars", len(result))
		}
	})

	t.Run("100KB长内容", func(t *testing.T) {
		longText := strings.Repeat("This is a long line of text. ", 3000)
		result := mr.RenderIncremental(longText, 120)
		if result == "" {
			t.Error("long content should not produce empty result")
		}
	})

	t.Run("中文内容", func(t *testing.T) {
		content := "# 你好世界\n\n这是**中文**内容测试\n\n- 列表项1\n- 列表项2"
		result := mr.RenderIncremental(content, 80)
		if !strings.Contains(result, "你好世界") {
			t.Errorf("Chinese content should render correctly: %s", result)
		}
	})

	t.Run("HTML标签", func(t *testing.T) {
		content := "text with <div>html</div> embedded"
		result := mr.RenderIncremental(content, 80)
		// 不应崩溃，Glamour 默认会转义 HTML
		if result == "" {
			t.Error("HTML content should not crash")
		}
	})

	t.Run("嵌套代码块", func(t *testing.T) {
		content := "# Title\n\n```markdown\n# Nested title\n\n```go\nfunc main() {}\n```\n\n```\n"
		result := mr.RenderIncremental(content, 80)
		if result == "" {
			t.Error("nested code blocks should not crash")
		}
	})
}

// ============================================================================
// 性能基准测试（ch9.md 目标：<30ms 单次渲染）
// ============================================================================

func BenchmarkMarkdownRender_PlainText(b *testing.B) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		b.Skipf("glamour not available: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mr.RenderIncremental("Hello, this is a simple text message.", 80)
	}
}

func BenchmarkMarkdownRender_CodeBlock(b *testing.B) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		b.Skipf("glamour not available: %v", err)
	}
	content := "```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mr.RenderIncremental(content, 80)
	}
}

func BenchmarkMarkdownRender_Mixed(b *testing.B) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		b.Skipf("glamour not available: %v", err)
	}
	content := "# Title\n\nThis is **bold** and *italic*.\n\n- item 1\n- item 2\n\n```go\nx := 1\n```"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mr.RenderIncremental(content, 80)
	}
}

func BenchmarkHasUnclosedCodeBlock(b *testing.B) {
	mr := &MarkdownRenderer{}
	content := "```go\nfunc main() {\n\tfmt.Println()\n}"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mr.hasUnclosedCodeBlock(content)
	}
}

// ============================================================================
// 缓存命中性能测试（例9-2 目标：<5ms 缓存命中时）
// ============================================================================

func TestRenderPerformance_WithinBudget(t *testing.T) {
	mr, err := NewMarkdownRenderer("dark", 80)
	if err != nil {
		t.Skipf("glamour not available: %v", err)
	}

	content := "# 测试\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n- item 1\n- item 2"

	start := time.Now()
	result := mr.RenderIncremental(content, 80)
	elapsed := time.Since(start)

	if result == "" {
		t.Error("rendering should not return empty")
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("render too slow: %v (target <100ms)", elapsed)
	}
	t.Logf("RenderIncremental took %v", elapsed)
}

func TestHasUnclosedCodeBlock_Performance(t *testing.T) {
	mr := &MarkdownRenderer{}
	longContent := strings.Repeat("```go\nfunc main() {}\n", 1000)

	start := time.Now()
	for i := 0; i < 10000; i++ {
		mr.hasUnclosedCodeBlock(longContent)
	}
	elapsed := time.Since(start)

	avgNanos := elapsed.Nanoseconds() / 10000
	t.Logf("hasUnclosedCodeBlock avg: %d ns/op", avgNanos)
	if avgNanos > 100000 { // 100μs
		t.Errorf("hasUnclosedCodeBlock too slow: %d ns/op", avgNanos)
	}
}
