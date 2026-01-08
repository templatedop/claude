package lsp

import (
	"context"
	"testing"
)

func TestNewServer(t *testing.T) {
	server := NewServer("test-lsp", "1.0.0")

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.info.Name != "test-lsp" {
		t.Errorf("Expected name 'test-lsp', got '%s'", server.info.Name)
	}

	if server.info.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", server.info.Version)
	}
}

func TestSetHandlers(t *testing.T) {
	server := NewServer("test", "1.0.0")

	// Set hover handler
	server.SetHoverHandler(func(ctx context.Context, doc *Document, pos Position) (*Hover, error) {
		return &Hover{
			Contents: MarkupContent{Kind: "plaintext", Value: "test hover"},
		}, nil
	})

	if server.hoverHandler == nil {
		t.Error("Hover handler not set")
	}

	// Set completion handler
	server.SetCompletionHandler(func(ctx context.Context, doc *Document, pos Position) (*CompletionList, error) {
		return &CompletionList{Items: []CompletionItem{}}, nil
	})

	if server.completionHandler == nil {
		t.Error("Completion handler not set")
	}

	// Set definition handler
	server.SetDefinitionHandler(func(ctx context.Context, doc *Document, pos Position) ([]Location, error) {
		return []Location{}, nil
	})

	if server.definitionHandler == nil {
		t.Error("Definition handler not set")
	}

	// Set references handler
	server.SetReferencesHandler(func(ctx context.Context, doc *Document, pos Position) ([]Location, error) {
		return []Location{}, nil
	})

	if server.referencesHandler == nil {
		t.Error("References handler not set")
	}

	// Set symbols handler
	server.SetSymbolsHandler(func(ctx context.Context, doc *Document) ([]DocumentSymbol, error) {
		return []DocumentSymbol{}, nil
	})

	if server.symbolsHandler == nil {
		t.Error("Symbols handler not set")
	}

	// Set diagnostic handler
	server.SetDiagnosticHandler(func(ctx context.Context, doc *Document) ([]Diagnostic, error) {
		return []Diagnostic{}, nil
	})

	if server.diagnosticHandler == nil {
		t.Error("Diagnostic handler not set")
	}
}

func TestGetWordAtPosition(t *testing.T) {
	doc := &Document{
		Content: "func main() {\n\tfmt.Println(\"hello\")\n}",
		Lines:   []string{"func main() {", "\tfmt.Println(\"hello\")", "}"},
	}

	tests := []struct {
		pos      Position
		expected string
	}{
		{Position{Line: 0, Character: 0}, "func"},
		{Position{Line: 0, Character: 5}, "main"},
		{Position{Line: 1, Character: 2}, "fmt"},
		{Position{Line: 1, Character: 6}, "Println"},
		{Position{Line: 2, Character: 0}, ""},
	}

	for _, tt := range tests {
		result := GetWordAtPosition(doc, tt.pos)
		if result != tt.expected {
			t.Errorf("At %v: expected '%s', got '%s'", tt.pos, tt.expected, result)
		}
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri      string
		expected string
	}{
		{"file:///home/user/test.go", "/home/user/test.go"},
		{"/home/user/test.go", "/home/user/test.go"},
		{"file:///C:/Users/test.go", "/C:/Users/test.go"},
	}

	for _, tt := range tests {
		result := URIToPath(tt.uri)
		if result != tt.expected {
			t.Errorf("URIToPath(%s) = '%s', expected '%s'", tt.uri, result, tt.expected)
		}
	}
}

func TestPathToURI(t *testing.T) {
	tests := []struct {
		path     string
		hasFile  bool
	}{
		{"/home/user/test.go", true},
		{"file:///home/user/test.go", true},
	}

	for _, tt := range tests {
		result := PathToURI(tt.path)
		if tt.hasFile && result[:7] != "file://" {
			t.Errorf("PathToURI(%s) should start with file://, got '%s'", tt.path, result)
		}
	}
}

func TestPosition(t *testing.T) {
	pos := Position{Line: 10, Character: 5}

	if pos.Line != 10 {
		t.Errorf("Expected line 10, got %d", pos.Line)
	}

	if pos.Character != 5 {
		t.Errorf("Expected character 5, got %d", pos.Character)
	}
}

func TestRange(t *testing.T) {
	r := Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: 10, Character: 5},
	}

	if r.Start.Line != 0 {
		t.Errorf("Expected start line 0, got %d", r.Start.Line)
	}

	if r.End.Line != 10 {
		t.Errorf("Expected end line 10, got %d", r.End.Line)
	}
}

func TestLocation(t *testing.T) {
	loc := Location{
		URI: "file:///test.go",
		Range: Range{
			Start: Position{Line: 5, Character: 0},
			End:   Position{Line: 5, Character: 10},
		},
	}

	if loc.URI != "file:///test.go" {
		t.Errorf("Expected URI 'file:///test.go', got '%s'", loc.URI)
	}
}

