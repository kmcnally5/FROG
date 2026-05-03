package main

import (
	"encoding/json"
	"fmt"
	"klex/ast"
	"path/filepath"
	"sync"
)

type DocumentState struct {
	URI     string
	Text    string
	Version int
	AST     *ast.Program
	Symbols *SymbolTable
}

type Server struct {
	transport     *Transport
	documents     map[string]*DocumentState
	fileCache     map[string]string // cache for file contents
	mu            sync.RWMutex
	initialized   bool
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
			s.initialized = false
			s.transport.SendResponse(msg.ID, nil, nil)
			continue
		}

		if msg.Method == "exit" {
			if s.initialized {
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
	s.initialized = true
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			HoverProvider:      true,
			DefinitionProvider: true,
			CompletionProvider: map[string]interface{}{
				"resolveProvider": false,
			},
			DiagnosticProvider: true,
			TextDocumentSyncKind: TextDocumentSyncFull,
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

	doc := &DocumentState{
		URI:     params.TextDocument.URI,
		Text:    params.TextDocument.Text,
		Version: params.TextDocument.Version,
	}
	s.parseDocument(doc)

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

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		return
	}

	// For full document sync, the entire new text is in contentChanges[0]
	if len(params.ContentChanges) > 0 {
		doc.Text = params.ContentChanges[0].Text
		doc.Version = params.TextDocument.Version
	}

	s.parseDocument(doc)
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

	s.mu.RLock()
	doc, exists := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !exists {
		s.transport.SendResponse(msg.ID, nil, nil)
		return
	}

	result := HoverAtPosition(doc, params.Position)
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

func (s *Server) parseDocument(doc *DocumentState) {
	doc.AST, doc.Symbols = ParseDocumentAndBuildSymbols(doc.URI, doc.Text)
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
