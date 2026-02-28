package tools

import (
	"context"
	"encoding/json"
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
	Text     string `json:"text,omitempty" jsonschema:"Text to match against milestone descriptions. Can be partial match."`
	ID       string `json:"id,omitempty" jsonschema:"ID of the milestone to update. More reliable than text matching. Use get_milestones to find IDs."`
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

// ListNotesInput is the input schema for the list_notes tool.
type ListNotesInput struct {
	Search string `json:"search,omitempty" jsonschema:"Text to filter notes by. Case-insensitive partial match."`
}

// ListNotesOutput is the output for the list_notes tool.
type ListNotesOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListNotesResult is the response payload for list_notes.
type ListNotesResult struct {
	Notes []string `json:"notes"`
	Total int      `json:"total"`
}

// GetMilestonesInput is the input schema for the get_milestones tool.
type GetMilestonesInput struct{}

// GetMilestonesOutput is the output for the get_milestones tool.
type GetMilestonesOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetMilestonesResult is the response payload for get_milestones.
type GetMilestonesResult struct {
	CurrentPhase        string          `json:"current_phase"`
	ActiveMilestones    []MilestoneItem `json:"active_milestones"`
	CompletedMilestones []MilestoneItem `json:"completed_milestones"`
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notes",
		Description: "List strategy notes with optional text search. Notes are plain text entries without dates or IDs.",
	}, t.listNotes)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_milestones",
		Description: "Get all strategy milestones with their completion status",
	}, t.getMilestones)
}

func (t *StrategyTools) updateMilestone(ctx context.Context, req *mcp.CallToolRequest, input UpdateMilestoneInput) (*mcp.CallToolResult, UpdateMilestoneOutput, error) {
	if strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.ID) == "" {
		return nil, UpdateMilestoneOutput{
			Success: false,
			Message: "Either text or id must be provided",
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

	// Helper to find milestone by ID or text in a slice
	findMilestone := func(milestones []storage.Milestone, label string) (int, *UpdateMilestoneOutput) {
		if id := strings.TrimSpace(input.ID); id != "" {
			for i, m := range milestones {
				if m.ID == id {
					return i, nil
				}
			}
			return -1, &UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("No %s milestone found with id %q", label, input.ID),
			}
		}

		searchText := strings.ToLower(strings.TrimSpace(input.Text))
		var matches []int
		for i, m := range milestones {
			if strings.Contains(strings.ToLower(m.Text), searchText) {
				matches = append(matches, i)
			}
		}

		if len(matches) == 0 {
			return -1, &UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("No %s milestone found matching %q", label, input.Text),
			}
		}

		if len(matches) > 1 {
			var matchTexts []string
			for _, idx := range matches {
				matchTexts = append(matchTexts, fmt.Sprintf("- [%s] %s", milestones[idx].ID, milestones[idx].Text))
			}
			return -1, &UpdateMilestoneOutput{
				Success: false,
				Message: fmt.Sprintf("Multiple milestones match %q. Please be more specific or use an id:\n%s", input.Text, strings.Join(matchTexts, "\n")),
			}
		}

		return matches[0], nil
	}

	if input.Complete {
		idx, errOut := findMilestone(s.ActiveMilestones, "active")
		if errOut != nil {
			return nil, *errOut, nil
		}

		// Mark as completed
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

		itemJSON, err := json.Marshal(milestoneToItem(milestone))
		if err != nil {
			return nil, UpdateMilestoneOutput{}, fmt.Errorf("marshaling response: %w", err)
		}

		return nil, UpdateMilestoneOutput{
			Success: true,
			Message: string(itemJSON),
		}, nil
	} else {
		idx, errOut := findMilestone(s.CompletedMilestones, "completed")
		if errOut != nil {
			return nil, *errOut, nil
		}

		// Mark as incomplete
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

		itemJSON, err := json.Marshal(milestoneToItem(milestone))
		if err != nil {
			return nil, UpdateMilestoneOutput{}, fmt.Errorf("marshaling response: %w", err)
		}

		return nil, UpdateMilestoneOutput{
			Success: true,
			Message: string(itemJSON),
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

	noteJSON, err := json.Marshal(struct {
		Note  string `json:"note"`
		Total int    `json:"total_notes"`
	}{
		Note:  strings.TrimSpace(input.Note),
		Total: len(s.Notes),
	})
	if err != nil {
		return nil, AddNoteOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, AddNoteOutput{
		Success: true,
		Message: string(noteJSON),
	}, nil
}

func (t *StrategyTools) listNotes(ctx context.Context, req *mcp.CallToolRequest, input ListNotesInput) (*mcp.CallToolResult, ListNotesOutput, error) {
	content, _, err := t.storage.ReadFile(ctx, "strategy.md")
	if err != nil {
		return nil, ListNotesOutput{}, fmt.Errorf("reading strategy.md: %w", err)
	}

	s, err := storage.ParseStrategy(content)
	if err != nil {
		return nil, ListNotesOutput{}, fmt.Errorf("parsing strategy: %w", err)
	}

	notes := s.Notes
	search := strings.ToLower(strings.TrimSpace(input.Search))
	if search != "" {
		var filtered []string
		for _, note := range notes {
			if strings.Contains(strings.ToLower(note), search) {
				filtered = append(filtered, note)
			}
		}
		notes = filtered
	}

	result := ListNotesResult{
		Notes: notes,
		Total: len(s.Notes),
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, ListNotesOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, ListNotesOutput{
		Success: true,
		Message: string(jsonBytes),
	}, nil
}

func (t *StrategyTools) getMilestones(ctx context.Context, req *mcp.CallToolRequest, input GetMilestonesInput) (*mcp.CallToolResult, GetMilestonesOutput, error) {
	content, _, err := t.storage.ReadFile(ctx, "strategy.md")
	if err != nil {
		return nil, GetMilestonesOutput{}, fmt.Errorf("reading strategy.md: %w", err)
	}

	s, err := storage.ParseStrategy(content)
	if err != nil {
		return nil, GetMilestonesOutput{}, fmt.Errorf("parsing strategy: %w", err)
	}

	active := make([]MilestoneItem, len(s.ActiveMilestones))
	for i, m := range s.ActiveMilestones {
		active[i] = milestoneToItem(m)
	}

	completed := make([]MilestoneItem, len(s.CompletedMilestones))
	for i, m := range s.CompletedMilestones {
		completed[i] = milestoneToItem(m)
	}

	result := GetMilestonesResult{
		CurrentPhase:        s.CurrentPhase,
		ActiveMilestones:    active,
		CompletedMilestones: completed,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, GetMilestonesOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, GetMilestonesOutput{
		Success: true,
		Message: string(jsonBytes),
	}, nil
}
