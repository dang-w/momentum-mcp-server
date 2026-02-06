// Package auth provides OAuth 2.0 endpoints for MCP server authentication.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OAuthServer handles OAuth 2.0 authorization flows.
type OAuthServer struct {
	tokenStore   *TokenStore
	clientStore  *ClientStore
	authCodes    *AuthCodeStore
	baseURL      string
	authorizePin string // Optional PIN for authorize page
}

// OAuthConfig configures the OAuth server.
type OAuthConfig struct {
	TokenStore   *TokenStore
	BaseURL      string
	AuthorizePin string
}

// logAuthEvent logs an authorization event without exposing sensitive data.
func logAuthEvent(event, clientID, detail string) {
	// Never log tokens, codes, or PINs - only event type and client identifier
	log.Printf("[OAuth] %s: client=%s %s", event, clientID, detail)
}

// NewOAuthServer creates a new OAuth server.
func NewOAuthServer(config OAuthConfig) *OAuthServer {
	return &OAuthServer{
		tokenStore:   config.TokenStore,
		clientStore:  NewClientStore(),
		authCodes:    NewAuthCodeStore(),
		baseURL:      strings.TrimSuffix(config.BaseURL, "/"),
		authorizePin: config.AuthorizePin,
	}
}

// ProtectedResourceMetadata returns the OAuth Protected Resource Metadata (RFC 9728).
// This endpoint tells clients where to find the authorization server.
func (s *OAuthServer) ProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"resource":              s.baseURL,
		"authorization_servers": []string{s.baseURL},
		"scopes_supported":      []string{"mcp:read", "mcp:write"},
		"bearer_methods_supported": []string{"header"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// AuthorizationServerMetadata returns the OAuth Authorization Server Metadata (RFC 8414).
// This endpoint advertises our OAuth capabilities.
func (s *OAuthServer) AuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/authorize",
		"token_endpoint":                        s.baseURL + "/token",
		"registration_endpoint":                 s.baseURL + "/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"}, // Public clients
		"scopes_supported":                      []string{"mcp:read", "mcp:write"},
		"service_documentation":                 "https://github.com/dang-w/momentum-mcp-server",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// AuthCode represents an authorization code with associated data.
type AuthCode struct {
	Code                string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
	Used                bool
}

// AuthCodeStore manages authorization codes.
type AuthCodeStore struct {
	mu    sync.RWMutex
	codes map[string]*AuthCode
}

// NewAuthCodeStore creates a new authorization code store.
func NewAuthCodeStore() *AuthCodeStore {
	store := &AuthCodeStore{
		codes: make(map[string]*AuthCode),
	}
	go store.cleanupExpired()
	return store
}

// Store saves an authorization code.
func (s *AuthCodeStore) Store(code *AuthCode) {
	s.mu.Lock()
	s.codes[code.Code] = code
	s.mu.Unlock()
}

// Get retrieves and marks an authorization code as used.
// Returns nil if code doesn't exist, is expired, or was already used.
func (s *AuthCodeStore) Get(code string) *AuthCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	ac, exists := s.codes[code]
	if !exists || ac.Used || time.Now().After(ac.ExpiresAt) {
		return nil
	}
	ac.Used = true
	return ac
}

func (s *AuthCodeStore) cleanupExpired() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for code, ac := range s.codes {
			if now.After(ac.ExpiresAt) || ac.Used {
				delete(s.codes, code)
			}
		}
		s.mu.Unlock()
	}
}

// ClientInfo represents a registered OAuth client.
type ClientInfo struct {
	ClientID     string
	ClientName   string
	RedirectURIs []string
	CreatedAt    time.Time
}

// ClientStore manages registered OAuth clients.
type ClientStore struct {
	mu      sync.RWMutex
	clients map[string]*ClientInfo
}

