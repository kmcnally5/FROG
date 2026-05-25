package eval

import (
	"flag"
	"fmt"
	"klex/ast"
	"os"
	"runtime"
)

func init() {
	// _scriptDir() → string
	//
	// Returns the directory of the .lex file whose code is currently
	// running. Walks the enclosing function/module env chain — inside
	// an imported module it returns THAT module's directory, not the
	// entry script's. Empty string if no script context (REPL).
	//
	// Use to locate sibling resources independent of CWD:
	//   bridge = nativeBridge("python3", [_scriptDir() + "/helper.py"], opts)
	//   font   = loadFont(_scriptDir() + "/fonts/Outfit.ttf", 32)
	//
	// The actual scope-walking implementation lives in evalCall's special
	// case (alongside async). This Fn is a placeholder that returns ""
	// — only reached if someone calls _scriptDir indirectly (e.g. via
	// safe()) and the special case is bypassed. Direct calls always go
	// through evalCall and get the real value.
	Builtins["_scriptDir"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: ""}
	}}
	scriptDirBuiltin = Builtins["_scriptDir"]

	// _osGetenv(key) → string or null
	// Returns the value of the named environment variable, or null if unset.
	Builtins["_osGetenv"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_osGetenv expects 1 argument", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osGetenv: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		val, set := os.LookupEnv(k.Value)
		if !set {
			return NULL
		}
		return &String{Value: val}
	}}

	// _osSetenv(key, val) → (null, err)
	Builtins["_osSetenv"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_osSetenv expects 2 arguments", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osSetenv: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		v, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osSetenv: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if err := os.Setenv(k.Value, v.Value); err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{NULL, NULL}}
	}}

	// _osCwd() → (path_string, err)
	Builtins["_osCwd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osCwd expects no arguments", ast.Pos{})
		}
		dir, err := os.Getwd()
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: dir}, NULL}}
	}}

	// _osHostname() → (hostname_string, err)
	Builtins["_osHostname"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osHostname expects no arguments", ast.Pos{})
		}
		h, err := os.Hostname()
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: h}, NULL}}
	}}

	// _osPid() → integer  — current process ID
	Builtins["_osPid"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osPid expects no arguments", ast.Pos{})
		}
		return &Integer{Value: os.Getpid()}
	}}

	// _osName() → string
	//
	// Return the operating-system identifier — Go's runtime.GOOS verbatim.
	// Common values: "darwin", "linux", "windows", "freebsd", "openbsd".
	// Use for cross-platform branching when path-probing would be brittle:
	//
	//   name = _osName()
	//   if name == "darwin"  { cmd = "open"     }
	//   if name == "linux"   { cmd = "xdg-open" }
	//   if name == "windows" { cmd = "cmd"      }
	//
	// Replaces the historical pattern of probing `/usr/bin/open` etc.,
	// which fails on minimal images where the standard tools live
	// elsewhere on PATH.
	Builtins["_osName"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osName expects no arguments", ast.Pos{})
		}
		return &String{Value: runtime.GOOS}
	}}

	// _osArgs() → array of strings
	//
	// Returns command-line arguments AFTER kLex's flag parser has
	// consumed its own options (--cpuprofile, --help, --version, etc.).
	//
	// Shape (stable across flag-or-no-flag invocations):
	//   args[0]    — the kLex binary path (os.Args[0])
	//   args[1]    — the script path
	//   args[2..N] — script arguments
	//
	// Pre-fix behaviour returned raw os.Args, which meant any script using
	// `args[2]` etc. broke if a kLex flag was present — the flag pair
	// shifted every positional. OFI #13. The prepended binary path keeps
	// `args[0]` meaningful for scripts that re-spawn klex on themselves
	// (e.g. frogLight UI spawning the cataloger via _processSpawnDetached).
	Builtins["_osArgs"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osArgs expects no arguments", ast.Pos{})
		}
		// flag.Parsed() is true once main.go has called flag.Parse() — which
		// it does before evaluating any script. The REPL path also calls
		// flag.Parse() (zero positionals). If for some reason Parse() hasn't
		// run yet (early init, tests), fall back to raw os.Args so the
		// builtin never returns garbage.
		var positional []string
		if flag.Parsed() {
			positional = flag.Args()
		} else {
			// Skip os.Args[0] (binary); the raw flags are still in there,
			// but at least the shape matches the post-Parse form below.
			if len(os.Args) > 1 {
				positional = os.Args[1:]
			}
		}
		// Prepend the binary path so args[0] still means "the klex binary".
		out := make([]Object, 0, 1+len(positional))
		if len(os.Args) > 0 {
			out = append(out, &String{Value: os.Args[0]})
		}
		for _, a := range positional {
			out = append(out, &String{Value: a})
		}
		return &Array{Elements: out}
	}}

	// _osExit(code) — terminates the process with the given exit code.
	// Does not return.
	Builtins["_osExit"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_osExit expects 1 argument", ast.Pos{})
		}
		code, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_osExit: argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		os.Exit(code.Value)
		return NULL // unreachable
	}}
}
