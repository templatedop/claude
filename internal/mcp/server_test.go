package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewServer(t *testing.T) {
	server := NewServer("test-server", "1.0.0")

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.info.Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", server.info.Name)
	}

	if server.info.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", server.info.Version)
	}
}

func TestRegisterTool(t *testing.T) {
	server := NewServer("test", "1.0.0")

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"input": {Type: "string", Description: "Test input"},
			},
			Required: []string{"input"},
		},
	}

	handler := func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return SuccessResult("test output"), nil
	}

	server.RegisterTool(tool, handler)

	if _, ok := server.tools["test_tool"]; !ok {
		t.Error("Tool was not registered")
	}

	if _, ok := server.toolHandlers["test_tool"]; !ok {
		t.Error("Tool handler was not registered")
	}
}

func TestRegisterResource(t *testing.T) {
	server := NewServer("test", "1.0.0")

	resource := Resource{
		URI:         "test://resource",
		Name:        "Test Resource",
		Description: "A test resource",
		MimeType:    "text/plain",
	}

	server.RegisterResource(resource)

	if _, ok := server.resources["test://resource"]; !ok {
		t.Error("Resource was not registered")
	}
}

func TestRegisterPrompt(t *testing.T) {
	server := NewServer("test", "1.0.0")

	prompt := Prompt{
		Name:        "test_prompt",
		Description: "A test prompt",
		Arguments: []PromptArgument{
			{Name: "arg1", Description: "First argument", Required: true},
		},
	}

	handler := func(ctx context.Context, args map[string]string) ([]PromptMessage, error) {
		return []PromptMessage{
			{Role: "user", Content: ContentBlock{Type: "text", Text: "Test"}},
		}, nil
	}

	server.RegisterPrompt(prompt, handler)

	if _, ok := server.prompts["test_prompt"]; !ok {
		t.Error("Prompt was not registered")
	}

	if _, ok := server.promptHandlers["test_prompt"]; !ok {
		t.Error("Prompt handler was not registered")
	}
}

func TestHandleRequest_Initialize(t *testing.T) {
	server := NewServer("test-server", "1.0.0")

	req := &JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  MethodInitialize,
	}

	resp := server.HandleRequest(context.Background(), req)

	if resp == nil {
		t.Fatal("Response is nil")
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("Expected protocol version %s, got %v", ProtocolVersion, result["protocolVersion"])
	}
}

func TestHandleRequest_ToolsList(t *testing.T) {
	server := NewServer("test", "1.0.0")

	server.RegisterTool(Tool{
		Name:        "tool1",
		Description: "First tool",
	}, func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return nil, nil
	})

	server.RegisterTool(Tool{
		Name:        "tool2",
		Description: "Second tool",
	}, func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return nil, nil
	})

	req := &JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  MethodToolsList,
	}

	resp := server.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Result is not a map")
	}

	tools, ok := result["tools"].([]Tool)
	if !ok {
		t.Fatal("Tools is not a slice")
	}

	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
}

func TestHandleRequest_ToolsCall(t *testing.T) {
	server := NewServer("test", "1.0.0")

	server.RegisterTool(Tool{
		Name:        "echo",
		Description: "Echo tool",
	}, func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		msg, _ := params["message"].(string)
		return SuccessResult("Echo: " + msg), nil
	})

	params, _ := json.Marshal(map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]interface{}{"message": "hello"},
	})

	req := &JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  MethodToolsCall,
		Params:  params,
	}

	resp := server.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(*ToolResult)
	if !ok {
		t.Fatal("Result is not a ToolResult")
	}

	if len(result.Content) == 0 {
		t.Fatal("Result has no content")
	}

	if result.Content[0].Text != "Echo: hello" {
		t.Errorf("Expected 'Echo: hello', got '%s'", result.Content[0].Text)
	}
}

func TestHandleRequest_MethodNotFound(t *testing.T) {
	server := NewServer("test", "1.0.0")

	req := &JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "unknown/method",
	}

	resp := server.HandleRequest(context.Background(), req)

	if resp.Error == nil {
		t.Fatal("Expected error for unknown method")
	}

	if resp.Error.Code != ErrorCodeMethodNotFound {
		t.Errorf("Expected error code %d, got %d", ErrorCodeMethodNotFound, resp.Error.Code)
	}
}

func TestTextContent(t *testing.T) {
	content := TextContent("test message")

	if content.Type != "text" {
		t.Errorf("Expected type 'text', got '%s'", content.Type)
	}

	if content.Text != "test message" {
		t.Errorf("Expected text 'test message', got '%s'", content.Text)
	}
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult(context.DeadlineExceeded)

	if !result.IsError {
		t.Error("Expected IsError to be true")
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected content in error result")
	}
}

func TestSuccessResult(t *testing.T) {
	result := SuccessResult("success message")

	if result.IsError {
		t.Error("Expected IsError to be false")
	}

	if result.Content[0].Text != "success message" {
		t.Errorf("Expected 'success message', got '%s'", result.Content[0].Text)
	}
}

func TestJSONResult(t *testing.T) {
	data := map[string]string{"key": "value"}
	result, err := JSONResult(data)

	if err != nil {
		t.Fatalf("JSONResult returned error: %v", err)
	}

	if result.IsError {
		t.Error("Expected IsError to be false")
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected content in result")
	}
}
