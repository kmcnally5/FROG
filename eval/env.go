package eval

import "sync"

// env.go implements kLex's variable scoping system.
//
// An Environment is a simple map from variable names to Objects.
// The key insight is the `outer` pointer: each Environment has a reference
// to the environment that enclosed it when it was created. This chain of
// environments is what gives kLex LEXICAL SCOPING.
//
// How it works in practice:
//
//   x = 10                  ← stored in the global env
//   fn double(n) {
//       return n * 2        ← n is in the function's local env
//   }
//   double(x)
//
// When the body of `double` runs, it gets a fresh env for `n`.
// That env's outer pointer points to the env where `double` was defined
// (the global env), so `x` is still accessible inside `double`.
//
// This also means inner scopes can shadow outer ones:
//   x = 1
//   fn f() {
//       x = 99   ← this x only exists inside f, does not overwrite the outer x
//   }

// Environment is the variable store with lexical scoping via outer chain.
//
// Concurrency model:
//
//	shared = true  → this env is accessible by multiple goroutines (only the
//	                 global env, created by NewEnv). All reads and writes are
//	                 guarded by mu.
//	shared = false → this env is goroutine-local (function frames, loop envs,
//	                 closure call envs). No locking needed; mu is never touched.
//
// When a goroutine-local env walks up to a shared outer env, the shared env
// locks itself — so correctness is preserved without paying mutex overhead on
// the 99% of envs that are never shared.
type Environment struct {
	mu     sync.RWMutex
	shared bool              // true only for the global env
	store  map[string]Object // variables defined in this scope
	consts map[string]bool   // names that cannot be reassigned (nil = none)
	outer  *Environment      // the enclosing scope, or nil for the global env

	// scriptDir is the directory of the source file whose top-level code is
	// being evaluated in this env. Set on the entry global env by main.go,
	// and on each module env when import resolves a file. Inner scopes
	// (function frames, blocks) leave it empty and inherit from outer via
	// ScriptDir(). Empty string means "no script context" (e.g. REPL).
	scriptDir string

	// liveBindings is a name → live-getter map populated by the VM's
	// module loader (M6, audit follow-up 2026-05-22). When an
	// imported module is compiled to bytecode, its top-level
	// bindings live as *UpvalueCell values that the module's
	// internal functions mutate through closures. To match
	// tree-walker semantics where external readers of a module's
	// mutable top-level state see the current value, the VM
	// installs a getter per binding that reads the cell's Value
	// live. Get() checks this BEFORE store so reads from outside
	// (DotExpr on a *Module) reflect ongoing mutations.
	//
	// nil for tree-walker-evaluated environments. The nil-check is
	// a single pointer load on every Get — negligible cost.
	liveBindings map[string]func() Object
}

// SetLiveBinding registers a live-value getter for `name`. Used by
// vm.CompileAndRunModule to expose module top-level cells without
// snapshotting their Value. Subsequent Get(name) calls invoke the
// getter and return the cell's current value.
//
// liveBindings shadows store: if both exist for the same name, the
// getter wins. Regular env.Set on the same name clears the getter
// (the bind reverts to a stored value).
func (e *Environment) SetLiveBinding(name string, getter func() Object) {
	if e.liveBindings == nil {
		e.liveBindings = make(map[string]func() Object)
	}
	e.liveBindings[name] = getter
}

// ScriptDir returns the directory of the script that introduced this scope
// chain — the global script for top-level code, or the imported module's
// file for code inside that module. Walks the outer chain so function calls
// invoked from another file still report the directory of the file that
// defined them. Returns "" when no script context is set (REPL, eval of a
// string, etc.).
func (e *Environment) ScriptDir() string {
	for env := e; env != nil; env = env.outer {
		if env.scriptDir != "" {
			return env.scriptDir
		}
	}
	return ""
}

// SetScriptDir records the directory of the source file evaluated in this
// env. Called by main.go for the entry script and by the import handler for
// each imported module.
func (e *Environment) SetScriptDir(dir string) {
	e.scriptDir = dir
}

// NewEnv creates the top-level (global) environment. It is the only env
// marked shared=true because it is the only one read by multiple goroutines
// concurrently after async tasks are launched.
func NewEnv() *Environment {
	return &Environment{
		shared: true,
		store:  make(map[string]Object),
	}
}

