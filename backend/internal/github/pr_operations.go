package github

import "context"

// Thin wrappers that can be used by server handlers, keeping token handling separate.

func ListPRsForReview(ctx context.Context, mcp MCPClient, token string) ([]PR, error) {
	return mcp.ListPRsForReview(ctx, token)
}

func ListUserPRs(ctx context.Context, mcp MCPClient, token string) ([]PR, error) {
	return mcp.ListUserPRs(ctx, token)
}

func GetPRComments(ctx context.Context, mcp MCPClient, token, repo string, prNumber int) ([]Comment, error) {
	return mcp.GetPRComments(ctx, token, repo, prNumber)
}

func MergePR(ctx context.Context, mcp MCPClient, token, repo string, prNumber int, method string) error {
	return mcp.MergePR(ctx, token, repo, prNumber, method)
}

func AddComment(ctx context.Context, mcp MCPClient, token, repo string, prNumber int, body string) error {
	return mcp.AddComment(ctx, token, repo, prNumber, body)
}

func ReplyToReview(ctx context.Context, mcp MCPClient, token, repo string, prNumber int, reviewID int, body string) error {
	return mcp.ReplyToReview(ctx, token, repo, prNumber, reviewID, body)
}

func GetPRStatus(ctx context.Context, mcp MCPClient, token, repo string, prNumber int) (Status, error) {
	return mcp.GetPRStatus(ctx, token, repo, prNumber)
}

func GetPRDiff(ctx context.Context, mcp MCPClient, token, repo string, prNumber int) (Diff, error) {
	return mcp.GetPRDiff(ctx, token, repo, prNumber)
}
