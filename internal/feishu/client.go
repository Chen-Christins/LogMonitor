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
	"time"

	"LogMonitor/internal/config"
)

type FeishuClient struct {
	config       config.FeishuConfig
	notification config.NotificationConfig
	httpClient   *http.Client
}

func NewClient(config config.FeishuConfig, notification config.NotificationConfig) *FeishuClient {
	return &FeishuClient{config: config, notification: notification, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *FeishuClient) Send(title, content string) error {
	payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": title + "\n\n" + content}}
	if c.config.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(ts+"\n"+c.config.Secret))
		payload["timestamp"], payload["sign"] = ts, base64.StdEncoding.EncodeToString(mac.Sum(nil))
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
