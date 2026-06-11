package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"AICodeAgent/internal/agent/tools"
	"AICodeAgent/internal/config"
	"AICodeAgent/internal/events"
	"AICodeAgent/internal/permission"
	"AICodeAgent/internal/skills"
)

// setupTestCoordinator 创建用于测试 handleToolCalls 的最小 coordinator
func setupTestCoordinator() *coordinator {
	r := tools.NewRegistry()
	r.Register(&testTool{
		name:        "view",
		description: "read file",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Output: "file content"}, nil
		},
	})
	r.Register(&testTool{
		name:        "failing_tool",
		description: "always fails",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "something went wrong"}, nil
		},
	})

	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	ps.AddBlacklist("rm")
	ps.AddBlacklist("sudo")
	ps.AddWhitelist("/workspace")

	return &coordinator{
		tools:       r,
		permService: ps,
		toolCaller:  tools.NewToolCaller(r, ps),
		confirmFn:   func(toolName, arguments string) bool { return true },
	}
}

// testTool 实现 tools.Tool 接口的测试桩
type testBroker struct{}

func (b *testBroker) Publish(event events.Event)                                 {}
func (b *testBroker) PublishMustDeliver(ctx context.Context, event events.Event) {}

type testTool struct {
	name        string
	description string
	execute     func(ctx context.Context, params json.RawMessage) (tools.Result, error)
}

func (t *testTool) Name() string                { return t.name }
func (t *testTool) Description() string         { return t.description }
func (t *testTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *testTool) Execute(ctx context.Context, p json.RawMessage) (tools.Result, error) {
	return t.execute(ctx, p)
}

func TestHandleToolCallsUnknownTool(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "nonexistent", Arguments: `{}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Role != RoleTool {
		t.Fatalf("expected RoleTool, got %s", results[0].Role)
	}
	if results[0].Content != "错误：未知工具 'nonexistent'" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsBlacklistDenial(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"command":"sudo rm -rf /"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "错误：操作被安全策略阻止" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsUserRejected(t *testing.T) {
	c := setupTestCoordinator()
	c.confirmFn = func(toolName, arguments string) bool { return false }

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/unknown/file.txt"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "操作被用户拒绝" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsNormalExecution(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/main.go"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "file content" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
	if results[0].ToolCallID != "call_1" {
		t.Fatalf("expected ToolCallID 'call_1', got '%s'", results[0].ToolCallID)
	}
}

func TestHandleToolCallsConfirmedGreyZoneExecutesAndCachesApproval(t *testing.T) {
	r := tools.NewRegistry()
	executions := 0
	r.Register(&testTool{
		name: "bash",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			executions++
			return tools.Result{Success: true, Output: "containers"}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	c := &coordinator{
		tools:       r,
		permService: ps,
		toolCaller:  tools.NewToolCaller(r, ps),
		confirmFn:   func(toolName, arguments string) bool { return true },
	}
	call := ToolCall{ID: "call_1", Function: FunctionCall{Name: "bash", Arguments: `{"command":"docker ps"}`}}

	results, err := c.handleToolCallsForSession(context.Background(), "s1", []ToolCall{call})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Content != "containers" || executions != 1 {
		t.Fatalf("expected confirmed command to execute once, results=%+v executions=%d", results, executions)
	}
	if !ps.IsOperationApproved("s1", "bash", json.RawMessage(call.Function.Arguments)) {
		t.Fatal("expected confirmed operation to be cached")
	}

	c.confirmFn = func(toolName, arguments string) bool {
		t.Fatalf("cached approval should not ask again")
		return false
	}
	results, err = c.handleToolCallsForSession(context.Background(), "s1", []ToolCall{call})
	if err != nil {
		t.Fatalf("unexpected error on cached call: %v", err)
	}
	if len(results) != 1 || results[0].Content != "containers" || executions != 2 {
		t.Fatalf("expected cached command to execute, results=%+v executions=%d", results, executions)
	}
}

func TestRequestUserConfirmationDefaultsToDeny(t *testing.T) {
	c := &coordinator{broker: &testBroker{}}
	if c.requestUserConfirmation("bash", `{"command":"docker ps"}`) {
		t.Fatal("default confirmation should fail closed")
	}
}

func TestHandleToolCallsFailingTool(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "failing_tool", Arguments: `{}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "错误: something went wrong" {
		t.Fatalf("unexpected content: %s", results[0].Content)
	}
}

