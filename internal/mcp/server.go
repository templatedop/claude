// Package mcp implements the Model Context Protocol (MCP) server.
// MCP is Anthropic's protocol for providing tools and resources to LLMs.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Protocol constants
const (
	ProtocolVersion = "2024-11-05"
	JSONRPCVersion  = "2.0"
)

// Message types
const (
	MethodInitialize           = "initialize"
	MethodInitialized          = "notifications/initialized"
	MethodToolsList            = "tools/list"
	MethodToolsCall            = "tools/call"
	MethodResourcesList        = "resources/list"
	MethodResourcesRead        = "resources/read"
	MethodPromptsList          = "prompts/list"
	MethodPromptsGet           = "prompts/get"
	MethodLoggingSetLevel      = "logging/setLevel"
	MethodNotificationProgress = "notifications/progress"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error codes
const (
	ErrorCodeParse          = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternal       = -32603
)

// ServerInfo contains server information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities describes what the server supports.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
}

// ToolsCapability indicates tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates resource support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates prompt support.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability indicates logging support.
type LoggingCapability struct{}

// Tool represents an MCP tool.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the JSON Schema for tool input.
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]Property    `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// Property defines a JSON Schema property.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents content in a tool result.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // Base64 encoded for binary
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent represents the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // Base64 encoded
}

// Prompt represents an MCP prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument defines an argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    string         `json:"role"`
	Content ContentBlock   `json:"content"`
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, params map[string]interface{}) (*ToolResult, error)

// ResourceHandler is a function that handles a resource read.
type ResourceHandler func(ctx context.Context, uri string) (*ResourceContent, error)

// PromptHandler is a function that handles a prompt get.
type PromptHandler func(ctx context.Context, args map[string]string) ([]PromptMessage, error)

// Server is an MCP server implementation.
type Server struct {
	info         ServerInfo
	capabilities ServerCapabilities

	tools           map[string]Tool
	toolHandlers    map[string]ToolHandler
	resources       map[string]Resource
	resourceHandler ResourceHandler
	prompts         map[string]Prompt
	promptHandlers  map[string]PromptHandler

	mu sync.RWMutex

	// Input/output for stdio transport
	input  io.Reader
	output io.Writer
}

// NewServer creates a new MCP server.
func NewServer(name, version string) *Server {
	return &Server{
		info: ServerInfo{
			Name:    name,
			Version: version,
		},
		capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
			Prompts:   &PromptsCapability{},
			Logging:   &LoggingCapability{},
		},
		tools:          make(map[string]Tool),
		toolHandlers:   make(map[string]ToolHandler),
		resources:      make(map[string]Resource),
		prompts:        make(map[string]Prompt),
		promptHandlers: make(map[string]PromptHandler),
	}
}

// RegisterTool registers a tool with the server.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.toolHandlers[tool.Name] = handler
}

// RegisterResource registers a resource with the server.
func (s *Server) RegisterResource(resource Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[resource.URI] = resource
}

// SetResourceHandler sets the handler for reading resources.
func (s *Server) SetResourceHandler(handler ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resourceHandler = handler
}

// RegisterPrompt registers a prompt with the server.
func (s *Server) RegisterPrompt(prompt Prompt, handler PromptHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts[prompt.Name] = prompt
	s.promptHandlers[prompt.Name] = handler
}

// HandleRequest processes a JSON-RPC request and returns a response.
func (s *Server) HandleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case MethodInitialize:
		return s.handleInitialize(ctx, req)
	case MethodInitialized:
		// Notification, no response needed
		return nil
	case MethodToolsList:
		return s.handleToolsList(ctx, req)
	case MethodToolsCall:
		return s.handleToolsCall(ctx, req)
	case MethodResourcesList:
		return s.handleResourcesList(ctx, req)
	case MethodResourcesRead:
		return s.handleResourcesRead(ctx, req)
	case MethodPromptsList:
		return s.handlePromptsList(ctx, req)
	case MethodPromptsGet:
		return s.handlePromptsGet(ctx, req)
	case MethodLoggingSetLevel:
		return s.handleLoggingSetLevel(ctx, req)
	default:
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": ProtocolVersion,
			"capabilities":    s.capabilities,
			"serverInfo":      s.info,
		},
	}
}

func (s *Server) handleToolsList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInvalidParams,
				Message: "Invalid parameters",
				Data:    err.Error(),
			},
		}
	}

	s.mu.RLock()
	handler, ok := s.toolHandlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeMethodNotFound,
				Message: fmt.Sprintf("Tool not found: %s", params.Name),
			},
		}
	}

	result, err := handler(ctx, params.Arguments)
	if err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Result: &ToolResult{
				Content: []ContentBlock{{Type: "text", Text: err.Error()}},
				IsError: true,
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleResourcesList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]Resource, 0, len(s.resources))
	for _, res := range s.resources {
		resources = append(resources, res)
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"resources": resources,
		},
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInvalidParams,
				Message: "Invalid parameters",
			},
		}
	}

	s.mu.RLock()
	handler := s.resourceHandler
	s.mu.RUnlock()

	if handler == nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternal,
				Message: "No resource handler configured",
			},
		}
	}

	content, err := handler(ctx, params.URI)
	if err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternal,
				Message: err.Error(),
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"contents": []ResourceContent{*content},
		},
	}
}

func (s *Server) handlePromptsList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prompts := make([]Prompt, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		prompts = append(prompts, prompt)
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"prompts": prompts,
		},
	}
}

func (s *Server) handlePromptsGet(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInvalidParams,
				Message: "Invalid parameters",
			},
		}
	}

	s.mu.RLock()
	handler, ok := s.promptHandlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeMethodNotFound,
				Message: fmt.Sprintf("Prompt not found: %s", params.Name),
			},
		}
	}

	messages, err := handler(ctx, params.Arguments)
	if err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternal,
				Message: err.Error(),
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result: map[string]interface{}{
			"messages": messages,
		},
	}
}

func (s *Server) handleLoggingSetLevel(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	// Acknowledge the logging level change
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result:  map[string]interface{}{},
	}
}

// TextContent creates a text content block.
func TextContent(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// ErrorResult creates an error tool result.
func ErrorResult(err error) *ToolResult {
	return &ToolResult{
		Content: []ContentBlock{TextContent(err.Error())},
		IsError: true,
	}
}

// SuccessResult creates a successful tool result.
func SuccessResult(text string) *ToolResult {
	return &ToolResult{
		Content: []ContentBlock{TextContent(text)},
	}
}

// JSONResult creates a tool result with JSON content.
func JSONResult(v interface{}) (*ToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &ToolResult{
		Content: []ContentBlock{TextContent(string(data))},
	}, nil
}
