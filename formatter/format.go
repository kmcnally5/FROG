// Package formatter provides text-level formatting for kLex source code.
//
// ── Why this package lives at the repo root ────────────────────────────────
//
// `formatter/` is the SHARED LIBRARY. Two binaries import it:
//
//   • tools/klexfmt/main.go     →  builds to ./klexfmt   (CLI)
//   • snowball/froglsp/format.go →  builds to ./froglsp  (LSP server)
//
// This is the same library-plus-two-clients pattern Go itself uses
// (`go/format` package + `gofmt` CLI + `gopls` LSP). The package lives
// at the top level of the kLex module (`klex/formatter`) because that
// matches the rest of the language packages — `klex/lexer`,
// `klex/parser`, `klex/ast`, `klex/eval` are all top-level too.
// Moving it under `tools/` would break the import path and force a
// confusing "library lives next to one of its clients" layout.
//
// Editing the formatter therefore affects BOTH the CLI and the LSP at
// once — a `klexfmt -w foo.lex` run and an editor "Format Document"
// command on the same file produce byte-identical output, by
// construction.
//
// ── What this package does ─────────────────────────────────────────────────
//
// The public surface is intentionally tiny — Format(src) takes the raw
// bytes of a .lex file and returns the canonical-formatted version. It
// makes no I/O calls; callers (CLI tool, LSP server, future pre-commit
// hook) wrap it for their own contexts.
//
// What it does:
//
//   - Re-indents based on `{` / `[` nesting depth (4 spaces per level).
//     Curly braces (blocks, hash literals) and square brackets (array
//     literals) increment the depth; `(` / `)` are intentionally
//     ignored because kLex idiom indents closure bodies against the
//     enclosing scope, not the call's paren depth.
//   - Strips trailing whitespace on every line.
//   - Caps consecutive blank lines at 2 (matching gofmt — single blank
//     between siblings, double for section breaks).
//   - Ensures exactly one trailing newline.
//   - Normalises CRLF → LF.
//   - Preserves the author's leading whitespace on continuation lines
//     (lines following a `,` `(` `+` `=` etc.), so column-aligned
//     argument continuations survive intact.
//   - Knows about strings, comments, interpolation (`"{expr}"`), and
//     multi-line backtick raw strings — won't touch braces inside any
//     of those.
//
// What it does NOT do (these need an AST-based formatter — out of scope
// for this text-level version):
//
//   - Reflow long lines.
//   - Restructure multi-line hash/array literal layout.
//   - Normalise operator spacing.
//   - Reformat comment text.
//
// The formatter is idempotent: Format(Format(x)) == Format(x) for any
// input x.
package formatter

import "strings"

const indentWidth = 4

// Format applies the formatter rules to a full source buffer. Always
// returns a buffer ending in exactly one '\n' (or an empty buffer when
// the input contains no non-blank lines).
func Format(src []byte) []byte {
	// Normalise to \n line endings before splitting so Windows-edited files
	// don't produce CR artefacts in the output.
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	// strings.Split leaves an empty trailing element when the input ends
	// in '\n'. Track that explicitly so we can re-emit a single trailing
	// newline regardless.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var out strings.Builder
	depth := 0
	// Track how many consecutive blank lines we've emitted. Up to 2 are
	// preserved (matching gofmt's policy — single blank between siblings,
	// double blank as a visual section break). Runs of 3+ collapse to 2.
	consecutiveBlanks := 1 // start at 1 to suppress leading blanks
	emittedAny := false

	// prevIsContinuation tracks whether the *previous* non-blank line
	// ended with a continuation operator (`,`, `(`, `+`, `&&`, …). When
	// true, the current line is a continuation of an expression — most
	// authors align such lines against a column in the prior line
	// (against the open paren, against the first argument), which is
	// information a brace-depth-only formatter can't reproduce. We
	// preserve the author's leading whitespace verbatim in that case,
	// rather than imposing canonical indent.
	prevIsContinuation := false

	// inRawString tracks whether we're inside a backtick `…` raw string
	// that opened on a previous line and hasn't closed yet. While true,
	// every line is emitted VERBATIM (no trim, no re-indent) — the raw
	// string's contents are user data we mustn't touch.
	inRawString := false

	for _, line := range lines {
		if inRawString {
			out.WriteString(line)
			out.WriteByte('\n')
			emittedAny = true
			consecutiveBlanks = 0
			// Did this line close the raw string?
			if rawClosed := lineClosesRawString(line); rawClosed {
				inRawString = false
				// Net brace count *after* the closing backtick still matters
				// for indentation — handle the trailing fragment.
				if frag := afterRawClose(line); frag != "" {
					_, n := scanLine(frag)
					depth += n
					if depth < 0 {
						depth = 0
					}
				}
			}
			continue
		}

		// Trim ends — leading whitespace will be replaced by canonical indent.
		trimmed := strings.TrimRight(line, " \t")
		leading := strings.TrimLeft(trimmed, " \t")

		if leading == "" {
			// Blank line — allow up to 2 consecutive (one for sibling
			// separation, two for section break). Runs of 3+ collapse to 2.
			// Suppress every blank before the first real content line.
			if !emittedAny {
				continue
			}
			if consecutiveBlanks >= 2 {
				continue
			}
			out.WriteByte('\n')
			consecutiveBlanks++
			continue
		}
		consecutiveBlanks = 0

		if prevIsContinuation {
			// Continuation line — preserve the author's leading whitespace.
			// We can't compute the right column from brace depth alone
			// (it depends on the opener's column in the prior line),
			// and rewriting would actively make things worse for the
			// common `error("CODE",` / `"long message")` pattern.
			out.WriteString(trimmed)
			out.WriteByte('\n')
		} else {
			// Lines that begin with '}' or ']' un-indent before emission
			// so the closer lines up with its opener.
			lineDepth := depth
			if startsWithCloser(leading) {
				lineDepth = depth - 1
				if lineDepth < 0 {
					lineDepth = 0
				}
			}
			out.WriteString(strings.Repeat(" ", lineDepth*indentWidth))
			out.WriteString(leading)
			out.WriteByte('\n')
		}
		emittedAny = true

		// Adjust depth based on net braces on this line, ignoring those
		// inside strings or comments. If this line opens a raw string
		// that doesn't close, flip into raw-passthrough mode for
		// subsequent lines.
		opens, net := scanLine(leading)
		depth += net
		if depth < 0 {
			depth = 0
		}
		if opens {
			inRawString = true
		}

		// Set the continuation flag for the next iteration based on
		// whether THIS line ends with a continuation operator.
		prevIsContinuation = endsInContinuation(leading)
	}

	if !emittedAny {
		return []byte("")
	}

	// Normalise the file terminator: strip any number of trailing
	// newlines, then add exactly one. Catches the single-trailing-blank
	// case that the per-line collapse alone misses (because that blank
	// was a legitimate transition from non-blank → blank).
	result := strings.TrimRight(out.String(), "\n") + "\n"
	return []byte(result)
}

