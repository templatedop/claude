package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvider_String(t *testing.T) {
	tests := []struct {
		provider Provider
		expected string
	}{
		{ProviderAPI, "api"},
		{ProviderClaudeCode, "claude_code"},
	}

	for _, tt := range tests {
		if string(tt.provider) != tt.expected {
			t.Errorf("Provider string: got %s, want %s", tt.provider, tt.expected)
		}
	}
}

func TestCompletionRequest_Defaults(t *testing.T) {
	req := CompletionRequest{}

	// Empty request should have zero values
	if req.System != "" {
		t.Error("Expected empty System")
	}
	if req.Prompt != "" {
		t.Error("Expected empty Prompt")
	}
	if req.MaxTokens != 0 {
		t.Error("Expected zero MaxTokens")
	}
	if req.Temperature != 0 {
		t.Error("Expected zero Temperature")
	}
}

func TestCompletionResponse_Fields(t *testing.T) {
	resp := CompletionResponse{
		Text:         "Hello!",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  10,
		OutputTokens: 5,
		StopReason:   "end_turn",
		ToolUses:     []ToolUse{{Type: "tool_use", ID: "123", Name: "read"}},
	}

	if resp.Text != "Hello!" {
		t.Errorf("Expected text 'Hello!', got '%s'", resp.Text)
	}
	if resp.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Unexpected model: %s", resp.Model)
	}
	if resp.InputTokens != 10 {
		t.Errorf("Expected 10 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("Expected 5 output tokens, got %d", resp.OutputTokens)
	}
	if len(resp.ToolUses) != 1 {
		t.Errorf("Expected 1 tool use, got %d", len(resp.ToolUses))
	}
}

func TestClient_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var req MessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		// Check that prompt was converted to message
		if len(req.Messages) == 0 {
			t.Error("Expected at least one message")
		}

		// Return mock response
		resp := MessageResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: req.Model,
			Content: []ContentBlock{
				{Type: "text", Text: "Completion response"},
			},
			StopReason: "end_turn",
			Usage: Usage{
				InputTokens:  15,
				OutputTokens: 8,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Test Complete method from interface
	req := CompletionRequest{
		System:      "You are helpful",
		Prompt:      "Hello",
		Model:       DefaultModel,
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.Text != "Completion response" {
		t.Errorf("Expected 'Completion response', got '%s'", resp.Text)
	}
	if resp.InputTokens != 15 {
		t.Errorf("Expected 15 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 8 {
		t.Errorf("Expected 8 output tokens, got %d", resp.OutputTokens)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("Expected stop reason 'end_turn', got '%s'", resp.StopReason)
	}
}

func TestClient_Complete_WithMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MessageRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Should have messages from history
		if len(req.Messages) < 2 {
			t.Errorf("Expected at least 2 messages, got %d", len(req.Messages))
		}

		resp := MessageResponse{
			Content: []ContentBlock{{Type: "text", Text: "OK"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	req := CompletionRequest{
		Prompt: "Hello",
		Messages: []Message{
			NewTextMessage(RoleUser, "Previous message"),
			NewTextMessage(RoleAssistant, "Previous response"),
		},
	}

	_, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
}

func TestClient_Complete_UsesDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MessageRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Check defaults were applied
		if req.Model != DefaultModel {
			t.Errorf("Expected default model '%s', got '%s'", DefaultModel, req.Model)
		}
		if req.MaxTokens != DefaultMaxTokens {
			t.Errorf("Expected default max tokens %d, got %d", DefaultMaxTokens, req.MaxTokens)
		}

		resp := MessageResponse{
			Content: []ContentBlock{{Type: "text", Text: "OK"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	// Request without model and max tokens - should use defaults
	req := CompletionRequest{
		Prompt: "Hello",
	}

	_, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
}

func TestClient_SimpleComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MessageRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify system prompt
		if req.System != "System prompt" {
			t.Errorf("Expected system 'System prompt', got '%s'", req.System)
		}

		resp := MessageResponse{
			Content: []ContentBlock{{Type: "text", Text: "Simple response"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	text, err := client.SimpleComplete(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("SimpleComplete failed: %v", err)
	}

	if text != "Simple response" {
		t.Errorf("Expected 'Simple response', got '%s'", text)
	}
}

func TestClient_Provider(t *testing.T) {
	client, _ := NewClient("test-key")

	if client.Provider() != ProviderAPI {
		t.Errorf("Expected ProviderAPI, got %s", client.Provider())
	}
}

func TestClient_Close(t *testing.T) {
	client, _ := NewClient("test-key")

	// Close should not return an error for API client
	err := client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestClient_ImplementsInterface(t *testing.T) {
	var _ ClaudeClient = (*Client)(nil)
}

func TestClient_Complete_WithToolUses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := MessageResponse{
			Content: []ContentBlock{
				{Type: "text", Text: "I'll read that file"},
				{Type: "tool_use", ID: "tool_1", Name: "read_file", Input: map[string]interface{}{"path": "/test.txt"}},
			},
			StopReason: "tool_use",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	req := CompletionRequest{Prompt: "Read the test file"}
	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if len(resp.ToolUses) != 1 {
		t.Errorf("Expected 1 tool use, got %d", len(resp.ToolUses))
	}
	if resp.ToolUses[0].Name != "read_file" {
		t.Errorf("Expected tool name 'read_file', got '%s'", resp.ToolUses[0].Name)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("Expected stop reason 'tool_use', got '%s'", resp.StopReason)
	}
}
