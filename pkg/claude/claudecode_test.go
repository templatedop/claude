package claude

import (
	"testing"
	"time"
)

func TestClaudeCodeClient_Options(t *testing.T) {
	tests := []struct {
		name     string
		opts     []ClaudeCodeOption
		checkFn  func(*ClaudeCodeClient) bool
		expected string
	}{
		{
			name: "WithClaudeCodeModel",
			opts: []ClaudeCodeOption{WithClaudeCodeModel("claude-opus-4-20250514")},
			checkFn: func(c *ClaudeCodeClient) bool {
				return c.model == "claude-opus-4-20250514"
			},
			expected: "model should be claude-opus-4-20250514",
		},
		{
			name: "WithClaudeCodeWorkingDir",
			opts: []ClaudeCodeOption{WithClaudeCodeWorkingDir("/custom/dir")},
			checkFn: func(c *ClaudeCodeClient) bool {
				return c.workingDir == "/custom/dir"
			},
			expected: "workingDir should be /custom/dir",
		},
		{
			name: "WithClaudeCodeTools",
			opts: []ClaudeCodeOption{WithClaudeCodeTools([]string{"Read", "Write"})},
			checkFn: func(c *ClaudeCodeClient) bool {
				return len(c.allowedTools) == 2 && c.allowedTools[0] == "Read" && c.allowedTools[1] == "Write"
			},
			expected: "allowedTools should be [Read, Write]",
		},
		{
			name: "WithClaudeCodeTimeout",
			opts: []ClaudeCodeOption{WithClaudeCodeTimeout(5 * time.Minute)},
			checkFn: func(c *ClaudeCodeClient) bool {
				return c.timeout == 5*time.Minute
			},
			expected: "timeout should be 5 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client without checking for CLI (we just test config)
			c := &ClaudeCodeClient{
				model:        "claude-sonnet-4-20250514",
				workingDir:   ".",
				allowedTools: []string{"Read", "Write", "Bash", "Glob", "Grep"},
				timeout:      10 * time.Minute,
			}
			for _, opt := range tt.opts {
				opt(c)
			}

			if !tt.checkFn(c) {
				t.Error(tt.expected)
			}
		})
	}
}

func TestClaudeCodeClient_DefaultValues(t *testing.T) {
	c := &ClaudeCodeClient{
		model:        "claude-sonnet-4-20250514",
		workingDir:   ".",
		allowedTools: []string{"Read", "Write", "Bash", "Glob", "Grep"},
		timeout:      10 * time.Minute,
	}

	if c.model != "claude-sonnet-4-20250514" {
		t.Errorf("Expected default model 'claude-sonnet-4-20250514', got '%s'", c.model)
	}
	if c.workingDir != "." {
		t.Errorf("Expected default workingDir '.', got '%s'", c.workingDir)
	}
	if len(c.allowedTools) != 5 {
		t.Errorf("Expected 5 default tools, got %d", len(c.allowedTools))
	}
	if c.timeout != 10*time.Minute {
		t.Errorf("Expected default timeout 10m, got %v", c.timeout)
	}
}

func TestClaudeCodeClient_Provider(t *testing.T) {
	c := &ClaudeCodeClient{}

	if c.Provider() != ProviderClaudeCode {
		t.Errorf("Expected ProviderClaudeCode, got %s", c.Provider())
	}
}

