package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	// Test with empty API key
	_, err := NewClient("")
	if err != ErrNoAPIKey {
		t.Errorf("Expected ErrNoAPIKey, got %v", err)
	}

	// Test with valid API key
	client, err := NewClient("test-api-key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Error("Expected non-nil client")
	}
}

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage(RoleUser, "Hello, Claude!")

	if msg.Role != RoleUser {
		t.Errorf("Expected role 'user', got '%s'", msg.Role)
	}

	if len(msg.Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(msg.Content))
	}

	if msg.Content[0].Type != ContentTypeText {
		t.Errorf("Expected content type 'text', got '%s'", msg.Content[0].Type)
	}

	if msg.Content[0].Text != "Hello, Claude!" {
		t.Errorf("Expected text 'Hello, Claude!', got '%s'", msg.Content[0].Text)
	}
}

func TestMessageResponse_GetText(t *testing.T) {
	resp := &MessageResponse{
		Content: []ContentBlock{
			{Type: "text", Text: "Hello!"},
			{Type: "tool_use", Name: "some_tool"},
		},
	}

	text := resp.GetText()
	if text != "Hello!" {
		t.Errorf("Expected 'Hello!', got '%s'", text)
	}

	// Empty response
	emptyResp := &MessageResponse{Content: []ContentBlock{}}
	if emptyResp.GetText() != "" {
		t.Error("Expected empty string for empty response")
	}
}

func TestMessageResponse_GetToolUses(t *testing.T) {
	resp := &MessageResponse{
		Content: []ContentBlock{
			{Type: "text", Text: "Hello!"},
			{Type: "tool_use", ID: "tool1", Name: "read_file", Input: map[string]interface{}{"path": "/test"}},
			{Type: "tool_use", ID: "tool2", Name: "write_file", Input: map[string]interface{}{"path": "/out"}},
		},
	}

	tools := resp.GetToolUses()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tool uses, got %d", len(tools))
	}

	if tools[0].Name != "read_file" {
		t.Errorf("Expected first tool 'read_file', got '%s'", tools[0].Name)
	}

	if tools[1].Name != "write_file" {
		t.Errorf("Expected second tool 'write_file', got '%s'", tools[1].Name)
	}
}

func TestClient_CreateMessage(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("Expected API key 'test-key', got '%s'", r.Header.Get("X-API-Key"))
		}

		if r.Header.Get("anthropic-version") != APIVersion {
			t.Errorf("Expected version '%s', got '%s'", APIVersion, r.Header.Get("anthropic-version"))
		}

		// Parse request body
		var req MessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if len(req.Messages) == 0 {
			t.Error("Expected at least one message")
		}

		// Return mock response
		resp := MessageResponse{
			ID:     "msg_123",
			Type:   "message",
			Role:   "assistant",
			Model:  req.Model,
			Content: []ContentBlock{
				{Type: "text", Text: "Hello! I'm Claude."},
			},
			StopReason: "end_turn",
			Usage: Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client with mock server
	client, err := NewClient("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Test CreateMessage
	req := MessageRequest{
		Model:     DefaultModel,
		MaxTokens: 100,
		Messages:  []Message{NewTextMessage(RoleUser, "Hi")},
	}

	resp, err := client.CreateMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	if resp.GetText() != "Hello! I'm Claude." {
		t.Errorf("Unexpected response text: %s", resp.GetText())
	}

	if resp.Usage.InputTokens != 10 {
		t.Errorf("Expected 10 input tokens, got %d", resp.Usage.InputTokens)
	}

	if resp.StopReason != "end_turn" {
		t.Errorf("Expected stop_reason 'end_turn', got '%s'", resp.StopReason)
	}
}

func TestClient_CreateMessage_EmptyMessages(t *testing.T) {
	client, _ := NewClient("test-key")

	req := MessageRequest{
		Model:     DefaultModel,
		MaxTokens: 100,
		Messages:  []Message{},
	}

	_, err := client.CreateMessage(context.Background(), req)
	if err != ErrEmptyMessage {
		t.Errorf("Expected ErrEmptyMessage, got %v", err)
	}
}

func TestClient_CreateMessage_DefaultValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MessageRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Check defaults were applied
		if req.Model != DefaultModel {
			t.Errorf("Expected default model '%s', got '%s'", DefaultModel, req.Model)
		}

		if req.MaxTokens != DefaultMaxTokens {
			t.Errorf("Expected default max_tokens %d, got %d", DefaultMaxTokens, req.MaxTokens)
		}

		resp := MessageResponse{
			ID:      "msg_123",
			Content: []ContentBlock{{Type: "text", Text: "OK"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	req := MessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "Hi")},
		// Model and MaxTokens not set - should use defaults
	}

	_, err := client.CreateMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}
}

func TestClient_CreateMessage_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Type: "error",
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{
				Type:    "invalid_request_error",
				Message: "Invalid request",
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	req := MessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "Hi")},
	}

	_, err := client.CreateMessage(context.Background(), req)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestClient_CreateMessage_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	req := MessageRequest{
		Messages: []Message{NewTextMessage(RoleUser, "Hi")},
	}

	_, err := client.CreateMessage(context.Background(), req)
	if err != ErrRateLimited {
		t.Errorf("Expected ErrRateLimited, got %v", err)
	}
}

func TestConversation(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req MessageRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Check that history accumulates
		if callCount == 2 && len(req.Messages) != 3 {
			t.Errorf("Expected 3 messages in second call, got %d", len(req.Messages))
		}

		resp := MessageResponse{
			Content: []ContentBlock{{Type: "text", Text: "Response " + string(rune('0'+callCount))}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	conv := client.NewConversation("You are a helpful assistant")
	conv.SetModel(DefaultModel)
	conv.SetMaxTokens(100)

	// First message
	resp1, err := conv.Say(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("First message failed: %v", err)
	}

	if resp1 != "Response 1" {
		t.Errorf("Expected 'Response 1', got '%s'", resp1)
	}

	// Second message - should include history
	resp2, err := conv.Say(context.Background(), "How are you?")
	if err != nil {
		t.Fatalf("Second message failed: %v", err)
	}

	if resp2 != "Response 2" {
		t.Errorf("Expected 'Response 2', got '%s'", resp2)
	}

	// Check history
	history := conv.GetHistory()
	if len(history) != 4 { // 2 user + 2 assistant
		t.Errorf("Expected 4 messages in history, got %d", len(history))
	}

	// Reset and check
	conv.Reset()
	if len(conv.GetHistory()) != 0 {
		t.Error("Expected empty history after reset")
	}
}
