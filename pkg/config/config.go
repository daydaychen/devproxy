package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 配置文件结构
type Config struct {
	Match      []string `yaml:"match"`
	Overwrite  []string `yaml:"overwrite"`
	Upstream   string   `yaml:"upstream"`
	Port       int      `yaml:"port"`
	Verbose    bool     `yaml:"verbose"`
	LogFile    string   `yaml:"log-file"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}
