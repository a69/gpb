package authz

import "sync"

// Tenant holds all configuration and secrets for a single tenant.
type Tenant struct {
	Name            string
	GitHubToken     string
	GitHubProjectID string
	BaleToken       string
	GroupChatID     string
	CronSpec        string
	UrgencyDays     int
}

// Registry stores tenants and enforces tenant isolation.
type Registry struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant // keyed by group chat ID
}

// NewRegistry creates an empty tenant registry.
func NewRegistry() *Registry {
	return &Registry{tenants: make(map[string]*Tenant)}
}

// Register adds or replaces a tenant.
func (r *Registry) Register(t Tenant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants[t.GroupChatID] = &t
}

// ByGroup looks up a tenant by group chat ID.
func (r *Registry) ByGroup(chatID string) (Tenant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tenants[chatID]
	if !ok {
		return Tenant{}, false
	}
	return *t, true
}

// All returns a snapshot of all registered tenants.
func (r *Registry) All() []Tenant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tenant, 0, len(r.tenants))
	for _, t := range r.tenants {
		out = append(out, *t)
	}
	return out
}
