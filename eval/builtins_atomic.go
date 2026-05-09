package eval

import (
	"fmt"
	"klex/ast"
	"math"
	"sync"
	"sync/atomic"
)

// builtins_atomic.go - Lock-free atomic array operations.
//
// kLex async tasks run on snapshotted environments (lock-free), but that
// isolation prevents shared mutable state. Channels work but have overhead.
// These atomic primitives let many goroutines update a shared array
// simultaneously WITHOUT mutexes - using CPU compare-and-swap instructions
// directly. The performance is roughly equivalent to native Go atomic code.
//
// Two backing types:
//   AtomicIntArray   - integer array, sync/atomic.AddInt64 directly
//   AtomicFloatArray - float64 stored as int64 bits, atomic CAS-loop for adds
//
// Use case in hantafrog: virus_map["cells"] becomes an AtomicFloatArray, and
// updateRodentVirus's Phase 1 (parallel deltas) and Phase 2 (serial merge)
// collapse into a single parallel pass that calls atomicAdd directly.

func init() {
	// atomicIntArray(size, [initial]) -> AtomicIntArray
	// Creates a fixed-size lock-free integer array. Optional initial value
	// fills every slot (default 0).
	Builtins["atomicIntArray"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("atomicIntArray expects 1 or 2 arguments (size, [initial])", ast.Pos{})
		}
		sizeObj, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicIntArray: size must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		if sizeObj.Value < 0 {
			return runtimeError("atomicIntArray: size must be non-negative", ast.Pos{})
		}
		var initial int64 = 0
		if len(args) == 2 {
			initObj, ok := args[1].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("atomicIntArray: initial must be integer, got %s", args[1].Type()), ast.Pos{})
			}
			initial = int64(initObj.Value)
		}
		data := make([]int64, sizeObj.Value)
		if initial != 0 {
			for i := range data {
				data[i] = initial
			}
		}
		return &AtomicIntArray{Data: data}
	}}

	// atomicFloatArray(size, [initial]) -> AtomicFloatArray
	// Creates a fixed-size lock-free float64 array. Optional initial value
	// fills every slot (default 0.0).
	Builtins["atomicFloatArray"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("atomicFloatArray expects 1 or 2 arguments (size, [initial])", ast.Pos{})
		}
		sizeObj, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicFloatArray: size must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		if sizeObj.Value < 0 {
			return runtimeError("atomicFloatArray: size must be non-negative", ast.Pos{})
		}
		var initial float64 = 0.0
		if len(args) == 2 {
			switch v := args[1].(type) {
			case *Float:
				initial = v.Value
			case *Integer:
				initial = float64(v.Value)
			default:
				return typeError(fmt.Sprintf("atomicFloatArray: initial must be number, got %s", args[1].Type()), ast.Pos{})
			}
		}
		bits := make([]int64, sizeObj.Value)
		if initial != 0.0 {
			b := int64(math.Float64bits(initial))
			for i := range bits {
				bits[i] = b
			}
		}
		return &AtomicFloatArray{Bits: bits}
	}}

	// atomicLoad(arr, idx) -> value
	// Atomically reads and returns the value at the given index.
	// Works on AtomicIntArray (returns Integer) or AtomicFloatArray (returns Float).
	Builtins["atomicLoad"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("atomicLoad expects 2 arguments (arr, idx)", ast.Pos{})
		}
		idxObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicLoad: idx must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		switch arr := args[0].(type) {
		case *AtomicIntArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Data) {
				return runtimeError(fmt.Sprintf("atomicLoad: index %d out of range [0, %d)", idxObj.Value, len(arr.Data)), ast.Pos{})
			}
			val := atomic.LoadInt64(&arr.Data[idxObj.Value])
			return &Integer{Value: int(val)}
		case *AtomicFloatArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Bits) {
				return runtimeError(fmt.Sprintf("atomicLoad: index %d out of range [0, %d)", idxObj.Value, len(arr.Bits)), ast.Pos{})
			}
			bits := atomic.LoadInt64(&arr.Bits[idxObj.Value])
			return &Float{Value: math.Float64frombits(uint64(bits))}
		default:
			return typeError(fmt.Sprintf("atomicLoad: first argument must be AtomicIntArray or AtomicFloatArray, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// atomicStore(arr, idx, value) -> null
	// Atomically writes the value at the given index.
	Builtins["atomicStore"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("atomicStore expects 3 arguments (arr, idx, value)", ast.Pos{})
		}
		idxObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicStore: idx must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		switch arr := args[0].(type) {
		case *AtomicIntArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Data) {
				return runtimeError(fmt.Sprintf("atomicStore: index %d out of range [0, %d)", idxObj.Value, len(arr.Data)), ast.Pos{})
			}
			valObj, ok := args[2].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("atomicStore: value must be integer for AtomicIntArray, got %s", args[2].Type()), ast.Pos{})
			}
			atomic.StoreInt64(&arr.Data[idxObj.Value], int64(valObj.Value))
			return NULL
		case *AtomicFloatArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Bits) {
				return runtimeError(fmt.Sprintf("atomicStore: index %d out of range [0, %d)", idxObj.Value, len(arr.Bits)), ast.Pos{})
			}
			var f float64
			switch v := args[2].(type) {
			case *Float:
				f = v.Value
			case *Integer:
				f = float64(v.Value)
			default:
				return typeError(fmt.Sprintf("atomicStore: value must be number for AtomicFloatArray, got %s", args[2].Type()), ast.Pos{})
			}
			atomic.StoreInt64(&arr.Bits[idxObj.Value], int64(math.Float64bits(f)))
			return NULL
		default:
			return typeError(fmt.Sprintf("atomicStore: first argument must be AtomicIntArray or AtomicFloatArray, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// atomicAdd(arr, idx, delta) -> new_value
	// Atomically adds delta to the value at idx and returns the result.
	// For AtomicFloatArray, uses a CAS retry loop (lock-free but may retry under contention).
	Builtins["atomicAdd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("atomicAdd expects 3 arguments (arr, idx, delta)", ast.Pos{})
		}
		idxObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicAdd: idx must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		switch arr := args[0].(type) {
		case *AtomicIntArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Data) {
				return runtimeError(fmt.Sprintf("atomicAdd: index %d out of range [0, %d)", idxObj.Value, len(arr.Data)), ast.Pos{})
			}
			deltaObj, ok := args[2].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("atomicAdd: delta must be integer for AtomicIntArray, got %s", args[2].Type()), ast.Pos{})
			}
			newVal := atomic.AddInt64(&arr.Data[idxObj.Value], int64(deltaObj.Value))
			return &Integer{Value: int(newVal)}
		case *AtomicFloatArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Bits) {
				return runtimeError(fmt.Sprintf("atomicAdd: index %d out of range [0, %d)", idxObj.Value, len(arr.Bits)), ast.Pos{})
			}
			var delta float64
			switch v := args[2].(type) {
			case *Float:
				delta = v.Value
			case *Integer:
				delta = float64(v.Value)
			default:
				return typeError(fmt.Sprintf("atomicAdd: delta must be number for AtomicFloatArray, got %s", args[2].Type()), ast.Pos{})
			}
			// CAS loop for lock-free float add.
			for {
				oldBits := atomic.LoadInt64(&arr.Bits[idxObj.Value])
				oldVal := math.Float64frombits(uint64(oldBits))
				newVal := oldVal + delta
				newBits := int64(math.Float64bits(newVal))
				if atomic.CompareAndSwapInt64(&arr.Bits[idxObj.Value], oldBits, newBits) {
					return &Float{Value: newVal}
				}
				// Another goroutine swapped in between - retry with the fresh value.
			}
		default:
			return typeError(fmt.Sprintf("atomicAdd: first argument must be AtomicIntArray or AtomicFloatArray, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// atomicCAS(arr, idx, old, new) -> bool
	// Compare-and-swap. If the current value at idx equals old, replaces it
	// with new and returns true. Otherwise returns false. Used for building
	// custom lock-free algorithms.
	Builtins["atomicCAS"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("atomicCAS expects 4 arguments (arr, idx, old, new)", ast.Pos{})
		}
		idxObj, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("atomicCAS: idx must be integer, got %s", args[1].Type()), ast.Pos{})
		}
		switch arr := args[0].(type) {
		case *AtomicIntArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Data) {
				return runtimeError(fmt.Sprintf("atomicCAS: index %d out of range [0, %d)", idxObj.Value, len(arr.Data)), ast.Pos{})
			}
			oldObj, ok := args[2].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("atomicCAS: old must be integer for AtomicIntArray, got %s", args[2].Type()), ast.Pos{})
			}
			newObj, ok := args[3].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("atomicCAS: new must be integer for AtomicIntArray, got %s", args[3].Type()), ast.Pos{})
			}
			swapped := atomic.CompareAndSwapInt64(&arr.Data[idxObj.Value], int64(oldObj.Value), int64(newObj.Value))
			if swapped {
				return TRUE
			}
			return FALSE
		case *AtomicFloatArray:
			if idxObj.Value < 0 || idxObj.Value >= len(arr.Bits) {
				return runtimeError(fmt.Sprintf("atomicCAS: index %d out of range [0, %d)", idxObj.Value, len(arr.Bits)), ast.Pos{})
			}
			toFloat := func(o Object, name string) (float64, Object) {
				switch v := o.(type) {
				case *Float:
					return v.Value, nil
				case *Integer:
					return float64(v.Value), nil
				}
				return 0, typeError(fmt.Sprintf("atomicCAS: %s must be number for AtomicFloatArray, got %s", name, o.Type()), ast.Pos{})
			}
			oldF, errObj := toFloat(args[2], "old")
			if errObj != nil {
				return errObj
			}
			newF, errObj := toFloat(args[3], "new")
			if errObj != nil {
				return errObj
			}
			swapped := atomic.CompareAndSwapInt64(
				&arr.Bits[idxObj.Value],
				int64(math.Float64bits(oldF)),
				int64(math.Float64bits(newF)),
			)
			if swapped {
				return TRUE
			}
			return FALSE
		default:
			return typeError(fmt.Sprintf("atomicCAS: first argument must be AtomicIntArray or AtomicFloatArray, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// parallelArrayForEach(arr, fn) -> null
	// Like parallelArrayMap but discards return values. Use this when the
	// callback's purpose is side effects (e.g. atomic updates to shared state)
	// rather than producing a transformed array. Saves the allocation of a
	// result array.
	Builtins["parallelArrayForEach"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayForEach expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayForEach: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		switch args[1].(type) {
		case *Function, *Builtin:
		default:
			return typeError(fmt.Sprintf("parallelArrayForEach: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		if n == 0 {
			return NULL
		}

		numWorkers, chunkSize := parallelChunks(n)
		snapshotEnv := snapshotForFn(args[1])

		var wg sync.WaitGroup
		var firstErr Object
		var errMu sync.Mutex

		for w := 0; w < numWorkers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			if start >= end {
				continue
			}
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				for i := start; i < end; i++ {
					_, err := callInParallel(args[1], []Object{arr.Elements[i], &Integer{Value: i}}, snapshotEnv)
					if err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
						return
					}
				}
			}(start, end)
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}
		return NULL
	}}
}
