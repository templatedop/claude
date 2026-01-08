// Package lsp implements a Language Server Protocol server.
// LSP provides code intelligence features like completion, diagnostics, and navigation.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Protocol constants
const (
	ContentTypeHeader = "Content-Type: application/vscode-jsonrpc; charset=utf-8"
)

// Message types
const (
	MethodInitialize            = "initialize"
	MethodInitialized           = "initialized"
	MethodShutdown              = "shutdown"
	MethodExit                  = "exit"
	MethodTextDocumentDidOpen   = "textDocument/didOpen"
	MethodTextDocumentDidChange = "textDocument/didChange"
	MethodTextDocumentDidClose  = "textDocument/didClose"
	MethodTextDocumentDidSave   = "textDocument/didSave"
	MethodTextDocumentHover     = "textDocument/hover"
	MethodTextDocumentCompletion = "textDocument/completion"
	MethodTextDocumentDefinition = "textDocument/definition"
	MethodTextDocumentReferences = "textDocument/references"
	MethodTextDocumentSymbol     = "textDocument/documentSymbol"
	MethodWorkspaceSymbol        = "workspace/symbol"
	MethodTextDocumentDiagnostic = "textDocument/diagnostic"
	MethodPublishDiagnostics     = "textDocument/publishDiagnostics"
)

// Message represents a JSON-RPC message.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError represents an error response.
type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error codes
const (
	ParseError           = -32700
	InvalidRequest       = -32600
	MethodNotFound       = -32601
	InvalidParams        = -32602
	InternalError        = -32603
	ServerNotInitialized = -32002
	RequestCancelled     = -32800
)

