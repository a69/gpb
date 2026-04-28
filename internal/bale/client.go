package bale

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client sends messages to Bale chats.
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a Bale client with the given bot token.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: http.DefaultClient,
		baseURL:    "https://tapi.bale.ai",
	}
}

// SendMessage posts text to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
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
		return fmt.Errorf("bale sendMessage: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
