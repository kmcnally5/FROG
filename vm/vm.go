package vm

// vm.go — minimal stack-machine interpreter for a BytecodeChunk.
//
// Today's scope is deliberately tiny: the opcodes the compiler
// skeleton actually emits, and nothing else. As compiler clauses
// fill in, new opcode handlers land here. Every opcode the compiler
// emits MUST have a handler — otherwise execute() panics with the
// opcode mnemonic, which is the early-warning we want.
//
// Wire format reminders (operand widths come straight from
// opcodeOperandLayout in opcodes_gen.go):
//
//   PushConst:   <u16 constant-pool index>
//   PushInt:     <i32 literal>
//   LoadLocal:   <u16 slot>
//   StoreLocal:  <u16 slot>
//   CallBuiltin: <u16 builtin index> <u8 argc>
//   Jump:        <i32 relative offset>
//   JumpIfFalse: <i32 relative offset>
//
// All other opcodes (Halt, Pop, PushNull/True/False, arithmetic,
// comparison, Not, Neg, Return) have no operands.
//
// Stack semantics
//
//   - The value stack holds eval.Object pointers. NULL / TRUE / FALSE
//     are the existing eval-package singletons, so reference equality
//     against eval.NULL/TRUE/FALSE keeps working from kLex code.
//   - Locals live in a flat fixed-size slot array sized to
//     chunk.NumLocals at frame entry. Unused slots stay nil.
//   - On error, execution halts and the error object is returned —
//     same convention as eval.Eval, so callers don't need a different
//     branch for error handling.

import (
	"encoding/binary"
	"fmt"
	"klex/ast"
	"klex/eval"
	"sync"
)

// scriptDirBuiltinIdx is the BuiltinIndex slot of `_scriptDir`,
// resolved once at init() so the dispatch loop can detect calls to
// it without per-call name comparison. The tree-walker special-cases
// `_scriptDir` inside evalCall to walk the env chain for the
// containing script's directory (the builtin's stored Fn returns
// "" as a placeholder). The VM has no env chain — instead, it
// reads `BytecodeChunk.ScriptDir`, which main.go fills in for the
// entry chunk before Run().
var scriptDirBuiltinIdx uint16
var scriptDirBuiltinResolved bool

// scriptDirBuiltinPtr is the *eval.Builtin pointer for `_scriptDir`,
// resolved once at init() so OpCall can identity-compare against it
// without a per-call map lookup. OpCallBuiltin uses scriptDirBuiltinIdx
// (the builtin-index slot) because the opcode encodes the index inline;
// OpCall arrives with the callee on the stack as an *eval.Builtin
// pointer instead — different shape, same intent, same intercept value.
var scriptDirBuiltinPtr *eval.Builtin

func init() {
	if idx, ok := BuiltinIndex["_scriptDir"]; ok {
		scriptDirBuiltinIdx = idx
		scriptDirBuiltinResolved = true
	}
	if b, ok := eval.Builtins["_scriptDir"]; ok {
		scriptDirBuiltinPtr = b
	}
}

// stackPool recycles value-stack slices across execute() invocations.
// Profile-driven: a single `stack := make([]eval.Object, 0, 64)` on
// every recursive execute() call accounted for ~80% of the fusion
// benchmark's heap allocations (~12.8 GB of 15.8 GB total over a 1 M
// element map/filter/reduce chain). The bytecode chain itself is
// fast; the GC pressure from per-call stack allocation was the
// dominant cost. Pooling keeps the underlying array alive across
// calls; each grabbing call resets length to 0 before use.
//
// Capacity tuning: 64 covers the vast majority of expression depths
// in real kLex code. Pool entries that grow beyond this stay grown
// — sync.Pool only drops them if memory pressure forces eviction.
var stackPool = sync.Pool{
	New: func() interface{} {
		s := make([]eval.Object, 0, 64)
		return &s
	},
}

func acquireStack() *[]eval.Object {
	s := stackPool.Get().(*[]eval.Object)
	*s = (*s)[:0]
	return s
}

func releaseStack(s *[]eval.Object) {
	// Nil out elements so any *eval.Object values held by the slice
	// don't keep their referents alive after release. Cheap (range
	// over the live length, which is whatever it grew to). Skips
	// nil-out for empty slices.
	for i := range *s {
		(*s)[i] = nil
	}
	*s = (*s)[:0]
	stackPool.Put(s)
}

// M4 (audit fix, 2026-05-22): the *eval.Builtin.RetainsArgs
// annotation is now defined and tagged on `async`, but the actual
// pool-the-args optimisation it would unlock was reverted after
// the simple benchmark regressed ~18% under 8-goroutine
// contention. Same trap as A5: sync.Pool wins on single-threaded
// batched access but loses badly on per-call single-object
// acquisition under heavy concurrent goroutine pressure.
// OpCallBuiltin uses make() for all calls — the annotation stays
// as load-bearing documentation (and as the correct invariant
// should a future single-threaded VM mode want to pool).

// A2 intrinsic flat-dispatch (audit cleanup, 2026-05-22): the
// previous `callCompiledOnce` and `callCompiledTwice` helpers
// lived here. They recursively called `execute()` for each per-
// element callback in OpMap / OpFilter / OpReduce, paying Go-level
// function call + defer setup + stackPool round-trip per element.
// OpMap/Filter/Reduce now push an iter frame and let the outer
// dispatch loop drive iteration via OpReturn's iter step — see
// `iterStep` and `callFrame.iterMode` for the design. Helpers
// deleted to remove the temptation to reintroduce per-element
// Go-recursion.

// framesPool recycles call-frame slices. Most execute() invocations
// originate from intrinsics (OpMap/OpFilter/OpReduce) calling the
// per-element callback via callCompiledOnce/Twice — those callbacks
// rarely push frames of their own. Lazy allocation + pool reuse
// avoids the zero-init of a 14 KB stack array on every recursive
// execute() entry, which initially caused a 4× chain regression
// when the flat dispatch loop landed.
//
// S2 (audit cleanup): stores POINTERS to slices rather than slice
// values. Same rationale as localsSlicePool — avoids 24 bytes of
// interface-boxing overhead per Put.
var framesPool = sync.Pool{
	New: func() interface{} {
		// Start at 16 — enough for typical kLex call depths without
		// growing. Recursive workloads (fib(20)+ etc.) grow via
		// append; sync.Pool will keep the larger backing array for
		// the next caller.
		s := make([]callFrame, 16)
		return &s
	},
}

// callFrame stores caller state so OpReturn can pop back to it
// without Go-level recursion. Held only in the in-loop frames
// array; never escapes execute().
//
// stackBase is the position the caller's stack was at when it
// fired the OpCall (after popping the callable + args). The
// callee operates on stack[stackBase:] — its own pushes / pops
// must never reach below stackBase or it would corrupt the
// caller's still-live values. OpReturn truncates stack to the
// returning frame's stackBase before pushing the return value
// onto the caller's window.
//
// A2 intrinsic flat-dispatch (audit cleanup, 2026-05-22):
// `iter` is non-nil only when this frame is the driver for an
// intrinsic loop (map/filter/reduce). OpReturn, on popping back
// to such a frame, runs the per-iteration step (apply retVal →
// output buffer or accumulator, advance index, either re-launch
// the callback OR finalise and pop the frame). Storing the iter
// state behind a pointer keeps callFrame small (~80 bytes) for
// the common non-iter case — the framesPool memory traffic
// dominates programs that do many small calls (recursive
// benchmarks, async workers).
type callFrame struct {
	chunk     *BytecodeChunk
	locals    []*UpvalueCell
	upvalues  []*UpvalueCell
	pc        int
	stackBase int
	iter      *iterState
}

// iterState holds the per-iteration bookkeeping for OpMap /
// OpFilter / OpReduce flat-dispatch. Allocated once per
// intrinsic-driving frame (NOT per element), so the cost is
// amortised across the whole iteration. mode is the originating
// opcode (OpMap/OpFilter/OpReduce).
type iterState struct {
	mode  opcode
	elems []eval.Object
	idx   int
	fn    *CompiledFunction
	out   []eval.Object // Map/Filter result buffer
	acc   eval.Object   // Reduce accumulator
	// M3: the originating opcode's pc (pointing at OpMap/OpFilter/
	// OpReduce itself), captured at frame-push time. Used by iterStep
	// to attribute mid-iteration errors back to the source line of
	// the call rather than to whatever instruction happens to follow
	// the intrinsic — parent.pc was saved as the post-dispatch pc,
	// so parent.pc-1 is one byte past the opcode, not the opcode.
	opPC int
}

// cellPool recycles individual *UpvalueCell objects. Cells that get
// captured by a closure (marked via Captured = true at OpMakeClosure
// time) MUST NOT be returned to the pool — the closure still holds
// them. Uncaptured cells are the common case (most function locals
// never escape) and reusing them eliminates the per-call allocation
// that dominated the fusion benchmark profile.
var cellPool = sync.Pool{
	New: func() interface{} { return &UpvalueCell{Value: eval.NULL} },
}

func acquireCell() *UpvalueCell {
	c := cellPool.Get().(*UpvalueCell)
	c.Value = eval.NULL
	c.IsConst = false
	c.ConstName = ""
	c.Captured = false
	return c
}

func releaseCell(c *UpvalueCell) {
	if c == nil || c.Captured {
		return
	}
	cellPool.Put(c)
}

// localsSlicePool recycles `[]*UpvalueCell` slice headers. Reused
// per execute() call to avoid the slice header allocation on top of
// the per-cell allocation. Cells inside are released individually
// via releaseCell() so captured ones live on.
//
// P1 (audit cleanup): stores POINTERS to slices rather than slice
// values. Storing a slice as `interface{}` boxes its 3-word header
// on every Put — about 24 bytes per call wasted. Storing a `*[]…`
// boxes only the 1-word pointer, matching stackPool's pattern.
var localsSlicePool sync.Pool

func acquireLocals(n int) []*UpvalueCell {
	if v := localsSlicePool.Get(); v != nil {
		slicePtr := v.(*[]*UpvalueCell)
		got := *slicePtr
		if cap(got) >= n {
			got = got[:n]
			for i := 0; i < n; i++ {
				got[i] = acquireCell()
			}
			return got
		}
	}
	out := make([]*UpvalueCell, n)
	for i := range out {
		out[i] = acquireCell()
	}
	return out
}

func releaseLocals(s []*UpvalueCell) {
	for i, c := range s {
		releaseCell(c)
		s[i] = nil
	}
	s = s[:0]
	localsSlicePool.Put(&s)
}

// Run compiles and executes a parsed program. Returns the value the
// program leaves on top of the stack at Halt, or an eval.Object that
// IsError if execution failed.
//
// Run is the convenience entry point — callers that already hold a
// compiled chunk should use Execute directly to skip the compile
// cost. Provided so the diff-runner can call Run(prog) without
// reaching into the compiler's API.
//
// A4 (audit cleanup): Run uses the cell pool. The top-level chunk's
// locals now round-trip through localsSlicePool / cellPool just like
// every other frame's locals. Earlier `newCellsFor` direct allocation
// leaked the slice + cells to GC each Run invocation, which mattered
// for vmdiff batch runs (one Run per .lex file) and any future caller
// that calls Run repeatedly.
func Run(chunk *BytecodeChunk) (eval.Object, error) {
	locals := acquireLocals(chunk.NumLocals)
	defer releaseLocals(locals)
	return execute(chunk, locals, nil)
}

// RunModule executes a chunk's top-level code (as Run does) and then
// builds an *eval.Environment exposing every top-level binding as a
// LIVE getter pointing at the persistent UpvalueCell. M6
// (audit follow-up, 2026-05-22): replaces the tree-walker import
// path when the module compiles to bytecode cleanly. Caller (eval's
// ImportStmt arm via the VMCompileAndRunModule hook) wraps the
// returned env in a *Module and caches it.
//
// Why live getters and not env.Set with the snapshotted value:
// tree-walker semantics expose modEnv.store directly, so an external
// reader of a mutable top-level binding (`t._passed` etc.) sees the
// current value as the module's own functions mutate it. With M6,
// the binding's cell is mutated by VM-compiled functions through
// closures (the cell is in the closure's Upvalues); the live getter
// returns the cell's current Value on every Get. Matches the
// tree-walker contract.
//
// Top-level cells are pinned via Captured=true so releaseLocals
// doesn't return them to the cell pool — they live as long as the
// returned env (which is in turn cached in eval.moduleCache).
func RunModule(chunk *BytecodeChunk) (*eval.Environment, error) {
	locals := acquireLocals(chunk.NumLocals)
	if _, err := execute(chunk, locals, nil); err != nil {
		releaseLocals(locals)
		return nil, err
	}
	env := eval.NewEnv()
	if chunk.ScriptDir != "" {
		env.SetScriptDir(chunk.ScriptDir)
	}
	// Wire live getters for every top-level name. Pin each cell so
	// releaseLocals (below) returns the slice header to the pool
	// but leaves the captured cells alive on the heap, still
	// referenced by both the closures that captured them and the
	// getters we install here.
	for name, slot := range chunk.TopLevelNames {
		if slot >= len(locals) || locals[slot] == nil {
			continue
		}
		cell := locals[slot]
		cell.Captured = true
		env.SetLiveBinding(name, func() eval.Object { return cell.Value })
	}
	releaseLocals(locals)
	return env, nil
}

