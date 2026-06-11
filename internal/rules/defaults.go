package rules

func DefaultRuleSet() RuleSet {
	return RuleSet{
		Constitution: DefaultConstitutionRules(),
		Coding:       DefaultGoCodingRules(),
		Workflow:     DefaultWorkflowRules(),
	}
}

func DefaultConstitutionRules() []Rule {
	return []Rule{
		{ID: "read_before_editing", Name: "READ BEFORE EDITING", Description: "Never edit a file you haven't already read", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "test_after_changes", Name: "TEST AFTER CHANGES", Description: "Run relevant tests immediately after significant modifications", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "never_commit_without_request", Name: "NEVER COMMIT", Description: "Never create commits unless the user explicitly asks", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "security_first", Name: "SECURITY FIRST", Description: "Only assist with authorized defensive security work", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "no_url_guessing", Name: "NO URL GUESSING", Description: "Only use URLs provided by the user or found in local files", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "never_push_without_request", Name: "NEVER PUSH TO REMOTE", Description: "Do not push changes unless explicitly asked", Category: "critical", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "no_hardcoded_secrets", Name: "禁止硬编码敏感信息", Description: "不要在代码中硬编码 API Key、密码或 Token，使用环境变量存储敏感信息", Category: "security", Enabled: true, Priority: PriorityDefault, Source: "default"},
	}
}

func DefaultGoCodingRules() []CodingRule {
	return []CodingRule{
		{
			ID:          "use_tabs",
			Name:        "使用 Tab 缩进",
			Description: "Go 代码使用 Tab 缩进，保持 gofmt 兼容",
			Category:    "style",
			Enabled:     true,
			Priority:    PriorityDefault,
			Source:      "default",
		},
		{
			ID:          "explicit_error_handling",
			Name:        "显式处理所有错误",
			Description: "不要使用 _ 忽略错误，错误必须被检查或向上返回",
			Category:    "error_handling",
			Enabled:     true,
			Priority:    PriorityDefault,
			Source:      "default",
			Examples: Examples{
				Good: []string{"result, err := doSomething()\nif err != nil {\n\treturn err\n}"},
				Bad:  []string{"result, _ := doSomething()"},
			},
		},
		{
			ID:          "match_existing_style",
			Name:        "匹配现有代码风格",
			Description: "修改代码前读取同目录相似文件，匹配现有命名、结构和依赖风格",
			Category:    "style",
			Enabled:     true,
			Priority:    PriorityDefault,
			Source:      "default",
		},
		{
			ID:          "never_log_secrets",
			Name:        "禁止记录敏感信息",
			Description: "不要在日志、错误信息或测试输出中打印 API Key、Token、Password 或 Secret",
			Category:    "security",
			Enabled:     true,
			Priority:    PriorityDefault,
			Source:      "default",
		},
	}
}

func DefaultWorkflowRules() []Rule {
	return []Rule{
		{ID: "search_before_assuming", Name: "先搜索再假设", Description: "行动前先搜索相关代码和上下文", Category: "workflow", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "one_logical_change", Name: "一次一个逻辑变更", Description: "保持修改最小且聚焦，避免无关重构", Category: "workflow", Enabled: true, Priority: PriorityDefault, Source: "default"},
		{ID: "verify_before_finish", Name: "完成前验证", Description: "结束前运行相关测试、lint 或类型检查", Category: "workflow", Enabled: true, Priority: PriorityDefault, Source: "default"},
	}
}
