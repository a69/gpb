package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const graphqlEndpoint = "https://api.github.com/graphql"

// GetProjectItem fetches a single item by its node ID.
func (c *Client) GetProjectItem(ctx context.Context, itemID string) (*ProjectItem, error) {
	query := fmt.Sprintf(`{
  node(id: "%s") {
    ... on ProjectV2Item {
      id
      type
      content {
        ... on Issue { title number url state assignees(first: 10) { nodes { login } } }
        ... on PullRequest { title number url state assignees(first: 10) { nodes { login } } }
        ... on DraftIssue { title assignees(first: 10) { nodes { login } } }
      }
      fieldValues(first: 50) {
        nodes {
          ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2Field { name } } }
          ... on ProjectV2ItemFieldSingleSelectValue { name field { ... on ProjectV2Field { name } } }
        }
      }
    }
  }
}`, itemID)

	body, err := c.doGraphQL(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("graphql request: %w", err)
	}

	var resp singleItemResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %v", resp.Errors)
	}

	item := resp.Data.Node
	pi := &ProjectItem{
		ID:    item.ID,
		Type:  item.Type,
		State: item.itemState(),
	}
	if item.Content.Title != "" {
		pi.Title = item.Content.Title
		pi.URL = item.Content.URL
	}
	for _, a := range item.Content.Assignees.Nodes {
		pi.Assignees = append(pi.Assignees, a.Login)
	}
	for _, fv := range item.FieldValues.Nodes {
		if fv.Date != "" {
			t, err := time.Parse("2006-01-02", fv.Date)
			if err == nil {
				pi.DueDate = &t
			}
		}
		if fv.Name != "" {
			pi.Status = fv.Name
		}
	}
	return pi, nil
}

type singleItemResponse struct {
	Data   singleItemData `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type singleItemData struct {
	Node singleItemNode `json:"node"`
}

type singleItemNode struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Content     contentNode     `json:"content"`
	FieldValues fieldValuePage  `json:"fieldValues"`
}

func (n singleItemNode) itemState() string {
	if n.Content.State != "" {
		return n.Content.State
	}
	return n.Type
}

// GetProjectItems fetches all items from a ProjectsV2 board.
// Returns the project title and items.
func (c *Client) GetProjectItems(ctx context.Context, projectID string) (string, []ProjectItem, error) {
	var allItems []ProjectItem
	cursor := ""
	projectTitle := ""

	for {
		query := buildQuery(projectID, cursor)
		body, err := c.doGraphQL(ctx, query)
		if err != nil {
			return "", nil, fmt.Errorf("graphql request: %w", err)
		}

		var resp graphQLResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", nil, fmt.Errorf("unmarshal response: %w", err)
		}

		if len(resp.Errors) > 0 {
			return "", nil, fmt.Errorf("graphql errors: %v", resp.Errors)
		}

		if projectTitle == "" {
			projectTitle = resp.Data.Node.Title
		}

		items := resp.Data.Node.Items
		for _, item := range items.Nodes {
			pi := ProjectItem{
				ID:    item.ID,
				Type:  item.Type,
				State: item.State(),
			}

			if item.Content.Title != "" {
				pi.Title = item.Content.Title
				pi.URL = item.Content.URL
			}

			for _, a := range item.Content.Assignees.Nodes {
				pi.Assignees = append(pi.Assignees, a.Login)
			}

			for _, fv := range item.FieldValues.Nodes {
				if fv.Date != "" {
					t, err := time.Parse("2006-01-02", fv.Date)
					if err == nil {
						pi.DueDate = &t
					}
				}
				if fv.Name != "" {
					pi.Status = fv.Name
				}
			}

			allItems = append(allItems, pi)
		}

		if !items.PageInfo.HasNextPage {
			break
		}
		cursor = items.PageInfo.EndCursor
	}

	return projectTitle, allItems, nil
}

func (c *Client) doGraphQL(ctx context.Context, query string) ([]byte, error) {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func buildQuery(projectID, cursor string) string {
	after := ""
	if cursor != "" {
		after = fmt.Sprintf(`, after: %q`, cursor)
	}
	return fmt.Sprintf(`{
  node(id: "%s") {
    ... on ProjectV2 {
      title
      items(first: 100%s) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          type
          content {
            ... on Issue {
              title number url state
              assignees(first: 10) { nodes { login } }
            }
            ... on PullRequest {
              title number url state
              assignees(first: 10) { nodes { login } }
            }
            ... on DraftIssue {
              title
              assignees(first: 10) { nodes { login } }
            }
          }
          fieldValues(first: 50) {
            nodes {
              ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2Field { name } } }
              ... on ProjectV2ItemFieldSingleSelectValue { name field { ... on ProjectV2Field { name } } }
            }
          }
        }
      }
    }
  }
}`, projectID, after)
}

// graphQLResponse mirrors the GitHub GraphQL response structure.
type graphQLResponse struct {
	Data   dataNode       `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type dataNode struct {
	Node projectNode `json:"node"`
}

type projectNode struct {
	Title string    `json:"title"`
	Items itemPage  `json:"items"`
}

type itemPage struct {
	PageInfo pageInfo   `json:"pageInfo"`
	Nodes    []itemNode `json:"nodes"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type itemNode struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Content     contentNode     `json:"content"`
	FieldValues fieldValuePage  `json:"fieldValues"`
}

func (n itemNode) State() string {
	if n.Content.State != "" {
		return n.Content.State
	}
	return n.Type
}

type contentNode struct {
	Title     string       `json:"title"`
	Number    int          `json:"number"`
	URL       string       `json:"url"`
	State     string       `json:"state"`
	Assignees assigneePage `json:"assignees"`
}

type assigneePage struct {
	Nodes []assigneeNode `json:"nodes"`
}

type assigneeNode struct {
	Login string `json:"login"`
}

type fieldValuePage struct {
	Nodes []fieldValueNode `json:"nodes"`
}

type fieldValueNode struct {
	Date  string      `json:"date"`
	Name  string      `json:"name"`
	Field fieldNode   `json:"field"`
}

type fieldNode struct {
	Name string `json:"name"`
}
