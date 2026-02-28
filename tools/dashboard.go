package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DashboardTools provides an aggregate dashboard view across all entity types.
type DashboardTools struct {
	storage storage.Storage
}

// NewDashboardTools creates a new DashboardTools instance.
func NewDashboardTools(s storage.Storage) *DashboardTools {
	return &DashboardTools{storage: s}
}

// GetDashboardInput is the input schema for the get_dashboard tool.
type GetDashboardInput struct {
	IncludeCompleted bool `json:"include_completed,omitempty" jsonschema:"Include completed items in the response. Defaults to false."`
}

// GetDashboardOutput is the output for the get_dashboard tool.
type GetDashboardOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Dashboard response types

// DashboardResult is the top-level dashboard response.
type DashboardResult struct {
	Todos       DashboardTodos    `json:"todos"`
	Reminders   DashboardReminders `json:"reminders"`
	ReadingList DashboardReading  `json:"reading_list"`
	Strategy    DashboardStrategy `json:"strategy"`
}

// DashboardTodos is the todos section of the dashboard.
type DashboardTodos struct {
	Active         []TodoItem `json:"active"`
	Completed      []TodoItem `json:"completed,omitempty"`
	ActiveCount    int        `json:"active_count"`
	CompletedCount int        `json:"completed_count"`
}

// DashboardReminders is the reminders section of the dashboard.
type DashboardReminders struct {
	Upcoming       []ReminderItem `json:"upcoming"`
	Overdue        []ReminderItem `json:"overdue"`
	Completed      []ReminderItem `json:"completed,omitempty"`
	CompletedCount int            `json:"completed_count"`
}

// DashboardReading is the reading list section of the dashboard.
type DashboardReading struct {
	Unread    []ReadingListItem `json:"unread"`
	Read      []ReadingListItem `json:"read,omitempty"`
	ReadCount int               `json:"read_count"`
}

// DashboardStrategy is the strategy section of the dashboard.
type DashboardStrategy struct {
	CurrentPhase    string          `json:"current_phase"`
	Active          []MilestoneItem `json:"active_milestones"`
	Completed       []MilestoneItem `json:"completed,omitempty"`
	CompletedCount  int             `json:"completed_count"`
	RecentNotes     []string        `json:"recent_notes"`
	TotalNotes      int             `json:"total_notes"`
}

// Register registers dashboard tools with the MCP server.
func (d *DashboardTools) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dashboard",
		Description: "Get an aggregate summary of all Momentum data: todos, reminders, reading list, and strategy milestones. Ideal for morning check-ins and productivity overviews.",
	}, d.getDashboard)
}

func (d *DashboardTools) getDashboard(ctx context.Context, req *mcp.CallToolRequest, input GetDashboardInput) (*mcp.CallToolResult, GetDashboardOutput, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	sevenDaysFromNow := today.AddDate(0, 0, 7)

	result := DashboardResult{}

	// Todos
	todosContent, _, err := d.storage.ReadFile(ctx, "todos.md")
	if err == nil {
		tf, parseErr := storage.ParseTodos(todosContent)
		if parseErr == nil {
			active := make([]TodoItem, len(tf.Active))
			for i, t := range tf.Active {
				active[i] = todoToItem(t)
			}
			result.Todos.Active = active
			result.Todos.ActiveCount = len(tf.Active)
			result.Todos.CompletedCount = len(tf.Completed)

			if input.IncludeCompleted {
				completed := make([]TodoItem, len(tf.Completed))
				for i, t := range tf.Completed {
					completed[i] = todoToItem(t)
				}
				result.Todos.Completed = completed
			}
		}
	}

	// Reminders
	remindersContent, _, err := d.storage.ReadFile(ctx, "reminders.md")
	if err == nil {
		rf, parseErr := storage.ParseReminders(remindersContent)
		if parseErr == nil {
			for _, r := range rf.Upcoming {
				item := reminderToItem(r, today)
				if item.Overdue {
					result.Reminders.Overdue = append(result.Reminders.Overdue, item)
				} else if r.Date.Before(sevenDaysFromNow) || r.Date.Equal(sevenDaysFromNow) {
					result.Reminders.Upcoming = append(result.Reminders.Upcoming, item)
				}
			}
			result.Reminders.CompletedCount = len(rf.Completed)

			if input.IncludeCompleted {
				completed := make([]ReminderItem, len(rf.Completed))
				for i, r := range rf.Completed {
					completed[i] = reminderToItem(r, today)
				}
				result.Reminders.Completed = completed
			}
		}
	}

	// Ensure nil slices become empty arrays in JSON
	if result.Reminders.Upcoming == nil {
		result.Reminders.Upcoming = []ReminderItem{}
	}
	if result.Reminders.Overdue == nil {
		result.Reminders.Overdue = []ReminderItem{}
	}

	// Reading list
	readingContent, _, err := d.storage.ReadFile(ctx, "reading-list.md")
	if err == nil {
		rl, parseErr := storage.ParseReadingList(readingContent)
		if parseErr == nil {
			unread := make([]ReadingListItem, len(rl.ToRead))
			for i, r := range rl.ToRead {
				unread[i] = readingToItem(r)
			}
			result.ReadingList.Unread = unread
			result.ReadingList.ReadCount = len(rl.Read)

			if input.IncludeCompleted {
				read := make([]ReadingListItem, len(rl.Read))
				for i, r := range rl.Read {
					read[i] = readingToItem(r)
				}
				result.ReadingList.Read = read
			}
		}
	}

	// Strategy
	strategyContent, _, err := d.storage.ReadFile(ctx, "strategy.md")
	if err == nil {
		s, parseErr := storage.ParseStrategy(strategyContent)
		if parseErr == nil {
			result.Strategy.CurrentPhase = s.CurrentPhase

			active := make([]MilestoneItem, len(s.ActiveMilestones))
			for i, m := range s.ActiveMilestones {
				active[i] = milestoneToItem(m)
			}
			result.Strategy.Active = active
			result.Strategy.CompletedCount = len(s.CompletedMilestones)
			result.Strategy.TotalNotes = len(s.Notes)

			// Recent notes: last 5
			recentCount := 5
			if len(s.Notes) < recentCount {
				recentCount = len(s.Notes)
			}
			if recentCount > 0 {
				result.Strategy.RecentNotes = s.Notes[len(s.Notes)-recentCount:]
			} else {
				result.Strategy.RecentNotes = []string{}
			}

			if input.IncludeCompleted {
				completed := make([]MilestoneItem, len(s.CompletedMilestones))
				for i, m := range s.CompletedMilestones {
					completed[i] = milestoneToItem(m)
				}
				result.Strategy.Completed = completed
			}
		}
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, GetDashboardOutput{}, fmt.Errorf("marshaling dashboard: %w", err)
	}

	return nil, GetDashboardOutput{
		Success: true,
		Message: string(jsonBytes),
	}, nil
}
