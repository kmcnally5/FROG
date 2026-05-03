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
	"flag"
	"fmt"
	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"klex/repl"
	"os"
	"runtime"
	"runtime/pprof"
)

const Version = "v0.3.11x"

func main() {
	eval.KLexVersion = Version

	// Optimize for parallelism: empirically tuned to 12 based on GOMAXPROCS benchmarking.
	// This accounts for hyperthreading and scheduler oversubscription benefits.
	// Users can override via MAXPROCS environment variable.
	if os.Getenv("MAXPROCS") == "" {
		runtime.GOMAXPROCS(12)
	} else {
		var procs int
		if _, err := fmt.Sscanf(os.Getenv("MAXPROCS"), "%d", &procs); err == nil && procs > 0 {
			runtime.GOMAXPROCS(procs)
		}
	}

	cpuprofile := flag.String("cpuprofile", "", "write CPU profile to file")
	flag.Parse()

	// Start CPU profiling if requested.
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot create CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot start CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	// Get remaining positional arguments after flag parsing.
	args := flag.Args()

	// No arguments — launch the interactive REPL.
	if len(args) == 0 {
		repl.Start()
		return
	}

	// --version / -v — print version and exit.
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Println("kLex (FROG) " + Version)
		return
	}

	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: klex [file.lex]")
		os.Exit(1)
	}

	// One argument — run the given source file.
	path := args[0]
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
