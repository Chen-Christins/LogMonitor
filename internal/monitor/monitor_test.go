package monitor

import (
	"io"
	"log"
	"strings"
	"testing"

	"LogMonitor/internal/config"
	"LogMonitor/internal/feishu"
)

type recordingSender struct{ messages []feishu.Message }

func (s *recordingSender) Send(m feishu.Message) error {
	s.messages = append(s.messages, m)
	return nil
}

func TestMonitorCollectsBeforeAndAfterContext(t *testing.T) {
	sender := &recordingSender{}
	cfg := config.Config{Notification: config.NotificationConfig{MaxContextLines: 100}}
	m := New(cfg, sender, log.New(io.Discard, "", 0))
	tracker := &tracker{source: config.LogSourceConfig{Name: "app", Levels: []string{"ERROR"}, LevelRegex: `\b(ERROR)\b`, BeforeLines: 2, AfterLines: 2}}

	for _, line := range []string{"before 1", "before 2", "ERROR failed", "after 1", "after 2"} {
		m.processLine("/var/log/app.log", line, tracker)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("messages = %d", len(sender.messages))
	}
	for _, expected := range []string{"before 1", "before 2", "ERROR failed", "after 1", "after 2"} {
		if !strings.Contains(sender.messages[0].Content, expected) {
			t.Errorf("message does not contain %q", expected)
		}
	}
}

func TestMonitorDeduplicatesIdenticalAlert(t *testing.T) {
	sender := &recordingSender{}
	cfg := config.Config{Notification: config.NotificationConfig{MaxContextLines: 100, CooldownSeconds: 60}}
	m := New(cfg, sender, log.New(io.Discard, "", 0))
	tracker := &tracker{source: config.LogSourceConfig{Name: "app", Levels: []string{"ERROR"}, LevelRegex: `\b(ERROR)\b`}}

	m.processLine("app.log", "ERROR same", tracker)
	m.processLine("app.log", "ERROR same", tracker)
	if len(sender.messages) != 1 {
		t.Fatalf("messages = %d", len(sender.messages))
	}
}

func TestLevelMatchingIsCaseInsensitive(t *testing.T) {
	if !LevelMatches("[2026-08-28 13:54:07.363179] [error] operation failed", []string{"ERROR"}, `\[\s*([A-Za-z]+)\s*\]`) {
		t.Fatal("expected level to match")
	}
}

func TestBracketLevelDoesNotMatchTextOutsideLevelField(t *testing.T) {
	line := "[2026-08-28 13:54:07.363179] [INFO] request contains ERROR as text"
	if LevelMatches(line, []string{"ERROR"}, `\[\s*([A-Za-z]+)\s*\]`) {
		t.Fatal("did not expect INFO line to match ERROR")
	}
	if !LevelMatches(line, []string{"INFO"}, `\[\s*([A-Za-z]+)\s*\]`) {
		t.Fatal("expected INFO level to match")
	}
}

func TestLogLevelExtractsExampleFormat(t *testing.T) {
	line := "[2026-08-28 13:54:07.365826]    543705  tick_0  [INFO]  [root]  src/manager/repo.cc:464 SyncPrFromGitHub: page 1"
	level, ok := LogLevel(line, `\[\s*([A-Za-z]+)\s*\]`)
	if !ok || level != "INFO" {
		t.Fatalf("LogLevel() = %q, %v", level, ok)
	}
}
