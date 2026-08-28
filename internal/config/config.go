package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogSources   []LogSourceConfig  `yaml:"log_sources"`
	Feishu       FeishuConfig       `yaml:"feishu"`
	Notification NotificationConfig `yaml:"notification"`
}

type LogSourceConfig struct {
	Name        string   `yaml:"name"`
	File        string   `yaml:"file"`
	Directory   string   `yaml:"directory"`
	Pattern     string   `yaml:"pattern"`
	Recursive   bool     `yaml:"recursive"`
	Levels      []string `yaml:"levels"`
	BeforeLines int      `yaml:"before_lines"`
	AfterLines  int      `yaml:"after_lines"`
}

type FeishuConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Secret     string `yaml:"secret"`
}

type NotificationConfig struct {
	CooldownSeconds int `yaml:"cooldown_seconds"`
	MaxContextLines int `yaml:"max_context_lines"`
	RetryCount      int `yaml:"retry_count"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	data = []byte(os.ExpandEnv(string(data)))
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if len(c.LogSources) == 0 {
		return errors.New("config: at least one log source is required")
	}
	if c.Feishu.WebhookURL == "" {
		return errors.New("config: feishu.webhook_url is required")
	}
	if c.Notification.CooldownSeconds < 0 {
		return errors.New("config: notification.cooldown_seconds must not be negative")
	}
	if c.Notification.MaxContextLines <= 0 {
		c.Notification.MaxContextLines = 100
	}
	if c.Notification.RetryCount <= 0 {
		c.Notification.RetryCount = 3
	}
	for i := range c.LogSources {
		s := &c.LogSources[i]
		if (s.File == "") == (s.Directory == "") {
			return fmt.Errorf("config: source %q must define exactly one of file or directory", s.Name)
		}
		if s.Name == "" {
			s.Name = s.File
			if s.Name == "" {
				s.Name = s.Directory
			}
		}
		if len(s.Levels) == 0 {
			s.Levels = []string{"ERROR"}
		}
		for j := range s.Levels {
			s.Levels[j] = strings.ToUpper(strings.TrimSpace(s.Levels[j]))
			if s.Levels[j] == "" {
				return fmt.Errorf("config: source %q contains an empty level", s.Name)
			}
		}
		if s.Directory != "" && s.Pattern == "" {
			s.Pattern = "*.log"
		}
		if s.BeforeLines < 0 || s.AfterLines < 0 {
			return fmt.Errorf("config: source %q context line counts must not be negative", s.Name)
		}
	}
	return nil
}
