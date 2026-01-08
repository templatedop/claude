// Package lsp provides a Go language server implementation.
package lsp

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// GoHandler provides Go language intelligence.
type GoHandler struct {
	fset *token.FileSet
}

// NewGoHandler creates a new Go handler.
func NewGoHandler() *GoHandler {
	return &GoHandler{
		fset: token.NewFileSet(),
	}
}

// RegisterWithServer registers Go handlers with an LSP server.
func (h *GoHandler) RegisterWithServer(s *Server) {
	s.SetHoverHandler(h.Hover)
	s.SetCompletionHandler(h.Completion)
	s.SetSymbolsHandler(h.DocumentSymbols)
	s.SetDiagnosticHandler(h.Diagnostics)
}

// Hover provides hover information for Go code.
func (h *GoHandler) Hover(ctx context.Context, doc *Document, pos Position) (*Hover, error) {
	if doc.LanguageID != "go" && !strings.HasSuffix(doc.URI, ".go") {
		return nil, nil
	}

	word := GetWordAtPosition(doc, pos)
	if word == "" {
		return nil, nil
	}

	// Check for Go keywords and builtins
	if info := getGoBuiltinInfo(word); info != "" {
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: info,
			},
		}, nil
	}

	// Try to find definition in the current file
	file, err := parser.ParseFile(h.fset, doc.URI, doc.Content, parser.ParseComments)
	if err != nil {
		return nil, nil
	}

	// Look for function, type, or variable definition
	var info string
	ast.Inspect(file, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			if decl.Name.Name == word {
				info = formatFuncDecl(decl)
				return false
			}
		case *ast.TypeSpec:
			if decl.Name.Name == word {
				info = formatTypeSpec(decl)
				return false
			}
		case *ast.ValueSpec:
			for _, name := range decl.Names {
				if name.Name == word {
					info = formatValueSpec(decl)
					return false
				}
			}
		}
		return true
	})

	if info != "" {
		return &Hover{
			Contents: MarkupContent{
				Kind:  "markdown",
				Value: info,
			},
		}, nil
	}

	return nil, nil
}

// Completion provides code completion for Go.
func (h *GoHandler) Completion(ctx context.Context, doc *Document, pos Position) (*CompletionList, error) {
	if doc.LanguageID != "go" && !strings.HasSuffix(doc.URI, ".go") {
		return &CompletionList{}, nil
	}

	items := make([]CompletionItem, 0)

	// Get partial word at cursor
	prefix := ""
	if pos.Line < len(doc.Lines) {
		line := doc.Lines[pos.Line]
		if pos.Character <= len(line) {
			start := pos.Character
			for start > 0 && isWordChar(line[start-1]) {
				start--
			}
			prefix = strings.ToLower(line[start:pos.Character])
		}
	}

	// Add Go keywords
	for _, kw := range goKeywords {
		if prefix == "" || strings.HasPrefix(strings.ToLower(kw), prefix) {
			items = append(items, CompletionItem{
				Label:  kw,
				Kind:   CompletionKindKeyword,
				Detail: "keyword",
			})
		}
	}

	// Add Go builtins
	for name, info := range goBuiltins {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   CompletionKindFunction,
				Detail: "builtin",
				Documentation: MarkupContent{
					Kind:  "markdown",
					Value: info,
				},
			})
		}
	}

	// Parse current file for local symbols
	file, err := parser.ParseFile(h.fset, doc.URI, doc.Content, 0)
	if err == nil {
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				name := decl.Name.Name
				if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
					items = append(items, CompletionItem{
						Label:      name,
						Kind:       CompletionKindFunction,
						Detail:     formatFuncSignature(decl),
						InsertText: name,
					})
				}
			case *ast.TypeSpec:
				name := decl.Name.Name
				if prefix == "" || strings.HasPrefix(strings.ToLower(name), prefix) {
					kind := CompletionKindClass
					if _, ok := decl.Type.(*ast.InterfaceType); ok {
						kind = CompletionKindInterface
					} else if _, ok := decl.Type.(*ast.StructType); ok {
						kind = CompletionKindStruct
					}
					items = append(items, CompletionItem{
						Label:      name,
						Kind:       kind,
						Detail:     "type",
						InsertText: name,
					})
				}
			case *ast.ValueSpec:
				for _, name := range decl.Names {
					n := name.Name
					if prefix == "" || strings.HasPrefix(strings.ToLower(n), prefix) {
						items = append(items, CompletionItem{
							Label:      n,
							Kind:       CompletionKindVariable,
							InsertText: n,
						})
					}
				}
			}
			return true
		})
	}

	// Add common snippets
	snippets := []CompletionItem{
		{
			Label:      "func",
			Kind:       CompletionKindSnippet,
			Detail:     "function declaration",
			InsertText: "func ${1:name}(${2:params}) ${3:returnType} {\n\t$0\n}",
		},
		{
			Label:      "if",
			Kind:       CompletionKindSnippet,
			Detail:     "if statement",
			InsertText: "if ${1:condition} {\n\t$0\n}",
		},
		{
			Label:      "for",
			Kind:       CompletionKindSnippet,
			Detail:     "for loop",
			InsertText: "for ${1:i := 0}; ${2:i < n}; ${3:i++} {\n\t$0\n}",
		},
		{
			Label:      "forr",
			Kind:       CompletionKindSnippet,
			Detail:     "for range loop",
			InsertText: "for ${1:i}, ${2:v} := range ${3:collection} {\n\t$0\n}",
		},
		{
			Label:      "err",
			Kind:       CompletionKindSnippet,
			Detail:     "error handling",
			InsertText: "if err != nil {\n\treturn ${1:err}\n}",
		},
		{
			Label:      "struct",
			Kind:       CompletionKindSnippet,
			Detail:     "struct declaration",
			InsertText: "type ${1:Name} struct {\n\t${2:field} ${3:type}\n}",
		},
		{
			Label:      "interface",
			Kind:       CompletionKindSnippet,
			Detail:     "interface declaration",
			InsertText: "type ${1:Name} interface {\n\t${2:Method}(${3:params}) ${4:returnType}\n}",
		},
	}

	for _, s := range snippets {
		if prefix == "" || strings.HasPrefix(s.Label, prefix) {
			items = append(items, s)
		}
	}

	return &CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// DocumentSymbols returns document symbols for Go.
