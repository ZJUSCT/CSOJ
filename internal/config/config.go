package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type CORS struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type Link struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url"  json:"url"`
}

type Config struct {
	Cluster []Cluster `yaml:"cluster"`
	Contest []string  `yaml:"contest"`
	Logger  Logger    `yaml:"logger"`
	Storage Storage   `yaml:"storage"`
	Auth    Auth      `yaml:"auth"`
	Listen  string    `yaml:"listen"`
	Admin   Admin     `yaml:"admin"`
	CORS    CORS      `yaml:"cors"`
	Links   []Link    `yaml:"links"`
}

type Cluster struct {
	Name  string `yaml:"name" json:"name"`
	Nodes []Node `yaml:"node" json:"node"`
}

type Node struct {
	Name   string `yaml:"name" json:"name"`
	CPU    int    `yaml:"cpu" json:"cpu"`
	Memory int64  `yaml:"memory" json:"memory"`
	Docker string `yaml:"docker" json:"docker"`
}

type Logger struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type Storage struct {
	UserAvatar        string `yaml:"user_avatar"`
	SubmissionContent string `yaml:"submission_content"`
	Database          string `yaml:"database"`
	SubmissionLog     string `yaml:"submission_log"`
}

type Auth struct {
	JWT    JWT    `yaml:"jwt"`
	GitLab GitLab `yaml:"gitlab"`
	Local  Local  `yaml:"local"`
}

// Local defines configuration for username/password authentication.
type Local struct {
	Enabled bool `yaml:"enabled"`
}

type JWT struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type GitLab struct {
	App                 string `yaml:"app"`
	URL                 string `yaml:"url"`
	ClientID            string `yaml:"client_id"`
	ClientSecret        string `yaml:"client_secret"`
	RedirectURI         string `yaml:"redirect_uri"`
	FrontendCallbackURL string `yaml:"frontend_callback_url"`
}

type Admin struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