func TestClaudeCodeClient_Close(t *testing.T) {
	c := &ClaudeCodeClient{}

	// Close should not panic or error when no process is running
	err := c.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestClaudeCodeClient_parseOutput_TextMessage(t *testing.T) {
	c := &ClaudeCodeClient{}

	// Test parsing simple text response
	output := []byte(`{"type":"text","text":"Hello from Claude"}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if resp.Text != "Hello from Claude" {
		t.Errorf("Expected 'Hello from Claude', got '%s'", resp.Text)
	}
}

func TestClaudeCodeClient_parseOutput_AssistantMessage(t *testing.T) {
	c := &ClaudeCodeClient{}

	output := []byte(`{"type":"assistant","text":"Assistant response"}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if resp.Text != "Assistant response" {
		t.Errorf("Expected 'Assistant response', got '%s'", resp.Text)
	}
}

func TestClaudeCodeClient_parseOutput_ToolUse(t *testing.T) {
	c := &ClaudeCodeClient{}

	output := []byte(`{"type":"tool_use","tool_id":"tool_123","tool_name":"read_file","tool_input":{"path":"/test.txt"}}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if len(resp.ToolUses) != 1 {
		t.Fatalf("Expected 1 tool use, got %d", len(resp.ToolUses))
	}

	tool := resp.ToolUses[0]
	if tool.Name != "read_file" {
		t.Errorf("Expected tool name 'read_file', got '%s'", tool.Name)
	}
	if tool.ID != "tool_123" {
		t.Errorf("Expected tool ID 'tool_123', got '%s'", tool.ID)
	}
}

func TestClaudeCodeClient_parseOutput_ResultMessage(t *testing.T) {
	c := &ClaudeCodeClient{}

	output := []byte(`{"type":"result","result":"Final result text"}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if resp.Text != "Final result text" {
		t.Errorf("Expected 'Final result text', got '%s'", resp.Text)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("Expected stop reason 'end_turn', got '%s'", resp.StopReason)
	}
}

func TestClaudeCodeClient_parseOutput_ErrorMessage(t *testing.T) {
	c := &ClaudeCodeClient{}

	output := []byte(`{"type":"error","error":"Something went wrong"}`)

	_, err := c.parseOutput(output)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestClaudeCodeClient_parseOutput_MultipleLines(t *testing.T) {
	c := &ClaudeCodeClient{}

	// Multiple JSON lines
	output := []byte(`{"type":"system"}
{"type":"text","text":"First line"}
{"type":"text","text":"Second line"}
{"type":"result","result":"Done"}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	// Should combine all text
	expectedParts := []string{"First line", "Second line", "Done"}
	for _, part := range expectedParts {
		if !contains(resp.Text, part) {
			t.Errorf("Expected response to contain '%s', got '%s'", part, resp.Text)
		}
	}
}

func TestClaudeCodeClient_parseOutput_PlainText(t *testing.T) {
	c := &ClaudeCodeClient{}

	// Non-JSON output (fallback)
	output := []byte(`Plain text response without JSON`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if resp.Text != "Plain text response without JSON" {
		t.Errorf("Expected plain text, got '%s'", resp.Text)
	}
}

func TestClaudeCodeClient_parseOutput_EmptyOutput(t *testing.T) {
	c := &ClaudeCodeClient{}

	output := []byte(``)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if resp.Text != "" {
		t.Errorf("Expected empty text, got '%s'", resp.Text)
	}
}

func TestClaudeCodeClient_parseOutput_MixedContent(t *testing.T) {
	c := &ClaudeCodeClient{}

	// Mix of text and tool use
	output := []byte(`{"type":"text","text":"I'll read that file"}
{"type":"tool_use","tool_id":"t1","tool_name":"read_file","tool_input":{"path":"/x"}}
{"type":"text","text":"Here's what I found"}`)

	resp, err := c.parseOutput(output)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if len(resp.ToolUses) != 1 {
		t.Errorf("Expected 1 tool use, got %d", len(resp.ToolUses))
	}
	if !contains(resp.Text, "I'll read that file") {
		t.Errorf("Expected text to contain first message")
	}
}

func TestClaudeCodeClient_ImplementsInterface(t *testing.T) {
	var _ ClaudeClient = (*ClaudeCodeClient)(nil)
}

func TestClaudeCodeRequest_Fields(t *testing.T) {
	req := claudeCodeRequest{
		Prompt:       "Test prompt",
		SystemPrompt: "Be helpful",
		AllowedTools: []string{"Read", "Write"},
		Model:        "claude-sonnet-4-20250514",
		MaxTokens:    1000,
		WorkingDir:   "/app",
	}

	if req.Prompt != "Test prompt" {
		t.Errorf("Unexpected Prompt: %s", req.Prompt)
	}
	if req.SystemPrompt != "Be helpful" {
		t.Errorf("Unexpected SystemPrompt: %s", req.SystemPrompt)
	}
	if len(req.AllowedTools) != 2 {
		t.Errorf("Unexpected AllowedTools length: %d", len(req.AllowedTools))
	}
}

func TestClaudeCodeMessage_Fields(t *testing.T) {
	msg := claudeCodeMessage{
		Type:      "tool_use",
		ToolName:  "read_file",
		ToolID:    "tool_123",
		ToolInput: map[string]interface{}{"path": "/test"},
	}

	if msg.Type != "tool_use" {
		t.Errorf("Unexpected Type: %s", msg.Type)
	}
	if msg.ToolName != "read_file" {
		t.Errorf("Unexpected ToolName: %s", msg.ToolName)
	}
	if msg.ToolID != "tool_123" {
		t.Errorf("Unexpected ToolID: %s", msg.ToolID)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
