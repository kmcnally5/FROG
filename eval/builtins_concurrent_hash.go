package eval

import (
	"fmt"
	"klex/ast"
	"math"
	"sync/atomic"
)

// builtins_concurrent_hash.go - Lock-free shared hash map for cross-goroutine state.
//
// Backed by Go's sync.Map (Go 1.20+) which provides:
//   - Lock-free atomic reads via Load
//   - Atomic writes via Store / Swap
//   - Compare-and-swap via CompareAndSwap (the basis for atomicHashIncr/Add)
//
// Compared to AtomicIntArray/AtomicFloatArray, ConcurrentHash trades a bit of
// per-op overhead (hashing, sync.Map indirection) for the ability to use
// arbitrary string/int/bool keys discovered at runtime. Use it when:
//   - Key set is dynamic (e.g. counting unknown event types)
//   - You want O(1) lookup by structured key
//   - Multiple goroutines need to share state without channel coordination
//
// Each entry is stored as HashPair{Key: <kLex object>, Value: <kLex object>},
// so keys(ch) can return original kLex values rather than reconstructed ones.

func init() {
	// concurrentHash() -> empty ConcurrentHash
	Builtins["concurrentHash"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("concurrentHash expects 0 arguments", ast.Pos{})
		}
		return &ConcurrentHash{}
	}}

	// atomicHashIncr(ch, key, delta) -> new integer value
	// Atomically increments the integer at key by delta. If the key doesn't
	// exist, treats current value as 0. Uses sync.Map CAS-loop internally;
	// safe under concurrent access from any number of goroutines.
	Builtins["atomicHashIncr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("atomicHashIncr expects 3 arguments (ch, key, delta)", ast.Pos{})
		}
		ch, ok := args[0].(*ConcurrentHash)
		if !ok {
			return typeError(fmt.Sprintf("atomicHashIncr: first argument must be ConcurrentHash, got %s", args[0].Type()), ast.Pos{})
		}
		deltaObj, ok := args[2].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicHashIncr: delta must be integer, got %s", args[2].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		// CAS-loop. On insert, increment Cnt. On replace, just CAS the pair.
		for {
			cur, loaded := ch.M.Load(hk)
			if !loaded {
				newPair := HashPair{Key: args[1], Value: &Integer{Value: deltaObj.Value}}
				if _, exists := ch.M.LoadOrStore(hk, newPair); !exists {
					atomic.AddInt64(&ch.Cnt, 1)
					return newPair.Value
				}
				continue // someone else inserted; retry as replace
			}
			pair, _ := cur.(HashPair)
			oldInt, isInt := pair.Value.(*Integer)
			if !isInt {
				return typeError(fmt.Sprintf("atomicHashIncr: existing value at key is %s, not integer", pair.Value.Type()), ast.Pos{})
			}
			newPair := HashPair{Key: pair.Key, Value: &Integer{Value: oldInt.Value + deltaObj.Value}}
			if ch.M.CompareAndSwap(hk, pair, newPair) {
				return newPair.Value
			}
			// CAS failed - another goroutine swapped first; retry
		}
	}}

	// atomicHashAdd(ch, key, delta) -> new float value
	// Same as atomicHashIncr but for floats. Stores Float values; if key
	// doesn't exist, treats current as 0.0.
	Builtins["atomicHashAdd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("atomicHashAdd expects 3 arguments (ch, key, delta)", ast.Pos{})
		}
		ch, ok := args[0].(*ConcurrentHash)
		if !ok {
			return typeError(fmt.Sprintf("atomicHashAdd: first argument must be ConcurrentHash, got %s", args[0].Type()), ast.Pos{})
		}
		var delta float64
		switch v := args[2].(type) {
		case *Float:
			delta = v.Value
		case *Integer:
			delta = float64(v.Value)
		default:
			return typeError(fmt.Sprintf("atomicHashAdd: delta must be number, got %s", args[2].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		for {
			cur, loaded := ch.M.Load(hk)
			if !loaded {
				newPair := HashPair{Key: args[1], Value: &Float{Value: delta}}
				if _, exists := ch.M.LoadOrStore(hk, newPair); !exists {
					atomic.AddInt64(&ch.Cnt, 1)
					return newPair.Value
				}
				continue
			}
			pair, _ := cur.(HashPair)
			var oldVal float64
			switch v := pair.Value.(type) {
			case *Float:
				oldVal = v.Value
			case *Integer:
				oldVal = float64(v.Value)
			default:
				return typeError(fmt.Sprintf("atomicHashAdd: existing value at key is %s, not number", pair.Value.Type()), ast.Pos{})
			}
			newF := oldVal + delta
			// Use Float64bits comparison to avoid NaN-equality weirdness in CAS.
			_ = math.Float64bits(newF) // ensure we use math (silences unused import in some configs)
			newPair := HashPair{Key: pair.Key, Value: &Float{Value: newF}}
			if ch.M.CompareAndSwap(hk, pair, newPair) {
				return newPair.Value
			}
		}
	}}

	// atomicHashCAS(ch, key, old, new) -> bool
	// Compare-and-swap the value at key. Returns true if swap succeeded
	// (current value was equal to old AND key existed), false otherwise.
	// Equality is by structural value comparison via objectsEqual.
	Builtins["atomicHashCAS"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("atomicHashCAS expects 4 arguments (ch, key, old, new)", ast.Pos{})
		}
		ch, ok := args[0].(*ConcurrentHash)
		if !ok {
			return typeError(fmt.Sprintf("atomicHashCAS: first argument must be ConcurrentHash, got %s", args[0].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		// Need a CAS-loop because sync.Map.CompareAndSwap compares by Go ==,
		// which compares HashPair fields including the Object pointer. Two
		// different *Integer pointers with the same value are NOT == in Go.
		// So load, compare values structurally, then CAS the actual stored pair.
		for {
			cur, loaded := ch.M.Load(hk)
			if !loaded {
				return FALSE
			}
			pair, _ := cur.(HashPair)
			if !valuesEqual(pair.Value, args[2]) {
				return FALSE
			}
			newPair := HashPair{Key: pair.Key, Value: args[3]}
			if ch.M.CompareAndSwap(hk, pair, newPair) {
				return TRUE
			}
			// Retry - another goroutine modified the entry
		}
	}}

	// quiesceLen(ch) -> integer
	//
	// Exact entry count for a ConcurrentHash. Unlike len(ch), which reads the
	// atomic Cnt counter and can briefly diverge from reality by the number of
	// in-flight Store/Delete operations, quiesceLen walks the entire sync.Map
	// via Range and counts live entries. O(n) in the size of the map.
	//
	// Use after a known quiescent point (all writer tasks awaited) when you
	// need a guaranteed-accurate size — e.g. for invariants, assertions, or
	// "did the pipeline emit exactly N items" checks. Prefer len(ch) for
	// progress indicators and other use-cases where ~1 off is fine.
	//
	// Concurrent writes during the Range are not blocked; an entry inserted
	// or deleted mid-walk may or may not be counted (sync.Map.Range semantics).
	// For a fully consistent snapshot, ensure no writers are active.
	Builtins["quiesceLen"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("quiesceLen expects 1 argument (ConcurrentHash)", ast.Pos{})
		}
		ch, ok := args[0].(*ConcurrentHash)
		if !ok {
			return typeError(fmt.Sprintf("quiesceLen: argument must be ConcurrentHash, got %s", args[0].Type()), ast.Pos{})
		}
		n := 0
		ch.M.Range(func(_, _ any) bool {
			n++
			return true
		})
		return intObj(n)
	}}
}

// valuesEqual compares two kLex values for atomicHashCAS. Delegates to
// primitiveEqual (eval.go) so CAS semantics stay locked to the `==`
// operator — a future change to primitive equality (e.g. a new primitive
// type, a numeric coercion rule) lands in one place and both sites pick
// it up. Reference types fall back to pointer identity, matching
// evalEquals's own reference-type rule.
func valuesEqual(a, b Object) bool {
	if a == b {
		return true // same pointer or both nil
	}
	if handled, equal := primitiveEqual(a, b); handled {
		return equal
	}
	return false
}
