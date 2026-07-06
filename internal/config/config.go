package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type CORS struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type Config struct {
	Cluster []Cluster `yaml:"cluster"`
	Logger  Logger    `yaml:"logger"`
	Storage Storage   `yaml:"storage"`
	Auth    Auth      `yaml:"auth"`
	Listen  string    `yaml:"listen"`
	CORS    CORS      `yaml:"cors"`
}

type Cluster struct {
	Name         string     `yaml:"name" json:"name"`
	Kubeconfig   string     `yaml:"kubeconfig" json:"kubeconfig"`
	Context      string     `yaml:"context" json:"context"`
	Namespace    string     `yaml:"namespace" json:"namespace"`
	Concurrency  int        `yaml:"concurrency" json:"concurrency"`
	HeartbeatTTL Duration   `yaml:"heartbeat_ttl" json:"heartbeat_ttl"`
	NodePools    []NodePool `yaml:"node_pools" json:"node_pools"`
}

type NodePool struct {
	Name string `yaml:"name" json:"name"`
}

// Duration wraps time.Duration so it unmarshals from a YAML string like "30s".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

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
