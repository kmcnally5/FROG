package main

import (
	"encoding/json"
	"fmt"
	"klex/ast"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// DocumentState is the parsed view of one open document.
//
// CONCURRENCY: once a *DocumentState pointer has been published into
// Server.documents, its fields MUST NOT be mutated. Updates from
// textDocument/didChange replace the map entry with a freshly constructed
// pointer (see buildDocument). Readers grab the pointer under s.mu.RLock()
// and then read the immutable fields lock-free — there is no risk of seeing
// Text update before AST/Symbols, or vice versa.
type DocumentState struct {
	URI     string
	Text    string
	Version int
	AST     *ast.Program
	Symbols *SymbolTable
}

type Server struct {
	transport   *Transport
	documents   map[string]*DocumentState
	fileCache   map[string]string // cache for file contents
	mu          sync.RWMutex
	initialized atomic.Bool
}

func NewServer(transport *Transport) *Server {
	return &Server{
		transport: transport,
		documents: make(map[string]*DocumentState),
		fileCache: make(map[string]string),
	}
}

// Run starts the server's message loop
func (s *Server) Run() error {
	for {
		msg, err := s.transport.ReadMessage()
		if err != nil {
			LogMessage("error reading message: %v", err)
			return err
		}

		if msg.Method == "shutdown" {
			s.initialized.Store(false)
			s.transport.SendResponse(msg.ID, nil, nil)
			continue
		}

		if msg.Method == "exit" {
			if s.initialized.Load() {
				return fmt.Errorf("exit called before shutdown")
			}
			return nil
		}

		// Dispatch to handler
		go s.handleMessage(msg)
	}
}

func (s *Server) handleMessage(msg *Message) {
	defer func() {
		if r := recover(); r != nil {
			LogMessage("panic in handler: %v", r)
			s.transport.SendResponse(msg.ID, nil, &RPCError{
				Code:    InternalError,
				Message: fmt.Sprintf("internal error: %v", r),
			})
		}
	}()

	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// no-op
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(msg)
	case "textDocument/diagnostic":
		s.handleDiagnostic(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "textDocument/formatting":
		s.handleFormatting(msg)
	case "$/cancelRequest":
		// no-op for now
	default:
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("method not found: %s", msg.Method),
		})
	}
}

func (s *Server) handleInitialize(msg *Message) {
	s.initialized.Store(true)
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			HoverProvider:      true,
			DefinitionProvider: true,
			CompletionProvider: map[string]interface{}{
				// Trigger completion on dot for module-member access
				// (`module.|`) and on most ASCII letters so the user
				// can keep typing without hitting Ctrl+Space.
				"resolveProvider":   false,
				"triggerCharacters": []string{".", ":"},
			},
			SignatureHelpProvider: map[string]interface{}{
				// Pop the param-help bubble on `(` and re-trigger on
				// `,` so the active-parameter highlight tracks the
				// cursor naturally.
				"triggerCharacters":   []string{"(", ","},
				"retriggerCharacters": []string{","},
			},
			DiagnosticProvider:         true,
			DocumentSymbolProvider:     true,
			DocumentFormattingProvider: true,
			CodeActionProvider: map[string]interface{}{
				// Restrict to the quickfix family for now — the lint
				// pass surfaces a small fixed set of diagnostics with
				// canned fixes.
				"codeActionKinds": []string{CodeActionKindQuickFix},
			},
			TextDocumentSyncKind: TextDocumentSyncFull,
		},
		ServerInfo: map[string]interface{}{
			"name":    "froglsp",
			"version": "0.2.0",
		},
	}
	s.transport.SendResponse(msg.ID, result, nil)
}

func (s *Server) handleDidOpen(msg *Message) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		LogMessage("didOpen: unmarshal error: %v", err)
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	doc := buildDocument(params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version)

	s.mu.Lock()
	s.documents[doc.URI] = doc
	s.mu.Unlock()

	// Publish diagnostics
	s.publishDiagnostics(doc)
}

