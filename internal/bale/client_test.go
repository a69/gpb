package bale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("TestToken")
	c.baseURL = srv.URL
	c.httpClient = srv.Client()
	return c
}

func TestClientSendMessage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/botTestToken/sendMessage") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("chat_id") != "g-123" {
				t.Errorf("chat_id = %q", r.FormValue("chat_id"))
			}
			if r.FormValue("text") != "hello" {
				t.Errorf("text = %q", r.FormValue("text"))
			}
			if r.FormValue("parse_mode") != "Markdown" {
				t.Errorf("parse_mode = %q", r.FormValue("parse_mode"))
			}
			w.WriteHeader(http.StatusOK)
		})

		err := c.SendMessage(context.Background(), "g-123", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("api error returns error", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"description":"Bad Request"}`))
		})

		err := c.SendMessage(context.Background(), "g-123", "hello")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "status 400") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
