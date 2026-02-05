package tools

import (
	"context"
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
