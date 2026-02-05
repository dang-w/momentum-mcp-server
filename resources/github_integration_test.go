//go:build integration
// +build integration

package resources

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGitHubActivityResource_Integration(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set, skipping integration test")
	}

	username := "dang-w"
	resource := NewGitHubActivityResource(token, username)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	activity, err := resource.getActivity(ctx)
	if err != nil {
		t.Fatalf("getActivity failed: %v", err)
	}

	t.Logf("GitHub Activity for %s:", username)
	t.Logf("  Commits this week: %d", activity.CommitsThisWeek)
	t.Logf("  Repos active: %d", activity.ReposActive)
	t.Logf("  Streak days: %d", activity.StreakDays)
	t.Logf("  Last commit: %v", activity.LastCommit)
	t.Logf("  Public repos: %v", activity.PublicRepos)
	t.Logf("  Private repos count: %d", activity.PrivateReposCount)

	// Verify we got some data
	if len(activity.PublicRepos) == 0 && activity.PrivateReposCount == 0 {
		t.Error("Expected at least some repos")
	}
}
