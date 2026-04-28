package reporter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/a69/gpb/internal/github"
)

// GitHubClient is the subset of the GitHub client the reporter needs.
type GitHubClient interface {
	GetProjectItems(ctx context.Context, projectID string) ([]github.ProjectItem, error)
}

// BaleClient is the subset of the Bale client the reporter needs.
type BaleClient interface {
	SendMessage(ctx context.Context, chatID, text string) error
}

// Reporter fetches project data, formats it, and posts to chat.
type Reporter struct {
	github GitHubClient
	bale   BaleClient
	urgencyDays int
}

const maxMessageLen = 4000

// New creates a reporter.
func New(gh GitHubClient, bl BaleClient, urgencyDays int) *Reporter {
	if urgencyDays <= 0 {
		urgencyDays = 2
	}
	return &Reporter{github: gh, bale: bl, urgencyDays: urgencyDays}
}

// SendReport fetches the project board and posts a report to the chat.
func (r *Reporter) SendReport(ctx context.Context, chatID, projectID string) error {
	items, err := r.github.GetProjectItems(ctx, projectID)
	if err != nil {
		_ = r.bale.SendMessage(ctx, chatID, "⚠️ Could not fetch project data. Will retry at the next scheduled time.")
		return fmt.Errorf("fetch project: %w", err)
	}

	if len(items) == 0 {
		return r.bale.SendMessage(ctx, chatID, "📋 No items on the board today.")
	}

	markdown := r.Format(items)
	chunks := splitMessage(markdown, maxMessageLen)
	for _, chunk := range chunks {
		if err := r.bale.SendMessage(ctx, chatID, chunk); err != nil {
			return fmt.Errorf("send message: %w", err)
		}
	}
	return nil
}

// Format builds a human-readable markdown summary of the project items.
func (r *Reporter) Format(items []github.ProjectItem) string {
	now := time.Now().Truncate(24 * time.Hour)
	urgentThreshold := now.AddDate(0, 0, r.urgencyDays)

	type group struct {
		assignee string
		items    []github.ProjectItem
	}

	grouped := map[string][]github.ProjectItem{}
	for _, item := range items {
		if len(item.Assignees) == 0 {
			grouped["Unassigned"] = append(grouped["Unassigned"], item)
			continue
		}
		for _, a := range item.Assignees {
			grouped[a] = append(grouped[a], item)
		}
	}

	// Sort keys: Unassigned last
	keys := make([]string, 0, len(grouped))
	hasUnassigned := false
	for k := range grouped {
		if k == "Unassigned" {
			hasUnassigned = true
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if hasUnassigned {
		keys = append(keys, "Unassigned")
	}

	// Sort items within each group: urgent first, then by due date, then by title
	for k := range grouped {
		sort.Slice(grouped[k], func(i, j int) bool {
			a, b := grouped[k][i], grouped[k][j]
			aUrgent := isUrgent(a.DueDate, now, urgentThreshold)
			bUrgent := isUrgent(b.DueDate, now, urgentThreshold)
			if aUrgent != bUrgent {
				return aUrgent
			}
			if a.DueDate != nil && b.DueDate != nil {
				return a.DueDate.Before(*b.DueDate)
			}
			if a.DueDate != nil {
				return true
			}
			if b.DueDate != nil {
				return false
			}
			return a.Title < b.Title
		})
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📋 *Board Report — %s*\n", now.Format("Jan 2, 2006")))

	for _, k := range keys {
		items := grouped[k]
		urgentCount := 0
		for _, item := range items {
			if isUrgent(item.DueDate, now, urgentThreshold) {
				urgentCount++
			}
		}

		flag := ""
		if urgentCount > 0 {
			flag = fmt.Sprintf(", 🔴 %d urgent", urgentCount)
		}
		b.WriteString(fmt.Sprintf("\n*@%s* (%d task(s)%s)\n", k, len(items), flag))

		for _, item := range items {
			line := formatItem(item, now, urgentThreshold)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func formatItem(item github.ProjectItem, now, threshold time.Time) string {
	prefix := "•"
	if isUrgent(item.DueDate, now, threshold) {
		prefix = "• 🔴"
	}

	title := item.Title
	if item.URL != "" {
		title = fmt.Sprintf("[%s](%s)", item.Title, item.URL)
	}

	due := ""
	if item.DueDate != nil {
		days := int(item.DueDate.Truncate(24*time.Hour).Sub(now).Hours() / 24)
		switch {
		case days < 0:
			due = fmt.Sprintf(" — overdue by %d day(s)", -days)
		case days == 0:
			due = " — due today"
		case days == 1:
			due = " — due tomorrow"
		default:
			due = fmt.Sprintf(" — due in %d days", days)
		}
	}

	return fmt.Sprintf("%s %s%s", prefix, title, due)
}

func isUrgent(dueDate *time.Time, now, threshold time.Time) bool {
	if dueDate == nil {
		return false
	}
	d := dueDate.Truncate(24 * time.Hour)
	return !d.After(threshold) && !d.Before(now)
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	lines := strings.Split(text, "\n")
	current := ""
	for _, line := range lines {
		if len(current)+len(line)+1 > maxLen {
			chunks = append(chunks, current)
			current = line
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}
