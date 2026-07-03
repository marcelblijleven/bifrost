package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the application configuration read from environment variables.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
	APIKey      string // programmatic admin bearer token
	JWTSecret   string // signs user session tokens
	// PublicURL is the externally reachable URL of this Bifrost instance (e.g. https://bifrost.example.com).
	// Required to auto-install webhooks on providers via the UI.
	PublicURL string
	GitHub    GitHubConfig
	Gitea     GiteaConfig
	Forgejo   GiteaConfig // same API as Gitea, different base URL and provider ID
}

// GitHubConfig holds GitHub-specific configuration.
// App credentials take priority over a personal access token when both are set.
type GitHubConfig struct {
	// Personal access token (GITHUB_TOKEN)
	Token string
	// GitHub App credentials (GITHUB_APP_ID, GITHUB_INSTALLATION_ID, GITHUB_PRIVATE_KEY)
	AppID          int64
	InstallationID int64
	PrivateKey     string // PEM content
	// Enterprise: API base URL (GITHUB_BASE_URL). Empty = use api.github.com.
	// GHES example:  https://github.company.com/api/v3/
	// EU cloud:      https://api.eu.github.com/
	BaseURL string
	// Enterprise: upload URL (GITHUB_UPLOAD_URL). Empty = derived from BaseURL.
	UploadURL string
}

// GiteaConfig holds configuration for a Gitea or Forgejo instance.
type GiteaConfig struct {
	// Base URL of the instance, e.g. "https://gitea.example.com" (GITEA_URL / FORGEJO_URL)
	URL string
	// Personal access token (GITEA_TOKEN / FORGEJO_TOKEN)
	Token string
}

// Load reads configuration from environment variables and returns a Config.
// Required variables: DATABASE_URL, API_KEY.
// Optional variables: HTTP_ADDR (default ":8080"), GITHUB_TOKEN.
func Load() (*Config, error) {
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("API_KEY environment variable is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	ghCfg := GitHubConfig{
		Token:      os.Getenv("GITHUB_TOKEN"),
		PrivateKey: os.Getenv("GITHUB_PRIVATE_KEY"),
		BaseURL:    os.Getenv("GITHUB_BASE_URL"),
		UploadURL:  os.Getenv("GITHUB_UPLOAD_URL"),
	}
	if v := os.Getenv("GITHUB_APP_ID"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
		}
		ghCfg.AppID = id
	}
	if v := os.Getenv("GITHUB_INSTALLATION_ID"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid GITHUB_INSTALLATION_ID: %w", err)
		}
		ghCfg.InstallationID = id
	}

	return &Config{
		HTTPAddr:    httpAddr,
		DatabaseURL: databaseURL,
		APIKey:      apiKey,
		JWTSecret:   jwtSecret,
		PublicURL:   os.Getenv("PUBLIC_URL"),
		GitHub:      ghCfg,
		Gitea: GiteaConfig{
			URL:   os.Getenv("GITEA_URL"),
			Token: os.Getenv("GITEA_TOKEN"),
		},
		Forgejo: GiteaConfig{
			URL:   os.Getenv("FORGEJO_URL"),
			Token: os.Getenv("FORGEJO_TOKEN"),
		},
	}, nil
}
