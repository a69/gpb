package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/a69/gpb/internal/github"
)

// ItemSnapshot stores the fields we compare to detect changes.
type ItemSnapshot struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// State is the persisted project state for change detection.
type State struct {
	Items map[string]ItemSnapshot `json:"items"`
}

// Change describes a detected change to a project item.
type Change struct {
	Item   github.ProjectItem
	Event  string // created, edited, deleted
	Sender string
}

// LoadState reads the persisted project state from a JSON file.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Items: map[string]ItemSnapshot{}}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	if s.Items == nil {
		s.Items = map[string]ItemSnapshot{}
	}
	return &s, nil
}

// SaveState writes the project state to a JSON file.
func SaveState(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Diff compares current items against the cached state and returns changes.
func Diff(prev *State, current []github.ProjectItem) ([]Change, *State) {
	next := &State{Items: map[string]ItemSnapshot{}}
	var changes []Change

	currentMap := map[string]github.ProjectItem{}
	for _, item := range current {
		currentMap[item.ID] = item
	}

	// Detect created and edited items.
	for id, item := range currentMap {
		snap := ItemSnapshot{
			ID:        item.ID,
			Title:     item.Title,
			Status:    item.Status,
			UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		next.Items[id] = snap

		prevSnap, existed := prev.Items[id]
		if !existed {
			changes = append(changes, Change{Item: item, Event: "created", Sender: "bot"})
		} else if prevSnap.UpdatedAt != snap.UpdatedAt {
			event := "edited"
			if prevSnap.Status != snap.Status {
				event = "moved"
			}
			changes = append(changes, Change{Item: item, Event: event, Sender: "bot"})
		}
	}

	// Detect deleted items.
	for id := range prev.Items {
		if _, ok := currentMap[id]; !ok {
			prevSnap := prev.Items[id]
			deletedItem := github.ProjectItem{
				ID:     prevSnap.ID,
				Title:  prevSnap.Title,
				Status: prevSnap.Status,
			}
			changes = append(changes, Change{Item: deletedItem, Event: "deleted", Sender: "bot"})
		}
	}

	return changes, next
}

// Poll fetches the current project state and detects changes since last run.
func (r *Reporter) Poll(ctx context.Context, projectID string, prev *State) ([]Change, *State, error) {
	_, items, err := r.github.GetProjectItems(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch items: %w", err)
	}
	changes, next := Diff(prev, items)
	return changes, next, nil
}
