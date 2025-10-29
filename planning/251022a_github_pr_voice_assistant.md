# GitHub PR Voice Assistant

## Goal

Build a voice-enabled GitHub AI assistant that helps developers manage their pull requests through natural conversation. The assistant leverages the existing ZANA voice infrastructure (ElevenLabs TTS + OpenAI Whisper) and integrates with GitHub via direct REST API calls to provide hands-free PR management.

**Note**: Future versions may migrate to MCP (Model Context Protocol) for standardized GitHub interactions, but v1 uses direct HTTP API calls to avoid Docker/MCP server dependencies.

**Core capabilities:**

- List PRs assigned for review
- List user's own PRs
- Fetch and explain PR comments with context
- Advanced PR operations (merge, respond to comments, check status, view diffs)

All interactions are voice-based, building on the existing voice conversation flow already implemented in the codebase.

## Progress Summary

### ✅ Completed (Frontend UI)

- **Voice-left, Content-right Layout**: Voice orb panel on left, GitHub content display on right
- **Session Management**: Fresh session on each page reload (localStorage cleared on mount)
- **Content Display**: Right panel shows only GitHub data (PRs, errors) with ChatGPT-style typing animation
- **Continuous Voice Loop**: Auto-restarts recording after TTS playback for seamless conversation
- **Audio Format Fix**: Improved MediaRecorder MIME selection and container format for better decoder compatibility
- **Request Session Continuity Fix**: Ensure consistent `sessionId` is sent in all requests within a session (fallback to localStorage when React state lags) and added debug logs for `X-Session-Id` roundtrips

### ✅ Completed (Backend)

- **Classifier Prompt Strengthening**: Explicitly instruct classifier to use prior turns and avoid re-asking
- **Option B Enabled**: Embed full transcript into a single system message for intent classification (temporary for reliability)
- **Removed Repo Format Guidance**: Prompt no longer requires explicit `owner/repo` phrasing

### ⚠️ In Progress

- **Voice Recording**: Audio format issues on some browsers (testing codec support)
- **PR Data Display**: Basic structure in place; need real data from GitHub API

### ✅ Completed (Database Integration)

- **Database Storage**: Migrated from file-based to PostgreSQL database storage for GitHub tokens and authentication
- **Migration System**: Created migration framework with automatic tracking and execution
- **Database Store**: Implemented persistent storage for session-based GitHub authentication
- **Fallback Support**: System gracefully falls back to file storage if database is not configured
- **Session Management**: Session ID stored in HTTP-only cookie with 30-minute expiration for security

### 🔜 Next Steps

1. Test database migration and OAuth flow with real database
2. Fix voice recording audio format issues (Safari/Chrome compatibility)
3. Test end-to-end voice flow with real GitHub data
4. Add error handling for GitHub API rate limits
5. Cost optimization: evaluate reverting to Option A (role-based history without transcript embedding) now that session continuity is fixed

## References

### Existing Codebase

- `backend/internal/server/server.go` - HTTP server with voice endpoints (`/api/voice`, `/api/chat/stream`, `/api/tts`)
- `backend/internal/config/config.go` - Configuration loading (OpenAI, ElevenLabs)
- `web/src/App.tsx` - Voice UI with recording, VAD (voice activity detection), TTS playback
- `backend/internal/types/types.go` - Request/response types
- `backend/internal/store/memory.go` - In-memory session storage
- `README.md` - Current system architecture and setup

### External Dependencies (to be integrated)

- **GitHub REST API** - Direct HTTP calls for PR operations (list, comments, merge, status, diffs)
- **GitHub OAuth** - For user authentication and authorization
- **OpenAI API** - Already integrated; will be used for LLM reasoning about PRs
- **Future**: GitHub MCP Server - Consider migrating to MCP for standardized interface (requires Docker setup)

### Documentation

- `docs/WRITE_PLANNING_DOC.md` - Planning document guidelines (this doc follows that structure)

## Principles & Key Decisions

1. **Voice-first design**: All features must work purely through voice interaction; no manual UI navigation required
2. **Single user, github.com only**: Start with single-user OAuth flow; no GitHub Enterprise support in v1
3. **Database persistence**: Store GitHub tokens, session data, and user information in PostgreSQL database (migrating from file-based storage)
4. **Configurable LLM**: Allow switching between OpenAI models (and potentially other providers later)
5. **Incremental rollout**: Build in phases - listing first, then comments, then advanced operations
6. **Graceful degradation**: If GitHub API fails, explain the issue via voice rather than showing errors
7. **Context-aware responses**: LLM should understand PR context (reviews requested, authored PRs, etc.) and respond naturally

