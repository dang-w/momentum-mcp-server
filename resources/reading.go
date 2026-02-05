package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReadingResource provides read access to the reading list.
type ReadingResource struct {
	storage storage.Storage
}

// NewReadingResource creates a new ReadingResource.
func NewReadingResource(s storage.Storage) *ReadingResource {
	return &ReadingResource{storage: s}
}

// Register registers the momentum://reading-list resource with the MCP server.
func (r *ReadingResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://reading-list",
		Name:        "Reading List",
		Description: "Articles to read and those already read",
		MIMEType:    "text/markdown",
	}, r.Read)
}

// Read fetches and formats the reading list.
func (r *ReadingResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	content, _, err := r.storage.ReadFile(ctx, "reading-list.md")
	if err != nil {
		return nil, fmt.Errorf("reading reading-list.md: %w", err)
	}

	rl, err := storage.ParseReadingList(content)
	if err != nil {
		return nil, fmt.Errorf("parsing reading list: %w", err)
	}

	// Build readable markdown output
	var b strings.Builder
	b.WriteString("# Reading List\n\n")

	// Summary
	b.WriteString(fmt.Sprintf("**%d unread**, **%d read** total\n\n", len(rl.ToRead), len(rl.Read)))

	// To read section
	if len(rl.ToRead) > 0 {
		b.WriteString("## 📚 To Read\n")
		for _, item := range rl.ToRead {
			b.WriteString(fmt.Sprintf("- [ ] %s", item.URL))
			if item.Notes != "" {
				b.WriteString(fmt.Sprintf("\n  - Notes: %s", item.Notes))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Recently read (last 5)
	if len(rl.Read) > 0 {
		b.WriteString("## ✅ Recently Read\n")
		limit := 5
		if len(rl.Read) < limit {
			limit = len(rl.Read)
		}
		for i := 0; i < limit; i++ {
			item := rl.Read[i]
			b.WriteString(fmt.Sprintf("- [x] %s", item.URL))
			if item.Notes != "" {
				b.WriteString(fmt.Sprintf("\n  - Notes: %s", item.Notes))
			}
			b.WriteString("\n")
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://reading-list",
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		},
	}, nil
}