func TestHandleToolCallsMultipleTools(t *testing.T) {
	c := setupTestCoordinator()

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/a.go"}`}},
		{ID: "call_2", Function: FunctionCall{Name: "nonexistent", Arguments: `{}`}},
		{ID: "call_3", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"/workspace/b.go"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Content != "file content" {
		t.Fatalf("result 0: %s", results[0].Content)
	}
	if results[1].Content != "错误：未知工具 'nonexistent'" {
		t.Fatalf("result 1: %s", results[1].Content)
	}
	if results[2].Content != "file content" {
		t.Fatalf("result 2: %s", results[2].Content)
	}
}

func TestToolCallerAllowsSkillViewWithoutConfirmation(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	path := filepath.Join(skillRoot, "code-review", skills.SkillFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	r := tools.NewRegistry()
	r.Register(tools.NewView(workDir).WithSkillRoots([]string{skillRoot}))
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	caller := tools.NewToolCaller(r, ps)

	params, _ := json.Marshal(tools.ViewParams{FilePath: path})
	_, allow, needConfirm, reason := caller.ResolveAndCheck("view", params)
	if !allow || needConfirm {
		t.Fatalf("skill view should bypass confirmation, allow=%v needConfirm=%v reason=%q", allow, needConfirm, reason)
	}
}

func TestToolCallerDoesNotBypassSensitiveToolsForSkillPath(t *testing.T) {
	skillRoot := t.TempDir()
	r := tools.NewRegistry()
	r.Register(&testTool{
		name:        "bash",
		description: "run shell",
		execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true}, nil
		},
	})
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	caller := tools.NewToolCaller(r, ps)

	params := json.RawMessage(`{"command":"cat ` + filepath.Join(skillRoot, "code-review", skills.SkillFileName) + `"}`)
	_, allow, needConfirm, _ := caller.ResolveAndCheck("bash", params)
	if allow || !needConfirm {
		t.Fatalf("bash should still require confirmation, allow=%v needConfirm=%v", allow, needConfirm)
	}
}

