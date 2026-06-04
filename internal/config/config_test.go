package config

import (
	"os"
	"testing"
)

func TestConfigLoad_ValidFile(t *testing.T) {
	content := `theme: dark
openai:
  model: gpt-4
log_level: info`

	tmpFile, _ := os.CreateTemp("", "config-*.yaml") // 简化处理：生产环境应检查错误
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(content) // 简化处理：生产环境应检查写入错误
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败:%v", err)
	}

	if cfg.Theme != "dark" {
		t.Errorf("期望 theme=dark, 实际:%s", cfg.Theme)
	}
}

func TestConfigLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("文件不存在时应返回默认配置:%v", err)
	}

	if cfg.Theme != "dark" {
		t.Errorf("期望默认 theme=dark, 实际:%s", cfg.Theme)
	}
}
