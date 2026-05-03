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
func (e *Environment) CheckWritable(name string) *Error {
	if e.shared {
		e.mu.RLock()
	}
	isConst := e.consts != nil && e.consts[name]
	_, inStore := e.store[name]
	if e.shared {
		e.mu.RUnlock()
	}

	if isConst {
		return &Error{Kind: RuntimeErr, Message: "cannot reassign constant " + name}
	}
	if inStore {
		return nil // found here and not const — writable
	}
	if e.outer != nil {
		return e.outer.CheckWritable(name)
	}
	return nil
}

// Get looks up a variable name. It searches:
//  1. This scope's own store
//  2. The outer (enclosing) scope, recursively
//  3. The built-in functions (println, len, push, etc.)
//
// If nothing is found, it returns (nil, false) and the evaluator will
// produce an "undefined variable" RuntimeError.
func (e *Environment) Get(name string) (Object, bool) {
	if e.shared {
		e.mu.RLock()
	}
	val, ok := e.store[name]
	if e.shared {
		e.mu.RUnlock()
	}
	if ok {
		return val, true
	}

	// Walk the full scope chain before falling back to builtins.
	// This means a user-defined function in any enclosing scope (e.g. a module)
	// can shadow a builtin of the same name for closures defined within it.
	if e.outer != nil {
		return e.outer.Get(name)
	}

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

// tryAssign attempts to update a variable only if it already exists in the chain.
// Returns (value, true) if updated, (nil, false) if not found.
// Written as a last ditch effort to avoid locking in the pond development stuff... Must
// decide if this is going to stay or not... To be continued...
func (e *Environment) tryAssign(name string, value Object) (Object, bool) {
	if e.shared {
		e.mu.Lock()
	}

	// Check if it exists in THIS scope
	if _, ok := e.store[name]; ok {
		e.store[name] = value
		if e.shared {
			e.mu.Unlock()
		}
		return value, true
	}

	// Unlock before recursing to avoid holding multiple locks (deadlock prevention)
	if e.shared {
		e.mu.Unlock()
	}

	// Recurse to parent
	if e.outer != nil {
		return e.outer.tryAssign(name, value)
	}

	return nil, false
}

// Snapshot creates a task-local copy of the global environment for async tasks.
// The returned environment has the same data as the parent but is not shared:
// it has no outer scope and is never locked. Mutations inside an async task
// are isolated and invisible to other tasks and the caller.
// This eliminates mutex contention while preventing shared mutable state bugs.
func (e *Environment) Snapshot() *Environment {
	// First pass: count total variables and constants to pre-allocate maps.
	var totalVars, totalConsts int
	env := e
	for env != nil {
		if env.shared {
			env.mu.RLock()
		}
		totalVars += len(env.store)
		totalConsts += len(env.consts)
		if env.shared {
			env.mu.RUnlock()
		}
		env = env.outer
	}

	snap := &Environment{
		store:  make(map[string]Object, totalVars),
		consts: make(map[string]bool, totalConsts),
		outer:  nil,
		shared: false,
	}

	// Copy all variables and constants from the current scope chain into the snapshot.
	// This includes all accessible variables (globals and parent scopes).
	env = e
	for env != nil {
		if env.shared {
			env.mu.RLock()
		}

		// Copy all variables in this scope level.
		for k, v := range env.store {
			// Only add if not already in snapshot (inner scopes override outer).
			if _, exists := snap.store[k]; !exists {
				snap.store[k] = v
			}
		}

		// Copy all const marks.
		for k, v := range env.consts {
			if _, exists := snap.consts[k]; !exists {
				snap.consts[k] = v
			}
		}

		if env.shared {
			env.mu.RUnlock()
		}

		env = env.outer
	}

	return snap
}
