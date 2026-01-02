package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ClaudeCodeClient uses the Claude Code CLI to interact with Claude.
// This uses your Claude subscription instead of API credits.
type ClaudeCodeClient struct {
	// Configuration
	model        string
	workingDir   string
	allowedTools []string
	timeout      time.Duration

	// Process management
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	scanner *bufio.Scanner
}

// ClaudeCodeOption is a function that configures the ClaudeCodeClient.
type ClaudeCodeOption func(*ClaudeCodeClient)

// WithClaudeCodeModel sets the model for Claude Code.
func WithClaudeCodeModel(model string) ClaudeCodeOption {
	return func(c *ClaudeCodeClient) {
		c.model = model
	}
}

// WithClaudeCodeWorkingDir sets the working directory for Claude Code.
func WithClaudeCodeWorkingDir(dir string) ClaudeCodeOption {
	return func(c *ClaudeCodeClient) {
		c.workingDir = dir
	}
}

// WithClaudeCodeTools sets the allowed tools for Claude Code.
func WithClaudeCodeTools(tools []string) ClaudeCodeOption {
	return func(c *ClaudeCodeClient) {
		c.allowedTools = tools
	}
}

// WithClaudeCodeTimeout sets the timeout for Claude Code operations.
func WithClaudeCodeTimeout(timeout time.Duration) ClaudeCodeOption {
	return func(c *ClaudeCodeClient) {
		c.timeout = timeout
	}
}

// NewClaudeCodeClient creates a new Claude Code client.
// Note: Claude Code CLI must be installed and authenticated.
// Run 'claude login' to authenticate with your Claude subscription.
func NewClaudeCodeClient(opts ...ClaudeCodeOption) (*ClaudeCodeClient, error) {
	c := &ClaudeCodeClient{
		model:        "claude-sonnet-4-20250514",
		workingDir:   ".",
		allowedTools: []string{"Read", "Write", "Bash", "Glob", "Grep"},
		timeout:      10 * time.Minute,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Check if Claude Code CLI is installed
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w. Install with: npm install -g @anthropic-ai/claude-code", err)
	}

	return c, nil
}

// Provider returns the provider type.
func (c *ClaudeCodeClient) Provider() Provider {
	return ProviderClaudeCode
}

// Close cleans up any resources.
func (c *ClaudeCodeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		c.stdin.Close()
		c.cmd.Process.Kill()
		c.cmd.Wait()
		c.cmd = nil
	}
	return nil
}

// claudeCodeRequest represents a request to Claude Code CLI.
type claudeCodeRequest struct {
	Prompt       string   `json:"prompt"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	Model        string   `json:"model,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	WorkingDir   string   `json:"cwd,omitempty"`
}

// claudeCodeMessage represents a message from Claude Code CLI.
type claudeCodeMessage struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
	Text    string          `json:"text,omitempty"`
	Error   string          `json:"error,omitempty"`

	// For tool use messages
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	ToolID    string                 `json:"tool_id,omitempty"`

	// For result messages
	Result string `json:"result,omitempty"`
}

// Complete sends a prompt to Claude Code and returns the response.
func (c *ClaudeCodeClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build the prompt with system context
	fullPrompt := req.Prompt
	if req.System != "" {
		fullPrompt = fmt.Sprintf("System: %s\n\nUser: %s", req.System, req.Prompt)
	}

	// Determine tools to use
	tools := req.AllowedTools
	if len(tools) == 0 {
		tools = c.allowedTools
	}

	// Determine working directory
	workDir := req.WorkingDir
	if workDir == "" {
		workDir = c.workingDir
	}

	// Build command arguments
	args := []string{
		"--print", // Print mode for non-interactive output
		"--output-format", "json", // JSON output for parsing
	}

	// Add model if specified
	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Add max tokens if specified
	if req.MaxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", req.MaxTokens))
	}

	// Add allowed tools
	if len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	}

	// Add the prompt
	args = append(args, fullPrompt)

	// Create command with context
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workDir

	// Set environment to ensure we don't use API key
	// This forces Claude Code to use the subscription
	env := os.Environ()
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		// Remove ANTHROPIC_API_KEY to force subscription auth
		if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = filteredEnv

	// Capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("claude CLI error: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run claude CLI: %w", err)
	}

	// Parse JSON output
	return c.parseOutput(output)
}

