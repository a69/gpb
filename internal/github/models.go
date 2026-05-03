package github

import (
	"net/http"
	"time"
)

// ProjectItem is a single item (issue, PR, or draft) on a GitHub project board.
type ProjectItem struct {
	ID        string
	Title     string
	URL       string
	Type      string
	State     string
	Assignees []string
	DueDate   *time.Time
	Status    string
	UpdatedAt time.Time
}

// Client queries a GitHub ProjectsV2 board.
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new GitHub client with the given PAT.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: http.DefaultClient,
		baseURL:    "https://api.github.com/graphql",
	}
}
