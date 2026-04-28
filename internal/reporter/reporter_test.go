package reporter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/a69/gpb/internal/github"
)

func timePtr(y, m, d int) *time.Time {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return &t
}

// ---------- mocks ----------

type mockGitHubClient struct {
	items []github.ProjectItem
	err   error
}

func (m *mockGitHubClient) GetProjectItems(_ context.Context, _ string) ([]github.ProjectItem, error) {
	return m.items, m.err
}

type mockBaleClient struct {
	messages []string
	err      error
}

func (m *mockBaleClient) SendMessage(_ context.Context, chatID, text string) error {
	m.messages = append(m.messages, text)
	return m.err
}

// ---------- helpers ----------

func newTestReporter(gh GitHubClient, bl BaleClient, urgencyDays int) *Reporter {
	r := New(gh, bl, urgencyDays)
	r.now = func() time.Time { return time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC) }
	return r
}

// ---------- TestFormat ----------

func TestReporterFormat(t *testing.T) {
	frozenNow := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)

	r := New(nil, nil, 2)
	r.now = func() time.Time { return frozenNow }

	tests := []struct {
		name  string
		items []github.ProjectItem
		want  []string // lines that must appear in output
		not   []string // lines that must NOT appear
	}{
		{
			name:  "empty board",
			items: nil,
			want:  []string{"📋 *Board Report — Apr 28, 2026*"},
			not:   []string{"🔴", "@", "•"},
		},
		{
			name: "single item unassigned no due date",
			items: []github.ProjectItem{
				{Title: "Fix login", URL: "https://github.com/o/r/issues/1"},
			},
			want: []string{
				"*@Unassigned*",
				"[Fix login](https://github.com/o/r/issues/1)",
			},
			not: []string{"🔴", "due"},
		},
		{
			name: "single item assigned due tomorrow",
			items: []github.ProjectItem{
				{Title: "Ship feature", URL: "https://gh/2", Assignees: []string{"alice"}, DueDate: timePtr(2026, 4, 29)},
			},
			want: []string{
				"*@alice*",
				"[Ship feature](https://gh/2)",
				"due tomorrow",
				"🔴",
			},
		},
		{
			name: "urgent item due within threshold",
			items: []github.ProjectItem{
				{Title: "Hotfix", URL: "https://gh/3", Assignees: []string{"bob"}, DueDate: timePtr(2026, 4, 30)},
			},
			want: []string{"*@bob*", "[Hotfix](https://gh/3)", "due in 2 days", "🔴"},
		},
		{
			name: "item due outside threshold not urgent",
			items: []github.ProjectItem{
				{Title: "Later", URL: "https://gh/4", Assignees: []string{"bob"}, DueDate: timePtr(2026, 5, 1)},
			},
			want: []string{"[Later](https://gh/4)", "due in 3 days"},
			not:  []string{"🔴"},
		},
		{
			name: "overdue item",
			items: []github.ProjectItem{
				{Title: "Overdue", URL: "https://gh/5", Assignees: []string{"charlie"}, DueDate: timePtr(2026, 4, 25)},
			},
			want: []string{"[Overdue](https://gh/5)", "overdue by 3 day(s)"},
			not:  []string{"🔴"}, // past-due items are not flagged urgent
		},
		{
			name: "due today",
			items: []github.ProjectItem{
				{Title: "Today", URL: "https://gh/6", Assignees: []string{"alice"}, DueDate: timePtr(2026, 4, 28)},
			},
			want: []string{"due today", "🔴"},
		},
		{
			name: "multiple assignees appear under each",
			items: []github.ProjectItem{
				{Title: "Pair", URL: "https://gh/7", Assignees: []string{"alice", "bob"}},
			},
			want: []string{
				"*@alice*",
				"*@bob*",
			},
		},
		{
			name: "grouping sorts alphabetically with unassigned last",
			items: []github.ProjectItem{
				{Title: "Z", Assignees: []string{"zoe"}},
				{Title: "A", Assignees: []string{"alice"}},
				{Title: "U", Assignees: nil},
			},
			want: []string{"*@alice*", "*@zoe*", "*@Unassigned*"},
		},
		{
			name: "urgent count in group header",
			items: []github.ProjectItem{
				{Title: "Urgent a", URL: "https://gh/8", Assignees: []string{"alice"}, DueDate: timePtr(2026, 4, 29)},
				{Title: "Urgent b", URL: "https://gh/9", Assignees: []string{"alice"}, DueDate: timePtr(2026, 4, 30)},
				{Title: "Normal", URL: "https://gh/10", Assignees: []string{"alice"}, DueDate: timePtr(2026, 5, 5)},
			},
			want: []string{"*@alice* (3 task(s), 🔴 2 urgent)"},
		},
		{
			name: "draft item no url uses title only",
			items: []github.ProjectItem{
				{Title: "Draft task", Assignees: []string{"alice"}},
			},
			want:  []string{"Draft task"},
			not:   []string{"[Draft task]("},
		},
		{
			name: "sorting within group urgent first then by date",
			items: []github.ProjectItem{
				{Title: "B late", URL: "https://gh/11", Assignees: []string{"alice"}, DueDate: timePtr(2026, 5, 10)},
				{Title: "A early", URL: "https://gh/12", Assignees: []string{"alice"}, DueDate: timePtr(2026, 5, 5)},
				{Title: "Urgent", URL: "https://gh/13", Assignees: []string{"alice"}, DueDate: timePtr(2026, 4, 29)},
			},
			want: []string{
				"Urgent", // urgent first
			},
		},
		{
			name: "items with due date come before items without",
			items: []github.ProjectItem{
				{Title: "Z no date", Assignees: []string{"bob"}},
				{Title: "A no date", Assignees: []string{"bob"}},
				{Title: "B with date", URL: "https://gh/14", Assignees: []string{"bob"}, DueDate: timePtr(2026, 5, 1)},
			},
			want: []string{"B with date"}, // items with due dates appear first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Format(tt.items)

			// Check positive assertions
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("Format() missing %q\n--- got ---\n%s", w, got)
				}
			}

			// Check negative assertions
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("Format() should not contain %q\n--- got ---\n%s", n, got)
				}
			}
		})
	}
}