// NewClientStore creates a new client store.
func NewClientStore() *ClientStore {
	store := &ClientStore{
		clients: make(map[string]*ClientInfo),
	}
	// Pre-register Claude.ai callback URLs as a default client
	store.RegisterDefaultClients()
	return store
}

// RegisterDefaultClients pre-registers known Claude clients.
func (s *ClientStore) RegisterDefaultClients() {
	// Claude.ai uses these callback URLs
	s.Register(&ClientInfo{
		ClientID:   "claude-ai",
		ClientName: "Claude.ai",
		RedirectURIs: []string{
			"https://claude.ai/api/mcp/auth_callback",
			"https://www.claude.ai/api/mcp/auth_callback",
		},
		CreatedAt: time.Now(),
	})
}

// Register adds a client to the store.
func (s *ClientStore) Register(client *ClientInfo) {
	s.mu.Lock()
	s.clients[client.ClientID] = client
	s.mu.Unlock()
}

// Get retrieves a client by ID.
func (s *ClientStore) Get(clientID string) *ClientInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[clientID]
}

// ValidateRedirectURI checks if a redirect URI is allowed for a client.
func (s *ClientStore) ValidateRedirectURI(clientID, redirectURI string) bool {
	client := s.Get(clientID)
	if client == nil {
		return false
	}
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// Authorize handles the OAuth authorization endpoint.
func (s *OAuthServer) Authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.authorizeGet(w, r)
	} else if r.Method == http.MethodPost {
		s.authorizePost(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *OAuthServer) authorizeGet(w http.ResponseWriter, r *http.Request) {
	// Parse required OAuth parameters
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	// Validate required parameters
	if clientID == "" || redirectURI == "" || responseType == "" {
		s.oauthError(w, "invalid_request", "Missing required parameters")
		return
	}

	if responseType != "code" {
		s.oauthError(w, "unsupported_response_type", "Only 'code' response type is supported")
		return
	}

	// PKCE is required per MCP spec
	if codeChallenge == "" || codeChallengeMethod != "S256" {
		s.oauthError(w, "invalid_request", "PKCE with S256 is required")
		return
	}

	// Validate redirect URI
	if !s.clientStore.ValidateRedirectURI(clientID, redirectURI) {
		s.oauthError(w, "invalid_request", "Invalid redirect_uri for client")
		return
	}

	// If no PIN required, auto-approve
	if s.authorizePin == "" {
		s.issueAuthorizationCode(w, r, clientID, redirectURI, state, codeChallenge, codeChallengeMethod)
		return
	}

	// Show authorization page with PIN entry
	s.renderAuthorizePage(w, clientID, redirectURI, state, codeChallenge, codeChallengeMethod)
}

func (s *OAuthServer) authorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.oauthError(w, "invalid_request", "Failed to parse form")
		return
	}

	pin := r.FormValue("pin")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	action := r.FormValue("action")

	// Check if user denied
	if action == "deny" {
		logAuthEvent("auth_denied", clientID, "user denied")
		redirectWithError(w, r, redirectURI, state, "access_denied", "User denied the request")
		return
	}

	// Validate PIN if required
	if s.authorizePin != "" {
		if subtle.ConstantTimeCompare([]byte(pin), []byte(s.authorizePin)) != 1 {
			logAuthEvent("auth_failed", clientID, "invalid PIN")
			// Re-render page with error
			s.renderAuthorizePageWithError(w, clientID, redirectURI, state, codeChallenge, codeChallengeMethod, "Invalid PIN")
			return
		}
	}

	s.issueAuthorizationCode(w, r, clientID, redirectURI, state, codeChallenge, codeChallengeMethod)
}