## Stages & Actions

### Stage: Project Setup & Infrastructure

- [x] Create `planning/` directory structure
- [x] Research GitHub REST API capabilities
  - GitHub REST API v3 supports: listing PRs, fetching comments, merging, creating comments, checking status, diffs
  - No Docker/MCP server required; direct HTTPS calls
  - Rate limits: 5,000 requests/hour for authenticated users
- [x] Research GitHub OAuth flow for single-user applications
  - OAuth flow implemented with `golang.org/x/oauth2`
  - Token stored in file for persistence
- [x] Add new Go dependencies to `backend/go.mod`
  - ✅ `golang.org/x/oauth2` - OAuth authentication
  - ✅ GitHub API client library: `github.com/google/go-github/v57/github` (optional, can use raw HTTP)
- [x] Update `backend/internal/config/config.go` to include GitHub configuration
  - ✅ Added `GitHubClientID`, `GitHubClientSecret`, `GitHubTokenFile`, `GitHubRedirectURL`, `GitHubScopes`
  - ✅ Load from environment variables
- [x] Create file storage utility in `backend/internal/store/file.go`
  - ✅ Functions to read/write GitHub tokens securely
- [ ] Update frontend environment variables in `web/.env.example`
  - Document any new configuration needed

### ✅ Stage: Database Migration & Storage

- [x] Add PostgreSQL driver dependency to `backend/go.mod`
  - ✅ Added `github.com/lib/pq` for PostgreSQL driver
  - ✅ Dependency installed successfully
- [x] Update `backend/internal/config/config.go` to include database configuration
  - ✅ Added `DatabaseURL` field to Config struct
  - ✅ Load from `DB_URL` environment variable
  - ✅ Included in config loading
- [x] Create database schema migration in `backend/migrations/`
  - ✅ Created `001_initial_schema.sql` migration file
  - ✅ Defined `github_auth` table with columns:
    - `session_id` (VARCHAR PRIMARY KEY)
    - `github_token` (TEXT NOT NULL)
    - `github_owner` (VARCHAR NOT NULL)
    - `created_at` (TIMESTAMP DEFAULT NOW())
    - `updated_at` (TIMESTAMP DEFAULT NOW())
  - ✅ Added indexes on `github_owner` and `created_at` for performance
- [x] Create database connection utility in `backend/internal/db/db.go`
  - ✅ Implemented connection pooling with configurable limits
  - ✅ Added `HealthCheck()` function
  - ✅ Added migration runner to apply migrations on startup
  - ✅ Tracks applied migrations in `schema_migrations` table
- [x] Create database store in `backend/internal/store/database.go`
  - ✅ Implemented `DatabaseStore` struct
  - ✅ Implemented `SaveGitHubAuth(sessionId, token, owner)` method with upsert logic
  - ✅ Implemented `GetGitHubAuth(sessionId)` method
  - ✅ Implemented `DeleteGitHubAuth(sessionId)` method for cleanup
  - ✅ Added `GetGitHubAuthByOwner(owner)` method for additional flexibility
- [x] Update OAuth callback handler in `backend/internal/server/github_auth.go`
  - ✅ Replaced file-based token storage with database insertion
  - ✅ Inserts session_id, github_token, and github_owner into database after OAuth completion
  - ✅ Added proper error handling for database operations
  - ✅ Falls back to file storage if database is not configured
- [x] Update token retrieval in `backend/internal/server/github_auth.go`
  - ✅ Modified `handleGitHubStatus` to query database first, with file fallback
  - ✅ Updated authentication check to use database store when available
- [x] Update environment configuration
  - ✅ Added `DB_URL` to `backend/env.example`
  - ✅ Documented PostgreSQL connection string format
- [ ] Test database migration and storage
  - [ ] Verify migration creates table correctly on first run
  - [ ] Test OAuth flow with database storage
  - [ ] Verify tokens persist across server restarts
  - [ ] Test cleanup of expired/invalid sessions
- [x] Health check actions
  - ✅ Run `go build` to verify no compilation errors
  - ✅ Build completed successfully with no errors
  - [ ] Test database connection on startup (requires database running)
  - [ ] Verify migration runs successfully (requires database running)

### ✅ Stage: Cookie-Based Session Management (Complete)

- [x] Update backend CORS configuration in `backend/internal/server/server.go`
  - ✅ Changed `AllowCredentials: true` for cookie support
