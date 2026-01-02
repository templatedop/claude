// Package claude provides clients for interacting with Claude.
// This file defines the common interface that both API and Claude Code clients implement.
package claude

import (
	"context"
)

// Provider represents the type of Claude provider.
type Provider string

const (
	// ProviderAPI uses the Anthropic API directly (requires API credits)
	ProviderAPI Provider = "api"
	// ProviderClaudeCode uses Claude Code CLI (uses Claude subscription)
	ProviderClaudeCode Provider = "claude_code"
)

// ClaudeClient is the interface that both API and Claude Code clients implement.
type ClaudeClient interface {
	// Complete sends a prompt and returns a response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// SimpleComplete is a helper for simple text completions.
	SimpleComplete(ctx context.Context, system, prompt string) (string, error)

	// Provider returns the provider type.
	Provider() Provider

	// Close cleans up any resources.
	Close() error
}

// CompletionRequest represents a completion request that works with both providers.
type CompletionRequest struct {
	// System prompt
	System string

	// User prompt
	Prompt string

	// Model to use (optional, uses default if empty)
	Model string

	// Maximum tokens in response
	MaxTokens int

	// Temperature for sampling
	Temperature float64

	// Working directory for Claude Code (only used by Claude Code provider)
	WorkingDir string

	// Allowed tools for Claude Code (only used by Claude Code provider)
	// e.g., ["Read", "Write", "Bash", "Glob", "Grep"]
	AllowedTools []string

	// Whether to allow Claude Code to execute tools automatically
	// If false, tool calls are returned but not executed
	AutoExecuteTools bool

	// Additional context/conversation history
	Messages []Message
}

// CompletionResponse represents a response from Claude.
type CompletionResponse struct {
	// The text response
	Text string

	// Model used
	Model string

	// Token usage (may not be available for Claude Code provider)
	InputTokens  int
	OutputTokens int

	// Stop reason
	StopReason string

	// Tool uses (if any tools were called)
	ToolUses []ToolUse

	// Raw response (provider-specific)
	Raw interface{}
}

// Ensure Client implements ClaudeClient
var _ ClaudeClient = (*Client)(nil)

// Complete implements ClaudeClient for the API client.
func (c *Client) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := req.Messages
	if len(messages) == 0 && req.Prompt != "" {
		messages = []Message{NewTextMessage(RoleUser, req.Prompt)}
	}

	model := req.Model
	if model == "" {
		model = DefaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = DefaultMaxTokens
	}

	temp := req.Temperature
	msgReq := MessageRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    messages,
		System:      req.System,
		Temperature: &temp,
	}

	resp, err := c.CreateMessage(ctx, msgReq)
	if err != nil {
		return nil, err
	}

	return &CompletionResponse{
		Text:         resp.GetText(),
		Model:        resp.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		StopReason:   resp.StopReason,
		ToolUses:     resp.GetToolUses(),
		Raw:          resp,
	}, nil
}

// SimpleComplete implements ClaudeClient for the API client.
func (c *Client) SimpleComplete(ctx context.Context, system, prompt string) (string, error) {
	return c.SimpleCompletion(ctx, system, prompt)
}

// Provider returns the provider type for the API client.
func (c *Client) Provider() Provider {
	return ProviderAPI
}

// Close implements ClaudeClient for the API client.
func (c *Client) Close() error {
	return nil
}
