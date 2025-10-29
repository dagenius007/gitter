package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	OpenAIAPIKey  string
	AllowedOrigin string
	Model         string
	TTSModel      string
	STTModel      string
	ElevenAPIKey  string
	ElevenVoiceID string
	ElevenModel   string
	// Database
	DatabaseURL string
	// GitHub OAuth
	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURL  string
	GitHubTokenFile    string
	GitHubScopes       []string
	// Optional static GitHub token (Personal Access Token) for local testing
	GitHubToken string
	// GitHub MCP
	GitHubMCPAddress string
	GitHubMCPEnabled bool
	// Default repo owner when user provides bare repo name
	DefaultRepoOwner string
}

func Load() Config {
	_ = godotenv.Load()
	cfg := Config{
		Port:               getEnvDefault("PORT", "8080"),
		OpenAIAPIKey:       os.Getenv("OPENAI_API_KEY"),
		AllowedOrigin:      getEnvDefault("ALLOWED_ORIGIN", "*"),
		Model:              getEnvDefault("OPENAI_MODEL", "gpt-4o-mini"),
		TTSModel:           getEnvDefault("OPENAI_TTS_MODEL", "tts-1"),
		STTModel:           getEnvDefault("OPENAI_STT_MODEL", "whisper-1"),
		ElevenAPIKey:       os.Getenv("ELEVEN_API_KEY"),
		ElevenVoiceID:      os.Getenv("ELEVEN_VOICE_ID"),
		ElevenModel:        getEnvDefault("ELEVEN_MODEL_ID", "eleven_multilingual_v2"),
		DatabaseURL:        os.Getenv("DB_URL"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubRedirectURL:  getEnvDefault("GITHUB_REDIRECT_URL", "http://localhost:8080/api/github/callback"),
		GitHubTokenFile:    getEnvDefault("GITHUB_TOKEN_FILE", "data/github_token.json"),
		GitHubScopes:       getEnvListDefault("GITHUB_OAUTH_SCOPES", []string{"repo", "read:user"}),
		GitHubToken:        os.Getenv("GITHUB_TOKEN"),
		GitHubMCPAddress:   os.Getenv("GITHUB_MCP_ADDRESS"),
		GitHubMCPEnabled:   getEnvBoolDefault("GITHUB_MCP_ENABLED", false),
		DefaultRepoOwner:   os.Getenv("DEFAULT_REPO_OWNER"),
	}
	if cfg.OpenAIAPIKey == "" {
		log.Println("warning: OPENAI_API_KEY is not set; API calls will fail until provided")
	}
	return cfg
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvListDefault(key string, def []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			s := strings.TrimSpace(p)
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return def
}

func getEnvBoolDefault(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return def
}
