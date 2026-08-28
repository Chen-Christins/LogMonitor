package monitor

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"LogMonitor/internal/config"
)

type Monitor struct {
	config   config.Config
	sender   Sender
	logger   *log.Logger
	trackers map[string]*tracker
	seen     map[string]time.Time
	mu       sync.Mutex
}

type Sender interface {
	Send(title, content string) error
}

type tracker struct {
	file       *os.File
	info       os.FileInfo
	offset     int64
	partial    string
	previous   []string
	pending    []*pendingAlert
	source     config.LogSourceConfig
	lastChange time.Time
}

type pendingAlert struct {
	trigger string
	lines   []string
	remain  int
}

func New(config config.Config, sender Sender, logger *log.Logger) *Monitor {
	return &Monitor{config: config, sender: sender, logger: logger, trackers: map[string]*tracker{}, seen: map[string]time.Time{}}
}

func (m *Monitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		m.scan(ctx)
		select {
		case <-ctx.Done():
			m.flushPending()
			m.close()
			return nil
		case <-ticker.C:
		}
	}
}

func (m *Monitor) scan(ctx context.Context) {
	for _, source := range m.config.LogSources {
		paths, err := discover(source)
		if err != nil {
			m.logger.Printf("source %s: %v", source.Name, err)
			continue
		}
		for _, path := range paths {
			if err := m.readFile(ctx, path, source); err != nil {
				m.logger.Printf("read %s: %v", path, err)
			}
		}
	}
	m.flushIdlePending()
}

func discover(source config.LogSourceConfig) ([]string, error) {
	if source.File != "" {
		return []string{filepath.Clean(source.File)}, nil
	}
	var result []string
	err := filepath.Walk(source.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != filepath.Clean(source.Directory) && !source.Recursive {
				return filepath.SkipDir
			}
			return nil
		}
		matched, matchErr := filepath.Match(source.Pattern, info.Name())
		if matchErr != nil {
			return fmt.Errorf("invalid pattern %q: %w", source.Pattern, matchErr)
		}
		if matched {
			result = append(result, filepath.Clean(path))
		}
		return nil
	})
	return result, err
}

func (m *Monitor) readFile(ctx context.Context, path string, source config.LogSourceConfig) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	t := m.trackers[path]
	if t == nil {
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		m.trackers[path] = &tracker{file: file, info: info, offset: info.Size(), source: source}
		return nil
	}
	if !os.SameFile(t.info, info) {
		m.flushTracker(t, path)
		_ = t.file.Close()
		delete(m.trackers, path)
		return m.openNewFile(path, source, info)
	}
	if info.Size() < t.offset {
		if _, err = t.file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		t.offset, t.partial, t.previous = 0, "", nil
	}
	if info.Size() == t.offset {
		return nil
	}

	if _, err = t.file.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(t.file)
	if err != nil {
		return err
	}
	t.offset += int64(len(data))
	t.info = info
	t.lastChange = time.Now()
	text := t.partial + string(data)
	parts := strings.Split(text, "\n")
	t.partial = parts[len(parts)-1]
	for _, raw := range parts[:len(parts)-1] {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		m.processLine(path, strings.TrimSuffix(raw, "\r"), t)
	}
	return nil
}

func (m *Monitor) openNewFile(path string, source config.LogSourceConfig, info os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	// A replacement at an already monitored path is a rotation, so read it from the start.
	m.trackers[path] = &tracker{file: file, info: info, source: source}
	return nil
}

func (m *Monitor) processLine(path, line string, t *tracker) {
	for i := 0; i < len(t.pending); {
		alert := t.pending[i]
		alert.lines = append(alert.lines, line)
		alert.remain--
		if alert.remain <= 0 || len(alert.lines) >= m.config.Notification.MaxContextLines {
			m.sendAlert(path, t.source, alert)
			t.pending = append(t.pending[:i], t.pending[i+1:]...)
			continue
		}
		i++
	}

	if LevelMatches(line, t.source.Levels) && !m.duplicate(path, line) {
		lines := append([]string(nil), t.previous...)
		if len(lines) >= m.config.Notification.MaxContextLines {
			lines = lines[len(lines)-m.config.Notification.MaxContextLines+1:]
		}
		lines = append(lines, line)
		alert := &pendingAlert{trigger: line, lines: lines, remain: t.source.AfterLines}
		if alert.remain == 0 || len(alert.lines) >= m.config.Notification.MaxContextLines {
			m.sendAlert(path, t.source, alert)
		} else {
			t.pending = append(t.pending, alert)
		}
	}

	t.previous = append(t.previous, line)
	if len(t.previous) > t.source.BeforeLines {
		t.previous = t.previous[len(t.previous)-t.source.BeforeLines:]
	}
}

func LevelMatches(line string, levels []string) bool {
	upper := strings.ToUpper(line)
	for _, level := range levels {
		if strings.Contains(upper, level) {
			return true
		}
	}
	return false
}

func (m *Monitor) duplicate(path, trigger string) bool {
	key := path + "\x00" + trigger
	now := time.Now()
	previous := m.seen[key]
	if m.config.Notification.CooldownSeconds > 0 && now.Sub(previous) < time.Duration(m.config.Notification.CooldownSeconds)*time.Second {
		return true
	}
	m.seen[key] = now
	return false
}

func (m *Monitor) sendAlert(path string, source config.LogSourceConfig, alert *pendingAlert) {
	if len(alert.lines) > m.config.Notification.MaxContextLines {
		alert.lines = alert.lines[:m.config.Notification.MaxContextLines]
	}
	content := fmt.Sprintf("Source: %s\nFile: %s\nTime: %s\n\n%s", source.Name, path, time.Now().Format(time.RFC3339), strings.Join(alert.lines, "\n"))
	if err := m.sender.Send("LogMonitor Alert", content); err != nil {
		m.logger.Printf("send alert for %s: %v", path, err)
	}
}

func (m *Monitor) flushIdlePending() {
	for path, t := range m.trackers {
		if len(t.pending) > 0 && !t.lastChange.IsZero() && time.Since(t.lastChange) >= 2*time.Second {
			m.flushTracker(t, path)
		}
	}
}

func (m *Monitor) flushPending() {
	for path, t := range m.trackers {
		m.flushTracker(t, path)
	}
}

func (m *Monitor) flushTracker(t *tracker, path string) {
	for _, alert := range t.pending {
		m.sendAlert(path, t.source, alert)
	}
	t.pending = nil
}

func (m *Monitor) close() {
	for path, t := range m.trackers {
		_ = t.file.Close()
		delete(m.trackers, path)
	}
}
