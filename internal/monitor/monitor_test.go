package monitor

import (
	"io"
	"log"
	"strings"
	"testing"

	"LogMonitor/internal/config"
)

type recordingSender struct{ messages []string }

func (s *recordingSender) Send(_, content string) error {
	s.messages = append(s.messages, content)
	return nil
}

func TestMonitorCollectsBeforeAndAfterContext(t *testing.T) {
	sender := &recordingSender{}
	cfg := config.Config{Notification: config.NotificationConfig{MaxContextLines: 100}}
	m := New(cfg, sender, log.New(io.Discard, "", 0))
	tracker := &tracker{source: config.LogSourceConfig{Name: "app", Levels: []string{"ERROR"}, BeforeLines: 2, AfterLines: 2}}

	for _, line := range []string{"before 1", "before 2", "ERROR failed", "after 1", "after 2"} {
		m.processLine("/var/log/app.log", line, tracker)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("messages = %d", len(sender.messages))
	}
	for _, expected := range []string{"before 1", "before 2", "ERROR failed", "after 1", "after 2"} {
		if !strings.Contains(sender.messages[0], expected) {
			t.Errorf("message does not contain %q", expected)
		}
	}
}

func TestMonitorDeduplicatesIdenticalAlert(t *testing.T) {
	sender := &recordingSender{}
	cfg := config.Config{Notification: config.NotificationConfig{MaxContextLines: 100, CooldownSeconds: 60}}
	m := New(cfg, sender, log.New(io.Discard, "", 0))
	tracker := &tracker{source: config.LogSourceConfig{Name: "app", Levels: []string{"ERROR"}}}

	m.processLine("app.log", "ERROR same", tracker)
	m.processLine("app.log", "ERROR same", tracker)
	if len(sender.messages) != 1 {
		t.Fatalf("messages = %d", len(sender.messages))
	}
}

func TestLevelMatchingIsCaseInsensitive(t *testing.T) {
	if !LevelMatches("level=error failed", []string{"ERROR"}) {
		t.Fatal("expected level to match")
	}
}
