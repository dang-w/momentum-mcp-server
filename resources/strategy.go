package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StrategyResource provides read access to the strategy progress.
type StrategyResource struct {
	storage storage.Storage
}

// NewStrategyResource creates a new StrategyResource.
func NewStrategyResource(s storage.Storage) *StrategyResource {
	return &StrategyResource{storage: s}
}

// Register registers the momentum://strategy resource with the MCP server.
func (r *StrategyResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://strategy",
		Name:        "Strategy Progress",
		Description: "Current phase, active milestones, and recent completions",
		MIMEType:    "text/markdown",
	}, r.Read)
}

// Read fetches and formats the strategy progress.
func (r *StrategyResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	content, _, err := r.storage.ReadFile(ctx, "strategy.md")
	if err != nil {
		return nil, fmt.Errorf("reading strategy.md: %w", err)
	}

	s, err := storage.ParseStrategy(content)
	if err != nil {
		return nil, fmt.Errorf("parsing strategy: %w", err)
	}

	// Build readable markdown output
	var b strings.Builder
	b.WriteString("# Strategy Progress\n\n")

	// Current phase
	b.WriteString(fmt.Sprintf("**Current Phase:** %s\n\n", s.CurrentPhase))

	// Summary
	b.WriteString(fmt.Sprintf("**%d active milestones**, **%d completed**\n\n", len(s.ActiveMilestones), len(s.CompletedMilestones)))

	// Active milestones
	if len(s.ActiveMilestones) > 0 {
		b.WriteString("## 🎯 Active Milestones\n")
		for _, m := range s.ActiveMilestones {
			line := fmt.Sprintf("- [ ] %s", m.Text)
			if m.Due != nil {
				line += fmt.Sprintf(" — Due: %s", m.Due.Format("2006-01-02"))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	// Recent completions (last 5)
	if len(s.CompletedMilestones) > 0 {
		b.WriteString("## ✅ Recently Completed\n")
		limit := 5
		if len(s.CompletedMilestones) < limit {
			limit = len(s.CompletedMilestones)
		}
		for i := 0; i < limit; i++ {
			m := s.CompletedMilestones[i]
			b.WriteString(fmt.Sprintf("- [x] %s\n", m.Text))
		}
		b.WriteString("\n")
	}

	// Notes
	if len(s.Notes) > 0 {
		b.WriteString("## 📝 Notes\n")
		for _, note := range s.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", note))
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://strategy",
				MIMEType: "text/markdown",
				Text:     b.String(),
			},
		},
	}, nil
}
