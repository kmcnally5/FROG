package eval

// eval.go is the evaluator — the third and final stage of the interpreter.
//
// It receives the AST built by the parser and "executes" it by walking the
// tree recursively. For each node type, Eval() either returns a value or a
// signal (ReturnValue, BreakSignal, ContinueSignal, Error).
//
// This architecture is called a TREE-WALKING INTERPRETER. It is the simplest
// possible execution model: no bytecode, no compilation, no virtual machine.
// The tradeoff is that it's slower than compiled approaches, but for a learning
// project the simplicity is worth it — you can trace exactly what happens.
//
// Error propagation:
// Errors bubble up the call stack like exceptions, but without try/catch.
// Every Eval() call checks `if isError(result) { return result }` immediately
// after getting a value. This means an error at any depth instantly unwinds
// all the way to the top-level program loop, which prints it and stops.
//
// Signal propagation (return, break, continue):
// These work the same way — they are special Object types that bubble up
// through Eval() calls until the right handler catches them:
//   - ReturnValue is unwrapped by the function-call handler
//   - BreakSignal / ContinueSignal are caught by the while-loop handler

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"klex/ast"
	"klex/lexer"
	"klex/parser"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// stdinReader is shared across all input() calls so buffered bytes are not lost.
var stdinReader = bufio.NewReader(os.Stdin)

// Module import state.
//
//	moduleCache    — absolute path → evaluated *Environment. Populated
//	                 on successful import; subsequent imports of the
//	                 same path hand back a Module wrapping the cached
//	                 Env (under whatever alias the new importer used).
//	                 This makes module-level mutable state (cost
//	                 ledgers, registries, pools) act as process-global
//	                 — which is how a sane scripting language reads
//	                 "two files importing the same library see the same
//	                 variables." The previous fresh-env-per-import
//	                 behaviour silently broke cross-module coordination.
//
//	importingFiles — set of paths currently mid-import. Used for cycle
//	                 detection; an entry is added before Eval runs the
//	                 module's top-level code and removed on completion.
//
//	moduleCacheMu  — guards BOTH maps. import evaluation recurses
//	                 through Eval, and async() goroutines can do their
//	                 own imports, so concurrent access is possible.
//	                 The lock is released across the Eval call itself
//	                 to avoid reentrant-deadlock when an imported file
//	                 imports another file.
//
// Failures are never cached: if Eval returns an error, the path is left
// out so a subsequent retry will re-attempt the import.
//
// Lifetime: process. There is no mtime check or invalidation — kLex
// programs are short-lived enough that file-during-run edits are not a
// real use case. Restart the process to pick up source changes.
var (
	moduleCacheMu  sync.RWMutex
	moduleCache    = map[string]*Environment{}
	importingFiles = map[string]bool{}
)

// ResetModuleCache clears every cached module env and any in-flight
// import marker. Used by differential test runners (vmdiff) that need
// the eval and vm passes to start from identical module state — the
// alternative is for module-level mutable counters (e.g.
// stdlib/test.lex's _passed) to carry leftover state from the eval
// pass into the vm pass and produce spurious diffs. Safe to call from
// any goroutine; takes the cache write lock.
func ResetModuleCache() {
	moduleCacheMu.Lock()
	moduleCache = map[string]*Environment{}
	importingFiles = map[string]bool{}
	moduleCacheMu.Unlock()
}

// resolveImportPath finds the file backing an `import "path"` statement.
// It tries five locations in order; the first existing file wins. Returns
// the resolved path, the list of paths it tried (for error messages), and
// ok=true on success.
//
//  1. path as-is                  — CWD-relative
//  2. <script-dir>/path           — next to the importing file
//  3. $KLEX_PATH/path             — user override
//  4. <klex-exe-dir>/path         — drop-in install
//  5. <klex-exe-parent>/path      — bin/klex + share style install
//
// Duplicates in the list are dropped (e.g. when CWD happens to equal the
// script directory) so the error message stays clean.
func resolveImportPath(path string, env *Environment) (resolved string, tried []string, ok bool) {
	scriptDir := ""
	if env != nil {
		scriptDir = env.ScriptDir()
	}
	return resolveImportPathFromDir(path, scriptDir)
}

// resolveImportPathFromDir is the env-less core of resolveImportPath.
// H4 (audit fix, 2026-05-22): the LookupOrLoadModule fast path uses
// this so cache-hit imports skip the throwaway NewEnv() allocation.
// scriptDir == "" disables step 2 (script-relative resolution); the
// remaining steps (KLEX_PATH, binary-dir, binary-parent) still fire.
func resolveImportPathFromDir(path, scriptDir string) (resolved string, tried []string, ok bool) {
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		tried = append(tried, p)
	}

	// 1. As given (CWD-relative).
	add(path)

	// 2. Next to the importing script.
	if scriptDir != "" {
		add(filepath.Join(scriptDir, path))
	}

	// 3. $KLEX_PATH.
	if kp := os.Getenv("KLEX_PATH"); kp != "" {
		add(filepath.Join(kp, path))
	}

	// 4 & 5. Next to the kLex binary and one level up.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(filepath.Join(exeDir, path))
		add(filepath.Join(filepath.Dir(exeDir), path))
	}

	for _, candidate := range tried {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, tried, true
		}
	}
	return "", tried, false
}

// EnableAsyncYield controls whether loop statements (while, for-in) yield to Go's
// scheduler periodically. When true, runtime.Gosched() is called to allow
// goroutines to be preempted and distributed across CPU cores for better
// parallelization with async tasks. When false, loops run tightly without yielding,
// which is faster for single-threaded or CPU-bound work.
const EnableAsyncYield = true

// AsyncYieldInterval controls how often to call runtime.Gosched() in loops.
// A value of 100 means yield every 100 iterations.
// Yielding too frequently (e.g., every iteration) kills performance for CPU-bound loops.
// Yielding too rarely (e.g., never) causes unfair scheduling with many concurrent tasks.
// Tuned for 10-core parallelism: balance between fair scheduling and loop efficiency.
const AsyncYieldInterval = 1000

// KLexVersion is the interpreter version, set by main.go at startup.
// Exposed as the __version__ builtin so FROG programs can read it.
var KLexVersion = "unknown"

// Output is the writer used by print, println and input's prompt.
// Defaults to os.Stdout. Override this to redirect output (e.g. the
// WASM playground redirects it to a strings.Builder to capture stdout).
var Output io.Writer = os.Stdout

// DeepFreeze is the exported entry point for other packages (the
// bytecode VM) to recursively freeze a const value the same way the
// tree-walker does. Tree-walker callers continue to use the lowercase
// alias inside this package.
func DeepFreeze(obj Object) {
	deepFreeze(obj, map[Object]bool{})
}

// deepFreeze recursively freezes an object and all mutable objects it contains.
// Visited tracks pointer identity to handle cycles. Call with an empty map on entry.
func deepFreeze(obj Object, visited map[Object]bool) {
	if visited[obj] {
		return
	}
	visited[obj] = true
	switch o := obj.(type) {
	case *Array:
		o.frozen = true
		for _, el := range o.Elements {
			deepFreeze(el, visited)
		}
	case *Hash:
		o.frozen = true
		for _, pair := range o.Pairs {
			deepFreeze(pair.Value, visited)
		}
	case *StructInstance:
		o.frozen = true
		for _, v := range o.Fields {
			deepFreeze(v, visited)
		}
	}
}

// Builtins are the built-in functions available in every kLex program.
// They live outside the environment chain so they are always accessible.
// asyncBuiltin caches the pointer to Builtins["async"]. evalCall compares
// against this pointer to detect the async call site, instead of string-
// comparing the identifier name on every builtin invocation. Assigned in
// the init() that registers async, so it's set before any kLex code runs.
var asyncBuiltin *Builtin

// scriptDirBuiltin caches the pointer to Builtins["_scriptDir"]. Same
// trick as asyncBuiltin — evalCall identity-compares to intercept calls
// and supply env.ScriptDir() since builtin Fns don't otherwise have env
// access. Lets kLex scripts find sibling files (Python bridges, fonts,
// assets) regardless of the caller's CWD.
var scriptDirBuiltin *Builtin

