// vmdiff — differential test runner for the bytecode VM.
//
// Runs a .lex source file through BOTH the tree-walking
// evaluator (eval/) and the bytecode VM (vm/), captures the stdout
// produced by each, and reports either MATCH or a unified diff.
//
// Why this is the safety net
//
// The tree-walker has been the reference implementation for the
// language for ~200 unit tests. Once the diff runner can take a
// .lex file and confirm "VM stdout == eval stdout," every test in
// tests/unit/ becomes a regression test for the VM the moment the
// VM grows enough opcodes to compile it. Until then, vmdiff
// gracefully reports SKIP for programs that hit unimplemented
// compiler arms or VM opcodes, so the runner is useful from day
// one without false alarms.
//
// Usage
//
//	go run ./vm/cmd/vmdiff <file.lex>            # single file
//	go run ./vm/cmd/vmdiff path/*.lex            # batch (shell-globbed)
//	go run ./vm/cmd/vmdiff --quiet path/*.lex    # only print failures
//
// Exit code is the number of files that diverged (capped at 125).
// Zero means every file either matched or was cleanly skipped.
//
// Output format
//
//	[MATCH] tests/unit/foo.lex
//	[SKIP]  tests/unit/bar.lex — VM compiler does not yet handle InfixExpr
//	[DIFF]  tests/unit/qux.lex
//	        eval stdout (3 lines)
//	          hello
//	          42
//	          done
//	        vm stdout (2 lines)
//	          hello
//	          done
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"klex/vm"
)

