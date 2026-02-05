package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewGitHubStorage(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		wantErr  bool
	}{
		{"valid", "owner/repo", false},
		{"invalid no slash", "ownerrepo", true},
		{"invalid empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGitHubStorage("token", tt.repoPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGitHubStorage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}


func TestGitHubStorage_CheckResponseError(t *testing.T) {
	gs := &GitHubStorage{}

	tests := []struct {
		name       string
		statusCode int
		headers    map[string]string
		wantErr    error
	}{
		{"ok", http.StatusOK, nil, nil},
		{"created", http.StatusCreated, nil, nil},
		{"not found", http.StatusNotFound, nil, ErrNotFound},
		{"conflict", http.StatusConflict, nil, ErrConflict},
		{"unauthorized", http.StatusUnauthorized, nil, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, nil, ErrUnauthorized},
		{"rate limited via header", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, ErrRateLimited},
		{"too many requests", http.StatusTooManyRequests, nil, ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a response with the desired status code
			rec := httptest.NewRecorder()
			// Headers must be set before WriteHeader
			for k, v := range tt.headers {
				rec.Header().Set(k, v)
			}
			rec.WriteHeader(tt.statusCode)
			resp := rec.Result()

			err := gs.checkResponseError(resp)
			if err != tt.wantErr {
				t.Errorf("checkResponseError() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitHubStorage_Integration(t *testing.T) {
	// This test requires GITHUB_TOKEN and GITHUB_REPO env vars
	// Skip if not available
	t.Skip("Integration test - run manually with environment variables set")

	// Example of how you'd run the integration test:
	// GITHUB_TOKEN=xxx GITHUB_REPO=owner/repo go test -run TestGitHubStorage_Integration -v
}

// mockTransport allows us to intercept HTTP requests for testing
type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func TestGitHubStorage_ReadFile_WithMockTransport(t *testing.T) {
	content := "# Test Content"
	encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

	gs, _ := NewGitHubStorage("test-token", "owner/repo")
	gs.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				// Verify the request
				if req.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing auth header")
				}

				// Create mock response
				resp := httptest.NewRecorder()
				json.NewEncoder(resp).Encode(map[string]string{
					"content":  encodedContent,
					"sha":      "sha123",
					"encoding": "base64",
				})
				return resp.Result(), nil
			},
		},
	}

	gotContent, gotSHA, err := gs.ReadFile(context.Background(), "test.md")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if gotContent != content {
		t.Errorf("ReadFile() content = %q, want %q", gotContent, content)
	}
	if gotSHA != "sha123" {
		t.Errorf("ReadFile() sha = %q, want %q", gotSHA, "sha123")
	}
}

func TestGitHubStorage_WriteFile_WithMockTransport(t *testing.T) {
	var capturedBody writeRequest

	gs, _ := NewGitHubStorage("test-token", "owner/repo")
	gs.httpClient = &http.Client{
		Transport: &mockTransport{
			handler: func(req *http.Request) (*http.Response, error) {
				// Verify request method
				if req.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", req.Method)
				}

				// Capture the body
				json.NewDecoder(req.Body).Decode(&capturedBody)

				// Return success
				resp := httptest.NewRecorder()
				resp.WriteHeader(http.StatusOK)
				return resp.Result(), nil
			},
		},
	}

	err := gs.WriteFile(context.Background(), "test.md", "new content", "old-sha", "Update file")
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Verify the captured body
	if capturedBody.Message != "Update file" {
		t.Errorf("message = %q, want %q", capturedBody.Message, "Update file")
	}
	if capturedBody.SHA != "old-sha" {
		t.Errorf("sha = %q, want %q", capturedBody.SHA, "old-sha")
	}

	decodedContent, _ := base64.StdEncoding.DecodeString(capturedBody.Content)
	if string(decodedContent) != "new content" {
		t.Errorf("content = %q, want %q", string(decodedContent), "new content")
	}
}
