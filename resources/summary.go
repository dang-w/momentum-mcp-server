package resources

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SummaryResource provides an aggregated weekly summary view.
type SummaryResource struct {
	storage        storage.Storage
	githubActivity *GitHubActivityResource
}

// NewSummaryResource creates a new SummaryResource.
func NewSummaryResource(s storage.Storage, ga *GitHubActivityResource) *SummaryResource {
	return &SummaryResource{
		storage:        s,
		githubActivity: ga,
	}
}

// Register registers the momentum://weekly-summary resource with the MCP server.
func (r *SummaryResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://weekly-summary",
		Name:        "Weekly Summary",
		Description: "Aggregated overview of todos, strategy, reading list, reminders, and GitHub activity",
		MIMEType:    "text/markdown",
	}, r.Read)
}

// Read fetches data from all sources and produces an aggregated summary.
func (r *SummaryResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	// Calculate the week boundaries (Monday-Sunday)
	now := time.Now()
	weekStart := startOfWeek(now)
	weekEnd := weekStart.AddDate(0, 0, 6)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Weekly Summary (%s to %s)\n\n",
		weekStart.Format("2006-01-02"),
		weekEnd.Format("2006-01-02")))

	// --- Momentum (GitHub Activity) ---
	b.WriteString("### Momentum\n")
	if r.githubActivity != nil {
		activity, err := r.githubActivity.getActivity(ctx)
		if err != nil {
			b.WriteString("- GitHub: *Data temporarily unavailable*\n")
		} else {
			b.WriteString(fmt.Sprintf("- GitHub: %d commits across %d repos",
				activity.CommitsThisWeek, activity.ReposActive))
			if activity.StreakDays > 0 {
				b.WriteString(fmt.Sprintf(", %d-day streak", activity.StreakDays))
			}
			b.WriteString("\n")

			if !activity.LastCommit.IsZero() {
				timeSince := formatTimeSince(activity.LastCommit)
				b.WriteString(fmt.Sprintf("- Last commit: %s\n", timeSince))
			}
		}
	} else {
		b.WriteString("- GitHub: *Not configured*\n")
	}
	b.WriteString("\n")

	// --- Focus Areas (Todos + Strategy + Reminders) ---
	b.WriteString("### Focus Areas\n")

	// High priority todos
	todosContent, _, err := r.storage.ReadFile(ctx, "todos.md")
	if err == nil {
		tf, err := storage.ParseTodos(todosContent)
		if err == nil {
			highPriorityCount := 0
			for _, todo := range tf.Active {
				if todo.Priority == storage.PriorityHigh {
					highPriorityCount++
				}
			}
			if highPriorityCount > 0 {
				b.WriteString(fmt.Sprintf("- %d high-priority todos pending\n", highPriorityCount))
			} else if len(tf.Active) > 0 {
				b.WriteString(fmt.Sprintf("- %d todos pending (no high priority)\n", len(tf.Active)))
			} else {
				b.WriteString("- No active todos\n")
			}
		}
	}

	// Milestones due this week
	strategyContent, _, err := r.storage.ReadFile(ctx, "strategy.md")
	if err == nil {
		s, err := storage.ParseStrategy(strategyContent)
		if err == nil {
			milestonesThisWeek := 0
			for _, m := range s.ActiveMilestones {
				if m.Due != nil && !m.Due.Before(weekStart) && !m.Due.After(weekEnd) {
					milestonesThisWeek++
					b.WriteString(fmt.Sprintf("- Milestone due this week: \"%s\"\n", m.Text))
				}
			}
			if milestonesThisWeek == 0 && len(s.ActiveMilestones) > 0 {
				b.WriteString(fmt.Sprintf("- %d active milestones (none due this week)\n", len(s.ActiveMilestones)))
			}
		}
	}

	// Overdue reminders
	remindersContent, _, err := r.storage.ReadFile(ctx, "reminders.md")
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if err == nil {
		rf, err := storage.ParseReminders(remindersContent)
		if err == nil {
			var overdue []storage.Reminder
			for _, reminder := range rf.Upcoming {
				if reminder.Date.Before(today) {
					overdue = append(overdue, reminder)
				}
			}
			if len(overdue) > 0 {
				// Sort by date (oldest first)
				sort.Slice(overdue, func(i, j int) bool {
					return overdue[i].Date.Before(overdue[j].Date)
				})
				for _, reminder := range overdue {
					daysOverdue := int(today.Sub(reminder.Date).Hours() / 24)
					b.WriteString(fmt.Sprintf("- ⚠️ Overdue reminder: \"%s\" (%d days overdue)\n",
						reminder.Text, daysOverdue))
				}
			}
		}
	}
	b.WriteString("\n")

	// --- Reading Queue ---
	b.WriteString("### Reading Queue\n")
	readingContent, _, err := r.storage.ReadFile(ctx, "reading-list.md")
	if err == nil {
		rl, err := storage.ParseReadingList(readingContent)
		if err == nil {
			// Count items read this week
			readThisWeek := 0
			for _, item := range rl.Read {
				if item.ReadAt != nil && !item.ReadAt.Before(weekStart) && item.ReadAt.Before(weekEnd.AddDate(0, 0, 1)) {
					readThisWeek++
				}
			}
			b.WriteString(fmt.Sprintf("- %d articles queued", len(rl.ToRead)))
			if readThisWeek > 0 {
				b.WriteString(fmt.Sprintf(", %d read this week", readThisWeek))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// --- Recent Completions ---
	b.WriteString("### Recent Completions\n")
	completions := r.getRecentCompletions(ctx, weekStart)
	if len(completions) == 0 {
		b.WriteString("- *No completions this week*\n")
	} else {
		for _, completion := range completions {
			b.WriteString(fmt.Sprintf("- ✓ %s (%s)\n", completion.text, completion.date.Format("Jan 2")))
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://weekly-summary",
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		},
	}, nil
}

// completion represents a completed item from any source.
type completion struct {
	text string
	date time.Time
}

// getRecentCompletions gathers completions from todos, strategy, reminders.
func (r *SummaryResource) getRecentCompletions(ctx context.Context, since time.Time) []completion {
	var completions []completion

	// Completed todos
	todosContent, _, err := r.storage.ReadFile(ctx, "todos.md")
	if err == nil {
		tf, _ := storage.ParseTodos(todosContent)
		for _, todo := range tf.Completed {
			if todo.CompletedAt != nil && !todo.CompletedAt.Before(since) {
				completions = append(completions, completion{
					text: todo.Text,
					date: *todo.CompletedAt,
				})
			}
		}
	}

	// Completed milestones
	strategyContent, _, err := r.storage.ReadFile(ctx, "strategy.md")
	if err == nil {
		s, _ := storage.ParseStrategy(strategyContent)
		for _, m := range s.CompletedMilestones {
			if m.CompletedAt != nil && !m.CompletedAt.Before(since) {
				completions = append(completions, completion{
					text: m.Text,
					date: *m.CompletedAt,
				})
			}
		}
	}

	// Completed reminders
	remindersContent, _, err := r.storage.ReadFile(ctx, "reminders.md")
	if err == nil {
		rf, _ := storage.ParseReminders(remindersContent)
		for _, reminder := range rf.Completed {
			if reminder.CompletedAt != nil && !reminder.CompletedAt.Before(since) {
				completions = append(completions, completion{
					text: reminder.Text,
					date: *reminder.CompletedAt,
				})
			}
		}
	}

	// Sort by date descending (most recent first)
	sort.Slice(completions, func(i, j int) bool {
		return completions[i].date.After(completions[j].date)
	})

	// Limit to 5 most recent
	if len(completions) > 5 {
		completions = completions[:5]
	}

	return completions
}

// formatTimeSince returns a human-readable time since string.
func formatTimeSince(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(duration.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
