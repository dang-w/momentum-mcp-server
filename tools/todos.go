// Package tools provides MCP tool handlers for modifying productivity data.
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

// TodoTools provides tools for managing todos.
type TodoTools struct {
	storage storage.Storage
}

// NewTodoTools creates a new TodoTools instance.
func NewTodoTools(s storage.Storage) *TodoTools {
	return &TodoTools{storage: s}
}

// AddTodoInput is the input schema for the add_todo tool.
type AddTodoInput struct {
	Text     string `json:"text" jsonschema:"The todo item text"`
	Priority string `json:"priority,omitempty" jsonschema:"Priority level: high, normal, or someday. Defaults to normal."`
}

// AddTodoOutput is the output for the add_todo tool.
type AddTodoOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CompleteTodoInput is the input schema for the complete_todo tool.
type CompleteTodoInput struct {
	Text string `json:"text,omitempty" jsonschema:"Text to match against todo items. Can be partial match."`
	ID   string `json:"id,omitempty" jsonschema:"ID of the todo to complete. More reliable than text matching. Use list_todos to find IDs."`
}

// CompleteTodoOutput is the output for the complete_todo tool.
type CompleteTodoOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListTodosInput is the input schema for the list_todos tool.
type ListTodosInput struct {
	Status   string `json:"status,omitempty" jsonschema:"Filter by status: active, completed, or all. Defaults to active."`
	Priority string `json:"priority,omitempty" jsonschema:"Filter by priority: high, normal, or someday. No filter if omitted."`
}

// ListTodosOutput is the output for the list_todos tool.
type ListTodosOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListTodosResult is the response payload for list_todos.
type ListTodosResult struct {
	Todos          []TodoItem `json:"todos"`
	TotalActive    int        `json:"total_active"`
	TotalCompleted int        `json:"total_completed"`
}

// EditTodoInput is the input schema for the edit_todo tool.
type EditTodoInput struct {
	ID       string `json:"id" jsonschema:"ID of the todo to edit. Use list_todos to find IDs."`
	Text     string `json:"text,omitempty" jsonschema:"New todo text. If omitted, keeps existing text."`
	Priority string `json:"priority,omitempty" jsonschema:"New priority level: high, normal, or someday. If omitted, keeps existing priority."`
}

// EditTodoOutput is the output for the edit_todo tool.
type EditTodoOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Register registers todo tools with the MCP server.
func (t *TodoTools) Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_todo",
		Description: "Add a new todo item to the list",
	}, t.addTodo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "complete_todo",
		Description: "Mark a todo item as completed",
	}, t.completeTodo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_todos",
		Description: "List todo items with optional filtering by status and priority",
	}, t.listTodos)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_todo",
		Description: "Edit a todo item's text or priority",
	}, t.editTodo)
}