- [x] Create cookie utility in `backend/internal/server/cookies.go`
  - ✅ Implemented `SetSessionCookie(w, sessionID)` function
  - ✅ Set HTTP-only, SameSite=Lax cookie
  - ✅ Set 30-minute expiration using `MaxAge: 1800`
  - ✅ Cookie name: `zana_session`
  - ✅ Added `GetSessionCookie` and `ClearSessionCookie` utilities
- [x] Update session creation in `backend/internal/server/server.go`
  - ✅ Created `getOrCreateSessionID(r, w)` helper function
  - ✅ Automatically sets session cookie on new sessions
  - ✅ Updated all handlers to use cookie-based session management
- [x] Add session ID retrieval helpers
  - ✅ Created `getSessionID(r)` to extract from cookie, header, or query parameter
  - ✅ Handles missing/invalid cookies gracefully with fallback to headers
- [x] Update frontend in `web/src/App.tsx`
  - ✅ Removed localStorage session management
  - ✅ Cookie-based session handling implemented
  - ✅ Added `credentials: 'include'` to all fetch requests
- [x] Update API client calls in frontend
  - ✅ Added `credentials: 'include'` to all fetch calls
  - ✅ Removed manual session ID handling
- [ ] Test cookie-based session management
  - Verify cookie is set on initial load
  - Verify cookie expires after 30 minutes
  - Verify session persists across page reloads
  - Verify OAuth flow works with cookies
- [ ] Health check actions
  - Run `go build` to verify no compilation errors
  - Test cookie setting and reading
  - Verify cookies work in browser DevTools

### Stage: Phase 1 - GitHub OAuth & Basic PR Listing

#### Authentication Flow

- [x] Create OAuth handler in `backend/internal/server/github_auth.go`
  - ✅ Implemented `/api/github/auth` endpoint to initiate OAuth flow
  - ✅ Implemented `/api/github/callback` endpoint to handle OAuth callback
  - ✅ Store access token securely in file specified by config
- [x] Add session-based auth state tracking
  - ✅ Store OAuth state parameter to prevent CSRF
  - ✅ Link GitHub token to user session
- [x] Create middleware to verify GitHub authentication
  - ✅ Check if valid token exists in config or file store
  - ✅ Return friendly voice message if not authenticated (via intent)
- [x] Add `/api/github/status` endpoint
  - ✅ Return whether user is authenticated
  - ✅ Return authenticated username if available

#### GitHub REST API Integration

- [x] Create `backend/internal/github/github_api_client.go`
  - ✅ `GitHubAPIClient` implements `MCPClient` via REST v3
  - ✅ Direct API calls using `net/http`
  - ✅ Error handling for non-2xx responses
- [x] Implement PR listing functions in `backend/internal/github/pr_operations.go`
  - ✅ `ListPRsForReview(token)` uses search API
  - ✅ `ListUserPRs(token)` uses search API
  - ✅ `PR` struct defined with fields: number, title, author, status, url, repository

#### Backend API Endpoints

- [x] Create `/api/github/prs/review` endpoint
  - ✅ Use GitHub token from session or config
  - ✅ Call GitHub API client to fetch PRs for review
  - ✅ Return JSON list of PRs
- [x] Create `/api/github/prs/mine` endpoint
  - ✅ Use GitHub token from session or config
  - ✅ Call GitHub API client to fetch user's PRs
  - ✅ Return JSON list of PRs
- [x] Update system prompt in chat handlers
  - ✅ LLM-based intent classification using `prompts/intent.yaml`
  - ✅ Intent types: `list_prs_mine`, `list_prs_review`, `clarify`, `not_implemented`
  - ✅ Natural language response formatting with playful tone

#### Voice Command Integration

- [x] Update `backend/internal/server/server.go` chat handlers
  - ✅ Detect intent from user voice input (e.g., "list my PRs", "show PRs to review")
  - ✅ Route to appropriate GitHub API calls via `classifyAndHandle()`
  - ✅ Format PR data for natural language response
  - ✅ Include summary counts (e.g., "You have 3 PRs to review")
- [x] Implement intent detection utility in `backend/internal/github/intent.go`
  - ✅ Heuristic-based intent detection (fallback)
  - ✅ LLM-based intent classification in `intent_llm.go` (primary)
  - ✅ Return structured intent (action, parameters)
  - ✅ Handle ambiguous requests with clarifying questions

#### Testing