// execute is the dispatch loop. One Go-level call per outermost
// invocation (Run or callCompiledOnce/Twice from an intrinsic); all
// in-loop kLex function calls go through frames[] push/pop, NOT
// recursive Go calls — that's the flat-dispatch architecture
// established in the post-2026-05-22 rewrite. See "Closures: how
// they work" and "Performance milestone — post-flat-dispatch" in
// the project memory for the design history.
//
// Error model (A1+A9, 2026-05-22 audit follow-up):
//
//   - Compiler bugs (unbalanced bytecode, missing handler, malformed
//     operands) → `return nil, vmError(...)`. The single `defer` at
//     the top releases the stack and frames pools; per-frame locals
//     are leaked to GC, which is fine because real-world bytecode
//     should never trigger these.
//   - kLex user errors (TypeError, RuntimeErr from EvalBinaryOp,
//     index out of bounds, const reassignment, etc.) → set
//     `bubbleErrVal = err; goto bubbleError`. The bubbleError label
//     pops one frame at a time, releasing each frame's locals via
//     releaseLocals before continuing the unwind. When frameCount
//     reaches 0, bubbleError returns the error value cleanly.
//   - The "push error onto stack and continue" pattern (used by
//     OpCallMethod fallback when CallCallable returns an error, and
//     by OpMap/Filter/Reduce when the per-element callback errors)
//     ALSO works — the next OpReturnIfError emitted between
//     statements catches the error and routes it through bubbleError.
func execute(chunk *BytecodeChunk, locals []*UpvalueCell, upvalues []*UpvalueCell) (eval.Object, error) {
	if chunk == nil || len(chunk.Code) == 0 {
		return eval.NULL, nil
	}
	// Defensive: caller may pass a too-small locals slice. Grow to
	// NumLocals so LoadLocal/StoreLocal never index out of range.
	// New cells are fresh — guarantees mutations don't leak into the
	// caller's frame (the caller's already-allocated cells stay
	// intact in the first len(locals) positions).
	if len(locals) < chunk.NumLocals {
		grown := make([]*UpvalueCell, chunk.NumLocals)
		copy(grown, locals)
		for i := len(locals); i < chunk.NumLocals; i++ {
			grown[i] = acquireCell()
		}
		locals = grown
	}

	// Acquire a pooled value stack. The fresh `make([]eval.Object,
	// 0, 64)` here used to dominate the fusion benchmark profile
	// (~12.8 GB of 15.8 GB total over a 1 M-call workload). Pool
	// reuse keeps the underlying array across calls; each grab
	// starts at length 0. Release path uses defer so every return
	// site (Halt, Return, error early-return, panic recovery)
	// puts the stack back without per-site bookkeeping.
	stackPtr := acquireStack()
	stack := *stackPtr
	// In-loop call frame stack. Replaces Go-level recursive
	// execute() calls for the OpCall (*CompiledFunction) path
	// with array push/pop on a lazily-allocated frames slice.
	// Most execute() entries (those from intrinsics calling a
	// per-element callback) never push a frame — those pay zero
	// allocation cost for the frames machinery. Only the first
	// OpCall in a given execute() invocation triggers
	// framesPool.Get.
	var frames []callFrame
	frameCount := 0
	// Single defer for both pool releases — defer setup cost is
	// real (~10 ns) and was a measurable regression when we had
	// two defers per execute() call across 3 M+ chain-workload
	// invocations.
	defer func() {
		*stackPtr = stack
		releaseStack(stackPtr)
		if frames != nil {
			for i := range frames {
				frames[i] = callFrame{}
			}
			framesPool.Put(&frames)
		}
	}()

	// Error-bubbling channel. Mid-op kLex errors set this and goto
	// the bubble label at the bottom of the for-loop body, which
	// pops the current frame the same way OpReturn does. The flat
	// dispatch can't use Go-level `return errVal, nil` for kLex
	// errors any more: that would skip popping the frames stack
	// and pollute the caller with whatever the callee left behind.
	var bubbleErrVal eval.Object

	// stackBase tracks where the current frame's stack window
	// begins. The callee's stack pushes / pops operate on
	// stack[stackBase:]; opcodes that ask "is the stack empty"
	// (OpReturnIfError, the "discard statement value" path) MUST
	// compare against stackBase, not 0 — the caller's values are
	// still on the shared stack below stackBase. This is the
	// classic clox stack-window technique; without it, the
	// callee corrupts caller state.
	stackBase := 0

	pc := 0
	for pc < len(chunk.Code) {
		op := opcode(chunk.Code[pc])
		pc++

		switch op {
		case OpHalt:
			if len(stack) == 0 {
				return eval.NULL, nil
			}
			return stack[len(stack)-1], nil

		case OpReturn:
			// Pop the return value (NULL if the body left
			// nothing in this frame's window).
			var retVal eval.Object = eval.NULL
			if len(stack) > stackBase {
				retVal = stack[len(stack)-1]
			}
			if frameCount == 0 {
				// Top-level chunk's return — exit execute() and
				// let the caller see the value. The current
				// locals slice was supplied by the caller (Run
				// or callCompiledOnce/Twice), which owns its
				// lifetime, so we don't release here.
				//
				// Enforce the declared return type here too: a function
				// invoked via ExternalCallable (safe/async/apply/generic
				// map fallback) runs as a fresh execute() and returns at
				// frameCount==0, so the post-iter check below never runs
				// for it. Skip when retVal is already an error (don't mask
				// it) — and the top-level program chunk has ReturnType=""
				// so it's a no-op there.
				if chunk.ReturnType != "" && !eval.IsError(retVal) {
					retName := chunk.FnName
					if retName == "" {
						retName = "anonymous"
					}
					if e := eval.CheckReturnAnnotation(chunk.ReturnType, retVal, retName,
						ast.Pos{Line: lineForOffset(chunk, pc-1)}); e != nil {
						return e, nil
					}
				}
				return retVal, nil
			}
			// A2 intrinsic flat-dispatch: if the frame we're
			// returning TO is an intrinsic driver (OpMap/Filter/
			// Reduce in progress), run the per-iteration step.
			// On "advance", re-launch the callback against the
			// same iter frame without push/pop. On "done", fall
			// through to the standard pop with retVal swapped
			// for the assembled result (or error).
			//
			// Hot-path note: the `parent.iter != nil` check is the
			// only overhead OpReturn pays for non-iter frames —
			// one pointer load + one nil-test. iter state lives
			// behind a pointer so callFrame stays ~80 bytes and
			// the framesPool memory traffic stays small.
			parentFrame := &frames[frameCount-1]
			if it := parentFrame.iter; it != nil {
				final, done := iterStep(parentFrame, retVal)
				if !done {
					// More iterations — relaunch.
					stack = stack[:stackBase]
					releaseLocals(locals)
					locals = acquireLocals(it.fn.Chunk.NumLocals)
					switch it.mode {
					case OpMap, OpFilter:
						locals[0].Value = it.elems[it.idx]
					case OpReduce:
						locals[0].Value = it.acc
						locals[1].Value = it.elems[it.idx]
					}
					if it.fn.SelfSlot >= 0 && it.fn.SelfSlot < len(locals) {
						locals[it.fn.SelfSlot].Value = it.fn
					}
					pc = 0
					// chunk, upvalues, stackBase already point at
					// the callback's window — no change needed.
					continue
				}
				// Iteration done. Push the assembled value
				// through the standard pop below.
				retVal = final
			}
			// H3-sweep: if the callee returned an internal *Error
			// (or an iter step finalised with one), route through
			// bubbleError so the error keeps propagating instead of
			// landing on the caller's expression-context stack where
			// the next op might consume and mask it. Same protection
			// the per-op IsError-on-operand checks provide; matters
			// for `let x = f() + 1` where f returns an error tuple
			// via the iter path or a tree-walker fn that bubbled.
			if eval.IsError(retVal) {
				bubbleErrVal = retVal
				goto bubbleError
			}
			// Enforce the callee's declared return type (if any). `chunk`
			// is still the callee's chunk here (reassigned to the caller's
			// below). Skip the intrinsic-finalisation path
			// (parentFrame.iter != nil): there retVal is the assembled
			// map/filter/reduce result attributed to the callback chunk,
			// not a single callback return — type-checking it against the
			// callback's annotation would be a false positive.
			if chunk.ReturnType != "" && parentFrame.iter == nil {
				retName := chunk.FnName
				if retName == "" {
					retName = "anonymous"
				}
				if e := eval.CheckReturnAnnotation(chunk.ReturnType, retVal, retName,
					ast.Pos{Line: lineForOffset(chunk, pc-1)}); e != nil {
					bubbleErrVal = e
					goto bubbleError
				}
			}
			// Inner-call return: discard everything the callee
			// pushed (truncate to its stackBase), release its
			// locals, restore the caller's frame, push retVal
			// onto the caller's window.
			stack = stack[:stackBase]
			releaseLocals(locals)
			frameCount--
			chunk = frames[frameCount].chunk
			locals = frames[frameCount].locals
			upvalues = frames[frameCount].upvalues
			pc = frames[frameCount].pc
			stackBase = frames[frameCount].stackBase
			// Clear out the now-unused frame entry so we don't
			// keep references alive between calls (helps GC for
			// any chunk constants the frame closed over).
			frames[frameCount] = callFrame{}
			stack = append(stack, retVal)

		case OpFreshCell:
			slot := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(slot) >= len(locals) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("FreshCell: slot %d out of range (NumLocals=%d) — compiler bug", slot, len(locals)))
			}
			// Refuse to overwrite a const cell — `let X = …` where
			// `X` is already a const triggers the same runtime error
			// as a bare reassignment. The tree-walker rejects via
			// env.Assign / env.CheckWritable; we reproduce that here
			// rather than silently dropping the const flag. The
			// pending RHS is still on top of the stack — pop it so
			// the error path doesn't leak the value.
			if c := locals[slot]; c != nil && c.IsConst {
				// M1: pop only if THIS frame's window has a value.
				// `len(stack) > 0` would pop a caller's value when a
				// callee frame's window is empty and stackBase > 0.
				if len(stack) > stackBase {
					stack = stack[:len(stack)-1]
				}
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
					Message: "cannot reassign constant " + c.ConstName,
				}
				goto bubbleError
			}
			// A5 reverted 2026-05-22 night: A/B against benchTest_simple
			// showed acquireCell here causes a 2× regression on the
			// 8-goroutine recursive workload. Hypothesis is cellPool
			// contention or sync.Pool overhead under heavy concurrent
			// per-iteration `let` bindings — newCell's direct alloc is
			// faster for this access pattern. acquireCell stays in
			// use everywhere else (acquireLocals, etc.) where it does
			// help.
			locals[slot] = newCell()

		case OpMarkConst:
			slot := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(slot) >= len(locals) {
				return nil, vmError(chunk, pc-5, fmt.Sprintf("MarkConst: slot %d out of range (NumLocals=%d) — compiler bug", slot, len(locals)))
			}
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-5, "MarkConst: name idx out of range")
			}
			nameStr, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-5, "MarkConst: name slot must hold *String")
			}
			locals[slot].IsConst = true
			locals[slot].ConstName = nameStr.Value
			// Deep-freeze the bound value so any reachable Array /
			// Hash / StructInstance refuses mutation — matches the
			// tree-walker's ConstStmt behaviour (every mutation site
			// checks .frozen and emits "cannot mutate frozen X").
			if v := locals[slot].Value; v != nil {
				eval.DeepFreeze(v)
			}

		case OpInstallMethod:
			// Pop closure, install on the StructDef on stack below.
			// See opcodes_def.go for the rationale (M5+M6 follow-up:
			// methods need proper closure semantics so they can
			// reach module-level fns like newObservable).
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if len(stack)-stackBase < 2 {
				return nil, vmError(chunk, pc-3, "InstallMethod: need [def, closure] on stack")
			}
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, "InstallMethod: name idx out of range")
			}
			nameObj, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-3, "InstallMethod: name constant must be *String")
			}
			closure := stack[len(stack)-1]
			defObj := stack[len(stack)-2]
			stack = stack[:len(stack)-1] // pop closure; def stays
			def, ok := defObj.(*eval.StructDef)
			if !ok {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("InstallMethod: expected *StructDef on stack, got %s", defObj.Type()))
			}
			if def.MethodsAny == nil {
				def.MethodsAny = make(map[string]eval.Object)
			}
			def.MethodsAny[nameObj.Value] = closure

		case OpLoadScriptArgs:
			// Runtime-injected identifier `__args__`. The tree-walker
			// finds it via env.Get because main.go does env.Set
			// ("__args__", ...) before Eval; the VM gets the same
			// shape by reading chunk.ScriptArgs (populated by
			// PropagateScriptArgs on entry) and pushing a fresh
			// *Array around it. Fresh per-load matches tree-walker
			// semantics: each read of `__args__` yields a distinct
			// array value, so a script that does `let a = __args__;
			// push(a, "x")` doesn't surprise a later `__args__` reader.
			// Empty slice is fine — Array{Elements: nil} behaves like
			// a zero-length array for len/index/iteration.
			stack = append(stack, &eval.Array{Elements: chunk.ScriptArgs})

		case OpUndefinedName:
			// Deferred-resolution opcode. The compiler emits this when
			// an identifier couldn't be resolved as a local or upvalue.
			// We give Builtins a last chance at runtime — that lets
			// builtins-used-as-values (e.g. `arr |> trim`, `map(arr,
			// trim)`) work without the compiler having to know about
			// every Go-side builtin. If even Builtins doesn't know the
			// name, we early-return a kLex *Error so the surrounding
			// OpReturnIfError chain bubbles it like the tree-walker
			// bubbles an "undefined variable" error from runtime.
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, "UndefinedName: const idx out of range")
			}
			nameObj, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-3, "UndefinedName: const slot must hold *String")
			}
			if b, ok := eval.Builtins[nameObj.Value]; ok {
				stack = append(stack, b)
				continue
			}
			// Match the tree-walker's special "_" message — `_` is a
			// formal discard placeholder that can be ASSIGNED to but
			// never READ. The user-visible message is part of the
			// language contract (discardTest depends on it byte-for-
			// byte).
			if nameObj.Value == "_" {
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
					Message: "_ is a discard — its value cannot be read",
				}
				goto bubbleError
			}
			bubbleErrVal = &eval.Error{
				Kind:    eval.RuntimeErr,
				Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
				Message: "undefined name: " + nameObj.Value,
			}
			goto bubbleError

		case OpPop:
			// M1: "empty" means at the current frame's stackBase.
			// Caller values below stackBase belong to a different frame.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-1, "Pop on empty stack")
			}
			// H3-sweep: if the value being discarded is an *Error,
			// bubble it instead of silently dropping it. OpPop is the
			// "statement value discard" op; an error that landed here
			// without OpReturnIfError catching it first means a
			// preceding op pushed it (e.g. via the legacy "push errObj
			// to stack" pattern that newer H3 fixes converted to
			// bubbleError). Defensive — preserves the invariant that
			// internal errors always propagate.
			top := stack[len(stack)-1]
			if eval.IsError(top) {
				stack = stack[:len(stack)-1]
				bubbleErrVal = top
				goto bubbleError
			}
			stack = stack[:len(stack)-1]

		case OpPushNull:
			stack = append(stack, eval.NULL)

		case OpPushTrue:
			stack = append(stack, eval.TRUE)

		case OpPushFalse:
			stack = append(stack, eval.FALSE)

		case OpPushConst:
			idx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(idx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("PushConst: constant index %d out of range (pool size %d)", idx, len(chunk.Constants)))
			}
			stack = append(stack, chunk.Constants[idx])

		case OpPushInt:
			v := int32(binary.LittleEndian.Uint32(chunk.Code[pc:]))
			pc += 4
			stack = append(stack, eval.NewInteger(int(v)))

		case OpLoadLocal:
			slot := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(slot) >= len(locals) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("LoadLocal: slot %d out of range (NumLocals=%d) — compiler emitted wrong slot index", slot, len(locals)))
			}
			cell := locals[slot]
			// L2 (audit fix): only `cell == nil` is a real possibility
			// — acquireCell and newCell both initialise Value to
			// eval.NULL, never nil, so `cell.Value == nil` was dead
			// code. A nil cell pointer in locals[] indicates a compiler
			// bug (locals slice not pre-populated). Keep the cheap
			// sanity guard; drop the impossible branch.
			if cell == nil {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("LoadLocal: slot %d has nil cell — compiler bug (locals not initialised)", slot))
			}
			stack = append(stack, cell.Value)

		case OpStoreLocal:
			slot := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(slot) >= len(locals) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("StoreLocal: slot %d out of range (NumLocals=%d) — compiler bug", slot, len(locals)))
			}
			// A8: stackBase-aware. The caller's stack values below
			// stackBase belong to a different frame; only the current
			// frame's window is fair game.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-3, "StoreLocal: empty stack")
			}
			cell := locals[slot]
			// H3-sweep: bubble *Error operand instead of silently
			// storing it as the binding's value (which would surface
			// later as a confusing "operator X not defined for ERROR"
			// or similar downstream type error).
			top := stack[len(stack)-1]
			if eval.IsError(top) {
				stack = stack[:len(stack)-1]
				bubbleErrVal = top
				goto bubbleError
			}
			if cell.IsConst {
				stack = stack[:len(stack)-1]
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
					Message: "cannot reassign constant " + cell.ConstName,
				}
				goto bubbleError
			}
			cell.Value = top
			stack = stack[:len(stack)-1]

		// ── Upvalues (closure capture) ────────────────────────────────
		case OpGetUpvalue:
			idx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(idx) >= len(upvalues) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("GetUpvalue: idx %d out of range (have %d) — compiler bug", idx, len(upvalues)))
			}
			cell := upvalues[idx]
			if cell == nil {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("GetUpvalue: upvalue[%d] is nil — capture never happened", idx))
			}
			if cell.Value == nil {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("GetUpvalue: upvalue[%d] is uninitialised", idx))
			}
			stack = append(stack, cell.Value)

		case OpSetUpvalue:
			idx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(idx) >= len(upvalues) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("SetUpvalue: idx %d out of range (have %d)", idx, len(upvalues)))
			}
			// A8: stackBase-aware.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-3, "SetUpvalue: empty stack")
			}
			upCell := upvalues[idx]
			// H3-sweep: bubble *Error operand (see OpStoreLocal).
			top := stack[len(stack)-1]
			if eval.IsError(top) {
				stack = stack[:len(stack)-1]
				bubbleErrVal = top
				goto bubbleError
			}
			if upCell.IsConst {
				stack = stack[:len(stack)-1]
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
					Message: "cannot reassign constant " + upCell.ConstName,
				}
				goto bubbleError
			}
			upCell.Value = top
			stack = stack[:len(stack)-1]

		case OpMakeClosure:
			templateIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(templateIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeClosure: template const idx %d out of range (pool size %d)", templateIdx, len(chunk.Constants)))
			}
			template, ok := chunk.Constants[templateIdx].(*CompiledFunction)
			if !ok {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeClosure: template at const[%d] is %s, not a CompiledFunction", templateIdx, chunk.Constants[templateIdx].Type()))
			}
			// Capture each upvalue per the template's compile-time refs.
			// IsLocal=true → grab the cell from the caller's locals.
			// IsLocal=false → grab the cell from the caller's upvalues
			// (i.e. continue chaining outward).
			caps := make([]*UpvalueCell, len(template.UpvalueRefs))
			for i, ref := range template.UpvalueRefs {
				if ref.IsLocal {
					if int(ref.Index) >= len(locals) {
						return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeClosure: local upvalue ref idx %d out of range (have %d)", ref.Index, len(locals)))
					}
					// Mark the cell as captured so the frame teardown
					// keeps it alive (doesn't pool-release the
					// memory the closure still references).
					locals[ref.Index].Captured = true
					caps[i] = locals[ref.Index]
				} else {
					if int(ref.Index) >= len(upvalues) {
						return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeClosure: outer upvalue ref idx %d out of range (have %d)", ref.Index, len(upvalues)))
					}
					// Outer upvalue: already a heap cell from a prior
					// capture; bump Captured (idempotent) so a
					// re-capture by a deeper closure stays
					// pool-safe.
					upvalues[ref.Index].Captured = true
					caps[i] = upvalues[ref.Index]
				}
			}
			// Closures share the Chunk, NumParams, Variadic, SelfSlot,
			// and UpvalueRefs descriptor with their template — only
			// the captured Upvalues slice is per-instance. Variadic
			// MUST be copied (was missing pre-2026-05-22): closures
			// produced from a variadic template silently lost the
			// flag, causing "expects N arguments, got M" instead of
			// "expects at least N" and rejecting variadic calls.
			closure := &CompiledFunction{
				Name:          template.Name,
				Chunk:         template.Chunk,
				NumParams:     template.NumParams,
				NumRequired:   template.NumRequired,
				DefaultValues: template.DefaultValues,
				Variadic:      template.Variadic,
				SelfSlot:      template.SelfSlot,
				UpvalueRefs:   template.UpvalueRefs,
				Upvalues:      caps,
				// Annotations are per-template, not per-capture — but they
				// MUST be copied or a closure silently loses type checking
				// (same class of bug as the Variadic-copy fix above).
				Params:      template.Params,
				ParamTypes:  template.ParamTypes,
				ReturnType:  template.ReturnType,
				TypeChecked: template.TypeChecked,
			}
			stack = append(stack, closure)

		// ── Arithmetic & comparison ────────────────────────────────────
		// All routed through eval.EvalBinaryOp — the SINGLE source of
		// truth for operator semantics shared with the tree-walker. If
		// you find yourself wanting to special-case a type here, change
		// EvalBinaryOp instead so both interpreters track.
		case OpAdd:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "+")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpSub:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "-")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpMul:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "*")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpDiv:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "/")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpMod:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "%")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpEq:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "==")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpNe:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "!=")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpLt:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "<")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpLe:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, "<=")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpGt:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, ">")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpGe:
			errVal, goErr := applyBinOp(&stack, chunk, pc-1, ">=")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpNeg:
			errVal, goErr := applyUnaryOp(&stack, chunk, pc-1, "-")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
		case OpNot:
			errVal, goErr := applyUnaryOp(&stack, chunk, pc-1, "!")
			if goErr != nil {
				return nil, goErr
			}
			if errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}

		// ── Collections ───────────────────────────────────────────────
		case OpMakeArray:
			n := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			if len(stack) < n {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeArray: stack has %d, need %d", len(stack), n))
			}
			// Copy the top n into a fresh slice so the Array doesn't
			// alias the stack buffer (the stack will be reused for
			// subsequent ops).
			els := make([]eval.Object, n)
			copy(els, stack[len(stack)-n:])
			stack = stack[:len(stack)-n]
			// H3-sweep: bubble first *Error element instead of
			// wrapping it inside the Array — otherwise a downstream
			// element-consumer would mask the real error.
			if errVal := firstStackError(els); errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
			stack = append(stack, &eval.Array{Elements: els})

		case OpMakeTuple:
			n := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			if len(stack) < n {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeTuple: stack has %d, need %d", len(stack), n))
			}
			els := make([]eval.Object, n)
			copy(els, stack[len(stack)-n:])
			stack = stack[:len(stack)-n]
			// H3-sweep: bubble first *Error element.
			if errVal := firstStackError(els); errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
			stack = append(stack, &eval.Tuple{Elements: els})

		case OpMakeHash:
			pairCount := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			need := pairCount * 2
			if len(stack) < need {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeHash: stack has %d, need %d", len(stack), need))
			}
			flat := make([]eval.Object, need)
			copy(flat, stack[len(stack)-need:])
			stack = stack[:len(stack)-need]
			// H3-sweep: bubble first *Error in keys or values.
			if errVal := firstStackError(flat); errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-3)}
			result := eval.EvalMakeHash(flat, pos)
			if eval.IsError(result) {
				bubbleErrVal = result
				goto bubbleError
			}
			stack = append(stack, result)

		case OpIndex:
			if len(stack) < 2 {
				return nil, vmError(chunk, pc-1, "Index: need [container, index] on stack")
			}
			index := stack[len(stack)-1]
			container := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			// H3-sweep: bubble *Error operand first so EvalIndex
			// doesn't synthesise a misleading "index operator not
			// supported for ERROR" / "index must be integer, got
			// ERROR" message.
			if eval.IsError(container) {
				bubbleErrVal = container
				goto bubbleError
			}
			if eval.IsError(index) {
				bubbleErrVal = index
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-1)}
			result := eval.EvalIndex(container, index, pos)
			if eval.IsError(result) {
				bubbleErrVal = result
				goto bubbleError
			}
			stack = append(stack, result)

		case OpReturnIfError:
			// Stack-discipline + error-propagation in one opcode.
			// Mirrors how the tree-walker's body loop bubbles
			// Errors: every statement Eval()s to a value, the loop
			// checks isError() and returns it; otherwise the value
			// is discarded (only the LAST statement's value would
			// have been kept in the tree-walker, but the VM emits
			// an explicit Return at the end of function bodies).
			//
			// "Empty stack" for THIS FRAME means stack length is
			// back at stackBase — the caller's values below
			// stackBase don't count, they belong to a different
			// frame and must not be touched.
			if len(stack) <= stackBase {
				continue
			}
			top := stack[len(stack)-1]
			// H3-sweep OFI #1 (2026-05-23): use eval.IsError so user
			// errors from error()/safe() (IsUserError=true) stay on the
			// stack as values. Previously a raw type-assert caught
			// user errors too — divergence from tree-walker, which
			// only propagates internal errors via isError().
			if eval.IsError(top) {
				// Bubble. Pop the error off the stack, hand it to
				// the shared bubble path which mirrors OpReturn's
				// frame-pop and pushes the error onto the caller's
				// stack. The caller's next OpReturnIfError continues
				// the chain until a frame catches it (safe(), the
				// top of execute(), or `?` consumer).
				stack = stack[:len(stack)-1]
				bubbleErrVal = top
				goto bubbleError
			}
			// Normal value — discard. Statement values aren't
			// observable except for the last statement's value,
			// which the tree-walker uses but the VM doesn't yet.
			stack = stack[:len(stack)-1]

		case OpUnwrap:
			// Postfix `?` — operand must be a 2-tuple (value, err).
			// err != null → early-return from this chunk with err
			// (propagates exactly like `return err` from inside a
			// function). err == null → push value, fall through.
			// A8: stackBase-aware.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-1, "Unwrap: empty stack")
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// H3-sweep: if the operand is itself an *Error from a
			// bubbled sub-expression, propagate it untouched. Without
			// this, the tuple type-check below produces a misleading
			// "?: operand must be (value, err) tuple, got ERROR".
			if eval.IsError(top) {
				bubbleErrVal = top
				goto bubbleError
			}
			tup, ok := top.(*eval.Tuple)
			if !ok {
				// Type-mismatch — matches the tree-walker's
				// typeError() emission for the same shape.
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Message: fmt.Sprintf("?: operand must be a (value, err) tuple, got %s", top.Type()),
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
				}
				goto bubbleError
			}
			if len(tup.Elements) != 2 {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Message: fmt.Sprintf("?: expected a 2-element (value, err) tuple, got %d elements — e.g. `data = readFile(path)?` requires the function to `return value, err`", len(tup.Elements)),
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
				}
				goto bubbleError
			}
			if tup.Elements[1] != eval.NULL {
				bubbleErrVal = tup.Elements[1]
				goto bubbleError
			}
			stack = append(stack, tup.Elements[0])

		case OpUnpackTuple:
			expected := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			// A8: stackBase-aware.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-3, "UnpackTuple: empty stack")
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// H3-adjacent (audit fix, 2026-05-23): if the RHS evaluated
			// to an *Error (because the call/expr that produced it
			// bubbled mid-evaluation), pass it through unchanged. The
			// tree-walker's MultiAssignStmt/MultiLetStmt arms check
			// isError(val) and propagate; without this guard the VM
			// re-types the error as "cannot unpack ERROR into N
			// variables", which masks the real cause and trips tools
			// like `safe()` into reporting the wrong message.
			if eval.IsError(top) {
				bubbleErrVal = top
				goto bubbleError
			}
			tuple, ok := top.(*eval.Tuple)
			if !ok {
				// Type-mismatch is a kLex runtime error. We can't
				// push N values (we don't have them) and we can't
				// safely fall through to the StoreLocal loop the
				// compiler emitted next. Bubble through the frame
				// stack — OpUnwrap uses the same pattern.
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Message: fmt.Sprintf("cannot unpack %s into %d variables — right side must return multiple values", top.Type(), expected),
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
				}
				goto bubbleError
			}
			if len(tuple.Elements) != expected {
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Message: fmt.Sprintf("cannot unpack %d values into %d variables", len(tuple.Elements), expected),
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
				}
				goto bubbleError
			}
			// Push elements in source order — element 0 first, so
			// element N-1 (the last) ends up on top of the stack.
			// The compiler's StoreLocal sequence consumes them in
			// reverse, binding the LAST name to the TOP value first.
			for _, el := range tuple.Elements {
				stack = append(stack, el)
			}

		// ── Structs ───────────────────────────────────────────────────
		case OpImport:
			pathIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			aliasIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(pathIdx) >= len(chunk.Constants) || int(aliasIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-5, "Import: const idx out of range")
			}
			pathStr, pOK := chunk.Constants[pathIdx].(*eval.String)
			aliasStr, aOK := chunk.Constants[aliasIdx].(*eval.String)
			if !pOK || !aOK {
				return nil, vmError(chunk, pc-5, "Import: path/alias constants must be strings — compiler bug")
			}
			mod, errObj := eval.ImportModule(pathStr.Value, aliasStr.Value)
			if errObj != nil {
				bubbleErrVal = errObj
				goto bubbleError
			}
			stack = append(stack, mod)

		case OpMatchVariant:
			bindCount := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			if len(stack) < 2 {
				return nil, vmError(chunk, pc-3, "MatchVariant: need [pattern, subject]")
			}
			subject := stack[len(stack)-1]
			pattern := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			// H3-sweep: bubble *Error subject. Pattern is a
			// compile-time constant (EnumVariant / String) so it
			// can't be an error — only subject needs checking.
			if eval.IsError(subject) {
				bubbleErrVal = subject
				goto bubbleError
			}

			inst, isInstance := subject.(*eval.EnumInstance)
			matched := false
			if isInstance {
				switch p := pattern.(type) {
				case *eval.String:
					// Short form — match by variant name only.
					matched = inst.VariantName == p.Value
				case *eval.EnumVariant:
					matched = inst.TypeName == p.TypeName && inst.VariantName == p.VariantName
				case *eval.EnumInstance:
					matched = inst.TypeName == p.TypeName && inst.VariantName == p.VariantName
				}
			}
			if !matched {
				stack = append(stack, eval.FALSE)
				continue
			}
			// Arity check for the binding list.
			if bindCount != len(inst.FieldNames) {
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Message: fmt.Sprintf("%s.%s has %d field(s) but pattern binds %d", inst.TypeName, inst.VariantName, len(inst.FieldNames), bindCount),
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
				}
				goto bubbleError
			}
			for _, name := range inst.FieldNames {
				val, ok := inst.Fields[name]
				if !ok {
					val = eval.NULL
				}
				stack = append(stack, val)
			}
			stack = append(stack, eval.TRUE)

		case OpMakeStruct:
			fieldCount := int(binary.LittleEndian.Uint16(chunk.Code[pc:]))
			pc += 2
			need := fieldCount*2 + 1 // pairs + def
			if len(stack) < need {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("MakeStruct: stack has %d, need %d", len(stack), need))
			}
			pairsStart := len(stack) - fieldCount*2
			defObj := stack[pairsStart-1]
			def, ok := defObj.(*eval.StructDef)
			if !ok {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-3)},
					Message: fmt.Sprintf("struct literal: expected StructDef, got %s", defObj.Type()),
				}
				goto bubbleError
			}
			flat := make([]eval.Object, fieldCount*2)
			copy(flat, stack[pairsStart:])
			// Pop pairs + def.
			stack = stack[:pairsStart-1]
			// H3-sweep: bubble first *Error field value before
			// wrapping inside the StructInstance.
			if errVal := firstStackError(flat); errVal != nil {
				bubbleErrVal = errVal
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-3)}
			result := eval.EvalMakeStruct(def, flat, pos)
			if eval.IsError(result) {
				bubbleErrVal = result
				goto bubbleError
			}
			stack = append(stack, result)

		case OpGetField:
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("GetField: name const idx %d out of range", nameIdx))
			}
			nameObj, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-3, "GetField: name constant is not a String — compiler bug")
			}
			// M1: stackBase-aware so a callee frame doesn't read into
			// the caller's window when its own window is empty.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-3, "GetField: empty stack")
			}
			recv := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// H3-sweep: bubble *Error receiver first.
			if eval.IsError(recv) {
				bubbleErrVal = recv
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-3)}
			res := eval.EvalGetField(recv, nameObj.Value, pos)
			if eval.IsError(res) {
				bubbleErrVal = res
				goto bubbleError
			}
			stack = append(stack, res)

		case OpSetField:
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-3, fmt.Sprintf("SetField: name const idx %d out of range", nameIdx))
			}
			nameObj, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-3, "SetField: name constant is not a String — compiler bug")
			}
			if len(stack) < 2 {
				return nil, vmError(chunk, pc-3, "SetField: need [instance, value]")
			}
			value := stack[len(stack)-1]
			recv := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			// H3-sweep: bubble *Error operand first.
			if eval.IsError(recv) {
				bubbleErrVal = recv
				goto bubbleError
			}
			if eval.IsError(value) {
				bubbleErrVal = value
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-3)}
			res := eval.EvalSetField(recv, nameObj.Value, value, pos)
			if eval.IsError(res) {
				bubbleErrVal = res
				goto bubbleError
			}
			// Statement-level — no push.

		case OpIndexStore:
			// Stack from bottom: [container, index, value]
			if len(stack) < 3 {
				return nil, vmError(chunk, pc-1, "IndexStore: need [container, index, value]")
			}
			value := stack[len(stack)-1]
			index := stack[len(stack)-2]
			container := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			// H3-sweep: bubble *Error operand first.
			if eval.IsError(container) {
				bubbleErrVal = container
				goto bubbleError
			}
			if eval.IsError(index) {
				bubbleErrVal = index
				goto bubbleError
			}
			if eval.IsError(value) {
				bubbleErrVal = value
				goto bubbleError
			}
			pos := ast.Pos{Line: lineForOffset(chunk, pc-1)}
			res := eval.EvalIndexAssign(container, index, value, pos)
			if eval.IsError(res) {
				bubbleErrVal = res
				goto bubbleError
			}
			// Statement-level — no push.

		// ── Control flow ──────────────────────────────────────────────
		// Jump offsets are SIGNED i32, computed by the compiler as
		// `dest - (operand_end)` so adding to pc (which already points
		// past the operand by this point) yields the destination.
		// See compileIf / patchJump in compiler.go for the encoding.
		case OpJump:
			off := int32(binary.LittleEndian.Uint32(chunk.Code[pc:]))
			pc += 4
			pc += int(off)

		case OpJumpIfFalse:
			off := int32(binary.LittleEndian.Uint32(chunk.Code[pc:]))
			pc += 4
			// M1: stackBase-aware. Caller values below stackBase
			// must not be consumed as this frame's condition.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-5, "JumpIfFalse: empty stack")
			}
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// H3-sweep: bubble *Error operand instead of synthesising
			// "condition must be Bool, got ERROR".
			if eval.IsError(cond) {
				bubbleErrVal = cond
				goto bubbleError
			}
			b, ok := cond.(*eval.Boolean)
			if !ok {
				// kLex's strict-bool rule: only Booleans drive conditionals.
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-5)},
					Message: fmt.Sprintf("condition must be Bool, got %s", cond.Type()),
				}
				goto bubbleError
			}
			if !b.Value {
				pc += int(off)
			}

		// ── Short-circuit peek-jumps ──────────────────────────────────
		// Used by &&/||. Unlike JumpIfFalse, these PEEK rather than pop,
		// so the short-circuited value remains on the stack as the
		// expression's result.
		case OpJumpIfFalsePeek:
			off := int32(binary.LittleEndian.Uint32(chunk.Code[pc:]))
			pc += 4
			// M1: stackBase-aware. Caller values below stackBase
			// must not be peeked as this frame's short-circuit operand.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-5, "JumpIfFalsePeek: empty stack")
			}
			peek := stack[len(stack)-1]
			// H3-sweep: bubble *Error operand. Pop it (don't leave on
			// stack) since the && expression has no meaningful value
			// to return when its left operand is itself an error.
			if eval.IsError(peek) {
				stack = stack[:len(stack)-1]
				bubbleErrVal = peek
				goto bubbleError
			}
			b, ok := peek.(*eval.Boolean)
			if !ok {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-5)},
					Message: fmt.Sprintf("&& operand must be Bool, got %s", peek.Type()),
				}
				goto bubbleError
			}
			if !b.Value {
				pc += int(off)
			}

		case OpJumpIfTruePeek:
			off := int32(binary.LittleEndian.Uint32(chunk.Code[pc:]))
			pc += 4
			// M1: stackBase-aware. Caller values below stackBase
			// must not be peeked as this frame's short-circuit operand.
			if len(stack) <= stackBase {
				return nil, vmError(chunk, pc-5, "JumpIfTruePeek: empty stack")
			}
			peek := stack[len(stack)-1]
			// H3-sweep: bubble *Error operand (see OpJumpIfFalsePeek).
			if eval.IsError(peek) {
				stack = stack[:len(stack)-1]
				bubbleErrVal = peek
				goto bubbleError
			}
			b, ok := peek.(*eval.Boolean)
			if !ok {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-5)},
					Message: fmt.Sprintf("|| operand must be Bool, got %s", peek.Type()),
				}
				goto bubbleError
			}
			if b.Value {
				pc += int(off)
			}

		case OpCall:
			// argc args on top, callable just below them.
			argc := int(chunk.Code[pc])
			pc++
			needed := argc + 1
			if len(stack)-stackBase < needed {
				return nil, vmError(chunk, pc-2, fmt.Sprintf("Call: stack has %d, need %d (1 callee + %d args)", len(stack)-stackBase, needed, argc))
			}
			calleeIdx := len(stack) - needed
			callee := stack[calleeIdx]

			// P5 (audit perf): check *CompiledFunction FIRST — the
			// common case is a VM-compiled function recursing on the
			// hot path. Skipping two failed type-asserts per call
			// saves ~5ns on the chain workload. The other arms
			// (tree-walker *Function from imports, *EnumVariant
			// constructor) only fire on cross-interp boundaries which
			// don't dominate.
			cf, ok := callee.(*CompiledFunction)
			if !ok {
				// Slow path: not a VM-compiled function.
				if _, isEvalFn := callee.(*eval.Function); isEvalFn {
					// Tree-walker function: callee was loaded from an
					// imported module (the module was evaluated by
					// the tree-walker, so its top-level fn bindings
					// are *eval.Function values). Dispatch via
					// eval.CallCallable — same code the tree-walker
					// runs internally.
					args := make([]eval.Object, argc)
					copy(args, stack[calleeIdx+1:])
					stack = stack[:calleeIdx]
					result, errObj := eval.CallCallable(callee, args)
					if errObj != nil {
						// H3: route through bubbleError so the unwind
						// happens immediately. The previous "push then
						// continue" pattern only worked when the next
						// op was OpReturnIfError — in expression
						// context (`let x = m.f() + 1`) the next op
						// consumed the *Error as an operand and
						// reported a downstream type error that
						// masked the real cause.
						bubbleErrVal = errObj
						goto bubbleError
					}
					if result == nil {
						result = eval.NULL
					}
					stack = append(stack, result)
					continue
				}
				if bi, ok := callee.(*eval.Builtin); ok {
					// Builtin used as a value (`f = trim; f("x")`) lands here:
					// compileIdent emits OpPushConst<Builtin> when the ident
					// resolves to a Go-side builtin, and OpCall sees that
					// pointer on the stack. Without this arm we fell through
					// to the "callee must be a compiled function, got BUILTIN"
					// error. Pattern mirrors OpCallMethod's fallback arm.
					//
					// scriptDir intercept: parity with OpCallBuiltin
					// (~vm.go:2073). Tree-walker walks the env chain to
					// resolve the script directory; the VM has no env chain
					// at runtime, so it reads chunk.ScriptDir directly.
					// Without this, `f = _scriptDir; f()` would return the
					// builtin's placeholder "" instead of the script dir.
					if argc == 0 && bi == scriptDirBuiltinPtr {
						stack = stack[:calleeIdx]
						stack = append(stack, &eval.String{Value: chunk.ScriptDir})
						continue
					}
					args := make([]eval.Object, argc)
					copy(args, stack[calleeIdx+1:])
					stack = stack[:calleeIdx]
					result := bi.Fn(args)
					if result == nil {
						result = eval.NULL
					}
					// H3-adjacent: a builtin's Fn may return an *Error
					// (e.g. sqrt("hi")). Pushing it raw lets the next
					// op consume it as a value and mask the real cause.
					// IsError gates on internal-only errors so user
					// errors from error()/safe() stay on the stack.
					if eval.IsError(result) {
						bubbleErrVal = result
						goto bubbleError
					}
					stack = append(stack, result)
					continue
				}
				if ev, evOk := callee.(*eval.EnumVariant); evOk {
					// Enum variant constructor: `Shape.Circle(r)`.
					// Build an EnumInstance with positional args
					// mapped to the variant's field names. Mirrors
					// the tree-walker's evalCall arm.
					if argc != len(ev.Fields) {
						bubbleErrVal = &eval.Error{
							Kind:    eval.RuntimeErr,
							Pos:     ast.Pos{Line: lineForOffset(chunk, pc-2)},
							Message: fmt.Sprintf("enum variant %s.%s expects %d fields, got %d", ev.TypeName, ev.VariantName, len(ev.Fields), argc),
						}
						goto bubbleError
					}
					fields := make(map[string]eval.Object, len(ev.Fields))
					for i, name := range ev.Fields {
						fields[name] = stack[calleeIdx+1+i]
					}
					stack = stack[:calleeIdx]
					stack = append(stack, &eval.EnumInstance{
						TypeName:    ev.TypeName,
						VariantName: ev.VariantName,
						FieldNames:  ev.Fields,
						Fields:      fields,
					})
					continue
				}
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-2)},
					Message: fmt.Sprintf("callee must be a compiled function, got %s", callee.Type()),
				}
				goto bubbleError
			}
			if msg := validateArity(cf, argc); msg != "" {
				bubbleErrVal = &eval.Error{
					Kind:    eval.RuntimeErr,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-2)},
					Message: msg,
				}
				goto bubbleError
			}
			// Lazy-acquire the frames slice on the first push of
			// this execute() invocation. Grow on demand for
			// recursive workloads. The pool keeps the grown
			// slice across pool round-trips, so deep-recursing
			// programs only pay growth cost once per process.
			if frames == nil {
				frames = *framesPool.Get().(*[]callFrame)
				if cap(frames) < 16 {
					frames = make([]callFrame, 16)
				}
				frames = frames[:cap(frames)]
			}
			if frameCount >= len(frames) {
				newSize := len(frames) * 2
				if newSize > 16384 {
					bubbleErrVal = &eval.Error{
						Kind:    eval.RuntimeErr,
						Pos:     ast.Pos{Line: lineForOffset(chunk, pc-2)},
						Message: "call stack overflow",
					}
					goto bubbleError
				}
				grown := make([]callFrame, newSize)
				copy(grown, frames)
				frames = grown
			}
			// Build the callee's local cells from the pool. Each
			// slot gets its own fresh cell — captures (if any)
			// reach back via the closure's Upvalues slice, NOT
			// through stale locals. The pool reuse + Captured-flag
			// scheme keeps memory traffic flat for the common case
			// (no closures created inside the callee).
			calleeLocals := acquireLocals(cf.Chunk.NumLocals)
			if e := bindStackArgs(cf, calleeLocals, stack, calleeIdx+1, argc, ast.Pos{Line: lineForOffset(chunk, pc-2)}); e != nil {
				// Annotation violation — release the freshly-acquired
				// (never-pushed, uncaptured) locals back to the pool and
				// bubble the type error.
				releaseLocals(calleeLocals)
				bubbleErrVal = e
				goto bubbleError
			}
			// Recursion support: if the callee was compiled with a
			// self-slot, put the CompiledFunction itself into that
			// slot so LoadLocal of the function's own name inside
			// the body resolves to the function.
			if cf.SelfSlot >= 0 && cf.SelfSlot < len(calleeLocals) {
				calleeLocals[cf.SelfSlot].Value = cf
			}
			// Pop callable + args.
			stack = stack[:calleeIdx]
			// FLAT DISPATCH: instead of recursively calling
			// execute() (which paid a Go function-call + defer-
			// setup per kLex function call — millions of times on
			// 1 M-element chains), save the caller's state into the
			// in-loop frames array and switch the dispatch's local
			// variables to the callee. The matching OpReturn pops
			// the frame and restores caller state. cf.Upvalues was
			// populated by OpMakeClosure (empty for plain non-
			// closure functions); upvalue mutations are visible to
			// the caller because cells are shared by pointer.
			//
			// stackBase is len(stack) AT THIS POINT — right after
			// the callable + args have been popped. The callee's
			// stack window starts here; anything below belongs
			// to a caller frame and must not be touched.
			frames[frameCount] = callFrame{
				chunk:     chunk,
				locals:    locals,
				upvalues:  upvalues,
				pc:        pc,
				stackBase: stackBase,
			}
			frameCount++
			chunk = cf.Chunk
			locals = calleeLocals
			upvalues = cf.Upvalues
			pc = 0
			stackBase = len(stack)

		case OpCallMethod:
			// `receiver.name(args)` dispatch — the runtime decides
			// between method dispatch and the fallback property-fetch-
			// then-call based on the receiver's actual type. Args + 1
			// receiver are on the stack; the receiver sits just below
			// the args.
			nameIdx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			argc := int(chunk.Code[pc])
			pc++
			needed := argc + 1
			// A8: stackBase-aware to match OpCall's check.
			if len(stack)-stackBase < needed {
				return nil, vmError(chunk, pc-4, fmt.Sprintf("CallMethod: stack window has %d, need %d", len(stack)-stackBase, needed))
			}
			if int(nameIdx) >= len(chunk.Constants) {
				return nil, vmError(chunk, pc-4, "CallMethod: name idx out of range")
			}
			nameObj, ok := chunk.Constants[nameIdx].(*eval.String)
			if !ok {
				return nil, vmError(chunk, pc-4, "CallMethod: name const slot must hold *String")
			}
			methodName := nameObj.Value
			recvIdx := len(stack) - needed
			receiver := stack[recvIdx]
			callPos := ast.Pos{Line: lineForOffset(chunk, pc-4)}

			// ─── Method dispatch on a struct instance ─────────────
			if inst, ok := receiver.(*eval.StructInstance); ok && inst.Def != nil {
				// 1. VM-compiled method (most common case)
				if inst.Def.MethodsAny != nil {
					if mAny, found := inst.Def.MethodsAny[methodName]; found {
						if cf, ok := mAny.(*CompiledFunction); ok {
							// self + user args = argc+1. Method's
							// NumParams already counts self. validateArity
							// covers both fixed and variadic methods.
							if msg := validateArity(cf, argc+1); msg != "" {
								// Reformat from "fn:X expects ..." to
								// "Struct.method expects ..." so the
								// method dispatch surface stays clean.
								required := cf.NumParams - 1
								if cf.Variadic {
									required = cf.NumParams - 2 // minus rest-param too
									bubbleErrVal = &eval.Error{
										Kind:    eval.RuntimeErr,
										Pos:     callPos,
										Message: fmt.Sprintf("%s.%s expects at least %d arguments, got %d", inst.Def.Name, methodName, required, argc),
									}
								} else {
									bubbleErrVal = &eval.Error{
										Kind:    eval.RuntimeErr,
										Pos:     callPos,
										Message: fmt.Sprintf("%s.%s expects %d arguments, got %d", inst.Def.Name, methodName, required, argc),
									}
								}
								goto bubbleError
							}
							// FLAT DISPATCH (A2 unification): mirror
							// OpCall's CompiledFunction path — push the
							// caller's frame onto the in-loop frames
							// stack, switch the dispatch's locals to
							// the callee, let OpReturn pop back. This
							// removes one of the three remaining
							// Go-level execute() recursion sites. See
							// OpCall arm above for the canonical
							// pattern; this site differs only in the
							// receiver-as-slot-0 self binding.
							if frames == nil {
								frames = *framesPool.Get().(*[]callFrame)
								if cap(frames) < 16 {
									frames = make([]callFrame, 16)
								}
								frames = frames[:cap(frames)]
							}
							if frameCount >= len(frames) {
								newSize := len(frames) * 2
								if newSize > 16384 {
									bubbleErrVal = &eval.Error{
										Kind:    eval.RuntimeErr,
										Pos:     callPos,
										Message: "call stack overflow",
									}
									goto bubbleError
								}
								grown := make([]callFrame, newSize)
								copy(grown, frames)
								frames = grown
							}
							calleeLocals := acquireLocals(cf.Chunk.NumLocals)
							calleeLocals[0].Value = receiver
							if e := bindMethodArgs(cf, calleeLocals, stack, recvIdx+1, argc, callPos); e != nil {
								releaseLocals(calleeLocals)
								bubbleErrVal = e
								goto bubbleError
							}
							stack = stack[:recvIdx]
							frames[frameCount] = callFrame{
								chunk:     chunk,
								locals:    locals,
								upvalues:  upvalues,
								pc:        pc,
								stackBase: stackBase,
							}
							frameCount++
							chunk = cf.Chunk
							locals = calleeLocals
							upvalues = cf.Upvalues
							pc = 0
							stackBase = len(stack)
							continue
						}
					}
				}
				// 2. Tree-walker-compiled method — dispatch through
				//    the ExternalCallable / CallCallable hook with
				//    self prepended, same as evalCall handles it.
				//
				// H2 (audit fix, 2026-05-22): the wrapped *eval.Function
				// (with self prepended) is now cached on the
				// StructDef via MethodWithSelf — built once on first
				// dispatch, reused forever. Was allocating 3 heap
				// objects per call (the Function + two appended
				// slices); now the per-call cost is the args slice
				// alone.
				if inst.Def.Methods != nil {
					if adapted := inst.Def.MethodWithSelf(methodName); adapted != nil {
						args := make([]eval.Object, argc+1)
						args[0] = receiver
						for i := 0; i < argc; i++ {
							args[i+1] = stack[recvIdx+1+i]
						}
						stack = stack[:recvIdx]
						result, errObj := eval.CallCallable(adapted, args)
						if errObj != nil {
							// H3: route through bubbleError so the
							// unwind happens immediately. See OpCall
							// arm for the rationale.
							bubbleErrVal = errObj
							goto bubbleError
						}
						if result == nil {
							result = eval.NULL
						}
						stack = append(stack, result)
						continue
					}
				}
				// 3. Not a method — must be a field that holds a
				//    callable. Fall through to property-fetch path.
			}

			// ─── Fallback: property-fetch + call ──────────────────
			// Covers module.func(args), hash["k"](args), and the
			// "method name is actually a field holding a callable"
			// case for structs.
			fetched := eval.EvalGetField(receiver, methodName, callPos)
			if eval.IsError(fetched) {
				// H3: pop the recv window then route through
				// bubbleError so the next op can't consume the
				// Error as a value.
				stack = stack[:recvIdx]
				bubbleErrVal = fetched
				goto bubbleError
			}
			// Re-shape the stack so the fallback resembles a normal
			// call: replace the receiver with the fetched callable
			// (args remain on top of it).
			stack[recvIdx] = fetched
			// Dispatch the same way OpCall would.
			callee := fetched
			calleeIdxFB := recvIdx
			// Enum variant constructor: `Shape.Circle(r)`. Build an
			// EnumInstance with positional args mapped to the
			// variant's field names. Same shape as OpCall's enum arm.
			if ev, ok := callee.(*eval.EnumVariant); ok {
				if argc != len(ev.Fields) {
					bubbleErrVal = &eval.Error{
						Kind:    eval.RuntimeErr,
						Pos:     callPos,
						Message: fmt.Sprintf("enum variant %s.%s expects %d fields, got %d", ev.TypeName, ev.VariantName, len(ev.Fields), argc),
					}
					goto bubbleError
				}
				fields := make(map[string]eval.Object, len(ev.Fields))
				for i, name := range ev.Fields {
					fields[name] = stack[calleeIdxFB+1+i]
				}
				stack = stack[:calleeIdxFB]
				stack = append(stack, &eval.EnumInstance{
					TypeName:    ev.TypeName,
					VariantName: ev.VariantName,
					FieldNames:  ev.Fields,
					Fields:      fields,
				})
				continue
			}
			if _, isEvalFn := callee.(*eval.Function); isEvalFn {
				args := make([]eval.Object, argc)
				copy(args, stack[calleeIdxFB+1:])
				stack = stack[:calleeIdxFB]
				result, errObj := eval.CallCallable(callee, args)
				if errObj != nil {
					// H3: route through bubbleError. See OpCall arm.
					bubbleErrVal = errObj
					goto bubbleError
				}
				if result == nil {
					result = eval.NULL
				}
				stack = append(stack, result)
				continue
			}
			if bi, ok := callee.(*eval.Builtin); ok {
				args := make([]eval.Object, argc)
				copy(args, stack[calleeIdxFB+1:])
				stack = stack[:calleeIdxFB]
				result := bi.Fn(args)
				if result == nil {
					result = eval.NULL
				}
				// H3-adjacent: builtin may return *Error directly.
				if eval.IsError(result) {
					bubbleErrVal = result
					goto bubbleError
				}
				stack = append(stack, result)
				continue
			}
			if cf, ok := callee.(*CompiledFunction); ok {
				if msg := validateArity(cf, argc); msg != "" {
					bubbleErrVal = &eval.Error{
						Kind:    eval.RuntimeErr,
						Pos:     callPos,
						Message: msg,
					}
					goto bubbleError
				}
				// FLAT DISPATCH (A2 unification): mirror OpCall's
				// CompiledFunction path. Removes the second of three
				// Go-level execute() recursion sites. See OpCall arm
				// for the canonical version.
				if frames == nil {
					frames = *framesPool.Get().(*[]callFrame)
					if cap(frames) < 16 {
						frames = make([]callFrame, 16)
					}
					frames = frames[:cap(frames)]
				}
				if frameCount >= len(frames) {
					newSize := len(frames) * 2
					if newSize > 16384 {
						bubbleErrVal = &eval.Error{
							Kind:    eval.RuntimeErr,
							Pos:     callPos,
							Message: "call stack overflow",
						}
						goto bubbleError
					}
					grown := make([]callFrame, newSize)
					copy(grown, frames)
					frames = grown
				}
				calleeLocals := acquireLocals(cf.Chunk.NumLocals)
				if e := bindStackArgs(cf, calleeLocals, stack, calleeIdxFB+1, argc, callPos); e != nil {
					releaseLocals(calleeLocals)
					bubbleErrVal = e
					goto bubbleError
				}
				if cf.SelfSlot >= 0 && cf.SelfSlot < len(calleeLocals) {
					calleeLocals[cf.SelfSlot].Value = cf
				}
				stack = stack[:calleeIdxFB]
				frames[frameCount] = callFrame{
					chunk:     chunk,
					locals:    locals,
					upvalues:  upvalues,
					pc:        pc,
					stackBase: stackBase,
				}
				frameCount++
				chunk = cf.Chunk
				locals = calleeLocals
				upvalues = cf.Upvalues
				pc = 0
				stackBase = len(stack)
				continue
			}
			bubbleErrVal = &eval.Error{
				Kind:    eval.TypeError,
				Pos:     callPos,
				Message: fmt.Sprintf("%s.%s: not callable (got %s)", receiver.Type(), methodName, callee.Type()),
			}
			goto bubbleError

		// ── Higher-order intrinsics ─────────────────────────────
		// A2 audit cleanup (2026-05-22): OpMap / OpFilter / OpReduce
		// now drive their iteration through the FLAT DISPATCH loop
		// rather than recursing into execute() per element via the
		// old callCompiledOnce / callCompiledTwice helpers. The iter
		// frame is pushed for the *CompiledFunction NumParams=1 (or
		// =2 for reduce) fast path; OpReturn's iter-aware arm runs
		// the per-iteration step on each return. The generic
		// fallback (eval.CallCallable for *Function / *Builtin
		// callbacks) stays as-is — it has no Go-recursion to
		// eliminate (the call goes through the hook, not execute()).
		case OpMap:
			if len(stack)-stackBase < 2 {
				return nil, vmError(chunk, pc-1, "Map: stack window has fewer than 2 values")
			}
			mapFn := stack[len(stack)-1]
			mapArrObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			mapArr, mapOk := mapArrObj.(*eval.Array)
			if !mapOk {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
					Message: fmt.Sprintf("map: first argument must be array, got %s", mapArrObj.Type()),
				}
				goto bubbleError
			}
			// !mapCF.TypeChecked: an annotated callback skips the flat-
			// dispatch fast path and takes the generic eval.CallCallable
			// fallback below, which enforces the callback's param/return
			// annotations exactly like the tree-walker's map builtin. The
			// intrinsic binds elements straight into locals and would
			// otherwise bypass the check — this keeps TW↔VM parity.
			if mapCF, isCF := mapFn.(*CompiledFunction); isCF && mapCF.NumParams == 1 && !mapCF.Variadic && !mapCF.TypeChecked {
				elements := mapArr.Elements
				if len(elements) == 0 {
					stack = append(stack, &eval.Array{Elements: []eval.Object{}})
					continue
				}
				if frames == nil {
					frames = *framesPool.Get().(*[]callFrame)
					if cap(frames) < 16 {
						frames = make([]callFrame, 16)
					}
					frames = frames[:cap(frames)]
				}
				if frameCount >= len(frames) {
					newSize := len(frames) * 2
					if newSize > 16384 {
						bubbleErrVal = &eval.Error{
							Kind:    eval.RuntimeErr,
							Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
							Message: "call stack overflow",
						}
						goto bubbleError
					}
					grown := make([]callFrame, newSize)
					copy(grown, frames)
					frames = grown
				}
				calleeLocals := acquireLocals(mapCF.Chunk.NumLocals)
				calleeLocals[0].Value = elements[0]
				if mapCF.SelfSlot >= 0 && mapCF.SelfSlot < len(calleeLocals) {
					calleeLocals[mapCF.SelfSlot].Value = mapCF
				}
				frames[frameCount] = callFrame{
					chunk:     chunk,
					locals:    locals,
					upvalues:  upvalues,
					pc:        pc,
					stackBase: stackBase,
					iter: &iterState{
						mode:  OpMap,
						elems: elements,
						fn:    mapCF,
						out:   make([]eval.Object, len(elements)),
						opPC:  pc - 1,
					},
				}
				frameCount++
				chunk = mapCF.Chunk
				locals = calleeLocals
				upvalues = mapCF.Upvalues
				pc = 0
				stackBase = len(stack)
				continue
			}
			// Generic fallback: tree-walker *Function / *Builtin.
			// H3 (audit fix, 2026-05-22): reuse a single 1-element
			// arg slice across the loop. eval.CallCallable doesn't
			// retain args, so the same backing array is safe to
			// rewrite per element. Saves N slice-header + N 1-elem
			// backing-array allocs on the generic path (~16 bytes
			// each, so ~16MB on a 1M-element chain).
			mapOut := make([]eval.Object, len(mapArr.Elements))
			var mapErrVal eval.Object
			var argBuf [1]eval.Object
			argSlice := argBuf[:]
			for i, el := range mapArr.Elements {
				argSlice[0] = el
				result, errObj := eval.CallCallable(mapFn, argSlice)
				if errObj != nil {
					mapErrVal = errObj
					break
				}
				mapOut[i] = result
			}
			if mapErrVal != nil {
				// H3: route through bubbleError. See OpCall arm.
				bubbleErrVal = mapErrVal
				goto bubbleError
			}
			stack = append(stack, &eval.Array{Elements: mapOut})

		case OpFilter:
			if len(stack)-stackBase < 2 {
				return nil, vmError(chunk, pc-1, "Filter: stack window has fewer than 2 values")
			}
			filFn := stack[len(stack)-1]
			filArrObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			filArr, filOk := filArrObj.(*eval.Array)
			if !filOk {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
					Message: fmt.Sprintf("filter: first argument must be array, got %s", filArrObj.Type()),
				}
				goto bubbleError
			}
			// !filCF.TypeChecked: annotated callbacks take the generic
			// fallback so their annotations are enforced — see OpMap.
			if filCF, isCF := filFn.(*CompiledFunction); isCF && filCF.NumParams == 1 && !filCF.Variadic && !filCF.TypeChecked {
				elements := filArr.Elements
				if len(elements) == 0 {
					stack = append(stack, &eval.Array{Elements: []eval.Object{}})
					continue
				}
				if frames == nil {
					frames = *framesPool.Get().(*[]callFrame)
					if cap(frames) < 16 {
						frames = make([]callFrame, 16)
					}
					frames = frames[:cap(frames)]
				}
				if frameCount >= len(frames) {
					newSize := len(frames) * 2
					if newSize > 16384 {
						bubbleErrVal = &eval.Error{
							Kind:    eval.RuntimeErr,
							Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
							Message: "call stack overflow",
						}
						goto bubbleError
					}
					grown := make([]callFrame, newSize)
					copy(grown, frames)
					frames = grown
				}
				calleeLocals := acquireLocals(filCF.Chunk.NumLocals)
				calleeLocals[0].Value = elements[0]
				if filCF.SelfSlot >= 0 && filCF.SelfSlot < len(calleeLocals) {
					calleeLocals[filCF.SelfSlot].Value = filCF
				}
				frames[frameCount] = callFrame{
					chunk:     chunk,
					locals:    locals,
					upvalues:  upvalues,
					pc:        pc,
					stackBase: stackBase,
					iter: &iterState{
						mode:  OpFilter,
						elems: elements,
						fn:    filCF,
						out:   make([]eval.Object, 0, len(elements)),
						opPC:  pc - 1,
					},
				}
				frameCount++
				chunk = filCF.Chunk
				locals = calleeLocals
				upvalues = filCF.Upvalues
				pc = 0
				stackBase = len(stack)
				continue
			}
			// Generic fallback. H3: reuse single 1-elem arg slice.
			filOut := make([]eval.Object, 0, len(filArr.Elements))
			var filErrVal eval.Object
			var filArgBuf [1]eval.Object
			filArgSlice := filArgBuf[:]
			for _, el := range filArr.Elements {
				filArgSlice[0] = el
				result, errObj := eval.CallCallable(filFn, filArgSlice)
				if errObj != nil {
					filErrVal = errObj
					break
				}
				b, isBool := result.(*eval.Boolean)
				if !isBool {
					bubbleErrVal = &eval.Error{
						Kind:    eval.TypeError,
						Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
						Message: fmt.Sprintf("filter: function must return bool, got %s", result.Type()),
					}
					goto bubbleError
				}
				if b.Value {
					filOut = append(filOut, el)
				}
			}
			if filErrVal != nil {
				// H3: route through bubbleError. See OpCall arm.
				bubbleErrVal = filErrVal
				goto bubbleError
			}
			stack = append(stack, &eval.Array{Elements: filOut})

		case OpReduce:
			if len(stack)-stackBase < 3 {
				return nil, vmError(chunk, pc-1, "Reduce: stack window has fewer than 3 values")
			}
			redInit := stack[len(stack)-1]
			redFn := stack[len(stack)-2]
			redArrObj := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			redArr, redOk := redArrObj.(*eval.Array)
			if !redOk {
				bubbleErrVal = &eval.Error{
					Kind:    eval.TypeError,
					Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
					Message: fmt.Sprintf("reduce: first argument must be array, got %s", redArrObj.Type()),
				}
				goto bubbleError
			}
			// !redCF.TypeChecked: annotated callbacks take the generic
			// fallback so their annotations are enforced — see OpMap.
			if redCF, isCF := redFn.(*CompiledFunction); isCF && redCF.NumParams == 2 && !redCF.Variadic && !redCF.TypeChecked {
				elements := redArr.Elements
				if len(elements) == 0 {
					stack = append(stack, redInit)
					continue
				}
				if frames == nil {
					frames = *framesPool.Get().(*[]callFrame)
					if cap(frames) < 16 {
						frames = make([]callFrame, 16)
					}
					frames = frames[:cap(frames)]
				}
				if frameCount >= len(frames) {
					newSize := len(frames) * 2
					if newSize > 16384 {
						bubbleErrVal = &eval.Error{
							Kind:    eval.RuntimeErr,
							Pos:     ast.Pos{Line: lineForOffset(chunk, pc-1)},
							Message: "call stack overflow",
						}
						goto bubbleError
					}
					grown := make([]callFrame, newSize)
					copy(grown, frames)
					frames = grown
				}
				calleeLocals := acquireLocals(redCF.Chunk.NumLocals)
				calleeLocals[0].Value = redInit
				calleeLocals[1].Value = elements[0]
				if redCF.SelfSlot >= 0 && redCF.SelfSlot < len(calleeLocals) {
					calleeLocals[redCF.SelfSlot].Value = redCF
				}
				frames[frameCount] = callFrame{
					chunk:     chunk,
					locals:    locals,
					upvalues:  upvalues,
					pc:        pc,
					stackBase: stackBase,
					iter: &iterState{
						mode:  OpReduce,
						elems: elements,
						fn:    redCF,
						acc:   redInit,
						opPC:  pc - 1,
					},
				}
				frameCount++
				chunk = redCF.Chunk
				locals = calleeLocals
				upvalues = redCF.Upvalues
				pc = 0
				stackBase = len(stack)
				continue
			}
			// Generic fallback. H3: reuse single 2-elem arg slice
			// (acc, el). eval.CallCallable doesn't retain args.
			acc := redInit
			var redErrVal eval.Object
			var redArgBuf [2]eval.Object
			redArgSlice := redArgBuf[:]
			for _, el := range redArr.Elements {
				redArgSlice[0] = acc
				redArgSlice[1] = el
				result, errObj := eval.CallCallable(redFn, redArgSlice)
				if errObj != nil {
					redErrVal = errObj
					break
				}
				acc = result
			}
			if redErrVal != nil {
				// H3: route through bubbleError. See OpCall arm.
				bubbleErrVal = redErrVal
				goto bubbleError
			}
			stack = append(stack, acc)

		case OpCallBuiltin:
			idx := binary.LittleEndian.Uint16(chunk.Code[pc:])
			pc += 2
			argc := int(chunk.Code[pc])
			pc++
			if int(idx) >= NumBuiltins {
				return nil, vmError(chunk, pc-4, fmt.Sprintf("CallBuiltin: builtin index %d out of range (NumBuiltins=%d)", idx, NumBuiltins))
			}
			// M5: argc check is per-frame — a callee with stackBase>0
			// must not read args from below its own window.
			if len(stack)-stackBase < argc {
				return nil, vmError(chunk, pc-4, fmt.Sprintf("CallBuiltin: stack has %d values in this frame, need %d args", len(stack)-stackBase, argc))
			}
			// _scriptDir intercept: the tree-walker special-cases
			// this builtin inside evalCall to walk the env chain
			// for the script's directory; the registered Fn is a
			// placeholder that returns "". The VM has no env
			// chain, so we read the directory from the entry
			// chunk and bypass the placeholder. Top-level
			// `_scriptDir()` in a VM-compiled script now matches
			// tree-walker behaviour (returning the script's
			// directory rather than empty string).
			if scriptDirBuiltinResolved && idx == scriptDirBuiltinIdx && argc == 0 {
				stack = append(stack, &eval.String{Value: chunk.ScriptDir})
				continue
			}
			// Args were pushed in order, so the bottom of the
			// argc-window is arg[0]. M4 (audit fix, 2026-05-22):
			// the *eval.Builtin.RetainsArgs annotation now exists
			// and tags `async`, but the pooled-args optimisation
			// was reverted because sync.Pool under 8-goroutine
			// contention regressed the simple benchmark ~18%
			// (same trap as A5). make() per call wins under heavy
			// concurrent load; the annotation stays as
			// documentation of the retention contract.
			argStart := len(stack) - argc
			args := make([]eval.Object, argc)
			copy(args, stack[argStart:])
			stack = stack[:argStart]
			result := BuiltinTable[idx].Fn(args)
			if result == nil {
				result = eval.NULL
			}
			// H3-adjacent: builtin's Fn may return an *Error directly
			// (e.g. sqrt("hi"), 1/0 surfaced via a builtin). Pushing
			// raw lets the next op consume it and mask the real cause.
			if eval.IsError(result) {
				bubbleErrVal = result
				goto bubbleError
			}
			stack = append(stack, result)

		default:
			return nil, vmError(chunk, pc-1, fmt.Sprintf("opcode %s not implemented", op))
		}
		continue

	bubbleError:
		// Reached when a mid-op kLex error was set in bubbleErrVal
		// via goto. Mirrors OpReturn's frame-pop logic but with
		// the error value instead of a return value. Caller's next
		// opcode is typically OpReturnIfError (between statements)
		// which catches the error and triggers another bubble pop
		// — the chain continues until the error reaches frameCount
		// 0 or a caller that handles it (e.g. safe()).
		//
		// H3 (audit fix): when popping lands on a frame whose `iter`
		// is non-nil, that frame is an in-progress OpMap / OpFilter /
		// OpReduce intrinsic driver. Its saved pc points PAST the
		// intrinsic opcode, so pushing the error there leaves it for
		// the next op to consume as an operand — the exact masking
		// the audit caught. Keep bubbling until we hit a non-iter
		// frame (or frameCount 0) so the error surfaces somewhere a
		// real OpReturnIfError can route it.
		//
		// Agentic hook (2026-05-23): if a user has registered an
		// error-bubble hook via _setErrorHook(...), fire it ONCE per
		// error here — at the FIRST bubbleError entry, before any
		// frames pop. The hook receives an event hash describing the
		// failure and runs synchronously in the caller's goroutine.
		// Re-entry guard inside eval.FireErrorBubbleHook prevents
		// infinite recursion when the hook itself errors. Cost when
		// no hook is registered: one atomic pointer load + nil check.
		if errObj, ok := bubbleErrVal.(*eval.Error); ok {
			eval.FireErrorBubbleHook(errObj, buildCallStackLines(chunk, pc, frames, frameCount))
		}
		for {
			if frameCount == 0 {
				return bubbleErrVal, nil
			}
			stack = stack[:stackBase]
			releaseLocals(locals)
			frameCount--
			wasIterDriver := frames[frameCount].iter != nil
			chunk = frames[frameCount].chunk
			locals = frames[frameCount].locals
			upvalues = frames[frameCount].upvalues
			pc = frames[frameCount].pc
			stackBase = frames[frameCount].stackBase
			frames[frameCount] = callFrame{}
			if wasIterDriver {
				continue
			}
			stack = append(stack, bubbleErrVal)
			bubbleErrVal = nil
			break
		}
	}

	// Fell off the end without hitting Halt — treat tos as the
	// result. Compiler emits Halt at end of Program so this path
	// shouldn't fire today, but it keeps us robust against
	// hand-crafted chunks.
	if len(stack) == 0 {
		return eval.NULL, nil
	}
	return stack[len(stack)-1], nil
}