// When you call println("hello"), the evaluator looks up "println" in the
// environment, finds this Builtin object, and calls its Fn.
var Builtins = map[string]*Builtin{
	// __version__ returns the interpreter version string.
	"__version__": {Fn: func(args []Object) Object {
		return &String{Value: KLexVersion}
	}},
	// println — print values, each followed by a newline.
	//
	// Each argument is printed on its own line using its display form; with no
	// arguments it prints a single blank line. Returns null.
	//
	// @sig     println(values...: any) -> null
	// @param   values  zero or more values to print, one per line
	// @returns null
	// @errors  none
	// @example no-run println("hello, world")
	// @since   0.1.0
	// @see     print, str
	"println": {Fn: func(args []Object) Object {
		for _, arg := range args {
			fmt.Fprintln(Output, arg.Inspect())
		}
		return NULL
	}},
	// len — the number of elements in an array, hash, string, or bytes value.
	//
	// For strings, len counts Unicode code points (runes), not bytes — len("café")
	// is 4. For bytes it's the raw byte count. For arrays and hashes it's the
	// element/entry count.
	//
	// @sig     len(value: any) -> int
	// @param   value  an array, hash, string, or bytes value
	// @returns the count: runes for strings, bytes for bytes, elements otherwise
	// @errors  TypeError if value has no length (e.g. an integer or boolean)
	// @example len([1, 2, 3])   → 3
	// @example len("café")      → 4
	// @since   0.1.0
	// @see     keys, slice
	"len": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("len expects 1 argument", ast.Pos{})
		}
		switch arg := args[0].(type) {
		case *Array:
			return intObj(len(arg.Elements))
		case *String:
			// Count Unicode code points, not bytes, so len("café") == 4.
			// RuneLen() caches the count on the *String so repeated len(s) calls
			// (e.g. json.parse's per-char `i < len(s.s)` bound check) are O(1).
			return intObj(arg.RuneLen())
		case *Bytes:
			// len() on bytes is the raw byte count — same as the wire size,
			// unlike strings where len() is rune count.
			return intObj(len(arg.Value))
		case *Hash:
			return intObj(arg.LenSafe()) // OFI #3 — locked read
		case *AtomicIntArray:
			return intObj(len(arg.Data))
		case *AtomicFloatArray:
			return intObj(len(arg.Bits))
		case *ConcurrentHash:
			return intObj(int(atomic.LoadInt64(&arg.Cnt)))
		default:
			return typeError(fmt.Sprintf("len not defined for %s", args[0].Type()), ast.Pos{})
		}
	}},
	// keys — an array of all keys in a hash.
	//
	// Key order is NOT guaranteed (hash iteration is unordered) — sort the result
	// if you need a stable order. Accepts a concurrent hash too.
	//
	// @sig     keys(h: hash) -> array
	// @param   h  the hash to read
	// @returns an array of the hash's keys, in unspecified order
	// @errors  TypeError if h is not a hash
	// @example keys({"a": 1})   → [a]
	// @since   0.1.0
	// @see     values, hasKey
	"keys": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keys expects 1 argument", ast.Pos{})
		}
		switch h := args[0].(type) {
		case *Hash:
			pairs := h.Snapshot() // OFI #3 — safe under concurrent writers
			out := make([]Object, 0, len(pairs))
			for _, pair := range pairs {
				out = append(out, pair.Key)
			}
			return &Array{Elements: out}
		case *ConcurrentHash:
			// Snapshot via Range; entries added/removed during iteration may be
			// included/excluded per sync.Map semantics.
			out := make([]Object, 0, atomic.LoadInt64(&h.Cnt))
			h.M.Range(func(_, val any) bool {
				if pair, ok := val.(HashPair); ok {
					out = append(out, pair.Key)
				}
				return true
			})
			return &Array{Elements: out}
		default:
			return typeError(fmt.Sprintf("keys expects hash, got %s", args[0].Type()), ast.Pos{})
		}
	}},
	// values — an array of all values in a hash.
	//
	// Mirrors keys(): same unspecified order, and also accepts a concurrent hash.
	//
	// @sig     values(h: hash) -> array
	// @param   h  the hash to read
	// @returns an array of the hash's values, in unspecified order
	// @errors  TypeError if h is not a hash
	// @example values({"a": 1})   → [1]
	// @since   0.1.0
	// @see     keys, hasKey
	"values": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("values expects 1 argument", ast.Pos{})
		}
		switch h := args[0].(type) {
		case *Hash:
			pairs := h.Snapshot() // OFI #3 — safe under concurrent writers
			out := make([]Object, 0, len(pairs))
			for _, pair := range pairs {
				out = append(out, pair.Value)
			}
			return &Array{Elements: out}
		case *ConcurrentHash:
			out := make([]Object, 0, atomic.LoadInt64(&h.Cnt))
			h.M.Range(func(_, val any) bool {
				if pair, ok := val.(HashPair); ok {
					out = append(out, pair.Value)
				}
				return true
			})
			return &Array{Elements: out}
		default:
			return typeError(fmt.Sprintf("values expects hash, got %s", args[0].Type()), ast.Pos{})
		}
	}},
	// hasKey — whether a hash contains the given key.
	//
	// Avoids the h["name"] == null pattern, which can't tell "absent" from
	// "present but null".
	//
	// @sig     hasKey(h: hash, key: any) -> bool
	// @param   h    the hash to check
	// @param   key  the key to look for
	// @returns true if the key is present
	// @errors  TypeError if h is not a hash or key is unhashable
	// @example hasKey({"a": 1}, "a")   → true
	// @example hasKey({"a": 1}, "b")   → false
	// @since   0.1.0
	// @see     keys, delete
	"hasKey": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("hasKey expects 2 arguments", ast.Pos{})
		}
		hash, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("hasKey: first argument must be hash, got %s", args[0].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		_, exists := hash.Get(hk) // OFI #3 — locked read
		return &Boolean{Value: exists}
	}},
	// delete — remove a key from a hash IN PLACE (mutates the hash).
	//
	// Unlike most kLex collection ops, delete mutates rather than copying: the
	// hash is changed and null is returned. Deleting an absent key is a no-op.
	//
	// @sig     delete(h: hash, key: any) -> null
	// @param   h    the hash to modify
	// @param   key  the key to remove
	// @returns null — the hash is mutated in place
	// @errors  TypeError if h is not a hash or key is unhashable; RuntimeError if the hash is frozen
	// @example delete({"a": 1}, "a")   → null
	// @since   0.1.0
	// @see     hasKey, keys
	"delete": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("delete expects 2 arguments", ast.Pos{})
		}
		switch h := args[0].(type) {
		case *Hash:
			if h.frozen {
				return runtimeError("cannot mutate frozen hash", ast.Pos{})
			}
			hk, err := toHashKey(args[1], ast.Pos{})
			if err != nil {
				return err
			}
			h.Del(hk) // OFI #3 — locked delete
			return NULL
		case *ConcurrentHash:
			hk, err := toHashKey(args[1], ast.Pos{})
			if err != nil {
				return err
			}
			if _, loaded := h.M.LoadAndDelete(hk); loaded {
				atomic.AddInt64(&h.Cnt, -1)
			}
			return NULL
		default:
			return typeError(fmt.Sprintf("delete expects hash as first argument, got %s", args[0].Type()), ast.Pos{})
		}
	}},
	// print — print values with no trailing newline.
	//
	// Each argument is printed using its display form, with nothing added between
	// or after — useful for building a line across several calls. Returns null.
	//
	// @sig     print(values...: any) -> null
	// @param   values  zero or more values to print
	// @returns null
	// @errors  none
	// @example no-run print("no newline")
	// @since   0.1.0
	// @see     println, str
	"print": {Fn: func(args []Object) Object {
		for _, arg := range args {
			fmt.Fprint(Output, arg.Inspect())
		}
		return NULL
	}},
	// type — the runtime type name of any value, as a string.
	//
	// Returns the strict type tag — "INTEGER", "STRING", "ARRAY", "HASH",
	// "BOOLEAN", "NULL", "FUNCTION", and so on. Handy for debugging and for
	// branching on a value's shape.
	//
	// @sig     type(value: any) -> string
	// @param   value  any value
	// @returns the uppercase type-name string
	// @errors  none
	// @example type(42)      → INTEGER
	// @example type("hi")    → STRING
	// @example type([1, 2])  → ARRAY
	// @since   0.1.0
	// @see     len
	"type": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("type expects 1 argument", ast.Pos{})
		}
		return &String{Value: string(args[0].Type())}
	}},
	// str — any value's string representation.
	//
	// The standard way to turn a number, bool, or null into text for building
	// output. Uses each value's display form (e.g. a string array shows as
	// [a, b], not ["a", "b"]).
	//
	// @sig     str(value: any) -> string
	// @param   value  any value
	// @returns the value's string form
	// @errors  none
	// @example str(42)     → 42
	// @example str(true)   → true
	// @since   0.1.0
	// @see     int, float, type
	"str": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("str expects 1 argument", ast.Pos{})
		}
		return &String{Value: args[0].Inspect()}
	}},
	// int — convert a float, string, or int to an integer.
	//
	// A float truncates toward zero. For untrusted strings (where a bad value
	// should be handled, not raised), prefer parseInt, which returns (value, err).
	//
	// @sig     int(x: any) -> int
	// @param   x  an integer, a float (truncated), or an integer string
	// @returns x as an integer
	// @errors  TypeError for non-numeric types; RuntimeError if a string isn't a valid integer
	// @example int("42")   → 42
	// @example int(3.9)    → 3
	// @since   0.1.0
	// @see     float, parseInt, str
	"int": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("int expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return v
		case *Float:
			return &Integer{Value: int(v.Value)}
		case *String:
			n, err := strconv.Atoi(v.Value)
			if err != nil {
				// Check whether it looks like a float to give a more helpful message.
				if _, ferr := strconv.ParseFloat(v.Value, 64); ferr == nil {
					return runtimeError(fmt.Sprintf("int: %q looks like a float — use int(float(%q))", v.Value, v.Value), ast.Pos{})
				}
				return runtimeError(fmt.Sprintf("int: cannot convert %q to integer", v.Value), ast.Pos{})
			}
			return &Integer{Value: n}
		default:
			return typeError(fmt.Sprintf("int: cannot convert %s to integer", args[0].Type()), ast.Pos{})
		}
	}},
	// split — break a string into pieces on a separator.
	//
	// Returns the substrings between each occurrence of sep, in order. Pair with
	// join to round-trip. Splitting on "" yields one entry per Unicode code
	// point (rune), not per byte.
	//
	// @sig     split(str: string, sep: string) -> array
	// @param   str  the string to split
	// @param   sep  the separator; "" splits into individual runes
	// @returns an array of string pieces; never null
	// @errors  TypeError if str or sep is not a string
	// @example split("a,b,c", ",")  → [a, b, c]
	// @example split("café", "")    → [c, a, f, é]
	// @since   0.1.0
	// @see     join, replace, substr
	"split": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("split expects 2 arguments", ast.Pos{})
		}
		str, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("split: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		sep, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("split: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		parts := strings.Split(str.Value, sep.Value)
		elements := make([]Object, len(parts))
		for i, p := range parts {
			elements[i] = &String{Value: p}
		}
		return &Array{Elements: elements}
	}},
	// join — concatenate an array of strings into one, separated by sep.
	//
	// The inverse of split. Every element must be a string.
	//
	// @sig     join(arr: array, sep: string) -> string
	// @param   arr  an array of strings
	// @param   sep  the separator placed between elements
	// @returns the elements of arr joined by sep
	// @errors  TypeError if arr isn't an array, an element isn't a string, or sep isn't a string
	// @example join(["a", "b", "c"], ",")   → a,b,c
	// @since   0.1.0
	// @see     split, concat
	"join": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("join expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("join: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		sep, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("join: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		parts := make([]string, len(arr.Elements))
		for i, el := range arr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("join: array element %d must be string, got %s", i, el.Type()), ast.Pos{})
			}
			parts[i] = s.Value
		}
		return &String{Value: strings.Join(parts, sep.Value)}
	}},
	// pop — a NEW array with the last element removed (does not mutate).
	//
	// Like push, pop copies rather than mutating. Popping an empty array returns
	// an empty array.
	//
	// @sig     pop(arr: array) -> array
	// @param   arr  the source array (left unchanged)
	// @returns a new array without arr's last element
	// @errors  TypeError if arr is not an array
	// @example pop([1, 2, 3])   → [1, 2]
	// @since   0.1.0
	// @see     push, slice
	"pop": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("pop expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("pop: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		if len(arr.Elements) == 0 {
			return &Array{Elements: []Object{}}
		}
		newElements := make([]Object, len(arr.Elements)-1)
		copy(newElements, arr.Elements)
		return &Array{Elements: newElements}
	}},
	// push — return a NEW array with one element appended (does not mutate).
	//
	// Arrays are mutable reference types, but push is deliberately immutable: it
	// copies, so the original array is untouched and concurrent readers stay
	// safe. To build a large array, prefer makeArray + index assignment — push in
	// a loop is O(n²).
	//
	// @sig     push(arr: array, value: any) -> array
	// @param   arr    the source array (left unchanged)
	// @param   value  the element to append
	// @returns a new array: arr's elements followed by value
	// @errors  TypeError if arr is not an array
	// @example push([1, 2], 3)   → [1, 2, 3]
	// @example push([], "a")     → [a]
	// @since   0.1.0
	// @see     pop, concat, makeArray
	"push": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("push expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("push: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		newElements := make([]Object, len(arr.Elements)+1)
		copy(newElements, arr.Elements)
		newElements[len(arr.Elements)] = args[1]
		return &Array{Elements: newElements}
	}},
	// makeArray — allocate an array of n elements, each set to default.
	//
	// One allocation — use this instead of push() in a loop (push is O(n) per
	// call, O(n²) total; makeArray is O(n) once). Fill with arr[i] = val after.
	// The default is null when omitted.
	//
	// @sig     makeArray(n: int, [default: any]) -> array
	// @param   n        the number of elements (non-negative)
	// @param   default  the value each element starts as; defaults to null
	// @returns a new array of n copies of default
	// @errors  TypeError if n is not an integer
	// @example makeArray(3, 0)   → [0, 0, 0]
	// @since   0.1.0
	// @see     range, push, concatAll
	"makeArray": {Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("makeArray expects 1 or 2 arguments: makeArray(n) or makeArray(n, defaultVal)", ast.Pos{})
		}
		nObj, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("makeArray: first argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		n := nObj.Value
		if n < 0 {
			return runtimeError("makeArray: size must be non-negative", ast.Pos{})
		}
		var fill Object = NULL
		if len(args) == 2 {
			fill = args[1]
		}
		elements := make([]Object, n)
		for i := range elements {
			elements[i] = fill
		}
		return &Array{Elements: elements}
	}},
	// concat — merge two arrays into one new array.
	//
	// Single-allocation merge of arr1 followed by arr2; faster than looping push
	// to combine two existing arrays. Neither input is modified.
	//
	// ANTIPATTERN: `acc = concat(acc, batch)` in a loop is O(n²) — each call
	// copies the growing accumulator. To merge many arrays, collect them into one
	// outer array and call concatAll(), which is O(total) in a single pass.
	//
	// @sig     concat(arr1: array, arr2: array) -> array
	// @param   arr1  the first array
	// @param   arr2  the array appended after arr1
	// @returns a new array: all of arr1 followed by all of arr2
	// @errors  TypeError if either argument is not an array
	// @example concat([1, 2], [3, 4])   → [1, 2, 3, 4]
	// @since   0.1.0
	// @see     concatAll, push
	"concat": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError(fmt.Sprintf("concat expects 2 arguments, got %d", len(args)), ast.Pos{})
		}
		arr1, ok1 := args[0].(*Array)
		arr2, ok2 := args[1].(*Array)
		if !ok1 || !ok2 {
			return typeError(fmt.Sprintf("concat: both arguments must be array, got %s and %s", args[0].Type(), args[1].Type()), ast.Pos{})
		}
		newElements := make([]Object, len(arr1.Elements)+len(arr2.Elements))
		copy(newElements, arr1.Elements)
		copy(newElements[len(arr1.Elements):], arr2.Elements)
		return &Array{Elements: newElements}
	}},
	// concatAll — flatten an array of arrays into one, in a single allocation.
	//
	// Replaces the O(n²) `acc = concat(acc, batch)` loop antipattern: collect the
	// pieces into one outer array and call concatAll, which sums the total length
	// first and copies each sub-array once — O(total). Empty input → empty array.
	//
	// @sig     concatAll(arrs: array) -> array
	// @param   arrs  an array whose elements are all arrays
	// @returns one array: every sub-array concatenated in order
	// @errors  TypeError if arrs isn't an array or any element isn't an array (the error names the offending index)
	// @example concatAll([[1, 2], [3, 4]])   → [1, 2, 3, 4]
	// @since   0.1.0
	// @see     concat, makeArray
	"concatAll": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError(fmt.Sprintf("concatAll expects 1 argument (array of arrays), got %d", len(args)), ast.Pos{})
		}
		outer, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("concatAll: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		total := 0
		for i, el := range outer.Elements {
			a, ok := el.(*Array)
			if !ok {
				return typeError(fmt.Sprintf("concatAll: element %d must be array, got %s", i, el.Type()), ast.Pos{})
			}
			total += len(a.Elements)
		}
		out := make([]Object, total)
		pos := 0
		for _, el := range outer.Elements {
			a := el.(*Array) // type-checked above
			copy(out[pos:], a.Elements)
			pos += len(a.Elements)
		}
		return &Array{Elements: out}
	}},
	// upper — a copy of a string with all letters uppercased.
	//
	// @sig     upper(s: string) -> string
	// @param   s  the string to convert
	// @returns s with every letter in uppercase
	// @errors  TypeError if s is not a string
	// @example upper("hi")   → HI
	// @since   0.1.0
	// @see     lower, trim
	"upper": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("upper expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("upper: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.ToUpper(s.Value)}
	}},
	// lower — a copy of a string with all letters lowercased.
	//
	// @sig     lower(s: string) -> string
	// @param   s  the string to convert
	// @returns s with every letter in lowercase
	// @errors  TypeError if s is not a string
	// @example lower("HI")   → hi
	// @since   0.1.0
	// @see     upper, trim
	"lower": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("lower expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("lower: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.ToLower(s.Value)}
	}},
	// float — convert an integer, float, or numeric string to a float.
	//
	// For untrusted input (where a bad string should be handled rather than raise),
	// prefer parseFloat, which returns a (value, err) tuple.
	//
	// @sig     float(x: any) -> float
	// @param   x  an integer, float, or numeric string
	// @returns x as a float
	// @errors  TypeError for non-numeric types; RuntimeError if a string isn't a valid float
	// @example float(3)       → 3
	// @example float("2.5")   → 2.5
	// @since   0.1.0
	// @see     int, parseFloat, str
	"float": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("float expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return &Float{Value: float64(v.Value)}
		case *Float:
			return v
		case *String:
			f, err := strconv.ParseFloat(v.Value, 64)
			if err != nil {
				return runtimeError(fmt.Sprintf("float: cannot convert %q to float", v.Value), ast.Pos{})
			}
			return &Float{Value: f}
		default:
			return typeError(fmt.Sprintf("float: cannot convert %s to float", args[0].Type()), ast.Pos{})
		}
	}},
	// range — an array of integers over a half-open interval.
	//
	// Three forms: range(stop) counts 0..stop-1; range(start, stop) counts
	// start..stop-1; range(start, stop, step) steps by step (a negative step
	// counts down). The stop value is always excluded. An empty range yields [].
	//
	// @sig     range(start: int, [stop: int], [step: int]) -> array
	// @param   start  with one argument this is the stop and start defaults to 0; otherwise the first value (inclusive)
	// @param   stop   the exclusive upper (or lower) bound
	// @param   step   the increment; defaults to 1, may be negative, must be non-zero
	// @returns an array of integers from start toward stop, excluding stop
	// @errors  TypeError if any argument is not an integer; RuntimeError if step is zero
	// @example range(3)      → [0, 1, 2]
	// @example range(1, 4)   → [1, 2, 3]
	// @since   0.1.0
	// @see     makeArray
	"range": {Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 3 {
			return runtimeError("range expects 1, 2, or 3 arguments", ast.Pos{})
		}
		toInt := func(o Object, name string) (int, Object) {
			i, ok := o.(*Integer)
			if !ok {
				return 0, typeError(fmt.Sprintf("range: %s must be integer, got %s", name, o.Type()), ast.Pos{})
			}
			return i.Value, nil
		}
		var start, stop, step int
		switch len(args) {
		case 1:
			var err Object
			stop, err = toInt(args[0], "stop")
			if err != nil {
				return err
			}
			start, step = 0, 1
		case 2:
			var err Object
			start, err = toInt(args[0], "start")
			if err != nil {
				return err
			}
			stop, err = toInt(args[1], "stop")
			if err != nil {
				return err
			}
			step = 1
		case 3:
			var err Object
			start, err = toInt(args[0], "start")
			if err != nil {
				return err
			}
			stop, err = toInt(args[1], "stop")
			if err != nil {
				return err
			}
			step, err = toInt(args[2], "step")
			if err != nil {
				return err
			}
			if step == 0 {
				return runtimeError("range: step cannot be zero", ast.Pos{})
			}
		}
		// Pre-calculate size to avoid repeated allocations
		var count int
		if step > 0 {
			if start < stop {
				count = (stop - start + step - 1) / step
			}
		} else {
			if start > stop {
				count = (start - stop - step - 1) / (-step)
			}
		}
		elements := make([]Object, 0, count)
		for i := start; (step > 0 && i < stop) || (step < 0 && i > stop); i += step {
			elements = append(elements, intObj(i))
		}
		return &Array{Elements: elements}
	}},
	// trim — a copy of a string with leading and trailing whitespace removed.
	//
	// @sig     trim(s: string) -> string
	// @param   s  the string to trim
	// @returns s without leading or trailing whitespace
	// @errors  TypeError if s is not a string
	// @example trim("  hi  ")   → hi
	// @since   0.1.0
	// @see     upper, lower, replace
	"trim": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("trim expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("trim: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.TrimSpace(s.Value)}
	}},
	// replace — a copy of str with EVERY occurrence of old replaced by new.
	//
	// Despite the bare name, this is replace-all (it wraps Go's ReplaceAll);
	// replaceAll is an identical alias for people who expect that name.
	//
	// @sig     replace(str: string, old: string, new: string) -> string
	// @param   str  the source string
	// @param   old  the substring to find
	// @param   new  the replacement
	// @returns str with all occurrences of old replaced by new
	// @errors  TypeError if any argument is not a string
	// @example replace("hello world", "world", "kLex")   → hello kLex
	// @since   0.1.0
	// @see     replaceAll, trim, split
	"replace": {Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("replace expects 3 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		old, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		new, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: third argument must be string, got %s", args[2].Type()), ast.Pos{})
		}
		return &String{Value: strings.ReplaceAll(s.Value, old.Value, new.Value)}
	}},
	// replaceAll — identical alias of replace (replaces every occurrence).
	//
	// Lives under both names so people from JS/Python — where replace() is
	// single and replaceAll() is all-occurrences — find what they expect. Both
	// replace every occurrence.
	//
	// @sig     replaceAll(str: string, old: string, new: string) -> string
	// @param   str  the source string
	// @param   old  the substring to find
	// @param   new  the replacement
	// @returns str with all occurrences of old replaced by new
	// @errors  TypeError if any argument is not a string
	// @example replaceAll("a-b-c", "-", "+")   → a+b+c
	// @since   0.1.0
	// @see     replace
	"replaceAll": {Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("replaceAll expects 3 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replaceAll: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		old, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replaceAll: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		new, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replaceAll: third argument must be string, got %s", args[2].Type()), ast.Pos{})
		}
		return &String{Value: strings.ReplaceAll(s.Value, old.Value, new.Value)}
	}},
	// indexOf — the rune index of the first occurrence of substr, or -1.
	//
	// Indices count Unicode code points (runes), consistent with string indexing.
	// An empty substr returns 0.
	//
	// @sig     indexOf(str: string, substr: string) -> int
	// @param   str     the string to search
	// @param   substr  the substring to find
	// @returns the 0-based rune index of the first match, or -1 if absent
	// @errors  TypeError if either argument is not a string
	// @example indexOf("hello", "ll")   → 2
	// @example indexOf("hello", "x")    → -1
	// @since   0.1.0
	// @see     startsWith, endsWith, substr
	"indexOf": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("indexOf expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("indexOf: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		sub, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("indexOf: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if sub.Value == "" {
			return &Integer{Value: 0}
		}
		// Find byte index, then convert to rune index
		byteIdx := strings.Index(s.Value, sub.Value)
		if byteIdx == -1 {
			return &Integer{Value: -1}
		}
		// Convert byte index to rune index. RuneCountInString avoids the
		// rune-slice allocation that the []rune conversion would do.
		runeIdx := utf8.RuneCountInString(s.Value[:byteIdx])
		return &Integer{Value: runeIdx}
	}},
	// startsWith — whether str begins with the given prefix.
	//
	// @sig     startsWith(str: string, prefix: string) -> bool
	// @param   str     the string to test
	// @param   prefix  the prefix to look for
	// @returns true if str begins with prefix
	// @errors  TypeError if either argument is not a string
	// @example startsWith("hello", "he")   → true
	// @since   0.1.0
	// @see     endsWith, indexOf
	"startsWith": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("startsWith expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("startsWith: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		prefix, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("startsWith: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		return &Boolean{Value: strings.HasPrefix(s.Value, prefix.Value)}
	}},
	// endsWith — whether str ends with the given suffix.
	//
	// @sig     endsWith(str: string, suffix: string) -> bool
	// @param   str     the string to test
	// @param   suffix  the suffix to look for
	// @returns true if str ends with suffix
	// @errors  TypeError if either argument is not a string
	// @example endsWith("hello", "lo")   → true
	// @since   0.1.0
	// @see     startsWith, indexOf
	"endsWith": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("endsWith expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("endsWith: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		suffix, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("endsWith: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		return &Boolean{Value: strings.HasSuffix(s.Value, suffix.Value)}
	}},
	// env — the value of an environment variable, or null if unset.
	//
	// @sig     env(name: string) -> string
	// @param   name  the variable name
	// @returns the variable's value, or null if it isn't set
	// @errors  TypeError if name is not a string
	// @example no-run env("HOME")
	// @since   0.1.0
	// @see     exec
	"env": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("env expects 1 argument", ast.Pos{})
		}
		name, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("env: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		val, set := os.LookupEnv(name.Value)
		if !set {
			return NULL
		}
		return &String{Value: val}
	}},
	// readFile — read a whole file into a string.
	//
	// Raises on failure (missing file, permission denied, …) — wrap with
	// safe(readFile, path) to handle errors without crashing.
	//
	// @sig     readFile(path: string) -> string
	// @param   path  the file path
	// @returns the file's contents as a string
	// @errors  TypeError if path isn't a string; RuntimeError if the file can't be read
	// @example no-run let text = readFile("notes.txt")
	// @since   0.1.0
	// @see     writeFile, appendFile, safe
	"readFile": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("readFile expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("readFile: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		data, err := os.ReadFile(path.Value)
		if err != nil {
			return runtimeError(fmt.Sprintf("readFile: %s", err.Error()), ast.Pos{})
		}
		return &String{Value: string(data)}
	}},
	// writeFile — write a string to a file, creating or truncating it.
	//
	// @sig     writeFile(path: string, content: string) -> null
	// @param   path     the file path (created if absent, truncated if present)
	// @param   content  the text to write
	// @returns null on success
	// @errors  TypeError if either argument isn't a string; RuntimeError if the write fails
	// @example no-run writeFile("out.txt", "hello")
	// @since   0.1.0
	// @see     readFile, appendFile
	"writeFile": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("writeFile expects 2 arguments", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("writeFile: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("writeFile: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		err := os.WriteFile(path.Value, []byte(content.Value), 0644)
		if err != nil {
			return runtimeError(fmt.Sprintf("writeFile: %s", err.Error()), ast.Pos{})
		}
		return NULL
	}},
	// appendFile — append a string to a file, creating it if absent.
	//
	// @sig     appendFile(path: string, content: string) -> null
	// @param   path     the file path
	// @param   content  the text to append
	// @returns null on success
	// @errors  TypeError if either argument isn't a string; RuntimeError if the write fails
	// @example no-run appendFile("log.txt", "another line\n")
	// @since   0.1.0
	// @see     writeFile, readFile
	"appendFile": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("appendFile expects 2 arguments", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("appendFile: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("appendFile: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return runtimeError(fmt.Sprintf("appendFile: %s", err.Error()), ast.Pos{})
		}
		defer f.Close()
		if _, err = f.WriteString(content.Value); err != nil {
			return runtimeError(fmt.Sprintf("appendFile: %s", err.Error()), ast.Pos{})
		}
		return NULL
	}},
	// exec — run an external command and return its stdout.
	//
	// The first argument is the command name or path; the second an array of
	// string arguments. Raises on a non-zero exit or OS error — wrap with
	// safe(exec, cmd, args) to handle failures.
	//
	// @sig     exec(cmd: string, args: array) -> string
	// @param   cmd   the command name or path
	// @param   args  an array of string arguments
	// @returns the command's stdout as a string
	// @errors  TypeError if cmd isn't a string or args isn't a string array; RuntimeError on non-zero exit or OS error
	// @example no-run exec("echo", ["hello"])
	// @since   0.1.0
	// @see     env, safe
	"exec": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("exec expects 2 arguments", ast.Pos{})
		}
		cmdName, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("exec: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		arr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("exec: second argument must be array, got %s", args[1].Type()), ast.Pos{})
		}
		cmdArgs := make([]string, len(arr.Elements))
		for i, el := range arr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("exec: args[%d] must be string, got %s", i, el.Type()), ast.Pos{})
			}
			cmdArgs[i] = s.Value
		}
		out, err := exec.Command(cmdName.Value, cmdArgs...).Output()
		if err != nil {
			return runtimeError(fmt.Sprintf("exec: %s", err.Error()), ast.Pos{})
		}
		return &String{Value: string(out)}
	}},
	// input — print an optional prompt and read one line from stdin.
	//
	// The trailing newline is stripped. Do not call from async — stdin is a single
	// shared reader.
	//
	// @sig     input([prompt: string]) -> string
	// @param   prompt  optional text printed before reading
	// @returns the line entered, without its trailing newline
	// @errors  TypeError if prompt is given and isn't a string
	// @example no-run let name = input("Name? ")
	// @since   0.1.0
	// @see     println, readFile
	"input": {Fn: func(args []Object) Object {
		if len(args) > 1 {
			return runtimeError("input expects 0 or 1 arguments", ast.Pos{})
		}
		if len(args) == 1 {
			prompt, ok := args[0].(*String)
			if !ok {
				return typeError(fmt.Sprintf("input: argument must be string, got %s", args[0].Type()), ast.Pos{})
			}
			fmt.Fprint(Output, prompt.Value)
		}
		line, err := stdinReader.ReadString('\n')
		if err != nil {
			// EOF with partial content is fine — return what we have.
			if line == "" {
				return &String{Value: ""}
			}
		}
		line = strings.TrimRight(line, "\r\n")
		return &String{Value: line}
	}},
	// channel — create a channel for passing values between async tasks.
	//
	// An unbuffered channel (no argument) blocks each send until a receiver is
	// ready; a buffered channel (capacity n) blocks send only when full. Pair with
	// async/send/recv to coordinate concurrent work.
	//
	// @sig     channel([capacity: int]) -> channel
	// @param   capacity  buffer size; 0 (the default) means unbuffered
	// @returns a new channel
	// @errors  TypeError if capacity isn't an integer; RuntimeError if it's negative
	// @example no-run let ch = channel(8)
	// @since   0.1.0
	// @see     send, recv, close, async
	"channel": {Fn: func(args []Object) Object {
		capacity := 0
		if len(args) == 1 {
			n, ok := args[0].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("channel: capacity must be an integer, got %s", args[0].Type()), ast.Pos{})
			}
			if n.Value < 0 {
				return runtimeError("channel: capacity must be non-negative", ast.Pos{})
			}
			capacity = n.Value
		} else if len(args) > 1 {
			return runtimeError("channel expects 0 or 1 arguments", ast.Pos{})
		}
		return &Channel{ch: make(chan Object, capacity), done: make(chan struct{})}
	}},

	// send — transmit a value on a channel.
	//
	// Blocks until a receiver is ready (unbuffered) or the buffer has room
	// (buffered). The value is shared with the receiving task. Returns false if
	// the channel was cancelled (a for-in consumer broke out) — stop sending then.
	//
	// @sig     send(ch: channel, value: any) -> any
	// @param   ch     the channel to send on
	// @param   value  the value to transmit
	// @returns null on success; false if the channel was cancelled
	// @errors  TypeError if ch isn't a channel; RuntimeError if the channel is already closed
	// @example no-run send(ch, 42)
	// @since   0.1.0
	// @see     recv, channel, cancel, close
	"send": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("send expects 2 arguments", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("send: first argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		// M5 lazy mutex (audit follow-up 2026-05-22): the value being
		// sent will be read by another goroutine on the receiver
		// side. Mark every reachable mutable container (Hash, and
		// hashes nested inside Arrays/Tuples/Structs) so subsequent
		// reads/writes on either side serialise via Hash.mu. The
		// atomic.Store inside Hash.MarkShared establishes the
		// happens-before edge with the channel send below.
		MarkSharedRecursive(args[1])
		var result Object = NULL
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = runtimeError("send: channel is closed", ast.Pos{})
				}
			}()
			select {
			case ch.ch <- args[1]:
				result = NULL
			case <-ch.done:
				result = FALSE
			}
		}()
		return result
	}},

	// cancel — tell a channel's producer to stop; future sends return false.
	//
	// Signals that the consumer is done. Any blocked or future send() returns
	// false instead of blocking. Idempotent. Breaking out of a for-in over a
	// channel cancels it automatically — this is the explicit form.
	//
	// @sig     cancel(ch: channel) -> null
	// @param   ch  the channel to cancel
	// @returns null
	// @errors  TypeError if ch is not a channel
	// @example no-run cancel(ch)
	// @since   0.1.0
	// @see     close, send, recv
	"cancel": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("cancel expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("cancel: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		func() {
			defer func() { recover() }()
			close(ch.done)
		}()
		return NULL
	}},

	// isError — whether a value is an Error.
	//
	// True for a runtime/type error or a value made with error(). Handy in stream
	// pipeline stages to detect errors surfaced by safe().
	//
	// @sig     isError(value: any) -> bool
	// @param   value  any value
	// @returns true if value is an Error
	// @errors  none
	// @example isError(42)                  → false
	// @example isError(error("X", "oops"))  → true
	// @since   0.1.0
	// @see     error, safe
	"isError": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("isError expects 1 argument", ast.Pos{})
		}
		_, ok := args[0].(*Error)
		return &Boolean{Value: ok}
	}},

	// assert — raise a RuntimeError unless condition is true.
	//
	// On success returns null; on failure raises (catchable with safe(), like any
	// error). The condition must be a bool — a non-bool is a TypeError, not falsy.
	//
	// @sig     assert(condition: bool, [message: string]) -> null
	// @param   condition  must be exactly true to pass
	// @param   message    optional failure message; defaults to "assert: condition is false"
	// @returns null on success
	// @errors  TypeError if condition isn't a bool; RuntimeError (with message) if it's false
	// @example assert(true)   → null
	// @since   0.1.0
	// @see     safe, error
	"assert": {Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("assert expects 1 or 2 arguments", ast.Pos{})
		}
		cond, ok := args[0].(*Boolean)
		if !ok {
			return typeError(fmt.Sprintf("assert: condition must be bool, got %s", args[0].Type()), ast.Pos{})
		}
		if cond.Value {
			return NULL
		}
		msg := "assert: condition is false"
		if len(args) == 2 {
			s, ok := args[1].(*String)
			if !ok {
				return typeError(fmt.Sprintf("assert: message must be string, got %s", args[1].Type()), ast.Pos{})
			}
			msg = s.Value
		}
		return runtimeError(msg, ast.Pos{})
	}},

	// recv — receive the next value from a channel (blocking).
	//
	// Blocks until a value arrives or the channel is closed. Returns (value, true)
	// for a value, or (null, false) once the channel is closed and drained — the
	// standard `let v, ok = recv(ch)` loop condition.
	//
	// @sig     recv(ch: channel) -> (any, bool)
	// @param   ch  the channel to receive from
	// @returns (value, true) when a value is available; (null, false) when closed and empty
	// @errors  TypeError if ch is not a channel
	// @example no-run let v, ok = recv(ch)
	// @since   0.1.0
	// @see     send, recvNonBlock, channel, close
	"recv": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("recv expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("recv: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		val, open := <-ch.ch
		if !open {
			return &Tuple{Elements: []Object{NULL, FALSE}}
		}
		return &Tuple{Elements: []Object{val, TRUE}}
	}},

	// recvNonBlock — receive from a channel without blocking.
	//
	// Returns a value if one is immediately available, otherwise null (empty, or
	// closed with nothing buffered). Used for cooperative cancellation polling in
	// parallel workers.
	//
	// @sig     recvNonBlock(ch: channel) -> any
	// @param   ch  the channel to poll
	// @returns the next value, or null if none is ready
	// @errors  TypeError if ch is not a channel
	// @example no-run let v = recvNonBlock(ch)
	// @since   0.1.0
	// @see     recv, send, channel
	"recvNonBlock": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("recvNonBlock expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("recvNonBlock: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		select {
		case val := <-ch.ch:
			return val
		default:
			return NULL
		}
	}},

	// close — signal that no more values will be sent on a channel.
	//
	// Receivers drain any buffered values, then recv returns (null, false). Use
	// close to end a producer cleanly; use cancel to stop from the consumer side.
	//
	// @sig     close(ch: channel) -> null
	// @param   ch  the channel to close
	// @returns null on success
	// @errors  TypeError if ch isn't a channel; RuntimeError if it's already closed
	// @example no-run close(ch)
	// @since   0.1.0
	// @see     cancel, send, recv, channel
	"close": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("close expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("close: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		var result Object = NULL
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = runtimeError("close: channel is already closed", ast.Pos{})
				}
			}()
			close(ch.ch)
		}()
		return result
	}},

	// sleep — pause execution for a number of milliseconds.
	//
	// @sig     sleep(ms: int) -> null
	// @param   ms  milliseconds to pause
	// @returns null
	// @errors  TypeError if ms is not an integer
	// @example no-run sleep(500)
	// @since   0.1.0
	// @see     async, await
	"sleep": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sleep expects 1 argument", ast.Pos{})
		}
		ms, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("sleep: argument must be an integer (milliseconds), got %s", args[0].Type()), ast.Pos{})
		}
		time.Sleep(time.Duration(ms.Value) * time.Millisecond)
		return NULL
	}},
}

