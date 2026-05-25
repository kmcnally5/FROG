package main

import (
	"fmt"
	"sort"
	"strings"
)

// DocumentSymbolsForDoc builds the hierarchical outline returned by
// textDocument/documentSymbol. The shape is the same one VS Code
// renders into its Outline view + breadcrumb bar.
//
// We emit a single flat list (no nesting) for now — kLex's surface
// language doesn't have classes/methods/etc, so a flat outline reads
// naturally. Constants are grouped under a single "Constants" parent
// only when there are ≥3 of them, otherwise inlined.
func DocumentSymbolsForDoc(doc *DocumentState) []DocumentSymbol {
	if doc == nil || doc.Symbols == nil || doc.AST == nil {
		return []DocumentSymbol{}
	}

	// Walk the symbol table in a stable order so the outline doesn't
	// reshuffle between edits — VS Code uses the order we return.
	names := make([]string, 0, len(doc.Symbols.Symbols))
	for n := range doc.Symbols.Symbols {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a := doc.Symbols.Symbols[names[i]]
		b := doc.Symbols.Symbols[names[j]]
		// Order by definition line, then by name to break ties.
		if a.DefPos.Line != b.DefPos.Line {
			return a.DefPos.Line < b.DefPos.Line
		}
		return names[i] < names[j]
	})

	var out []DocumentSymbol
	var consts []DocumentSymbol

	for _, name := range names {
		sym := doc.Symbols.Symbols[name]
		if sym == nil {
			continue
		}
		// Parameters never go in the outline — they belong to a
		// function's scope, not the document's top level.
		if sym.Kind == KindParameter {
			continue
		}
		// Builtins certainly don't.
		if sym.Kind == KindBuiltin {
			continue
		}

		ds := buildSymbolEntry(doc, name, sym)
		if ds == nil {
			continue
		}

		// Defer constants — we may collapse them under a single parent.
		if sym.Kind == KindConst {
			consts = append(consts, *ds)
			continue
		}
		out = append(out, *ds)
	}

	// Constants. Collapse if many; inline if few.
	if len(consts) >= 3 {
		// Range spans first to last const definition line.
		first := consts[0].Range.Start
		last := consts[len(consts)-1].Range.End
		out = append(out, DocumentSymbol{
			Name:           "Constants",
			Detail:         fmt.Sprintf("%d items", len(consts)),
			Kind:           SymbolKindNamespace,
			Range:          Range{Start: first, End: last},
			SelectionRange: Range{Start: first, End: first},
			Children:       consts,
		})
	} else {
		out = append(out, consts...)
	}

	// Also expose struct definitions even if they don't have a
	// matching entry in Symbols (which they do via KindVariable
	// holding the struct's type info, but some flows surface them
	// only via Symbols.Structs). Add anything missing.
	if doc.Symbols.Structs != nil {
		existing := make(map[string]bool, len(out))
		for _, e := range out {
			existing[e.Name] = true
		}
		for name, sd := range doc.Symbols.Structs {
			if existing[name] {
				continue
			}
			pos := Position{Line: sd.DefPos.Line - 1, Character: sd.DefPos.Col - 1}
			if pos.Line < 0 {
				pos.Line = 0
			}
			if pos.Character < 0 {
				pos.Character = 0
			}
			end := Position{Line: pos.Line, Character: pos.Character + len(name)}
			out = append(out, DocumentSymbol{
				Name:           name,
				Detail:         fmt.Sprintf("struct (%d fields)", len(sd.Fields)),
				Kind:           SymbolKindStruct,
				Range:          Range{Start: pos, End: end},
				SelectionRange: Range{Start: pos, End: end},
				Children:       structFieldEntries(sd),
			})
		}
	}

	// Final stable order — by start line.
	sort.Slice(out, func(i, j int) bool {
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})

	return out
}

