// Package auth provides authentication and authorization for the MCP server.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// TokenType distinguishes between access and refresh tokens.
type TokenType int

const (
	AccessToken TokenType = iota
	RefreshToken
)

// TokenInfo holds metadata about an issued token.
type TokenInfo struct {
	Token     string
	Type      TokenType
	ClientID  string
	ExpiresAt time.Time
	CreatedAt time.Time
	// RefreshTokenID links an access token to its refresh token (for revocation).
	RefreshTokenID string
}

// TokenStore manages OAuth tokens in memory.
// For a single-user server, in-memory storage is appropriate.
// Tokens are lost on server restart, requiring re-authentication.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*TokenInfo // keyed by token value

	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewTokenStore creates a new token store with the specified TTLs.
func NewTokenStore(accessTTL, refreshTTL time.Duration) *TokenStore {
	store := &TokenStore{
		tokens:          make(map[string]*TokenInfo),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}

	// Start background cleanup goroutine
	go store.cleanupExpired()

	return store
}

// GenerateAccessToken creates a new access token for the given client.
func (s *TokenStore) GenerateAccessToken(clientID string, refreshTokenID string) (string, time.Time, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(s.accessTokenTTL)

	s.mu.Lock()
	s.tokens[token] = &TokenInfo{
		Token:          token,
		Type:           AccessToken,
		ClientID:       clientID,
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Now(),
		RefreshTokenID: refreshTokenID,
	}
	s.mu.Unlock()

	return token, expiresAt, nil
}

// GenerateRefreshToken creates a new refresh token for the given client.
func (s *TokenStore) GenerateRefreshToken(clientID string) (string, time.Time, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(s.refreshTokenTTL)

	s.mu.Lock()
	s.tokens[token] = &TokenInfo{
		Token:     token,
		Type:      RefreshToken,
		ClientID:  clientID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	return token, expiresAt, nil
}

// ValidateToken checks if a token is valid and returns its info.
// Returns nil if the token is invalid or expired.
func (s *TokenStore) ValidateToken(token string, expectedType TokenType) *TokenInfo {
	s.mu.RLock()
	info, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return nil
	}

	if info.Type != expectedType {
		return nil
	}

	if time.Now().After(info.ExpiresAt) {
		// Token expired, remove it
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
		return nil
	}

	return info
}

// ValidateAccessToken is a convenience method for validating access tokens.
func (s *TokenStore) ValidateAccessToken(token string) *TokenInfo {
	return s.ValidateToken(token, AccessToken)
}

// ValidateRefreshToken is a convenience method for validating refresh tokens.
func (s *TokenStore) ValidateRefreshToken(token string) *TokenInfo {
	return s.ValidateToken(token, RefreshToken)
}

// RevokeToken removes a token from the store.
func (s *TokenStore) RevokeToken(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// RevokeRefreshTokenAndAccessTokens revokes a refresh token and all access tokens
// that were issued using it.
func (s *TokenStore) RevokeRefreshTokenAndAccessTokens(refreshToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove the refresh token
	delete(s.tokens, refreshToken)

	// Find and remove all access tokens linked to this refresh token
	for token, info := range s.tokens {
		if info.RefreshTokenID == refreshToken {
			delete(s.tokens, token)
		}
	}
}

// cleanupExpired periodically removes expired tokens.
func (s *TokenStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, info := range s.tokens {
			if now.After(info.ExpiresAt) {
				delete(s.tokens, token)
			}
		}
		s.mu.Unlock()
	}
}

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken() (string, error) {
	// 32 bytes = 256 bits of entropy
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// AccessTokenTTL returns the configured access token lifetime.
func (s *TokenStore) AccessTokenTTL() time.Duration {
	return s.accessTokenTTL
}

// RefreshTokenTTL returns the configured refresh token lifetime.
func (s *TokenStore) RefreshTokenTTL() time.Duration {
	return s.refreshTokenTTL
}