// init registers builtins that need to call Eval (higher-order functions).
// These cannot be in the Builtins var literal because Go's initialisation
// cycle checker sees: Builtins → closure → Eval → (indirectly) Builtins.
// init() runs after all functions are fully defined, so no cycle exists.
func init() {
	// apply — call fn with the elements of args as its positional arguments.
	//
	// The spread/variadic-call operator: where fn(a, b, c) is fixed at parse time,
	// apply(fn, [a, b, c]) builds the argument list at runtime — essential for
	// partial, flip, curry, and pipelines that forward an arbitrary-arity call.
	//
	// @sig     apply(fn: function, args: array) -> any
	// @param   fn    the function (or builtin) to call
	// @param   args  an array whose elements become fn's positional arguments
	// @returns whatever fn returns
	// @errors  TypeError if fn isn't callable or args isn't an array; any error fn raises propagates
	// @example apply(fn(a, b) { a + b }, [3, 4])   → 7
	// @since   0.1.0
	// @see     filter, map, reduce
	Builtins["apply"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("apply expects 2 arguments (fn, args)", ast.Pos{})
		}
		if !IsCallable(args[0]) {
			return typeError(fmt.Sprintf("apply: first argument must be function, got %s", args[0].Type()), ast.Pos{})
		}
		argArr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("apply: second argument must be array, got %s", args[1].Type()), ast.Pos{})
		}
		result, errObj := callCallable(args[0], argArr.Elements)
		if errObj != nil {
			return errObj
		}
		return result
	}}

	// filter — a new array of the elements for which fn returns true.
	//
	// Calls fn(element) for each item and keeps those where it returns true. fn
	// must take one argument and return a bool. The input is not modified.
	//
	// @sig     filter(arr: array, fn: function) -> array
	// @param   arr  the array to filter
	// @param   fn   a one-argument predicate returning a bool
	// @returns a new array of the accepted elements, in order
	// @errors  TypeError if arr isn't an array, fn isn't callable, or fn returns a non-bool; any error fn raises propagates
	// @example filter([1, 2, 3, 4], fn(x) { x > 2 })   → [3, 4]
	// @since   0.1.0
	// @see     map, reduce
	Builtins["filter"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("filter expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("filter: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if fn.NumRequired != 1 {
				return runtimeError(fmt.Sprintf("filter: function must take 1 argument, got %d required", fn.NumRequired), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			if !IsCallable(args[1]) {
				return typeError(fmt.Sprintf("filter: second argument must be function, got %s", args[1].Type()), ast.Pos{})
			}
		}
		out := make([]Object, 0, len(arr.Elements))
		for _, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{el})
			if err != nil {
				return err
			}
			b, ok := result.(*Boolean)
			if !ok {
				return typeError(fmt.Sprintf("filter: function must return bool, got %s", result.Type()), ast.Pos{})
			}
			if b.Value {
				out = append(out, el)
			}
		}
		return &Array{Elements: out}
	}}

	// reduce — fold an array into a single value with an accumulator.
	//
	// Calls fn(accumulator, element) for each element left to right, starting from
	// init, and returns the final accumulator. The classic sum/product/build-up
	// operation.
	//
	// @sig     reduce(arr: array, fn: function, init: any) -> any
	// @param   arr   the array to fold
	// @param   fn    a two-argument function fn(accumulator, element)
	// @param   init  the starting accumulator value
	// @returns the final accumulator after folding every element
	// @errors  TypeError if arr is not an array or fn is not callable; any error fn raises propagates
	// @example reduce([1, 2, 3, 4], fn(acc, x) { acc + x }, 0)   → 10
	// @since   0.1.0
	// @see     map, filter
	Builtins["reduce"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("reduce expects 3 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("reduce: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if fn.NumRequired != 2 {
				return runtimeError(fmt.Sprintf("reduce: function must take 2 arguments, got %d required", fn.NumRequired), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			if !IsCallable(args[1]) {
				return typeError(fmt.Sprintf("reduce: second argument must be function, got %s", args[1].Type()), ast.Pos{})
			}
		}
		accumulator := args[2] // start with the initial value
		for _, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{accumulator, el})
			if err != nil {
				return err
			}
			accumulator = result
		}
		return accumulator
	}}

	// map — apply a function to every element, returning a new array.
	//
	// Calls fn(element) for each item in order and collects the results. fn must
	// take exactly one argument. The input array is not modified.
	//
	// @sig     map(arr: array, fn: function) -> array
	// @param   arr  the array to transform
	// @param   fn   a one-argument function applied to each element
	// @returns a new array of the same length holding each fn(element)
	// @errors  TypeError if arr is not an array or fn is not callable; any error fn raises propagates
	// @example map([1, 2, 3], fn(x) { x * 2 })   → [2, 4, 6]
	// @since   0.1.0
	// @see     filter, reduce
	Builtins["map"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("map expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("map: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if fn.NumRequired != 1 {
				return runtimeError(fmt.Sprintf("map: function must take 1 argument, got %d required", fn.NumRequired), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			if !IsCallable(args[1]) {
				return typeError(fmt.Sprintf("map: second argument must be function, got %s", args[1].Type()), ast.Pos{})
			}
		}
		out := make([]Object, len(arr.Elements))
		for i, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{el})
			if err != nil {
				return err
			}
			out[i] = result
		}
		return &Array{Elements: out}
	}}

	// error — build a first-class Error value with a code and message.
	//
	// The returned Error is a VALUE, not a raised/propagating error — it sits in a
	// variable and is inspected via .code, .message, and .is(code). Return it as
	// the second element of a (value, err) tuple to signal failure.
	//
	// @sig     error(code: string, message: string) -> error
	// @param   code     a short machine-readable code, e.g. "NOT_FOUND"
	// @param   message  a human-readable description
	// @returns an Error value carrying code and message
	// @errors  TypeError if code or message is not a string
	// @example error("NOT_FOUND", "missing").code   → NOT_FOUND
	// @since   0.1.0
	// @see     isError, safe
	Builtins["error"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("error() expects 2 arguments: code, message", ast.Pos{})
		}
		code, ok1 := args[0].(*String)
		msg, ok2 := args[1].(*String)
		if !ok1 {
			return typeError("error() first argument (code) must be a string", ast.Pos{})
		}
		if !ok2 {
			return typeError("error() second argument (message) must be a string", ast.Pos{})
		}
		return &Error{IsUserError: true, Code: code.Value, Message: msg.Value}
	}}

	// safe — call a function and capture any error as a (value, err) tuple.
	//
	// Runs fn(...args) and, instead of letting a runtime/type error propagate,
	// returns it in the second slot of a tuple. This is the canonical way to ask
	// "did this fail?": `let v, err = safe(fn, ...)`. A function that itself
	// returns error(...) is routed the same way; a function returning a tuple is
	// passed through unchanged.
	//
	// @sig     safe(fn: function, args...: any) -> (any, error)
	// @param   fn    the function (or builtin) to call
	// @param   args  arguments forwarded to fn
	// @returns (result, null) on success; (null, error) on failure
	// @errors  none — failures are returned in the tuple, never raised; err.code is "RUNTIME_ERROR" or "TYPE_ERROR" and err.message carries the detail
	// @example safe(fn() { 42 })   → (42, null)
	// @since   0.1.0
	// @see     error, async
	Builtins["safe"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("safe expects at least 1 argument", ast.Pos{})
		}
		callArgs := args[1:]
		// Delegate to callCallable so VM-compiled functions
		// (*CompiledFunction registered via ExternalCallable) work
		// here too — without this, `safe(fn() { ... })` would
		// reject the closure with "not callable" because safe only
		// recognised *Function / *Builtin directly.
		result, errObj := callCallable(args[0], callArgs)
		if errObj != nil {
			// The callee errored. Wrap into a (null, ERR) tuple.
			// The error may not be a *Error if the dispatch hook
			// returned something unusual; guard the type assertion.
			if e, ok := errObj.(*Error); ok {
				code := "RUNTIME_ERROR"
				if e.Kind == TypeError {
					code = "TYPE_ERROR"
				}
				return &Tuple{Elements: []Object{NULL, &Error{IsUserError: true, Code: code, Message: e.Message}}}
			}
			return &Tuple{Elements: []Object{NULL, errObj}}
		}
		// If the function returned an Error VALUE (via `return error(...)`)
		// rather than propagating one, route it into the err slot of the
		// tuple. This matches the mental model that `_, err = safe(fn)` is
		// always the way to ask "did this fail?" — regardless of whether the
		// callee bailed by raising a runtime error or by returning error().
		if e, ok := result.(*Error); ok {
			return &Tuple{Elements: []Object{NULL, e}}
		}
		if t, ok := result.(*Tuple); ok {
			return t
		}
		return &Tuple{Elements: []Object{result, NULL}}
	}}

	// async — run a function in the background, returning a Task immediately.
	//
	// The function runs in a snapshot of the current environment: it can read
	// globals as they were when the task launched, but its mutations are
	// task-local and invisible to the caller (no shared-state races). Collect the
	// result with await. Accepts user functions and builtins. Do not call input()
	// from async — it shares one global stdin reader.
	//
	// @sig     async(fn: function, args...: any) -> task
	// @param   fn    the function to run in the background
	// @param   args  arguments forwarded to fn
	// @returns a Task; pass it to await to block for the result
	// @errors  TypeError if fn is not callable
	// @example await(async(fn() { 21 + 21 }))   → 42
	// @since   0.1.0
	// @see     await, safe, channel
	//
	// impl: RetainsArgs=true — async keeps args[1:] alive in the spawned
	// goroutine that runs AFTER Fn returns, so the VM's OpCallBuiltin must
	// allocate a fresh args slice instead of reusing a pooled buffer (M4 audit
	// fix 2026-05-22); otherwise the next caller's args clobber what the
	// goroutine still reads.
	Builtins["async"] = &Builtin{RetainsArgs: true, Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("async expects at least 1 argument", ast.Pos{})
		}
		fnArgs := args[1:]
		// M5 lazy mutex: the goroutine spawned below reads the args
		// concurrently with the caller. Mark every reachable mutable
		// container so Hash accesses on either side acquire mu. Done
		// BEFORE `go func()` so the atomic.Store happens-before the
		// goroutine start; the new goroutine sees shared=true and
		// uses the lock.
		for _, a := range fnArgs {
			MarkSharedRecursive(a)
		}
		// Agentic hook (Phase 2, 2026-05-23): validate callability
		// UPFRONT so we don't fire an on_async_spawn event for a task
		// that was never going to run. IsCallable covers *Function,
		// *Builtin, and *vm.CompiledFunction (via the type-tag check).
		if !IsCallable(args[0]) {
			return typeError(fmt.Sprintf("async: first argument must be a function, got %s", args[0].Type()), ast.Pos{})
		}
		task := getTask()

		// Fire on_async_spawn before launching. The agent sees the
		// spawn synchronously here; the matching on_async_done fires
		// from inside the goroutine when the task body returns,
		// carrying the same task_id so events can be paired.
		//
		// spawnEventID is also captured: the child goroutine pushes
		// this onto its own causal stack so any events fired by the
		// task body inherit the spawn as their caused_by parent.
		taskID := NextAsyncTaskID()
		spawnedAt := time.Now().UnixNano()
		spawnEventID := FireAsyncSpawnHook(taskID, AsyncCalleeName(args[0]), len(fnArgs), spawnedAt)

		switch fn := args[0].(type) {
		case *Function:
			// H1 (2026-05-22): match tree-walker's env-snapshot
			// semantic when this Fn is reached without going through
			// evalCall's intercept (i.e. when async() is dispatched
			// from VM code, or via eval-side higher-order
			// indirection like `tasks = map(workers, async)`). The
			// intercept path in evalCall snapshots the CALLER's env;
			// here we have no caller env, so we snapshot the
			// function's own lexical env. For module-level functions
			// fn.Env IS the module env (same data the tree-walker
			// would snapshot when async is called from that module);
			// for closures fn.Env is the lexical env at definition
			// time (arguably MORE correct than the tree-walker's
			// dynamic-scope-leaning behaviour). Either way the task
			// body now has its own isolated view of globals instead
			// of racing with the caller on fn.Env directly under
			// the env mutex.
			taskEnv := fn.Env.Snapshot()
			go func() {
				if spawnEventID != 0 {
					pushEvent(spawnEventID)
					defer popEvent()
				}
				startNs := time.Now().UnixNano()
				result, err := applyFunctionInEnv(fn, fnArgs, taskEnv)
				if err != nil {
					task.result = err
				} else {
					task.result = result
				}
				task.done.Store(true)
				FireAsyncDoneHook(taskID, task.result, time.Now().UnixNano()-startNs)
			}()
		case *Builtin:
			go func() {
				if spawnEventID != 0 {
					pushEvent(spawnEventID)
					defer popEvent()
				}
				startNs := time.Now().UnixNano()
				task.result = fn.Fn(fnArgs)
				task.done.Store(true)
				FireAsyncDoneHook(taskID, task.result, time.Now().UnixNano()-startNs)
			}()
		default:
			// VM closures (and any future external callable) dispatch
			// through ExternalCallableAsync (preferred — snapshots
			// primitive upvalues so the task is isolated) or fall
			// back to ExternalCallable. IsCallable already returned
			// true above, so a sane callable is guaranteed; the
			// dispatched=false path remains as belt-and-braces for
			// unusual external-callable shapes.
			callee := args[0]
			// M5 lazy mutex (audit follow-up 2026-05-22): MUST
			// mark the closure's reachable upvalue state BEFORE
			// the goroutine starts, otherwise concurrent sibling
			// async-spawners would race on the same MarkShared
			// walk inside the new goroutines. Runs in the
			// SPAWNER's context so it's serialised against any
			// other spawn the spawner initiates. M2: hook has a
			// default no-op stub — no nil-check needed.
			MarkExternalUpvaluesShared(callee)
			// M2: both hooks have default no-op stubs. Prefer
			// async dispatch for VM closures (it snapshots
			// primitive upvalues); fall back to ExternalCallable
			// when the async hook reports dispatched=false.
			go func() {
				if spawnEventID != 0 {
					pushEvent(spawnEventID)
					defer popEvent()
				}
				startNs := time.Now().UnixNano()
				result, dispatched := ExternalCallableAsync(callee, fnArgs)
				if !dispatched {
					result, dispatched = ExternalCallable(callee, fnArgs)
				}
				if !dispatched {
					task.result = typeError(fmt.Sprintf("async: dispatch lost callable %s", callee.Type()), ast.Pos{})
				} else if result == nil {
					task.result = NULL
				} else {
					task.result = result
				}
				task.done.Store(true)
				FireAsyncDoneHook(taskID, task.result, time.Now().UnixNano()-startNs)
			}()
		}
		return task
	}}

	// Cache the pointer so evalCall can detect the async call site by
	// identity rather than by string-comparing the identifier name on
	// every builtin invocation.
	asyncBuiltin = Builtins["async"]
}

