package vm

// external_callable.go — cross-interpreter calling hook.
//
// eval-side code (`safe`, `map`, `filter`, `reduce`, `async`, plus
// anywhere callCallable runs) switches on *eval.Function /
// *eval.Builtin. A *vm.CompiledFunction passed into that code
// would fail to dispatch — eval can't import vm (cycle), so it
// can't case-match the VM's callable type directly.
//
// The fix is the eval.ExternalCallable hook: a function-valued
// package variable in eval that vm sets at init() time. eval's
// dispatchers consult the hook AFTER their built-in cases miss,
// giving vm a chance to recognise and run its own callables.
//
// What this enables in practice
//
//   - `safe(fn() { ... })` where fn is a VM closure — now runs.
//   - `map(arr, double)` from inside the VM where `double` is a VM
//     closure passed to the (eval-side) `map` builtin — now runs.
//   - `async(fn)` of a VM closure — now spawns a goroutine that
//     re-enters the VM via the same hook.

import (
	"klex/eval"
)

// init wires eval.ExternalCallable to dispatch *CompiledFunction
// callables through the VM's execute() loop. Set once at package
// init; subsequent assignments are not supported (the hook is
// effectively global per-process). Matches the standard pattern
// for cross-package wiring without import cycles.
func init() {
	// M2 (audit fix, 2026-05-22): the IsExternalCallable predicate
	// hook is gone — eval.IsCallable now checks
	// `fn.Type() == eval.COMPILED_FUNCTION_OBJ` directly. The hook
	// variable is still declared for backwards compatibility but
	// nothing in eval reads it.

	// M5 lazy mutex (audit follow-up 2026-05-22): when async() is
	// dispatched on a VM closure, the closure's upvalue Values are
	// reachable from the new goroutine. Mark every reachable Hash
	// as shared FROM THE SPAWNER GOROUTINE (before `go func()`) so
	// the marking itself doesn't race with sibling async-spawners
	// walking the same hash.
	eval.MarkExternalUpvaluesShared = func(fn eval.Object) {
		cf, ok := fn.(*CompiledFunction)
		if !ok {
			return
		}
		for _, u := range cf.Upvalues {
			if u == nil {
				continue
			}
			eval.MarkSharedRecursive(u.Value)
		}
	}

	// Async-specialised hook. Same dispatch shape as
	// ExternalCallable, but snapshots primitive upvalue values
	// before invoking so the goroutine doesn't race with the caller
	// on shared cells. Reference-type upvalues (Array, Hash, Struct,
	// Channel, etc.) stay shared — kLex's documented async-snapshot
	// semantic ("mutations to contents ARE shared, REBIND is task-
	// local"). Primitives (Integer, Float, String, Bool, Null) get
	// cloned into fresh cells so an outer-scope `i = …` inside the
	// async body can't blow up parallel siblings reading/writing the
	// same cell. See hashConcurrentTest section 4 for the failing
	// pattern.
	eval.ExternalCallableAsync = func(fn eval.Object, args []eval.Object) (eval.Object, bool) {
		cf, ok := fn.(*CompiledFunction)
		if !ok {
			return nil, false
		}
		if msg := validateArity(cf, len(args)); msg != "" {
			return makeRuntimeError(msg), true
		}
		// Snapshot upvalues: clone the cell, copy IF the value is a
		// primitive. Reference types (Object pointers) stay shared
		// by pointing the new cell at the same Object.
		// NOTE: MarkSharedRecursive on the upvalue Values is the
		// spawner's responsibility (via eval.MarkExternalUpvaluesShared
		// installed in init below) — doing it here would race with
		// sibling async-spawners' MarkShared walks.
		snapUpvalues := make([]*UpvalueCell, len(cf.Upvalues))
		for i, u := range cf.Upvalues {
			if u == nil {
				snapUpvalues[i] = newCell()
				continue
			}
			fresh := &UpvalueCell{Value: u.Value, IsConst: u.IsConst, ConstName: u.ConstName}
			snapUpvalues[i] = fresh
		}
		calleeLocals := acquireLocals(cf.Chunk.NumLocals)
		bindArgs(cf, calleeLocals, args)
		if cf.SelfSlot >= 0 && cf.SelfSlot < len(calleeLocals) {
			calleeLocals[cf.SelfSlot].Value = cf
		}
		ret, err := execute(cf.Chunk, calleeLocals, snapUpvalues)
		releaseLocals(calleeLocals)
		if err != nil {
			return makeRuntimeError(err.Error()), true
		}
		if ret == nil {
			ret = eval.NULL
		}
		return ret, true
	}

	eval.ExternalCallable = func(fn eval.Object, args []eval.Object) (eval.Object, bool) {
		cf, ok := fn.(*CompiledFunction)
		if !ok {
			return nil, false
		}
		if msg := validateArity(cf, len(args)); msg != "" {
			// Don't error-out — return the kLex-shape error so the
			// caller wraps it into the (null, err) tuple. Caller
			// converts via eval.IsError.
			return makeRuntimeError(msg), true
		}
		calleeLocals := acquireLocals(cf.Chunk.NumLocals)
		bindArgs(cf, calleeLocals, args)
		if cf.SelfSlot >= 0 && cf.SelfSlot < len(calleeLocals) {
			calleeLocals[cf.SelfSlot].Value = cf
		}
		ret, err := execute(cf.Chunk, calleeLocals, cf.Upvalues)
		releaseLocals(calleeLocals)
		if err != nil {
			return makeRuntimeError(err.Error()), true
		}
		if ret == nil {
			ret = eval.NULL
		}
		return ret, true
	}
}

// makeRuntimeError builds an Error Object the eval-side dispatchers
// understand. We don't have access to eval's unexported runtimeError
// helper, so construct the same shape directly.
func makeRuntimeError(msg string) eval.Object {
	return &eval.Error{
		Kind:        eval.RuntimeErr,
		Message:     msg,
		IsUserError: false,
	}
}
