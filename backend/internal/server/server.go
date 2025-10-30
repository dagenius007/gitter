package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	openai "github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"zana-speech-backend/internal/config"
	"zana-speech-backend/internal/db"
	gh "zana-speech-backend/internal/github"
	"zana-speech-backend/internal/store"
	"zana-speech-backend/internal/types"
)

type Server struct {
	router        *chi.Mux
	store         *store.MemoryStore
	client        *openai.Client
	cfg           config.Config
	oauthCfg      *oauth2.Config
	tokenStore    *store.FileTokenStore
	database      *db.DB
	databaseStore *store.DatabaseStore
	mcp           gh.MCPClient
	// LLM-based intent classifier
	intent *gh.IntentClassifier
}

func NewServer(cfg config.Config) (*Server, error) {
	client := openai.NewClient(cfg.OpenAIAPIKey)
	ms := store.NewMemoryStore(40)
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.AllowedOrigin},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Session-Id"},
		AllowCredentials: true, // Enable credentials for cookies
		MaxAge:           300,
	}))

	// OAuth2 config (may be partially empty if env not set; handlers will check)
	oCfg := &oauth2.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		RedirectURL:  cfg.GitHubRedirectURL,
		Scopes:       cfg.GitHubScopes,
		Endpoint:     github.Endpoint,
	}
	ts := store.NewFileTokenStore(cfg.GitHubTokenFile)

	// Initialize database if DB_URL is provided
	var database *db.DB
	var databaseStore *store.DatabaseStore
	if cfg.DatabaseURL != "" {
		var err error
		database, err = db.New(cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		log.Println("database connection established")

		// Run migrations
		// if err := database.RunMigrations("./migrations"); err != nil {
		// 	database.Close()
		// 	return nil, fmt.Errorf("failed to run migrations: %w", err)
		// }
		log.Println("database migrations completed")

		databaseStore = store.NewDatabaseStore(database)
	} else {
		log.Println("warning: DB_URL not provided, using file-based storage only")
	}

	mcp := gh.NewMCPClient(cfg.GitHubMCPAddress, cfg.GitHubMCPEnabled)
	intent, err := gh.LoadIntentClassifier("./prompts/intent.yaml", client, cfg.Model)
	if err != nil {
		log.Println("error loading intent classifier", err)
		return nil, fmt.Errorf("failed to load intent classifier: %w", err)
	}
	s := &Server{
		router:        r,
		store:         ms,
		client:        client,
		cfg:           cfg,
		oauthCfg:      oCfg,
		tokenStore:    ts,
		database:      database,
		databaseStore: databaseStore,
		mcp:           mcp,
		intent:        intent,
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.router.Get("/api/health", s.handleHealth)
	s.router.Post("/api/chat", s.handleChat)
	s.router.Post("/api/chat/stream", s.handleChatStream)
	s.router.Post("/api/voice", s.handleVoice)
	s.router.Post("/api/tts", s.handleTTS)
	s.router.Get("/api/tts/voices", s.handleTTSVoices)
	// GitHub OAuth
	s.router.Get("/api/github/status", s.handleGitHubStatus)
	s.router.Get("/api/github/auth", s.handleGitHubAuth)
	s.router.Get("/api/github/callback", s.handleGitHubCallback)
	// PR listing
	s.router.Get("/api/github/prs/review", s.handlePRsForReview)
	s.router.Get("/api/github/prs/mine", s.handlePRsMine)
	// PR details operations
	s.router.Get("/api/github/repos/{owner}/{repo}/prs/{number}/comments", s.handlePRComments)
	s.router.Post("/api/github/repos/{owner}/{repo}/prs/{number}/comments", s.handleAddPRComment)
	s.router.Post("/api/github/repos/{owner}/{repo}/prs/{number}/merge", s.handleMergePR)
	s.router.Get("/api/github/repos/{owner}/{repo}/prs/{number}/status", s.handlePRStatus)
	s.router.Get("/api/github/repos/{owner}/{repo}/prs/{number}/diff", s.handlePRDiff)
}

func (s *Server) Router() http.Handler { return s.router }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req types.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sid := getOrCreateSessionID(r, w)
	if strings.TrimSpace(req.Message) == "" {
		s.writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	if req.System != "" {
		s.store.Append(sid, store.Message{Role: "system", Content: req.System})
	}
	s.store.Append(sid, store.Message{Role: "user", Content: req.Message})

	// Check if GitHub account is connected for this session
	token := s.getGitHubToken(sid)
	if strings.TrimSpace(token) == "" {
		reply := "Please connect your GitHub account to use this application. This service helps you manage GitHub pull requests - fetching, listing, merging, and viewing PR comments."
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Session-Id", sid)
		_ = json.NewEncoder(w).Encode(types.ChatResponse{
			SessionID: sid,
			Reply:     reply,
			Intent:    &types.IntentResponse{Type: "require_github_auth"},
		})
		return
	}

	// Single-pass LLM intent classification and handling
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	reply, intent, ok := s.classifyAndHandle(ctx, sid, req.Message)
	if !ok {
		log.Printf("[chat] intent classification failed for message: %s", req.Message)
		s.writeError(w, http.StatusInternalServerError, "I'm having trouble understanding your request right now. Please try again.")
		return
	}
	s.store.Append(sid, store.Message{Role: "assistant", Content: reply})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-Id", sid)
	_ = json.NewEncoder(w).Encode(types.ChatResponse{SessionID: sid, Reply: reply, Intent: intent})
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	var req types.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sid := getOrCreateSessionID(r, w)
	if strings.TrimSpace(req.Message) == "" {
		s.writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.System != "" {
		s.store.Append(sid, store.Message{Role: "system", Content: req.System})
	}
	s.store.Append(sid, store.Message{Role: "user", Content: req.Message})

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Session-Id", sid)
	w.Header().Set("Cache-Control", "no-cache")

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	messages := s.convertMessages(s.store.Get(sid))

	stream, err := s.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    s.cfg.Model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		log.Println("openai stream error:", err)
		s.writeError(w, http.StatusBadGateway, "chat stream init failed")
		return
	}
	defer stream.Close()

	var builder strings.Builder
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println("stream recv error:", err)
			break
		}
		if len(response.Choices) == 0 {
			continue
		}
		chunk := response.Choices[0].Delta.Content
		if chunk == "" {
			continue
		}
		builder.WriteString(chunk)
		_, _ = w.Write([]byte(chunk))
		flusher.Flush()
	}
	final := builder.String()
	if strings.TrimSpace(final) != "" {
		s.store.Append(sid, store.Message{Role: "assistant", Content: final})
	}
}