func TestMarkupContent(t *testing.T) {
	mc := MarkupContent{
		Kind:  "markdown",
		Value: "# Title\n\nContent",
	}

	if mc.Kind != "markdown" {
		t.Errorf("Expected kind 'markdown', got '%s'", mc.Kind)
	}
}

func TestCompletionItem(t *testing.T) {
	item := CompletionItem{
		Label:      "myFunc",
		Kind:       CompletionKindFunction,
		Detail:     "func myFunc()",
		InsertText: "myFunc()",
	}

	if item.Label != "myFunc" {
		t.Errorf("Expected label 'myFunc', got '%s'", item.Label)
	}

	if item.Kind != CompletionKindFunction {
		t.Errorf("Expected kind %d, got %d", CompletionKindFunction, item.Kind)
	}
}

func TestCompletionList(t *testing.T) {
	list := CompletionList{
		IsIncomplete: false,
		Items: []CompletionItem{
			{Label: "item1"},
			{Label: "item2"},
		},
	}

	if len(list.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(list.Items))
	}
}

func TestDocumentSymbol(t *testing.T) {
	sym := DocumentSymbol{
		Name:   "MyType",
		Detail: "type",
		Kind:   SymbolKindClass,
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: 10, Character: 0},
		},
		SelectionRange: Range{
			Start: Position{Line: 0, Character: 5},
			End:   Position{Line: 0, Character: 11},
		},
		Children: []DocumentSymbol{
			{Name: "Field1", Kind: SymbolKindField},
		},
	}

	if sym.Name != "MyType" {
		t.Errorf("Expected name 'MyType', got '%s'", sym.Name)
	}

	if len(sym.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(sym.Children))
	}
}

func TestDiagnostic(t *testing.T) {
	diag := Diagnostic{
		Range: Range{
			Start: Position{Line: 5, Character: 0},
			End:   Position{Line: 5, Character: 10},
		},
		Severity: DiagnosticSeverityError,
		Source:   "test",
		Message:  "Test error message",
	}

	if diag.Severity != DiagnosticSeverityError {
		t.Errorf("Expected severity %d, got %d", DiagnosticSeverityError, diag.Severity)
	}

	if diag.Message != "Test error message" {
		t.Errorf("Expected message 'Test error message', got '%s'", diag.Message)
	}
}

func TestDocument(t *testing.T) {
	doc := &Document{
		URI:        "file:///test.go",
		LanguageID: "go",
		Version:    1,
		Content:    "package main\n\nfunc main() {}\n",
		Lines:      []string{"package main", "", "func main() {}"},
	}

	if doc.URI != "file:///test.go" {
		t.Errorf("Expected URI 'file:///test.go', got '%s'", doc.URI)
	}

	if doc.LanguageID != "go" {
		t.Errorf("Expected language 'go', got '%s'", doc.LanguageID)
	}

	if len(doc.Lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(doc.Lines))
	}
}

func TestSymbolKindConstants(t *testing.T) {
	// Test that symbol kind constants are defined
	constants := []int{
		SymbolKindFile,
		SymbolKindModule,
		SymbolKindClass,
		SymbolKindMethod,
		SymbolKindFunction,
		SymbolKindVariable,
		SymbolKindConstant,
		SymbolKindStruct,
		SymbolKindInterface,
	}

	for _, c := range constants {
		if c <= 0 {
			t.Errorf("Symbol kind constant should be positive, got %d", c)
		}
	}
}

func TestCompletionKindConstants(t *testing.T) {
	// Test that completion kind constants are defined
	constants := []int{
		CompletionKindText,
		CompletionKindMethod,
		CompletionKindFunction,
		CompletionKindVariable,
		CompletionKindClass,
		CompletionKindInterface,
		CompletionKindKeyword,
		CompletionKindSnippet,
	}

	for _, c := range constants {
		if c <= 0 {
			t.Errorf("Completion kind constant should be positive, got %d", c)
		}
	}
}

func TestDiagnosticSeverityConstants(t *testing.T) {
	if DiagnosticSeverityError != 1 {
		t.Errorf("Expected DiagnosticSeverityError = 1, got %d", DiagnosticSeverityError)
	}
	if DiagnosticSeverityWarning != 2 {
		t.Errorf("Expected DiagnosticSeverityWarning = 2, got %d", DiagnosticSeverityWarning)
	}
	if DiagnosticSeverityInformation != 3 {
		t.Errorf("Expected DiagnosticSeverityInformation = 3, got %d", DiagnosticSeverityInformation)
	}
	if DiagnosticSeverityHint != 4 {
		t.Errorf("Expected DiagnosticSeverityHint = 4, got %d", DiagnosticSeverityHint)
	}
}
