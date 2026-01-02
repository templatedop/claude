package claude

import (
	"fmt"
)

// ClientConfig contains configuration for creating a Claude client.
type ClientConfig struct {
	// Provider determines which Claude backend to use
	// "api" uses the Anthropic API (requires API credits)
	// "claude_code" uses Claude Code CLI (uses Claude subscription)
	Provider Provider

	// API-specific settings (only used when Provider = "api")
	APIKey string

	// Claude Code-specific settings (only used when Provider = "claude_code")
	WorkingDir   string
	AllowedTools []string

	// Common settings
	Model string
}

// NewClientFromConfig creates a Claude client based on configuration.
func NewClientFromConfig(cfg ClientConfig) (ClaudeClient, error) {
	switch cfg.Provider {
	case ProviderAPI, "":
		// Default to API if not specified
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key required for API provider")
		}
		return NewClient(cfg.APIKey)

	case ProviderClaudeCode:
		opts := []ClaudeCodeOption{}
		if cfg.Model != "" {
			opts = append(opts, WithClaudeCodeModel(cfg.Model))
		}
		if cfg.WorkingDir != "" {
			opts = append(opts, WithClaudeCodeWorkingDir(cfg.WorkingDir))
		}
		if len(cfg.AllowedTools) > 0 {
			opts = append(opts, WithClaudeCodeTools(cfg.AllowedTools))
		}
		return NewClaudeCodeClient(opts...)

	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

// MustNewClient creates a client and panics on error.
func MustNewClient(cfg ClientConfig) ClaudeClient {
	client, err := NewClientFromConfig(cfg)
	if err != nil {
		panic(err)
	}
	return client
}

// AutoDetectProvider determines the best provider based on environment.
// Returns "claude_code" if Claude Code CLI is available and authenticated,
// otherwise returns "api" if ANTHROPIC_API_KEY is set.
func AutoDetectProvider() (Provider, error) {
	// Try Claude Code first (uses subscription)
	ccClient, err := NewClaudeCodeClient()
	if err == nil {
		// Close the client since we're just detecting
		ccClient.Close()
		// Claude Code CLI is available
		return ProviderClaudeCode, nil
	}

	// Fall back to API if key is available
	// (The caller would need to provide the key)
	return ProviderAPI, nil
}
