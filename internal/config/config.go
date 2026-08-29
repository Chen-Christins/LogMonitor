package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogSources   []LogSourceConfig  `yaml:"log_sources"`
	Feishu       FeishuConfig       `yaml:"feishu"`
	Notification NotificationConfig `yaml:"notification"`
	Runtime      RuntimeConfig      `yaml:"runtime"`
}

type LogSourceConfig struct {
	Name        string   `yaml:"name"`
	File        string   `yaml:"file"`
	Directory   string   `yaml:"directory"`
	Pattern     string   `yaml:"pattern"`
	Recursive   bool     `yaml:"recursive"`
	Levels      []string `yaml:"levels"`
	LevelRegex  string   `yaml:"level_regex"`
	BeforeLines int      `yaml:"before_lines"`
	AfterLines  int      `yaml:"after_lines"`
}

type FeishuConfig struct {
	WebhookURL      string       `yaml:"webhook_url"`
	EnableSignature OptionalBool `yaml:"enable_signature"`
	Secret          string       `yaml:"secret"`
}

type OptionalBool struct {
	Set   bool
	Value bool
}

func (b *OptionalBool) UnmarshalYAML(value *yaml.Node) error {
	var raw any
	if err := value.Decode(&raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case bool:
		b.Value = v
	case int:
		if v != 0 && v != 1 {
			return fmt.Errorf("must be 0, 1, true, or false")
		}
		b.Value = v == 1
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true":
			b.Value = true
		case "0", "false":
			b.Value = false
		default:
			return fmt.Errorf("must be 0, 1, true, or false")
		}
	default:
		return fmt.Errorf("must be 0, 1, true, or false")
	}
	b.Set = true
	return nil
}

func (c FeishuConfig) SignatureEnabled() bool {
	if c.EnableSignature.Set {
		return c.EnableSignature.Value
	}
	return c.Secret != ""
}

type NotificationConfig struct {
	CooldownSeconds  int  `yaml:"cooldown_seconds"`
	MaxContextLines  int  `yaml:"max_context_lines"`
	MaxContextBytes  int  `yaml:"max_context_bytes"`
	RetryCount       int  `yaml:"retry_count"`
	AggregateSeconds *int `yaml:"aggregate_seconds"`
}

type RuntimeConfig struct {
	MemoryLimitMB   int `yaml:"memory_limit_mb"`
	ReadChunkBytes  int `yaml:"read_chunk_bytes"`
	MaxLineBytes    int `yaml:"max_line_bytes"`
	MaxTrackedFiles int `yaml:"max_tracked_files"`
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
	if c.Feishu.SignatureEnabled() && c.Feishu.Secret == "" {
		return errors.New("config: feishu.secret is required when enable_signature is enabled")
	}
	if c.Notification.CooldownSeconds < 0 {
		return errors.New("config: notification.cooldown_seconds must not be negative")
	}
	if c.Notification.AggregateSeconds == nil {
		def := 5
		c.Notification.AggregateSeconds = &def
	} else if *c.Notification.AggregateSeconds < 0 {
		return errors.New("config: notification.aggregate_seconds must not be negative")
	}
	if c.Notification.MaxContextLines <= 0 {
		c.Notification.MaxContextLines = 100
	}
	if c.Notification.MaxContextBytes <= 0 {
		c.Notification.MaxContextBytes = 64 * 1024
	}
	if c.Notification.RetryCount <= 0 {
		c.Notification.RetryCount = 3
	}
	if c.Runtime.MemoryLimitMB <= 0 {
		c.Runtime.MemoryLimitMB = 256
	}
	if c.Runtime.ReadChunkBytes <= 0 {
		c.Runtime.ReadChunkBytes = 64 * 1024
	}
	if c.Runtime.MaxLineBytes <= 0 {
		c.Runtime.MaxLineBytes = 1024 * 1024
	}
	if c.Runtime.MaxTrackedFiles <= 0 {
		c.Runtime.MaxTrackedFiles = 1000
	}
	if c.Runtime.ReadChunkBytes > c.Runtime.MaxLineBytes {
		return errors.New("config: runtime.read_chunk_bytes must not exceed runtime.max_line_bytes")
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
		if s.LevelRegex == "" {
			s.LevelRegex = `\[\s*([A-Za-z][A-Za-z0-9_-]*)\s*\]`
		}
		if _, err := regexp.Compile(s.LevelRegex); err != nil {
			return fmt.Errorf("config: source %q has invalid level_regex: %w", s.Name, err)
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
		if s.BeforeLines > c.Notification.MaxContextLines || s.AfterLines > c.Notification.MaxContextLines {
			return fmt.Errorf("config: source %q context line counts must not exceed notification.max_context_lines", s.Name)
		}
	}
	return nil
}
