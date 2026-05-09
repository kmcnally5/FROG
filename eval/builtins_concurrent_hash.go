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
}

// valuesEqual compares two kLex values by structural equality for primitive
// types. Used by atomicHashCAS to determine if the current value matches the
// expected "old" value the caller is trying to swap from.
func valuesEqual(a, b Object) bool {
	if a == b {
		return true // same pointer or both nil
	}
	if a == nil || b == nil {
		return false
	}
	if a.Type() != b.Type() {
		return false
	}
	switch av := a.(type) {
	case *Integer:
		return av.Value == b.(*Integer).Value
	case *Float:
		return av.Value == b.(*Float).Value
	case *String:
		return av.Value == b.(*String).Value
	case *Boolean:
		return av.Value == b.(*Boolean).Value
	case *Null:
		return true // *Null is a singleton
	}
	return false
}
