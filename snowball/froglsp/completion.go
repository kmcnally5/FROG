package main

import (
	"fmt"
	"strings"
)

// CompletionsAtPosition returns completions for the position in the document.
// Three sources are merged: language snippets (highest priority via SortText
// prefix "0_"), in-scope user symbols (prefix "1_"), and builtins (prefix
// "2_"). VS Code sorts by SortText then label, so user-defined names appear
// above builtins of the same prefix.
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
		// Module-member completion: cross-file resolution would go
		// here. Until that's wired, return an empty list — VS Code
		// will fall back to its word-list fallback, which is at least
		// not actively wrong.
		items = []CompletionItem{}
	} else {
		// Snippets (FROG language idioms) ALWAYS surface — even with
		// an empty prefix — so new users discover the canonical form
		// of `fn`, `if`, `for-in`, `struct`, etc by Tab-completion.
		items = append(items, languageSnippets()...)
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
		// Skip params and builtins (handled separately) — they don't
		// belong in the user-symbol completion list.
		if sym.Kind == KindParameter || sym.Kind == KindBuiltin {
			continue
		}
		item := CompletionItem{
			Label:    name,
			SortText: "1_" + name, // user symbols sort above builtins
		}

		switch sym.Kind {
		case KindFunction:
			item.Kind = CompletionFunction
			item.Detail = "fn " + renderFunctionSignaturePreview(sym)
			// Generate a snippet that fills in the call with placeholders
			// for each parameter, jumping between them with Tab. Skip
			// snippet form when there are no params — a plain insertion
			// is faster.
			if len(sym.Params) > 0 {
				item.InsertText = renderCallSnippet(name, sym.Params, sym.Variadic)
				item.InsertTextFormat = InsertTextFormatSnippet
			}
			item.Documentation = &MarkupContent{
				Kind:  "markdown",
				Value: renderSymbolHover(sym),
			}

		case KindVariable:
			item.Kind = CompletionVariable
			item.Detail = "var"
			if sym.Type != "" && sym.Type != "unknown" {
				item.Detail = "var · " + sym.Type
			}

		case KindConst:
			item.Kind = CompletionConstant
			item.Detail = "const"
			if sym.Type != "" && sym.Type != "unknown" {
				item.Detail = "const · " + sym.Type
			}

		case KindModule:
			item.Kind = CompletionModule
			item.Detail = "module"
		}

		items = append(items, item)
	}

	return items
}

func completionsFromBuiltins() []CompletionItem {
	var items []CompletionItem

	for name, info := range builtinSignatures {
		cat := categoryFor(name)
		// Snippet-form insertion: fills paren args with TabStops so the
		// user can fly through them. For builtins with no params just
		// insert "name()" with the cursor parked between the parens.
		insert := name + "()"
		insertFmt := InsertTextFormatPlainText
		if len(info.Params) > 0 {
			insert = renderCallSnippet(name, info.Params, false)
			insertFmt = InsertTextFormatSnippet
		}

		items = append(items, CompletionItem{
			Label:            name,
			Kind:             CompletionFunction,
			Detail:           cat.Display + " · " + prettySignature(info.Signature),
			SortText:         "2_" + name,
			InsertText:       insert,
			InsertTextFormat: insertFmt,
			Documentation: &MarkupContent{
				Kind:  "markdown",
				Value: renderBuiltinHover(name, info),
			},
		})
	}

	return items
}

// renderCallSnippet builds an LSP snippet string of the form
// `name(${1:p1}, ${2:p2}, $0)` so the user Tabs through the arguments
// and ends with the cursor outside the parens. Variadic parameters
// drop the `...` prefix in the snippet placeholder text — VS Code
// users find the prefix distracting in the active hint.
func renderCallSnippet(name string, params []string, variadic bool) string {
	if len(params) == 0 {
		return name + "($0)"
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("(")
	for i, p := range params {
		label := strings.TrimPrefix(p, "...")
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("${%d:%s}", i+1, label))
	}
	b.WriteString(")$0")
	return b.String()
}