// applyBinOp pops the top two stack values (right then left), calls
// eval.EvalBinaryOp with the chunk's line for the dispatching opcode,
// and pushes the result. Return values discriminate the three
// outcomes:
//
//	(nil,    nil)  — success; result pushed onto stack.
//	(*Error, nil)  — kLex error from EvalBinaryOp; caller bubbles
//	                 via bubbleError so per-frame locals release
//	                 correctly (matches tree-walker isError() flow).
//	(nil,    err)  — Go-level error (compiler emitted unbalanced
//	                 bytecode); caller halts execute() via
//	                 `return nil, err`. P6: replaces the previous
//	                 panics so a compiler bug halts cleanly instead
//	                 of taking down the process.
//
// stack is taken by pointer because we mutate it in-place; that
// keeps the per-op dispatch cost down to the minimum the Go runtime
// can deliver. *Important*: in-place pop must happen BEFORE
// EvalBinaryOp is called — if the op errors and we leave the
// operands on the stack, a recovered VM would see them as garbage.
func applyBinOp(stack *[]eval.Object, chunk *BytecodeChunk, codeOffset int, op string) (eval.Object, error) {
	s := *stack
	if len(s) < 2 {
		return nil, vmError(chunk, codeOffset, fmt.Sprintf("%s: need 2 operands on stack, have %d — compiler emitted unbalanced bytecode", op, len(s)))
	}
	right := s[len(s)-1]
	left := s[len(s)-2]
	s = s[:len(s)-2]
	// H3-sweep (2026-05-23): bubble *Error operands instead of letting
	// EvalBinaryOp synthesise "operator X not defined for ERROR and Y",
	// which masks the real error from whichever sub-expression bubbled.
	// Left checked first so left-bubbled errors take precedence.
	if eval.IsError(left) {
		*stack = s
		return left, nil
	}
	if eval.IsError(right) {
		*stack = s
		return right, nil
	}
	pos := ast.Pos{Line: lineForOffset(chunk, codeOffset)}
	result := eval.EvalBinaryOp(left, right, op, pos)
	if eval.IsError(result) {
		*stack = s
		return result, nil
	}
	s = append(s, result)
	*stack = s
	return nil, nil
}