// evalAsync handles the async builtin with environment snapshots.
// It receives the snapshotted environment from evalCall and launches the task
// in that isolated environment, eliminating mutex contention.
func evalAsync(args []Object, env *Environment) Object {
	if len(args) < 1 {
		return runtimeError("async expects at least 1 argument", ast.Pos{})
	}
	fnArgs := args[1:]
	// M5 lazy mutex: mark every reachable container in the args
	// before the goroutine starts, so Hash accesses on either side
	// serialise via mu. See the Builtins["async"].Fn variant for
	// the same instrumentation.
	for _, a := range fnArgs {
		MarkSharedRecursive(a)
	}
	// Agentic hook (Phase 2): validate callability up-front so we
	// don't emit on_async_spawn for a task we'd reject below.
	if _, isFn := args[0].(*Function); !isFn {
		if _, isBI := args[0].(*Builtin); !isBI {
			return typeError(fmt.Sprintf("async: first argument must be a function, got %s", args[0].Type()), ast.Pos{})
		}
	}
	task := getTask()

	// Snapshot the current environment for this task.
	// The task will run in this snapshot: it can read globals but mutations are local.
	taskEnv := env.Snapshot()

	// Fire on_async_spawn (Phase 2 hook). See the Builtins["async"]
	// variant for the rationale — this path runs when the tree-walker
	// dispatches async() via evalCall's intercept, so both paths must
	// emit the same lifecycle events for the agent to see all tasks.
	taskID := NextAsyncTaskID()
	spawnEventID := FireAsyncSpawnHook(taskID, AsyncCalleeName(args[0]), len(fnArgs), time.Now().UnixNano())

	switch fn := args[0].(type) {
	case *Function:
		go func() {
			if spawnEventID != 0 {
				pushEvent(spawnEventID)
				defer popEvent()
			}
			startNs := time.Now().UnixNano()
			result, err := applyFunctionInEnv(fn, fnArgs, taskEnv)
			if err != nil {
				task.result = err
			} else {
				task.result = result
			}
			task.done.Store(true)
			FireAsyncDoneHook(taskID, task.result, time.Now().UnixNano()-startNs)
		}()
	case *Builtin:
		go func() {
			if spawnEventID != 0 {
				pushEvent(spawnEventID)
				defer popEvent()
			}
			startNs := time.Now().UnixNano()
			task.result = fn.Fn(fnArgs)
			task.done.Store(true)
			FireAsyncDoneHook(taskID, task.result, time.Now().UnixNano()-startNs)
		}()
	}
	return task
}

// ============================================================================
// BUILTIN INITIALIZATION
// ============================================================================

func init() {
	// await — block until a task finishes and return its result.
	//
	// Collects the value produced by an async task. If the task's function raised
	// an error, await re-raises it here.
	//
	// @sig     await(task: task) -> any
	// @param   task  a Task from async
	// @returns the task's result
	// @errors  TypeError if task is not a Task; any error the task raised propagates
	// @example await(async(fn() { 6 * 7 }))   → 42
	// @since   0.1.0
	// @see     async, channel
	Builtins["await"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("await expects 1 argument", ast.Pos{})
		}
		task, ok := args[0].(*Task)
		if !ok {
			return typeError(fmt.Sprintf("await: argument must be a task, got %s", args[0].Type()), ast.Pos{})
		}
		// Hybrid strategy: spin briefly (fast-path for quick completions),
		// then sleep (prevents busy-waiting for slower tasks).
		for i := 0; i < 100; i++ {
			if task.done.Load() {
				result := task.result
				returnTask(task)
				return result
			}
		}
		for !task.done.Load() {
			time.Sleep(100 * time.Microsecond)
		}
		result := task.result
		returnTask(task)
		return result
	}}
}