func (h *GoHandler) DocumentSymbols(ctx context.Context, doc *Document) ([]DocumentSymbol, error) {
	if doc.LanguageID != "go" && !strings.HasSuffix(doc.URI, ".go") {
		return nil, nil
	}

	file, err := parser.ParseFile(h.fset, doc.URI, doc.Content, parser.ParseComments)
	if err != nil {
		return nil, nil
	}

	symbols := make([]DocumentSymbol, 0)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			symbols = append(symbols, h.funcToSymbol(d))
		case *ast.GenDecl:
			symbols = append(symbols, h.genDeclToSymbols(d)...)
		}
	}

	return symbols, nil
}

func (h *GoHandler) funcToSymbol(fn *ast.FuncDecl) DocumentSymbol {
	start := h.fset.Position(fn.Pos())
	end := h.fset.Position(fn.End())

	name := fn.Name.Name
	detail := formatFuncSignature(fn)

	return DocumentSymbol{
		Name:   name,
		Detail: detail,
		Kind:   SymbolKindFunction,
		Range: Range{
			Start: Position{Line: start.Line - 1, Character: start.Column - 1},
			End:   Position{Line: end.Line - 1, Character: end.Column - 1},
		},
		SelectionRange: Range{
			Start: Position{Line: start.Line - 1, Character: start.Column - 1},
			End:   Position{Line: start.Line - 1, Character: start.Column - 1 + len(name)},
		},
	}
}

