package subagent

func BuiltinSubagents() []*Subagent {
	return []*Subagent{
		builtinCodeReviewer(),
		builtinArchitect(),
		builtinTestWriter(),
		builtinDocWriter(),
		builtinSecurityAuditor(),
	}
}

func RegisterBuiltins(registry *SubagentRegistry) error {
	for _, agent := range BuiltinSubagents() {
		if err := registry.Override(agent); err != nil {
			return err
		}
	}
	return nil
}

func builtinCodeReviewer() *Subagent {
	return builtin("code-reviewer", BuildDescription(
		"Professional code reviewer",
		"quality, security, and performance issues",
		[]string{"reviewing code", "checking security issues", "finding performance bottlenecks"},
		[]string{"Review this function for security", "Check for performance issues"},
		[]string{"writing code", "modifying files"},
	), "gpt-x", DefaultPermissions("code-reviewer"), []string{"view", "grep"}, "你是一位经验丰富的代码审查员，只能读取和分析代码，不能修改任何文件。")
}

func builtinArchitect() *Subagent {
	return builtin("architect", BuildDescription(
		"Software architect",
		"system design, module boundaries, and technical trade-offs",
		[]string{"analyzing architecture", "reviewing module boundaries"},
		[]string{"Analyze this architecture", "Find coupling risks"},
		[]string{"modifying files directly"},
	), "gpt-x", DefaultPermissions("architect"), []string{"view", "grep", "bash"}, "你是一位软件架构师，关注系统边界、依赖关系和架构风险。")
}

func builtinTestWriter() *Subagent {
	return builtin("test-writer", BuildDescription(
		"Test writer",
		"test strategy, coverage gaps, and failing test output",
		[]string{"designing tests", "analyzing test failures"},
		[]string{"Write a test plan", "Analyze this failing test"},
		[]string{"writing production code"},
	), "gpt-x", DefaultPermissions("test-writer"), []string{"view", "bash"}, "你是一位测试工程师，关注验证路径、边界条件和失败原因。")
}

func builtinDocWriter() *Subagent {
	return builtin("doc-writer", BuildDescription(
		"Documentation writer",
		"clear technical explanations and API usage notes",
		[]string{"writing documentation", "explaining existing code"},
		[]string{"Document this API", "Explain this module"},
		[]string{"running shell commands", "modifying files"},
	), "gpt-x", DefaultPermissions("doc-writer"), []string{"view", "grep"}, "你是一位技术文档编写者，只能读取和整理信息，不执行命令，不修改文件。")
}

func builtinSecurityAuditor() *Subagent {
	return builtin("security-auditor", BuildDescription(
		"Security auditor",
		"vulnerability review, unsafe patterns, and permission risks",
		[]string{"auditing code security", "checking permission boundaries", "finding unsafe input handling"},
		[]string{"Audit this file for security issues", "Check this module for unsafe behavior"},
		[]string{"writing code", "modifying files", "running shell commands"},
	), "gpt-x", DefaultPermissions("security-auditor"), []string{"view", "grep"}, "你是一位安全审计员，只能读取和分析代码，重点检查权限边界、输入校验、命令执行和敏感数据风险，不能修改文件或执行命令。")
}

func builtin(name, desc, model string, perm *PermissionSet, tools []string, prompt string) *Subagent {
	toolMap := make(map[string]bool)
	for _, tool := range tools {
		toolMap[tool] = true
	}
	return &Subagent{
		Config: &SubagentConfig{
			Name:        name,
			Description: desc,
			Mode:        ModeSubagent,
			Model:       model,
			Tools:       toolMap,
			Permissions: &PermissionConfig{
				Level:        PermissionLevelStandard,
				AllowedTools: append([]string(nil), perm.Allowed...),
				DeniedTools:  append([]string(nil), perm.Denied...),
				ConfirmTools: append([]string(nil), perm.Confirm...),
			},
		},
		SystemPrompt: prompt,
		Path:         "builtin://" + name,
	}
}