// computeNumRequired walks the defaults slice to find the first non-nil
// entry, which (since the parser enforces defaults-must-come-last) is the
// boundary between required and optional params. Called exactly once per
// Function — at construction — so the cached fn.NumRequired field is O(1)
// for every subsequent read.
func computeNumRequired(defaults []ast.Node, paramCount int) int {
	for i, d := range defaults {
		if d != nil {
			return i
		}
	}
	return paramCount
}

// arityError builds a clear argument-count error message that accounts for
// optional (defaulted) parameters.
func arityError(name string, fn *Function, got int, pos ast.Pos) *Error {
	req := fn.NumRequired
	total := len(fn.Params)
	var msg string
	if req == total {
		msg = fmt.Sprintf("%s expects %d argument(s), got %d", name, total, got)
	} else {
		msg = fmt.Sprintf("%s expects %d to %d argument(s), got %d", name, req, total, got)
	}
	return runtimeError(msg, pos)
}

// bindArgs binds call arguments to parameter names in env, filling any
// missing trailing arguments from their default expressions evaluated in
// the function's closure environment (fn.Env).
func bindArgs(fn *Function, args []Object, env *Environment) Object {
	for i, param := range fn.Params {
		if i < len(args) {
			env.Set(param, args[i])
		} else {
			// Missing arg — evaluate default in the closure env.
			defVal := Eval(fn.Defaults[i], fn.Env)
			if isError(defVal) {
				return defVal
			}
			// A default must satisfy its own parameter annotation.
			if fn.TypeChecked && i < len(fn.ParamTypes) {
				if errObj := CheckDefaultAnnotation(fn.ParamTypes[i], param, defVal, fnDisplayName(fn), ast.Pos{}); errObj != nil {
					return errObj
				}
			}
			env.Set(param, defVal)
		}
	}
	return nil
}

// applyFunction calls a user-defined Function with the given arguments and
// returns the result. It is used by higher-order builtins (map, filter, reduce)
// that need to invoke kLex functions from inside Go code.
// Returns (result, nil) on success, or (nil, *Error) on failure.
func applyFunction(fn *Function, args []Object) (Object, Object) {
	env := &Environment{
		store:  make(map[string]Object, len(fn.Params)),
		outer:  fn.Env,
		shared: fn.Env.shared,
	}
	if fn.Variadic {
		required := len(fn.Params) - 1
		if len(args) < required {
			return nil, runtimeError(
				fmt.Sprintf("function expects at least %d argument(s), got %d", required, len(args)),
				ast.Pos{},
			)
		}
		for i := 0; i < required; i++ {
			env.Set(fn.Params[i], args[i])
		}
		env.Set(fn.Params[required], &Array{Elements: args[required:]})
	} else {
		req := fn.NumRequired
		if len(args) < req || len(args) > len(fn.Params) {
			return nil, arityError("function", fn, len(args), ast.Pos{})
		}
		if errObj := bindArgs(fn, args, env); errObj != nil {
			return nil, errObj
		}
	}
	if fn.TypeChecked {
		if errObj := checkArgTypes(fn, args, fnDisplayName(fn), ast.Pos{}); errObj != nil {
			return nil, errObj
		}
	}
	var result Object = NULL
	for _, node := range fn.Body {
		result = Eval(node, env)
		if isReturn(result) {
			rv := result.(*ReturnValue).Value
			if fn.TypeChecked {
				if errObj := checkReturnType(fn, rv, fnDisplayName(fn), ast.Pos{}); errObj != nil {
					return nil, errObj
				}
			}
			return rv, nil
		}
		if isError(result) {
			return nil, result
		}
	}
	if fn.TypeChecked {
		if errObj := checkReturnType(fn, result, fnDisplayName(fn), ast.Pos{}); errObj != nil {
			return nil, errObj
		}
	}
	return result, nil
}

// applyFunctionInEnv is like applyFunction but runs the function in an explicit
// environment instead of fn.Env. Used by async tasks to run in a snapshotted env.
func applyFunctionInEnv(fn *Function, args []Object, taskEnv *Environment) (Object, Object) {
	env := &Environment{
		store:  make(map[string]Object, len(fn.Params)),
		outer:  taskEnv,
		shared: taskEnv.shared,
	}
	if fn.Variadic {
		required := len(fn.Params) - 1
		if len(args) < required {
			return nil, runtimeError(
				fmt.Sprintf("function expects at least %d argument(s), got %d", required, len(args)),
				ast.Pos{},
			)
		}
		for i := 0; i < required; i++ {
			env.Set(fn.Params[i], args[i])
		}
		env.Set(fn.Params[required], &Array{Elements: args[required:]})
	} else {
		req := fn.NumRequired
		if len(args) < req || len(args) > len(fn.Params) {
			return nil, arityError("function", fn, len(args), ast.Pos{})
		}
		if errObj := bindArgs(fn, args, env); errObj != nil {
			return nil, errObj
		}
	}
	if fn.TypeChecked {
		if errObj := checkArgTypes(fn, args, fnDisplayName(fn), ast.Pos{}); errObj != nil {
			return nil, errObj
		}
	}
	var result Object = NULL
	for _, node := range fn.Body {
		result = Eval(node, env)
		if isReturn(result) {
			rv := result.(*ReturnValue).Value
			if fn.TypeChecked {
				if errObj := checkReturnType(fn, rv, fnDisplayName(fn), ast.Pos{}); errObj != nil {
					return nil, errObj
				}
			}
			return rv, nil
		}
		if isError(result) {
			return nil, result
		}
	}
	if fn.TypeChecked {
		if errObj := checkReturnType(fn, result, fnDisplayName(fn), ast.Pos{}); errObj != nil {
			return nil, errObj
		}
	}
	return result, nil
}

// callCallable invokes either a user-defined Function or a Builtin with the
// given arguments. Returns (result, nil) on success, (nil, *Error) on failure.
// Used by map/filter/reduce so they accept both function types uniformly.
func callCallable(fn Object, args []Object) (Object, Object) {
	// M1+M2 (audit fix, 2026-05-22): VM closures
	// (*CompiledFunction) are the most common callable now that
	// --vm is the default. Check by type-tag directly — no hook
	// indirection, no nil-check (ExternalCallable defaults to a
	// no-op stub when vm isn't linked).
	if fn != nil && fn.Type() == COMPILED_FUNCTION_OBJ {
		if result, dispatched := ExternalCallable(fn, args); dispatched {
			if isError(result) {
				return nil, result
			}
			return result, nil
		}
	}
	switch f := fn.(type) {
	case *Function:
		return applyFunction(f, args)
	case *Builtin:
		result := f.Fn(args)
		if isError(result) {
			return nil, result
		}
		return result, nil
	}
	return nil, typeError(fmt.Sprintf("not callable: %s — only functions and builtins can be invoked with f(args). Did you forget `fn` or accidentally shadow the name?", fn.Type()), ast.Pos{})
}

// toFloat64 extracts the numeric value from an Integer or Float as float64.
// Must only be called after canArithmetic/canCompare has confirmed the type is
// INTEGER_OBJ or FLOAT_OBJ — the fallback 0 is unreachable in correct usage.
func toFloat64(o Object) float64 {
	switch v := o.(type) {
	case *Integer:
		return float64(v.Value)
	case *Float:
		return v.Value
	}
	panic("toFloat64: called with non-numeric type " + string(o.Type()))
}

// -------------------- HASH KEY --------------------

// toHashKey converts a kLex Object into a HashKey suitable for use as a Go
// map key. Only string, integer, and boolean values are hashable.
//
// We combine the Type and a string representation so that integer 1 and
// string "1" produce different keys — {"1": "a", 1: "b"} has two entries.
func toHashKey(obj Object, pos ast.Pos) (HashKey, Object) {
	switch o := obj.(type) {
	case *String:
		return HashKey{Type: STRING_OBJ, Value: o.Value}, nil
	case *Integer:
		return HashKey{Type: INTEGER_OBJ, Value: fmt.Sprintf("%d", o.Value)}, nil
	case *Float:
		return HashKey{Type: FLOAT_OBJ, Value: strconv.FormatFloat(o.Value, 'f', -1, 64)}, nil
	case *Boolean:
		v := "false"
		if o.Value {
			v = "true"
		}
		return HashKey{Type: BOOLEAN_OBJ, Value: v}, nil
	default:
		return HashKey{}, typeError(fmt.Sprintf("unhashable type: %s — hash keys must be string, integer, float, or boolean", obj.Type()), pos)
	}
}

// -------------------- HELPERS --------------------

func isError(obj Object) bool {
	if e, ok := obj.(*Error); ok {
		return !e.IsUserError
	}
	return false
}

// IsError is the exported form of isError for use by main.
func IsError(obj Object) bool { return isError(obj) }

func isReturn(obj Object) bool {
	return obj != nil && obj.Type() == RETURN_OBJ
}

func isBreak(obj Object) bool {
	return obj != nil && obj.Type() == BREAK_OBJ
}

func isContinue(obj Object) bool {
	return obj != nil && obj.Type() == CONTINUE_OBJ
}

// toBool extracts the bool value from a Boolean object.
// Returns (false, false) if the object is not a Boolean — the caller then
// produces a TypeError. This is how kLex enforces that conditions must be bool.
func toBool(obj Object) (bool, bool) {
	b, ok := obj.(*Boolean)
	if !ok {
		return false, false
	}
	return b.Value, true
}

// primitiveEqual is the canonical value-equality check for the primitive
// types (Integer, Float, String, Bytes, Boolean, Null). It is the shared
// source of truth used by both evalEquals (for the kLex `==` operator) and
// valuesEqual (for atomicHashCAS).
//
// Returns (handled, equal):
//   - handled=true  → both args are primitives of the same type and
//     `equal` is the result of the value comparison.
//   - handled=false → at least one arg is nil, the types differ, or the
//     type isn't a primitive (e.g. *Array, *Function,
//     *EnumInstance). Callers dispatch their own rules
//     (reference-equality, recursive enum compare, etc.).
//
// Keeping this in one place means a future numeric coercion rule or new
// primitive type lands once and both `==` and CAS pick it up automatically.
func primitiveEqual(a, b Object) (handled bool, equal bool) {
	if a == nil || b == nil {
		return false, false
	}
	if a.Type() != b.Type() {
		return false, false
	}
	switch av := a.(type) {
	case *Integer:
		return true, av.Value == b.(*Integer).Value
	case *Float:
		return true, av.Value == b.(*Float).Value
	case *String:
		return true, av.Value == b.(*String).Value
	case *Bytes:
		return true, bytes.Equal(av.Value, b.(*Bytes).Value)
	case *Boolean:
		return true, av.Value == b.(*Boolean).Value
	case *Null:
		return true, true
	}
	return false, false
}

// evalEquals handles == comparisons.
// Special rules:
//   - null == null  → true  (null is a real value, not an error)
//   - null == T     → false (not a TypeError — enables null-check patterns like x == null)
//   - T == U (different non-null types) → TypeError
//   - Same-type values → compare by value
func evalEquals(left, right Object, pos ast.Pos) Object {
	if left.Type() == NULL_OBJ || right.Type() == NULL_OBJ {
		return boolObj(left.Type() == NULL_OBJ && right.Type() == NULL_OBJ)
	}

	// EnumInstance can be compared to EnumVariant for switch pattern matching:
	// Shape.Circle(5.0) == Shape.Circle  →  true (same type and variant name)
	// The check is symmetric: both orderings are valid.
	if li, ok := left.(*EnumInstance); ok {
		switch r := right.(type) {
		case *EnumVariant:
			return boolObj(li.TypeName == r.TypeName && li.VariantName == r.VariantName)
		case *EnumInstance:
			// handled below after the type check
		default:
			return FALSE
		}
	}
	if lv, ok := left.(*EnumVariant); ok {
		switch r := right.(type) {
		case *EnumInstance:
			return boolObj(lv.TypeName == r.TypeName && lv.VariantName == r.VariantName)
		case *EnumVariant:
			return boolObj(lv.TypeName == r.TypeName && lv.VariantName == r.VariantName)
		default:
			return FALSE
		}
	}

	if left.Type() != right.Type() {
		return typeError(fmt.Sprintf("cannot compare %s and %s", left.Type(), right.Type()), pos)
	}

	// Primitive value comparison (Integer, Float, String, Bytes, Boolean).
	// Shared with valuesEqual via primitiveEqual so both `==` and atomicHashCAS
	// follow exactly the same rules — no "KEEP IN SYNC" landmine.
	if handled, eq := primitiveEqual(left, right); handled {
		return boolObj(eq)
	}

	// Structural-equality types: walk fields.
	if l, ok := left.(*EnumInstance); ok {
		r := right.(*EnumInstance)
		if l.TypeName != r.TypeName || l.VariantName != r.VariantName {
			return FALSE
		}
		for name, lv := range l.Fields {
			rv, ok := r.Fields[name]
			if !ok {
				return FALSE
			}
			eq := evalEquals(lv, rv, pos)
			if isError(eq) {
				return eq
			}
			if eq == FALSE {
				return FALSE
			}
		}
		return TRUE
	}

	// Reference-equality default — covers *Array, *Hash, *Function,
	// *Builtin, *Channel, *vm.CompiledFunction, and any other reference
	// type that other packages register as Objects. Two reference
	// values are == iff they're the same pointer. This is the rule the
	// tree-walker has always applied to callables, and it's the only
	// sensible default for "identical type, primitiveEqual didn't
	// match" — content-equality on mutable containers would need a
	// deep walk that kLex doesn't expose via `==`.
	return boolObj(left == right)
}

// evalOrderCompare handles <, >, <=, >= for integers, floats, and strings.
// Mixed integer/float is allowed (integer is promoted to float).
// String comparison is lexicographic (Unicode code point order).
// Any other type combination is a TypeError.
func evalNumericCompare(left, right Object, op string, pos ast.Pos) Object {
	if !canCompare(left.Type()) || !canCompare(right.Type()) {
		return typeMismatchError(op, left.Type(), right.Type(), pos)
	}
	if left.Type() == STRING_OBJ && right.Type() == STRING_OBJ {
		l := left.(*String).Value
		r := right.(*String).Value
		switch op {
		case "<":
			return boolObj(l < r)
		case ">":
			return boolObj(l > r)
		case "<=":
			return boolObj(l <= r)
		case ">=":
			return boolObj(l >= r)
		}
	}
	l := toFloat64(left)
	r := toFloat64(right)
	switch op {
	case "<":
		return boolObj(l < r)
	case ">":
		return boolObj(l > r)
	case "<=":
		return boolObj(l <= r)
	case ">=":
		return boolObj(l >= r)
	}
	return runtimeError("unknown comparison operator: "+op, pos)
}

// evalLogical handles && and || with proper short-circuit evaluation.
// For &&: if left is false, return false immediately without evaluating right.
// For ||: if left is true, return true immediately without evaluating right.
func evalLogical(n *ast.InfixExpr, env *Environment) Object {
	left := Eval(n.Left, env)
	if isError(left) {
		return left
	}
	if !canLogical(left.Type()) {
		return typeError(fmt.Sprintf("operator %s requires bool, got %s", n.Operator, left.Type()), n.Pos)
	}
	lval := left.(*Boolean).Value
	if n.Operator == "&&" && !lval {
		return FALSE
	}
	if n.Operator == "||" && lval {
		return TRUE
	}
	right := Eval(n.Right, env)
	if isError(right) {
		return right
	}
	if !canLogical(right.Type()) {
		return typeError(fmt.Sprintf("operator %s requires bool, got %s", n.Operator, right.Type()), n.Pos)
	}
	// right is already a *Boolean (type-checked above) — return it directly
	// rather than rewrapping. With evalEquals/evalNumericCompare now
	// returning the TRUE/FALSE singletons, this propagates singleton
	// identity through chained logical expressions too.
	return right
}

// -------------------- EVAL CALL --------------------

