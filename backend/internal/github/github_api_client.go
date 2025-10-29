package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Keep the same public interface the rest of the code uses.
// v1 implements it using direct GitHub REST API calls.
type MCPClient interface {
	ListPRsForReview(ctx context.Context, token string) ([]PR, error)
	ListUserPRs(ctx context.Context, token string) ([]PR, error)
	GetPRComments(ctx context.Context, token, repo string, prNumber int) ([]Comment, error)
	MergePR(ctx context.Context, token, repo string, prNumber int, method string) error
	AddComment(ctx context.Context, token, repo string, prNumber int, body string) error
	ReplyToReview(ctx context.Context, token, repo string, prNumber int, reviewID int, body string) error
	GetPRStatus(ctx context.Context, token, repo string, prNumber int) (Status, error)
	GetPRDiff(ctx context.Context, token, repo string, prNumber int) (Diff, error)
}

// GitHubAPIClient implements MCPClient using direct GitHub REST API calls.
// It keeps a very small surface area tailored to our needs.
type GitHubAPIClient struct {
	httpClient *http.Client
	baseAPI    string
}

func newGitHubAPIClient() GitHubAPIClient {
	return GitHubAPIClient{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		baseAPI:    "https://api.github.com",
	}
}

// NewMCPClient retains the old constructor signature but returns the REST client.
func NewMCPClient(address string, enabled bool) MCPClient { //nolint:revive,stylecheck
	_ = address
	_ = enabled
	c := newGitHubAPIClient()
	return c
}

// ---- Helpers ----

func (c GitHubAPIClient) do(ctx context.Context, token, method, path string, accept string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseAPI+path, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	req.Header.Set("Accept", accept)
	return c.httpClient.Do(req)
}