func (s *Server) handleDidChange(msg *Message) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	// LSP "full sync" delivers the entire new text in contentChanges[0].
	// If there are no changes we have nothing to do.
	if len(params.ContentChanges) == 0 {
		return
	}

	s.mu.RLock()
	_, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()
	if !exists {
		return
	}

	// Build a fresh document snapshot. The OLD pointer may still be in flight
	// inside a concurrent hover/definition/completion handler — those readers
	// keep using their snapshot until they return. Our swap publishes the new
	// snapshot atomically.
	doc := buildDocument(
		params.TextDocument.URI,
		params.ContentChanges[0].Text,
		params.TextDocument.Version,
	)

	s.mu.Lock()
	s.documents[doc.URI] = doc
	s.mu.Unlock()

	s.publishDiagnostics(doc)
}

func (s *Server) handleDidClose(msg *Message) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	s.mu.Lock()
	delete(s.documents, params.TextDocument.URI)
	s.mu.Unlock()

	// Clear diagnostics for this document
	s.transport.SendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

func (s *Server) handleHover(msg *Message) {
	var params HoverParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	LogMessage("HOVER REQUEST: line=%d char=%d uri=%s", params.Position.Line, params.Position.Character, params.TextDocument.URI)

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		LogMessage("HOVER: document not found")
		s.transport.SendResponse(msg.ID, nil, nil)
		return
	}

	LogMessage("HOVER: document exists, calling HoverAtPosition")
	result := HoverAtPosition(doc, params.Position)

	if result != nil {
		content := result.Contents.Value
		if len(content) > 100 {
			content = content[:100]
		}
		LogMessage("HOVER RESULT: contents=%s", content)
	} else {
		LogMessage("HOVER RESULT: nil")
	}

	s.transport.SendResponse(msg.ID, result, nil)
}

func (s *Server) handleDefinition(msg *Message) {
	var params DefinitionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		s.transport.SendResponse(msg.ID, nil, nil)
		return
	}

	result := DefinitionAtPosition(doc, params.Position)
	s.transport.SendResponse(msg.ID, result, nil)
}

func (s *Server) handleCompletion(msg *Message) {
	var params CompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		s.transport.SendResponse(msg.ID, nil, nil)
		return
	}

	result := CompletionsAtPosition(doc, params.Position)
	s.transport.SendResponse(msg.ID, result, nil)
}

func (s *Server) handleSignatureHelp(msg *Message) {
	var params SignatureHelpParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		s.transport.SendResponse(msg.ID, nil, nil)
		return
	}

	result := SignatureHelpAtPosition(doc, params.Position)
	s.transport.SendResponse(msg.ID, result, nil)
}

func (s *Server) handleDiagnostic(msg *Message) {
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		s.transport.SendResponse(msg.ID, map[string]interface{}{"items": []Diagnostic{}}, nil)
		return
	}

	diags := DiagnosticsFromProgram(doc.AST)
	s.transport.SendResponse(msg.ID, map[string]interface{}{"items": diags}, nil)
}

func (s *Server) handleDocumentSymbol(msg *Message) {
	var params DocumentSymbolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}
	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()
	if !exists {
		s.transport.SendResponse(msg.ID, []DocumentSymbol{}, nil)
		return
	}
	syms := DocumentSymbolsForDoc(doc)
	s.transport.SendResponse(msg.ID, syms, nil)
}

func (s *Server) handleCodeAction(msg *Message) {
	var params CodeActionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}
	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()
	if !exists {
		s.transport.SendResponse(msg.ID, []CodeAction{}, nil)
		return
	}
	actions := CodeActionsForRange(doc, params.Range, params.Context.Diagnostics)
	s.transport.SendResponse(msg.ID, actions, nil)
}


// buildDocument constructs a fully-parsed, immutable *DocumentState.
// Used by handleDidOpen and handleDidChange — once returned, the pointer
// is published into Server.documents and its fields are never mutated.
func buildDocument(uri, text string, version int) *DocumentState {
	prog, syms := ParseDocumentAndBuildSymbols(uri, text)
	return &DocumentState{
		URI:     uri,
		Text:    text,
		Version: version,
		AST:     prog,
		Symbols: syms,
	}
}

func (s *Server) publishDiagnostics(doc *DocumentState) {
	diags := DiagnosticsFromProgram(doc.AST)
	s.transport.SendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diags,
	})
}

// URIToPath converts file:// URI to filesystem path
func URIToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}

// PathToURI converts filesystem path to file:// URI
func PathToURI(path string) string {
	return "file://" + filepath.Clean(path)
}
