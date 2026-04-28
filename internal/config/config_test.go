package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func setenv(t *testing.T, key, value string) {
	t.Helper()
	old := os.Getenv(key)
	os.Setenv(key, value)
	t.Cleanup(func() { os.Setenv(key, old) })
}

func TestLoad(t *testing.T) {
	t.Run("minimal valid config with env vars", func(t *testing.T) {
		setenv(t, "SECRET", "s1")
		setenv(t, "GH_TOKEN", "ghp_test")
		setenv(t, "BALE_TOKEN", "bot123")

		path := writeConfig(t, `
server:
  webhook_secret: "${SECRET}"
tenants:
  - name: alpha
    github_token: "${GH_TOKEN}"
    github_project_id: "PVT_123"
    bale_token: "${BALE_TOKEN}"
    group_chat_id: "g-1"
`)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Server.Listen != ":8080" {
			t.Errorf("Listen = %q, want :8080", cfg.Server.Listen)
		}
		if cfg.Server.WebhookPath != "/webhook" {
			t.Errorf("WebhookPath = %q, want /webhook", cfg.Server.WebhookPath)
		}
		if cfg.Server.WebhookSecret != "s1" {
			t.Errorf("WebhookSecret = %q, want s1", cfg.Server.WebhookSecret)
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("Logging.Level = %q, want info", cfg.Logging.Level)
		}
		if cfg.Logging.Format != "json" {
			t.Errorf("Logging.Format = %q, want json", cfg.Logging.Format)
		}
		if len(cfg.Tenants) != 1 {
			t.Fatalf("expected 1 tenant, got %d", len(cfg.Tenants))
		}
		tt := cfg.Tenants[0]
		if tt.Name != "alpha" {
			t.Errorf("Name = %q", tt.Name)
		}
		if tt.GitHubToken != "ghp_test" {
			t.Errorf("GitHubToken = %q", tt.GitHubToken)
		}
		if tt.GitHubProjectID != "PVT_123" {
			t.Errorf("GitHubProjectID = %q", tt.GitHubProjectID)
		}
		if tt.BaleToken != "bot123" {
			t.Errorf("BaleToken = %q", tt.BaleToken)
		}
		if tt.GroupChatID != "g-1" {
			t.Errorf("GroupChatID = %q", tt.GroupChatID)
		}
		if tt.CronSpec != "0 9 * * *" {
			t.Errorf("CronSpec = %q, want default", tt.CronSpec)
		}
		if tt.UrgencyDays != 2 {
			t.Errorf("UrgencyDays = %d, want 2", tt.UrgencyDays)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := writeConfig(t, `: :: :::`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("no tenants", func(t *testing.T) {
		path := writeConfig(t, `server: {}`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "at least one tenant is required" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("tenant missing name", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - github_token: x
    github_project_id: y
    bale_token: z
    group_chat_id: g
`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tenant missing github_token", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - name: a
    github_project_id: y
    bale_token: z
    group_chat_id: g
`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tenant missing github_project_id", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - name: a
    github_token: x
    bale_token: z
    group_chat_id: g
`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("custom urgency and cron override defaults", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - name: a
    github_token: x
    github_project_id: y
    bale_token: z
    group_chat_id: g
    cron_spec: "30 8 * * 1-5"
    urgency_days: 7
`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Tenants[0].CronSpec != "30 8 * * 1-5" {
			t.Errorf("CronSpec = %q", cfg.Tenants[0].CronSpec)
		}
		if cfg.Tenants[0].UrgencyDays != 7 {
			t.Errorf("UrgencyDays = %d", cfg.Tenants[0].UrgencyDays)
		}
	})

	t.Run("env var not set leaves empty string", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - name: a
    github_token: "${MISSING_VAR}"
    github_project_id: y
    bale_token: z
    group_chat_id: g
`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected validation error for empty github_token")
		}
	})

	t.Run("multiple tenants", func(t *testing.T) {
		path := writeConfig(t, `
tenants:
  - name: alpha
    github_token: x1
    github_project_id: p1
    bale_token: b1
    group_chat_id: g1
  - name: beta
    github_token: x2
    github_project_id: p2
    bale_token: b2
    group_chat_id: g2
    urgency_days: 3
`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Tenants) != 2 {
			t.Fatalf("expected 2 tenants, got %d", len(cfg.Tenants))
		}
		if cfg.Tenants[0].Name != "alpha" || cfg.Tenants[1].Name != "beta" {
			t.Error("tenant ordering mismatch")
		}
		if cfg.Tenants[1].UrgencyDays != 3 {
			t.Errorf("beta urgency = %d, want 3", cfg.Tenants[1].UrgencyDays)
		}
	})
}