func (c GitHubAPIClient) getJSON(ctx context.Context, token, path string, out any) error {
	resp, err := c.do(ctx, token, http.MethodGet, path, "application/vnd.github+json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api %s failed: %s", path, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func repoFromHTMLURL(u string) string {
	// Example: https://github.com/owner/repo/pull/123
	i := strings.Index(u, "github.com/")
	if i == -1 {
		return ""
	}
	rest := u[i+len("github.com/"):]
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// ---- Implementations ----

// Search Issues response (minimal fields used)
type searchIssuesResponse struct {
	Items []struct {
		Number        int    `json:"number"`
		Title         string `json:"title"`
		HTMLURL       string `json:"html_url"`
		RepositoryURL string `json:"repository_url"`
		User          struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"items"`
}

func (c GitHubAPIClient) searchPRs(ctx context.Context, token, q string) ([]PR, error) {
	// Build q parameter properly encoded
	u := url.URL{Path: "/search/issues"}
	qv := url.Values{}
	qv.Set("q", q)
	qv.Set("per_page", "20")
	u.RawQuery = qv.Encode()
	var resp searchIssuesResponse
	if err := c.getJSON(ctx, token, u.String(), &resp); err != nil {
		return nil, err
	}
	out := make([]PR, 0, len(resp.Items))
	for _, it := range resp.Items {
		repo := repoFromHTMLURL(it.HTMLURL)
		out = append(out, PR{
			Number:     it.Number,
			Title:      it.Title,
			Author:     it.User.Login,
			Status:     "open",
			URL:        it.HTMLURL,
			Repository: repo,
		})
	}
	return out, nil
}

func (c GitHubAPIClient) ListPRsForReview(ctx context.Context, token string) ([]PR, error) {
	// type:pr state:open review-requested:@me
	return c.searchPRs(ctx, token, "type:pr state:open review-requested:@me")
}

func (c GitHubAPIClient) ListUserPRs(ctx context.Context, token string) ([]PR, error) {
	// type:pr state:open author:@me
	return c.searchPRs(ctx, token, "type:pr state:open author:@me")
}

// ReviewComment represents a pull request review comment (inline)
type reviewComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body string `json:"body"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

// IssueComment represents a general PR (issue) comment
type issueComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

func (c GitHubAPIClient) GetPRComments(ctx context.Context, token, repo string, prNumber int) ([]Comment, error) {
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	// Review comments (inline)
	var review []reviewComment
	if err := c.getJSON(ctx, token, fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, name, prNumber), &review); err != nil {
		return nil, err
	}
	// Issue comments (general)
	var issue []issueComment
	if err := c.getJSON(ctx, token, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, name, prNumber), &issue); err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(review)+len(issue))
	for _, rc := range review {
		out = append(out, Comment{Author: rc.User.Login, Body: rc.Body, Timestamp: "", Type: "inline", Path: rc.Path, Line: rc.Line})
	}
	for _, ic := range issue {
		out = append(out, Comment{Author: ic.User.Login, Body: ic.Body, Timestamp: ic.CreatedAt, Type: "general"})
	}
	return out, nil
}

func (c GitHubAPIClient) MergePR(ctx context.Context, token, repo string, prNumber int, method string) error {
	if method == "" {
		method = "merge"
	}
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	// Build minimal JSON body
	body := strings.NewReader(fmt.Sprintf(`{"merge_method":"%s"}`, method))
	resp, err := c.do(ctx, token, http.MethodPut, fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, name, prNumber), "application/vnd.github+json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("merge failed: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

func (c GitHubAPIClient) AddComment(ctx context.Context, token, repo string, prNumber int, body string) error {
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	payload := strings.NewReader(fmt.Sprintf(`{"body":%q}`, body))
	resp, err := c.do(ctx, token, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, name, prNumber), "application/vnd.github+json", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add comment failed: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

// ReplyToReview posts a reply to a specific review comment thread.
// GitHub API: POST /repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies
// Note: This endpoint creates a threaded reply under a review comment.
func (c GitHubAPIClient) ReplyToReview(ctx context.Context, token, repo string, prNumber int, reviewID int, body string) error {
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	payload := strings.NewReader(fmt.Sprintf(`{"body":%q}`, body))
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments/%d/replies", owner, name, prNumber, reviewID)
	resp, err := c.do(ctx, token, http.MethodPost, path, "application/vnd.github+json", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reply to review failed: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

// PR details minimal subset
type prDetails struct {
	Mergeable *bool  `json:"mergeable"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type review struct {
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
}

type commitStatus struct {
	State    string `json:"state"`
	Statuses []struct {
		State   string `json:"state"`
		Context string `json:"context"`
	} `json:"statuses"`
}

func (c GitHubAPIClient) GetPRStatus(ctx context.Context, token, repo string, prNumber int) (Status, error) {
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return Status{}, fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	var pr prDetails
	if err := c.getJSON(ctx, token, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, name, prNumber), &pr); err != nil {
		return Status{}, err
	}
	// Reviews (accumulate approvals)
	var revs []review
	if err := c.getJSON(ctx, token, fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, name, prNumber), &revs); err != nil {
		return Status{}, err
	}
	approvals := make([]string, 0)
	for _, r := range revs {
		if strings.EqualFold(r.State, "APPROVED") {
			approvals = append(approvals, r.User.Login)
		}
	}
	// Status checks for head sha
	checksPassing, checksTotal := 0, 0
	var cs commitStatus
	if pr.Head.SHA != "" {
		if err := c.getJSON(ctx, token, fmt.Sprintf("/repos/%s/%s/commits/%s/status", owner, name, pr.Head.SHA), &cs); err == nil {
			checksTotal = len(cs.Statuses)
			for _, s := range cs.Statuses {
				if strings.EqualFold(s.State, "success") {
					checksPassing++
				}
			}
		}
	}
	st := Status{
		ChecksPassing: checksPassing,
		ChecksTotal:   checksTotal,
		Approvals:     approvals,
		Mergeable:     pr.Mergeable != nil && *pr.Mergeable,
		HasConflicts:  false,
	}
	return st, nil
}

type prFile struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

func (c GitHubAPIClient) GetPRDiff(ctx context.Context, token, repo string, prNumber int) (Diff, error) {
	ownerRepo := strings.Split(repo, "/")
	if len(ownerRepo) != 2 {
		return Diff{}, fmt.Errorf("invalid repo: %s", repo)
	}
	owner, name := ownerRepo[0], ownerRepo[1]
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=%s", owner, name, prNumber, url.QueryEscape(strconv.Itoa(100)))
	var files []prFile
	if err := c.getJSON(ctx, token, path, &files); err != nil {
		return Diff{}, err
	}
	diff := Diff{Files: make([]DiffFile, 0, len(files))}
	for _, f := range files {
		df := DiffFile(f)
		diff.Files = append(diff.Files, df)
		diff.Additions += df.Additions
		diff.Deletions += df.Deletions
	}
	diff.FilesChanged = len(diff.Files)
	return diff, nil
}
