package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"unicode/utf8"
)

type AuthError struct {
	Message string
}

func (err AuthError) Error() string { return err.Message }

type APIError struct {
	Message string
}

func (err APIError) Error() string { return err.Message }

type MiraClient struct {
	config          Config
	baseURL         string
	httpClient      *http.Client
	remoteSessionID string
}

func NewMiraClient(config Config) *MiraClient {
	return &MiraClient{
		config:     config,
		baseURL:    config.BaseURL,
		httpClient: &http.Client{},
	}
}

func (client *MiraClient) Execute(ctx context.Context, prompt Prompt, opts Options) (Stream, error) {
	if err := client.ensureSession(ctx); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"sessionId":     client.remoteSessionID,
		"content":       client.promptWithEnv(prompt.Text),
		"messageType":   1,
		"summaryAgent":  client.config.ModelID(),
		"dataSources":   []string{"manus"},
		"comprehensive": 1,
		"config": map[string]any{
			"online": true,
			"mode":   client.config.ModelMode(),
		},
	}
	response, err := client.doJSONRequest(ctx, client.baseURL+"/mira/api/v1/chat/completion", payload, true)
	if err != nil {
		return nil, err
	}
	contentType := response.Header.Get("Content-Type")
	if !contains(contentType, "text/event-stream") && contains(contentType, "application/json") {
		defer response.Body.Close()
		body, _ := ioReadAll(response.Body)
		return nil, client.parseJSONAPIError(response.StatusCode, body)
	}
	return NewSSEStream(response), nil
}

func (client *MiraClient) ensureSession(ctx context.Context) error {
	if client.remoteSessionID != "" {
		return nil
	}
	payload := map[string]any{
		"sessionProperties": map[string]any{
			"topic":       "",
			"dataSource":  "360_performance",
			"dataSources": []string{"manus"},
			"model":       client.config.ModelID(),
		},
	}
	response, err := client.doJSONRequest(ctx, client.baseURL+"/mira/api/v1/chat/create", payload, false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := ioReadAll(response.Body)
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if code, _ := parsed["code"].(float64); code == 20001 {
		return AuthError{Message: "认证失效，请更新 ~/.mira/config.json 中的 cookies"}
	}
	item, _ := parsed["sessionItem"].(map[string]any)
	if item == nil {
		item, _ = parsed["session_item"].(map[string]any)
	}
	sessionID, _ := item["sessionId"].(string)
	if sessionID == "" {
		sessionID, _ = item["session_id"].(string)
	}
	if sessionID == "" {
		return APIError{Message: "创建会话失败: 响应中无 sessionId"}
	}
	client.remoteSessionID = sessionID
	return nil
}

func (client *MiraClient) doJSONRequest(ctx context.Context, url string, payload map[string]any, stream bool) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range client.headers(stream) {
		request.Header.Set(key, value)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, APIError{Message: fmt.Sprintf("网络错误: %v", err)}
	}
	if response.StatusCode == http.StatusUnauthorized {
		defer response.Body.Close()
		return nil, AuthError{Message: "认证失效，请更新 ~/.mira/config.json 中的 cookies"}
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		body, _ := ioReadAll(response.Body)
		return nil, client.parseJSONAPIError(response.StatusCode, body)
	}
	return response, nil
}

func (client *MiraClient) parseJSONAPIError(statusCode int, body []byte) error {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if code, _ := parsed["code"].(float64); code == 20001 {
			return AuthError{Message: "认证失效，请更新 ~/.mira/config.json 中的 cookies"}
		}
		if message, _ := parsed["msg"].(string); message != "" {
			return APIError{Message: fmt.Sprintf("服务端错误 (code=%d): %s", int(parsed["code"].(float64)), message)}
		}
	}
	return APIError{Message: fmt.Sprintf("API 错误 %d: %s", statusCode, string(body))}
}

func (client *MiraClient) headers(stream bool) map[string]string {
	accept := "application/json, text/event-stream"
	if stream {
		accept = "text/event-stream"
	}
	return map[string]string{
		"Content-Type":    "application/json",
		"Accept":          accept,
		"Cookie":          safeCookie(client.config.Cookies),
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) MiraCLI/3.0.1",
		"x-mira-timezone": "Asia/Shanghai",
		"x-mira-client":   "web",
		"Origin":          client.baseURL,
		"Referer":         client.baseURL + "/",
	}
}

func (client *MiraClient) promptWithEnv(prompt string) string {
	return prompt + fmt.Sprintf(
		"\n\n[System Context] cwd=%s | platform=%s | user=%s | model=%s | time=%s",
		mustGetwd(),
		runtimeGOOS(),
		envUser(),
		*client.config.ModelName(),
		timeNow(),
	)
}

func (client *MiraClient) SetRemoteSessionID(sessionID string) {
	client.remoteSessionID = sessionID
}

func (client *MiraClient) RemoteSessionID() string {
	return client.remoteSessionID
}

func (client *MiraClient) ModelName() *string {
	return client.config.ModelName()
}

func (client *MiraClient) HasAuth() bool {
	return client.config.HasAuth()
}

func safeCookie(value string) string {
	cleaned := make([]rune, 0, len(value))
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			continue
		}
		if char > utf8.RuneSelf {
			continue
		}
		cleaned = append(cleaned, char)
	}
	return string(cleaned)
}

func contains(text, part string) bool {
	return bytes.Contains([]byte(text), []byte(part))
}

func envUser() string {
	if value := os.Getenv("USER"); value != "" {
		return value
	}
	return "unknown"
}