// parseOutput parses the JSON output from Claude Code CLI.
func (c *ClaudeCodeClient) parseOutput(output []byte) (*CompletionResponse, error) {
	// Claude Code outputs JSON lines
	var texts []string
	var toolUses []ToolUse
	var lastModel string
	var stopReason string

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg claudeCodeMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not JSON, might be plain text
			texts = append(texts, line)
			continue
		}

		switch msg.Type {
		case "text", "assistant":
			if msg.Text != "" {
				texts = append(texts, msg.Text)
			}
			// Try to extract text from content
			if msg.Content != nil {
				var textContent string
				if err := json.Unmarshal(msg.Content, &textContent); err == nil {
					texts = append(texts, textContent)
				}
			}

		case "tool_use":
			toolUses = append(toolUses, ToolUse{
				Type:  "tool_use",
				ID:    msg.ToolID,
				Name:  msg.ToolName,
				Input: msg.ToolInput,
			})

		case "result":
			if msg.Result != "" {
				texts = append(texts, msg.Result)
			}
			stopReason = "end_turn"

		case "error":
			return nil, fmt.Errorf("claude error: %s", msg.Error)

		case "system":
			// System messages, might contain model info
			lastModel = "claude-sonnet-4-20250514"
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading output: %w", err)
	}

	// If we got no structured output, use raw output as text
	if len(texts) == 0 && len(output) > 0 {
		texts = append(texts, string(output))
	}

	return &CompletionResponse{
		Text:       strings.Join(texts, "\n"),
		Model:      lastModel,
		ToolUses:   toolUses,
		StopReason: stopReason,
		Raw:        output,
	}, nil
}

// SimpleComplete is a helper for simple text completions.
func (c *ClaudeCodeClient) SimpleComplete(ctx context.Context, system, prompt string) (string, error) {
	resp, err := c.Complete(ctx, CompletionRequest{
		System: system,
		Prompt: prompt,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// RunWithTools runs Claude Code with specific tools and returns the result.
// This is useful for tasks that need to interact with the filesystem or execute commands.
func (c *ClaudeCodeClient) RunWithTools(ctx context.Context, prompt string, tools []string) (*CompletionResponse, error) {
	return c.Complete(ctx, CompletionRequest{
		Prompt:       prompt,
		AllowedTools: tools,
	})
}

// Interactive starts an interactive session with Claude Code.
// Returns a channel of messages and an error channel.
func (c *ClaudeCodeClient) Interactive(ctx context.Context, prompt string) (<-chan *claudeCodeMessage, <-chan error) {
	msgCh := make(chan *claudeCodeMessage, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(msgCh)
		defer close(errCh)

		args := []string{
			"--output-format", "stream-json",
			prompt,
		}

		cmd := exec.CommandContext(ctx, "claude", args...)
		cmd.Dir = c.workingDir

		// Remove API key from environment
		env := os.Environ()
		filteredEnv := make([]string, 0, len(env))
		for _, e := range env {
			if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
				filteredEnv = append(filteredEnv, e)
			}
		}
		cmd.Env = filteredEnv

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errCh <- fmt.Errorf("failed to get stdout: %w", err)
			return
		}

		if err := cmd.Start(); err != nil {
			errCh <- fmt.Errorf("failed to start claude: %w", err)
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var msg claudeCodeMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			select {
			case msgCh <- &msg:
			case <-ctx.Done():
				cmd.Process.Kill()
				return
			}
		}

		if err := cmd.Wait(); err != nil {
			if ctx.Err() == nil {
				errCh <- fmt.Errorf("claude exited with error: %w", err)
			}
		}
	}()

	return msgCh, errCh
}

// CheckAuth verifies that Claude Code is authenticated.
func (c *ClaudeCodeClient) CheckAuth(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "--version")

	// Remove API key to check subscription auth
	env := os.Environ()
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = filteredEnv

	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("claude CLI not working: %w", err)
	}

	// Try a simple command to verify auth
	testCmd := exec.CommandContext(ctx, "claude", "--print", "Say 'authenticated' if you can hear me")
	testCmd.Env = filteredEnv

	output, err := testCmd.Output()
	if err != nil {
		return fmt.Errorf("authentication failed. Run 'claude login' to authenticate with your Claude subscription: %w", err)
	}

	if !strings.Contains(strings.ToLower(string(output)), "authenticated") {
		return fmt.Errorf("unexpected response from Claude, may not be authenticated properly")
	}

	return nil
}
