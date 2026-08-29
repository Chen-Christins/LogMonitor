package monitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"LogMonitor/internal/config"
	"LogMonitor/internal/feishu"
)

type Monitor struct {
	config   config.Config
	sender   Sender
	logger   *log.Logger
	trackers map[string]*tracker
	seen     map[string]time.Time
	batches  map[string]*batchGroup
	mu       sync.Mutex
}

type Sender interface {
	Send(msg feishu.Message) error
}

type tracker struct {
	file        *os.File
	info        os.FileInfo
	offset      int64
	partial     string
	discardLine bool
	previous    []string
	pending     []*pendingAlert
	source      config.LogSourceConfig
	lastChange  time.Time
}

type pendingAlert struct {
	trigger string
	level   string
	lines   []string
	remain  int
}

// batchGroup accumulates alerts that share the same source and level within the
// aggregation window, so a burst of similar logs becomes a single message.
type batchGroup struct {
	source string
	level  string
	file   string
	lines  []string
	count  int
	first  time.Time
}

func New(config config.Config, sender Sender, logger *log.Logger) *Monitor {
	if config.Notification.MaxContextLines <= 0 {
		config.Notification.MaxContextLines = 100
	}
	if config.Notification.MaxContextBytes <= 0 {
		config.Notification.MaxContextBytes = 64 * 1024
	}
	if config.Runtime.ReadChunkBytes <= 0 {
		config.Runtime.ReadChunkBytes = 64 * 1024
	}
	if config.Runtime.MaxLineBytes <= 0 {
		config.Runtime.MaxLineBytes = 1024 * 1024
	}
	if config.Runtime.MaxTrackedFiles <= 0 {
		config.Runtime.MaxTrackedFiles = 1000
	}
	return &Monitor{config: config, sender: sender, logger: logger, trackers: map[string]*tracker{}, seen: map[string]time.Time{}, batches: map[string]*batchGroup{}}
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
	m.sweepSeen()
	m.flushBatches()
	for _, source := range m.config.LogSources {
		paths, err := discover(source)
		if err != nil {
			m.logger.Printf("source %s: %v", source.Name, err)
			continue
		}
		for _, path := range paths {
			if _, exists := m.trackers[path]; !exists && len(m.trackers) >= m.config.Runtime.MaxTrackedFiles {
				m.logger.Printf("skip %s: max tracked files (%d) reached", path, m.config.Runtime.MaxTrackedFiles)
				continue
			}
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
		t.offset, t.partial, t.previous, t.discardLine = 0, "", nil, false
	}
	if info.Size() == t.offset {
		return nil
	}

	if _, err = t.file.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	t.info = info
	buffer := make([]byte, m.config.Runtime.ReadChunkBytes)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := t.file.Read(buffer)
		if n > 0 {
			t.offset += int64(n)
			t.lastChange = time.Now()
			m.processChunk(path, buffer[:n], t)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (m *Monitor) processChunk(path string, data []byte, t *tracker) {
	for len(data) > 0 {
		newline := bytes.IndexByte(data, '\n')
		if newline < 0 {
			m.appendPartial(data, t)
			return
		}

		m.appendPartial(data[:newline], t)
		if !t.discardLine {
			m.processLine(path, strings.TrimSuffix(t.partial, "\r"), t)
		}
		t.partial = ""
		t.discardLine = false
		data = data[newline+1:]
	}
}

func (m *Monitor) appendPartial(data []byte, t *tracker) {
	if t.discardLine {
		return
	}
	remaining := m.config.Runtime.MaxLineBytes - len(t.partial)
	if len(data) > remaining {
		t.partial = ""
		t.discardLine = true
		m.logger.Printf("discard overlong line from %s (limit %d bytes)", t.source.Name, m.config.Runtime.MaxLineBytes)
		return
	}
	t.partial += string(data)
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
		alert.lines = appendLimited(alert.lines, line, m.config.Notification.MaxContextLines, m.config.Notification.MaxContextBytes)
		alert.remain--
		if alert.remain <= 0 || contextFull(alert.lines, m.config.Notification) {
			m.sendAlert(path, t.source, alert)
			t.pending = append(t.pending[:i], t.pending[i+1:]...)
			continue
		}
		i++
	}

	if level, ok := MatchLevel(line, t.source.Levels, t.source.LevelRegex); ok && !m.duplicate(path, line) {
		lines := append([]string(nil), t.previous...)
		if len(lines) >= m.config.Notification.MaxContextLines {
			lines = lines[len(lines)-m.config.Notification.MaxContextLines+1:]
		}
		lines = append(lines, line)
		lines = trimContext(lines, m.config.Notification.MaxContextLines, m.config.Notification.MaxContextBytes)
		alert := &pendingAlert{trigger: line, level: level, lines: lines, remain: t.source.AfterLines}
		if alert.remain == 0 || len(alert.lines) >= m.config.Notification.MaxContextLines {
			m.sendAlert(path, t.source, alert)
		} else {
			t.pending = append(t.pending, alert)
		}
	}

	t.previous = append(t.previous, line)
	if len(t.previous) > t.source.BeforeLines || contextBytes(t.previous) > m.config.Notification.MaxContextBytes {
		t.previous = t.previous[len(t.previous)-t.source.BeforeLines:]
		t.previous = trimContext(t.previous, t.source.BeforeLines, m.config.Notification.MaxContextBytes)
	}
}

func LevelMatches(line string, levels []string, levelRegex string) bool {
	_, ok := MatchLevel(line, levels, levelRegex)
	return ok
}

// MatchLevel returns the configured level that matched the line, if any.
func MatchLevel(line string, levels []string, levelRegex string) (string, bool) {
	if levelRegex == "" {
		levelRegex = `\[\s*([A-Za-z][A-Za-z0-9_-]*)\s*\]`
	}
	if level, ok := LogLevel(line, levelRegex); ok {
		for _, configured := range levels {
			if strings.EqualFold(level, configured) {
				return configured, true
			}
		}
	}
	return "", false
}

// LogLevel extracts levels such as [INFO], [WARN] and [ERROR].
func LogLevel(line, levelRegex string) (string, bool) {
	pattern, err := regexp.Compile(levelRegex)
	if err != nil {
		return "", false
	}
	match := pattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return "", false
	}
	return strings.ToUpper(match[1]), true
}

func (m *Monitor) duplicate(path, trigger string) bool {
	if m.config.Notification.CooldownSeconds <= 0 {
		return false
	}
	key := fmt.Sprintf("%s\x00%x", path, sha256.Sum256([]byte(trigger)))
	now := time.Now()
	cooldown := time.Duration(m.config.Notification.CooldownSeconds) * time.Second
	if previous, ok := m.seen[key]; ok && now.Sub(previous) < cooldown {
		return true
	}
	m.seen[key] = now
	return false
}

// sweepSeen removes dedup entries that are older than the cooldown window, so
// the map does not grow without bound. Entries older than the cooldown can no
// longer suppress duplicates, so deleting them is safe.
func (m *Monitor) sweepSeen() {
	cooldown := time.Duration(m.config.Notification.CooldownSeconds) * time.Second
	if cooldown <= 0 {
		if len(m.seen) > 0 {
			m.seen = map[string]time.Time{}
		}
		return
	}
	cutoff := time.Now().Add(-cooldown)
	for k, t := range m.seen {
		if t.Before(cutoff) {
			delete(m.seen, k)
		}
	}
}

func (m *Monitor) aggregateSeconds() int {
	a := m.config.Notification.AggregateSeconds
	if a == nil {
		return 0
	}
	return *a
}

func (m *Monitor) sendAlert(path string, source config.LogSourceConfig, alert *pendingAlert) {
	alert.lines = trimContext(alert.lines, m.config.Notification.MaxContextLines, m.config.Notification.MaxContextBytes)
	if m.aggregateSeconds() <= 0 {
		m.sendFeishu(feishu.Message{
			Title:   fmt.Sprintf("LogMonitor 告警 - %s", alert.level),
			Level:   alert.level,
			Source:  source.Name,
			File:    path,
			Time:    time.Now().Format(time.RFC3339),
			Content: strings.Join(alert.lines, "\n"),
		})
		return
	}
	m.addToBatch(source.Name, path, alert)
}

func (m *Monitor) addToBatch(source, file string, alert *pendingAlert) {
	key := source + "\x00" + alert.level
	g, ok := m.batches[key]
	if !ok {
		g = &batchGroup{source: source, level: alert.level, file: file, first: time.Now()}
		m.batches[key] = g
	}
	if g.file == "" {
		g.file = file
	}
	for _, line := range alert.lines {
		g.lines = appendLimited(g.lines, line, m.config.Notification.MaxContextLines, m.config.Notification.MaxContextBytes)
	}
	g.count++
}

// flushBatches sends and clears groups whose aggregation window has elapsed or
// that have grown past the context limit, turning a burst into a single message.
func (m *Monitor) flushBatches() {
	window := time.Duration(m.aggregateSeconds()) * time.Second
	maxLines := m.config.Notification.MaxContextLines
	for k, g := range m.batches {
		if time.Since(g.first) < window && len(g.lines) < maxLines && contextBytes(g.lines) < m.config.Notification.MaxContextBytes {
			continue
		}
		m.sendFeishu(feishu.Message{
			Title:   fmt.Sprintf("LogMonitor 告警 - %s (%d 条)", g.level, g.count),
			Level:   g.level,
			Source:  g.source,
			File:    g.file,
			Time:    time.Now().Format(time.RFC3339),
			Content: strings.Join(g.lines, "\n"),
		})
		delete(m.batches, k)
	}
}

func appendLimited(lines []string, line string, maxLines, maxBytes int) []string {
	if len(lines) >= maxLines || contextBytes(lines)+len(line)+1 > maxBytes {
		return lines
	}
	return append(lines, line)
}

func trimContext(lines []string, maxLines, maxBytes int) []string {
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for len(lines) > 0 && contextBytes(lines) > maxBytes {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func contextFull(lines []string, notification config.NotificationConfig) bool {
	return len(lines) >= notification.MaxContextLines || contextBytes(lines) >= notification.MaxContextBytes
}

func contextBytes(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	return total
}

func (m *Monitor) sendFeishu(msg feishu.Message) {
	if err := m.sender.Send(msg); err != nil {
		m.logger.Printf("send alert for %s: %v", msg.File, err)
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
	m.flushBatches()
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