func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	// Get or create session ID (cookie-based)
	sid := getOrCreateSessionID(r, w)
	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "audio file is required (field 'file')")
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	tr, err := s.client.CreateTranscription(ctx, openai.AudioRequest{
		Model:    s.cfg.STTModel,
		Reader:   file,
		FilePath: header.Filename,
	})
	if err != nil {
		log.Println("transcription error:", err)
		s.writeError(w, http.StatusBadGateway, "transcription failed")
		return
	}
	transcribed := strings.TrimSpace(tr.Text)
	if transcribed == "" {
		s.writeError(w, http.StatusBadGateway, "empty transcription")
		return
	}
	s.store.Append(sid, store.Message{Role: "user", Content: transcribed})

	// Check if GitHub account is connected for this session
	token := s.getGitHubToken(sid)
	if strings.TrimSpace(token) == "" {
		reply := "Please connect your GitHub account to use this application. This service helps you manage GitHub pull requests - fetching, listing, merging, and viewing PR comments."
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Session-Id", sid)
		_ = json.NewEncoder(w).Encode(types.ChatResponse{
			SessionID:  sid,
			Reply:      reply,
			Transcript: transcribed,
			Intent:     &types.IntentResponse{Type: "require_github_auth"},
		})
		return
	}

	// Single-pass LLM intent classification and handling (voice)
	reply, intent, ok := s.classifyAndHandle(ctx, sid, transcribed)
	if !ok {
		log.Printf("[voice] intent classification failed for message: %s", transcribed)
		s.writeError(w, http.StatusInternalServerError, "I'm having trouble understanding your request right now. Please try again.")
		return
	}
	s.store.Append(sid, store.Message{Role: "assistant", Content: reply})

	// Return JSON (frontend will speak via browser TTS)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-Id", sid)
	_ = json.NewEncoder(w).Encode(types.ChatResponse{SessionID: sid, Reply: reply, Transcript: transcribed, Intent: intent})
}

