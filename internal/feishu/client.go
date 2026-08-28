package feishu

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"LogMonitor/internal/config"
)

type FeishuClient struct {
	config       config.FeishuConfig
	notification config.NotificationConfig
	httpClient   *http.Client
}

// Message is the structured payload sent to Feishu.
type Message struct {
	Title   string
	Level   string
	Source  string
	File    string
	Time    string
	Content string
}

func NewClient(config config.FeishuConfig, notification config.NotificationConfig) *FeishuClient {
	return &FeishuClient{config: config, notification: notification, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func levelColor(level string) string {
	switch strings.ToUpper(level) {
	case "FATAL", "ERROR", "CRITICAL":
		return "red"
	case "WARN", "WARNING":
		return "orange"
	default:
		return "blue"
	}
}

func levelEmoji(level string) string {
	switch strings.ToUpper(level) {
	case "FATAL", "ERROR", "CRITICAL":
		return "🚨"
	case "WARN", "WARNING":
		return "⚠️"
	default:
		return "ℹ️"
	}
}

func fieldColumn(label, value string) map[string]any {
	return map[string]any{
		"tag":    "column",
		"width":  "weighted",
		"weight": 1,
		"elements": []any{
			map[string]any{"tag": "markdown", "content": "**" + label + "**\n" + value},
		},
	}
}

func columnSet(left, right map[string]any) map[string]any {
	return map[string]any{
		"tag":     "column_set",
		"columns": []any{left, right},
	}
}

func (c *FeishuClient) Send(m Message) error {
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"update_multi": true,
		},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": levelEmoji(m.Level) + " " + m.Title},
			"template": levelColor(m.Level),
		},
		"body": map[string]any{
			"direction": "vertical",
			"padding":   "12px 12px 12px 12px",
			"elements": []any{
				columnSet(
					fieldColumn("📦 来源", m.Source),
					fieldColumn("🔖 级别", m.Level),
				),
				columnSet(
					fieldColumn("📁 文件", m.File),
					fieldColumn("⏰ 时间", m.Time),
				),
				map[string]any{"tag": "hr"},
				map[string]any{
					"tag":     "markdown",
					"content": "📜 日志上下文\n\n```plain_text\n" + m.Content + "\n```",
				},
			},
		},
	}
	payload := map[string]any{"msg_type": "interactive", "card": card}
	if c.config.SignatureEnabled() {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		payload["timestamp"], payload["sign"] = ts, signature(ts, c.config.Secret)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Feishu payload: %w", err)
	}
	var last error
	for attempt := 0; attempt < c.notification.RetryCount; attempt++ {
		resp, err := c.httpClient.Post(c.config.WebhookURL, "application/json", bytes.NewReader(body))
		if err == nil {
			responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				err = fmt.Errorf("HTTP status %s", resp.Status)
			} else {
				var result struct {
					Code       *int   `json:"code"`
					StatusCode *int   `json:"StatusCode"`
					Message    string `json:"msg"`
				}
				if len(responseBody) == 0 || json.Unmarshal(responseBody, &result) != nil {
					return nil
				}
				code := 0
				if result.Code != nil {
					code = *result.Code
				}
				if result.StatusCode != nil {
					code = *result.StatusCode
				}
				if code == 0 {
					return nil
				}
				err = fmt.Errorf("Feishu response code %d: %s", code, result.Message)
			}
		}
		last = err
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return fmt.Errorf("send Feishu webhook: %w", last)
}

func signature(timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
