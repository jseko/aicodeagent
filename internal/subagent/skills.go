package subagent

// LoadSkills 为指定子代理加载关联的 Skills，当前为桩实现
func LoadSkills(registry *SubagentRegistry, _ interface{}, subagentName string) ([]interface{}, error) {
	if registry == nil {
		return nil, nil
	}
	agent, err := registry.Get(subagentName)
	if err != nil {
		return nil, err
	}
	if len(agent.Config.Skills) == 0 {
		return nil, nil
	}
	return nil, nil
}
