// Package auth provides persistence for OAuth tokens and clients.
package auth

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistentData holds all data that survives server restarts.
type PersistentData struct {
	Tokens  map[string]*TokenInfo  `json:"tokens"`
	Clients map[string]*ClientInfo `json:"clients"`
	SavedAt time.Time              `json:"saved_at"`
}

// Persistence manages saving and loading OAuth state to disk.
type Persistence struct {
	mu       sync.Mutex
	filePath string
	tokens   *TokenStore
	clients  *ClientStore

	// For periodic saves
	saveInterval time.Duration
	stopCh       chan struct{}
}

// NewPersistence creates a persistence manager.
// If dataDir is empty, persistence is disabled (in-memory only).
func NewPersistence(dataDir string, tokens *TokenStore, clients *ClientStore) *Persistence {
	p := &Persistence{
		tokens:       tokens,
		clients:      clients,
		saveInterval: time.Minute, // Save every minute
		stopCh:       make(chan struct{}),
	}

	if dataDir != "" {
		p.filePath = filepath.Join(dataDir, "oauth_state.json")
	}

	return p
}

// Start begins periodic saving and loads existing state.
func (p *Persistence) Start() error {
	if p.filePath == "" {
		log.Println("Persistence disabled (no data directory configured)")
		return nil
	}

	// Load existing state
	if err := p.Load(); err != nil {
		// Log but don't fail - might be first run
		log.Printf("Could not load persisted state (may be first run): %v", err)
	}

	// Start periodic save goroutine
	go p.periodicSave()

	log.Printf("Persistence enabled: %s", p.filePath)
	return nil
}

// Stop performs a final save and stops periodic saving.
func (p *Persistence) Stop() {
	if p.filePath == "" {
		return
	}

	close(p.stopCh)

	// Final save
	if err := p.Save(); err != nil {
		log.Printf("Error during final save: %v", err)
	} else {
		log.Println("OAuth state saved successfully")
	}
}

// Load reads persisted state from disk.
func (p *Persistence) Load() error {
	if p.filePath == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, that's OK
		}
		return err
	}

	var persisted PersistentData
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	// Load tokens (only non-expired ones)
	now := time.Now()
	loadedTokens := 0
	for token, info := range persisted.Tokens {
		if now.Before(info.ExpiresAt) {
			p.tokens.mu.Lock()
			p.tokens.tokens[token] = info
			p.tokens.mu.Unlock()
			loadedTokens++
		}
	}

	// Load clients (excluding the default claude-ai which is always registered)
	loadedClients := 0
	for clientID, info := range persisted.Clients {
		if clientID != "claude-ai" { // Don't override the default
			p.clients.mu.Lock()
			p.clients.clients[clientID] = info
			p.clients.mu.Unlock()
			loadedClients++
		}
	}

	log.Printf("Loaded %d tokens and %d clients from %s (saved at %s)",
		loadedTokens, loadedClients, p.filePath, persisted.SavedAt.Format(time.RFC3339))

	return nil
}

// Save writes current state to disk.
func (p *Persistence) Save() error {
	if p.filePath == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Gather tokens
	p.tokens.mu.RLock()
	tokens := make(map[string]*TokenInfo, len(p.tokens.tokens))
	now := time.Now()
	for token, info := range p.tokens.tokens {
		// Only save non-expired tokens
		if now.Before(info.ExpiresAt) {
			tokens[token] = info
		}
	}
	p.tokens.mu.RUnlock()

	// Gather clients
	p.clients.mu.RLock()
	clients := make(map[string]*ClientInfo, len(p.clients.clients))
	for clientID, info := range p.clients.clients {
		clients[clientID] = info
	}
	p.clients.mu.RUnlock()

	persisted := PersistentData{
		Tokens:  tokens,
		Clients: clients,
		SavedAt: time.Now(),
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(p.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write atomically using temp file + rename
	tmpFile := p.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, p.filePath)
}

// periodicSave runs in the background and saves state periodically.
func (p *Persistence) periodicSave() {
	ticker := time.NewTicker(p.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := p.Save(); err != nil {
				log.Printf("Error during periodic save: %v", err)
			}
		case <-p.stopCh:
			return
		}
	}
}

// TriggerSave triggers an immediate save (call after important changes).
func (p *Persistence) TriggerSave() {
	if p.filePath == "" {
		return
	}
	go func() {
		if err := p.Save(); err != nil {
			log.Printf("Error during triggered save: %v", err)
		}
	}()
}
