// Package auth provides HTTP middleware for bearer token authentication.
package auth

import (
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that validates bearer token authentication.
// It checks the Authorization header for a valid "Bearer <token>" value.
// Requests without valid authentication receive a 401 Unauthorized response.
func Middleware(expectedToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			// Check for Bearer token format
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Extract and validate token
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Token valid, proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}
