package vm

// upvalue.go — closure capture mechanics.
//
// Design
//
// kLex's tree-walker has classic lexical-scope semantics: an inner
// function sees its enclosing function's bindings, and MUTATIONS via
// the inner function are visible to the outer. A counter-closure
// works:
//
//   counter = 0
//   fn inc() { counter = counter + 1 }
//   inc(); inc()
//   println(counter)   // 2
//
// To match this in a stack VM, each local that ANY enclosed closure
// references must be boxed in a heap-allocated cell. Locals that no
// inner function references stay un-boxed — but for simplicity (and
// because the Go GC handles the small extra allocation cheaply) the
// VM boxes EVERY local as a cell unconditionally. The simpler design
// pays a single pointer-indirection per LoadLocal / StoreLocal and
// avoids a compile-time "is this local captured?" pre-pass.
//
// At closure-creation time (OpMakeClosure), the new CompiledFunction
// captures the CELL POINTERS (not their values) from the parent
// frame's locals or upvalues. Read/write through the cell is then
// shared between the outer frame and every closure that captured
// it. When the outer frame exits, the cells continue to live —
// they're Go-GC'd when no closure references them anymore.
//
// This implements REFERENCE semantics (Lua / Python / Wren) rather
// than value-capture (no JavaScript "closures over loop vars
// surprise"). It matches the tree-walker exactly for the test cases
// that exercise mutation through closures.

import (
	"klex/eval"
)

// UpvalueCell is the heap-allocated box that holds a kLex value
// shared between an enclosing frame and any closures that captured
// it. Mutations through the cell propagate everywhere.
//
// The Value field is intentionally public so the VM dispatch loop
// can read/write it inline without a method call — every LoadLocal
// / StoreLocal touches it, and the per-op cost matters.
type UpvalueCell struct {
	Value eval.Object
	// IsConst marks the cell as read-only after its first store.
	// Set by OpMarkConst (emitted by ConstStmt) right after the
	// initial store. Subsequent stores via OpStoreLocal / OpSetUpvalue
	// check this flag and produce a kLex runtime error matching the
	// tree-walker's "cannot reassign constant <name>" message. The
	// runtime check (rather than compile-time) lets safe() catch the
	// error the way the tree-walker does — the const-reassignment
	// failure mode in constTest expects a recoverable runtime error,
	// not a compile-time refusal.
	IsConst bool
	// ConstName carries the source-level identifier so the runtime
	// reassignment-error matches the tree-walker's "cannot reassign
	// constant <name>" message exactly. Only meaningful when IsConst
	// is true; left empty otherwise.
	ConstName string

	// Captured is set true by OpMakeClosure when this cell is
	// promoted into a closure's upvalue table. The frame teardown
	// must NOT return captured cells to the pool — the closure
	// still references them and reusing the memory would corrupt
	// the closure's captured value. Uncaptured cells (the common
	// case) round-trip through the pool without allocation.
	Captured bool
}

// newCell creates a fresh cell. Used by OpCall to set up a callee's
// locals (each slot gets its own cell so the callee's closures can
// independently capture different slots).
func newCell() *UpvalueCell {
	return &UpvalueCell{Value: eval.NULL}
}

// newCellsFor returns n fresh cells, initialised to NULL. Used to
// pre-allocate a frame's locals at the start of execute().
func newCellsFor(n int) []*UpvalueCell {
	out := make([]*UpvalueCell, n)
	for i := range out {
		out[i] = newCell()
	}
	return out
}

// upvalueRef is the COMPILE-TIME descriptor of one captured value.
// Recorded on the sub-compiler when an inner function references an
// outer-scope name. Becomes operand-data on the OpMakeClosure
// instruction so the runtime can chase pointers from the calling
// frame.
//
// IsLocal=true  → Index is a slot index into the immediate parent's
//                 locals (capture that cell directly).
// IsLocal=false → Index is into the parent's OWN upvalues (chain
//                 capture — for variables more than one nesting
//                 level outside the closure).
type upvalueRef struct {
	IsLocal bool
	Index   uint16
}
