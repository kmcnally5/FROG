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
	default:
		return nil, typeError(fmt.Sprintf("not callable: %s", fn.Type()), ast.Pos{})
	}
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
	// parallelArrayUpdate mutates each element in-place using fn(value, index).
	// The function's return value replaces the element at that index.
	// Uses runtime.NumCPU() goroutines; each works on a contiguous chunk.
	//
	// Use this for dense data-parallel updates (e.g. virus map decay):
	//   parallelArrayUpdate(cells, fn(v, i) { v * 0.5 })
	//
	// Returns the same array (mutated). On error, returns the first error
	// encountered; other workers may have completed their chunks before the
	// error is observed.
	Builtins["parallelArrayUpdate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayUpdate expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayUpdate: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		switch args[1].(type) {
		case *Function, *Builtin:
		default:
			return typeError(fmt.Sprintf("parallelArrayUpdate: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		if n == 0 {
			return arr
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

	// parallelArrayMap returns a new array where each element is fn(value, index)
	// of the corresponding input element. Element order is preserved.
	// Uses runtime.NumCPU() goroutines.
	//
	// Use this when you need a transformed copy without mutating the input:
	//   doubled = parallelArrayMap(nums, fn(v, i) { v * 2 })
	Builtins["parallelArrayMap"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("parallelArrayMap expects 2 arguments (array, fn)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayMap: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		switch args[1].(type) {
		case *Function, *Builtin:
		default:
			return typeError(fmt.Sprintf("parallelArrayMap: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}

		n := len(arr.Elements)
		result := make([]Object, n)
		if n == 0 {
			return &Array{Elements: result}
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

	// parallelArrayReduce performs a parallel reduction over the array.
	// fn(accumulator, element) must be ASSOCIATIVE (e.g. add, multiply, max).
	// Non-associative functions produce undefined results because chunk order
	// is not guaranteed.
	//
	// Each worker reduces its chunk independently starting from `initial`,
	// then a final serial reduce combines the per-worker partials.
	//
	//   total = parallelArrayReduce(nums, fn(a, b) { a + b }, 0)
	Builtins["parallelArrayReduce"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("parallelArrayReduce expects 3 arguments (array, fn, initial)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("parallelArrayReduce: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		switch args[1].(type) {
		case *Function, *Builtin:
		default:
			return typeError(fmt.Sprintf("parallelArrayReduce: second argument must be a function, got %s", args[1].Type()), ast.Pos{})
		}
		initial := args[2]

		n := len(arr.Elements)
		if n == 0 {
			return initial
		}

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
