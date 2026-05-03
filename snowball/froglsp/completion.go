package main

import (
	"fmt"
	"strings"
)

// CompletionsAtPosition returns completions for the position in the document
func CompletionsAtPosition(doc *DocumentState, pos Position) *CompletionList {
	if doc.AST == nil || doc.Symbols == nil {
		return &CompletionList{
			IsIncomplete: false,
			Items:        []CompletionItem{},
		}
	}

	// Determine context: are we in a dot access?
	isAfterDot, moduleAlias := checkIfAfterDot(doc, pos)

	var items []CompletionItem

	if isAfterDot && moduleAlias != "" {
		// Completing module member (Phase 2: cross-file)
		// For now, just show nothing
		items = []CompletionItem{}
	} else {
		// Completing identifiers: show in-scope symbols + all builtins
		items = append(items, completionsFromSymbolTable(doc.Symbols)...)
		items = append(items, completionsFromBuiltins()...)
	}

	return &CompletionList{
		IsIncomplete: false,
		Items:        items,
	}
}

func checkIfAfterDot(doc *DocumentState, pos Position) (bool, string) {
	// Simple heuristic: check if there's a dot on this line before the cursor
	lines := strings.Split(doc.Text, "\n")
	if pos.Line < len(lines) {
		lineText := lines[pos.Line]
		if pos.Character > 0 && pos.Character-1 < len(lineText) && lineText[pos.Character-1] == '.' {
			// Find the identifier before the dot
			start := pos.Character - 2
			for start >= 0 && (isIdentChar(rune(lineText[start])) || lineText[start] == '_') {
				start--
			}
			start++
			if start < pos.Character-1 {
				moduleAlias := lineText[start : pos.Character-1]
				return true, moduleAlias
			}
		}
	}
	return false, ""
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func completionsFromSymbolTable(symtab *SymbolTable) []CompletionItem {
	var items []CompletionItem

	for name, sym := range symtab.Symbols {
		item := CompletionItem{
			Label:  name,
			Detail: symbolKindString(sym.Kind),
		}

		switch sym.Kind {
		case KindFunction:
			item.Kind = CompletionFunction
			item.Detail = fmt.Sprintf("fn(%s)", strings.Join(sym.Params, ", "))
		case KindVariable:
			item.Kind = CompletionVariable
			item.Detail = "variable"
		case KindConst:
			item.Kind = CompletionConstant
			item.Detail = "const"
		case KindModule:
			item.Kind = CompletionModule
			item.Detail = "module"
		case KindBuiltin:
			item.Kind = CompletionFunction
			item.Detail = "builtin"
		}

		items = append(items, item)
	}

	return items
}

func completionsFromBuiltins() []CompletionItem {
	var items []CompletionItem

	for name, info := range builtinSignatures {
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          CompletionFunction,
			Detail:        info.Signature,
			Documentation: info.Documentation,
		})
	}

	return items
}

func symbolKindString(kind SymbolKind) string {
	switch kind {
	case KindVariable:
		return "variable"
	case KindFunction:
		return "function"
	case KindModule:
		return "module"
	case KindBuiltin:
		return "builtin"
	case KindConst:
		return "const"
	default:
		return "unknown"
	}
}

// FilterCompletions filters completion items by prefix
func FilterCompletions(items []CompletionItem, prefix string) []CompletionItem {
	var filtered []CompletionItem
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), strings.ToLower(prefix)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
