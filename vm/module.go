package vm

// module.go — VM-side entry points for eval hooks.
//
// CompileAndRunModule: M6 (audit follow-up, 2026-05-22): imported module
// execution via the eval.VMCompileAndRunModule hook.
//
// Wired into eval's *ast.ImportStmt arm via the
// eval.VMCompileAndRunModule hook. When the hook is set (main.go /
// vmdiff sets it for the --vm pipeline), eval's import loader tries
// the VM path FIRST, falling back to its own tree-walker Eval only
// when vm.Compile returns an error (e.g. the module uses
// InterpolatedString with embedded expressions, which the VM
// compiler doesn't handle yet).
//
// Why fall back to tree-walker: keeps M6 incremental — stdlib
// modules that already work on the VM run on the VM; modules that
// hit unimplemented compiler arms keep working via the tree-walker.
// No "all-or-nothing" cutover risk.
//
// What this enables in practice
//
//   - stdlib/strings.lex's split/join/etc. called in a hot loop
//     from VM-compiled main: was per-call CallCallable +
//     applyFunction (env map alloc + interpreted AST walk); now
//     OpCall on a *CompiledFunction = ~10ns dispatch.
//   - Same for any imported module that compiles cleanly.

import (
	"klex/ast"
	"klex/eval"
)

// CompileAndRunModule is the eval.VMCompileAndRunModule hook
// implementation. It compiles `prog` (the parsed module AST) to
// bytecode, runs the top-level chunk via RunModule, and returns
// an *eval.Environment with live getters for every top-level
// binding. The scriptDir argument is recorded on the chunk so the
// _scriptDir intercept inside the module's own VM-compiled code
// returns the correct directory.
//
// Returns (nil, nil) on compile error so eval's loader can fall
// back to the tree-walker path. Returns (nil, *eval.Error) on
// runtime error — the module's top-level threw, and the user
// should see it just as they would with the tree-walker.
// CompileAndRunScript is the eval.VMRunScript hook implementation.
// It compiles and runs a complete isolated program — the WASM runScript
// builtin path where each call gets a fresh execution context (no
// persistent environment). No ScriptDir or __args__ propagation is
// needed: in-browser scripts have no filesystem path, and runScript
// programs don't receive command-line arguments.
//
// Returns (result, true) on success or runtime error — result may be
// an *eval.Error if the program itself failed. Output is captured by
// the caller via eval.Output before this is called.
// Returns (nil, false) on compile error so the caller falls through to
// tree-walker Eval, keeping the same incremental fallback as modules.
func CompileAndRunScript(prog *ast.Program) (eval.Object, bool) {
	chunk, err := Compile(prog)
	if err != nil {
		return nil, false
	}
	result, runErr := Run(chunk)
	if runErr != nil {
		return &eval.Error{
			Kind:    eval.RuntimeErr,
			Message: "vm script error: " + runErr.Error(),
		}, true
	}
	return result, true
}

func CompileAndRunModule(prog *ast.Program, scriptDir string) (*eval.Environment, eval.Object) {
	chunk, err := Compile(prog)
	if err != nil {
		// Compile error → caller falls back to tree-walker. We do
		// NOT propagate the error as an eval.Error because tree-
		// walker may handle the construct fine (e.g. interpolated
		// strings with embedded expressions). Returning (nil, nil)
		// is the signal.
		_ = err
		return nil, nil
	}
	// M3 (audit fix, 2026-05-22): propagate ScriptDir to every
	// sub-chunk (closures, methods) so _scriptDir() resolves
	// correctly from anywhere inside the module.
	PropagateScriptDir(chunk, scriptDir)
	env, runErr := RunModule(chunk)
	if runErr != nil {
		// M2 (audit fix, 2026-05-23): a Go-level error from
		// RunModule is a VM bug — the compiler emitted bad
		// bytecode and the dispatch loop bailed via vmError().
		// Previously this silently fell back to the tree-walker,
		// which hid the bug until vmdiff happened to run that
		// module's failing path. Now we hard-fail: wrap the Go
		// error in a kLex *Error and let it propagate. The user
		// sees a clear VM-bug signal and can report / repro
		// instead of running on tree-walker fallback unknowingly.
		return nil, &eval.Error{
			Kind:    eval.RuntimeErr,
			Message: "vm module bug: " + runErr.Error(),
		}
	}
	return env, nil
}