func (s *Server) convertMessages(msgs []store.Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "" {
			role = openai.ChatMessageRoleUser
		}
		fmt.Println("role", role, m.Content)
		out = append(out, openai.ChatCompletionMessage{Role: role, Content: m.Content})
	}

	return out
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(types.ErrorResponse{Error: msg})
}

func newSessionID() string {
	return fmt.Sprintf("s_%d", time.Now().UnixNano())
}

// getSessionID retrieves the session ID from cookie or query parameter/header
func getSessionID(r *http.Request) string {
	// Try cookie first
	if cookie, err := GetSessionCookie(r); err == nil && cookie != "" {
		return cookie
	}
	// Fall back to header
	if sid := r.Header.Get("X-Session-Id"); sid != "" {
		return sid
	}
	// Fall back to query parameter
	if sid := r.URL.Query().Get("sessionId"); sid != "" {
		return sid
	}
	return ""
}

// getOrCreateSessionID gets existing session ID or creates a new one, setting the cookie
func getOrCreateSessionID(r *http.Request, w http.ResponseWriter) string {
	sid := getSessionID(r)
	if sid == "" {
		sid = newSessionID()
		log.Printf("[session] creating new session: %s for endpoint: %s", sid, r.URL.Path)
		SetSessionCookie(w, r, sid)
	} else {
		log.Printf("[session] reusing existing session: %s for endpoint: %s", sid, r.URL.Path)
	}
	return sid
}

// getGitHubToken retrieves the GitHub token for a session with proper fallback:
// 1. Try database (session-specific token)
// 2. Try file-based token store (OAuth token)
// 3. Try config (fallback)
func (s *Server) getGitHubToken(sessionID string) string {
	// First priority: Check database for session-specific token
	if s.databaseStore != nil {
		if auth, err := s.databaseStore.GetGitHubAuth(sessionID); err == nil && auth != nil && strings.TrimSpace(auth.GitHubToken) != "" {
			return auth.GitHubToken
		}
	}

	// Second priority: Check file-based token store (OAuth flow)
	if token, err := s.tokenStore.Read(); err == nil && token != nil && strings.TrimSpace(token.AccessToken) != "" {
		return token.AccessToken
	}

	// Last priority: Fall back to config token
	return s.cfg.GitHubToken
}

// classifyAndHandle: LLM classifies a single intent and we handle it once.
// Returns reply text and a structured intent for the frontend.
func (s *Server) classifyAndHandle(ctx context.Context, sessionID, message string) (string, *types.IntentResponse, bool) {
	fmt.Println("classifying and handling", message)
	if s.intent == nil {
		return "", nil, false
	}
	// Convert full history to chat messages for role-aware classification.
	// Do NOT append the latest user message again; it is already included from store.
	chat := s.convertMessages(s.store.Get(sessionID))

	ci, err := s.intent.ClassifyChat(ctx, chat)
	if err != nil || ci == nil {
		fmt.Println("error classifying chat", err)
		return "", nil, false
	}
	fmt.Println("classified chat", ci)
	return s.handleWithArgs(ctx, sessionID, ci)
}