// evalCall handles function invocation — both user-defined functions and builtins.
//
// For user-defined functions:
//  1. A new Environment is created with its outer set to the function's closure env.
//  2. Parameters are bound to argument values in that new env.
//  3. The function body is evaluated in that new env.
//  4. If a ReturnValue signal is encountered, it is unwrapped here.
//
// This is why kLex has lexical scoping: the function body always runs inside
// the environment where the function was DEFINED (fn.Env), not where it was CALLED.
func evalCall(c *ast.CallExpr, env *Environment) Object {
	// If this is a dot call (obj.method(...)), resolve the receiver first so we
	// can bind `self` inside the function's environment.
	var selfReceiver Object
	if dotExpr, ok := c.Function.(*ast.DotExpr); ok {
		recv := Eval(dotExpr.Left, env)
		if isError(recv) {
			return recv
		}
		switch recv.(type) {
		case *StructInstance:
			selfReceiver = recv
		}
	}

	fnObj := Eval(c.Function, env)
	if isError(fnObj) {
		return fnObj
	}

	// Special handling for async: it needs access to the environment to snapshot it.
	// Identity-compare against the cached async pointer so this check is one
	// branch instead of a type-assert + string-compare on every builtin call.
	if fnObj == asyncBuiltin {
		args := make([]Object, 0, len(c.Args))
		for _, argNode := range c.Args {
			val := Eval(argNode, env)
			if isError(val) {
				return val
			}
			args = append(args, val)
		}
		return evalAsync(args, env)
	}

	// Same env-special-casing for _scriptDir(). Walks the env's outer
	// chain via env.ScriptDir() — so inside an imported module it returns
	// the module's own dir, not the entry script's. Lets scripts find
	// their sibling files (Python bridges, fonts, etc.) regardless of CWD.
	if fnObj == scriptDirBuiltin {
		if len(c.Args) != 0 {
			return runtimeError("_scriptDir expects no arguments", c.Pos)
		}
		return &String{Value: env.ScriptDir()}
	}

	// Evaluate all arguments before calling — arguments are eager, not lazy.
	args := make([]Object, 0, len(c.Args))
	for _, argNode := range c.Args {
		val := Eval(argNode, env)
		if isError(val) {
			return val
		}
		args = append(args, val)
	}

	// M1+M2 (audit fix, 2026-05-22): once --vm is the default, the
	// most common callee here is a *vm.CompiledFunction reached
	// via the env (e.g. main script's tree-walker eval calling a
	// VM-compiled imported fn). Direct type-tag check on the hot
	// path — no hook nil-check, no extra function-pointer indirect.
	if fnObj != nil && fnObj.Type() == COMPILED_FUNCTION_OBJ {
		if result, dispatched := ExternalCallable(fnObj, args); dispatched {
			if isError(result) {
				err := result.(*Error)
				err.Stack = append(err.Stack, Frame{CallPos: c.Pos})
				return err
			}
			return result
		}
	}

	switch fn := fnObj.(type) {
	case *Builtin:
		return fn.Fn(args)

	case *Function:
		name := fn.Name
		if name == "" {
			name = "anonymous"
		}
		newEnv := &Environment{
			store:  make(map[string]Object, len(fn.Params)),
			outer:  fn.Env,
			shared: fn.Env.shared,
		}
		if selfReceiver != nil {
			newEnv.Set("self", selfReceiver)
		}
		if fn.Variadic {
			required := len(fn.Params) - 1
			if len(args) < required {
				return runtimeError(
					fmt.Sprintf("%s expects at least %d argument(s), got %d", name, required, len(args)),
					c.Pos,
				)
			}
			for i := 0; i < required; i++ {
				newEnv.Set(fn.Params[i], args[i])
			}
			rest := args[required:]
			newEnv.Set(fn.Params[required], &Array{Elements: rest})
		} else {
			req := fn.NumRequired
			if len(args) < req || len(args) > len(fn.Params) {
				return arityError(name, fn, len(args), c.Pos)
			}
			if errObj := bindArgs(fn, args, newEnv); errObj != nil {
				return errObj
			}
		}

		// Enforce optional parameter type annotations at the call boundary.
		if fn.TypeChecked {
			if errObj := checkArgTypes(fn, args, name, c.Pos); errObj != nil {
				return errObj
			}
		}

		var result Object = NULL
		for _, node := range fn.Body {
			result = Eval(node, newEnv)
			if isReturn(result) {
				rv := result.(*ReturnValue).Value // unwrap the ReturnValue signal
				if fn.TypeChecked {
					if errObj := checkReturnType(fn, rv, name, c.Pos); errObj != nil {
						return errObj
					}
				}
				return rv
			}
			if isError(result) {
				err := result.(*Error)
				err.Stack = append(err.Stack, Frame{FnName: fn.Name, CallPos: c.Pos})
				return err
			}
		}
		if fn.TypeChecked {
			if errObj := checkReturnType(fn, result, name, c.Pos); errObj != nil {
				return errObj
			}
		}
		return result

	case *EnumVariant:
		if len(args) != len(fn.Fields) {
			return runtimeError(fmt.Sprintf("%s.%s expects %d argument(s), got %d",
				fn.TypeName, fn.VariantName, len(fn.Fields), len(args)), c.Pos)
		}
		fields := make(map[string]Object, len(fn.Fields))
		for i, name := range fn.Fields {
			fields[name] = args[i]
		}
		return &EnumInstance{
			TypeName:    fn.TypeName,
			VariantName: fn.VariantName,
			FieldNames:  fn.Fields,
			Fields:      fields,
		}

	default:
		// External-callable fallback (e.g. a future external callable
		// type that isn't *CompiledFunction — the fast-path above
		// already handles those). M2: ExternalCallable has a default
		// no-op stub, no nil-check needed.
		if result, dispatched := ExternalCallable(fnObj, args); dispatched {
			if isError(result) {
				err := result.(*Error)
				err.Stack = append(err.Stack, Frame{CallPos: c.Pos})
				return err
			}
			return result
		}
		return typeError(fmt.Sprintf("not a function, got %s", fnObj.Type()), c.Pos)
	}
}

// -------------------- MAIN EVAL --------------------

