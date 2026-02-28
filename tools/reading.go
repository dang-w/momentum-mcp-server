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

// ReadingTools provides tools for managing the reading list.
type ReadingTools struct {
	storage storage.Storage
}

// NewReadingTools creates a new ReadingTools instance.
func NewReadingTools(s storage.Storage) *ReadingTools {
	return &ReadingTools{storage: s}
}

// AddToReadingListInput is the input schema for the add_to_reading_list tool.
type AddToReadingListInput struct {
	URL   string `json:"url" jsonschema:"The URL of the article to add"`
	Notes string `json:"notes,omitempty" jsonschema:"Optional notes about why this is interesting"`
}

// AddToReadingListOutput is the output for the add_to_reading_list tool.
type AddToReadingListOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// MarkReadInput is the input schema for the mark_read tool.
type MarkReadInput struct {
	URL   string `json:"url" jsonschema:"URL or partial URL to match against reading list items"`
	Notes string `json:"notes,omitempty" jsonschema:"Optional notes about the article (will replace existing notes)"`
}

// MarkReadOutput is the output for the mark_read tool.
type MarkReadOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListReadingListInput is the input schema for the list_reading_list tool.
type ListReadingListInput struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by status: unread, read, or all. Defaults to all."`
}

// ListReadingListOutput is the output for the list_reading_list tool.
type ListReadingListOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListReadingListResult is the response payload for list_reading_list.
type ListReadingListResult struct {
	Items       []ReadingListItem `json:"items"`
	TotalUnread int               `json:"total_unread"`
	TotalRead   int               `json:"total_read"`
}

// Register registers reading list tools with the MCP server.
func (t *ReadingTools) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_to_reading_list",
		Description: "Add a URL to the reading list",
	}, t.addToReadingList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_read",
		Description: "Mark a reading list item as read",
	}, t.markRead)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_reading_list",
		Description: "List reading list items with optional filtering by read status",
	}, t.listReadingList)
}

