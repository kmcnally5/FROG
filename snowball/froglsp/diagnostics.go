package main

import (
	"fmt"
	"klex/ast"
	"regexp"
	"strconv"
	"strings"
)

// DiagnosticsFromProgram converts parser errors to LSP diagnostics
// and ALSO runs the static-analysis lint pass over the AST. Parser
// errors keep severity Error; lint warnings get Warning, and stylistic
// hints get Hint (or Information). Each lint result carries a stable
// Code so code actions can match against specific fixes.
func DiagnosticsFromProgram(program *ast.Program) []Diagnostic {
	var diags []Diagnostic

	if program == nil {
		return diags
	}

	// 1. Parser errors first (severity Error). These already carry
	//    line/col in their formatted string.
	for _, errStr := range program.Errors {
		diag := parseErrorString(errStr)
		if diag != nil {
			diags = append(diags, *diag)
		}
	}

	// 2. Lint pass over the AST. Only runs if the parser produced an
	//    AST — there's no point linting nonsense.
	diags = append(diags, LintDiagnostics(program)...)

	return diags
}

// parseErrorString parses "line:col: message" format
func parseErrorString(errStr string) *Diagnostic {
	// Format: "line:col: message"
	parts := strings.SplitN(errStr, ": ", 2)
	if len(parts) < 2 {
		return nil
	}

	posStr := parts[0]
	message := parts[1]

	// Parse "line:col"
	posParts := strings.SplitN(posStr, ":", 2)
	if len(posParts) < 2 {
		return nil
	}

	line, err1 := strconv.Atoi(posParts[0])
	col, err2 := strconv.Atoi(posParts[1])
	if err1 != nil || err2 != nil {
		return nil
	}

	// Convert to 0-based
	line--
	col--

	return &Diagnostic{
		Range: Range{
			Start: Position{Line: line, Character: col},
			End:   Position{Line: line, Character: col + 1},
		},
		Severity: DiagnosticError,
		Source:   "klex",
		Message:  message,
	}
}

// RuntimeErrorDiagnostic creates a diagnostic from a runtime error (Phase 2)
func RuntimeErrorDiagnostic(line int, col int, message string) Diagnostic {
	return Diagnostic{
		Range: Range{
			Start: Position{Line: line - 1, Character: col - 1},
			End:   Position{Line: line - 1, Character: col},
		},
		Severity: DiagnosticError,
		Source:   "klex",
		Message:  message,
	}
}

// WarningDiagnostic creates a warning diagnostic
func WarningDiagnostic(line int, col int, message string) Diagnostic {
	return Diagnostic{
		Range: Range{
			Start: Position{Line: line - 1, Character: col - 1},
			End:   Position{Line: line - 1, Character: col},
		},
		Severity: DiagnosticWarning,
		Source:   "klex",
		Message:  message,
	}
}

// HintDiagnostic creates a hint diagnostic
func HintDiagnostic(line int, col int, message string) Diagnostic {
	return Diagnostic{
		Range: Range{
			Start: Position{Line: line - 1, Character: col - 1},
			End:   Position{Line: line - 1, Character: col},
		},
		Severity: DiagnosticHint,
		Source:   "klex",
		Message:  message,
	}
}