// startsWithCloser reports whether `leading` (a line with whitespace
// trimmed off both ends) begins with a closing punctuator that should
// un-indent the line before emission. Mirrors scanLine's depth set:
// '}' (block / hash) and ']' (array) only — ')' is intentionally
// excluded (see scanLine for the rationale).
func startsWithCloser(leading string) bool {
	if leading == "" {
		return false
	}
	c := leading[0]
	return c == '}' || c == ']'
}

// endsInContinuation reports whether `leading` ends with an operator or
// punctuator that implies the next line is a continuation of the same
// expression.
func endsInContinuation(leading string) bool {
	body := stripTrailingComment(leading)
	body = strings.TrimRight(body, " \t")
	if body == "" {
		return false
	}
	last := body[len(body)-1]
	switch last {
	case ',', '(', '[', '+', '-', '*', '/', '=', '&', '|':
		return true
	}
	return false
}

// stripTrailingComment removes any trailing `// …` comment from the
// line's text, respecting strings (a "//" inside a string literal is
// NOT a comment).
func stripTrailingComment(line string) string {
	i := 0
	for i < len(line) {
		c := line[i]
		switch c {
		case '"':
			i++
			for i < len(line) {
				if line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == '"' {
					i++
					break
				}
				if line[i] == '{' {
					br := 1
					i++
					for i < len(line) && br > 0 {
						if line[i] == '{' {
							br++
						} else if line[i] == '}' {
							br--
						}
						i++
					}
					continue
				}
				i++
			}
		case '`':
			i++
			for i < len(line) && line[i] != '`' {
				i++
			}
			if i < len(line) {
				i++
			}
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return line[:i]
			}
			i++
		default:
			i++
		}
	}
	return line
}

// scanLine walks a single source line, ignoring content inside double-
// quoted strings (with interpolation), backtick raw strings, and //
// line comments. Returns:
//
//   - opensRaw: true if this line opens a backtick raw string that
//     never closes on the same line — the caller flips into
//     raw-passthrough mode until lineClosesRawString fires.
//   - net:      net brace count (open '{' minus close '}') OUTSIDE
//     strings and comments, used to update the indent depth.
func scanLine(line string) (opensRaw bool, net int) {
	i := 0
	for i < len(line) {
		c := line[i]
		switch c {
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return false, net
			}
			i++
		case '"':
			i++
			for i < len(line) {
				if line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == '"' {
					i++
					break
				}
				if line[i] == '{' {
					br := 1
					i++
					for i < len(line) && br > 0 {
						if line[i] == '{' {
							br++
						} else if line[i] == '}' {
							br--
						}
						i++
					}
					continue
				}
				i++
			}
		case '`':
			i++
			for i < len(line) {
				if line[i] == '`' {
					i++
					return false, net
				}
				i++
			}
			return true, net
		case '{', '[':
			net++
			i++
		case '}', ']':
			net--
			i++
		case '(', ')':
			i++
		default:
			i++
		}
	}
	return false, net
}

// lineClosesRawString reports whether the line contains a backtick that
// closes a raw string opened on a previous line.
func lineClosesRawString(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			return true
		}
	}
	return false
}

// afterRawClose returns the portion of `line` AFTER the first backtick.
func afterRawClose(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			return line[i+1:]
		}
	}
	return ""
}
