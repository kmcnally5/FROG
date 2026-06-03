package eval

import (
	"fmt"
	"klex/ast"
	"runtime"
	"sync"
)

// parallel.go implements parallel array primitives for kLex.
//
// These builtins use Go's native goroutines to partition array work across
// runtime.NumCPU() workers, enabling true CPU-parallel computation on dense
// data without relying on the channel/async machinery.
//
// Each worker uses a snapshotted environment (shared=false) for any user
// functions it calls, so worker code is lock-free with respect to the global
// env mutex. This is the same isolation model async() uses.
//
// Design:
//   - Array of length n is split into runtime.NumCPU() contiguous chunks.
//   - Each chunk is processed by one goroutine.
//   - Workers run independently; results are collected by index, not channel.
//   - First error wins; remaining workers finish their assigned work.
//   - For *Function: env snapshot taken once, reused by all workers (no
//     mutex contention on the global env).
//   - For *Builtin: called directly, no env involvement.

// callInParallel invokes a callable in a way that's safe for parallel workers.
// For *Function, it uses applyFunctionInEnv with the supplied snapshot env so
// the call chain is lock-free. For *Builtin, it calls Fn directly.
func callInParallel(fn Object, args []Object, snapshotEnv *Environment) (Object, Object) {
	switch f := fn.(type) {
	case *Function:
		return applyFunctionInEnv(f, args, snapshotEnv)
	case *Builtin:
		result := f.Fn(args)
		if isError(result) {
			return nil, result
		}
		return result, nil
	}
	// External callable (e.g. *vm.CompiledFunction). No env-snapshot
	// is propagated because VM closures carry their own upvalue cells
	// — the snapshot mechanism is *Function-specific. M2 (audit fix):
	// ExternalCallable now has a default no-op stub, so no nil-check.
	if result, dispatched := ExternalCallable(fn, args); dispatched {
		if isError(result) {
			return nil, result
		}
		return result, nil
	}
	return nil, typeError(fmt.Sprintf("not callable: %s — parallel.map/filter/reduce expect a function as the callback, e.g. parallel.map(arr, fn(x) { return x * 2 })", fn.Type()), ast.Pos{})
}

// snapshotForFn returns a lock-free snapshot env for a *Function.
// For *Builtin it returns nil (snapshot not needed).
func snapshotForFn(fn Object) *Environment {
	if userFn, ok := fn.(*Function); ok {
		return userFn.Env.Snapshot()
	}
	return nil
}

// parallelChunks computes (numWorkers, chunkSize) for an array of length n.
// Avoids spawning more workers than elements; ensures every chunk is non-empty.
func parallelChunks(n int) (int, int) {
	numWorkers := runtime.NumCPU()
	if numWorkers > n {
		numWorkers = n
	}
	if numWorkers < 1 {
		numWorkers = 1
	}
	chunkSize := (n + numWorkers - 1) / numWorkers
	return numWorkers, chunkSize
}

