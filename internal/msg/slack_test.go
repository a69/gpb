package msg

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackSendMessage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			body, _ := io.ReadAll(r.Body)
			var p slackPayload
			if err := json.Unmarshal(body, &p); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if p.Text != "hello slack" {
				t.Errorf("text = %q", p.Text)
			}
			if !p.Mrkdwn {
				t.Error("mrkdwn should be true")
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewSlack(srv.URL)
		c.httpClient = srv.Client()
		err := c.SendMessage(context.Background(), "ignored", "hello slack")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("api error returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`no_text`))
		}))
		defer srv.Close()

		c := NewSlack(srv.URL)
		c.httpClient = srv.Client()
		err := c.SendMessage(context.Background(), "ignored", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "status 400") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("chatID is ignored", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewSlack(srv.URL)
		c.httpClient = srv.Client()
		// chatID can be anything — webhook URL encodes the channel
		if err := c.SendMessage(context.Background(), "", "msg"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