// Position represents a position in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location in a document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem represents a text document.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams represents params for position-based requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// InitializeParams represents initialize request params.
type InitializeParams struct {
	ProcessID    int                `json:"processId"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities represents client capabilities.
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

// TextDocumentClientCapabilities represents text document capabilities.
type TextDocumentClientCapabilities struct {
	Completion CompletionClientCapabilities `json:"completion,omitempty"`
	Hover      HoverClientCapabilities      `json:"hover,omitempty"`
}

// CompletionClientCapabilities represents completion capabilities.
type CompletionClientCapabilities struct {
	CompletionItem CompletionItemCapabilities `json:"completionItem,omitempty"`
}

// CompletionItemCapabilities represents completion item capabilities.
type CompletionItemCapabilities struct {
	SnippetSupport bool `json:"snippetSupport,omitempty"`
}

// HoverClientCapabilities represents hover capabilities.
type HoverClientCapabilities struct {
	ContentFormat []string `json:"contentFormat,omitempty"`
}

// WorkspaceClientCapabilities represents workspace capabilities.
type WorkspaceClientCapabilities struct {
	WorkspaceFolders bool `json:"workspaceFolders,omitempty"`
}

// InitializeResult represents initialize response.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo represents server information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities represents what the server supports.
type ServerCapabilities struct {
	TextDocumentSync           int                        `json:"textDocumentSync"`
	HoverProvider              bool                       `json:"hoverProvider"`
	CompletionProvider         *CompletionOptions         `json:"completionProvider,omitempty"`
	DefinitionProvider         bool                       `json:"definitionProvider"`
	ReferencesProvider         bool                       `json:"referencesProvider"`
	DocumentSymbolProvider     bool                       `json:"documentSymbolProvider"`
	WorkspaceSymbolProvider    bool                       `json:"workspaceSymbolProvider"`
	DiagnosticProvider         *DiagnosticOptions         `json:"diagnosticProvider,omitempty"`
}

// CompletionOptions represents completion options.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
}

// DiagnosticOptions represents diagnostic options.
type DiagnosticOptions struct {
	Identifier            string `json:"identifier,omitempty"`
	InterFileDependencies bool   `json:"interFileDependencies"`
	WorkspaceDiagnostics  bool   `json:"workspaceDiagnostics"`
}

// Hover represents hover information.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent represents markup content.
type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// CompletionItem represents a completion item.
type CompletionItem struct {
	Label         string        `json:"label"`
	Kind          int           `json:"kind,omitempty"`
	Detail        string        `json:"detail,omitempty"`
	Documentation MarkupContent `json:"documentation,omitempty"`
	InsertText    string        `json:"insertText,omitempty"`
	SortText      string        `json:"sortText,omitempty"`
}

// CompletionItemKind constants
const (
	CompletionKindText          = 1
	CompletionKindMethod        = 2
	CompletionKindFunction      = 3
	CompletionKindConstructor   = 4
	CompletionKindField         = 5
	CompletionKindVariable      = 6
	CompletionKindClass         = 7
	CompletionKindInterface     = 8
	CompletionKindModule        = 9
	CompletionKindProperty      = 10
	CompletionKindUnit          = 11
	CompletionKindValue         = 12
	CompletionKindEnum          = 13
	CompletionKindKeyword       = 14
	CompletionKindSnippet       = 15
	CompletionKindColor         = 16
	CompletionKindFile          = 17
	CompletionKindReference     = 18
	CompletionKindFolder        = 19
	CompletionKindEnumMember    = 20
	CompletionKindConstant      = 21
	CompletionKindStruct        = 22
	CompletionKindEvent         = 23
	CompletionKindOperator      = 24
	CompletionKindTypeParameter = 25
)

// CompletionList represents a list of completion items.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// DocumentSymbol represents a symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolKind constants
const (
	SymbolKindFile          = 1
	SymbolKindModule        = 2
	SymbolKindNamespace     = 3
	SymbolKindPackage       = 4
	SymbolKindClass         = 5
	SymbolKindMethod        = 6
	SymbolKindProperty      = 7
	SymbolKindField         = 8
	SymbolKindConstructor   = 9
	SymbolKindEnum          = 10
	SymbolKindInterface     = 11
	SymbolKindFunction      = 12
	SymbolKindVariable      = 13
	SymbolKindConstant      = 14
	SymbolKindString        = 15
	SymbolKindNumber        = 16
	SymbolKindBoolean       = 17
	SymbolKindArray         = 18
	SymbolKindObject        = 19
	SymbolKindKey           = 20
	SymbolKindNull          = 21
	SymbolKindEnumMember    = 22
	SymbolKindStruct        = 23
	SymbolKindEvent         = 24
	SymbolKindOperator      = 25
	SymbolKindTypeParameter = 26
)

// SymbolInformation represents a symbol in the workspace.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// Diagnostic represents a diagnostic message.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// DiagnosticSeverity constants
const (
	DiagnosticSeverityError       = 1
	DiagnosticSeverityWarning     = 2
	DiagnosticSeverityInformation = 3
	DiagnosticSeverityHint        = 4
)

// PublishDiagnosticsParams represents diagnostics publish params.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Document represents an open document.
type Document struct {
	URI        string
	LanguageID string
	Version    int
	Content    string
	Lines      []string
}

// Server is an LSP server implementation.
type Server struct {
	info          ServerInfo
	rootURI       string
	initialized   bool
	shutdownReq   bool

	documents     map[string]*Document
	documentsMu   sync.RWMutex

	// Handlers
	hoverHandler      func(ctx context.Context, doc *Document, pos Position) (*Hover, error)
	completionHandler func(ctx context.Context, doc *Document, pos Position) (*CompletionList, error)
	definitionHandler func(ctx context.Context, doc *Document, pos Position) ([]Location, error)
	referencesHandler func(ctx context.Context, doc *Document, pos Position) ([]Location, error)
	symbolsHandler    func(ctx context.Context, doc *Document) ([]DocumentSymbol, error)
	diagnosticHandler func(ctx context.Context, doc *Document) ([]Diagnostic, error)

	input  io.Reader
	output io.Writer
}

// NewServer creates a new LSP server.
func NewServer(name, version string) *Server {
	return &Server{
		info: ServerInfo{
			Name:    name,
			Version: version,
		},
		documents: make(map[string]*Document),
		input:     os.Stdin,
		output:    os.Stdout,
	}
}

// SetHoverHandler sets the hover handler.
func (s *Server) SetHoverHandler(h func(ctx context.Context, doc *Document, pos Position) (*Hover, error)) {
	s.hoverHandler = h
}

// SetCompletionHandler sets the completion handler.
func (s *Server) SetCompletionHandler(h func(ctx context.Context, doc *Document, pos Position) (*CompletionList, error)) {
	s.completionHandler = h
}

// SetDefinitionHandler sets the definition handler.
func (s *Server) SetDefinitionHandler(h func(ctx context.Context, doc *Document, pos Position) ([]Location, error)) {
	s.definitionHandler = h
}

// SetReferencesHandler sets the references handler.
func (s *Server) SetReferencesHandler(h func(ctx context.Context, doc *Document, pos Position) ([]Location, error)) {
	s.referencesHandler = h
}

// SetSymbolsHandler sets the document symbols handler.
func (s *Server) SetSymbolsHandler(h func(ctx context.Context, doc *Document) ([]DocumentSymbol, error)) {
	s.symbolsHandler = h
}

// SetDiagnosticHandler sets the diagnostic handler.
func (s *Server) SetDiagnosticHandler(h func(ctx context.Context, doc *Document) ([]Diagnostic, error)) {
	s.diagnosticHandler = h
}

// Run starts the LSP server.
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(s.input)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := s.readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			continue
		}

		response := s.handleMessage(ctx, msg)
		if response != nil {
			if err := s.writeMessage(response); err != nil {
				return err
			}
		}

		if s.shutdownReq && msg.Method == MethodExit {
			return nil
		}
	}
}

func (s *Server) readMessage(reader *bufio.Reader) (*Message, error) {
	// Read headers
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// Read content
	content := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(content, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (s *Server) writeMessage(msg *Message) error {
	content, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(content))
	if _, err := io.WriteString(s.output, header); err != nil {
		return err
	}
	if _, err := s.output.Write(content); err != nil {
		return err
	}

	return nil
}

func (s *Server) handleMessage(ctx context.Context, msg *Message) *Message {
	// Check if initialized
	if !s.initialized && msg.Method != MethodInitialize {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ResponseError{
				Code:    ServerNotInitialized,
				Message: "Server not initialized",
			},
		}
	}

	switch msg.Method {
	case MethodInitialize:
		return s.handleInitialize(ctx, msg)
	case MethodInitialized:
		return nil // Notification, no response
	case MethodShutdown:
		return s.handleShutdown(ctx, msg)
	case MethodExit:
		return nil // Notification, no response
	case MethodTextDocumentDidOpen:
		s.handleDidOpen(ctx, msg)
		return nil
	case MethodTextDocumentDidChange:
		s.handleDidChange(ctx, msg)
		return nil
	case MethodTextDocumentDidClose:
		s.handleDidClose(ctx, msg)
		return nil
	case MethodTextDocumentHover:
		return s.handleHover(ctx, msg)
	case MethodTextDocumentCompletion:
		return s.handleCompletion(ctx, msg)
	case MethodTextDocumentDefinition:
		return s.handleDefinition(ctx, msg)
	case MethodTextDocumentReferences:
		return s.handleReferences(ctx, msg)
	case MethodTextDocumentSymbol:
		return s.handleDocumentSymbol(ctx, msg)
	default:
		if msg.ID != nil {
			return &Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Error: &ResponseError{
					Code:    MethodNotFound,
					Message: fmt.Sprintf("Method not found: %s", msg.Method),
				},
			}
		}
		return nil
	}
}

func (s *Server) handleInitialize(ctx context.Context, msg *Message) *Message {
	var params InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ResponseError{
				Code:    InvalidParams,
				Message: err.Error(),
			},
		}
	}

	s.rootURI = params.RootURI
	s.initialized = true

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:        1, // Full sync
			HoverProvider:           s.hoverHandler != nil,
			DefinitionProvider:      s.definitionHandler != nil,
			ReferencesProvider:      s.referencesHandler != nil,
			DocumentSymbolProvider:  s.symbolsHandler != nil,
			WorkspaceSymbolProvider: false,
		},
		ServerInfo: &s.info,
	}

	if s.completionHandler != nil {
		result.Capabilities.CompletionProvider = &CompletionOptions{
			TriggerCharacters: []string{".", ":", "<", "\"", "'", "/"},
		}
	}

	if s.diagnosticHandler != nil {
		result.Capabilities.DiagnosticProvider = &DiagnosticOptions{
			Identifier:            s.info.Name,
			InterFileDependencies: false,
			WorkspaceDiagnostics:  false,
		}
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  result,
	}
}

func (s *Server) handleShutdown(ctx context.Context, msg *Message) *Message {
	s.shutdownReq = true
	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  nil,
	}
}

func (s *Server) handleDidOpen(ctx context.Context, msg *Message) {
	var params struct {
		TextDocument TextDocumentItem `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	doc := &Document{
		URI:        params.TextDocument.URI,
		LanguageID: params.TextDocument.LanguageID,
		Version:    params.TextDocument.Version,
		Content:    params.TextDocument.Text,
		Lines:      strings.Split(params.TextDocument.Text, "\n"),
	}

	s.documentsMu.Lock()
	s.documents[params.TextDocument.URI] = doc
	s.documentsMu.Unlock()

	// Trigger diagnostics
	s.publishDiagnostics(ctx, doc)
}

