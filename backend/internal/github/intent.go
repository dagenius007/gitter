package github

import "strings"

type IntentKind string

const (
	IntentUnknown    IntentKind = "unknown"
	IntentListMine   IntentKind = "list_prs_mine"
	IntentListReview IntentKind = "list_prs_review"
)

type Intent struct {
	Kind IntentKind
}

// DetectIntent performs simple heuristics for PR listing intents.
func DetectIntent(message string) Intent {
	m := strings.ToLower(strings.TrimSpace(message))
	if m == "" {
		return Intent{Kind: IntentUnknown}
	}
	// Mine
	if containsAny(m, []string{
		"my prs", "my pr", "my pull requests", "list my prs", "list my pull requests",
		"show my prs", "show my pull requests", "what am i working on", "i'm working on",
		"i am working on", "my open prs", "my open pull requests",
	}) {
		return Intent{Kind: IntentListMine}
	}
	// For review
	if containsAny(m, []string{
		"prs to review", "pull requests to review", "need to review", "requested to review",
		"what do i need to review", "review requests", "assigned for review", "requested reviewer",
		"show reviews", "list reviews",
	}) || (strings.Contains(m, "review") && strings.Contains(m, "pr")) {
		return Intent{Kind: IntentListReview}
	}
	return Intent{Kind: IntentUnknown}
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
