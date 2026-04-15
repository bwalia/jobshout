package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BotClient is a thin HTTP wrapper around the Telegram Bot API.
type BotClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewBotClient creates a new Telegram Bot API client.
func NewBotClient(token string) *BotClient {
	return &BotClient{
		token:   token,
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BotUsername returns the bot's username by calling getMe.
func (b *BotClient) BotUsername(ctx context.Context) (string, error) {
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := b.callAPI(ctx, "getMe", nil, &result); err != nil {
		return "", err
	}
	return result.Result.Username, nil
}

// SendMessage sends a text message to a chat. Optionally includes an inline keyboard.
func (b *BotClient) SendMessage(ctx context.Context, chatID int64, text string, keyboard *InlineKeyboard) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	var result struct {
		OK bool `json:"ok"`
	}
	return b.callAPI(ctx, "sendMessage", payload, &result)
}

// AnswerCallbackQuery acknowledges an inline keyboard button press.
func (b *BotClient) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackQueryID,
	}
	if text != "" {
		payload["text"] = text
	}

	var result struct {
		OK bool `json:"ok"`
	}
	return b.callAPI(ctx, "answerCallbackQuery", payload, &result)
}

// SetWebhook registers a webhook URL with Telegram.
func (b *BotClient) SetWebhook(ctx context.Context, webhookURL, secretToken string) error {
	payload := map[string]any{
		"url":            webhookURL,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if secretToken != "" {
		payload["secret_token"] = secretToken
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := b.callAPI(ctx, "setWebhook", payload, &result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram: setWebhook failed: %s", result.Description)
	}
	return nil
}

// DeleteWebhook removes the webhook.
func (b *BotClient) DeleteWebhook(ctx context.Context) error {
	var result struct {
		OK bool `json:"ok"`
	}
	return b.callAPI(ctx, "deleteWebhook", nil, &result)
}

// callAPI makes a POST request to the Telegram Bot API.
func (b *BotClient) callAPI(ctx context.Context, method string, payload any, result any) error {
	url := fmt.Sprintf("%s/%s", b.baseURL, method)

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("telegram: marshal payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("telegram: create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %s request failed: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: %s returned %d: %s", method, resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("telegram: unmarshal response: %w", err)
		}
	}

	return nil
}