func (h *GoHandler) genDeclToSymbols(decl *ast.GenDecl) []DocumentSymbol {
	symbols := make([]DocumentSymbol, 0)

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			start := h.fset.Position(s.Pos())
			end := h.fset.Position(s.End())

			kind := SymbolKindClass
			if _, ok := s.Type.(*ast.InterfaceType); ok {
				kind = SymbolKindInterface
			} else if _, ok := s.Type.(*ast.StructType); ok {
				kind = SymbolKindStruct
			}

			sym := DocumentSymbol{
				Name: s.Name.Name,
				Kind: kind,
				Range: Range{
					Start: Position{Line: start.Line - 1, Character: start.Column - 1},
					End:   Position{Line: end.Line - 1, Character: end.Column - 1},
				},
				SelectionRange: Range{
					Start: Position{Line: start.Line - 1, Character: start.Column - 1},
					End:   Position{Line: start.Line - 1, Character: start.Column - 1 + len(s.Name.Name)},
				},
			}

			// Add struct fields as children
			if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						fieldStart := h.fset.Position(name.Pos())
						sym.Children = append(sym.Children, DocumentSymbol{
							Name: name.Name,
							Kind: SymbolKindField,
							Range: Range{
								Start: Position{Line: fieldStart.Line - 1, Character: fieldStart.Column - 1},
								End:   Position{Line: fieldStart.Line - 1, Character: fieldStart.Column - 1 + len(name.Name)},
							},
							SelectionRange: Range{
								Start: Position{Line: fieldStart.Line - 1, Character: fieldStart.Column - 1},
								End:   Position{Line: fieldStart.Line - 1, Character: fieldStart.Column - 1 + len(name.Name)},
							},
						})
					}
				}
			}

			symbols = append(symbols, sym)

		case *ast.ValueSpec:
			for _, name := range s.Names {
				start := h.fset.Position(name.Pos())

				kind := SymbolKindVariable
				if decl.Tok == token.CONST {
					kind = SymbolKindConstant
				}

				symbols = append(symbols, DocumentSymbol{
					Name: name.Name,
					Kind: kind,
					Range: Range{
						Start: Position{Line: start.Line - 1, Character: start.Column - 1},
						End:   Position{Line: start.Line - 1, Character: start.Column - 1 + len(name.Name)},
					},
					SelectionRange: Range{
						Start: Position{Line: start.Line - 1, Character: start.Column - 1},
						End:   Position{Line: start.Line - 1, Character: start.Column - 1 + len(name.Name)},
					},
				})
			}
		}
	}

	return symbols
}

// Diagnostics provides diagnostics for Go.
func (h *GoHandler) Diagnostics(ctx context.Context, doc *Document) ([]Diagnostic, error) {
	if doc.LanguageID != "go" && !strings.HasSuffix(doc.URI, ".go") {
		return nil, nil
	}

	diagnostics := make([]Diagnostic, 0)

	// Parse for syntax errors
	_, err := parser.ParseFile(h.fset, doc.URI, doc.Content, parser.AllErrors)
	if err != nil {
		// Extract error position and message
		errStr := err.Error()
		// Parse error format: "filename:line:col: message"
		re := regexp.MustCompile(`:(\d+):(\d+): (.+)$`)
		matches := re.FindStringSubmatch(errStr)
		if len(matches) == 4 {
			line := 0
			col := 0
			fmt.Sscanf(matches[1], "%d", &line)
			fmt.Sscanf(matches[2], "%d", &col)

			diagnostics = append(diagnostics, Diagnostic{
				Range: Range{
					Start: Position{Line: line - 1, Character: col - 1},
					End:   Position{Line: line - 1, Character: col},
				},
				Severity: DiagnosticSeverityError,
				Source:   "go",
				Message:  matches[3],
			})
		}
	}

	return diagnostics, nil
}

// Go keywords
var goKeywords = []string{
	"break", "case", "chan", "const", "continue",
	"default", "defer", "else", "fallthrough", "for",
	"func", "go", "goto", "if", "import",
	"interface", "map", "package", "range", "return",
	"select", "struct", "switch", "type", "var",
}

// Go builtins with documentation
var goBuiltins = map[string]string{
	"append":  "```go\nfunc append(slice []Type, elems ...Type) []Type\n```\nAppends elements to the end of a slice.",
	"cap":     "```go\nfunc cap(v Type) int\n```\nReturns the capacity of a slice, array, or channel.",
	"close":   "```go\nfunc close(c chan<- Type)\n```\nCloses a channel.",
	"complex": "```go\nfunc complex(r, i FloatType) ComplexType\n```\nConstructs a complex number.",
	"copy":    "```go\nfunc copy(dst, src []Type) int\n```\nCopies elements from source to destination slice.",
	"delete":  "```go\nfunc delete(m map[Type]Type1, key Type)\n```\nDeletes an element from a map.",
	"imag":    "```go\nfunc imag(c ComplexType) FloatType\n```\nReturns the imaginary part of a complex number.",
	"len":     "```go\nfunc len(v Type) int\n```\nReturns the length of a string, slice, array, map, or channel.",
	"make":    "```go\nfunc make(t Type, size ...IntegerType) Type\n```\nAllocates and initializes a slice, map, or channel.",
	"new":     "```go\nfunc new(Type) *Type\n```\nAllocates memory and returns a pointer to it.",
	"panic":   "```go\nfunc panic(v interface{})\n```\nStops normal execution and begins panicking.",
	"print":   "```go\nfunc print(args ...Type)\n```\nFormats and prints to stderr.",
	"println": "```go\nfunc println(args ...Type)\n```\nFormats and prints to stderr with newline.",
	"real":    "```go\nfunc real(c ComplexType) FloatType\n```\nReturns the real part of a complex number.",
	"recover": "```go\nfunc recover() interface{}\n```\nRegains control of a panicking goroutine.",
	"error":   "```go\ntype error interface {\n    Error() string\n}\n```\nThe built-in error interface.",
	"true":    "Boolean constant true.",
	"false":   "Boolean constant false.",
	"nil":     "Zero value for pointers, interfaces, maps, slices, channels, and functions.",
	"iota":    "Constant generator for enumerated constants.",
}