// applyUnaryOp — prefix-operator equivalent. Same return contract
// as applyBinOp.
func applyUnaryOp(stack *[]eval.Object, chunk *BytecodeChunk, codeOffset int, op string) (eval.Object, error) {
	s := *stack
	if len(s) == 0 {
		return nil, vmError(chunk, codeOffset, fmt.Sprintf("%s: empty stack — compiler emitted unbalanced bytecode", op))
	}
	operand := s[len(s)-1]
	s = s[:len(s)-1]
	// H3-sweep: bubble *Error operand instead of letting EvalUnaryOp
	// synthesise "operator X not defined for ERROR".
	if eval.IsError(operand) {
		*stack = s
		return operand, nil
	}
	pos := ast.Pos{Line: lineForOffset(chunk, codeOffset)}
	result := eval.EvalUnaryOp(operand, op, pos)
	if eval.IsError(result) {
		*stack = s
		return result, nil
	}
	s = append(s, result)
	*stack = s
	return nil, nil
}

// iterStep is the per-iteration logic for OpMap / OpFilter / OpReduce
// flat-dispatch (A2 audit cleanup). Called by OpReturn when the frame
// being returned to has a non-nil `iter`.
//
// Returns (final, true) when iteration is done — either the assembled
// result (Array for Map/Filter, accumulator for Reduce) or a kLex
// *Error to short-circuit. Caller pops the iter frame and pushes
// `final` onto the parent's stack.
//
// Returns (nil, false) when iteration advanced; caller re-launches
// the callback on the same iter frame (no push/pop, just reset pc=0
// and reload locals[0] / locals[1] for the next element).
func iterStep(parent *callFrame, retVal eval.Object) (eval.Object, bool) {
	if eval.IsError(retVal) {
		return retVal, true
	}
	it := parent.iter
	switch it.mode {
	case OpMap:
		it.out[it.idx] = retVal
	case OpFilter:
		b, isBool := retVal.(*eval.Boolean)
		if !isBool {
			// M3: attribute the error to OpFilter itself (it.opPC),
			// not parent.pc-1 — parent.pc was saved as the post-
			// dispatch pc, so parent.pc-1 points one byte past
			// OpFilter and lineForOffset can land on the wrong
			// source line when OpFilter and the next op span lines.
			return &eval.Error{
				Kind:    eval.TypeError,
				Pos:     ast.Pos{Line: lineForOffset(parent.chunk, it.opPC)},
				Message: fmt.Sprintf("filter: function must return bool, got %s", retVal.Type()),
			}, true
		}
		if b.Value {
			it.out = append(it.out, it.elems[it.idx])
		}
	case OpReduce:
		it.acc = retVal
	}
	it.idx++
	if it.idx < len(it.elems) {
		return nil, false
	}
	switch it.mode {
	case OpMap, OpFilter:
		return &eval.Array{Elements: it.out}, true
	case OpReduce:
		return it.acc, true
	}
	return eval.NULL, true // unreachable; iter.mode is one of the three
}

