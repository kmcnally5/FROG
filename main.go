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
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"syscall"
)

func init() {
	// OpenGL requires all GL calls on the main OS thread (mandatory on macOS).
	runtime.LockOSThread()
}

const Version = "v0.3.35"

// printUsage writes the kLex command-line help to stderr. Registered as
// flag.Usage so it is also invoked when an unknown flag is encountered.
func printUsage() {
	fmt.Fprintf(os.Stderr, `kLex (FROG) %s — a pure, strict-typed scripting language with
built-in concurrency, graphics, UI widgets, and native bridges.

USAGE
  klex [options] <script.lex> [script-args...]
  klex                          start the interactive REPL
  klex --version                print version and exit

OPTIONS
  -h, --help            show this help and exit
  -v, --version         print version and exit
  --cpuprofile <file>   write a CPU profile to <file> (for go tool pprof)

ENVIRONMENT
  KLEX_PATH    Directory containing stdlib/ for import resolution.
               Optional — kLex also finds stdlib next to the binary and
               next to the script that is doing the importing.
  MAXPROCS     Override GOMAXPROCS (default 12).

IMPORT PATH RESOLUTION
  When a script does:  import "stdlib/foo.lex" as foo
  kLex tries, in order, the first existing file:
    1. ./stdlib/foo.lex                  (current working directory)
    2. <script-dir>/stdlib/foo.lex       (next to the importing .lex file)
    3. $KLEX_PATH/stdlib/foo.lex         (user override)
    4. <klex-bin-dir>/stdlib/foo.lex     (drop-in install: klex + stdlib together)
    5. <klex-bin-parent>/stdlib/foo.lex  (bin/klex + share/klex/stdlib style)
  On failure the error lists every path tried.

EXAMPLES
  klex test.lex                   run a script in the current directory
  klex tests/unit/jsonTest.lex    run any path
  klex /full/path/to/script.lex   run from anywhere — paths resolve
                                  relative to the script and the binary
  klex --cpuprofile=cpu.prof big.lex
                                  profile a heavy run
  klex                            start the interactive REPL

`, Version)
}

func main() {
	eval.KLexVersion = Version

	// Bridge subprocess cleanup. Two mechanisms together cover every exit path:
	//
	//   1. defer — handles the normal return from main() (REPL quit, script
	//      completion). os.Exit() bypasses defer, so this alone is insufficient.
	//   2. signal handler — handles Ctrl+C, kill, terminal close. Without this,
	//      a kLex hit with SIGINT during a long bridge call would leave the
	//      Python/external process orphaned and consuming resources.
	defer eval.CleanupAllBridges()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		eval.CleanupAllBridges()
		// Conventional exit code for SIGINT: 130 = 128 + SIGINT(2).
		os.Exit(130)
	}()

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

	var showHelp, showVersion bool
	flag.BoolVar(&showHelp, "help", false, "show this help and exit")
	flag.BoolVar(&showHelp, "h", false, "show this help and exit (short)")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit (short)")
	cpuprofile := flag.String("cpuprofile", "", "write CPU profile to file")
	flag.Usage = printUsage
	flag.Parse()

	if showHelp {
		printUsage()
		return
	}
	if showVersion {
		fmt.Println("kLex (FROG) " + Version)
		return
	}

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

	// First argument is the file path; remaining arguments are passed to the script.
	path := args[0]
	scriptArgs := args[1:]

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

	// Record the script's directory so `import` resolves paths next to the
	// entry file, not just CWD. Use the absolute path so the resolver isn't
	// confused by `..` or other CWD-relative quirks in the supplied path.
	if abs, absErr := filepath.Abs(path); absErr == nil {
		env.SetScriptDir(filepath.Dir(abs))
	} else {
		env.SetScriptDir(filepath.Dir(path))
	}

	// Set the __args__ variable with command-line arguments passed to the script.
	argsArray := make([]eval.Object, len(scriptArgs))
	for i, arg := range scriptArgs {
		argsArray[i] = &eval.String{Value: arg}
	}
	env.Set("__args__", &eval.Array{Elements: argsArray})

	result := eval.Eval(program, env)
	if eval.IsError(result) {
		// Manually drain bridges before exiting — os.Exit() bypasses the
		// deferred CleanupAllBridges at the top of main().
		eval.CleanupAllBridges()
		os.Exit(1)
	}
}
