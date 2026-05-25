package main

// format.go — LSP textDocument/formatting handler.
//
// ── Relationship to klexfmt ────────────────────────────────────────────────
//
// All formatting logic lives in `klex/formatter` (at the repo root).
// This file is the LSP front-end; tools/klexfmt is the CLI front-end.
// Both import the SAME Format() function, so editor "Format Document"
// commands and a `klexfmt -w foo.lex` terminal run produce
// byte-identical output. There is no second implementation to drift.
//
// See formatter/format.go for the formatting spec and the rationale
// for why that package lives at the repo root.

import (
	"encoding/json"
	"strings"

	"klex/formatter"
)

// handleFormatting answers a textDocument/formatting request. Returns a
// single TextEdit covering the entire document — the simplest valid
// shape, and the one VS Code / Neovim / Zed all handle without fuss.
//
// We deliberately don't honour the client's FormattingOptions (tabSize,
// insertSpaces). kLex has one canonical style (4 spaces, no tabs); the
// formatter package enforces it regardless of what the editor asks for.
func (s *Server) handleFormatting(msg *Message) {
	var params DocumentFormattingParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.transport.SendResponse(msg.ID, nil, &RPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		})
		return
	}

	s.mu.RLock()
	doc, ok := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()
	if !ok {
		// Unknown doc — clients sometimes ask before didOpen for new
		// files. Return an empty edits array (no-op) rather than an
		// error, which would surface as a popup in VS Code.
		s.transport.SendResponse(msg.ID, []TextEdit{}, nil)
		return
	}

	original := doc.Text
	formatted := string(formatter.Format([]byte(original)))

	if formatted == original {
		// Nothing changed — return empty edits so the client knows the
		// file is already canonical.
		s.transport.SendResponse(msg.ID, []TextEdit{}, nil)
		return
	}

	// One TextEdit replacing the entire document. The end Position
	// covers everything; we compute the last line number + the column
	// at end-of-line so the range exactly matches the original buffer.
	lines := strings.Split(original, "\n")
	lastLine := len(lines) - 1
	lastCol := len(lines[lastLine])

	edits := []TextEdit{
		{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: lastLine, Character: lastCol},
			},
			NewText: formatted,
		},
	}
	s.transport.SendResponse(msg.ID, edits, nil)
}
