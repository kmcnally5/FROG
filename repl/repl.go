package repl

// repl.go implements the kLex Read-Eval-Print Loop.
//
// The REPL lets you interact with kLex directly from the terminal without
// writing a file. Each line (or block) you type is lexed, parsed, and
// evaluated immediately, and the result is printed.
//
// The persistent Environment means variables you define in one line are
// available in all subsequent lines — the session accumulates state.
//
// Multi-line input:
// kLex uses { } for blocks. When you open a brace that isn't closed yet,
// the REPL switches to a continuation prompt ("...") and keeps collecting
// input until all braces and parentheses are balanced. Only then does it
// parse and evaluate the full block.
//
// Error recovery:
// Parse errors and runtime errors are printed but do NOT end the session.
// You can make a mistake and keep going, just like Python or Node.

import (
	"bufio"
	"fmt"
	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"os"
	"strings"
)

const (
	prompt       = ">> "
	continuation = ".. " // shown when a block is incomplete
)

// Start launches the REPL and runs until the user exits (Ctrl+C or Ctrl+D).
// It creates a single top-level Environment that persists for the entire session.
func Start() {
	env := eval.NewEnv()
	scanner := bufio.NewScanner(os.Stdin)
	var buf strings.Builder // accumulates multi-line input

	fmt.Println("kLex REPL — type 'exit' to quit")
	fmt.Print(prompt)

	for scanner.Scan() {
		line := scanner.Text()

		// Allow the user to exit cleanly.
		if strings.TrimSpace(line) == "exit" {
			fmt.Println("bye")
			return
		}

		buf.WriteString(line)
		buf.WriteByte('\n')

		input := buf.String()

		// Count unmatched braces and parens. If any are still open,
		// the user is mid-block — show the continuation prompt and
		// keep reading rather than trying to parse an incomplete program.
		if depth(input) > 0 {
			fmt.Print(continuation)
			continue
		}

		// We have a balanced input — parse and evaluate it.
		buf.Reset()

		src := strings.TrimSpace(input)
		if src == "" {
			fmt.Print(prompt)
			continue
		}

		l := lexer.New(src)
		p := parser.New(l)
		program := p.ParseProgram()

		// Report parse errors but stay alive — the session continues.
		if len(program.Errors) > 0 {
			for _, e := range program.Errors {
				fmt.Fprintf(os.Stderr, "ParseError: %s\n", e)
			}
			fmt.Print(prompt)
			continue
		}

		// Evaluate against the persistent session environment.
		// Runtime errors are printed by Eval itself and execution continues.
		eval.Eval(program, env)

		fmt.Print(prompt)
	}

	// scanner.Scan() returns false on EOF (Ctrl+D) — exit cleanly.
	fmt.Println()
}

// depth counts unmatched opening braces and parentheses in the input so far.
// A positive return value means the input is incomplete — at least one block
// or group is still open. Zero means the input is syntactically balanced
// (though not necessarily valid — that's the parser's job).
//
// This is intentionally simple: it counts characters, not tokens, so a { or (
// inside a string literal would be miscounted. For a REPL this is acceptable —
// the worst case is an extra line of input before the parse attempt.
func depth(input string) int {
	d := 0
	inString := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch == '"' && (i == 0 || input[i-1] != '\\') {
			inString = !inString
		}
		if inString {
			continue
		}
		if ch == '{' || ch == '(' {
			d++
		} else if ch == '}' || ch == ')' {
			d--
		}
	}
	if d < 0 {
		return 0 // more closing than opening — let the parser report the error
	}
	return d
}
