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
	// atomicIntArray — create a fixed-size lock-free integer array.
	//
	// Backs shared mutable state that many async tasks can update concurrently
	// without mutexes (via atomicAdd/atomicCAS). The size is fixed at creation;
	// every slot starts at `initial` (default 0).
	//
	// @sig     atomicIntArray(size: int, [initial: int]) -> AtomicIntArray
	// @param   size     number of slots (must be >= 0)
	// @param   initial  value to fill every slot with (default 0)
	// @returns a new AtomicIntArray of the given size
	// @errors  TypeError if size/initial aren't integers; RuntimeError if size is negative
	// @example atomicIntArray(3)        → AtomicIntArray(size=3)
	// @example atomicLoad(atomicIntArray(4, 7), 2) → 7
	// @since   0.1.0
	// @see     atomicFloatArray, atomicLoad, atomicAdd
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

	// atomicFloatArray — create a fixed-size lock-free float array.
	//
	// Like atomicIntArray but for floats. Each slot stores a float64 as its raw
	// bits; atomicAdd uses a compare-and-swap retry loop to stay lock-free. The
	// size is fixed at creation; every slot starts at `initial` (default 0.0).
	//
	// @sig     atomicFloatArray(size: int, [initial: number]) -> AtomicFloatArray
	// @param   size     number of slots (must be >= 0)
	// @param   initial  value to fill every slot with (default 0.0)
	// @returns a new AtomicFloatArray of the given size
	// @errors  TypeError if size isn't an integer or initial isn't a number; RuntimeError if size is negative
	// @example atomicFloatArray(2)      → AtomicFloatArray(size=2)
	// @example atomicLoad(atomicFloatArray(3, 1.5), 0) → 1.5
	// @since   0.1.0
	// @see     atomicIntArray, atomicLoad, atomicAdd
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

	// atomicLoad — atomically read the value at an index.
	//
	// Returns an int for an AtomicIntArray, a float for an AtomicFloatArray. The
	// read is atomic, so it never observes a half-written value during concurrent
	// updates.
	//
	// @sig     atomicLoad(arr: AtomicIntArray|AtomicFloatArray, idx: int) -> number
	// @param   arr  the atomic array to read from
	// @param   idx  the slot index (0-based)
	// @returns the value at idx (int or float, matching the array type)
	// @errors  TypeError if idx isn't an integer or arr isn't an atomic array; RuntimeError if idx is out of range
	// @example atomicLoad(atomicIntArray(3, 5), 1) → 5
	// @since   0.1.0
	// @see     atomicStore, atomicAdd, atomicCAS
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

	// atomicStore — atomically write a value at an index.
	//
	// The value's type must match the array: integer for an AtomicIntArray,
	// number (int or float) for an AtomicFloatArray. Overwrites unconditionally —
	// use atomicCAS when the write should depend on the current value.
	//
	// @sig     atomicStore(arr: AtomicIntArray|AtomicFloatArray, idx: int, value: number) -> null
	// @param   arr    the atomic array to write to
	// @param   idx    the slot index (0-based)
	// @param   value  the value to store (type must match the array)
	// @returns null
	// @errors  TypeError if idx isn't an integer, arr isn't an atomic array, or value's type doesn't match; RuntimeError if idx is out of range
	// @example atomicStore(atomicIntArray(2), 0, 9) → null
	// @since   0.1.0
	// @see     atomicLoad, atomicAdd, atomicCAS
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

	// atomicAdd — atomically add delta to a slot and return the new value.
	//
	// The whole read-modify-write is atomic, so concurrent adds never lose
	// updates (unlike load-then-store). For an AtomicFloatArray it uses a
	// compare-and-swap retry loop — still lock-free, but may spin under heavy
	// contention. This is the workhorse for parallel accumulators.
	//
	// @sig     atomicAdd(arr: AtomicIntArray|AtomicFloatArray, idx: int, delta: number) -> number
	// @param   arr    the atomic array to update
	// @param   idx    the slot index (0-based)
	// @param   delta  the amount to add (type must match the array)
	// @returns the new value at idx after adding delta
	// @errors  TypeError if idx isn't an integer, arr isn't an atomic array, or delta's type doesn't match; RuntimeError if idx is out of range
	// @example atomicAdd(atomicIntArray(3), 0, 5) → 5
	// @since   0.1.0
	// @see     atomicLoad, atomicStore, atomicCAS
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

	// atomicCAS — compare-and-swap a slot, the building block for lock-free algorithms.
	//
	// If the current value at idx equals `old`, atomically replaces it with `new`
	// and returns true; otherwise leaves it untouched and returns false. Loop on
	// a false result to build custom retry logic (e.g. atomic max, conditional
	// update).
	//
	// @sig     atomicCAS(arr: AtomicIntArray|AtomicFloatArray, idx: int, old: number, new: number) -> bool
	// @param   arr  the atomic array to update
	// @param   idx  the slot index (0-based)
	// @param   old  the value the slot must currently hold for the swap to happen
	// @param   new  the value to store if the comparison succeeds
	// @returns true if the swap happened, false if the current value didn't match old
	// @errors  TypeError if idx isn't an integer, arr isn't an atomic array, or old/new types don't match; RuntimeError if idx is out of range
	// @example atomicCAS(atomicIntArray(3), 0, 0, 9) → true
	// @example atomicCAS(atomicIntArray(3, 1), 0, 0, 9) → false
	// @since   0.1.0
	// @see     atomicLoad, atomicStore, atomicAdd
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

	// parallelArrayForEach — run fn over every element across worker goroutines, discarding results.
	//
	// Like parallelArrayMap but for side effects: the callback's return value is
	// thrown away, saving the result-array allocation. The natural partner to the
	// atomic builtins — workers call atomicAdd/atomicCAS on a shared array while
	// this fans the elements out. fn receives (element, index). The first error
	// from any worker aborts and is returned.
	//
	// @sig     parallelArrayForEach(arr: array, fn: function) -> null
	// @param   arr  the array to iterate over
	// @param   fn   callback invoked as fn(element, index); its return value is ignored
	// @returns null
	// @errors  TypeError if arr isn't an array or fn isn't callable; propagates the first error raised by fn
	// @example no-run parallelArrayForEach([1, 2, 3], fn(x, i) { atomicAdd(counts, 0, x) })
	// @since   0.1.0
	// @see     atomicAdd, parallelArrayMap
	Builtins["parallelArrayForEach"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayForEach expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayForEach: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		if !IsCallable(args[1]) {
			return typeError(fmt.Sprintf("parallelArrayForEach: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		if n == 0 {
			return NULL
		}

		// M5 lazy mutex: workers read arr.Elements concurrently.
		MarkSharedRecursive(arr)

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
