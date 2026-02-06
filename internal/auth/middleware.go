// Package auth provides authentication and authorization for the MCP server.
package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// TokenValidator is an interface for validating tokens from multiple sources.
type TokenValidator interface {
	// ValidateToken checks if a token is valid.
	// Returns true if valid, false otherwise.
	ValidateToken(token string) bool
}

// staticTokenValidator validates against a pre-shared static token.
type staticTokenValidator struct {
	token string
}

func (v *staticTokenValidator) ValidateToken(token string) bool {
	return token != "" && token == v.token
}

// oauthTokenValidator validates OAuth-issued access tokens.
type oauthTokenValidator struct {
	store *TokenStore
}

func (v *oauthTokenValidator) ValidateToken(token string) bool {
	return v.store.ValidateAccessToken(token) != nil
}

// MultiValidator combines multiple token validators.
// A token is valid if ANY validator accepts it.
type MultiValidator struct {
	validators []TokenValidator
}

// NewMultiValidator creates a validator that accepts tokens from multiple sources.
func NewMultiValidator(validators ...TokenValidator) *MultiValidator {
	return &MultiValidator{validators: validators}
}

// ValidateToken returns true if any validator accepts the token.
func (m *MultiValidator) ValidateToken(token string) bool {
	for _, v := range m.validators {
		if v.ValidateToken(token) {
			return true
		}
	}
	return false
}

// NewStaticTokenValidator creates a validator for static bearer tokens.
func NewStaticTokenValidator(token string) TokenValidator {
	return &staticTokenValidator{token: token}
}

// NewOAuthTokenValidator creates a validator for OAuth-issued tokens.
func NewOAuthTokenValidator(store *TokenStore) TokenValidator {
	return &oauthTokenValidator{store: store}
}

// MiddlewareConfig configures the auth middleware behavior.
type MiddlewareConfig struct {
	// Validator checks if tokens are valid.
	Validator TokenValidator

	// ResourceMetadataURL is included in WWW-Authenticate header on 401.
	// Per MCP spec, this helps clients discover the OAuth authorization server.
	ResourceMetadataURL string
}

// Middleware returns an HTTP middleware that validates bearer token authentication.
// It accepts tokens from any configured source (static token, OAuth tokens).
// Requests without valid authentication receive a 401 Unauthorized response
// with a WWW-Authenticate header per RFC 9728.
func Middleware(config MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			// Check for Bearer token format
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeUnauthorized(w, config.ResourceMetadataURL, "missing or invalid authorization header")
				return
			}

			// Extract and validate token
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if !config.Validator.ValidateToken(token) {
				writeUnauthorized(w, config.ResourceMetadataURL, "invalid token")
				return
			}

			// Token valid, proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// writeUnauthorized writes a 401 response with proper WWW-Authenticate header.
func writeUnauthorized(w http.ResponseWriter, resourceMetadataURL, errorDesc string) {
	// Build WWW-Authenticate header per RFC 9728
	wwwAuth := `Bearer`
	if resourceMetadataURL != "" {
		wwwAuth = fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadataURL)
	}
	if errorDesc != "" {
		wwwAuth += fmt.Sprintf(`, error="invalid_token", error_description="%s"`, errorDesc)
	}

	w.Header().Set("WWW-Authenticate", wwwAuth)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// LegacyMiddleware provides backwards compatibility with the old middleware signature.
// DEPRECATED: Use Middleware with MiddlewareConfig instead.
func LegacyMiddleware(expectedToken string) func(http.Handler) http.Handler {
	config := MiddlewareConfig{
		Validator: NewStaticTokenValidator(expectedToken),
	}
	return Middleware(config)
}