func (s *Server) handleDidChange(ctx context.Context, msg *Message) {
	var params struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	if len(params.ContentChanges) == 0 {
		return
	}

	s.documentsMu.Lock()
	doc, ok := s.documents[params.TextDocument.URI]
	if ok {
		doc.Version = params.TextDocument.Version
		doc.Content = params.ContentChanges[len(params.ContentChanges)-1].Text
		doc.Lines = strings.Split(doc.Content, "\n")
	}
	s.documentsMu.Unlock()

	if ok {
		s.publishDiagnostics(ctx, doc)
	}
}

func (s *Server) handleDidClose(ctx context.Context, msg *Message) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	s.documentsMu.Lock()
	delete(s.documents, params.TextDocument.URI)
	s.documentsMu.Unlock()

	// Clear diagnostics
	s.writeMessage(&Message{
		JSONRPC: "2.0",
		Method:  MethodPublishDiagnostics,
		Params:  mustMarshal(PublishDiagnosticsParams{URI: params.TextDocument.URI, Diagnostics: []Diagnostic{}}),
	})
}

func (s *Server) handleHover(ctx context.Context, msg *Message) *Message {
	if s.hoverHandler == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InvalidParams, Message: err.Error()},
		}
	}

	s.documentsMu.RLock()
	doc := s.documents[params.TextDocument.URI]
	s.documentsMu.RUnlock()

	if doc == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	hover, err := s.hoverHandler(ctx, doc, params.Position)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InternalError, Message: err.Error()},
		}
	}

	return &Message{JSONRPC: "2.0", ID: msg.ID, Result: hover}
}

