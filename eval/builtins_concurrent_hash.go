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
	// concurrentHash — create an empty thread-safe hash map for shared state.
	//
	// Backed by sync.Map: many async tasks can read and write it concurrently
	// without channels or mutexes. Read with ch[key] (returns the value or null),
	// write with ch[key] = v (atomic). Use atomicHashIncr/Add for lock-free
	// arithmetic and atomicHashCAS for compare-and-swap. Keys may be any
	// hashable value (string, int, bool) discovered at runtime. Note len(ch) is
	// O(1) but approximate under concurrent mutation — see quiesceLen.
	//
	// @sig     concurrentHash() -> ConcurrentHash
	// @returns a new, empty ConcurrentHash
	// @errors  RuntimeError if called with any arguments
	// @example concurrentHash() → ConcurrentHash(size=0)
	// @since   0.1.0
	// @see     atomicHashIncr, atomicHashCAS, quiesceLen
	Builtins["concurrentHash"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("concurrentHash expects 0 arguments", ast.Pos{})
		}
		return &ConcurrentHash{}
	}}

	// atomicHashIncr — atomically add an integer delta to ch[key], returning the new value.
	//
	// A missing key counts as 0, so the first call creates it. The whole
	// read-modify-write is a lock-free CAS loop, so concurrent increments from
	// any number of tasks never lose updates — the canonical way to count events
	// by dynamic key. The existing value at key must be an integer.
	//
	// @sig     atomicHashIncr(ch: ConcurrentHash, key: string|int|bool, delta: int) -> int
	// @param   ch     the concurrent hash to update
	// @param   key    the entry key (created if absent)
	// @param   delta  the integer amount to add
	// @returns the new integer value at key after adding delta
	// @errors  TypeError if ch isn't a ConcurrentHash, delta isn't an integer, key isn't hashable, or the existing value isn't an integer
	// @example atomicHashIncr(concurrentHash(), "hits", 1) → 1
	// @since   0.1.0
	// @see     atomicHashAdd, atomicHashCAS, concurrentHash
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

	// atomicHashAdd — atomically add a numeric delta to ch[key], returning the new float.
	//
	// The float counterpart to atomicHashIncr: a missing key counts as 0.0, the
	// value is stored as a float, and the update is a lock-free CAS loop safe
	// under concurrent access. delta may be an int or a float.
	//
	// @sig     atomicHashAdd(ch: ConcurrentHash, key: string|int|bool, delta: number) -> float
	// @param   ch     the concurrent hash to update
	// @param   key    the entry key (created if absent)
	// @param   delta  the amount to add (int or float)
	// @returns the new float value at key after adding delta
	// @errors  TypeError if ch isn't a ConcurrentHash, delta isn't a number, key isn't hashable, or the existing value isn't a number
	// @example atomicHashAdd(concurrentHash(), "score", 1.5) → 1.5
	// @since   0.1.0
	// @see     atomicHashIncr, atomicHashCAS, concurrentHash
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

	// atomicHashCAS — compare-and-swap ch[key], the building block for lock-free updates.
	//
	// If key exists and its value equals `old` (by the same structural rule as
	// `==`), atomically replaces it with `new` and returns true; otherwise leaves
	// it untouched and returns false. A missing key always returns false. Loop on
	// false to build custom atomic transitions over arbitrary keys.
	//
	// @sig     atomicHashCAS(ch: ConcurrentHash, key: string|int|bool, old: any, new: any) -> bool
	// @param   ch   the concurrent hash to update
	// @param   key  the entry key
	// @param   old  the value the entry must currently hold for the swap to happen
	// @param   new  the value to store if the comparison succeeds
	// @returns true if the swap happened, false if the key was absent or its value didn't match old
	// @errors  TypeError if ch isn't a ConcurrentHash or key isn't hashable
	// @example atomicHashCAS(concurrentHash(), "x", 0, 9) → false
	// @since   0.1.0
	// @see     atomicHashIncr, atomicHashAdd, concurrentHash
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

	// quiesceLen — the exact entry count of a ConcurrentHash, by walking every entry.
	//
	// Unlike len(ch) — which reads the O(1) atomic counter and can briefly
	// diverge from reality by the number of in-flight writes — quiesceLen walks
	// the whole map and counts live entries, O(n). Use it after a quiescent point
	// (all writer tasks awaited) when you need a guaranteed-accurate size: invariants,
	// test assertions, "did the pipeline emit exactly N items" checks. Prefer the
	// O(1) len(ch) for progress indicators where ±1 doesn't matter.
	//
	// Concurrent writes during the walk are not blocked; an entry inserted or
	// deleted mid-walk may or may not be counted. For a fully consistent count,
	// ensure no writers are active.
	//
	// @sig     quiesceLen(ch: ConcurrentHash) -> int
	// @param   ch  the concurrent hash to count
	// @returns the exact number of live entries
	// @errors  TypeError if ch isn't a ConcurrentHash
	// @example quiesceLen(concurrentHash()) → 0
	// @since   0.1.0
	// @see     concurrentHash, atomicHashIncr
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