// LintDiagnostics runs a structural lint pass over the parsed AST.
// Each lint produces diagnostics with a stable Code that codeAction
// matches against to surface quick-fixes.
//
// Codes published here:
//   PUSH_ANTIPATTERN  — calls to push() in any loop body, suggesting
//                       makeArray() + index instead (O(n) vs O(n²))
//   SHADOWED_CONST    — `let` or `=` to a name previously declared
//                       with `const` (kLex enforces this at runtime
//                       but flagging it at edit-time is friendlier)
//   EMPTY_BLOCK       — empty function or block bodies — likely
//                       unfinished code
//
// The pass is intentionally conservative: false positives erode the
// signal. Anything that requires deep dataflow (unused variables that
// might be referenced through tuple unpacking, for instance) is
// deferred to a future iteration.
func LintDiagnostics(program *ast.Program) []Diagnostic {
	var diags []Diagnostic
	if program == nil {
		return diags
	}

	// Track const names declared at file scope so we can flag re-binds.
	consts := map[string]ast.Pos{}
	for _, stmt := range program.Statements {
		if cs, ok := stmt.(*ast.ConstStmt); ok {
			consts[cs.Name] = cs.Pos
		}
	}

	// Custom walker. We can't use walkAST2 here because it visits a
	// node AND recurses into children — there's no "enter / leave"
	// hook, so we can't track the inLoop counter cleanly. Explicit
	// recursion lets every loop-context lint be precise.
	//
	// Nested functions reset inLoop: a push() inside a function
	// literal that just happens to be assigned inside a while-loop
	// is NOT itself "inside a loop" in the FROG sense — the function
	// hasn't been called yet at that point.
	var inLoop int
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if n == nil {
			return
		}
		switch node := n.(type) {

		case *ast.WhileStmt:
			walk(node.Condition)
			inLoop++
			for _, s := range node.Body {
				walk(s)
			}
			inLoop--

		case *ast.ForInStmt:
			walk(node.Collection)
			inLoop++
			for _, s := range node.Body {
				walk(s)
			}
			inLoop--

		case *ast.IfStmt:
			walk(node.Condition)
			for _, s := range node.Body {
				walk(s)
			}
			for _, s := range node.ElseBody {
				walk(s)
			}

		case *ast.SwitchStmt:
			walk(node.Subject)
			for _, c := range node.Cases {
				for _, s := range c.Body {
					walk(s)
				}
			}
			for _, s := range node.Default {
				walk(s)
			}

		case *ast.FunctionLiteral:
			if len(node.Body) == 0 {
				diags = append(diags, lintDiagnostic(
					node.Pos,
					DiagnosticHint,
					"EMPTY_BODY",
					"function body is empty",
					"fn",
				))
			}
			// Lexical-scope reset — nested fn's body is not "inside"
			// the outer loop. Save + restore.
			outer := inLoop
			inLoop = 0
			for _, s := range node.Body {
				walk(s)
			}
			inLoop = outer

		case *ast.CallExpr:
			if inLoop > 0 {
				if ident, ok := node.Function.(*ast.Ident); ok && ident.Value == "push" {
					diags = append(diags, lintDiagnostic(
						node.Pos,
						DiagnosticWarning,
						"PUSH_ANTIPATTERN",
						"push() inside a loop is O(n²) — pre-allocate with `makeArray(n, default)` and index by position",
						"push",
					))
				}
			}
			walk(node.Function)
			for _, a := range node.Args {
				walk(a)
			}

		case *ast.AssignStmt:
			if pos, ok := consts[node.Name]; ok && (pos.Line != node.Pos.Line || pos.Col != node.Pos.Col) {
				diags = append(diags, lintDiagnostic(
					node.Pos,
					DiagnosticWarning,
					"SHADOWED_CONST",
					fmt.Sprintf("`%s` is declared `const` at line %d — reassignment will be rejected at runtime", node.Name, pos.Line),
					node.Name,
				))
			}
			walk(node.Value)

		case *ast.LetStmt:
			if pos, ok := consts[node.Name]; ok && (pos.Line != node.Pos.Line || pos.Col != node.Pos.Col) {
				diags = append(diags, lintDiagnostic(
					node.Pos,
					DiagnosticWarning,
					"SHADOWED_CONST",
					fmt.Sprintf("`%s` is declared `const` at line %d — shadowing is allowed but probably unintentional", node.Name, pos.Line),
					node.Name,
				))
			}
			walk(node.Value)

		case *ast.ConstStmt:
			walk(node.Value)

		case *ast.ReturnStmt:
			walk(node.Value)

			// Other nodes (Ident, literals, BinaryExpr, etc) need no
			// special handling for the current lint set. We could add
			// targeted recursion (e.g. into BinaryExpr.Left/Right)
			// when future lints care about deeper expression shapes.
		}
	}

	for _, stmt := range program.Statements {
		walk(stmt)
	}

	return diags
}

// lintDiagnostic is the canonical constructor for lint output. The
// `targetText` argument is the literal token the lint highlights; the
// range spans len(targetText) characters from the position so the
// editor's squiggle hugs the identifier rather than just a one-char
// caret.
func lintDiagnostic(pos ast.Pos, severity int, code, message, targetText string) Diagnostic {
	line := pos.Line - 1
	col := pos.Col - 1
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	width := len(targetText)
	if width <= 0 {
		width = 1
	}
	return Diagnostic{
		Range: Range{
			Start: Position{Line: line, Character: col},
			End:   Position{Line: line, Character: col + width},
		},
		Severity: severity,
		Code:     code,
		Source:   "klex-lint",
		Message:  message,
	}
}

// FindErrorAtPosition finds diagnostics at a given position (Phase 2)
func FindErrorAtPosition(diags []Diagnostic, line int, col int) *Diagnostic {
	for i := range diags {
		d := &diags[i]
		if d.Range.Start.Line == line && d.Range.Start.Character <= col && col < d.Range.End.Character {
			return d
		}
	}
	return nil
}

// ExtractLineCol extracts line/col from error strings using regex
func ExtractLineCol(errStr string) (line int, col int, message string) {
	// Match "number:number: .*"
	re := regexp.MustCompile(`^(\d+):(\d+):\s+(.*)$`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) < 4 {
		return 0, 0, errStr
	}

	line, _ = strconv.Atoi(matches[1])
	col, _ = strconv.Atoi(matches[2])
	message = matches[3]
	return
}

// CodeActionForDiagnostic suggests code actions for a diagnostic (Phase 2)
func CodeActionForDiagnostic(diag Diagnostic) []interface{} {
	// TODO: quick fixes
	return nil
}

// FormatDiagnosticMessage formats a diagnostic for display
func FormatDiagnosticMessage(diag Diagnostic) string {
	return fmt.Sprintf("[%s] %s (%d:%d)",
		severityName(diag.Severity),
		diag.Message,
		diag.Range.Start.Line+1,
		diag.Range.Start.Character+1,
	)
}

func severityName(sev int) string {
	switch sev {
	case DiagnosticError:
		return "ERROR"
	case DiagnosticWarning:
		return "WARN"
	case DiagnosticInformation:
		return "INFO"
	case DiagnosticHint:
		return "HINT"
	default:
		return "UNKNOWN"
	}
}