// SetConst stores value under name and marks it as immutable in this scope.
// Any subsequent attempt to assign to this name (from any scope that can see it)
// will produce a RuntimeError.
func (e *Environment) SetConst(name string, value Object) Object {
	if e.shared {
		e.mu.Lock()
	}
	if e.consts == nil {
		e.consts = make(map[string]bool)
	}
	e.store[name] = value
	e.consts[name] = true
	if e.shared {
		e.mu.Unlock()
	}
	return value
}

// CheckWritable returns a RuntimeError if name resolves to a const binding in
// this scope chain, or nil if the assignment is permitted.
// Mirrors Assign's lookup logic: checks current scope first, then walks outer.
// Iterative walk — each scope locks/unlocks independently to keep the shared
// global env's read window brief and avoid Go-stack growth on deep nests.
func (e *Environment) CheckWritable(name string) *Error {
	for env := e; env != nil; env = env.outer {
		if env.shared {
			env.mu.RLock()
		}
		isConst := env.consts != nil && env.consts[name]
		_, inStore := env.store[name]
		if env.shared {
			env.mu.RUnlock()
		}
		if isConst {
			return &Error{Kind: RuntimeErr, Message: "cannot reassign constant " + name}
		}
		if inStore {
			return nil // found here and not const — writable
		}
	}
	return nil
}

// Get looks up a variable name. It searches:
//  1. This scope's own store
//  2. The outer (enclosing) scopes in order
//  3. The built-in functions (println, len, push, etc.)
//
// If nothing is found, it returns (nil, false) and the evaluator will
// produce an "undefined variable" RuntimeError.
//
// Iterative walk so deep scope chains (closures inside modules inside
// loops, etc.) don't stack a Go frame per level. Only the global env can
// be shared, so the RLock cost is bounded.
func (e *Environment) Get(name string) (Object, bool) {
	for env := e; env != nil; env = env.outer {
		// M6: live-bindings (VM-compiled module top-level cells)
		// shadow regular store entries. The nil-check on the map
		// is a single pointer load; for tree-walker envs this is
		// always nil-fast.
		if env.liveBindings != nil {
			if getter, ok := env.liveBindings[name]; ok {
				return getter(), true
			}
		}
		if env.shared {
			env.mu.RLock()
		}
		val, ok := env.store[name]
		if env.shared {
			env.mu.RUnlock()
		}
		if ok {
			return val, true
		}
	}

	// Fall back to builtins only after exhausting the scope chain — a
	// user-defined function in any enclosing scope can shadow a builtin
	// of the same name for closures defined within it.
	if builtin, ok := Builtins[name]; ok {
		return builtin, true
	}
	return nil, false
}

// Set stores a value in THIS scope's store only.
// Used when we know a variable belongs to the current scope (e.g. function parameters,
// loop variables in for-in). Do not use for general assignment — use Assign instead.
func (e *Environment) Set(name string, value Object) Object {
	if e.shared {
		e.mu.Lock()
		e.store[name] = value
		e.mu.Unlock()
	} else {
		e.store[name] = value
	}
	return value
}

// Assign implements the semantics of kLex assignment statements.
// It walks the scope chain to find where the variable already lives and
// updates it there. If the variable doesn't exist anywhere in the chain,
// it is created in the current (innermost) scope.
//
// This is what makes closures work correctly:
//
//	fn makeCounter() {
//	    count = 0
//	    fn next() { count = count + 1 }  ← updates makeCounter's count, not a new local
//	}
//
// The tradeoff: a function CAN modify a variable in an outer scope. There is
// no `local` keyword to prevent this. Assign outer-scope variables intentionally.
func (e *Environment) Assign(name string, value Object) Object {
	if e.shared {
		e.mu.Lock()
		if _, ok := e.store[name]; ok {
			e.store[name] = value
			e.mu.Unlock()
			return value
		}
		e.mu.Unlock()
	} else {
		if _, ok := e.store[name]; ok {
			e.store[name] = value
			return value
		}
	}

	if e.outer != nil {
		// Let the parent's own Lock handle the safety
		if _, updated := e.outer.tryAssign(name, value); updated {
			return value
		}
	}

	// Variable not found anywhere — create it in the current scope.
	if e.shared {
		e.mu.Lock()
		e.store[name] = value
		e.mu.Unlock()
	} else {
		e.store[name] = value
	}
	return value
}