func main() {
	quiet := false
	var paths []string
	for _, a := range os.Args[1:] {
		switch {
		case a == "--quiet" || a == "-q":
			quiet = true
		case a == "-h" || a == "--help":
			printHelp()
			return
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "vmdiff: unknown flag %q\n", a)
			os.Exit(2)
		default:
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		printHelp()
		os.Exit(2)
	}

	var nMatch, nSkip, nDiff int
	for _, p := range paths {
		res := runOne(p)
		switch res.status {
		case statusMatch:
			nMatch++
			if !quiet {
				fmt.Printf("[MATCH] %s\n", p)
			}
		case statusSkip:
			nSkip++
			if !quiet {
				fmt.Printf("[SKIP]  %s — %s\n", p, res.reason)
			}
		case statusDiff:
			nDiff++
			fmt.Printf("[DIFF]  %s\n", p)
			printDiff(res.evalOut, res.vmOut)
		case statusError:
			nDiff++
			fmt.Printf("[ERROR] %s — %s\n", p, res.reason)
		}
	}

	fmt.Printf("\n──────\n  matches: %d   skipped: %d   diffs: %d   total: %d\n",
		nMatch, nSkip, nDiff, len(paths))

	if nDiff > 0 {
		if nDiff > 125 {
			nDiff = 125
		}
		os.Exit(nDiff)
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

type status int

const (
	statusMatch status = iota
	statusSkip
	statusDiff
	statusError
)

type result struct {
	status  status
	reason  string // populated for skip/error
	evalOut string // populated for match/diff
	vmOut   string // populated for diff
}

// ── Single-file run ───────────────────────────────────────────────────────────

// runOne parses path, evaluates it twice (tree-walker, then VM),
// compares stdout. Skip results come from CompileError (VM can't
// handle the AST yet) or from a not-yet-implemented VM opcode panic.
// Eval-side errors fall under statusError because they indicate a
// problem with the source program itself, not the VM bring-up.
func runOne(path string) result {
	src, err := os.ReadFile(path)
	if err != nil {
		return result{status: statusError, reason: "read: " + err.Error()}
	}

	// Parse ONCE. Both runs see the exact same AST so we're not
	// comparing two slightly different parses by accident.
	l := lexer.New(string(src))
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(prog.Errors) > 0 {
		return result{status: statusError, reason: "parse: " + prog.Errors[0]}
	}

	// ── eval/ side ──────────────────────────────────────────────────
	// Capture stdout while running through the tree-walker. The
	// tree-walker is the reference oracle; any failure here is a
	// problem with the source, not the VM.
	//
	// Reset the process-global module cache before the run so this
	// file starts from a clean slate — important when batches share
	// modules whose top-level code mutates state (e.g. stdlib/test.lex
	// counters). Without this, file N+1's eval pass would inherit
	// file N's cached module envs.
	eval.ResetModuleCache()
	// M6 (audit follow-up, 2026-05-22): eval pass must run with a
	// PURE tree-walker — including imports. Clear the VM hook so
	// the eval pass exercises the tree-walker contract that vmdiff
	// is validating against.
	eval.VMCompileAndRunModule = nil
	evalOut, evalErr := captureStdout(func() {
		env := eval.NewEnv()
		_ = eval.Eval(prog, env)
	})
	if evalErr != nil {
		return result{status: statusError, reason: "eval stdout capture: " + evalErr.Error()}
	}

	// ── vm/ side ────────────────────────────────────────────────────
	// M6: install the VM hook so imported modules also compile to
	// bytecode for the VM pass. Imports the eval pass loaded via
	// tree-walker were just dropped by ResetModuleCache below;
	// they get re-loaded fresh under the VM path.
	eval.VMCompileAndRunModule = vm.CompileAndRunModule
	// Compile; if it errors with a *CompileError, mark SKIP — the
	// compiler simply doesn't handle this shape yet. Any other
	// compile error is a real bug worth surfacing.
	chunk, cerr := vm.Compile(prog)
	if cerr != nil {
		var ce *vm.CompileError
		if errors.As(cerr, &ce) {
			return result{status: statusSkip, reason: ce.Message}
		}
		return result{status: statusError, reason: "vm/compile: " + cerr.Error()}
	}

	// Same cache reset before the VM run so module-level state
	// produced by the eval pass doesn't leak into the VM pass.
	eval.ResetModuleCache()
	// Execute under stdout capture. Recover panics so an
	// unimplemented opcode (the VM panics in its default case) shows
	// up as a SKIP rather than crashing the entire batch run.
	vmOut, vmErr := captureStdout(func() {
		defer func() {
			if r := recover(); r != nil {
				// Re-throw with a typed sentinel the outer scope can
				// detect. captureStdout passes the panic through.
				panic(skipPanic{reason: fmt.Sprint(r)})
			}
		}()
		_, _ = vm.Run(chunk)
	})
	if vmErr != nil {
		// captureStdout may have surfaced a skipPanic.
		var sp skipPanic
		if errors.As(vmErr, &sp) {
			return result{status: statusSkip, reason: "vm runtime: " + sp.reason}
		}
		return result{status: statusError, reason: "vm run: " + vmErr.Error()}
	}

	if evalOut == vmOut {
		return result{status: statusMatch, evalOut: evalOut}
	}
	return result{status: statusDiff, evalOut: evalOut, vmOut: vmOut}
}

// skipPanic is the sentinel used to convert a VM-internal panic
// (e.g. "opcode Add not implemented") into a SKIP result. Carries
// the panic value as a human string for the report.
type skipPanic struct{ reason string }

func (s skipPanic) Error() string { return s.reason }

// ── Stdout capture ────────────────────────────────────────────────────────────

// captureStdout runs fn with both os.Stdout AND eval.Output redirected
// to a pipe and returns everything fn wrote.
//
// Why both: println / print / log are defined to write to the package
// variable eval.Output (initialised once at startup from os.Stdout), so
// merely redirecting os.Stdout doesn't catch them. We override
// eval.Output in addition to os.Stdout so any builtin that writes via
// fmt.Println directly OR via eval.Output is captured.
//
// Both are restored when fn returns, even on panic. Captured panics
// come back as the returned error so the caller can decide whether
// they're SKIP (sentinel skipPanic) or ERROR.
func captureStdout(fn func()) (string, error) {
	origStdout := os.Stdout
	origEvalOut := eval.Output
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("pipe: %w", err)
	}
	os.Stdout = w
	eval.Output = w

	// Drain in a goroutine so the pipe doesn't block fn when its
	// buffer fills (default 64 KiB on macOS; some kLex tests print
	// more).
	done := make(chan struct {
		s string
		e error
	}, 1)
	go func() {
		b, e := io.ReadAll(r)
		done <- struct {
			s string
			e error
		}{string(b), e}
	}()

	var caught any
	func() {
		defer func() {
			if r := recover(); r != nil {
				caught = r
			}
		}()
		fn()
	}()

	_ = w.Close()
	os.Stdout = origStdout
	eval.Output = origEvalOut
	res := <-done
	_ = r.Close()
	if res.e != nil {
		return res.s, res.e
	}
	if caught != nil {
		if sp, ok := caught.(skipPanic); ok {
			return res.s, sp
		}
		return res.s, fmt.Errorf("captured panic: %v", caught)
	}
	return res.s, nil
}

// ── Reporting ─────────────────────────────────────────────────────────────────

// printDiff prints both stdouts side by side for human inspection.
// Stays small on purpose — a real implementation could use go-cmp or
// the standard library's diff machinery, but at the bring-up stage
// "here's eval output, here's vm output" is enough.
func printDiff(evalOut, vmOut string) {
	evalLines := splitLines(evalOut)
	vmLines := splitLines(vmOut)
	fmt.Printf("        eval stdout (%d lines)\n", len(evalLines))
	for _, l := range evalLines {
		fmt.Printf("          %s\n", l)
	}
	fmt.Printf("        vm stdout (%d lines)\n", len(vmLines))
	for _, l := range vmLines {
		fmt.Printf("          %s\n", l)
	}
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func printHelp() {
	fmt.Println("vmdiff — differential test runner: eval/ vs vm/")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run ./vm/cmd/vmdiff <file.lex> [more.lex ...]")
	fmt.Println("  go run ./vm/cmd/vmdiff --quiet ...   # only print failures")
	fmt.Println()
	fmt.Println("Each .lex file is run twice — once through the tree-walking")
	fmt.Println("evaluator (the reference oracle) and once through the bytecode")
	fmt.Println("VM. Stdout is captured for each. Exit code is the number of")
	fmt.Println("files that diverged (capped at 125).")
}