func getGoBuiltinInfo(name string) string {
	return goBuiltins[name]
}

func formatFuncDecl(fn *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("```go\n")
	sb.WriteString("func ")

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sb.WriteString("(")
		formatFieldList(&sb, fn.Recv)
		sb.WriteString(") ")
	}

	sb.WriteString(fn.Name.Name)
	sb.WriteString("(")
	if fn.Type.Params != nil {
		formatFieldList(&sb, fn.Type.Params)
	}
	sb.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		sb.WriteString(" ")
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString("(")
		}
		formatFieldList(&sb, fn.Type.Results)
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString(")")
		}
	}

	sb.WriteString("\n```")
	return sb.String()
}

func formatFuncSignature(fn *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func(")
	if fn.Type.Params != nil {
		formatFieldList(&sb, fn.Type.Params)
	}
	sb.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		sb.WriteString(" ")
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString("(")
		}
		formatFieldList(&sb, fn.Type.Results)
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func formatTypeSpec(ts *ast.TypeSpec) string {
	var sb strings.Builder
	sb.WriteString("```go\ntype ")
	sb.WriteString(ts.Name.Name)
	sb.WriteString(" ")

	switch t := ts.Type.(type) {
	case *ast.StructType:
		sb.WriteString("struct { ... }")
	case *ast.InterfaceType:
		sb.WriteString("interface { ... }")
	case *ast.Ident:
		sb.WriteString(t.Name)
	default:
		sb.WriteString("...")
	}

	sb.WriteString("\n```")
	return sb.String()
}

func formatValueSpec(vs *ast.ValueSpec) string {
	var sb strings.Builder
	sb.WriteString("```go\nvar ")

	for i, name := range vs.Names {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(name.Name)
	}

	if vs.Type != nil {
		sb.WriteString(" ")
		formatExpr(&sb, vs.Type)
	}

	sb.WriteString("\n```")
	return sb.String()
}

func formatFieldList(sb *strings.Builder, fl *ast.FieldList) {
	for i, field := range fl.List {
		if i > 0 {
			sb.WriteString(", ")
		}

		for j, name := range field.Names {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name.Name)
		}

		if len(field.Names) > 0 {
			sb.WriteString(" ")
		}

		formatExpr(sb, field.Type)
	}
}

func formatExpr(sb *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		sb.WriteString(e.Name)
	case *ast.StarExpr:
		sb.WriteString("*")
		formatExpr(sb, e.X)
	case *ast.ArrayType:
		sb.WriteString("[]")
		formatExpr(sb, e.Elt)
	case *ast.MapType:
		sb.WriteString("map[")
		formatExpr(sb, e.Key)
		sb.WriteString("]")
		formatExpr(sb, e.Value)
	case *ast.SelectorExpr:
		formatExpr(sb, e.X)
		sb.WriteString(".")
		sb.WriteString(e.Sel.Name)
	case *ast.InterfaceType:
		sb.WriteString("interface{}")
	case *ast.FuncType:
		sb.WriteString("func(...)")
	case *ast.ChanType:
		sb.WriteString("chan ")
		formatExpr(sb, e.Value)
	case *ast.Ellipsis:
		sb.WriteString("...")
		formatExpr(sb, e.Elt)
	default:
		sb.WriteString("...")
	}
}
