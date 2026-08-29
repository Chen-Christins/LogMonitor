package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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

func TestEnableSignatureAcceptsBooleanAndInteger(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE"} {
		var cfg Config
		yamlText := "log_sources:\n  - file: app.log\nfeishu:\n  webhook_url: https://example.com/hook\n  enable_signature: " + value + "\n  secret: test-secret\n"
		if err := yaml.Unmarshal([]byte(yamlText), &cfg); err != nil {
			t.Fatalf("value %q: %v", value, err)
		}
		if !cfg.Feishu.SignatureEnabled() {
			t.Fatalf("value %q did not enable signature", value)
		}
	}
}

func TestEnableSignatureRequiresSecret(t *testing.T) {
	cfg := Config{
		LogSources: []LogSourceConfig{{File: "app.log"}},
		Feishu: FeishuConfig{
			WebhookURL:      "https://example.com/hook",
			EnableSignature: OptionalBool{Set: true, Value: true},
		},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDisabledSignatureIgnoresConfiguredSecret(t *testing.T) {
	cfg := FeishuConfig{
		EnableSignature: OptionalBool{Set: true, Value: false},
		Secret:          "configured-but-disabled",
	}
	if cfg.SignatureEnabled() {
		t.Fatal("signature should be disabled")
	}
}

func TestRuntimeDefaults(t *testing.T) {
	cfg := Config{
		LogSources: []LogSourceConfig{{File: "app.log"}},
		Feishu:     FeishuConfig{WebhookURL: "https://example.com/hook"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.MemoryLimitMB != 256 || cfg.Runtime.ReadChunkBytes != 64*1024 {
		t.Fatalf("runtime defaults = %+v", cfg.Runtime)
	}
	if cfg.Notification.MaxContextBytes != 64*1024 {
		t.Fatalf("max context bytes = %d", cfg.Notification.MaxContextBytes)
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