func (t *TodoTools) addTodo(ctx context.Context, req *mcp.CallToolRequest, input AddTodoInput) (*mcp.CallToolResult, AddTodoOutput, error) {
	if strings.TrimSpace(input.Text) == "" {
		return nil, AddTodoOutput{
			Success: false,
			Message: "Todo text cannot be empty",
		}, nil
	}

	// Read current todos
	content, sha, err := t.storage.ReadFile(ctx, "todos.md")
	if err != nil {
		return nil, AddTodoOutput{}, fmt.Errorf("reading todos.md: %w", err)
	}

	tf, err := storage.ParseTodos(content)
	if err != nil {
		return nil, AddTodoOutput{}, fmt.Errorf("parsing todos: %w", err)
	}

	// Determine priority
	priority := storage.PriorityNormal
	switch strings.ToLower(input.Priority) {
	case "high":
		priority = storage.PriorityHigh
	case "someday":
		priority = storage.PrioritySomeday
	case "normal", "":
		priority = storage.PriorityNormal
	default:
		return nil, AddTodoOutput{
			Success: false,
			Message: fmt.Sprintf("Invalid priority %q. Use: high, normal, or someday", input.Priority),
		}, nil
	}

	// Add the new todo
	newTodo := storage.Todo{
		ID:       storage.GenerateID(),
		Text:     strings.TrimSpace(input.Text),
		Priority: priority,
		Added:    time.Now().UTC().Truncate(24 * time.Hour),
	}
	tf.Active = append(tf.Active, newTodo)

	// Serialize and write back
	newContent := storage.SerializeTodos(tf)
	if err := t.storage.WriteFile(ctx, "todos.md", newContent, sha, fmt.Sprintf("Add todo: %s", truncate(input.Text, 50))); err != nil {
		if err == storage.ErrConflict {
			return nil, AddTodoOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, AddTodoOutput{}, fmt.Errorf("writing todos.md: %w", err)
	}

	itemJSON, err := json.Marshal(todoToItem(newTodo))
	if err != nil {
		return nil, AddTodoOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, AddTodoOutput{
		Success: true,
		Message: string(itemJSON),
	}, nil
}

func (t *TodoTools) completeTodo(ctx context.Context, req *mcp.CallToolRequest, input CompleteTodoInput) (*mcp.CallToolResult, CompleteTodoOutput, error) {
	if strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.ID) == "" {
		return nil, CompleteTodoOutput{
			Success: false,
			Message: "Either text or id must be provided",
		}, nil
	}

	// Read current todos
	content, sha, err := t.storage.ReadFile(ctx, "todos.md")
	if err != nil {
		return nil, CompleteTodoOutput{}, fmt.Errorf("reading todos.md: %w", err)
	}

	tf, err := storage.ParseTodos(content)
	if err != nil {
		return nil, CompleteTodoOutput{}, fmt.Errorf("parsing todos: %w", err)
	}

	// Find matching todos — prefer ID match if provided
	var matches []int
	if id := strings.TrimSpace(input.ID); id != "" {
		for i, todo := range tf.Active {
			if todo.ID == id {
				matches = append(matches, i)
				break
			}
		}
		if len(matches) == 0 {
			return nil, CompleteTodoOutput{
				Success: false,
				Message: fmt.Sprintf("No active todo found with id %q", input.ID),
			}, nil
		}
	} else {
		searchText := strings.ToLower(strings.TrimSpace(input.Text))
		for i, todo := range tf.Active {
			if strings.Contains(strings.ToLower(todo.Text), searchText) {
				matches = append(matches, i)
			}
		}

		if len(matches) == 0 {
			return nil, CompleteTodoOutput{
				Success: false,
				Message: fmt.Sprintf("No active todo found matching %q", input.Text),
			}, nil
		}

		if len(matches) > 1 {
			var matchTexts []string
			for _, idx := range matches {
				matchTexts = append(matchTexts, fmt.Sprintf("- [%s] %s", tf.Active[idx].ID, tf.Active[idx].Text))
			}
			return nil, CompleteTodoOutput{
				Success: false,
				Message: fmt.Sprintf("Multiple todos match %q. Please be more specific or use an id:\n%s", input.Text, strings.Join(matchTexts, "\n")),
			}, nil
		}
	}

	// Mark as completed
	idx := matches[0]
	todo := tf.Active[idx]
	todo.Completed = true
	now := time.Now().UTC().Truncate(24 * time.Hour)
	todo.CompletedAt = &now

	// Move from active to completed
	tf.Active = append(tf.Active[:idx], tf.Active[idx+1:]...)
	tf.Completed = append([]storage.Todo{todo}, tf.Completed...) // Add to front

	// Serialize and write back
	newContent := storage.SerializeTodos(tf)
	if err := t.storage.WriteFile(ctx, "todos.md", newContent, sha, fmt.Sprintf("Complete todo: %s", truncate(todo.Text, 50))); err != nil {
		if err == storage.ErrConflict {
			return nil, CompleteTodoOutput{
				Success: false,
				Message: "File was modified by another process. Please try again.",
			}, nil
		}
		return nil, CompleteTodoOutput{}, fmt.Errorf("writing todos.md: %w", err)
	}

	itemJSON, err := json.Marshal(todoToItem(todo))
	if err != nil {
		return nil, CompleteTodoOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, CompleteTodoOutput{
		Success: true,
		Message: string(itemJSON),
	}, nil
}

func (t *TodoTools) listTodos(ctx context.Context, req *mcp.CallToolRequest, input ListTodosInput) (*mcp.CallToolResult, ListTodosOutput, error) {
	content, _, err := t.storage.ReadFile(ctx, "todos.md")
	if err != nil {
		return nil, ListTodosOutput{}, fmt.Errorf("reading todos.md: %w", err)
	}

	tf, err := storage.ParseTodos(content)
	if err != nil {
		return nil, ListTodosOutput{}, fmt.Errorf("parsing todos: %w", err)
	}

	// Determine which items to include based on status filter
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "active"
	}

	var items []storage.Todo
	switch status {
	case "active":
		items = tf.Active
	case "completed":
		items = tf.Completed
	case "all":
		items = append(items, tf.Active...)
		items = append(items, tf.Completed...)
	default:
		return nil, ListTodosOutput{
			Success: false,
			Message: fmt.Sprintf("Invalid status %q. Use: active, completed, or all", input.Status),
		}, nil
	}

	// Filter by priority if specified
	priority := strings.ToLower(strings.TrimSpace(input.Priority))
	if priority != "" {
		var p storage.Priority
		switch priority {
		case "high":
			p = storage.PriorityHigh
		case "normal":
			p = storage.PriorityNormal
		case "someday":
			p = storage.PrioritySomeday
		default:
			return nil, ListTodosOutput{
				Success: false,
				Message: fmt.Sprintf("Invalid priority %q. Use: high, normal, or someday", input.Priority),
			}, nil
		}

		var filtered []storage.Todo
		for _, todo := range items {
			if todo.Priority == p {
				filtered = append(filtered, todo)
			}
		}
		items = filtered
	}

	// Convert to response items
	todoItems := make([]TodoItem, len(items))
	for i, todo := range items {
		todoItems[i] = todoToItem(todo)
	}

	result := ListTodosResult{
		Todos:          todoItems,
		TotalActive:    len(tf.Active),
		TotalCompleted: len(tf.Completed),
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, ListTodosOutput{}, fmt.Errorf("marshaling response: %w", err)
	}

	return nil, ListTodosOutput{
		Success: true,
		Message: string(jsonBytes),
	}, nil
}

func (t *TodoTools) editTodo(ctx context.Context, req *mcp.CallToolRequest, input EditTodoInput) (*mcp.CallToolResult, EditTodoOutput, error) {
	if strings.TrimSpace(input.ID) == "" {
		return nil, EditTodoOutput{
			Success: false,
			Message: "id is required",
		}, nil
	}

	if strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.Priority) == "" {
		return nil, EditTodoOutput{
			Success: false,
			Message: "At least one of text or priority must be provided",
		}, nil
	}

	// Validate priority if provided
	var newPriority storage.Priority
	if p := strings.ToLower(strings.TrimSpace(input.Priority)); p != "" {
		switch p {
		case "high":
			newPriority = storage.PriorityHigh
		case "normal":
			newPriority = storage.PriorityNormal
		case "someday":
			newPriority = storage.PrioritySomeday
		default:
			return nil, EditTodoOutput{
				Success: false,
				Message: fmt.Sprintf("Invalid priority %q. Use: high, normal, or someday", input.Priority),
			}, nil
		}
	}

	// Read current todos
	content, sha, err := t.storage.ReadFile(ctx, "todos.md")
	if err != nil {
		return nil, EditTodoOutput{}, fmt.Errorf("reading todos.md: %w", err)
	}

	tf, err := storage.ParseTodos(content)
	if err != nil {
		return nil, EditTodoOutput{}, fmt.Errorf("parsing todos: %w", err)
	}

	// Find the todo by ID in active list
	id := strings.TrimSpace(input.ID)
	found := false
	for i, todo := range tf.Active {
		if todo.ID == id {
			if text := strings.TrimSpace(input.Text); text != "" {
				tf.Active[i].Text = text
			}
			if newPriority != "" {
				tf.Active[i].Priority = newPriority
			}
			found = true

			// Serialize and write back
			newContent := storage.SerializeTodos(tf)
			if err := t.storage.WriteFile(ctx, "todos.md", newContent, sha, fmt.Sprintf("Edit todo: %s", truncate(tf.Active[i].Text, 50))); err != nil {
				if err == storage.ErrConflict {
					return nil, EditTodoOutput{
						Success: false,
						Message: "File was modified by another process. Please try again.",
					}, nil
				}
				return nil, EditTodoOutput{}, fmt.Errorf("writing todos.md: %w", err)
			}

			itemJSON, err := json.Marshal(todoToItem(tf.Active[i]))
			if err != nil {
				return nil, EditTodoOutput{}, fmt.Errorf("marshaling response: %w", err)
			}

			return nil, EditTodoOutput{
				Success: true,
				Message: string(itemJSON),
			}, nil
		}
	}

	if !found {
		return nil, EditTodoOutput{
			Success: false,
			Message: fmt.Sprintf("No active todo found with id %q", id),
		}, nil
	}

	return nil, EditTodoOutput{}, nil // unreachable
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
