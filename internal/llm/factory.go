package llm

import (
	"fmt"

	"AICodeAgent/internal/config"
)

// BuildProvider 根据配置创建LLM提供商实例
func BuildProvider(providerCfg config.ProviderConfig, model config.SelectedModel) (Provider, error) {
	apiKey := providerCfg.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required for provider %s", providerCfg.Name)
	}

	switch providerCfg.Type {
	case "openai":
		return NewOpenAIProvider(providerCfg.BaseURL, apiKey, model.Model), nil
	case "anthropic":
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "google":
		return nil, fmt.Errorf("google provider not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerCfg.Type)
	}
}

// FindProviderConfig 从配置列表中查找指定名称的Provider配置
func FindProviderConfig(providers []config.ProviderConfig, name string) (config.ProviderConfig, error) {
	for _, p := range providers {
		if p.Name == name || p.Type == name {
			return p, nil
		}
	}
	return config.ProviderConfig{}, fmt.Errorf("provider %q not found in config", name)
}
