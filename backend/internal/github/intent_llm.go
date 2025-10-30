package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

type IntentSpec struct {
	System    string `yaml:"system"`
	Functions []struct {
		Name        string                 `yaml:"name"`
		Description string                 `yaml:"description"`
		ArgsSchema  map[string]interface{} `yaml:"args_schema"`
	} `yaml:"functions"`
	Style struct {
		Temperature float32 `yaml:"temperature"`
		Language    string  `yaml:"language"`
		MaxTokens   int     `yaml:"max_tokens"`
	} `yaml:"style"`
}

type ClassifiedIntent struct {
	Type       string                 `json:"type"`
	Args       map[string]interface{} `json:"args"`
	Confidence float32                `json:"confidence"`
	Message    string                 `json:"message,omitempty"`
}

type IntentClassifier struct {
	spec   IntentSpec
	client *openai.Client
	model  string
}

func LoadIntentClassifier(path string, client *openai.Client, model string) (*IntentClassifier, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec IntentSpec
	if err := yaml.Unmarshal(b, &spec); err != nil {
		return nil, err
	}
	return &IntentClassifier{spec: spec, client: client, model: model}, nil
}

// ClassifyChat accepts a full chat history with roles and classifies the user's intent
// using the same intent spec. It prepends the system instructions and function schema
// then appends the provided chat messages as-is.
func (c *IntentClassifier) ClassifyChat(ctx context.Context, chat []openai.ChatCompletionMessage) (*ClassifiedIntent, error) {
	fmt.Println("classifying chat", chat)
	sys := c.spec.System
	var fnSchema []map[string]interface{}
	for _, f := range c.spec.Functions {
		fnSchema = append(fnSchema, map[string]interface{}{
			"name":        f.Name,
			"description": f.Description,
			"args_schema": f.ArgsSchema,
		})
	}
	schemaJSON, _ := json.Marshal(fnSchema)
	styleT := c.spec.Style.Temperature
	if styleT <= 0 {
		styleT = 0.1
	}
	maxTok := c.spec.Style.MaxTokens
	if maxTok <= 0 {
		maxTok = 300
	}

	// Build a compact transcript and embed it into the single system message to avoid role ambiguity
	var b strings.Builder
	b.WriteString(sys)
	b.WriteString("\n\nFunctions:\n")
	b.WriteString(string(schemaJSON))
	b.WriteString("\n\nTranscript (role: content):\n")
	for _, m := range chat {
		role := strings.ToUpper(m.Role)
		if role == "" {
			role = "USER"
		}
		// Collapse whitespace to keep prompt compact
		content := strings.TrimSpace(m.Content)
		content = strings.ReplaceAll(content, "\n\n", "\n")
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	b.WriteString("\nInstructions: Use the transcript to extract any missing arguments. Do not re-ask for details clearly present in earlier turns. If multiple repositories share the same PR number, ask a targeted choice. Output ONLY the JSON object.\n")

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: b.String()},
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Temperature: styleT,
		MaxTokens:   maxTok,
		Messages:    messages,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices")
	}
	raw := resp.Choices[0].Message.Content
	var out ClassifiedIntent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		first := -1
		last := -1
		for i, r := range raw {
			if r == '{' {
				first = i
				break
			}
		}
		for i := len(raw) - 1; i >= 0; i-- {
			if raw[i] == '}' {
				last = i
				break
			}
		}
		if first >= 0 && last > first {
			if err2 := json.Unmarshal([]byte(raw[first:last+1]), &out); err2 != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if out.Args == nil {
		out.Args = map[string]interface{}{}
	}
	fmt.Println("classified chat", out)
	return &out, nil
}
