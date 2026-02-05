// Package storage provides data persistence via the GitHub API.
package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Common errors returned by the storage layer.
var (
	ErrNotFound      = errors.New("file not found")
	ErrConflict      = errors.New("file was modified concurrently (SHA mismatch)")
	ErrUnauthorized  = errors.New("GitHub API authentication failed")
	ErrRateLimited   = errors.New("GitHub API rate limit exceeded")
)

// Storage defines the interface for reading and writing data files.
// This abstraction exists primarily for testability.
type Storage interface {
	ReadFile(ctx context.Context, path string) (content string, sha string, err error)
	WriteFile(ctx context.Context, path string, content string, sha string, message string) error
}

// GitHubStorage implements Storage using the GitHub Contents API.
type GitHubStorage struct {
	token      string
	owner      string
	repo       string
	httpClient *http.Client
}

// NewGitHubStorage creates a new GitHubStorage instance.
// The repoPath should be in "owner/repo" format.
func NewGitHubStorage(token, repoPath string) (*GitHubStorage, error) {
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo path %q: expected owner/repo format", repoPath)
	}

	return &GitHubStorage{
		token: token,
		owner: parts[0],
		repo:  parts[1],
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// contentsResponse represents the GitHub Contents API response.
type contentsResponse struct {
	Content  string `json:"content"`
	SHA      string `json:"sha"`
	Encoding string `json:"encoding"`
	Message  string `json:"message"` // Present on error responses
}

// ReadFile fetches a file from the GitHub repository.
// Returns the file content, its SHA (needed for updates), and any error.
func (g *GitHubStorage) ReadFile(ctx context.Context, path string) (string, string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", g.owner, g.repo, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if err := g.checkResponseError(resp); err != nil {
		return "", "", err
	}

	var data contentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", fmt.Errorf("decoding response: %w", err)
	}

	if data.Encoding != "base64" {
		return "", "", fmt.Errorf("unexpected encoding: %s", data.Encoding)
	}

	// GitHub returns base64 with newlines, so we need to remove them
	cleanContent := strings.ReplaceAll(data.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleanContent)
	if err != nil {
		return "", "", fmt.Errorf("decoding base64 content: %w", err)
	}

	return string(decoded), data.SHA, nil
}

// writeRequest represents the GitHub Contents API PUT request body.
type writeRequest struct {
	Message string `json:"message"`
	Content string `json:"content"`
	SHA     string `json:"sha,omitempty"` // Required for updates, omit for creates
}

// WriteFile writes content to a file in the GitHub repository.
// The sha parameter should be the SHA from the last ReadFile call (for updates)
// or empty string (for new files).
func (g *GitHubStorage) WriteFile(ctx context.Context, path string, content string, sha string, message string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", g.owner, g.repo, path)

	body := writeRequest{
		Message: message,
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
		SHA:     sha,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	return g.checkResponseError(resp)
}

// checkResponseError converts HTTP error responses to appropriate errors.
func (g *GitHubStorage) checkResponseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		// GitHub returns 403 for rate limiting, check headers
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return ErrRateLimited
		}
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		// Read body for error details
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
}
