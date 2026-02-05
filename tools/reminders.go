package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReminderTools provides tools for managing reminders.
type ReminderTools struct {
	storage storage.Storage
}

// NewReminderTools creates a new ReminderTools instance.
func NewReminderTools(s storage.Storage) *ReminderTools {
	return &ReminderTools{storage: s}
}

// SetReminderInput is the input schema for the set_reminder tool.
type SetReminderInput struct {
	Date string `json:"date" jsonschema:"The date for the reminder in YYYY-MM-DD format"`
	Text string `json:"text" jsonschema:"The reminder text"`
}

// SetReminderOutput is the output for the set_reminder tool.
type SetReminderOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CompleteReminderInput is the input schema for the complete_reminder tool.
type CompleteReminderInput struct {
	Text string `json:"text" jsonschema:"Text to match against reminder descriptions. Can be partial match."`
}

// CompleteReminderOutput is the output for the complete_reminder tool.
type CompleteReminderOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Register registers reminder tools with the MCP server.
func (t *ReminderTools) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_reminder",
		Description: "Set a new reminder for a specific date",
	}, t.setReminder)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "complete_reminder",
		Description: "Mark a reminder as completed",
	}, t.completeReminder)
}

func (t *ReminderTools) setReminder(ctx context.Context, req *mcp.CallToolRequest, input SetReminderInput) (*mcp.CallToolResult, SetReminderOutput, error) {
	if strings.TrimSpace(input.Date) == "" {
		return nil, SetReminderOutput{
			Success: false,
			Message: "Date cannot be empty",
		}, nil
	}
	if strings.TrimSpace(input.Text) == "" {
		return nil, SetReminderOutput{
			Success: false,
			Message: "Reminder text cannot be empty",
		}, nil
	}

	// Parse the date
	date, err := time.Parse("2006-01-02", strings.TrimSpace(input.Date))
	if err != nil {
		return nil, SetReminderOutput{
			Success: false,
			Message: fmt.Sprintf("Invalid date format %q. Use YYYY-MM-DD format.", input.Date),
		}, nil
	}

	// Read current reminders
	content, sha, err := t.storage.ReadFile(ctx, "reminders.md")
	if err != nil {
		return nil, SetReminderOutput{}, fmt.Errorf("reading reminders.md: %w", err)
	}

	rf, err := storage.ParseReminders(content)
	if err != nil {
		return nil, SetReminderOutput{}, fmt.Errorf("parsing reminders: %w", err)
	}

	// Add the new reminder
	newReminder := storage.Reminder{
		Date:  date,
		Text:  strings.TrimSpace(input.Text),
		Added: time.Now().UTC().Truncate(24 * time.Hour),
	}
	rf.Upcoming = append(rf.Upcoming, newReminder)

	// Serialize and write back
	newContent := storage.SerializeReminders(rf)
	if err := t.storage.WriteFile(ctx, "reminders.md", newContent, sha, fmt.Sprintf("Set reminder: %s", truncate(input.Text, 50))); err != nil {
		if err == storage.ErrConflict {
			return nil, SetReminderOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, SetReminderOutput{}, fmt.Errorf("writing reminders.md: %w", err)
	}

	return nil, SetReminderOutput{
		Success: true,
		Message: fmt.Sprintf("Set reminder for %s: %s", date.Format("2006-01-02"), input.Text),
	}, nil
}

func (t *ReminderTools) completeReminder(ctx context.Context, req *mcp.CallToolRequest, input CompleteReminderInput) (*mcp.CallToolResult, CompleteReminderOutput, error) {
	if strings.TrimSpace(input.Text) == "" {
		return nil, CompleteReminderOutput{
			Success: false,
			Message: "Search text cannot be empty",
		}, nil
	}

	// Read current reminders
	content, sha, err := t.storage.ReadFile(ctx, "reminders.md")
	if err != nil {
		return nil, CompleteReminderOutput{}, fmt.Errorf("reading reminders.md: %w", err)
	}

	rf, err := storage.ParseReminders(content)
	if err != nil {
		return nil, CompleteReminderOutput{}, fmt.Errorf("parsing reminders: %w", err)
	}

	// Find matching reminders
	searchText := strings.ToLower(strings.TrimSpace(input.Text))
	var matches []int
	for i, r := range rf.Upcoming {
		if strings.Contains(strings.ToLower(r.Text), searchText) {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return nil, CompleteReminderOutput{
			Success: false,
			Message: fmt.Sprintf("No upcoming reminder found matching %q", input.Text),
		}, nil
	}

	if len(matches) > 1 {
		var matchTexts []string
		for _, idx := range matches {
			r := rf.Upcoming[idx]
			matchTexts = append(matchTexts, fmt.Sprintf("- %s (%s)", r.Text, r.Date.Format("2006-01-02")))
		}
		return nil, CompleteReminderOutput{
			Success: false,
			Message: fmt.Sprintf("Multiple reminders match %q. Please be more specific:\n%s", input.Text, strings.Join(matchTexts, "\n")),
		}, nil
	}

	// Mark as completed
	idx := matches[0]
	reminder := rf.Upcoming[idx]
	reminder.Completed = true
	now := time.Now().UTC().Truncate(24 * time.Hour)
	reminder.CompletedAt = &now

	// Move from upcoming to completed
	rf.Upcoming = append(rf.Upcoming[:idx], rf.Upcoming[idx+1:]...)
	rf.Completed = append([]storage.Reminder{reminder}, rf.Completed...) // Add to front

	// Serialize and write back
	newContent := storage.SerializeReminders(rf)
	if err := t.storage.WriteFile(ctx, "reminders.md", newContent, sha, fmt.Sprintf("Complete reminder: %s", truncate(reminder.Text, 50))); err != nil {
		if err == storage.ErrConflict {
			return nil, CompleteReminderOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, CompleteReminderOutput{}, fmt.Errorf("writing reminders.md: %w", err)
	}

	return nil, CompleteReminderOutput{
		Success: true,
		Message: fmt.Sprintf("Completed reminder: %s", reminder.Text),
	}, nil
}
