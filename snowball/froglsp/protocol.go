package main

import "encoding/json"

// RPC message types
type RequestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type ResponseMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type NotificationMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
	ServerError    = -32000
)

// LSP types
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// Hover
type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// Definition
type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type LocationLink struct {
	OriginSelectionRange *Range   `json:"originSelectionRange,omitempty"`
	TargetURI            string   `json:"targetUri"`
	TargetRange          Range    `json:"targetRange"`
	TargetSelectionRange Range    `json:"targetSelectionRange"`
}

// Completion
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *CompletionContext     `json:"context,omitempty"`
}

type CompletionContext struct {
	TriggerKind      int     `json:"triggerKind"`
	TriggerCharacter *string `json:"triggerCharacter,omitempty"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	SortText      string `json:"sortText,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool              `json:"isIncomplete"`
	Items        []CompletionItem  `json:"items"`
}

// Completion kind constants
const (
	CompletionText         = 1
	CompletionMethod       = 2
	CompletionFunction     = 3
	CompletionConstructor  = 4
	CompletionField        = 5
	CompletionVariable     = 6
	CompletionClass        = 7
	CompletionInterface    = 8
	CompletionModule       = 9
	CompletionProperty     = 10
	CompletionUnit         = 11
	CompletionValue        = 12
	CompletionEnum         = 13
	CompletionKeyword      = 14
	CompletionSnippet      = 15
	CompletionColor        = 16
	CompletionFile         = 17
	CompletionReference    = 18
	CompletionFolder       = 19
	CompletionEnumMember   = 20
	CompletionConstant     = 21
	CompletionStruct       = 22
	CompletionEvent        = 23
	CompletionOperator     = 24
	CompletionTypeParameter = 25
)

// Diagnostics
type Diagnostic struct {
	Range    Range    `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string        `json:"uri"`
	Diagnostics []Diagnostic  `json:"diagnostics"`
}

const (
	DiagnosticError       = 1
	DiagnosticWarning     = 2
	DiagnosticInformation = 3
	DiagnosticHint        = 4
)

// Initialize
type InitializeParams struct {
	ProcessID             interface{} `json:"processId"`
	RootPath              *string     `json:"rootPath,omitempty"`
	RootURI               *string     `json:"rootUri,omitempty"`
	ClientCapabilities    interface{} `json:"capabilities"`
	InitializationOptions interface{} `json:"initializationOptions,omitempty"`
}

type ServerCapabilities struct {
	HoverProvider                 bool        `json:"hoverProvider"`
	DefinitionProvider            bool        `json:"definitionProvider"`
	CompletionProvider            interface{} `json:"completionProvider,omitempty"`
	DiagnosticProvider            bool        `json:"diagnosticProvider,omitempty"`
	TextDocumentSyncKind          int         `json:"textDocumentSyncKind"`
	Workspace                     interface{} `json:"workspace,omitempty"`
	CodeActionProvider            bool        `json:"codeActionProvider,omitempty"`
	DocumentFormattingProvider    bool        `json:"documentFormattingProvider,omitempty"`
	DocumentRangeFormattingProvider bool      `json:"documentRangeFormattingProvider,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   interface{}        `json:"serverInfo,omitempty"`
}

// Document sync kinds
const (
	TextDocumentSyncNone       = 0
	TextDocumentSyncFull       = 1
	TextDocumentSyncIncremental = 2
)

// Document change
type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength int    `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// Signature help
type SignatureHelpParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *SignatureHelpContext  `json:"context,omitempty"`
}

type SignatureHelpContext struct {
	TriggerKind       int     `json:"triggerKind"`
	TriggerCharacter  string  `json:"triggerCharacter,omitempty"`
	IsRetrigger       bool    `json:"isRetrigger"`
	ActiveSignatureHelp *SignatureHelp `json:"activeSignatureHelp,omitempty"`
}

type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation string                 `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

type ParameterInformation struct {
	Label         string `json:"label"`
	Documentation string `json:"documentation,omitempty"`
}
