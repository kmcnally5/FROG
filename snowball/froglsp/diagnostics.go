package main

import (
	"fmt"
	"klex/ast"
	"regexp"
	"strconv"
	"strings"
)

// DiagnosticsFromProgram converts parser errors to LSP diagnostics
func DiagnosticsFromProgram(program *ast.Program) []Diagnostic {
	var diags []Diagnostic

	if program == nil || len(program.Errors) == 0 {
		return diags
	}

	for _, errStr := range program.Errors {
		diag := parseErrorString(errStr)
		if diag != nil {
			diags = append(diags, *diag)
		}
	}

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

// LintDiagnostics checks for common issues (Phase 2 bonus)
func LintDiagnostics(program *ast.Program) []Diagnostic {
	var diags []Diagnostic
	// TODO: unused variables, unreachable code, etc.
	return diags
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