func (s *Server) handleCompletion(ctx context.Context, msg *Message) *Message {
	if s.completionHandler == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InvalidParams, Message: err.Error()},
		}
	}

	s.documentsMu.RLock()
	doc := s.documents[params.TextDocument.URI]
	s.documentsMu.RUnlock()

	if doc == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: &CompletionList{}}
	}

	list, err := s.completionHandler(ctx, doc, params.Position)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InternalError, Message: err.Error()},
		}
	}

	return &Message{JSONRPC: "2.0", ID: msg.ID, Result: list}
}

func (s *Server) handleDefinition(ctx context.Context, msg *Message) *Message {
	if s.definitionHandler == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InvalidParams, Message: err.Error()},
		}
	}

	s.documentsMu.RLock()
	doc := s.documents[params.TextDocument.URI]
	s.documentsMu.RUnlock()

	if doc == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	locations, err := s.definitionHandler(ctx, doc, params.Position)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InternalError, Message: err.Error()},
		}
	}

	return &Message{JSONRPC: "2.0", ID: msg.ID, Result: locations}
}

func (s *Server) handleReferences(ctx context.Context, msg *Message) *Message {
	if s.referencesHandler == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InvalidParams, Message: err.Error()},
		}
	}

	s.documentsMu.RLock()
	doc := s.documents[params.TextDocument.URI]
	s.documentsMu.RUnlock()

	if doc == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	locations, err := s.referencesHandler(ctx, doc, params.Position)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InternalError, Message: err.Error()},
		}
	}

	return &Message{JSONRPC: "2.0", ID: msg.ID, Result: locations}
}

func (s *Server) handleDocumentSymbol(ctx context.Context, msg *Message) *Message {
	if s.symbolsHandler == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InvalidParams, Message: err.Error()},
		}
	}

	s.documentsMu.RLock()
	doc := s.documents[params.TextDocument.URI]
	s.documentsMu.RUnlock()

	if doc == nil {
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: nil}
	}

	symbols, err := s.symbolsHandler(ctx, doc)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &ResponseError{Code: InternalError, Message: err.Error()},
		}
	}

	return &Message{JSONRPC: "2.0", ID: msg.ID, Result: symbols}
}

func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	if s.diagnosticHandler == nil {
		return
	}

	diagnostics, err := s.diagnosticHandler(ctx, doc)
	if err != nil {
		return
	}

	s.writeMessage(&Message{
		JSONRPC: "2.0",
		Method:  MethodPublishDiagnostics,
		Params:  mustMarshal(PublishDiagnosticsParams{URI: doc.URI, Diagnostics: diagnostics}),
	})
}

// GetDocument returns an open document by URI.
func (s *Server) GetDocument(uri string) *Document {
	s.documentsMu.RLock()
	defer s.documentsMu.RUnlock()
	return s.documents[uri]
}

// GetWordAtPosition returns the word at the given position.
func GetWordAtPosition(doc *Document, pos Position) string {
	if pos.Line >= len(doc.Lines) {
		return ""
	}

	line := doc.Lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	// Find word boundaries
	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}

	end := pos.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	return line[start:end]
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// URIToPath converts a file:// URI to a file path.
func URIToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

// PathToURI converts a file path to a file:// URI.
func PathToURI(path string) string {
	if !strings.HasPrefix(path, "file://") {
		absPath, _ := filepath.Abs(path)
		return "file://" + absPath
	}
	return path
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