- [x] Manual testing of OAuth flow
  - ✅ Test initial authentication (popup flow working)
  - ✅ Verify token storage and retrieval (file-based)
  - ⚠️ Token refresh not yet implemented (tokens expire after ~8 hours)
- [ ] Test voice commands via frontend
  - ⚠️ Voice recording issues on some browsers (audio format)
  - Need to test: "List my pull requests"
  - Need to test: "Show me PRs I need to review"
  - Need to test: "What PRs am I working on?"
- [ ] Test edge cases
  - ✅ No PRs available (respond gracefully)
  - ✅ Unauthenticated requests (prompt to authenticate via intent)
  - ⚠️ GitHub API rate limiting (not yet handled; need error detection)

#### Health Checks

- [ ] Run `go build` in backend to verify no compilation errors
- [ ] Test all new API endpoints with curl/Postman
- [ ] Verify voice interaction flow end-to-end

#### Stop & Review

- [ ] Review with user - demonstrate Phase 1 functionality
- [ ] Gather feedback on voice interaction quality
- [ ] Discuss any adjustments before Phase 2

### Optimization Backlog (Cost/Latency)

- Revert to Option A classification once stable (remove transcript embedding, rely on role-based history and strengthened prompt). Rationale: session ID continuity bug caused earlier failures; with it fixed, Option A should work and significantly reduce tokens/cost and latency. Steps:
  - Update classifier to stop embedding transcript; pass role messages only
  - Keep "Conversation Use" guidance in prompt
  - Consider adding a lightweight summary of last PR list to assist mapping without large transcripts

### Stage: Phase 2 - PR Comments & AI Explanation

#### Comment Fetching

- [x] Extend `backend/internal/github/pr_operations.go`
  - ✅ `GetPRComments(token, repo, prNumber)` implemented
  - ✅ `Comment` struct defined
  - ✅ Fetch review and issue comments via REST
- [x] Create comments endpoints
  - ✅ `GET /api/github/repos/{owner}/{repo}/prs/{number}/comments`
  - ✅ `POST /api/github/repos/{owner}/{repo}/prs/{number}/comments`

#### AI-Powered Comment Analysis

- [ ] Create `backend/internal/github/comment_analyzer.go`
  - `AnalyzeComments(comments []Comment, llmClient) (Analysis, error)`
  - Define `Analysis` struct with:
    - Brief summary of feedback
    - List of action items
    - Sentiment classification (blocking, suggestions, questions, approvals)
    - Grouped themes (code quality, performance, security, etc.)
- [ ] Integrate analysis into chat flow
  - User asks: "Explain the comments on PR #123"
  - Fetch comments via GitHub REST API
  - Send to LLM for analysis
  - Return voice-friendly explanation
- [ ] Implement comment grouping logic
  - Group by file/location for inline comments
  - Separate review-level comments
  - Prioritize blocking issues first

#### Voice Commands for Comments

- [ ] Extend intent detection for comment-related queries
  - "What are the comments on PR 123?"
  - "Explain the feedback on my latest PR"
  - "Are there any blockers on PR 456?"
  - "Summarize review comments"
- [ ] Add PR reference resolution
  - Handle "my latest PR" → resolve to most recent PR by user
  - Handle "PR 123" → resolve to specific PR number
  - Handle repository context (if user has multiple repos)

#### Detailed Comment Explanation

- [ ] Create multi-level explanation responses
  - High-level summary (30 seconds or less)
  - Option to "tell me more" for detailed breakdown
  - Option to focus on specific aspects (e.g., "just the action items")
- [ ] Format responses for voice clarity
  - Use natural pauses and transitions
  - Number action items clearly ("First, second, third...")
  - Avoid reading raw code/technical jargon verbatim

#### Testing

- [ ] Test with PRs containing various comment types
  - Approving reviews
  - Requesting changes
  - Mixed feedback (some approvals, some change requests)
  - Long comment threads
- [ ] Test analysis accuracy
  - Verify action items are correctly identified
  - Check sentiment classification
  - Ensure critical feedback is highlighted
- [ ] Test voice interaction flow
  - Natural conversation about comments
  - Follow-up questions work correctly
  - Context is maintained across turns

#### Health Checks

- [ ] Run `go build` to verify compilation
- [ ] Check type safety with `go vet`
- [ ] Verify comment fetching with real GitHub PRs
- [ ] Test LLM analysis quality with sample comments

#### Stop & Review

- [ ] Demonstrate comment analysis to user
- [ ] Review voice explanation quality
- [ ] Confirm Phase 2 completion before Phase 3

