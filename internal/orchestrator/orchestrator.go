package orchestrator

import (
	"github.com/a69/gpb/internal/authz"
	"github.com/a69/gpb/internal/reporter"
)

// Orchestrator wires tenants to their GitHub and Bale clients.
type Orchestrator struct {
	registry    *authz.Registry
	githubFactory func(authz.Tenant) reporter.GitHubClient
	baleFactory   func(authz.Tenant) reporter.BaleClient
}

// New creates an orchestrator.
func New(reg *authz.Registry, ghFn func(authz.Tenant) reporter.GitHubClient, blFn func(authz.Tenant) reporter.BaleClient) *Orchestrator {
	return &Orchestrator{registry: reg, githubFactory: ghFn, baleFactory: blFn}
}
