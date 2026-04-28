package command

import (
	"context"
	"fmt"
	"strings"
)

// Command is a parsed slash command from a Bale message.
type Command struct {
	Name   string
	Args   []string
	ChatID string
	UserID string
}

// Handler is a function that processes a command and returns a reply.
type Handler func(ctx context.Context, cmd Command) (string, error)

// Router dispatches incoming commands to registered handlers.
type Router struct {
	handlers map[string]Handler
}

// NewRouter creates a command router.
func NewRouter() *Router {
	return &Router{handlers: make(map[string]Handler)}
}

// Register adds a handler for a command name.
func (r *Router) Register(name string, h Handler) {
	r.handlers[name] = h
}

// Dispatch finds and runs the handler for a command.
func (r *Router) Dispatch(ctx context.Context, cmd Command) (string, error) {
	h, ok := r.handlers[cmd.Name]
	if !ok {
		return fmt.Sprintf("Unknown command: /%s. Try /status.", cmd.Name), nil
	}
	return h(ctx, cmd)
}

// Parse extracts a command name and arguments from raw message text.
func Parse(text string) Command {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return Command{}
	}

	// Strip bot mention suffix: "/status@gpb_bot" → "/status"
	if idx := strings.Index(text, "@"); idx > 0 {
		text = text[:idx]
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return Command{}
	}

	name := strings.TrimPrefix(parts[0], "/")
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	return Command{Name: name, Args: args}
}
