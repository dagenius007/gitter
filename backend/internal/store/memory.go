package store

import (
	"sync"
	"time"
)

type Message struct {
	Role    string
	Content string
}

type MemoryStore struct {
	mu          sync.RWMutex
	sessions    map[string][]Message
	maxMessages int
	// OAuth state mapping per session (for CSRF protection)
	oauthStateBySession map[string]string
	// Optional: username associated with session after auth
	usernameBySession map[string]string
	// Reverse mapping: state -> sessionID to resolve callbacks
	sessionByOAuthState map[string]string
	// Last PRs cache for quick repo resolution by PR number
	lastPRsBySession map[string]LastPRsCache
	// Pending intent with partially filled slots
	pendingBySession map[string]PendingIntent
}

func NewMemoryStore(maxMessages int) *MemoryStore {
	return &MemoryStore{
		sessions:            make(map[string][]Message),
		maxMessages:         maxMessages,
		oauthStateBySession: make(map[string]string),
		usernameBySession:   make(map[string]string),
		sessionByOAuthState: make(map[string]string),
		lastPRsBySession:    make(map[string]LastPRsCache),
		pendingBySession:    make(map[string]PendingIntent),
	}
}

func (m *MemoryStore) Append(sessionID string, msg Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sessionID] = append(m.sessions[sessionID], msg)
	m.trimLocked(sessionID)
}

func (m *MemoryStore) Get(sessionID string) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.sessions[sessionID]
	copyMsgs := make([]Message, len(msgs))
	copy(copyMsgs, msgs)
	return copyMsgs
}

func (m *MemoryStore) Set(sessionID string, msgs []Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sessionID] = append([]Message(nil), msgs...)
	m.trimLocked(sessionID)
}

func (m *MemoryStore) trimLocked(sessionID string) {
	if m.maxMessages <= 0 {
		return
	}
	msgs := m.sessions[sessionID]
	if len(msgs) > m.maxMessages {
		m.sessions[sessionID] = msgs[len(msgs)-m.maxMessages:]
	}
}

// OAuth helpers

func (m *MemoryStore) SetOAuthState(sessionID, state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauthStateBySession[sessionID] = state
	m.sessionByOAuthState[state] = sessionID
}

func (m *MemoryStore) GetOAuthState(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.oauthStateBySession[sessionID]
}

func (m *MemoryStore) ClearOAuthState(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.oauthStateBySession[sessionID]; ok {
		delete(m.sessionByOAuthState, st)
		delete(m.oauthStateBySession, sessionID)
	}
}

func (m *MemoryStore) SetUsername(sessionID, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usernameBySession[sessionID] = username
}

func (m *MemoryStore) GetUsername(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.usernameBySession[sessionID]
}

func (m *MemoryStore) ClearUsername(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.usernameBySession, sessionID)
}

func (m *MemoryStore) GetSessionByOAuthState(state string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionByOAuthState[state]
}

// Slot/PR cache TTLs
var (
	lastPRsTTL = 7 * time.Minute
	pendingTTL = 7 * time.Minute
)

// PRRef holds just enough to resolve a repo from a PR number
type PRRef struct {
	Number     int
	Repository string
}

type LastPRsCache struct {
	PRs       []PRRef
	UpdatedAt time.Time
}

type PendingIntent struct {
	Type      string
	Args      map[string]any
	UpdatedAt time.Time
}

// SetLastPRs caches the most recent PR list for a session (used for repo resolution)
func (m *MemoryStore) SetLastPRs(sessionID string, prs []PRRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPRsBySession[sessionID] = LastPRsCache{PRs: append([]PRRef(nil), prs...), UpdatedAt: time.Now()}
}

// GetLastPRs returns cached PRs if within TTL.
func (m *MemoryStore) GetLastPRs(sessionID string) ([]PRRef, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cache, ok := m.lastPRsBySession[sessionID]
	if !ok {
		return nil, false
	}
	if time.Since(cache.UpdatedAt) > lastPRsTTL {
		delete(m.lastPRsBySession, sessionID)
		return nil, false
	}
	out := append([]PRRef(nil), cache.PRs...)
	return out, true
}

// SetPendingIntent stores/updates a pending intent with args and timestamp.
func (m *MemoryStore) SetPendingIntent(sessionID, typ string, args map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy args to avoid external mutation
	copyArgs := make(map[string]any, len(args))
	for k, v := range args {
		copyArgs[k] = v
	}
	m.pendingBySession[sessionID] = PendingIntent{Type: typ, Args: copyArgs, UpdatedAt: time.Now()}
}

// GetPendingIntent returns the pending intent if within TTL.
func (m *MemoryStore) GetPendingIntent(sessionID string) (string, map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pendingBySession[sessionID]
	if !ok {
		return "", nil, false
	}
	if time.Since(p.UpdatedAt) > pendingTTL {
		delete(m.pendingBySession, sessionID)
		return "", nil, false
	}
	// Return a copy of args
	args := make(map[string]any, len(p.Args))
	for k, v := range p.Args {
		args[k] = v
	}
	return p.Type, args, true
}

// ClearPendingIntent removes any pending intent for the session.
func (m *MemoryStore) ClearPendingIntent(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pendingBySession, sessionID)
}