// validateArity checks whether `argc` is acceptable for cf's
// declared parameter shape. Returns "" on success or an error
// message describing the mismatch. Variadic functions accept any
// argc >= NumParams-1 (the rest-param can collect zero or more
// args); non-variadic require exact equality.
//
// Used at every CompiledFunction call site (OpCall, OpCallMethod
// fast + fallback, ExternalCallable, ExternalCallableAsync) so the
// variadic rule is enforced uniformly.
func validateArity(cf *CompiledFunction, argc int) string {
	if cf.Variadic {
		required := cf.NumParams - 1
		if argc < required {
			return fmt.Sprintf("%s expects at least %d arguments, got %d", cf.Inspect(), required, argc)
		}
		return ""
	}
	// Range check accounts for trailing defaulted params: argc may be
	// anywhere in [NumRequired, NumParams]. When the function has no
	// defaults NumRequired == NumParams, collapsing to the legacy
	// exact-count check. Error message expands to "N to M args" when
	// defaults are in play so the caller sees the actual range.
	if argc < cf.NumRequired || argc > cf.NumParams {
		if cf.NumRequired == cf.NumParams {
			return fmt.Sprintf("%s expects %d arguments, got %d", cf.Inspect(), cf.NumParams, argc)
		}
		return fmt.Sprintf("%s expects %d to %d arguments, got %d", cf.Inspect(), cf.NumRequired, cf.NumParams, argc)
	}
	return ""
}