// handleWithArgs routes a classified intent, applying autofill and pending storage rules.
func (s *Server) handleWithArgs(ctx context.Context, sessionID string, ci *gh.ClassifiedIntent) (string, *types.IntentResponse, bool) {
	// Merge with any pending intent to support slot-filling across turns
	targetType := ci.Type
	// Copy args from classifier
	mergedArgs := make(map[string]any, len(ci.Args))
	for k, v := range ci.Args {
		mergedArgs[k] = v
	}
	if pType, pArgs, ok := s.store.GetPendingIntent(sessionID); ok {
		// If the model asked to clarify, treat it as continuing the pending intent
		if targetType == "clarify" && pType != "" {
			targetType = pType
		}
		if targetType == pType && targetType != "" {
			// Fill any missing args from pending
			for k, v := range pArgs {
				if _, exists := mergedArgs[k]; !exists {
					mergedArgs[k] = v
				}
			}
		}
	}

	switch targetType {
	case "list_prs_mine", "list_prs_review":
		fmt.Println("listing PRs", targetType)
		token := s.getGitHubToken(sessionID)
		if strings.TrimSpace(token) == "" {
			// Ask user to auth via friendly reply and structured intent.
			reply := "Whoops! I need your GitHub connection to fetch your pull requests. Let's connect GitHub first."
			return reply, &types.IntentResponse{Type: "require_github_auth"}, true
		}
		var prs []gh.PR
		var err error
		if targetType == "list_prs_mine" {
			prs, err = s.mcp.ListUserPRs(ctx, token)
		} else {
			prs, err = s.mcp.ListPRsForReview(ctx, token)
		}
		if err != nil {
			reply := "I couldn't fetch your pull requests from GitHub right now. This might be a temporary issue with GitHub's API. Try again in a moment?"
			return reply, &types.IntentResponse{Type: "error"}, true
		}
		kind := gh.IntentListMine
		if targetType == "list_prs_review" {
			kind = gh.IntentListReview
		}
		// Cache last PRs for auto-resolution by PR number (7m TTL in store)
		if len(prs) > 0 {
			refs := make([]store.PRRef, 0, len(prs))
			for _, p := range prs {
				refs = append(refs, store.PRRef{Number: p.Number, Repository: p.Repository})
			}
			s.store.SetLastPRs(sessionID, refs)
		}
		// Clear any pending intent when listing
		s.store.ClearPendingIntent(sessionID)
		reply := s.formatPRListReply(kind, prs)
		listKind := "mine"
		if kind == gh.IntentListReview {
			listKind = "review"
		}
		return reply, &types.IntentResponse{Type: "show_prs", Payload: map[string]any{"prs": prs, "kind": listKind}}, true
	case "get_pr_comments":
		fmt.Println("getting PR comments", targetType)
		repo, _ := mergedArgs["repo"].(string)
		var prNumber int
		if n, ok := mergedArgs["pr_number"].(float64); ok {
			prNumber = int(n)
		} else if n2, ok2 := mergedArgs["pr_number"].(int); ok2 {
			prNumber = n2
		}
		// Resolve bare repo to owner/repo if possible
		repo = strings.TrimSpace(repo)
		if repo != "" && !strings.Contains(repo, "/") {
			// Build owner/repo using username from session or default config
			owner := strings.TrimSpace(s.store.GetUsername(sessionID))
			if owner == "" {
				owner = strings.TrimSpace(s.cfg.DefaultRepoOwner)
			}
			if owner != "" {
				repo = owner + "/" + repo
			}
		}
		// Attempt auto-resolve repo via last listed PRs when missing
		repo = strings.TrimSpace(repo)
		if repo == "" && prNumber > 0 {
			if refs, ok := s.store.GetLastPRs(sessionID); ok {
				matches := make([]string, 0, 2)
				for _, r := range refs {
					if r.Number == prNumber {
						matches = append(matches, r.Repository)
					}
				}
				if len(matches) == 1 {
					repo = matches[0]
				} else if len(matches) > 1 {
					// Targeted clarification with options
					msg := fmt.Sprintf("Did you mean PR %d in %s?", prNumber, strings.Join(matches, " or "))
					// store pending with known pr_number
					mergedArgs["pr_number"] = prNumber
					s.store.SetPendingIntent(sessionID, "get_pr_comments", mergedArgs)
					return msg, &types.IntentResponse{Type: "clarify"}, true
				}
			}
		}
		// If still missing args, ask targeted clarifications and persist pending intent
		repo = strings.TrimSpace(repo)
		if repo == "" && prNumber <= 0 {
			msg := "Which repository and PR number should I look at?"
			s.store.SetPendingIntent(sessionID, "get_pr_comments", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}
		if repo == "" {
			msg := fmt.Sprintf("Which repo is PR %d in?", prNumber)
			s.store.SetPendingIntent(sessionID, "get_pr_comments", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}
		if prNumber <= 0 {
			msg := fmt.Sprintf("Which PR number in %s?", repo)
			s.store.SetPendingIntent(sessionID, "get_pr_comments", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}

		token := s.getGitHubToken(sessionID)
		if strings.TrimSpace(token) == "" {
			reply := "I need your GitHub connection to fetch PR comments. Let's connect GitHub first."
			return reply, &types.IntentResponse{Type: "require_github_auth"}, true
		}

		fmt.Println("Getting PR comments", repo, prNumber)
		comments, err := s.mcp.GetPRComments(ctx, token, repo, prNumber)
		if err != nil {
			fmt.Println("Error fetching comments", err)
			reply := "I couldn't retrieve the PR comments from GitHub. This could be a temporary GitHub API issue or the PR might not exist. Mind trying again?"
			return reply, &types.IntentResponse{Type: "error"}, true
		}
		// Update memory on success
		s.store.ClearPendingIntent(sessionID)
		reply := fmt.Sprintf("I found %d comment(s) on GitHub pull request %s#%d.", len(comments), repo, prNumber)
		return reply, &types.IntentResponse{Type: "show_comments", Payload: map[string]any{"repo": repo, "prNumber": prNumber, "comments": comments}}, true
	case "merge_pr":
		fmt.Println("merging PR", targetType)
		repo, _ := mergedArgs["repo"].(string)
		var prNumber int
		if n, ok := mergedArgs["pr_number"].(float64); ok {
			prNumber = int(n)
		} else if n2, ok2 := mergedArgs["pr_number"].(int); ok2 {
			prNumber = n2
		}
		method, _ := mergedArgs["merge_method"].(string)
		method = strings.ToLower(strings.TrimSpace(method))
		if method == "" {
			method = "merge"
		}
		// Resolve repo owner/repo if only name given
		repo = strings.TrimSpace(repo)
		if repo != "" && !strings.Contains(repo, "/") {
			owner := strings.TrimSpace(s.store.GetUsername(sessionID))
			if owner == "" {
				owner = strings.TrimSpace(s.cfg.DefaultRepoOwner)
			}
			if owner != "" {
				repo = owner + "/" + repo
			}
		}
		// Attempt auto-resolve repo via last listed PRs when missing
		if repo == "" && prNumber > 0 {
			if refs, ok := s.store.GetLastPRs(sessionID); ok {
				matches := make([]string, 0, 2)
				for _, r := range refs {
					if r.Number == prNumber {
						matches = append(matches, r.Repository)
					}
				}
				if len(matches) == 1 {
					repo = matches[0]
				} else if len(matches) > 1 {
					msg := fmt.Sprintf("Did you mean PR %d in %s?", prNumber, strings.Join(matches, " or "))
					mergedArgs["pr_number"] = prNumber
					s.store.SetPendingIntent(sessionID, "merge_pr", mergedArgs)
					return msg, &types.IntentResponse{Type: "clarify"}, true
				}
			}
		}
		// Missing fields clarifications
		if repo == "" && prNumber <= 0 {
			msg := "Which repo and PR should I merge?"
			s.store.SetPendingIntent(sessionID, "merge_pr", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}
		if repo == "" {
			msg := fmt.Sprintf("Which repo is PR %d in?", prNumber)
			s.store.SetPendingIntent(sessionID, "merge_pr", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}
		if prNumber <= 0 {
			msg := fmt.Sprintf("Which PR number in %s?", repo)
			s.store.SetPendingIntent(sessionID, "merge_pr", mergedArgs)
			return msg, &types.IntentResponse{Type: "clarify"}, true
		}
		token := s.getGitHubToken(sessionID)
		if strings.TrimSpace(token) == "" {
			reply := "I need your GitHub connection to merge pull requests. Let's connect GitHub first."
			return reply, &types.IntentResponse{Type: "require_github_auth"}, true
		}
		if err := s.mcp.MergePR(ctx, token, repo, prNumber, method); err != nil {
			reply := "I couldn't merge the pull request on GitHub. This could be due to failing checks, merge conflicts, or insufficient permissions. Would you like me to check the PR status?"
			return reply, &types.IntentResponse{Type: "error"}, true
		}
		s.store.ClearPendingIntent(sessionID)
		reply := fmt.Sprintf("Successfully merged GitHub pull request %s#%d using %s method.", repo, prNumber, method)
		return reply, &types.IntentResponse{Type: "merged", Payload: map[string]any{"repo": repo, "prNumber": prNumber, "method": method}}, true
	case "clarify":
		// Use LLM-provided playful message
		msg := strings.TrimSpace(ci.Message)
		if msg == "" {
			msg = "Mind giving me a tiny bit more detail? I promise I listen better than your rubber duck."
		}
		// Capture any args we already know (transcript mode only uses payload)
		repo, _ := mergedArgs["repo"].(string)

		var prNumber int
		if n, ok := mergedArgs["pr_number"].(float64); ok {
			prNumber = int(n)
		} else if n2, ok2 := mergedArgs["pr_number"].(int); ok2 {
			prNumber = n2
		}
		var reviewID int
		if rid, ok := mergedArgs["review_id"].(float64); ok {
			reviewID = int(rid)
		}

		repo = strings.TrimSpace(repo)
		// No slot memory update; transcript carries context
		payload := map[string]any{}
		if repo != "" {
			payload["repo"] = repo
		}
		if prNumber > 0 {
			payload["prNumber"] = prNumber
		}
		if reviewID > 0 {
			payload["reviewId"] = reviewID
		}
		// Keep pending intent if any exists so follow-up can merge
		if pType, pArgs, ok := s.store.GetPendingIntent(sessionID); ok && pType != "" {
			// Merge any new info into pending and refresh TTL
			for k, v := range payload {
				pArgs[k] = v
			}
			s.store.SetPendingIntent(sessionID, pType, pArgs)
		}
		return msg, &types.IntentResponse{Type: "clarify", Payload: payload}, true
	case "not_implemented", "unknown":
		// Treat unknown as not_implemented; use LLM-provided playful message
		msg := strings.TrimSpace(ci.Message)
		if msg == "" {
			msg = "I haven't learned that trick yet — but I'm practicing!"
		}
		// Do not carry stale pending intents across unknowns
		s.store.ClearPendingIntent(sessionID)
		return msg, &types.IntentResponse{Type: "not_implemented"}, true
	default:
		return "", nil, false
	}
}

// (no-op helpers removed; transcript-only mode)

// Removed per-session slot memory; classification uses full chat transcript

func (s *Server) formatPRListReply(kind gh.IntentKind, prs []gh.PR) string {
	if len(prs) == 0 {
		if kind == gh.IntentListReview {
			return "You have no GitHub pull requests to review at the moment."
		}
		return "You have no open pull requests on GitHub."
	}
	max := 5
	if len(prs) < max {
		max = len(prs)
	}
	var b strings.Builder
	if kind == gh.IntentListReview {
		fmt.Fprintf(&b, "You have %d GitHub pull request(s) to review. ", len(prs))
	} else {
		fmt.Fprintf(&b, "You have %d GitHub pull request(s). ", len(prs))
	}
	for i := 0; i < max; i++ {
		p := prs[i]
		if i == 0 {
			fmt.Fprintf(&b, "#%d %s (%s)", p.Number, p.Title, p.Repository)
		} else {
			fmt.Fprintf(&b, "; #%d %s (%s)", p.Number, p.Title, p.Repository)
		}
	}
	if len(prs) > max {
		fmt.Fprintf(&b, "; and %d more.", len(prs)-max)
	}
	return b.String()
}

// ElevenLabs TTS proxy: JSON { text, voiceId? } -> audio/mpeg
func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Text    string `json:"text"`
		VoiceID string `json:"voiceId,omitempty"`
	}
	var body reqBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid text body")
		return
	}
	if s.cfg.ElevenAPIKey == "" {
		s.writeError(w, http.StatusBadRequest, "elevenlabs not configured")
		return
	}

	// Build ElevenLabs request
	voiceID := s.cfg.ElevenVoiceID
	if strings.TrimSpace(body.VoiceID) != "" {
		voiceID = body.VoiceID
	}
	if strings.TrimSpace(voiceID) == "" {
		s.writeError(w, http.StatusBadRequest, "no elevenlabs voice configured or provided")
		return
	}
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s/stream", voiceID)
	payload := map[string]any{
		"text":     body.Text,
		"model_id": s.cfg.ElevenModel,
		"voice_settings": map[string]any{
			"stability":         0.5,
			"similarity_boost":  0.7,
			"style":             0.2,
			"use_speaker_boost": true,
		},
		"optimize_streaming_latency": 4,
		"output_format":              "mp3_44100_128",
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "tts request build failed")
		return
	}
	req.Header.Set("xi-api-key", s.cfg.ElevenAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "tts request failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		log.Println("elevenlabs error:", string(bb))
		s.writeError(w, http.StatusBadGateway, "tts error")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}

// ElevenLabs Voices proxy: GET -> JSON { voices: [...] }
func (s *Server) handleTTSVoices(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ElevenAPIKey == "" {
		s.writeError(w, http.StatusBadRequest, "elevenlabs not configured")
		return
	}

	req, err := http.NewRequest("GET", "https://api.elevenlabs.io/v1/voices", nil)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "voices request build failed")
		return
	}
	req.Header.Set("xi-api-key", s.cfg.ElevenAPIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "voices request failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		log.Println("elevenlabs voices error:", string(bb))
		s.writeError(w, http.StatusBadGateway, "voices error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}