func (t *ReadingTools) addToReadingList(ctx context.Context, req *mcp.CallToolRequest, input AddToReadingListInput) (*mcp.CallToolResult, AddToReadingListOutput, error) {
	if strings.TrimSpace(input.URL) == "" {
		return nil, AddToReadingListOutput{
			Success: false,
			Message: "URL cannot be empty",
		}, nil
	}

	// Read current reading list
	content, sha, err := t.storage.ReadFile(ctx, "reading-list.md")
	if err != nil {
		return nil, AddToReadingListOutput{}, fmt.Errorf("reading reading-list.md: %w", err)
	}

	rl, err := storage.ParseReadingList(content)
	if err != nil {
		return nil, AddToReadingListOutput{}, fmt.Errorf("parsing reading list: %w", err)
	}

	// Check for duplicates
	url := strings.TrimSpace(input.URL)
	for _, item := range rl.ToRead {
		if item.URL == url {
			return nil, AddToReadingListOutput{
				Success: false,
				Message: fmt.Sprintf("URL already in reading list: %s", url),
			}, nil
		}
	}
	for _, item := range rl.Read {
		if item.URL == url {
			return nil, AddToReadingListOutput{
				Success: false,
				Message: fmt.Sprintf("URL already marked as read: %s", url),
			}, nil
		}
	}

	// Add the new item
	newItem := storage.ReadingItem{
		URL:   url,
		Notes: strings.TrimSpace(input.Notes),
		Added: time.Now().UTC().Truncate(24 * time.Hour),
	}
	rl.ToRead = append(rl.ToRead, newItem)

	// Serialize and write back
	newContent := storage.SerializeReadingList(rl)
	if err := t.storage.WriteFile(ctx, "reading-list.md", newContent, sha, "Add to reading list"); err != nil {
		if err == storage.ErrConflict {
			return nil, AddToReadingListOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, AddToReadingListOutput{}, fmt.Errorf("writing reading-list.md: %w", err)
	}

	return nil, AddToReadingListOutput{
		Success: true,
		Message: fmt.Sprintf("Added to reading list: %s", url),
	}, nil
}

func (t *ReadingTools) markRead(ctx context.Context, req *mcp.CallToolRequest, input MarkReadInput) (*mcp.CallToolResult, MarkReadOutput, error) {
	if strings.TrimSpace(input.URL) == "" {
		return nil, MarkReadOutput{
			Success: false,
			Message: "URL cannot be empty",
		}, nil
	}

	// Read current reading list
	content, sha, err := t.storage.ReadFile(ctx, "reading-list.md")
	if err != nil {
		return nil, MarkReadOutput{}, fmt.Errorf("reading reading-list.md: %w", err)
	}

	rl, err := storage.ParseReadingList(content)
	if err != nil {
		return nil, MarkReadOutput{}, fmt.Errorf("parsing reading list: %w", err)
	}

	// Find matching items
	searchText := strings.ToLower(strings.TrimSpace(input.URL))
	var matches []int
	for i, item := range rl.ToRead {
		if strings.Contains(strings.ToLower(item.URL), searchText) {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return nil, MarkReadOutput{
			Success: false,
			Message: fmt.Sprintf("No unread item found matching %q", input.URL),
		}, nil
	}

	if len(matches) > 1 {
		var matchURLs []string
		for _, idx := range matches {
			matchURLs = append(matchURLs, fmt.Sprintf("- %s", rl.ToRead[idx].URL))
		}
		return nil, MarkReadOutput{
			Success: false,
			Message: fmt.Sprintf("Multiple items match %q. Please be more specific:\n%s", input.URL, strings.Join(matchURLs, "\n")),
		}, nil
	}

	// Mark as read
	idx := matches[0]
	item := rl.ToRead[idx]
	item.Read = true
	now := time.Now().UTC().Truncate(24 * time.Hour)
	item.ReadAt = &now
	if input.Notes != "" {
		item.Notes = strings.TrimSpace(input.Notes)
	}

	// Move from to-read to read
	rl.ToRead = append(rl.ToRead[:idx], rl.ToRead[idx+1:]...)
	rl.Read = append([]storage.ReadingItem{item}, rl.Read...) // Add to front

	// Serialize and write back
	newContent := storage.SerializeReadingList(rl)
	if err := t.storage.WriteFile(ctx, "reading-list.md", newContent, sha, "Mark as read"); err != nil {
		if err == storage.ErrConflict {
			return nil, MarkReadOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, MarkReadOutput{}, fmt.Errorf("writing reading-list.md: %w", err)
	}

	return nil, MarkReadOutput{
		Success: true,
		Message: fmt.Sprintf("Marked as read: %s", item.URL),
	}, nil
}

func (t *ReadingTools) listReadingList(ctx context.Context, req *mcp.CallToolRequest, input ListReadingListInput) (*mcp.CallToolResult, ListReadingListOutput, error) {
	content, _, err := t.storage.ReadFile(ctx, "reading-list.md")
	if err != nil {
		return nil, ListReadingListOutput{}, fmt.Errorf("reading reading-list.md: %w", err)
	}

	rl, err := storage.ParseReadingList(content)
	if err != nil {
		return nil, ListReadingListOutput{}, fmt.Errorf("parsing reading list: %w", err)
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "all"
	}

	var items []storage.ReadingItem
	switch status {
	case "unread":
		items = rl.ToRead
	case "read":
		items = rl.Read
	case "all":
		items = append(items, rl.ToRead...)
		items = append(items, rl.Read...)
	default:
		return nil, ListReadingListOutput{
			Success: false,
			Message: fmt.Sprintf("Invalid status %q. Use: unread, read, or all", input.Status),
		}, nil
	}

	readingItems := make([]ReadingListItem, len(items))
	for i, item := range items {
		readingItems[i] = readingToItem(item)
	}

	result := ListReadingListResult{
		Items:       readingItems,
		TotalUnread: len(rl.ToRead),
		TotalRead:   len(rl.Read),
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, ListReadingListOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, ListReadingListOutput{
		Success: true,
		Message: string(jsonBytes),
	}, nil
}
