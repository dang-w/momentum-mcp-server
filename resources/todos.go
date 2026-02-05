// Package resources provides MCP resource handlers for reading productivity data.
package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TodosResource provides read access to the todos list.
type TodosResource struct {
	storage storage.Storage
}

// NewTodosResource creates a new TodosResource.
func NewTodosResource(s storage.Storage) *TodosResource {
	return &TodosResource{storage: s}
}

// Register registers the momentum://todos resource with the MCP server.
func (r *TodosResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://todos",
		Name:        "Current Todos",
		Description: "Active and completed todos from momentum-data",
		MIMEType:    "text/markdown",
	}, r.Read)
}

// Read fetches and formats the todos list.
func (r *TodosResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	content, _, err := r.storage.ReadFile(ctx, "todos.md")
	if err != nil {
		return nil, fmt.Errorf("reading todos.md: %w", err)
	}

	tf, err := storage.ParseTodos(content)
	if err != nil {
		return nil, fmt.Errorf("parsing todos: %w", err)
	}

	// Count by priority
	highCount := 0
	normalCount := 0
	somedayCount := 0
	for _, todo := range tf.Active {
		switch todo.Priority {
		case storage.PriorityHigh:
			highCount++
		case storage.PriorityNormal:
			normalCount++
		case storage.PrioritySomeday:
			somedayCount++
		}
	}

	// Build readable markdown output
	var b strings.Builder
	b.WriteString("# Current Todos\n\n")

	// Summary line
	b.WriteString(fmt.Sprintf("**%d active** (", len(tf.Active)))
	parts := []string{}
	if highCount > 0 {
		parts = append(parts, fmt.Sprintf("%d high priority", highCount))
	}
	if normalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d normal", normalCount))
	}
	if somedayCount > 0 {
		parts = append(parts, fmt.Sprintf("%d someday", somedayCount))
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(fmt.Sprintf("), **%d completed**\n\n", len(tf.Completed)))

	// High priority section
	if highCount > 0 {
		b.WriteString("## 🔴 High Priority\n")
		for _, todo := range tf.Active {
			if todo.Priority == storage.PriorityHigh {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", todo.Text))
			}
		}
		b.WriteString("\n")
	}

	// Normal priority section
	if normalCount > 0 {
		b.WriteString("## Normal\n")
		for _, todo := range tf.Active {
			if todo.Priority == storage.PriorityNormal {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", todo.Text))
			}
		}
		b.WriteString("\n")
	}

	// Someday section
	if somedayCount > 0 {
		b.WriteString("## 💭 Someday\n")
		for _, todo := range tf.Active {
			if todo.Priority == storage.PrioritySomeday {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", todo.Text))
			}
		}
		b.WriteString("\n")
	}

	// Recent completions (last 5)
	if len(tf.Completed) > 0 {
		b.WriteString("## ✅ Recently Completed\n")
		limit := 5
		if len(tf.Completed) < limit {
			limit = len(tf.Completed)
		}
		for i := 0; i < limit; i++ {
			todo := tf.Completed[i]
			b.WriteString(fmt.Sprintf("- [x] %s\n", todo.Text))
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://todos",
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		},
	}, nil
}