func (s *OAuthServer) issueAuthorizationCode(w http.ResponseWriter, r *http.Request, clientID, redirectURI, state, codeChallenge, codeChallengeMethod string) {
	// Generate authorization code
	code, err := generateSecureToken()
	if err != nil {
		s.oauthError(w, "server_error", "Failed to generate code")
		return
	}

	// Store the code with PKCE challenge
	s.authCodes.Store(&AuthCode{
		Code:                code,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(5 * time.Minute), // Short-lived
	})

	logAuthEvent("auth_code_issued", clientID, "")

	// Redirect back to client with code
	redirectURL := redirectURI + "?code=" + code
	if state != "" {
		redirectURL += "&state=" + state
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *OAuthServer) renderAuthorizePage(w http.ResponseWriter, clientID, redirectURI, state, codeChallenge, codeChallengeMethod string) {
	s.renderAuthorizePageWithError(w, clientID, redirectURI, state, codeChallenge, codeChallengeMethod, "")
}

func (s *OAuthServer) renderAuthorizePageWithError(w http.ResponseWriter, clientID, redirectURI, state, codeChallenge, codeChallengeMethod, errorMsg string) {
	client := s.clientStore.Get(clientID)
	clientName := clientID
	if client != nil {
		clientName = client.ClientName
	}

	data := map[string]string{
		"ClientName":          clientName,
		"ClientID":            clientID,
		"RedirectURI":         redirectURI,
		"State":               state,
		"CodeChallenge":       codeChallenge,
		"CodeChallengeMethod": codeChallengeMethod,
		"Error":               errorMsg,
		"PinRequired":         "true",
	}

	w.Header().Set("Content-Type", "text/html")
	if err := authorizeTemplate.Execute(w, data); err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// Token handles the OAuth token endpoint.
func (s *OAuthServer) Token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.tokenError(w, "invalid_request", "Failed to parse form")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		s.handleRefreshTokenGrant(w, r)
	default:
		s.tokenError(w, "unsupported_grant_type", "Grant type not supported")
	}
}

func (s *OAuthServer) handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")
	clientID := r.FormValue("client_id")

	if code == "" || codeVerifier == "" || clientID == "" {
		s.tokenError(w, "invalid_request", "Missing required parameters")
		return
	}

	// Retrieve and validate authorization code
	authCode := s.authCodes.Get(code)
	if authCode == nil {
		logAuthEvent("token_failed", clientID, "invalid or expired code")
		s.tokenError(w, "invalid_grant", "Invalid or expired authorization code")
		return
	}

	// Validate client_id matches
	if authCode.ClientID != clientID {
		logAuthEvent("token_failed", clientID, "client ID mismatch")
		s.tokenError(w, "invalid_grant", "Client ID mismatch")
		return
	}

	// Validate redirect_uri matches
	if authCode.RedirectURI != redirectURI {
		logAuthEvent("token_failed", clientID, "redirect URI mismatch")
		s.tokenError(w, "invalid_grant", "Redirect URI mismatch")
		return
	}

	// Validate PKCE code_verifier
	if !validatePKCE(codeVerifier, authCode.CodeChallenge) {
		logAuthEvent("token_failed", clientID, "invalid PKCE verifier")
		s.tokenError(w, "invalid_grant", "Invalid code_verifier")
		return
	}

	// Generate tokens
	s.issueTokens(w, clientID)
}

func (s *OAuthServer) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")

	if refreshToken == "" {
		s.tokenError(w, "invalid_request", "Missing refresh_token")
		return
	}

	// Validate refresh token
	tokenInfo := s.tokenStore.ValidateRefreshToken(refreshToken)
	if tokenInfo == nil {
		logAuthEvent("refresh_failed", clientID, "invalid or expired token")
		s.tokenError(w, "invalid_grant", "Invalid or expired refresh token")
		return
	}

	// Validate client_id if provided
	if clientID != "" && tokenInfo.ClientID != clientID {
		logAuthEvent("refresh_failed", clientID, "client ID mismatch")
		s.tokenError(w, "invalid_grant", "Client ID mismatch")
		return
	}

	// Issue new tokens (rotate refresh token for security)
	s.tokenStore.RevokeToken(refreshToken)
	logAuthEvent("token_refreshed", tokenInfo.ClientID, "")
	s.issueTokens(w, tokenInfo.ClientID)
}