// languageSnippets returns the 12 canonical kLex idioms as snippet
// completion items. They sort above everything else (`0_` prefix), so
// when the user types `fn ` the function-skeleton snippet is the top
// suggestion. The snippets encode the FROG style (no implicit
// coercion, explicit safe() wrapping, no push() in loops, etc).
func languageSnippets() []CompletionItem {
	type snip struct {
		label, detail, body, summary string
	}
	snips := []snip{
		{
			label:   "fn",
			detail:  "function declaration",
			summary: "Named function. The canonical kLex declaration form.",
			body:    "fn ${1:name}(${2:args}) {\n\t${0}\n}",
		},
		{
			label:   "fn-anon",
			detail:  "anonymous function literal",
			summary: "Inline function value. Used with map / filter / async / safe.",
			body:    "fn(${1:args}) {\n\t${0}\n}",
		},
		{
			label:   "if",
			detail:  "if statement",
			summary: "Boolean condition required — kLex has no truthy-int coercion.",
			body:    "if ${1:cond} {\n\t${0}\n}",
		},
		{
			label:   "if-else",
			detail:  "if / else statement",
			summary: "Branching with explicit alternative.",
			body:    "if ${1:cond} {\n\t${2}\n} else {\n\t${0}\n}",
		},
		{
			label:   "while",
			detail:  "while loop",
			summary: "Loop while condition is true.",
			body:    "while ${1:cond} {\n\t${0}\n}",
		},
		{
			label:   "for-in",
			detail:  "for-in iteration",
			summary: "Iterate elements. Use the two-binding form `for i, v in arr` for indices.",
			body:    "for ${1:item} in ${2:collection} {\n\t${0}\n}",
		},
		{
			label:   "struct",
			detail:  "struct declaration",
			summary: "Nominal record type. Fields are positional at construction.",
			body:    "struct ${1:Name} {\n\t${2:field1}, ${3:field2}\n}",
		},
		{
			label:   "enum",
			detail:  "enum (sum type)",
			summary: "Variants carry their own fields. Match with `switch ... case`.",
			body:    "enum ${1:Name} {\n\t${2:Variant1}\n\t${3:Variant2}(${4:field})\n}",
		},
		{
			label:   "switch",
			detail:  "switch / case",
			summary: "First case wins. No fallthrough.",
			body:    "switch ${1:value} {\n\tcase ${2:pattern} { ${3} }\n\tdefault { ${0} }\n}",
		},
		{
			label:   "safe",
			detail:  "safe() error guard",
			summary: "Wraps a fn that may error. Returns (value, err) tuple.",
			body:    "${1:result}, ${2:err} = safe(fn() { return ${0} })",
		},
		{
			label:   "import",
			detail:  "import as alias",
			summary: "Module import. Members are accessed via `alias.name`.",
			body:    "import \"${1:path/to/module.lex}\" as ${2:alias}",
		},
		{
			label:   "async",
			detail:  "async goroutine",
			summary: "Spawn concurrent work. Send results back through a channel — globals are snapshot, not shared.",
			body:    "async(fn() {\n\t${0}\n})",
		},
	}

	items := make([]CompletionItem, 0, len(snips))
	for _, s := range snips {
		items = append(items, CompletionItem{
			Label:            s.label,
			Kind:             CompletionSnippet,
			Detail:           "snippet · " + s.detail,
			SortText:         "0_" + s.label, // sort first
			InsertText:       s.body,
			InsertTextFormat: InsertTextFormatSnippet,
			Documentation: &MarkupContent{
				Kind:  "markdown",
				Value: "**kLex snippet · " + s.detail + "**\n\n" + s.summary + "\n\n```klex\n" + snippetPreview(s.body) + "\n```",
			},
		})
	}
	return items
}

// snippetPreview strips the LSP snippet tab-stop syntax (${N:label} /
// ${N} / $0) from a body so the hover Documentation can show what the
// snippet expands to without the placeholder markers cluttering the
// preview.
func snippetPreview(body string) string {
	out := body
	for {
		i := strings.Index(out, "${")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}")
		if j < 0 {
			break
		}
		// ${1:foo} → foo (preserve label after colon when present)
		inner := out[i+2 : i+j]
		label := inner
		if colon := strings.Index(inner, ":"); colon >= 0 {
			label = inner[colon+1:]
		}
		out = out[:i] + label + out[i+j+1:]
	}
	out = strings.ReplaceAll(out, "$0", "")
	return out
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
