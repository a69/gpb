package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func timePtr(y, m, d int) *time.Time {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return &t
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("test-token")
	c.baseURL = srv.URL
	c.httpClient = srv.Client()
	return c
}

func graphQLHandler(responses []string, statuses []int) http.HandlerFunc {
	call := 0
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if call >= len(responses) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data": {"node": {"items": {"pageInfo": {"hasNextPage": false}, "nodes": []}}}}`))
			return
		}
		status := http.StatusOK
		if call < len(statuses) {
			status = statuses[call]
		}
		w.WriteHeader(status)
		w.Write([]byte(responses[call]))
		call++
	}
}

func TestGetProjectItems(t *testing.T) {
	t.Run("single page with full data", func(t *testing.T) {
		resp := mustJSON(map[string]any{
			"data": map[string]any{
				"node": map[string]any{
					"title": "Board",
					"items": map[string]any{
						"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
						"nodes": []any{
							map[string]any{
								"id":   "PVTI_1",
								"type": "ISSUE",
								"content": map[string]any{
									"title":  "Fix bug",
									"number": 42,
									"url":    "https://github.com/o/r/issues/42",
									"state":  "OPEN",
									"assignees": map[string]any{
										"nodes": []any{map[string]any{"login": "alice"}},
									},
								},
								"fieldValues": map[string]any{
									"nodes": []any{
										map[string]any{"date": "2026-04-30", "field": map[string]any{"name": "Due Date"}},
										map[string]any{"name": "In Progress", "field": map[string]any{"name": "Status"}},
									},
								},
							},
						},
					},
				},
			},
		})
		c := newTestClient(t, graphQLHandler([]string{resp}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		item := items[0]
		if item.ID != "PVTI_1" {
			t.Errorf("ID = %q", item.ID)
		}
		if item.Title != "Fix bug" {
			t.Errorf("Title = %q", item.Title)
		}
		if item.URL != "https://github.com/o/r/issues/42" {
			t.Errorf("URL = %q", item.URL)
		}
		if item.State != "OPEN" {
			t.Errorf("State = %q", item.State)
		}
		if len(item.Assignees) != 1 || item.Assignees[0] != "alice" {
			t.Errorf("Assignees = %v", item.Assignees)
		}
		if item.DueDate == nil || !item.DueDate.Equal(*timePtr(2026, 4, 30)) {
			t.Errorf("DueDate = %v", item.DueDate)
		}
		if item.Status != "In Progress" {
			t.Errorf("Status = %q", item.Status)
		}
	})

	t.Run("paginated", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"data": {"node": {"items": {
				"pageInfo": {"hasNextPage": true, "endCursor": "cursor1"},
				"nodes": [{
					"id": "1", "type": "ISSUE",
					"content": {"title": "Page 1", "state": "OPEN", "assignees": {"nodes": []}},
					"fieldValues": {"nodes": []}
				}]
			}}}}`,
			`{"data": {"node": {"items": {
				"pageInfo": {"hasNextPage": false, "endCursor": ""},
				"nodes": [{
					"id": "2", "type": "PULL_REQUEST",
					"content": {"title": "Page 2", "state": "MERGED", "assignees": {"nodes": []}},
					"fieldValues": {"nodes": []}
				}]
			}}}}`,
		}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if items[0].Title != "Page 1" || items[1].Title != "Page 2" {
			t.Errorf("items = %v", items)
		}
		if items[1].State != "MERGED" {
			t.Errorf("State = %q", items[1].State)
		}
	})

	t.Run("draft issue has no url", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"data": {"node": {"items": {
				"pageInfo": {"hasNextPage": false},
				"nodes": [{
					"id": "1", "type": "DraftIssue",
					"content": {"title": "Draft task", "state": "", "assignees": {"nodes": []}},
					"fieldValues": {"nodes": []}
				}]
			}}}}`,
		}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if items[0].URL != "" {
			t.Errorf("URL should be empty for draft, got %q", items[0].URL)
		}
		if items[0].Title != "Draft task" {
			t.Errorf("Title = %q", items[0].Title)
		}
	})

	t.Run("graphql errors", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"errors": [{"message": "Resource not accessible by integration"}]}`,
		}, nil))

		_, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "graphql errors") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("http error", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{""}, []int{http.StatusInternalServerError}))

		_, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unexpected status 500") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{`{invalid`}, nil))

		_, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unmarshal response") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty board", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"data": {"node": {"items": {"pageInfo": {"hasNextPage": false}, "nodes": []}}}}`,
		}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected 0 items, got %d", len(items))
		}
	})

	t.Run("multiple assignees", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"data": {"node": {"items": {
				"pageInfo": {"hasNextPage": false},
				"nodes": [{
					"id": "1", "type": "ISSUE",
					"content": {"title": "Pair task", "state": "OPEN", "assignees": {"nodes": [{"login": "alice"}, {"login": "bob"}]}},
					"fieldValues": {"nodes": []}
				}]
			}}}}`,
		}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items[0].Assignees) != 2 {
			t.Errorf("expected 2 assignees, got %v", items[0].Assignees)
		}
		if items[0].Assignees[0] != "alice" || items[0].Assignees[1] != "bob" {
			t.Errorf("assignees = %v", items[0].Assignees)
		}
	})

	t.Run("invalid date string silently ignored", func(t *testing.T) {
		c := newTestClient(t, graphQLHandler([]string{
			`{"data": {"node": {"items": {
				"pageInfo": {"hasNextPage": false},
				"nodes": [{
					"id": "1", "type": "ISSUE",
					"content": {"title": "T", "state": "OPEN", "assignees": {"nodes": []}},
					"fieldValues": {"nodes": [{"date": "not-a-date", "field": {"name": "Due Date"}}]}
				}]
			}}}}`,
		}, nil))

		items, err := c.GetProjectItems(context.Background(), "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if items[0].DueDate != nil {
			t.Error("DueDate should be nil for invalid date")
		}
	})
}