// Eval is the central dispatch function. It receives any AST node and returns
// the Object that node evaluates to. It is called recursively — evaluating a
// program evaluates each statement, evaluating an infix expression evaluates
// both sides, and so on.
func Eval(node ast.Node, env *Environment) Object {
	switch n := node.(type) {

	// ---------------- PROGRAM ----------------
	// Evaluate each statement in order. Stop and print if an error occurs.
	// The final statement's value is the program's result (unused in practice).
	// Leaked control-flow signals (return/break/continue outside their valid
	// context) are programming errors — convert them to RuntimeErrors rather
	// than silently swallowing them.
	case *ast.Program:
		var result Object = NULL
		for _, stmt := range n.Statements {
			result = Eval(stmt, env)
			if isError(result) {
				fmt.Println(result.Inspect())
				return result
			}
			if isReturn(result) {
				err := runtimeError("return outside function", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
			if isBreak(result) {
				err := runtimeError("break outside loop", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
			if isContinue(result) {
				err := runtimeError("continue outside loop", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
		}
		return result

	// ---------------- ASSIGNMENT ----------------
	// Evaluate the right-hand side, then store the result in the environment.
	// If the value is an anonymous function, stamp its name onto it now —
	// this is what enables recursion (the function can refer to itself by name
	// because the name is in the outer env when the body eventually runs).
	case *ast.AssignStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if n.Name == "_" {
			return val // discard — evaluate for side effects, do not store
		}
		if errObj := env.CheckWritable(n.Name); errObj != nil {
			return errObj
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		env.Assign(n.Name, val)
		return val

	case *ast.LetStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if n.Name == "_" {
			return val // discard — evaluate for side effects, do not store
		}
		// let only creates in current scope — only block if const in THIS scope.
		if env.consts != nil && env.consts[n.Name] {
			return runtimeError("cannot reassign constant "+n.Name, n.Pos)
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		env.Set(n.Name, val)
		return val

	// ---------------- CONST DECLARATION ----------------
	case *ast.ConstStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		deepFreeze(val, map[Object]bool{})
		env.SetConst(n.Name, val)
		return val

	// ---------------- ENUM DECLARATION ----------------
	// Build an EnumDef and bind it in the environment under the enum name.
	case *ast.EnumDecl:
		def := &EnumDef{
			Name:     n.Name,
			Variants: make(map[string][]string, len(n.Variants)),
		}
		for _, v := range n.Variants {
			def.Variants[v.Name] = v.Fields
		}
		env.Assign(n.Name, def)
		return def

	// ---------------- STRUCT DECLARATION ----------------
	// Build a StructDef and bind it in the environment under the struct name.
	case *ast.StructDecl:
		def := &StructDef{
			Name:    n.Name,
			Fields:  n.Fields,
			Methods: make(map[string]*Function),
		}
		for _, m := range n.Methods {
			def.Methods[m.Name] = &Function{
				Name:        m.Name,
				Params:      m.Params,
				ParamTypes:  m.ParamTypes,
				ReturnType:  m.ReturnType,
				Defaults:    m.Defaults,
				Variadic:    m.Variadic,
				NumRequired: computeNumRequired(m.Defaults, len(m.Params)),
				TypeChecked: HasTypeAnnotations(m.ParamTypes, m.ReturnType),
				Body:        m.Body,
				Env:         env,
			}
		}
		env.Assign(n.Name, def)
		return def

	// ---------------- STRUCT LITERAL ----------------
	// Look up the StructDef, validate field names, evaluate values, create instance.
	case *ast.StructLiteral:
		defObj, ok := env.Get(n.Name)
		if !ok {
			return runtimeError(fmt.Sprintf("undefined struct type %q", n.Name), n.Pos)
		}
		def, ok := defObj.(*StructDef)
		if !ok {
			return typeError(fmt.Sprintf("%q is not a struct type", n.Name), n.Pos)
		}
		// Check all provided fields are declared.
		declared := make(map[string]bool, len(def.Fields))
		for _, f := range def.Fields {
			declared[f] = true
		}
		provided := make(map[string]bool, len(n.Fields))
		for _, fi := range n.Fields {
			if !declared[fi.Name] {
				return runtimeError(fmt.Sprintf("struct %s has no field %q", def.Name, fi.Name), n.Pos)
			}
			if provided[fi.Name] {
				return runtimeError(fmt.Sprintf("field %q set more than once in struct literal", fi.Name), n.Pos)
			}
			provided[fi.Name] = true
		}
		// All declared fields must be initialised.
		for _, f := range def.Fields {
			if !provided[f] {
				return runtimeError(fmt.Sprintf("struct %s: field %q not initialised", def.Name, f), n.Pos)
			}
		}
		// Evaluate field values.
		fields := make(map[string]Object, len(n.Fields))
		for _, fi := range n.Fields {
			val := Eval(fi.Value, env)
			if isError(val) {
				return val
			}
			fields[fi.Name] = val
		}
		return &StructInstance{Def: def, Fields: fields}

	// ---------------- IDENTIFIER ----------------
	// Look up the variable name in the current environment chain.
	case *ast.Ident:
		if n.Value == "_" {
			return runtimeError("_ is a discard — its value cannot be read", n.Pos)
		}
		val, ok := env.Get(n.Value)
		if !ok {
			return runtimeError(undefinedIdentifierMessage(n.Value), n.Pos)
		}
		return val

	// ---------------- FUNCTION LITERAL ----------------
	// Capture the current environment as the closure. The function object
	// remembers where it was created, not where it will be called.
	case *ast.FunctionLiteral:
		return &Function{
			Params:      n.Params,
			ParamTypes:  n.ParamTypes,
			ReturnType:  n.ReturnType,
			Defaults:    n.Defaults,
			Variadic:    n.Variadic,
			NumRequired: computeNumRequired(n.Defaults, len(n.Params)),
			TypeChecked: HasTypeAnnotations(n.ParamTypes, n.ReturnType),
			Body:        n.Body,
			Env:         env, // closure captured here
		}

	// ---------------- SWITCH ----------------
	// Value switch:      switch expr { case val, val { } default { } }
	// Expression switch: switch       { case bool_expr { } default { } }
	// Cases are tried in order; first match wins, no fallthrough.
	case *ast.SwitchStmt:
		var subject Object
		if n.Subject != nil {
			subject = Eval(n.Subject, env)
			if isError(subject) {
				return subject
			}
		}
		for _, sc := range n.Cases {
			matched := false
			matchEnv := env
			for _, valNode := range sc.Values {
				if pat, ok := valNode.(*ast.EnumPattern); ok {
					if subject == nil {
						return runtimeError("enum pattern requires a switch subject", pat.Pos)
					}
					inst, ok := subject.(*EnumInstance)
					if !ok {
						break
					}
					// Resolve which variant this pattern targets.
					// Short form — case Circle(r):    match by variant name only (no type check).
					// Full form  — case Shape.Circle(r): evaluate and match type + variant.
					patVariant := ""
					skipCase := false
					if ident, ok := pat.Pattern.(*ast.Ident); ok {
						patVariant = ident.Value
					} else {
						patVal := Eval(pat.Pattern, env)
						if isError(patVal) {
							return patVal
						}
						switch pv := patVal.(type) {
						case *EnumVariant:
							if inst.TypeName != pv.TypeName || inst.VariantName != pv.VariantName {
								skipCase = true
							} else {
								patVariant = pv.VariantName
							}
						case *EnumInstance:
							if inst.TypeName != pv.TypeName || inst.VariantName != pv.VariantName {
								skipCase = true
							} else {
								patVariant = pv.VariantName
							}
						default:
							return runtimeError(fmt.Sprintf("enum pattern must reference an enum variant, got %s", patVal.Type()), pat.Pos)
						}
					}
					if skipCase || inst.VariantName != patVariant {
						break
					}
					if len(pat.Bindings) != len(inst.FieldNames) {
						return runtimeError(fmt.Sprintf(
							"%s.%s has %d field(s) but pattern binds %d",
							inst.TypeName, inst.VariantName, len(inst.FieldNames), len(pat.Bindings),
						), pat.Pos)
					}
					childEnv := &Environment{store: make(map[string]Object, len(pat.Bindings)), outer: env}
					for i, name := range pat.Bindings {
						childEnv.Set(name, inst.Fields[inst.FieldNames[i]])
					}
					matched = true
					matchEnv = childEnv
					break
				}
				val := Eval(valNode, env)
				if isError(val) {
					return val
				}
				if subject != nil {
					// Value switch: use the same equality logic as the == operator.
					eq := evalEquals(subject, val, n.Pos)
					if isError(eq) {
						return eq
					}
					if eq.(*Boolean).Value {
						matched = true
						break
					}
				} else {
					// Expression switch: each value must be a boolean.
					b, ok := val.(*Boolean)
					if !ok {
						return typeError(
							fmt.Sprintf("switch case expression must be boolean, got %s", val.Type()),
							n.Pos,
						)
					}
					if b.Value {
						matched = true
						break
					}
				}
			}
			if matched {
				var result Object = NULL
				for _, stmt := range sc.Body {
					result = Eval(stmt, matchEnv)
					if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
						return result
					}
				}
				return result
			}
		}
		// No case matched — run default if present.
		if n.HasDefault {
			var result Object = NULL
			for _, stmt := range n.Default {
				result = Eval(stmt, env)
				if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}
		// No default. If the subject is an enum instance, the unmatched variant
		// is a programming error — not a silent null. Require either a matching
		// case or an explicit default {}.
		if subject != nil {
			if inst, ok := subject.(*EnumInstance); ok {
				return runtimeError(
					fmt.Sprintf("switch: %s.%s not handled — add a case or a default",
						inst.TypeName, inst.VariantName),
					n.Pos,
				)
			}
		}
		return NULL

	// ---------------- SELECT ----------------
	// Blocks until one channel operation can proceed, then runs that case's body.
	// Uses reflect.Select so multiple channels can be waited on simultaneously.
	// If multiple cases are ready at once, one is chosen at random (Go semantics).
	// A default case makes the select non-blocking.
	case *ast.SelectStmt:
		// Build the reflect.SelectCase slice in lock-step with n.Cases so we
		// can map the chosen index back to the right body and bindings.
		reflCases := make([]reflect.SelectCase, 0, len(n.Cases))
		for _, sc := range n.Cases {
			switch sc.Kind {
			case ast.SelectRecv:
				chObj := Eval(sc.Chan, env)
				if isError(chObj) {
					return chObj
				}
				ch, ok := chObj.(*Channel)
				if !ok {
					return typeError(fmt.Sprintf("select recv: expected channel as recv() argument, got %s — e.g. `case x = recv(ch) { ... }` where ch was created by channel(n)", chObj.Type()), sc.Pos)
				}
				reflCases = append(reflCases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(ch.ch),
				})
			case ast.SelectSend:
				chObj := Eval(sc.Chan, env)
				if isError(chObj) {
					return chObj
				}
				ch, ok := chObj.(*Channel)
				if !ok {
					return typeError(fmt.Sprintf("select send: expected channel as first send() argument, got %s — e.g. `case send(ch, value) { ... }` where ch was created by channel(n)", chObj.Type()), sc.Pos)
				}
				val := Eval(sc.SendVal, env)
				if isError(val) {
					return val
				}
				// reflect.Select requires the Send value to match the channel's
				// element type exactly. chan Object has element type Object
				// (interface), so we must wrap the concrete value in an interface
				// reflect.Value rather than using the concrete type directly.
				var iface Object = val
				reflCases = append(reflCases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.ch),
					Send: reflect.ValueOf(&iface).Elem(),
				})
			case ast.SelectDefault:
				reflCases = append(reflCases, reflect.SelectCase{
					Dir: reflect.SelectDefault,
				})
			}
		}

		chosen, recvVal, recvOK := reflect.Select(reflCases)
		sc := n.Cases[chosen]

		// Build a fresh scope for the chosen case body.
		caseEnv := &Environment{store: make(map[string]Object), outer: env}

		// For recv cases, bind the received value and ok flag using Assign() so
		// that existing outer-scope variables are updated (same semantics as a
		// regular val, ok = recv(ch) assignment statement).
		if sc.Kind == ast.SelectRecv {
			var val Object
			if recvOK {
				val = recvVal.Interface().(Object)
			} else {
				val = NULL
			}
			ok := &Boolean{Value: recvOK}
			if len(sc.Vars) >= 1 && sc.Vars[0] != "_" {
				if errObj := env.CheckWritable(sc.Vars[0]); errObj != nil {
					return errObj
				}
				env.Assign(sc.Vars[0], val)
			}
			if len(sc.Vars) >= 2 && sc.Vars[1] != "_" {
				if errObj := env.CheckWritable(sc.Vars[1]); errObj != nil {
					return errObj
				}
				env.Assign(sc.Vars[1], ok)
			}
		}

		var result Object = NULL
		for _, stmt := range sc.Body {
			result = Eval(stmt, caseEnv)
			if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
				return result
			}
		}
		return result

	// ---------------- FOR IN ----------------
	// Evaluates the collection, then iterates over each element, binding it
	// to the loop variable in a fresh inner scope for each iteration.
	// The loop variable does not leak into the outer scope after the loop ends.
	// break and continue work exactly as they do in while loops.
	case *ast.ForInStmt:
		collection := Eval(n.Collection, env)
		if isError(collection) {
			return collection
		}

		runBody := func(bindings map[string]Object) Object {
			iterEnv := &Environment{store: bindings, outer: env}
			for _, stmt := range n.Body {
				result := Eval(stmt, iterEnv)
				if isError(result) || isReturn(result) {
					return result
				}
				if isBreak(result) {
					// Propagate the break sentinel up so the per-collection
					// loop below can do something only the outer code knows
					// to do — e.g. close(coll.done) on a channel for-in so
					// the producer learns the consumer abandoned the stream.
					// Replacing this with NULL silently disabled bridgeStream
					// cancel signalling, so keep it as the actual break.
					return result
				}
				if isContinue(result) {
					break
				}
			}
			return nil // nil = keep going
		}

		var result Object = NULL
		iterCount := 0

		addBinding := func(m map[string]Object, name string, val Object) {
			if name != "_" {
				m[name] = val
			}
		}

		switch coll := collection.(type) {
		case *Array:
			for i, el := range coll.Elements {
				bindings := make(map[string]Object, 2)
				if n.ValueVar == "" {
					addBinding(bindings, n.Variable, el)
				} else {
					addBinding(bindings, n.Variable, &Integer{Value: i})
					addBinding(bindings, n.ValueVar, el)
				}
				if r := runBody(bindings); r != nil {
					if isBreak(r) {
						return NULL
					}
					return r
				}
				iterCount++
				if EnableAsyncYield && iterCount%AsyncYieldInterval == 0 {
					runtime.Gosched()
				}
			}
		case *Hash:
			if n.ValueVar == "" {
				return typeError("for-in over a hash requires two variables: for k, v in hash", n.Pos)
			}
			// Snapshot under the lock so the loop body — which may
			// itself touch this or other hashes, possibly across
			// goroutines — never iterates a Go map that another
			// goroutine is mutating. OFI #3.
			pairs := coll.Snapshot()
			for _, pair := range pairs {
				bindings := make(map[string]Object, 2)
				addBinding(bindings, n.Variable, pair.Key)
				addBinding(bindings, n.ValueVar, pair.Value)
				if r := runBody(bindings); r != nil {
					if isBreak(r) {
						return NULL
					}
					return r
				}
				iterCount++
				if EnableAsyncYield && iterCount%AsyncYieldInterval == 0 {
					runtime.Gosched()
				}
			}
		case *Channel:
			if n.ValueVar != "" {
				return typeError("for-in over a channel does not support two variables", n.Pos)
			}
			for val := range coll.ch {
				bindings := make(map[string]Object, 1)
				addBinding(bindings, n.Variable, val)
				if r := runBody(bindings); r != nil {
					if isBreak(r) {
						// Signal the producer that the consumer is done.
						// Closing done is idempotent via recover.
						func() {
							defer func() { recover() }()
							close(coll.done)
						}()
						return NULL
					}
					return r
				}
				iterCount++
				if EnableAsyncYield && iterCount%AsyncYieldInterval == 0 {
					runtime.Gosched()
				}
			}
		default:
			return typeError(fmt.Sprintf("for-in requires array, hash, or channel, got %s", collection.Type()), n.Pos)
		}

		return result

	// ---------------- WHILE ----------------
	// Re-evaluate the condition before each iteration.
	// BreakSignal exits the loop. ContinueSignal skips remaining body statements
	// so the outer for{} loop re-evaluates the condition on the next iteration.
	// ReturnValue and errors pass through unchanged — they unwind the call stack.
	case *ast.WhileStmt:
		var result Object = NULL
		iterCount := 0
		for {
			cond := Eval(n.Condition, env)
			if isError(cond) {
				return cond
			}
			b, ok := toBool(cond)
			if !ok {
				return typeError(
					fmt.Sprintf("while condition must be bool, got %s (%s)", cond.Type(), cond.Inspect()),
					n.Pos,
				)
			}
			if !b {
				break
			}

			for _, stmt := range n.Body {
				result = Eval(stmt, env)
				if isError(result) {
					return result
				}
				if isReturn(result) {
					return result
				}
				if isBreak(result) {
					return NULL
				}
				if isContinue(result) {
					// Consume the signal here — it belongs to THIS loop. Leaving
					// it in `result` would leak out via `return result` below if
					// the next condition check is false (e.g. the final inner
					// iteration ended on continue), causing any enclosing loop
					// to read it as its own continue and skip its counter-bump.
					result = NULL
					break // skip remaining stmts, outer for{} re-evaluates condition
				}
			}

			iterCount++
			// Yield to scheduler only every N iterations to balance fairness with performance.
			if EnableAsyncYield && iterCount%AsyncYieldInterval == 0 {
				runtime.Gosched()
			}
		}
		return result

	// ---------------- RETURN ----------------
	// Wrap the value in ReturnValue so it can bubble up through Eval() calls
	// until evalCall() unwraps it.
	case *ast.ReturnStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		return &ReturnValue{Value: val}

	// Break and continue produce signal objects that bubble up to the while handler.
	case *ast.BreakStmt:
		return &BreakSignal{}

	case *ast.ContinueStmt:
		return &ContinueSignal{}

	// ---------------- IF ----------------
	case *ast.IfStmt:
		cond := Eval(n.Condition, env)
		if isError(cond) {
			return cond
		}
		b, ok := toBool(cond)
		if !ok {
			return typeError(
				fmt.Sprintf("if condition must be bool, got %s (%s)", cond.Type(), cond.Inspect()),
				n.Pos,
			)
		}

		if b {
			var result Object = NULL
			for _, stmt := range n.Body {
				result = Eval(stmt, env)
				if isError(result) || isReturn(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}

		if n.ElseBody != nil {
			var result Object = NULL
			for _, stmt := range n.ElseBody {
				result = Eval(stmt, env)
				if isError(result) || isReturn(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}

		return NULL

	// ---------------- TUPLE LITERAL ----------------
	// Produced by `return a, b` — evaluates each element and wraps them in a Tuple.
	case *ast.TupleLiteral:
		elements := make([]Object, len(n.Elements))
		for i, el := range n.Elements {
			val := Eval(el, env)
			if isError(val) {
				return val
			}
			elements[i] = val
		}
		return &Tuple{Elements: elements}

	// ---------------- MULTI ASSIGN ----------------
	// Unpacks a Tuple into multiple variables: val, err = divide(10, 2)
	// The RHS must evaluate to a Tuple with exactly the right number of elements.
	case *ast.MultiAssignStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		tuple, ok := val.(*Tuple)
		if !ok {
			return runtimeError(
				fmt.Sprintf("cannot unpack %s into %d variables — right side must return multiple values", val.Type(), len(n.Names)),
				n.Pos,
			)
		}
		if len(tuple.Elements) != len(n.Names) {
			return runtimeError(
				fmt.Sprintf("cannot unpack %d values into %d variables", len(tuple.Elements), len(n.Names)),
				n.Pos,
			)
		}
		for i, name := range n.Names {
			if name != "_" {
				if errObj := env.CheckWritable(name); errObj != nil {
					return errObj
				}
				env.Assign(name, tuple.Elements[i])
			}
		}
		return tuple

	// ---------------- MULTI LET ----------------
	// Declares multiple variables from a Tuple RHS: let a, b = divide(10, 2)
	// Like LetStmt, every name is bound in the CURRENT scope via Set — never
	// walks the chain. Names already present in an outer scope are shadowed.
	case *ast.MultiLetStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		tuple, ok := val.(*Tuple)
		if !ok {
			return runtimeError(
				fmt.Sprintf("cannot unpack %s into %d variables — right side must return multiple values", val.Type(), len(n.Names)),
				n.Pos,
			)
		}
		if len(tuple.Elements) != len(n.Names) {
			return runtimeError(
				fmt.Sprintf("cannot unpack %d values into %d variables", len(tuple.Elements), len(n.Names)),
				n.Pos,
			)
		}
		for i, name := range n.Names {
			if name == "_" {
				continue
			}
			if env.consts != nil && env.consts[name] {
				return runtimeError("cannot reassign constant "+name, n.Pos)
			}
			env.Set(name, tuple.Elements[i])
		}
		return tuple

	// ---------------- ARRAY LITERAL ----------------
	// Evaluate each element expression and collect the results into an Array.
	case *ast.ArrayLiteral:
		elements := make([]Object, len(n.Elements))
		for i, el := range n.Elements {
			val := Eval(el, env)
			if isError(val) {
				return val
			}
			elements[i] = val
		}
		return &Array{Elements: elements}

	// ---------------- HASH LITERAL ----------------
	// Evaluate each key and value, convert the key to a HashKey, store the pair.
	case *ast.HashLiteral:
		pairs := make(map[HashKey]HashPair, len(n.Pairs))
		for _, p := range n.Pairs {
			key := Eval(p.Key, env)
			if isError(key) {
				return key
			}
			hk, err := toHashKey(key, n.Pos)
			if err != nil {
				return err
			}
			val := Eval(p.Value, env)
			if isError(val) {
				return val
			}
			pairs[hk] = HashPair{Key: key, Value: val}
		}
		return &Hash{Pairs: pairs}

	// ---------------- INDEX EXPRESSION ----------------
	// Handles both arr[i] and map["key"].
	// The runtime type of `left` determines which path is taken.
	case *ast.IndexExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(n.Index, env)
		if isError(index) {
			return index
		}
		switch l := left.(type) {
		case *Array:
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("array index must be integer, got %s", index.Type()), n.Pos)
			}
			if idx.Value < 0 || idx.Value >= len(l.Elements) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (array length %d)", idx.Value, len(l.Elements)), n.Pos)
			}
			return l.Elements[idx.Value]
		case *Hash:
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			// Locked read — see Hash's CONCURRENCY note. OFI #3.
			pair, ok := l.Get(hk)
			if !ok {
				// Missing key returns null, not an error — enables `m["k"] == null` checks.
				return NULL
			}
			return pair.Value
		case *ConcurrentHash:
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			val, ok := l.M.Load(hk)
			if !ok {
				return NULL
			}
			pair, ok := val.(HashPair)
			if !ok {
				return NULL
			}
			return pair.Value
		case *String:
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("string index must be integer, got %s", index.Type()), n.Pos)
			}
			r, inBounds := l.RuneAt(idx.Value)
			if !inBounds {
				return runtimeError(fmt.Sprintf("index %d out of bounds (string length %d)", idx.Value, l.RuneLen()), n.Pos)
			}
			return &String{Value: string(r)}
		case *Bytes:
			// Indexing returns an integer in [0, 255] — same as Python and Go.
			// This is deliberately different from string indexing (which returns
			// a single-character string) because bytes are fundamentally numeric.
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("bytes index must be integer, got %s", index.Type()), n.Pos)
			}
			if idx.Value < 0 || idx.Value >= len(l.Value) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (bytes length %d)", idx.Value, len(l.Value)), n.Pos)
			}
			return intObj(int(l.Value[idx.Value]))
		case *StructInstance:
			return typeError(fmt.Sprintf("cannot use bracket access on struct %s — use dot notation: struct.field", l.Def.Name), n.Pos)
		default:
			return typeError(fmt.Sprintf("index operator not supported for %s", left.Type()), n.Pos)
		}

	// ---------------- INDEX ASSIGNMENT ----------------
	// Handles arr[i] = val and map["key"] = val.
	// Both arrays and hashes are pointer types in Go, so mutation is visible
	// everywhere the same object is referenced.
	case *ast.IndexAssignStmt:
		obj := Eval(n.Left.Left, env) // the thing being indexed (array or hash)
		if isError(obj) {
			return obj
		}
		index := Eval(n.Left.Index, env)
		if isError(index) {
			return index
		}
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		switch o := obj.(type) {
		case *Hash:
			if o.frozen {
				return runtimeError("cannot mutate frozen hash", n.Pos)
			}
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			// Locked write — kLex async semantics share *Hash by
			// pointer across goroutines (OFI #3, see Hash struct doc).
			// Concurrent map writes panic without this guard.
			o.Set(hk, HashPair{Key: index, Value: val})
			return val
		case *ConcurrentHash:
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			// Swap returns whether a previous value existed; if not, increment count
			if _, loaded := o.M.Swap(hk, HashPair{Key: index, Value: val}); !loaded {
				atomic.AddInt64(&o.Cnt, 1)
			}
			return val
		case *Array:
			if o.frozen {
				return runtimeError("cannot mutate frozen array", n.Pos)
			}
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("array index must be integer, got %s", index.Type()), n.Pos)
			}
			if idx.Value < 0 || idx.Value >= len(o.Elements) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (array length %d)", idx.Value, len(o.Elements)), n.Pos)
			}
			o.Elements[idx.Value] = val
			return val
		default:
			return typeError(fmt.Sprintf("index assignment not supported for %s", obj.Type()), n.Pos)
		}

	// ---------------- IMPORT ----------------
	// Loads a kLex file, evaluates it in a fresh environment, and binds
	// the resulting module to the alias name in the current scope.
	//
	// Resolution order — first existing file wins:
	//   1. n.Path as-is               (CWD-relative — explicit relative imports)
	//   2. <script-dir>/n.Path        (next to the importing .lex file)
	//   3. $KLEX_PATH/n.Path          (user-configured override)
	//   4. <klex-binary-dir>/n.Path   (drop-in installs: klex.exe + stdlib/ together)
	//   5. <klex-binary-parent>/n.Path (bin/klex + share/klex/stdlib style installs)
	//
	// This makes `import "stdlib/strings.lex"` work out of the box whenever
	// stdlib sits next to the kLex binary, regardless of where the script
	// itself is located or what the current working directory is.
	case *ast.ImportStmt:
		resolvedPath, tried, ok := resolveImportPath(n.Path, env)

		// Embedded-stdlib fallback. When EmbeddedImportLookup is set
		// (typically the WASM build, where there is no filesystem),
		// consult it AFTER the disk search fails. The lookup key is
		// the verbatim n.Path so embedded `stdlib/json.lex` resolves
		// regardless of script-dir or CWD.
		var embeddedSrc string
		var fromEmbed bool
		if !ok && EmbeddedImportLookup != nil {
			if src, foundEmbedded := EmbeddedImportLookup(n.Path); foundEmbedded {
				embeddedSrc = src
				fromEmbed = true
				ok = true
			}
		}
		if !ok {
			return runtimeError(
				fmt.Sprintf("cannot import %q: not found. Searched:\n  %s",
					n.Path, strings.Join(tried, "\n  ")),
				n.Pos)
		}

		var absPath string
		if fromEmbed {
			// Synthetic absolute path so the module cache, in-flight
			// map, and ScriptDir machinery all treat the embedded
			// module distinctly from any disk file of the same name.
			absPath = "embedded://" + n.Path
		} else {
			ap, err := filepath.Abs(resolvedPath)
			if err != nil {
				ap = resolvedPath
			}
			absPath = ap
		}

		// Read-locked fast path. The vast majority of imports after the
		// first one for any given file land here.
		moduleCacheMu.RLock()
		cachedEnv, hit := moduleCache[absPath]
		moduleCacheMu.RUnlock()
		if hit {
			mod := &Module{Name: n.Alias, Env: cachedEnv}
			env.Assign(n.Alias, mod)
			return mod
		}

		var src []byte
		if fromEmbed {
			src = []byte(embeddedSrc)
		} else {
			b, readErr := os.ReadFile(resolvedPath)
			if readErr != nil {
				return runtimeError(fmt.Sprintf("cannot import %q: %s", n.Path, readErr.Error()), n.Pos)
			}
			src = b
		}

		// Reserve the in-flight slot under the write lock. Re-check the
		// cache here in case another goroutine finished the same import
		// between our RLock above and this point. The check + reserve
		// pair must be atomic — otherwise two goroutines could both pass
		// the importingFiles test and both proceed to Eval.
		moduleCacheMu.Lock()
		if cachedEnv, hit := moduleCache[absPath]; hit {
			moduleCacheMu.Unlock()
			mod := &Module{Name: n.Alias, Env: cachedEnv}
			env.Assign(n.Alias, mod)
			return mod
		}
		if importingFiles[absPath] {
			moduleCacheMu.Unlock()
			return runtimeError(fmt.Sprintf("import cycle detected: %q is already being imported", n.Path), n.Pos)
		}
		importingFiles[absPath] = true
		moduleCacheMu.Unlock()

		l := lexer.New(string(src))
		p := parser.New(l)
		program := p.ParseProgram()
		if len(program.Errors) > 0 {
			moduleCacheMu.Lock()
			delete(importingFiles, absPath)
			moduleCacheMu.Unlock()
			return runtimeError(fmt.Sprintf("parse error in %q: %s", n.Path, program.Errors[0]), n.Pos)
		}
		// M6 (audit follow-up, 2026-05-22): if the VM-compile hook
		// is installed (set by main.go / vmdiff for --vm mode),
		// try compiling the module to bytecode first. Live getters
		// on the returned env give external readers (DotExpr on
		// the *Module) the same live-value semantics the
		// tree-walker provides via shared modEnv.store. Falls back
		// to tree-walker Eval if vm.Compile errors (e.g.
		// InterpolatedString with embedded expressions); a genuine
		// runtime error in the module's top-level propagates as a
		// kLex *Error.
		var modEnv *Environment
		var result Object
		if VMCompileAndRunModule != nil {
			vmModEnv, vmErr := VMCompileAndRunModule(program, filepath.Dir(absPath))
			if vmErr != nil {
				moduleCacheMu.Lock()
				delete(importingFiles, absPath)
				moduleCacheMu.Unlock()
				return vmErr
			}
			if vmModEnv != nil {
				modEnv = vmModEnv
				// Defer the result-capture to mimic the
				// Eval-returns-NULL-or-last-value shape; modules
				// generally don't care about the program-level
				// return value, the cache only stores the env.
				result = NULL
			}
		}
		if modEnv == nil {
			modEnv = NewEnv()
			// Record where this module lives so its own imports resolve relative
			// to it — chained "import" calls in the module use its own dir, not
			// the importer's. The Eval call below runs WITHOUT the lock held,
			// so recursive imports can take the lock themselves without
			// deadlocking.
			modEnv.SetScriptDir(filepath.Dir(absPath))
			result = Eval(program, modEnv)
		}

		moduleCacheMu.Lock()
		delete(importingFiles, absPath)
		if isError(result) {
			moduleCacheMu.Unlock()
			return result
		}
		// Cache only on successful evaluation — a failed import (parse OK,
		// runtime error during top-level code) should be retryable.
		moduleCache[absPath] = modEnv
		moduleCacheMu.Unlock()

		mod := &Module{Name: n.Alias, Env: modEnv}
		env.Assign(n.Alias, mod)
		return mod

	// ---------------- DOT EXPRESSION ----------------
	// Looks up a property name in a module's environment.
	// math.add → finds "add" in math's env and returns it.
	// The returned value can be anything — a function, a number, a string.
	case *ast.DotExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		switch obj := left.(type) {
		case *Module:
			val, ok := obj.Env.store[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("module %q has no property %q", obj.Name, n.Property), n.Pos)
			}
			return val
		case *StructInstance:
			// Field access — locked read.
			if val, ok := obj.GetField(n.Property); ok {
				return val
			}
			// Method access — return the function itself (self injected at call time).
			if fn, ok := obj.Def.Methods[n.Property]; ok {
				return fn
			}
			return runtimeError(fmt.Sprintf("struct %s has no field or method %q", obj.Def.Name, n.Property), n.Pos)
		case *EnumDef:
			fields, ok := obj.Variants[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("enum %s has no variant %q", obj.Name, n.Property), n.Pos)
			}
			// Zero-field variants are instances — no call required.
			if len(fields) == 0 {
				return &EnumInstance{
					TypeName:    obj.Name,
					VariantName: n.Property,
					FieldNames:  nil,
					Fields:      map[string]Object{},
				}
			}
			// Data-carrying variants return a descriptor; calling it produces an instance.
			return &EnumVariant{TypeName: obj.Name, VariantName: n.Property, Fields: fields}
		case *EnumInstance:
			val, ok := obj.Fields[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("enum variant %s.%s has no field %q",
					obj.TypeName, obj.VariantName, n.Property), n.Pos)
			}
			return val
		case *Error:
			if !obj.IsUserError {
				return typeError(fmt.Sprintf("dot access not supported on %s", left.Type()), n.Pos)
			}
			switch n.Property {
			case "code":
				return &String{Value: obj.Code}
			case "message":
				return &String{Value: obj.Message}
			case "errorType":
				// Originating exception class name for bridge errors
				// (e.g. "ValueError", "FileNotFoundError"). Empty string
				// for kLex-native errors that don't cross a bridge.
				return &String{Value: obj.ErrorType}
			case "traceback":
				// Full Python traceback for bridge errors. Empty string
				// for non-bridge errors.
				return &String{Value: obj.Traceback}
			case "is":
				captured := obj
				return &Builtin{Fn: func(args []Object) Object {
					if len(args) != 1 {
						return runtimeError("is() expects 1 argument", n.Pos)
					}
					s, ok := args[0].(*String)
					if !ok {
						return typeError("is() argument must be a string", n.Pos)
					}
					return boolObj(captured.Code == s.Value)
				}}
			default:
				return runtimeError(fmt.Sprintf("error has no property %q", n.Property), n.Pos)
			}
		case *Null:
			return typeError(fmt.Sprintf("cannot access .%s on null — check for null before dot access", n.Property), n.Pos)
		case *Hash:
			return typeError(fmt.Sprintf("cannot use dot access on hash — use bracket notation: hash[\"%s\"]", n.Property), n.Pos)
		default:
			return typeError(fmt.Sprintf("dot access not supported on %s", left.Type()), n.Pos)
		}

	case *ast.DotAssignStmt:
		obj := Eval(n.Left.Left, env)
		if isError(obj) {
			return obj
		}
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		switch target := obj.(type) {
		case *StructInstance:
			if target.frozen {
				return runtimeError("cannot mutate frozen struct", n.Pos)
			}
			if _, ok := target.GetField(n.Left.Property); !ok {
				return runtimeError(fmt.Sprintf("struct %s has no field %q", target.Def.Name, n.Left.Property), n.Pos)
			}
			target.SetField(n.Left.Property, val)
		default:
			return typeError(fmt.Sprintf("dot assignment not supported on %s", obj.Type()), n.Pos)
		}
		return val

	// ---------------- LITERALS ----------------
	// Leaf nodes — they just produce their value directly.
	case *ast.NullLiteral:
		return NULL

	case *ast.BoolLiteral:
		return boolObj(n.Value)

	case *ast.IntLiteral:
		return intObj(n.Value)

	case *ast.FloatLiteral:
		return &Float{Value: n.Value}

	case *ast.StringLiteral:
		return &String{Value: n.Value}

	case *ast.BytesLiteral:
		// Copy the slice so future runtime mutations of one literal's bytes
		// can't leak into another evaluation of the same source-level literal.
		// (Today nothing mutates Bytes in place, but future builtins might.)
		buf := make([]byte, len(n.Value))
		copy(buf, n.Value)
		return &Bytes{Value: buf}

	case *ast.InterpolatedString:
		var buf strings.Builder
		for _, seg := range n.Segments {
			if !seg.IsExpr {
				buf.WriteString(seg.Text)
			} else {
				val := Eval(seg.Expr, env)
				if isError(val) {
					return val
				}
				buf.WriteString(val.Inspect())
			}
		}
		return &String{Value: buf.String()}

	// ---------------- CALL ----------------
	case *ast.CallExpr:
		return evalCall(n, env)

	// ---------------- UNWRAP (?) ----------------
	// Postfix error-propagation operator: expr?
	// The operand must evaluate to a 2-element tuple (value, err).
	// If err != null: return the error from the enclosing function immediately.
	// If err == null: evaluate to value (the tuple is unwrapped).
	case *ast.UnwrapExpr:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		tup, ok := val.(*Tuple)
		if !ok {
			return typeError(fmt.Sprintf("?: operand must be a (value, err) tuple, got %s", val.Type()), n.Pos)
		}
		if len(tup.Elements) != 2 {
			return typeError(fmt.Sprintf("?: expected a 2-element (value, err) tuple, got %d elements — e.g. `data = readFile(path)?` requires the function to `return value, err`", len(tup.Elements)), n.Pos)
		}
		if tup.Elements[1] != NULL {
			return &ReturnValue{Value: tup.Elements[1]}
		}
		return tup.Elements[0]

	// ---------------- PREFIX ----------------
	// Unary operators: ! (logical not).
	case *ast.PrefixExpr:
		val := Eval(n.Right, env)
		if isError(val) {
			return val
		}
		switch n.Operator {
		case "!":
			if !canLogical(val.Type()) {
				return typeMismatchError("!", val.Type(), val.Type(), n.Pos)
			}
			return boolObj(!val.(*Boolean).Value)
		case "-":
			if !canArithmetic(val.Type()) {
				return typeMismatchError("-", val.Type(), val.Type(), n.Pos)
			}
			if f, ok := val.(*Float); ok {
				return &Float{Value: -f.Value}
			}
			return intObj(-val.(*Integer).Value)
		}
		return runtimeError("unknown prefix operator: "+n.Operator, n.Pos)

	// ---------------- INFIX ----------------
	// Binary operators. && and || short-circuit and are handled first.
	// All other operators evaluate both sides eagerly.
	case *ast.InfixExpr:
		if n.Operator == "&&" || n.Operator == "||" {
			return evalLogical(n, env)
		}
		// ── Fast path: pure-integer arithmetic & comparison ─────────────
		// Walks integer-shaped expression trees without boxing any
		// intermediate *Integer. Boxes once (via intObj) at the
		// final result, or returns the TRUE/FALSE singleton for
		// comparisons. Falls back to the general path the moment a
		// non-integer operand appears, so semantics (string + string,
		// float promotion, type-mismatch errors) are unchanged.
		switch n.Operator {
		case "+", "-", "*", "/", "%":
			if v, ok, err := evalIntFast(n, env); err != nil {
				return err
			} else if ok {
				return intObj(v)
			}
		case "==", "!=", "<", ">", "<=", ">=":
			if res, ok := evalIntCompareFast(n, env); ok {
				return res
			}
		}
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(n.Right, env)
		if isError(right) {
			return right
		}

		switch n.Operator {
		case "+":
			if left.Type() == STRING_OBJ && right.Type() == STRING_OBJ {
				return &String{Value: left.(*String).Value + right.(*String).Value}
			}
			if left.Type() == BYTES_OBJ && right.Type() == BYTES_OBJ {
				lb := left.(*Bytes).Value
				rb := right.(*Bytes).Value
				out := make([]byte, len(lb)+len(rb))
				copy(out, lb)
				copy(out[len(lb):], rb)
				return &Bytes{Value: out}
			}
			if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
				return typeMismatchError("+", left.Type(), right.Type(), n.Pos)
			}
			if left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ {
				return intObj(left.(*Integer).Value + right.(*Integer).Value)
			}
			return &Float{Value: toFloat64(left) + toFloat64(right)}

		case "-", "*", "/", "%":
			// % is integer-only; the others promote to float when either operand is float.
			if n.Operator == "%" {
				if left.Type() != INTEGER_OBJ || right.Type() != INTEGER_OBJ {
					return typeMismatchError("%", left.Type(), right.Type(), n.Pos)
				}
				if right.(*Integer).Value == 0 {
					return runtimeError("modulo by zero — guard the right operand with `if y != 0` before using `%`", n.Pos)
				}
				return intObj(left.(*Integer).Value % right.(*Integer).Value)
			}
			if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
				return typeMismatchError(n.Operator, left.Type(), right.Type(), n.Pos)
			}
			// Fast path: both operands are integers — skip the two unconditional
			// toFloat64 conversions. The integer path is the common case in loop
			// counters, indices, and identifier arithmetic.
			if li, lok := left.(*Integer); lok {
				if ri, rok := right.(*Integer); rok {
					switch n.Operator {
					case "-":
						return intObj(li.Value - ri.Value)
					case "*":
						return intObj(li.Value * ri.Value)
					case "/":
						if ri.Value == 0 {
							return runtimeError("division by zero — guard the right operand with `if y != 0` before using `/`", n.Pos)
						}
						return intObj(li.Value / ri.Value)
					}
				}
			}
			// Slow path: at least one float — promote both.
			lf, rf := toFloat64(left), toFloat64(right)
			switch n.Operator {
			case "-":
				return &Float{Value: lf - rf}
			case "*":
				return &Float{Value: lf * rf}
			case "/":
				if rf == 0 {
					return runtimeError("division by zero — guard the right operand with `if y != 0` before using `/`", n.Pos)
				}
				return &Float{Value: lf / rf}
			}

		case "==":
			result := evalEquals(left, right, n.Pos)
			if isError(result) {
				return result
			}
			return result

		case "!=":
			result := evalEquals(left, right, n.Pos)
			if isError(result) {
				return result
			}
			// evalEquals now returns the TRUE/FALSE singleton, so identity
			// comparison is sufficient — no need to unwrap and re-wrap.
			if result == TRUE {
				return FALSE
			}
			return TRUE

		case "<", ">", "<=", ">=":
			return evalNumericCompare(left, right, n.Operator, n.Pos)

		}

		return runtimeError("internal error: unknown operator '"+n.Operator+"' in evaluator — this is a kLex bug, please report it with the source that triggered it", n.Pos)

	// ---------------- PIPE ----------------
	// left |> right — pipes left as the first argument of the right-hand callable.
	// If right is a CallExpr, left is prepended to its argument list.
	// If right is a bare reference, it is called with left as the only argument.
	case *ast.PipeExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}

		// Determine the callable and any extra arguments from the right side.
		var fnNode ast.Node
		var extraArgs []ast.Node
		if call, ok := n.Right.(*ast.CallExpr); ok {
			fnNode = call.Function
			extraArgs = call.Args
		} else {
			fnNode = n.Right
		}

		// Build args: piped value is always first, with pre-allocated capacity.
		pipeArgs := make([]Object, 1, 1+len(extraArgs))
		pipeArgs[0] = left

		for _, argNode := range extraArgs {
			val := Eval(argNode, env)
			if isError(val) {
				return val
			}
			pipeArgs = append(pipeArgs, val)
		}

		fnObj := Eval(fnNode, env)
		if isError(fnObj) {
			return fnObj
		}

		result, errObj := callCallable(fnObj, pipeArgs)
		if errObj != nil {
			return errObj
		}
		return result

	}

	return NULL
}
