//go:build integration
// +build integration

package resources

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dang-w/momentum-mcp-server/storage"
)

func TestSummaryResource_Integration(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	repo := os.Getenv("GITHUB_REPO")
	if token == "" || repo == "" {
		t.Skip("GITHUB_TOKEN or GITHUB_REPO not set, skipping integration test")
	}

	// Extract username from repo
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) < 1 {
		t.Fatalf("Invalid GITHUB_REPO format: %s", repo)
	}
	username := parts[0]

	// Create storage
	store, err := storage.NewGitHubStorage(token, repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create GitHub activity resource
	githubActivity := NewGitHubActivityResource(token, username)

	// Create summary resource
	resource := NewSummaryResource(store, githubActivity)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Read the summary
	result, err := resource.Read(ctx, nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("Expected at least one content item")
	}

	content := result.Contents[0].Text
	t.Logf("Weekly Summary:\n%s", content)

	// Verify structure
	if !strings.Contains(content, "Weekly Summary") {
		t.Error("Expected 'Weekly Summary' header")
	}
	if !strings.Contains(content, "Momentum") {
		t.Error("Expected 'Momentum' section")
	}
	if !strings.Contains(content, "Focus Areas") {
		t.Error("Expected 'Focus Areas' section")
	}
	if !strings.Contains(content, "Reading Queue") {
		t.Error("Expected 'Reading Queue' section")
	}
	if !strings.Contains(content, "Recent Completions") {
		t.Error("Expected 'Recent Completions' section")
	}
}
