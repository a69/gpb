package msg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TelegramClient sends messages via the Telegram Bot API (or Bale, which uses the same protocol).
type TelegramClient struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewTelegram creates a client for Telegram.
func NewTelegram(token string) *TelegramClient {
	return &TelegramClient{
		token:      token,
		httpClient: http.DefaultClient,
		baseURL:    "https://api.telegram.org",
	}
}

// NewBale creates a client for Bale Messenger.
func NewBale(token string) *TelegramClient {
	return &TelegramClient{
		token:      token,
		httpClient: http.DefaultClient,
		baseURL:    "https://tapi.bale.ai",
	}
}

// SendMessage posts text to a chat.
func (c *TelegramClient) SendMessage(ctx context.Context, chatID, text string) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.token)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendMessage: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
