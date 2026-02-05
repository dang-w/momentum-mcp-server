package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GitHubActivityResource provides read access to GitHub activity data.
type GitHubActivityResource struct {
	token    string
	username string
	client   *http.Client

	// Cache
	mu          sync.RWMutex
	cachedData  *GitHubActivity
	cachedAt    time.Time
	cacheTTL    time.Duration
}

// GitHubActivity represents the GitHub activity data returned by this resource.
type GitHubActivity struct {
	CommitsThisWeek   int       `json:"commits_this_week"`
	ReposActive       int       `json:"repos_active"`
	StreakDays        int       `json:"streak_days"`
	LastCommit        time.Time `json:"last_commit"`
	PublicRepos       []string  `json:"public_repos"`
	PrivateReposCount int       `json:"private_repos_count"`
}

// NewGitHubActivityResource creates a new GitHubActivityResource.
// username should be the GitHub username to fetch activity for.
func NewGitHubActivityResource(token, username string) *GitHubActivityResource {
	return &GitHubActivityResource{
		token:    token,
		username: username,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cacheTTL: 15 * time.Minute,
	}
}

// Register registers the momentum://github-activity resource with the MCP server.
func (r *GitHubActivityResource) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "momentum://github-activity",
		Name:        "GitHub Activity",
		Description: "Recent GitHub contribution activity including commits, streaks, and active repos",
		MIMEType:    "application/json",
	}, r.Read)
}

