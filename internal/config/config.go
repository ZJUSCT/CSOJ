package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen  string  `yaml:"listen"`
	Storage Storage `yaml:"storage"`
	Auth    Auth    `yaml:"auth"`
}

type Storage struct {
	UserAvatar        string `yaml:"user_avatar"`
	SubmissionContent string `yaml:"submission_content"`
	Database          string `yaml:"database"`
	SubmissionLog     string `yaml:"submission_log"`
}

type Auth struct {
	JWT JWT `yaml:"jwt"`
}

type JWT struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"` // overridden by settings at runtime
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
