// Package claude provides a client for the Anthropic Claude API.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultBaseURL     = "https://api.anthropic.com/v1"
	DefaultModel       = "claude-sonnet-4-20250514"
	DefaultMaxTokens   = 4096
	DefaultTemperature = 0.7
	DefaultTimeout     = 120 * time.Second
	APIVersion         = "2023-06-01"
)

// Common errors.
var (
	ErrNoAPIKey      = errors.New("API key is required")
	ErrEmptyMessage  = errors.New("message content is required")
	ErrRateLimited   = errors.New("rate limited by API")
	ErrServerError   = errors.New("server error")
	ErrInvalidResponse = errors.New("invalid response from API")
)

// Client is the Claude API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// ClientOption is a function that configures the client.
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets a custom timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// NewClient creates a new Claude API client.
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}

	c := &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Role represents a message role.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentType represents the type of content in a message.
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// Content represents a content block in a message.
type Content struct {
	Type   ContentType    `json:"type"`
	Text   string         `json:"text,omitempty"`
	Source *ImageSource   `json:"source,omitempty"`
}

// ImageSource represents an image source.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// Message represents a conversation message.
type Message struct {
	Role    Role      `json:"role"`
	Content []Content `json:"content"`
}

// NewTextMessage creates a new text message.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []Content{
			{Type: ContentTypeText, Text: text},
		},
	}
}

// ToolDefinition represents a tool that Claude can use.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolUse represents a tool call made by Claude.
type ToolUse struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// MessageRequest represents a request to create a message.
type MessageRequest struct {
	Model         string           `json:"model"`
	MaxTokens     int              `json:"max_tokens"`
	Messages      []Message        `json:"messages"`
	System        string           `json:"system,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	TopP          *float64         `json:"top_p,omitempty"`
	TopK          *int             `json:"top_k,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
	Tools         []ToolDefinition `json:"tools,omitempty"`
	ToolChoice    *ToolChoice      `json:"tool_choice,omitempty"`
	Metadata      *Metadata        `json:"metadata,omitempty"`
}

// ToolChoice represents how tools should be chosen.
type ToolChoice struct {
	Type string `json:"type"` // "auto", "any", or "tool"
	Name string `json:"name,omitempty"` // Required when type is "tool"
}

// Metadata represents request metadata.
type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

// ContentBlock represents a content block in a response.
type ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

// Usage represents token usage.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// MessageResponse represents a response from the messages API.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// GetText returns the text content from the response.
func (r *MessageResponse) GetText() string {
	for _, block := range r.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// GetToolUses returns all tool use blocks from the response.
func (r *MessageResponse) GetToolUses() []ToolUse {
	var tools []ToolUse
	for _, block := range r.Content {
		if block.Type == "tool_use" {
			tools = append(tools, ToolUse{
				Type:  block.Type,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return tools
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// CreateMessage sends a message request to the Claude API.
func (c *Client) CreateMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessage
	}

	// Set defaults
	if req.Model == "" {
		req.Model = DefaultModel
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("anthropic-version", APIVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("API error (%d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, ErrRateLimited
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return nil, ErrServerError
		default:
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
		}
	}

	var msgResp MessageResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &msgResp, nil
}

// SimpleCompletion is a helper for simple text completions.
func (c *Client) SimpleCompletion(ctx context.Context, system, prompt string) (string, error) {
	req := MessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, prompt)},
		System:   system,
	}

	resp, err := c.CreateMessage(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.GetText(), nil
}

// CompletionWithOptions allows more configuration for completions.
func (c *Client) CompletionWithOptions(ctx context.Context, opts CompletionOptions) (*MessageResponse, error) {
	temp := opts.Temperature
	req := MessageRequest{
		Model:         opts.Model,
		MaxTokens:     opts.MaxTokens,
		Messages:      opts.Messages,
		System:        opts.System,
		Temperature:   &temp,
		StopSequences: opts.StopSequences,
		Tools:         opts.Tools,
		ToolChoice:    opts.ToolChoice,
	}

	return c.CreateMessage(ctx, req)
}

// CompletionOptions provides options for completion requests.
type CompletionOptions struct {
	Model         string
	MaxTokens     int
	Messages      []Message
	System        string
	Temperature   float64
	StopSequences []string
	Tools         []ToolDefinition
	ToolChoice    *ToolChoice
}

// Conversation helps manage a multi-turn conversation.
type Conversation struct {
	client   *Client
	messages []Message
	system   string
	model    string
	maxTokens int
}

// NewConversation creates a new conversation.
func (c *Client) NewConversation(system string) *Conversation {
	return &Conversation{
		client:    c,
		messages:  []Message{},
		system:    system,
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}
}

// SetModel sets the model for the conversation.
func (conv *Conversation) SetModel(model string) *Conversation {
	conv.model = model
	return conv
}

// SetMaxTokens sets the max tokens for responses.
func (conv *Conversation) SetMaxTokens(tokens int) *Conversation {
	conv.maxTokens = tokens
	return conv
}

// Say sends a message and returns the response.
func (conv *Conversation) Say(ctx context.Context, text string) (string, error) {
	conv.messages = append(conv.messages, NewTextMessage(RoleUser, text))

	req := MessageRequest{
		Model:     conv.model,
		MaxTokens: conv.maxTokens,
		Messages:  conv.messages,
		System:    conv.system,
	}

	resp, err := conv.client.CreateMessage(ctx, req)
	if err != nil {
		// Remove the failed message
		conv.messages = conv.messages[:len(conv.messages)-1]
		return "", err
	}

	// Add assistant response to history
	assistantContent := []Content{}
	for _, block := range resp.Content {
		if block.Type == "text" {
			assistantContent = append(assistantContent, Content{Type: ContentTypeText, Text: block.Text})
		}
	}
	conv.messages = append(conv.messages, Message{Role: RoleAssistant, Content: assistantContent})

	return resp.GetText(), nil
}

// GetHistory returns the conversation history.
func (conv *Conversation) GetHistory() []Message {
	return conv.messages
}

// Reset clears the conversation history.
func (conv *Conversation) Reset() {
	conv.messages = []Message{}
}