func (s *OAuthServer) issueTokens(w http.ResponseWriter, clientID string) {
	// Generate refresh token first
	refreshToken, _, err := s.tokenStore.GenerateRefreshToken(clientID)
	if err != nil {
		s.tokenError(w, "server_error", "Failed to generate tokens")
		return
	}

	// Generate access token linked to refresh token
	accessToken, expiresAt, err := s.tokenStore.GenerateAccessToken(clientID, refreshToken)
	if err != nil {
		s.tokenError(w, "server_error", "Failed to generate tokens")
		return
	}

	// Calculate expires_in
	expiresIn := int(time.Until(expiresAt).Seconds())

	logAuthEvent("token_issued", clientID, "")

	response := map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": refreshToken,
		"scope":         "mcp:read mcp:write",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(response)
}

// Register handles dynamic client registration (RFC 7591).
func (s *OAuthServer) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		GrantTypes   []string `json:"grant_types"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.registrationError(w, "invalid_client_metadata", "Invalid JSON")
		return
	}

	if len(req.RedirectURIs) == 0 {
		s.registrationError(w, "invalid_redirect_uri", "At least one redirect_uri is required")
		return
	}

	// Generate client ID
	clientID, err := generateSecureToken()
	if err != nil {
		http.Error(w, "Failed to generate client ID", http.StatusInternalServerError)
		return
	}
	// Use shorter client ID
	clientID = clientID[:16]

	client := &ClientInfo{
		ClientID:     clientID,
		ClientName:   req.ClientName,
		RedirectURIs: req.RedirectURIs,
		CreatedAt:    time.Now(),
	}
	s.clientStore.Register(client)
	logAuthEvent("client_registered", clientID, req.ClientName)

	response := map[string]any{
		"client_id":                clientID,
		"client_name":              req.ClientName,
		"redirect_uris":            req.RedirectURIs,
		"grant_types":              []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_method": "none",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (s *OAuthServer) oauthError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func (s *OAuthServer) tokenError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func (s *OAuthServer) registrationError(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errorCode, description string) {
	url := redirectURI + "?error=" + errorCode + "&error_description=" + description
	if state != "" {
		url += "&state=" + state
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// validatePKCE validates the code_verifier against the code_challenge using S256.
func validatePKCE(verifier, challenge string) bool {
	// S256: BASE64URL(SHA256(verifier)) == challenge
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// Simple HTML template for the authorize page
var authorizeTemplate = template.Must(template.New("authorize").Parse(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authorize - Momentum MCP Server</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-width: 400px;
            margin: 50px auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .card {
            background: white;
            border-radius: 8px;
            padding: 24px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 { font-size: 1.5em; margin-top: 0; }
        .client-name { color: #0066cc; font-weight: bold; }
        .error { color: #cc0000; margin-bottom: 16px; }
        input[type="text"] {
            width: 100%;
            padding: 12px;
            margin: 8px 0 16px;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 1em;
            box-sizing: border-box;
        }
        .buttons { display: flex; gap: 12px; }
        button {
            flex: 1;
            padding: 12px;
            border: none;
            border-radius: 4px;
            font-size: 1em;
            cursor: pointer;
        }
        .approve { background: #0066cc; color: white; }
        .deny { background: #f0f0f0; color: #333; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Authorization Request</h1>
        <p><span class="client-name">{{.ClientName}}</span> wants to access your Momentum MCP Server.</p>
        {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
        <form method="POST">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
            {{if eq .PinRequired "true"}}
            <label for="pin">Enter PIN to authorize:</label>
            <input type="text" id="pin" name="pin" autocomplete="off" autofocus>
            {{end}}
            <div class="buttons">
                <button type="submit" name="action" value="deny" class="deny">Deny</button>
                <button type="submit" name="action" value="approve" class="approve">Approve</button>
            </div>
        </form>
    </div>
</body>
</html>
`))
