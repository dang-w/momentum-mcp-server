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

// RemindersResource provides read access to reminders.
type RemindersResource struct {
	storage storage.Storage
}

// NewRemindersResource creates a new RemindersResource.
func NewRemindersResource(s storage.Storage) *RemindersResource {
	return &RemindersResource{storage: s}
}

// Register registers the momentum://reminders resource with the MCP server.
func (r *RemindersResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://reminders",
		Name:        "Reminders",
		Description: "Upcoming and completed reminders",
		MIMEType:    "text/markdown",
	}, r.Read)
}

// Read fetches and formats the reminders.
func (r *RemindersResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	content, _, err := r.storage.ReadFile(ctx, "reminders.md")
	if err != nil {
		return nil, fmt.Errorf("reading reminders.md: %w", err)
	}

	rf, err := storage.ParseReminders(content)
	if err != nil {
		return nil, fmt.Errorf("parsing reminders: %w", err)
	}

	// Sort upcoming reminders by date
	sort.Slice(rf.Upcoming, func(i, j int) bool {
		return rf.Upcoming[i].Date.Before(rf.Upcoming[j].Date)
	})

	today := time.Now().UTC().Truncate(24 * time.Hour)

	// Build readable markdown output
	var b strings.Builder
	b.WriteString("# Reminders\n\n")

	// Summary
	overdueCount := 0
	for _, r := range rf.Upcoming {
		if r.Date.Before(today) {
			overdueCount++
		}
	}
	b.WriteString(fmt.Sprintf("**%d upcoming**", len(rf.Upcoming)))
	if overdueCount > 0 {
		b.WriteString(fmt.Sprintf(" (%d overdue)", overdueCount))
	}
	b.WriteString(fmt.Sprintf(", **%d completed**\n\n", len(rf.Completed)))

	// Upcoming section
	if len(rf.Upcoming) > 0 {
		b.WriteString("## ⏰ Upcoming\n")
		for _, reminder := range rf.Upcoming {
			prefix := ""
			if reminder.Date.Before(today) {
				prefix = "⚠️ OVERDUE: "
			} else if reminder.Date.Equal(today) {
				prefix = "📍 TODAY: "
			}
			b.WriteString(fmt.Sprintf("- %s%s (%s)\n", prefix, reminder.Text, reminder.Date.Format("2006-01-02")))
		}
		b.WriteString("\n")
	}

	// Recently completed (last 5)
	if len(rf.Completed) > 0 {
		b.WriteString("## ✅ Recently Completed\n")
		limit := 5
		if len(rf.Completed) < limit {
			limit = len(rf.Completed)
		}
		for i := 0; i < limit; i++ {
			reminder := rf.Completed[i]
			b.WriteString(fmt.Sprintf("- %s (%s)\n", reminder.Text, reminder.Date.Format("2006-01-02")))
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://reminders",
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		},
	}, nil
}
