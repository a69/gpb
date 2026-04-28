package authz

import (
	"net/http"
)

// GroupGuard verifies that incoming webhooks come from authorized groups.
type GroupGuard struct {
	registry      *Registry
	webhookSecret string
}

// NewGroupGuard creates a group guard.
func NewGroupGuard(reg *Registry, webhookSecret string) *GroupGuard {
	return &GroupGuard{registry: reg, webhookSecret: webhookSecret}
}

// Allow checks whether an incoming HTTP request is authorized.
func (g *GroupGuard) Allow(r *http.Request) bool {
	if g.webhookSecret == "" {
		return true // No secret configured; allow all (dev mode)
	}
	token := r.Header.Get("X-Bot-Token")
	return token == g.webhookSecret
}

// CheckGroup verifies that a chat ID belongs to a registered tenant.
func (g *GroupGuard) CheckGroup(chatID string) bool {
	_, ok := g.registry.ByGroup(chatID)
	return ok
}

// FilterFields returns a copy of fields with sensitive values redacted.
func FilterFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if v == "" {
			out[k] = ""
		} else {
			out[k] = "***"
		}
	}
	return out
}