// bindStackArgs populates calleeLocals from stack[srcStart:srcStart+argc]
// per cf's variadic shape. Assumes validateArity passed. For variadic
// functions the last declared param collects the trailing args into a
// fresh *eval.Array. The caller still owns slots beyond cf.NumParams
// (e.g. self-slot for recursion) and populates those separately.
// bindStackArgs returns a non-nil *eval.Error when the callee carries type
// annotations and a supplied argument violates one (TypeChecked gates the
// check, so unannotated functions pay nothing). callPos locates the error at
// the call site. Returns nil on success.
func bindStackArgs(cf *CompiledFunction, calleeLocals []*UpvalueCell, stack []eval.Object, srcStart, argc int, callPos ast.Pos) *eval.Error {
	if !cf.Variadic {
		for i := 0; i < argc; i++ {
			calleeLocals[i].Value = stack[srcStart+i]
		}
		// Fill trailing defaulted slots when the caller supplied
		// fewer args than NumParams. validateArity already ensured
		// argc >= NumRequired, so any missing index i is in the
		// range [NumRequired, NumParams) — guaranteed to have a
		// non-nil DefaultValues entry.
		for i := argc; i < cf.NumParams; i++ {
			calleeLocals[i].Value = cf.DefaultValues[i]
			// A default must satisfy its own parameter annotation.
			if cf.TypeChecked && i < len(cf.ParamTypes) && i < len(cf.Params) {
				if e := eval.CheckDefaultAnnotation(cf.ParamTypes[i], cf.Params[i],
					cf.DefaultValues[i], cf.displayName(), callPos); e != nil {
					return e
				}
			}
		}
	} else {
		required := cf.NumParams - 1
		for i := 0; i < required; i++ {
			calleeLocals[i].Value = stack[srcStart+i]
		}
		restEls := make([]eval.Object, argc-required)
		copy(restEls, stack[srcStart+required:srcStart+argc])
		calleeLocals[required].Value = &eval.Array{Elements: restEls}
	}
	if cf.TypeChecked {
		return eval.CheckArgAnnotations(cf.Params, cf.ParamTypes, cf.Variadic,
			stack[srcStart:srcStart+argc], cf.displayName(), callPos)
	}
	return nil
}

