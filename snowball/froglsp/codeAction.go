package main

import (
	"fmt"
	"strings"
)

// CodeActionsForRange returns the lightbulb suggestions VS Code shows
// next to a diagnostic. We match against the diagnostic Code values
// emitted by LintDiagnostics — each known code gets a canned fix
// (or an explainer if no auto-fix is safe).
//
// Two flavours of action are emitted:
//   QuickFix    — a concrete WorkspaceEdit that flips the offending
//                 code to a known-good shape (preferred when safe).
//   Explainer   — text-only action ("Open kLex style guide …") for
//                 cases where the right fix needs human judgement.
//
// Returns an empty slice — never nil — so VS Code consistently
// reports "no quick fixes available" when nothing matches.
func CodeActionsForRange(doc *DocumentState, _ Range, diags []Diagnostic) []CodeAction {
	actions := []CodeAction{}
	if doc == nil {
		return actions
	}

	for _, diag := range diags {
		switch diag.Code {

		case "PUSH_ANTIPATTERN":
			// Suggested rewrite is structural (push() → makeArray +
			// index), which we can't do in-place without knowing how
			// many iterations the loop runs. We surface an EXPLAINER
			// action that walks the user to the docs, and a TEMPLATE
			// action that drops a canonical pre-allocated pattern at
			// the cursor so they can adapt it.
			actions = append(actions, CodeAction{
				Title:       "Replace `push()` with pre-allocated `makeArray(n, default)` (kLex idiom)",
				Kind:        CodeActionKindQuickFix,
				Diagnostics: []Diagnostic{diag},
				IsPreferred: true,
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						doc.URI: {
							{
								// Insert the canonical pattern as a
								// comment block ABOVE the offending
								// call. The user keeps their original
								// code visible while comparing.
								Range: Range{
									Start: Position{Line: diag.Range.Start.Line, Character: 0},
									End:   Position{Line: diag.Range.Start.Line, Character: 0},
								},
								NewText: pushAntipatternTemplate(),
							},
						},
					},
				},
			})

		case "SHADOWED_CONST":
			// Auto-fix: prepend an underscore to the new binding's
			// name to make the shadowing intentional. Conservative —
			// only fires when the diagnostic name is unambiguous (we
			// captured it in the message).
			name := extractConstName(diag.Message)
			if name == "" {
				continue
			}
			actions = append(actions, CodeAction{
				Title:       fmt.Sprintf("Rename `%s` → `_%s` to make shadowing explicit", name, name),
				Kind:        CodeActionKindQuickFix,
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						doc.URI: {
							{
								Range:   diag.Range,
								NewText: "_" + name,
							},
						},
					},
				},
			})

		case "EMPTY_BODY":
			// Drop a stub `return null` inside the body so the function
			// at least has a defined return value.
			actions = append(actions, CodeAction{
				Title:       "Insert `return null` stub",
				Kind:        CodeActionKindQuickFix,
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{
					Changes: map[string][]TextEdit{
						doc.URI: {
							{
								Range: Range{
									Start: Position{Line: diag.Range.End.Line, Character: 0},
									End:   Position{Line: diag.Range.End.Line, Character: 0},
								},
								NewText: "    return null\n",
							},
						},
					},
				},
			})
		}
	}

	return actions
}

// pushAntipatternTemplate returns the canonical kLex pattern that
// replaces a push() inside a loop. Indentation matches the surrounding
// code's left margin — VS Code's quick-fix inserter doesn't auto-
// indent, so we hard-tab the two helper lines.
func pushAntipatternTemplate() string {
	return strings.Join([]string{
		"    // kLex idiom: pre-allocate then index by position (O(n)).",
		"    // out = makeArray(n, default)",
		"    // out[i] = value      // instead of out = push(out, value)",
		"",
	}, "\n")
}

// extractConstName pulls the const identifier out of the SHADOWED_CONST
// diagnostic's message format, which is:
//     `name` is declared `const` at line N — …
// Returns "" when the message doesn't match (defensive — the lint
// could be reworded later and we shouldn't crash).
func extractConstName(msg string) string {
	if !strings.HasPrefix(msg, "`") {
		return ""
	}
	end := strings.Index(msg[1:], "`")
	if end <= 0 {
		return ""
	}
	return msg[1 : 1+end]
}