### Stage: Phase 3 - Advanced PR Operations

#### Merge PR Capability

- [x] Extend `backend/internal/github/pr_operations.go`
  - ✅ `MergePR(token, repo, prNumber, mergeMethod)`
  - ✅ Supports merge, squash, rebase
  - ✅ Uses `PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge`
- [x] Create merge endpoint
  - ✅ `POST /api/github/repos/{owner}/{repo}/prs/{number}/merge`
- [ ] Add voice confirmation flow
  - User: "Merge PR 123"
  - Assistant: "PR 123 'Feature XYZ' has 2 approvals and all checks passed. Merge with squash, merge commit, or rebase?"
  - User: "Squash"
  - Assistant: "Merging now..." → perform merge → "Done! PR 123 has been merged."
- [ ] Implement safety checks
  - Warn if PR has no approvals
  - Warn if status checks are failing
  - Require explicit confirmation for force-merge scenarios

#### Respond to Comments

- [x] Extend `backend/internal/github/pr_operations.go`
  - ✅ `AddComment(token, repo, prNumber, body)`
  - ✅ `ReplyToReview(token, repo, prNumber, reviewID, body)`
  - ✅ APIs: issue comments and review replies
- [x] Create comments POST endpoint
  - ✅ Accepts general comment body (reply wired via intent handler)
- [ ] Voice-to-comment flow
  - User: "Reply to the comment about error handling in PR 123"
  - Assistant: "What would you like to say?"
  - User: [speaks response]
  - Assistant: "I'll post: '[transcribed text]'. Should I send it?"
  - User: "Yes"
  - Assistant: "Posted!"
- [ ] Support voice editing
  - User can say "change it to..." to revise before posting
  - User can say "cancel" to abort

#### PR Status & Checks

- [x] Extend `backend/internal/github/pr_operations.go`
  - ✅ `GetPRStatus(token, repo, prNumber)`
  - ✅ `Status` struct defined
  - ✅ Calls PR details, reviews, commit statuses
- [x] Create status endpoint
  - ✅ `GET /api/github/repos/{owner}/{repo}/prs/{number}/status`
- [ ] Voice status reporting
  - User: "What's the status of PR 123?"
  - Assistant: "PR 123 has 3 of 3 checks passing: build, test, and lint. It has 2 approvals from Alice and Bob. No merge conflicts. Ready to merge."
- [ ] Include check details on request
  - User: "Why did the build check fail?"
  - Assistant: [fetch check logs, summarize with LLM] "The build failed because..."

#### View Diffs

- [x] Extend `backend/internal/github/pr_operations.go`
  - ✅ `GetPRDiff(token, repo, prNumber)`
  - ✅ `Diff` and `DiffFile` structs defined
  - ✅ Uses `GET /repos/{owner}/{repo}/pulls/{pull_number}/files`
- [ ] Create diff summarization logic
  - If diff > 50 lines: return summary + GitHub URL
  - If diff ≤ 50 lines: can read key changes aloud
- [x] Voice diff interaction (backend endpoint only)
  - ✅ `GET /api/github/repos/{owner}/{repo}/prs/{number}/diff`
- [ ] Smart diff summarization
  - Identify changed functions/classes
  - Highlight significant logic changes
  - Mention new files or deletions
  - Categorize changes (refactoring, bug fix, feature, etc.)

#### Testing

- [ ] Test merge operations
  - Merge with different methods
  - Test merge conflict detection
  - Verify merge confirmation flow
- [ ] Test commenting
  - Post general comments
  - Reply to specific review comments
  - Test voice editing and confirmation
- [ ] Test status checks
  - PRs with passing checks
  - PRs with failing checks
  - PRs with pending checks
- [ ] Test diff handling
  - Small diffs (readable)
  - Large diffs (link only)
  - Summary quality

#### Health Checks

- [ ] Run `go build` for compilation check
- [ ] Test all Phase 3 endpoints
- [ ] Verify GitHub REST API operations work correctly
- [ ] Check voice interaction flow for all new features

#### Stop & Review

- [ ] Demonstrate all Phase 3 features to user
- [ ] Review overall voice assistant experience
- [ ] Discuss any refinements or additional features

### Stage: Final Polish & Documentation

#### Code Quality

- [ ] Run linter on Go backend (`go fmt`, `go vet`, `golangci-lint` if available)
- [ ] Run linter on TypeScript frontend (`npm run lint` in web/)
- [ ] Add inline comments for complex logic
- [ ] Ensure error messages are voice-friendly

