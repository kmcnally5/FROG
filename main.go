package main

// main.go is the entry point for the kLex interpreter.
//
// The pipeline every interpreter follows:
//
//   source text
//       ↓
//   Lexer   — breaks raw text into a stream of tokens (words the language understands)
//       ↓
//   Parser  — reads tokens and builds an AST (a tree that represents the program's structure)
//       ↓
//   Eval    — walks the AST and actually executes each node
//
// kLex is a "tree-walking interpreter": there is no bytecode, no compilation step.
// The evaluator just recurses through the AST and computes values directly.
// This is the simplest possible interpreter architecture.
//
// Invocation:
//   klex           — start the interactive REPL
//   klex <file>    — run a .lex source file

import (
	"fmt"
	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"klex/repl"
	"os"
)

const Version = "v0.3.1"

func main() {
	eval.KLexVersion = Version

	// No arguments — launch the interactive REPL.
	if len(os.Args) == 1 {
		repl.Start()
		return
	}

	// --version / -v — print version and exit.
	if os.Args[1] == "--version" || os.Args[1] == "-v" {
		fmt.Println("kLex (FROG) " + Version)
		return
	}

	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: klex [file.lex]")
		os.Exit(1)
	}

	// One argument — run the given source file.
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}

	// Phase 1 & 2: lex and parse the entire source file into an AST.
	// The parser collects parse errors inside program.Errors rather than
	// panicking immediately — this lets it report multiple errors at once.
	l := lexer.New(string(src))
	p := parser.New(l)
	program := p.ParseProgram()

	// Check for parse errors BEFORE evaluating anything.
	// If the program is syntactically broken, nothing should run.
	// This matches the behaviour of every mainstream language interpreter.
	if len(program.Errors) > 0 {
		for _, e := range program.Errors {
			fmt.Fprintf(os.Stderr, "ParseError: %s\n", e)
		}
		os.Exit(1)
	}

	// Phase 3: evaluate the AST.
	// NewEnv() creates the top-level (global) variable scope.
	env := eval.NewEnv()
	result := eval.Eval(program, env)
	if eval.IsError(result) {
		os.Exit(1)
	}
}
