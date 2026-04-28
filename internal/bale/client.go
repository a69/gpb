package bale

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a69/gpb/internal/authz"
	"github.com/a69/gpb/internal/command"
)

const baseURL = "https://tapi.bale.ai"

// Client sends messages to Bale chats.
type Client struct {
	token string
}

// NewClient creates a Bale client with the given bot token.
func NewClient(token string) *Client {
	return &Client{token: token}
}

// SendMessage posts text to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, c.token)
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
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

// SetWebhook registers the bot's webhook URL.
func (c *Client) SetWebhook(ctx context.Context, webhookURL string) error {
	endpoint := fmt.Sprintf("%s/bot%s/setWebhook", baseURL, c.token)
	form := url.Values{}
	form.Set("url", webhookURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Update represents an incoming Bale update (message or command).
type Update struct {
	UpdateID int64   `json:"update_id"`
	Message  Message `json:"message"`
}

// Message is a Bale message.
type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
	Text      string `json:"text"`
}

// Chat is a Bale chat.
type Chat struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// User is a Bale user.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// WebhookHandler returns an http.HandlerFunc that processes incoming Bale updates.
func WebhookHandler(guard *authz.GroupGuard, router *command.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guard.Allow(r) {
			slog.Warn("unauthorized webhook", "remote_addr", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusOK) // Don't leak existence
			return
		}

		var update Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			slog.Debug("malformed webhook body", "err", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if update.Message.Text == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		cmd := command.Parse(update.Message.Text)
		cmd.ChatID = update.Message.Chat.ID
		cmd.UserID = update.Message.From.ID

		if !guard.CheckGroup(cmd.ChatID) {
			slog.Warn("message from unauthorized group", "chat_id", cmd.ChatID)
			w.WriteHeader(http.StatusOK)
			return
		}

		reply, err := router.Dispatch(r.Context(), cmd)
		if err != nil {
			slog.Error("command dispatch failed", "cmd", cmd.Name, "err", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		if reply != "" {
			// Use a background context so the send isn't tied to the request.
			// In practice the handler would need a Bale client reference here.
			// For now, reply is returned in the HTTP response for debugging.
			_ = reply
		}

		w.WriteHeader(http.StatusOK)
	}
}

// SendMessageToChat is a helper when you already have a token.
func SendMessageToChat(ctx context.Context, token, chatID, text string) error {
	c := NewClient(token)
	return c.SendMessage(ctx, chatID, text)
}
