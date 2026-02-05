package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StrategyTools provides tools for managing strategy milestones and notes.
type StrategyTools struct {
	storage storage.Storage
}

// NewStrategyTools creates a new StrategyTools instance.
func NewStrategyTools(s storage.Storage) *StrategyTools {
	return &StrategyTools{storage: s}
}

// UpdateMilestoneInput is the input schema for the update_milestone tool.
type UpdateMilestoneInput struct {
	Text     string `json:"text" jsonschema:"Text to match against milestone descriptions. Can be partial match."`
	Complete bool   `json:"complete" jsonschema:"Set to true to mark as complete, false to mark as incomplete"`
}

// UpdateMilestoneOutput is the output for the update_milestone tool.
type UpdateMilestoneOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// AddNoteInput is the input schema for the add_note tool.
type AddNoteInput struct {
	Note string `json:"note" jsonschema:"The note text to add to the strategy notes section"`
}

// AddNoteOutput is the output for the add_note tool.
type AddNoteOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Register registers strategy tools with the MCP server.
func (t *StrategyTools) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_milestone",
		Description: "Toggle a milestone's completion status",
	}, t.updateMilestone)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_note",
		Description: "Add a note to the strategy notes section",
	}, t.addNote)
}

func (t *StrategyTools) updateMilestone(ctx context.Context, req *mcp.CallToolRequest, input UpdateMilestoneInput) (*mcp.CallToolResult, UpdateMilestoneOutput, error) {
	if strings.TrimSpace(input.Text) == "" {
		return nil, UpdateMilestoneOutput{
			Success: false,
			Message: "Search text cannot be empty",
		}, nil
	}

	// Read current strategy
	content, sha, err := t.storage.ReadFile(ctx, "strategy.md")
	if err != nil {
		return nil, UpdateMilestoneOutput{}, fmt.Errorf("reading strategy.md: %w", err)
	}

	s, err := storage.ParseStrategy(content)
	if err != nil {
		return nil, UpdateMilestoneOutput{}, fmt.Errorf("parsing strategy: %w", err)
	}

	searchText := strings.ToLower(strings.TrimSpace(input.Text))

	if input.Complete {
		// Find matching active milestone to complete
		var matches []int
		for i, m := range s.ActiveMilestones {
			if strings.Contains(strings.ToLower(m.Text), searchText) {
				matches = append(matches, i)
			}
		}

		if len(matches) == 0 {
			return nil, UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("No active milestone found matching %q", input.Text),
			}, nil
		}

		if len(matches) > 1 {
			var matchTexts []string
			for _, idx := range matches {
				matchTexts = append(matchTexts, fmt.Sprintf("- %s", s.ActiveMilestones[idx].Text))
			}
			return nil, UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("Multiple milestones match %q. Please be more specific:\n%s", input.Text, strings.Join(matchTexts, "\n")),
			}, nil
		}

		// Mark as completed
		idx := matches[0]
		milestone := s.ActiveMilestones[idx]
		milestone.Completed = true
		now := time.Now().UTC().Truncate(24 * time.Hour)
		milestone.CompletedAt = &now

		// Move from active to completed
		s.ActiveMilestones = append(s.ActiveMilestones[:idx], s.ActiveMilestones[idx+1:]...)
		s.CompletedMilestones = append([]storage.Milestone{milestone}, s.CompletedMilestones...)

		// Serialize and write back
		newContent := storage.SerializeStrategy(s)
		if err := t.storage.WriteFile(ctx, "strategy.md", newContent, sha, fmt.Sprintf("Complete milestone: %s", truncate(milestone.Text, 50))); err != nil {
			if err == storage.ErrConflict {
				return nil, UpdateMilestoneOutput{
					Success: false,
					Message: "File was modified by another process. Please try again.",
				}, nil
			}
			return nil, UpdateMilestoneOutput{}, fmt.Errorf("writing strategy.md: %w", err)
		}

		return nil, UpdateMilestoneOutput{
			Success: true,
			Message: fmt.Sprintf("Completed milestone: %s", milestone.Text),
		}, nil
	} else {
		// Find matching completed milestone to uncomplete
		var matches []int
		for i, m := range s.CompletedMilestones {
			if strings.Contains(strings.ToLower(m.Text), searchText) {
				matches = append(matches, i)
			}
		}

		if len(matches) == 0 {
			return nil, UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("No completed milestone found matching %q", input.Text),
			}, nil
		}

		if len(matches) > 1 {
			var matchTexts []string
			for _, idx := range matches {
				matchTexts = append(matchTexts, fmt.Sprintf("- %s", s.CompletedMilestones[idx].Text))
			}
			return nil, UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("Multiple milestones match %q. Please be more specific:\n%s", input.Text, strings.Join(matchTexts, "\n")),
			}, nil
		}

		// Mark as incomplete
		idx := matches[0]
		milestone := s.CompletedMilestones[idx]
		milestone.Completed = false
		milestone.CompletedAt = nil

		// Move from completed to active
		s.CompletedMilestones = append(s.CompletedMilestones[:idx], s.CompletedMilestones[idx+1:]...)
		s.ActiveMilestones = append(s.ActiveMilestones, milestone)

		// Serialize and write back
		newContent := storage.SerializeStrategy(s)
		if err := t.storage.WriteFile(ctx, "strategy.md", newContent, sha, fmt.Sprintf("Reopen milestone: %s", truncate(milestone.Text, 50))); err != nil {
			if err == storage.ErrConflict {
				return nil, UpdateMilestoneOutput{
					Success: false,
					Message: "File was modified by another process. Please try again.",
				}, nil
			}
			return nil, UpdateMilestoneOutput{}, fmt.Errorf("writing strategy.md: %w", err)
		}

		return nil, UpdateMilestoneOutput{
			Success: true,
			Message: fmt.Sprintf("Reopened milestone: %s", milestone.Text),
		}, nil
	}
}

func (t *StrategyTools) addNote(ctx context.Context, req *mcp.CallToolRequest, input AddNoteInput) (*mcp.CallToolResult, AddNoteOutput, error) {
	if strings.TrimSpace(input.Note) == "" {
		return nil, AddNoteOutput{
			Success: false,
			Message: "Note text cannot be empty",
		}, nil
	}

	// Read current strategy
	content, sha, err := t.storage.ReadFile(ctx, "strategy.md")
	if err != nil {
		return nil, AddNoteOutput{}, fmt.Errorf("reading strategy.md: %w", err)
	}

	s, err := storage.ParseStrategy(content)
	if err != nil {
		return nil, AddNoteOutput{}, fmt.Errorf("parsing strategy: %w", err)
	}

	// Add the note
	s.Notes = append(s.Notes, strings.TrimSpace(input.Note))

	// Serialize and write back
	newContent := storage.SerializeStrategy(s)
	if err := t.storage.WriteFile(ctx, "strategy.md", newContent, sha, "Add strategy note"); err != nil {
		if err == storage.ErrConflict {
			return nil, AddNoteOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, AddNoteOutput{}, fmt.Errorf("writing strategy.md: %w", err)
	}

	return nil, AddNoteOutput{
		Success: true,
		Message: fmt.Sprintf("Added note: %s", truncate(input.Note, 50)),
	}, nil
}