// ---------- TestSendReport ----------

func TestSendReport(t *testing.T) {
	ctx := context.Background()

	t.Run("github error sends warning", func(t *testing.T) {
		gh := &mockGitHubClient{err: errors.New("network down")}
		bl := &mockBaleClient{}
		r := newTestReporter(gh, bl, 2)

		err := r.SendReport(ctx, "g-1", "PVT_1")
		if err == nil {
			t.Fatal("expected error from SendReport")
		}
		if len(bl.messages) != 1 {
			t.Fatalf("expected 1 warning message, got %d", len(bl.messages))
		}
		if !strings.Contains(bl.messages[0], "Could not fetch") {
			t.Errorf("unexpected warning: %s", bl.messages[0])
		}
	})

	t.Run("empty board sends no-items message", func(t *testing.T) {
		gh := &mockGitHubClient{items: nil}
		bl := &mockBaleClient{}
		r := newTestReporter(gh, bl, 2)

		err := r.SendReport(ctx, "g-1", "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(bl.messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(bl.messages))
		}
		if !strings.Contains(bl.messages[0], "No items on the board") {
			t.Errorf("unexpected: %s", bl.messages[0])
		}
	})

	t.Run("success sends formatted report", func(t *testing.T) {
		gh := &mockGitHubClient{items: []github.ProjectItem{
			{Title: "Task 1", URL: "https://gh/1", Assignees: []string{"alice"}},
		}}
		bl := &mockBaleClient{}
		r := newTestReporter(gh, bl, 2)

		err := r.SendReport(ctx, "g-1", "PVT_1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(bl.messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(bl.messages))
		}
		if !strings.Contains(bl.messages[0], "Task 1") {
			t.Errorf("unexpected: %s", bl.messages[0])
		}
	})

	t.Run("bale send fails after fetch", func(t *testing.T) {
		gh := &mockGitHubClient{items: []github.ProjectItem{
			{Title: "Task 1", Assignees: []string{"alice"}},
		}}
		bl := &mockBaleClient{err: errors.New("send failed")}
		r := newTestReporter(gh, bl, 2)

		err := r.SendReport(ctx, "g-1", "PVT_1")
		if err == nil {
			t.Fatal("expected error from SendReport")
		}
		if !strings.Contains(err.Error(), "send message") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ---------- TestSplitMessage ----------

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want int // number of chunks
	}{
		{name: "fits in one", text: "short", max: 4000, want: 1},
		{name: "exactly at limit", text: strings.Repeat("a", 100), max: 100, want: 1},
		{name: "one line over limit kept intact", text: strings.Repeat("a", 101), max: 100, want: 2},
		{name: "empty string", text: "", max: 100, want: 1},
		{
			name: "respects line boundaries",
			text: "line-a\nline-b\nline-c\nline-d\nline-e",
			max:  15,
			want: 3, // "line-a\nline-b", "line-c\nline-d", "line-e"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMessage(tt.text, tt.max)

			// Verify chunk count
			if len(got) != tt.want {
				t.Errorf("splitMessage() = %d chunks, want %d: %v", len(got), tt.want, got)
			}

			// Verify each chunk is <= maxLen (skip if it was a single oversized line)
			for i, chunk := range got {
				if len(chunk) > tt.max && len(strings.Split(tt.text, "\n")) > 1 {
					t.Errorf("chunk %d length %d exceeds max %d", i, len(chunk), tt.max)
				}
			}

			// Verify no line was broken mid-way (lines appear intact in some chunk)
			for _, line := range strings.Split(tt.text, "\n") {
				found := false
				for _, chunk := range got {
					if strings.Contains(chunk, line) {
						found = true
						break
					}
				}
				if !found && line != "" {
					t.Errorf("line %q not found in any chunk", line)
				}
			}
		})
	}
}

// ---------- TestNewDefaults ----------

func TestNewDefaults(t *testing.T) {
	t.Run("urgencyDays zero defaults to 2", func(t *testing.T) {
		r := New(nil, nil, 0)
		if r.urgencyDays != 2 {
			t.Errorf("urgencyDays = %d, want 2", r.urgencyDays)
		}
	})

	t.Run("urgencyDays negative defaults to 2", func(t *testing.T) {
		r := New(nil, nil, -1)
		if r.urgencyDays != 2 {
			t.Errorf("urgencyDays = %d, want 2", r.urgencyDays)
		}
	})

	t.Run("urgencyDays positive preserved", func(t *testing.T) {
		r := New(nil, nil, 5)
		if r.urgencyDays != 5 {
			t.Errorf("urgencyDays = %d, want 5", r.urgencyDays)
		}
	})

	t.Run("now defaults to time.Now", func(t *testing.T) {
		r := New(nil, nil, 2)
		if r.now == nil {
			t.Fatal("now should not be nil")
		}
		before := time.Now()
		got := r.now()
		after := time.Now()
		if got.Before(before) || got.After(after) {
			t.Errorf("now() = %v, expected between %v and %v", got, before, after)
		}
	})
}