func TestCoordinatorLoadAvailableSkills(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("HOME", filepath.Join(projectDir, "home"))
	t.Setenv("AICODE_SKILLS_DIR", "")
	skillDir := filepath.Join(projectDir, ".aicode", "skills", "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: code-review\ndescription: Review code safely\n---\n# Instructions\nReview the code."
	if err := os.WriteFile(filepath.Join(skillDir, skills.SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	c := &coordinator{
		config: &config.Config{},
		skills: skills.NewManager(projectDir),
	}

	xml := c.loadAvailableSkills(projectDir)
	if !strings.Contains(xml, "<available_skills>") {
		t.Fatalf("expected available skills XML, got %q", xml)
	}
	if !strings.Contains(xml, "code-review") {
		t.Fatalf("expected code-review skill in XML, got %q", xml)
	}
	if c.matcher == nil {
		t.Fatal("expected skill matcher after loading skills")
	}
	matched, confidence := c.matcher.Match("please review and analyze this code safely")
	if matched == nil || matched.Name != "code-review" || confidence < 0.7 {
		t.Fatalf("expected code-review match, got %+v %.2f", matched, confidence)
	}
}

func TestCoordinatorLoadAvailableSkillsEmpty(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("HOME", filepath.Join(projectDir, "home"))
	t.Setenv("AICODE_SKILLS_DIR", "")
	c := &coordinator{
		config: &config.Config{},
		skills: skills.NewManager(projectDir),
	}
	if xml := c.loadAvailableSkills(projectDir); xml != "" {
		t.Fatalf("expected empty skills XML, got %q", xml)
	}
	if c.matcher != nil {
		t.Fatal("expected no matcher when no skills are configured")
	}
}

func TestCoordinatorPromptIncludesSkillMetadataOnly(t *testing.T) {
	projectDir := t.TempDir()
	skillDir := filepath.Join(projectDir, "custom-skills", "code-review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: Review code safely\n---\n# Secret workflow\nFull instructions stay on disk."
	if err := os.WriteFile(filepath.Join(skillDir, skills.SkillFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	c := &coordinator{
		config: &config.Config{SkillsPaths: []string{skillDir}},
		skills: skills.NewManager(projectDir),
		tools:  tools.NewRegistry(),
	}
	agent := &sessionAgent{}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	c.loadSystemPrompt(config.AgentConfig{}, agent)
	if !strings.Contains(agent.systemPrompt, "<available_skills>") {
		t.Fatalf("expected skills metadata in system prompt:\n%s", agent.systemPrompt)
	}
	if !strings.Contains(agent.systemPrompt, "code-review") {
		t.Fatalf("expected skill name in system prompt:\n%s", agent.systemPrompt)
	}
	if strings.Contains(agent.systemPrompt, "Secret workflow") {
		t.Fatalf("system prompt should not pre-inject full skill instructions:\n%s", agent.systemPrompt)
	}
	if !strings.Contains(agent.systemPrompt, filepath.Join(skillDir, skills.SkillFileName)) {
		t.Fatalf("expected SKILL.md location in prompt:\n%s", agent.systemPrompt)
	}
}

func TestCoordinatorPromptIncludesRulesAndTools(t *testing.T) {
	projectDir := t.TempDir()
	rulesDir := filepath.Join(projectDir, ".aicode")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ruleFile := filepath.Join(rulesDir, "rules.yaml")
	content := `constitution:
  - id: custom_safety
    name: 自定义安全规则
    description: 必须先说明风险
    enabled: true
coding:
  - id: custom_style
    name: 自定义风格规则
    description: 使用短函数
    enabled: true
workflow:
  - id: disabled_workflow
    name: 禁用流程规则
    description: 不应出现在提示词
    enabled: false
`
	if err := os.WriteFile(ruleFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := tools.NewRegistry()
	r.Register(&testTool{name: "view", description: "read file", execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
		return tools.Result{Success: true}, nil
	}})
	c := &coordinator{
		config: &config.Config{},
		tools:  r,
	}
	agent := &sessionAgent{}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	c.loadSystemPrompt(config.AgentConfig{}, agent)
	for _, want := range []string{"<critical_rules>", "自定义安全规则", "<code_conventions>", "自定义风格规则", "<tools>", "view", "read file"} {
		if !strings.Contains(agent.systemPrompt, want) {
			t.Fatalf("expected prompt to contain %q:\n%s", want, agent.systemPrompt)
		}
	}
	if strings.Contains(agent.systemPrompt, "不应出现在提示词") {
		t.Fatalf("disabled rule should be omitted:\n%s", agent.systemPrompt)
	}
}

func TestCoordinatorViewActivatesDiscoveredSkill(t *testing.T) {
	workDir := t.TempDir()
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "code-review")
	content := "---\nname: code-review\ndescription: Review code safely\n---\n# Instructions\nReview the code."
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, skills.SkillFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := tools.NewRegistry()
	r.Register(tools.NewView(workDir).WithSkillRoots([]string{skillRoot}))
	ps := permission.NewPermissionService(permission.PermLevelNormal, nil, nil)
	c := &coordinator{
		config:     &config.Config{},
		tools:      r,
		toolCaller: tools.NewToolCaller(r, ps),
		confirmFn: func(toolName, arguments string) bool {
			t.Fatalf("skill view should not request confirmation: %s %s", toolName, arguments)
			return false
		},
	}

	results, err := c.handleToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Function: FunctionCall{Name: "view", Arguments: `{"file_path":"` + filepath.Join(skillDir, skills.SkillFileName) + `"}`}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0].Content, "# Instructions") {
		t.Fatalf("expected skill instructions from view, got %q", results[0].Content)
	}
}

func TestCoordinatorIrrelevantPromptOmitsSkillsWhenNoneConfigured(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("HOME", filepath.Join(projectDir, "home"))
	t.Setenv("AICODE_SKILLS_DIR", "")
	c := &coordinator{
		config: &config.Config{},
		skills: skills.NewManager(projectDir),
		tools:  tools.NewRegistry(),
	}
	agent := &sessionAgent{}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	c.loadSystemPrompt(config.AgentConfig{}, agent)
	if strings.Contains(agent.systemPrompt, "<available_skills>") {
		t.Fatalf("expected prompt without skills when none are configured:\n%s", agent.systemPrompt)
	}
}

func TestSkillNameFromToolCall(t *testing.T) {
	got := skillNameFromToolCall("view", `{"file_path":"/tmp/find-skills/SKILL.md"}`)
	if got != "find-skills" {
		t.Fatalf("expected find-skills, got %q", got)
	}

	if got := skillNameFromToolCall("bash", `{"command":"cat /tmp/find-skills/SKILL.md"}`); got != "" {
		t.Fatalf("expected non-view tool not to activate skill, got %q", got)
	}
	if got := skillNameFromToolCall("view", `{"file_path":"/tmp/README.md"}`); got != "" {
		t.Fatalf("expected non-skill file not to activate skill, got %q", got)
	}
	if got := skillNameFromToolCall("view", `bad-json`); got != "" {
		t.Fatalf("expected invalid json not to activate skill, got %q", got)
	}
}