// buildSymbolEntry converts one Symbol into the LSP DocumentSymbol shape.
// Returns nil when the symbol's DefPos is unusable (line < 1).
func buildSymbolEntry(doc *DocumentState, name string, sym *Symbol) *DocumentSymbol {
	if sym.DefPos.Line < 1 {
		return nil
	}
	// LSP positions are 0-based; kLex AST positions are 1-based.
	pos := Position{Line: sym.DefPos.Line - 1, Character: sym.DefPos.Col - 1}
	if pos.Character < 0 {
		pos.Character = 0
	}
	// Selection range covers just the identifier; the broader range
	// also includes the definition body so VS Code's breadcrumb
	// stays selected as the cursor moves through it.
	selectionEnd := Position{Line: pos.Line, Character: pos.Character + len(name)}

	entry := DocumentSymbol{
		Name:           name,
		Range:          Range{Start: pos, End: selectionEnd},
		SelectionRange: Range{Start: pos, End: selectionEnd},
	}

	switch sym.Kind {
	case KindFunction:
		entry.Kind = SymbolKindFunction
		entry.Detail = renderFunctionSignaturePreview(sym)
		// Extend Range to encompass the function body so VS Code's
		// breadcrumb stays anchored to the function as the cursor
		// moves through its body. lastLineOfFunctionBody returns the
		// AST line (1-based); convert to LSP-0-based before comparing
		// to pos.Line.
		if astEnd := lastLineOfFunctionBody(sym); astEnd > 0 {
			lspEnd := astEnd - 1
			if lspEnd > pos.Line {
				entry.Range.End = Position{Line: lspEnd, Character: 0}
			}
		}

	case KindVariable:
		entry.Kind = SymbolKindVariable
		if sym.Type != "" && sym.Type != "unknown" {
			entry.Detail = sym.Type
		}

	case KindConst:
		entry.Kind = SymbolKindConstant
		if sym.Type != "" && sym.Type != "unknown" {
			entry.Detail = sym.Type
		}

	case KindModule:
		entry.Kind = SymbolKindModule

	default:
		return nil
	}

	return &entry
}

// renderFunctionSignaturePreview is a compact param-list used as the
// `detail` shown in the outline view to the right of the name. It's
// intentionally lighter than renderFunctionSignature (which prints the
// full kLex signature for hover); the outline cares about a quick
// recognise-at-a-glance signature, not a full type annotation.
func renderFunctionSignaturePreview(sym *Symbol) string {
	if len(sym.Params) == 0 {
		return "()"
	}
	var parts []string
	for i, p := range sym.Params {
		if sym.Variadic && i == len(sym.Params)-1 {
			parts = append(parts, "..."+p)
		} else if i < len(sym.Defaults) && sym.Defaults[i] {
			parts = append(parts, p+"?")
		} else {
			parts = append(parts, p)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// lastLineOfFunctionBody walks the function body looking for the
// highest line number reached by any descendant node, so the outline
// entry's Range covers the whole function. Returns 0 if the body is
// nil or no positioned node is found (caller falls back to def line).
func lastLineOfFunctionBody(sym *Symbol) int {
	if sym.Body == nil {
		return 0
	}
	maxLine := 0
	for _, stmt := range sym.Body {
		if pos, ok := getNodePos(stmt); ok && pos.Line > maxLine {
			maxLine = pos.Line
		}
	}
	if maxLine == 0 {
		return 0
	}
	return maxLine // already 1-based; subtract on assignment by caller
}

// structFieldEntries turns a struct definition's fields into child
// outline entries so the user can expand a struct in VS Code's outline
// and jump to individual fields. Field positions aren't tracked
// per-field in the kLex AST, so we co-locate them at the struct's
// declaration line for now.
func structFieldEntries(sd *StructDef) []DocumentSymbol {
	if sd == nil || len(sd.Fields) == 0 {
		return nil
	}
	out := make([]DocumentSymbol, 0, len(sd.Fields))
	line := sd.DefPos.Line - 1
	if line < 0 {
		line = 0
	}
	for _, f := range sd.Fields {
		p := Position{Line: line, Character: 0}
		end := Position{Line: line, Character: len(f)}
		out = append(out, DocumentSymbol{
			Name:           f,
			Kind:           SymbolKindField,
			Range:          Range{Start: p, End: end},
			SelectionRange: Range{Start: p, End: end},
		})
	}
	return out
}
