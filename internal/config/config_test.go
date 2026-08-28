package config

import "testing"

func TestConfigSupportsFileAndDirectorySources(t *testing.T) {
	cfg := Config{
		LogSources: []LogSourceConfig{
			{Name: "file", File: "/var/log/app.log"},
			{Name: "directory", Directory: "C:/logs/app"},
		},
		Feishu: FeishuConfig{WebhookURL: "https://example.com/hook"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.LogSources[0].Levels[0] != "ERROR" {
		t.Fatalf("default level = %q", cfg.LogSources[0].Levels[0])
	}
	if cfg.LogSources[1].Pattern != "*.log" {
		t.Fatalf("default pattern = %q", cfg.LogSources[1].Pattern)
	}
	if cfg.LogSources[0].LevelRegex == "" {
		t.Fatal("expected default level regex")
	}
}

func TestConfigRejectsFileAndDirectoryTogether(t *testing.T) {
	cfg := Config{
		LogSources: []LogSourceConfig{{Name: "bad", File: "app.log", Directory: "logs"}},
		Feishu:     FeishuConfig{WebhookURL: "https://example.com/hook"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