func init() {
	// parallelArrayUpdate — update every element in place via fn(value, index), across CPU cores.
	//
	// Each worker handles a contiguous chunk on its own goroutine
	// (runtime.NumCPU() workers); fn's return value replaces the element at that
	// index. Mutates and returns the same array — the in-place counterpart to
	// parallelArrayMap, for dense data-parallel updates (e.g. decay passes). Since
	// chunks run concurrently, fn must not depend on other elements. On error,
	// returns the first error; other workers may already have finished their
	// chunks.
	//
	// @sig     parallelArrayUpdate(arr: array, fn: function) -> array
	// @param   arr  the array to update in place
	// @param   fn   callback invoked as fn(element, index); its return replaces the element
	// @returns the same array, mutated
	// @errors  TypeError if arr isn't an array or fn isn't callable; propagates the first error raised by fn
	// @example parallelArrayUpdate([1, 2, 3], fn(v, i) { v * 2 }) → [2, 4, 6]
	// @since   0.1.0
	// @see     parallelArrayMap, parallelArrayForEach
	Builtins["parallelArrayUpdate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayUpdate expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayUpdate: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		if !IsCallable(args[1]) {
			return typeError(fmt.Sprintf("parallelArrayUpdate: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		if n == 0 {
			return arr
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
					result, err := callInParallel(args[1], []Object{arr.Elements[i], &Integer{Value: i}}, snapshotEnv)
					if err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
						return
					}
					arr.Elements[i] = result
				}
			}(start, end)
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}
		return arr
	}}

	// parallelArrayMap — a new array of fn(value, index) for each element, across CPU cores.
	//
	// The parallel `map`: work is split into runtime.NumCPU() contiguous chunks,
	// each transformed on its own goroutine, and results are collected by index
	// so order is preserved. The input is left unchanged. Because chunks run
	// concurrently, fn must be independent per element (no cross-element state).
	// On error, returns the first error raised.
	//
	// @sig     parallelArrayMap(arr: array, fn: function) -> array
	// @param   arr  the array to transform (left unchanged)
	// @param   fn   callback invoked as fn(element, index); its return becomes the output element
	// @returns a new array of the same length holding each fn result, in order
	// @errors  TypeError if arr isn't an array or fn isn't callable; propagates the first error raised by fn
	// @example parallelArrayMap([1, 2, 3], fn(v, i) { v * 2 }) → [2, 4, 6]
	// @since   0.1.0
	// @see     parallelArrayUpdate, parallelArrayReduce, map
	Builtins["parallelArrayMap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayMap expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayMap: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		if !IsCallable(args[1]) {
			return typeError(fmt.Sprintf("parallelArrayMap: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		result := make([]Object, n)
		if n == 0 {
			return &Array{Elements: result}
		}

		// M5 lazy mutex: workers below read arr.Elements concurrently;
		// any hashes inside need their mutex on from now on.
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
					out, err := callInParallel(args[1], []Object{arr.Elements[i], &Integer{Value: i}}, snapshotEnv)
					if err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
						return
					}
					result[i] = out
				}
			}(start, end)
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}
		return &Array{Elements: result}
	}}

	// parallelArrayReduce — fold an array to a single value across CPU cores.
	//
	// Each worker reduces its own chunk starting from `initial`, then the partial
	// results are folded together serially. This only matches a serial reduce
	// when BOTH hold: (1) fn(acc, element) is ASSOCIATIVE (e.g. +, *, max — chunk
	// order isn't guaranteed); (2) `initial` is the IDENTITY for fn (0 for +, 1
	// for *, "" for concat, [] for array concat). A non-identity init is folded in
	// once per worker plus once more in the merge, so the result is wrong. When
	// you need a non-identity seed, use stdlib parallel.lex's parallel_reduce,
	// which takes a separate merge function and applies the seed exactly once.
	//
	// @sig     parallelArrayReduce(arr: array, fn: function, initial: any) -> any
	// @param   arr      the array to reduce
	// @param   fn       associative reducer invoked as fn(accumulator, element)
	// @param   initial  the identity value for fn (seeds every worker)
	// @returns the reduced value; returns initial unchanged for an empty array
	// @errors  TypeError if arr isn't an array or fn isn't callable; propagates the first error raised by fn
	// @example parallelArrayReduce([1, 2, 3, 4], fn(a, b) { a + b }, 0) → 10
	// @since   0.1.0
	// @see     parallelArrayMap, reduce
	Builtins["parallelArrayReduce"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("parallelArrayReduce expects 3 arguments (array, fn, initial)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayReduce: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		if !IsCallable(args[1]) {
			return typeError(fmt.Sprintf("parallelArrayReduce: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}
		initial := args[2]

		n := len(arr.Elements)
		if n == 0 {
			return initial
		}

		// M5 lazy mutex: workers read arr.Elements concurrently;
		// initial may also be shared between workers as their seed.
		MarkSharedRecursive(arr)
		MarkSharedRecursive(initial)

		numWorkers, chunkSize := parallelChunks(n)
		snapshotEnv := snapshotForFn(args[1])

		partials := make([]Object, numWorkers)
		used := make([]bool, numWorkers)
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
			go func(workerIdx, start, end int) {
				defer wg.Done()
				acc := initial
				for i := start; i < end; i++ {
					out, err := callInParallel(args[1], []Object{acc, arr.Elements[i]}, snapshotEnv)
					if err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
						return
					}
					acc = out
				}
				partials[workerIdx] = acc
				used[workerIdx] = true
			}(w, start, end)
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}

		// Final serial reduce of partials.
		acc := initial
		for i := 0; i < numWorkers; i++ {
			if !used[i] {
				continue
			}
			out, err := callInParallel(args[1], []Object{acc, partials[i]}, snapshotEnv)
			if err != nil {
				return err
			}
			acc = out
		}
		return acc
	}}
}
