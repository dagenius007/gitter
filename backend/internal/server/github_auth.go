package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"zana-speech-backend/internal/store"
)

// GET /api/github/status
// Returns { authenticated: bool, username?: string }
func (s *Server) handleGitHubStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sid := getSessionID(r)

	var authed bool
	var username string

	// Try database first if available
	if s.databaseStore != nil && sid != "" {
		auth, err := s.databaseStore.GetGitHubAuth(sid)
		if err == nil && auth != nil {
			authed = true
			username = auth.GitHubOwner
		}
	} else {
		// Fallback to file storage
		tok, _ := s.tokenStore.Read()
		authed = s.cfg.GitHubToken != "" || tok != nil
		if sid != "" {
			username = s.store.GetUsername(sid)
		}
	}

	resp := map[string]any{"authenticated": authed}
	if username != "" {
		resp["username"] = username
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// GET /api/github/auth?sessionId=...
// Initiates OAuth flow and returns { url } to redirect the browser
func (s *Server) handleGitHubAuth(w http.ResponseWriter, r *http.Request) {
	if s.oauthCfg == nil || s.oauthCfg.ClientID == "" || s.oauthCfg.ClientSecret == "" {
		s.writeError(w, http.StatusBadRequest, "github oauth not configured")
		return
	}
	sid := getOrCreateSessionID(r, w)
	state := randomState()
	s.store.SetOAuthState(sid, state)
	url := s.oauthCfg.AuthCodeURL(state)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-Id", sid)
	_ = json.NewEncoder(w).Encode(map[string]string{"url": url, "sessionId": sid})
}

// GET /api/github/callback?code=...&state=...
// Exchanges code for token and persists it; responds with a small HTML page that can close the popup
func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if s.oauthCfg == nil {
		s.writeError(w, http.StatusBadRequest, "github oauth not configured")
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		s.writeError(w, http.StatusBadRequest, "missing state or code")
		return
	}
	sid := s.store.GetSessionByOAuthState(state)
	if sid == "" || s.store.GetOAuthState(sid) != state {
		s.writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	fmt.Println("sid", sid)
	fmt.Println("state", state)
	fmt.Println("code", code)

	ctx := r.Context()
	tok, err := s.oauthCfg.Exchange(ctx, code)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "token exchange failed")
		return
	}

	// Fetch username for database storage
	username := fetchGitHubUsername(tok.AccessToken)
	if username == "" {
		s.writeError(w, http.StatusInternalServerError, "failed to fetch GitHub username")
		return
	}

	// Store in database if available, otherwise fall back to file storage
	if s.databaseStore != nil {
		if err := s.databaseStore.SaveGitHubAuth(sid, tok.AccessToken, username); err != nil {
			s.writeError(w, http.StatusInternalServerError, "failed to save GitHub auth to database")
			return
		}
	} else {
		// Fallback to file storage
		if err := s.tokenStore.Write(&store.GitHubToken{AccessToken: tok.AccessToken, TokenType: tok.TokenType}); err != nil {
			s.writeError(w, http.StatusInternalServerError, "token persist failed")
			return
		}
	}

	// Store username in memory store for quick access
	s.store.SetUsername(sid, username)
	s.store.ClearOAuthState(sid)

	// Set session cookie so popup and main window share the same session
	SetSessionCookie(w, r, sid)

	// Redirect to frontend with success indicator
	redirectURL := fmt.Sprintf("%s?githubAuth=success", s.cfg.FrontendURL)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func randomState() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Minimal call to get the GitHub username; avoid adding HTTP client deps, use stdlib
func fetchGitHubUsername(accessToken string) string {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Login)
}
