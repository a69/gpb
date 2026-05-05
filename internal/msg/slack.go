package msg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SlackClient sends messages to a Slack channel via an incoming webhook.
type SlackClient struct {
	webhookURL string
	httpClient *http.Client
}

// NewSlack creates a Slack client with the given incoming webhook URL.
func NewSlack(webhookURL string) *SlackClient {
	return &SlackClient{
		webhookURL: webhookURL,
		httpClient: http.DefaultClient,
	}
}

type slackPayload struct {
	Text   string `json:"text"`
	Mrkdwn bool   `json:"mrkdwn"`
}

// SendMessage posts text to the webhook URL. chatID is ignored — the webhook already encodes the channel.
func (c *SlackClient) SendMessage(ctx context.Context, _ /*chatID*/, text string) error {
	payload := slackPayload{Text: text, Mrkdwn: true}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack sendMessage: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
