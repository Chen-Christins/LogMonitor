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

func (c *FeishuClient) Send(m Message) error {
	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": levelEmoji(m.Level) + " " + m.Title},
			"template": levelColor(m.Level),
		},
		"elements": []any{
			map[string]any{
				"tag": "div",
				"fields": []any{
					map[string]any{"is_short": true, "text": map[string]any{"tag": "lark_md", "content": "**📦 来源**\n" + m.Source}},
					map[string]any{"is_short": true, "text": map[string]any{"tag": "lark_md", "content": "**🔖 级别**\n" + m.Level}},
				},
			},
			map[string]any{
				"tag":  "div",
				"text": map[string]any{"tag": "lark_md", "content": "**📁 文件**\n" + m.File},
			},
			map[string]any{
				"tag":  "div",
				"text": map[string]any{"tag": "lark_md", "content": "**⏰ 时间**\n" + m.Time},
			},
			map[string]any{"tag": "hr"},
			map[string]any{
				"tag":  "div",
				"text": map[string]any{"tag": "lark_md", "content": "📜 日志上下文"},
			},
			map[string]any{
				"tag":     "code",
				"content": m.Content,
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