// bindMethodArgs is the OpCallMethod-shaped variant: calleeLocals[0] is
// pre-populated with the receiver (self), and stack[srcStart:srcStart+argc]
// are the USER args (not counting self). cf.NumParams already counts
// self. For variadic methods, `required` = cf.NumParams - 1 still, but
// the first one is already self; user args fill slots 1..required-1, and
// any extras become the rest-array at slot[required].
func bindMethodArgs(cf *CompiledFunction, calleeLocals []*UpvalueCell, stack []eval.Object, srcStart, argc int, callPos ast.Pos) *eval.Error {
	if !cf.Variadic {
		// Non-variadic method: caller already wrote slot[0]=receiver.
		for i := 0; i < argc; i++ {
			calleeLocals[i+1].Value = stack[srcStart+i]
		}
	} else {
		required := cf.NumParams - 1
		// Slot 0 is self (caller-populated). Required user slots start
		// at slot 1; rest-array slot is at `required`.
		userRequired := required - 1
		for i := 0; i < userRequired; i++ {
			calleeLocals[i+1].Value = stack[srcStart+i]
		}
		restEls := make([]eval.Object, argc-userRequired)
		copy(restEls, stack[srcStart+userRequired:srcStart+argc])
		calleeLocals[required].Value = &eval.Array{Elements: restEls}
	}
	if cf.TypeChecked && len(cf.Params) >= 1 && len(cf.ParamTypes) >= 1 {
		// Skip self (Params[0]/slot 0) — check only the user-supplied args
		// against the user params. cf.Params/cf.ParamTypes are parallel
		// with "self" prepended, so [1:] lines up with the stack args.
		return eval.CheckArgAnnotations(cf.Params[1:], cf.ParamTypes[1:], cf.Variadic,
			stack[srcStart:srcStart+argc], cf.displayName(), callPos)
	}
	return nil
}