#### Documentation

- [ ] Update `README.md` with GitHub integration setup
  - Document OAuth setup process
  - List environment variables for GitHub
  - Add usage examples for voice commands
- [ ] Create `docs/GITHUB_VOICE_COMMANDS.md`
  - List all supported voice commands
  - Provide examples and variations
  - Document best practices for natural interaction
- [ ] Update `.env.example` files
  - Add GitHub-related variables
  - Document each variable's purpose

#### Configuration & Deployment

- [ ] Add health check for GitHub integration
  - Extend `/api/health` to include GitHub API status (optional ping endpoint)
  - Include auth status (without exposing tokens)
- [ ] Test cold start behavior
  - Verify OAuth flow works on first run
  - Ensure file persistence survives restarts

#### Final Testing

- [ ] End-to-end test: Complete voice workflow
  - Authenticate via OAuth
  - List PRs
  - Get comment explanations
  - Merge a PR
  - Check status
  - Post a comment
- [ ] Test error scenarios
  - Network failures
  - GitHub API rate limits
  - Invalid PR numbers
  - Unauthenticated requests
- [ ] Test on different browsers/devices
  - Verify voice recording works
  - Check TTS playback
  - Test mobile experience

#### Health Check

- [ ] Run complete build: `go build` in backend
- [ ] Run complete build: `npm run build` in web/
- [ ] Execute all tests (if any exist)
- [ ] Verify no linter warnings

#### Git & Cleanup

- [ ] Commit all changes following `docs/GIT_COMMITS.md` if available
- [ ] Create summary of changes and learnings
- [ ] Move this planning doc to `planning/finished/`
- [ ] Update any relevant evergreen docs

## Appendix

### Voice Command Examples

**Authentication:**

- "Connect my GitHub account"
- "Am I logged into GitHub?"

**Listing PRs:**

- "List my pull requests"
- "What PRs do I need to review?"
- "Show me PRs I'm working on"
- "Do I have any open PRs?"

**Comments:**

- "What are the comments on PR 123?"
- "Explain the feedback on my latest PR"
- "Are there any blockers on PR 456?"
- "Summarize the review for PR 789"
- "Tell me more about the comments"
- "What action items do I have?"

**Advanced Operations:**

- "Merge PR 123"
- "What's the status of PR 456?"
- "Show me the diff for PR 789"
- "Reply to the comment about error handling"
- "Are the checks passing on PR 123?"

### GitHub REST API Research Notes

- **API Version**: GitHub REST API v3
- **Rate Limits**: 5,000 requests/hour for authenticated users, 60/hour for unauthenticated
- **Authentication**: OAuth2 with personal access tokens
- **Key Endpoints**:
  - Search PRs: `GET /search/issues?q=type:pr+...`
  - PR Details: `GET /repos/{owner}/{repo}/pulls/{number}`
  - Comments: `GET /repos/{owner}/{repo}/pulls/{number}/comments`
  - Merge: `PUT /repos/{owner}/{repo}/pulls/{number}/merge`
  - Status Checks: `GET /repos/{owner}/{repo}/commits/{ref}/status`
- **Best Practices**: Cache responses, handle 403 rate limit errors, use conditional requests (ETags)

### GitHub MCP Research Notes (Future)

- MCP requires Docker container running GitHub MCP server
- Provides standardized interface across different code hosts
- Consider migration after v1 stable
- Benefits: Abstraction layer, easier to add GitLab/Bitbucket support later

### OAuth Implementation Notes

- To be filled during implementation
- Document token refresh strategy
- Note any security considerations
- Record testing approach for OAuth flow

### Alternative Approaches Considered

**MCP vs Direct GitHub API:**

- **Chosen (v1)**: Direct GitHub REST API calls
- **Rationale**: Simpler deployment, no Docker/MCP server required, faster to implement
- **Tradeoff**: Tighter coupling to GitHub API; future refactor to MCP may be beneficial for abstraction
- **Future**: May migrate to MCP when ready for Docker-based deployment

**Token Storage:**

- **Chosen**: File-based storage
- **Rationale**: Simple, works for single-user, survives restarts
- **Tradeoff**: Not suitable for multi-user without enhancement (future consideration: use database)

**LLM for Intent Detection:**

- **Chosen**: Use LLM for natural language understanding
- **Rationale**: Flexible, handles variations naturally, already integrated
- **Tradeoff**: Slightly slower than regex, but much better UX
