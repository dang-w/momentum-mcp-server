package tools

import (
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
)

// Response types for list/read tools and the dashboard.
// These are JSON-serializable representations of storage types.

// TodoItem is a JSON-serializable todo for API responses.
type TodoItem struct {
	Text        string  `json:"text"`
	Priority    string  `json:"priority"`
	Completed   bool    `json:"completed"`
	Added       string  `json:"added,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// ReminderItem is a JSON-serializable reminder for API responses.
type ReminderItem struct {
	Date        string  `json:"date"`
	Text        string  `json:"text"`
	Completed   bool    `json:"completed"`
	Overdue     bool    `json:"overdue"`
	Added       string  `json:"added,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// ReadingListItem is a JSON-serializable reading list entry for API responses.
type ReadingListItem struct {
	URL    string  `json:"url"`
	Notes  string  `json:"notes,omitempty"`
	Read   bool    `json:"read"`
	Added  string  `json:"added,omitempty"`
	ReadAt *string `json:"read_at,omitempty"`
}

// MilestoneItem is a JSON-serializable milestone for API responses.
type MilestoneItem struct {
	Text        string  `json:"text"`
	Due         *string `json:"due,omitempty"`
	Completed   bool    `json:"completed"`
	Added       string  `json:"added,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// Conversion helpers

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func todoToItem(t storage.Todo) TodoItem {
	return TodoItem{
		Text:        t.Text,
		Priority:    string(t.Priority),
		Completed:   t.Completed,
		Added:       formatDate(t.Added),
		CompletedAt: formatDatePtr(t.CompletedAt),
	}
}

func reminderToItem(r storage.Reminder, today time.Time) ReminderItem {
	return ReminderItem{
		Date:        formatDate(r.Date),
		Text:        r.Text,
		Completed:   r.Completed,
		Overdue:     !r.Completed && r.Date.Before(today),
		Added:       formatDate(r.Added),
		CompletedAt: formatDatePtr(r.CompletedAt),
	}
}

func readingToItem(r storage.ReadingItem) ReadingListItem {
	return ReadingListItem{
		URL:    r.URL,
		Notes:  r.Notes,
		Read:   r.Read,
		Added:  formatDate(r.Added),
		ReadAt: formatDatePtr(r.ReadAt),
	}
}

func milestoneToItem(m storage.Milestone) MilestoneItem {
	return MilestoneItem{
		Text:        m.Text,
		Due:         formatDatePtr(m.Due),
		Completed:   m.Completed,
		Added:       formatDate(m.Added),
		CompletedAt: formatDatePtr(m.CompletedAt),
	}
}
