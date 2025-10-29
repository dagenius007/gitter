package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// GET /api/github/prs/review
func (s *Server) handlePRsForReview(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	prs, err := s.mcp.ListPRsForReview(ctx, token)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to list PRs for review")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"prs": prs})
}

// GET /api/github/prs/mine
func (s *Server) handlePRsMine(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	prs, err := s.mcp.ListUserPRs(ctx, token)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to list user PRs")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"prs": prs})
}

// GET /api/github/repos/{owner}/{repo}/prs/{number}/comments
func (s *Server) handlePRComments(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	prNumber, err := strconv.Atoi(numStr)
	if err != nil || owner == "" || repoName == "" || prNumber <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid repo or PR number")
		return
	}
	repo := owner + "/" + repoName
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	comments, err := s.mcp.GetPRComments(ctx, token, repo, prNumber)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to fetch PR comments")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"comments": comments})
}

// POST /api/github/repos/{owner}/{repo}/prs/{number}/comments
func (s *Server) handleAddPRComment(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	prNumber, err := strconv.Atoi(numStr)
	if err != nil || owner == "" || repoName == "" || prNumber <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid repo or PR number")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Body) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid comment body")
		return
	}
	repo := owner + "/" + repoName
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.mcp.AddComment(ctx, token, repo, prNumber, body.Body); err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to add comment")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// POST /api/github/repos/{owner}/{repo}/prs/{number}/merge
func (s *Server) handleMergePR(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	prNumber, err := strconv.Atoi(numStr)
	if err != nil || owner == "" || repoName == "" || prNumber <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid repo or PR number")
		return
	}
	var body struct {
		Method string `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	repo := owner + "/" + repoName
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err := s.mcp.MergePR(ctx, token, repo, prNumber, strings.ToLower(strings.TrimSpace(body.Method))); err != nil {
		s.writeError(w, http.StatusBadGateway, "merge failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"merged": true})
}

// GET /api/github/repos/{owner}/{repo}/prs/{number}/status
func (s *Server) handlePRStatus(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	prNumber, err := strconv.Atoi(numStr)
	if err != nil || owner == "" || repoName == "" || prNumber <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid repo or PR number")
		return
	}
	repo := owner + "/" + repoName
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	st, err := s.mcp.GetPRStatus(ctx, token, repo, prNumber)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to fetch PR status")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": st})
}

// GET /api/github/repos/{owner}/{repo}/prs/{number}/diff
func (s *Server) handlePRDiff(w http.ResponseWriter, r *http.Request) {
	token := s.cfg.GitHubToken
	if strings.TrimSpace(token) == "" {
		if t, _ := s.tokenStore.Read(); t != nil {
			token = t.AccessToken
		}
	}
	if strings.TrimSpace(token) == "" {
		s.writeError(w, http.StatusUnauthorized, "not authenticated with GitHub")
		return
	}
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	prNumber, err := strconv.Atoi(numStr)
	if err != nil || owner == "" || repoName == "" || prNumber <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid repo or PR number")
		return
	}
	repo := owner + "/" + repoName
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	df, err := s.mcp.GetPRDiff(ctx, token, repo, prNumber)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to fetch PR diff")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"diff": df})
}