// Read fetches and formats the GitHub activity data.
func (r *GitHubActivityResource) Read(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	activity, err := r.getActivity(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching GitHub activity: %w", err)
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(activity, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serializing activity: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "momentum://github-activity",
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

// getActivity returns cached data if fresh, otherwise fetches from GitHub.
func (r *GitHubActivityResource) getActivity(ctx context.Context) (*GitHubActivity, error) {
	// Check cache first
	r.mu.RLock()
	if r.cachedData != nil && time.Since(r.cachedAt) < r.cacheTTL {
		cached := r.cachedData
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	// Fetch fresh data
	activity, err := r.fetchActivity(ctx)
	if err != nil {
		// If fetch fails but we have stale data, return it
		r.mu.RLock()
		if r.cachedData != nil {
			cached := r.cachedData
			r.mu.RUnlock()
			return cached, nil
		}
		r.mu.RUnlock()
		return nil, err
	}

	// Update cache
	r.mu.Lock()
	r.cachedData = activity
	r.cachedAt = time.Now()
	r.mu.Unlock()

	return activity, nil
}

// graphQLRequest is the request body for GitHub GraphQL API.
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// graphQLResponse is the response from GitHub GraphQL API.
type graphQLResponse struct {
	Data   *graphQLData   `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLData struct {
	User *graphQLUser `json:"user"`
}

type graphQLUser struct {
	ContributionsCollection *contributionsCollection `json:"contributionsCollection"`
	Repositories            *repositoriesConnection  `json:"repositories"`
}

type contributionsCollection struct {
	ContributionCalendar *contributionCalendar `json:"contributionCalendar"`
}

type contributionCalendar struct {
	TotalContributions int                    `json:"totalContributions"`
	Weeks              []contributionWeek     `json:"weeks"`
}

type contributionWeek struct {
	ContributionDays []contributionDay `json:"contributionDays"`
}

type contributionDay struct {
	ContributionCount int    `json:"contributionCount"`
	Date              string `json:"date"`
}

type repositoriesConnection struct {
	Nodes []repositoryNode `json:"nodes"`
}

type repositoryNode struct {
	Name            string    `json:"name"`
	IsPrivate       bool      `json:"isPrivate"`
	PushedAt        string    `json:"pushedAt"`
}

// fetchActivity fetches contribution data from GitHub GraphQL API.
func (r *GitHubActivityResource) fetchActivity(ctx context.Context) (*GitHubActivity, error) {
	query := `
query($username: String!) {
  user(login: $username) {
    contributionsCollection {
      contributionCalendar {
        totalContributions
        weeks {
          contributionDays {
            contributionCount
            date
          }
        }
      }
    }
    repositories(first: 100, orderBy: {field: PUSHED_AT, direction: DESC}, ownerAffiliations: OWNER) {
      nodes {
        name
        isPrivate
        pushedAt
      }
    }
  }
}
`

	reqBody := graphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"username": r.username,
		},
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encoding GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		var errMsgs []string
		for _, e := range gqlResp.Errors {
			errMsgs = append(errMsgs, e.Message)
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(errMsgs, "; "))
	}

	if gqlResp.Data == nil || gqlResp.Data.User == nil {
		return nil, fmt.Errorf("user %q not found", r.username)
	}

	return r.parseActivity(gqlResp.Data.User)
}

// parseActivity converts the GraphQL response into GitHubActivity.
func (r *GitHubActivityResource) parseActivity(user *graphQLUser) (*GitHubActivity, error) {
	activity := &GitHubActivity{
		PublicRepos: []string{},
	}

	// Parse contribution calendar
	if user.ContributionsCollection != nil && user.ContributionsCollection.ContributionCalendar != nil {
		calendar := user.ContributionsCollection.ContributionCalendar

		// Flatten all days
		var allDays []contributionDay
		for _, week := range calendar.Weeks {
			allDays = append(allDays, week.ContributionDays...)
		}

		// Calculate commits this week (Monday-Sunday of current week)
		now := time.Now()
		weekStart := startOfWeek(now)
		weekEnd := weekStart.AddDate(0, 0, 7)

		for _, day := range allDays {
			date, err := time.Parse("2006-01-02", day.Date)
			if err != nil {
				continue
			}
			if !date.Before(weekStart) && date.Before(weekEnd) {
				activity.CommitsThisWeek += day.ContributionCount
			}
		}

		// Calculate streak (consecutive days with contributions ending today or yesterday)
		activity.StreakDays = calculateStreak(allDays)
	}

	// Parse repositories
	if user.Repositories != nil {
		// Track repos with recent activity (pushed in last 7 days)
		now := time.Now()
		oneWeekAgo := now.AddDate(0, 0, -7)
		var lastCommitTime time.Time

		for _, repo := range user.Repositories.Nodes {
			pushedAt, err := time.Parse(time.RFC3339, repo.PushedAt)
			if err != nil {
				continue
			}

			// Track most recent push
			if pushedAt.After(lastCommitTime) {
				lastCommitTime = pushedAt
			}

			// Count repos active this week
			if pushedAt.After(oneWeekAgo) {
				activity.ReposActive++
			}

			// Categorize by visibility
			if repo.IsPrivate {
				activity.PrivateReposCount++
			} else {
				activity.PublicRepos = append(activity.PublicRepos, repo.Name)
			}
		}

		activity.LastCommit = lastCommitTime
	}

	return activity, nil
}

// startOfWeek returns the Monday 00:00:00 of the week containing t.
func startOfWeek(t time.Time) time.Time {
	// Go's Weekday: Sunday=0, Monday=1, ..., Saturday=6
	// We want Monday as start of week
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday becomes 7
	}
	daysFromMonday := weekday - 1
	monday := t.AddDate(0, 0, -daysFromMonday)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location())
}

// calculateStreak calculates the current contribution streak.
// A streak is consecutive days with at least 1 contribution, ending on today or yesterday.
func calculateStreak(days []contributionDay) int {
	if len(days) == 0 {
		return 0
	}

	// Sort days by date descending (most recent first)
	sorted := make([]contributionDay, len(days))
	copy(sorted, days)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date > sorted[j].Date
	})

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	streak := 0
	expectDate := today

	for _, day := range sorted {
		// If we haven't found the streak start yet
		if streak == 0 {
			// Streak must start today or yesterday
			if day.Date != today && day.Date != yesterday {
				if day.Date < yesterday {
					// We've passed yesterday without contributions
					return 0
				}
				continue
			}
			// Check if this day has contributions
			if day.ContributionCount > 0 {
				streak = 1
				expectDate = day.Date
			}
			continue
		}

		// We're in a streak - check for consecutive days
		expectedPrev := prevDate(expectDate)
		if day.Date == expectedPrev {
			if day.ContributionCount > 0 {
				streak++
				expectDate = day.Date
			} else {
				// Gap in contributions
				break
			}
		} else if day.Date < expectedPrev {
			// Skip days not in sequence (shouldn't happen with sorted data)
			break
		}
	}

	return streak
}

// prevDate returns the date string for the day before the given date.
func prevDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}