// bindArgs is the ExternalCallable-shaped variant: args come from a
// []eval.Object rather than the value stack. Same arity rules.
// bindArgs returns a non-nil *eval.Error when the callee carries type
// annotations and a supplied argument violates one (gated by TypeChecked).
// This is the ExternalCallable path — args arrive as a slice rather than off
// the value stack — so the error position is the zero Pos (line-less), matching
// the convention for cross-interpreter boundary calls. Returns nil on success.
func bindArgs(cf *CompiledFunction, calleeLocals []*UpvalueCell, args []eval.Object) *eval.Error {
	if !cf.Variadic {
		for i, a := range args {
			calleeLocals[i].Value = a
		}
		// Fill trailing defaulted slots — see bindStackArgs for the
		// rationale. Used by ExternalCallable / ExternalCallableAsync
		// when the eval side calls into a VM function.
		for i := len(args); i < cf.NumParams; i++ {
			calleeLocals[i].Value = cf.DefaultValues[i]
			// A default must satisfy its own parameter annotation.
			if cf.TypeChecked && i < len(cf.ParamTypes) && i < len(cf.Params) {
				if e := eval.CheckDefaultAnnotation(cf.ParamTypes[i], cf.Params[i],
					cf.DefaultValues[i], cf.displayName(), ast.Pos{}); e != nil {
					return e
				}
			}
		}
	} else {
		required := cf.NumParams - 1
		for i := 0; i < required; i++ {
			calleeLocals[i].Value = args[i]
		}
		restEls := make([]eval.Object, len(args)-required)
		copy(restEls, args[required:])
		calleeLocals[required].Value = &eval.Array{Elements: restEls}
	}
	if cf.TypeChecked {
		return eval.CheckArgAnnotations(cf.Params, cf.ParamTypes, cf.Variadic,
			args, cf.displayName(), ast.Pos{})
	}
	return nil
}

// buildCallStackLines returns an innermost-first list of source lines
// for the active call stack at bubbleError entry. The first element is
// the line of the opcode that bubbled (current chunk + pc-1); each
// subsequent element is the line of the call site that pushed that
// frame (frames[i].chunk + frames[i].pc - 1). Used by the agentic
// error-bubble hook to give the agent a structured view of where in
// the program the error happened.
//
// Capped at 64 frames to bound allocation for deep-recursion errors
// (fib(30) bubbling a div-by-zero shouldn't allocate a 10000-element
// hash array). The cap is "depth shown to the hook", not a runtime
// limit — bubble still pops the full stack.
func buildCallStackLines(chunk *BytecodeChunk, pc int, frames []callFrame, frameCount int) []int {
	const maxStack = 64
	// Innermost first: current frame + each saved frame in reverse.
	n := frameCount + 1
	if n > maxStack {
		n = maxStack
	}
	out := make([]int, 0, n)
	out = append(out, lineForOffset(chunk, pc-1))
	for i := frameCount - 1; i >= 0 && len(out) < maxStack; i-- {
		f := frames[i]
		if f.chunk == nil {
			continue
		}
		out = append(out, lineForOffset(f.chunk, f.pc-1))
	}
	return out
}

// firstStackError scans a freshly-popped operand slice for an internal
// *Error and returns it. Used by collection-builder opcodes
// (OpMakeArray / OpMakeTuple / OpMakeHash / OpMakeStruct /
// OpMakeClosure) so that an upstream bubbled error doesn't get wrapped
// inside the collection — the caller bubbles it instead. Returns nil
// when no internal error is present (user errors with IsUserError=true
// stay as values, matching tree-walker semantics).
func firstStackError(els []eval.Object) eval.Object {
	for _, el := range els {
		if eval.IsError(el) {
			return el
		}
	}
	return nil
}

// lineForOffset reads chunk.Lines safely — out-of-range returns 0
// so vmError() messages still print rather than panicking on a
// missing position table entry.
func lineForOffset(chunk *BytecodeChunk, codeOffset int) int {
	if codeOffset < 0 || codeOffset >= len(chunk.Lines) {
		return 0
	}
	return chunk.Lines[codeOffset]
}

// vmError builds a runtime error object with source-position context
// pulled from chunk.Lines. Same shape as eval.runtimeError so callers
// don't have to special-case VM-side errors.
func vmError(chunk *BytecodeChunk, codeOffset int, msg string) error {
	line := 0
	if codeOffset >= 0 && codeOffset < len(chunk.Lines) {
		line = chunk.Lines[codeOffset]
	}
	return fmt.Errorf("vm runtime error at line %d (code offset %d): %s", line, codeOffset, msg)
}
