package types

type ChatRequest struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	System    string `json:"system,omitempty"`
}

type ChatResponse struct {
	SessionID  string          `json:"sessionId"`
	Reply      string          `json:"reply"`
	Transcript string          `json:"transcript,omitempty"`
	Intent     *IntentResponse `json:"intent,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// IntentResponse allows the backend to indicate a structured action/content
// for the frontend to display.
type IntentResponse struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}