// tryAssign attempts to update a variable only if it already exists somewhere
// in the chain starting from this env. Returns (value, true) if updated,
// (nil, false) if the name doesn't exist in this env or any outer scope.
//
// Why this exists separately from Assign(): Assign needs to walk the scope
// chain looking for an existing binding, but holding a single lock across
// the whole walk would serialise every concurrent assignment against the
// global env, even when the assigning goroutines are touching completely
// disjoint local scopes that just happen to share the same global outer.
// tryAssign locks-then-unlocks each level independently, so the read on the
// (typically only) shared env is brief.
//
// Correctness under concurrent writers: if two goroutines both call
// Assign("x", ...) and "x" doesn't initially exist, both will fall through
// to "create in current scope" and both writes happen in their own
// goroutine-local envs — there is no race. If "x" already exists in the
// shared global env, the race is "last writer wins" which is the standard
// concurrent-map semantics kLex programs are expected to handle via
// concurrentHash() or async barriers, not via the global env.
func (e *Environment) tryAssign(name string, value Object) (Object, bool) {
	// Iterative walk — each scope locks/unlocks independently so we never
	// hold two locks at once (deadlock prevention) and don't stack a Go
	// frame per scope level.
	for env := e; env != nil; env = env.outer {
		if env.shared {
			env.mu.Lock()
		}
		if _, ok := env.store[name]; ok {
			env.store[name] = value
			if env.shared {
				env.mu.Unlock()
			}
			return value, true
		}
		if env.shared {
			env.mu.Unlock()
		}
	}
	return nil, false
}

// Snapshot creates a task-local copy of the scope chain for async tasks.
// The returned environment has the same data as the parent but is not shared:
// it has no outer scope and is never locked.
//
// SHARING SEMANTICS — read carefully before touching code that relies on this:
//
//   - PRIMITIVES (Integer, Float, String, Boolean, Null) are value-like in
//     kLex's surface language. Reassigning a name in an async task affects
//     ONLY the task's snapshot; the caller never sees it.
//
//         x = 0
//         async(fn() { x = 99 })   // task sees its own x; caller still has 0
//
//   - REFERENCE TYPES (*Array, *Hash, *StructInstance, etc.) are shared by
//     pointer. The snapshot's binding points to the same underlying object.
//     Mutating contents IS visible across goroutines and is a data race.
//
//         arr = [1, 2, 3]
//         async(fn() { arr[0] = 99 })   // ALSO MODIFIES the caller's arr!
//
//     If goroutines need shared mutable state, use concurrentHash(),
//     atomicIntArray(), or atomicFloatArray() — those have explicit
//     synchronisation. Never share a plain Array/Hash across an async()
//     boundary as a mutation target.
//
// Implementation: one pass through the scope chain, one read-lock window
// per env. Maps grow dynamically — pre-sizing was previously done but
// required two passes (and twice the lock acquisitions on the shared
// global env), which cost more than the saved rehash work.
func (e *Environment) Snapshot() *Environment {
	snap := &Environment{
		store:  make(map[string]Object),
		consts: make(map[string]bool),
		outer:  nil,
		shared: false,
	}

	// Walk the chain inner → outer. Inner scopes win because they're
	// visited first; an existing entry in snap.store is never overwritten.
	for env := e; env != nil; env = env.outer {
		if env.shared {
			env.mu.RLock()
		}
		for k, v := range env.store {
			if _, exists := snap.store[k]; !exists {
				snap.store[k] = v
			}
		}
		for k, v := range env.consts {
			if _, exists := snap.consts[k]; !exists {
				snap.consts[k] = v
			}
		}
		if env.shared {
			env.mu.RUnlock()
		}
	}

	// M5 lazy mutex: Snapshot is called BEFORE a goroutine starts
	// (async / parallel / select tasks). Every reference-type value
	// in the snapshot is now reachable from both the original
	// goroutine (which still has the parent env) AND the new
	// goroutine (which gets `snap`). Mark every reachable Hash as
	// shared so subsequent reads/writes from either side acquire
	// the mutex. Primitives + non-hash references fall through
	// cheaply.
	for _, v := range snap.store {
		MarkSharedRecursive(v)
	}

	return snap
}
