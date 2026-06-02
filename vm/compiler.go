package vm

// compiler.go — AST → bytecode translator.
//
// Every concrete ast.Node type carries a switch arm so vmcoverage
// can declare 100% coverage and the build fails immediately when
// somebody adds a new AST node without telling the compiler.
//
// Status: most arms have real implementations. The handful that
// still return errUnimplemented are listed under "Still stubbed"
// in [[vm-bytecode-project]] (MethodDecl's top-level arm,
// SelectStmt, InterpolatedString with embedded expressions).
//
// What works end-to-end:
//
//   - Every literal kind (Int / Float / String / Bool / Null / Bytes;
//     InterpolatedString in pure-literal form).
//   - Idents — locals, upvalues (closure capture), builtins-as-value.
//   - Arithmetic / comparison / logical operators (short-circuit
//     via peek-jumps). Routes through eval.EvalBinaryOp / EvalUnaryOp
//     so the VM never duplicates type rules.
//   - Control flow: if/else, while, for-in, switch (value + expression
//     forms, enum-pattern matching, exhaustive checking), break,
//     continue, return.
//   - CallExpr — builtins (OpCallBuiltin), user-defined functions
//     (OpCall on *CompiledFunction), tree-walker fns from imported
//     modules (slow-path *eval.Function arm), method calls
//     (OpCallMethod), enum-variant constructors, builtin-as-value
//     dispatch (`f = trim; f("x")`).
//   - Collections: Array / Hash / Tuple literals, IndexExpr, indexed
//     and dot assignment, MultiAssignStmt (tuple-strict), MultiLetStmt
//     (`let a, b = …`).
//   - Closures with full upvalue capture; mutual recursion via top-
//     level pre-pass; per-iteration fresh-cell binding so loop-var
//     closures observe the iteration's value (compileLet / compileAssign
//     / compileMultiAssign / compileForIn all emit OpFreshCell on new-
//     slot allocations).
//   - Map / Filter / Reduce intrinsics with flat-dispatch iter frames.
//   - Structs (decl, instance, field read/write, methods with
//     OpInstallMethod), enums (decl, variant construction, pattern
//     match), error propagation (postfix `?` via OpUnwrap, statement-
//     boundary OpReturnIfError, mid-op `goto bubbleError`).
//
// Stubbed arms call c.unimplemented(node) so the user gets a
// position-anchored error rather than a silent miss. The vmdiff
// runner uses errUnimplemented as its "this program isn't VM-
// runnable yet" signal and skips comparing.
//
// Design notes
//
//   - We pass a *Compiler around rather than threading state through
//     function args. Keeps the switch arms readable.
//   - emit*() helpers handle endianness (little-endian throughout).
//     Operand widths follow opcodeOperandLayout exactly — the
//     disassembler walks the SAME layout, so any width mismatch is
//     caught the moment you disassemble the chunk.
//   - The constant pool dedupes by value: re-emitting `println("hi")`
//     in a loop adds "hi" once. Necessary because the LSP code paths
//     emit a lot of identical literals.

import (
	"encoding/binary"
	"fmt"
	"klex/ast"
	"klex/eval"
)

// BytecodeChunk is the output of compiling a function or top-level
// program. One chunk = one callable unit.
type BytecodeChunk struct {
	// Code is the instruction stream. Each instruction is one opcode
	// byte followed by zero or more operand bytes whose widths are
	// described by opcodeOperandLayout[opcode].
	Code []byte

	// Constants is the per-chunk constant pool. Strings, floats,
	// big-ints, and other non-trivial literals get pooled here and
	// referenced by PushConst <uint16-idx>. Capacity is uint16-1.
	Constants []eval.Object

	// NumLocals is the high-water mark of local slot allocation.
	// The VM allocates a local-slot array of this size per call frame.
	NumLocals int

	// Lines maps Code[i]'s byte offset to its 1-based source line.
	// Used by the VM to produce error messages with source positions.
	// Stored as a parallel slice for simplicity; could move to a
	// run-length table later if it becomes a memory issue.
	Lines []int

	// ScriptDir is the absolute directory of the source file this
	// chunk was compiled from. Set by the entry-point launcher
	// (main.go's --vm path) before Run(), used by the VM's
	// _scriptDir() intercept so builtin calls return the same
	// directory the tree-walker would. The tree-walker stores this
	// on the Environment and walks the env chain via env.ScriptDir();
	// VM bytecode has no env chain at runtime, so we attach it to
	// the chunk instead and the OpCallBuiltin dispatch site reads it.
	ScriptDir string

	// ScriptArgs holds the command-line arguments the entry-point
	// launcher would have placed on the tree-walker's global env as
	// `__args__`. The tree-walker resolves `__args__` via env.Get;
	// VM bytecode has no env chain, so the compiler emits
	// OpLoadScriptArgs for free references to `__args__` and the
	// dispatch loop builds a fresh *Array around this slice on each
	// load (mirrors the _scriptDir intercept pattern, just at load
	// not call). Propagated to every sub-chunk by PropagateScriptArgs.
	ScriptArgs []eval.Object

	// TopLevelNames maps every top-level binding name to its slot
	// index in the Program's local-slot table. M6 (audit follow-up,
	// 2026-05-22): populated for Program chunks only by
	// compileProgram so vm.CompileAndRunModule can extract module
	// exports after running the top-level. Nil for sub-chunks
	// (per-function chunks). The slot lookup happens once at
	// module-load time (not on a hot path) so the simple map shape
	// is fine.
	TopLevelNames map[string]int

	// ReturnType is the function's optional return-type annotation
	// ("" = none), copied from the FunctionLiteral at compile time.
	// OpReturn reads it off the executing chunk (which is loop-local
	// in the dispatch) to enforce the declared return type — the same
	// check the tree-walker does at its function-return points. Empty
	// for the top-level program chunk (top-level can't be annotated).
	ReturnType string

	// FnName is the function's source name ("" for anonymous / the
	// top-level program), used purely to make the return-type error
	// message match the tree-walker's wording.
	FnName string
}

// Compiler holds the in-progress compile state. New one per chunk.
type Compiler struct {
	chunk *BytecodeChunk

	// constIndex dedupes the constant pool. We use Inspect() of the
	// kLex Object as the key — that's the same equality used by the
	// language for primitive literals, so two `"hello"` strings share
	// a slot.
	constIndex map[string]uint16

	// locals maps name → slot for the current scope. The compiler
	// does NOT yet implement proper lexical scoping (let / for-in
	// bodies will need a stack of these); the skeleton sticks to a
	// flat map and the for-in clause is errUnimplemented.
	locals   map[string]int
	nextSlot int

	// loopStack tracks the enclosing loops so break/continue know
	// where to jump. Pushed on entry to compileWhile / compileForIn,
	// popped on exit. Empty stack + break/continue is a compile-time
	// error ("X outside loop"), matching the tree-walker's behaviour.
	loopStack []loopContext

	// constSlots marks slots whose binding was created by ConstStmt
	// and must reject reassignment. compileAssign and compileLet
	// consult this set before emitting StoreLocal and produce a
	// compile-time error on violation — matching the tree-walker's
	// runtime const enforcement, just earlier.
	constSlots map[int]bool

	// outer points to the parent compiler when this is a sub-compiler
	// for a FunctionLiteral body. nil for the top-level compiler.
	// Used by resolveUpvalue to walk outward looking for a name that
	// isn't locally bound — the foundation of closure capture.
	outer *Compiler

	// upvalueRefs records every value this function captures from an
	// enclosing scope. Populated lazily by resolveUpvalue as the body
	// references outer-scope names. Becomes CompiledFunction.UpvalueRefs
	// when the body finishes compiling.
	upvalueRefs []upvalueRef

	// upvalueIndex dedupes upvalues by name so multiple references to
	// the same outer-scope name reuse one capture slot.
	upvalueIndex map[string]uint16
}

// loopContext records what break and continue need to know inside
// a single loop. BOTH break and continue use patch-lists rather
// than fixed offsets because:
//
//   - break always jumps OUT of the loop, but "out" isn't known
//     until the body finishes emitting (we have to compile the
//     body, the increment / back-jump, and only THEN do we know
//     the byte offset of `end`).
//   - continue's target varies by loop kind: while loops jump to
//     the condition check (which IS known at body start), but
//     for-in loops have a synthesised increment step that has to
//     run before the next condition check — and that increment
//     isn't emitted until after the body. Treating both
//     destinations as "patched later" keeps the model uniform.
//
// compileWhile / compileForIn push a context on entry, the body's
// break/continue compile arms push patch offsets onto these lists,
// and the loop emits the destination and resolves every patch
// before popping the context.
type loopContext struct {
	breakPatches    []int
	continuePatches []int
}

// Compile translates a parsed kLex program into a single
// BytecodeChunk. Returns errUnimplemented as soon as the compiler
// hits an AST shape it doesn't yet handle — the chunk that comes
// back in that case is partial and should not be executed.
func Compile(prog *ast.Program) (*BytecodeChunk, error) {
	c := newCompiler()
	if err := c.compileNode(prog); err != nil {
		return nil, err
	}
	c.emitOp(OpHalt, ast.Pos{})
	return c.chunk, nil
}

// PropagateScriptDir sets chunk.ScriptDir AND recursively writes the
// same value to every sub-chunk reachable via constant-pool
// *CompiledFunction values. M3 (audit fix, 2026-05-22): the entry
// launcher (main.go --vm, vm.CompileAndRunModule) used to set only
// the top-level chunk's ScriptDir, leaving nested-function chunks
// at "". That meant `_scriptDir()` called from inside any nested
// closure or struct method returned the empty string. With this
// post-process, every chunk reachable from the program inherits
// the module's directory.
//
// Cycle-safe via a seen-set: the constant pool can in principle
// hold the same *CompiledFunction template referenced multiple times
// (the compiler dedupes via constIndex but only for value-dedupable
// types; CompiledFunction isn't dedupable so cycles shouldn't
// happen — the seen-set is defensive).
func PropagateScriptDir(chunk *BytecodeChunk, scriptDir string) {
	if chunk == nil {
		return
	}
	seen := map[*BytecodeChunk]bool{}
	var walk func(c *BytecodeChunk)
	walk = func(cc *BytecodeChunk) {
		if cc == nil || seen[cc] {
			return
		}
		seen[cc] = true
		cc.ScriptDir = scriptDir
		for _, k := range cc.Constants {
			if cf, ok := k.(*CompiledFunction); ok && cf.Chunk != nil {
				walk(cf.Chunk)
			}
		}
	}
	walk(chunk)
}

// PropagateScriptArgs sets chunk.ScriptArgs on the entry chunk AND
// recursively on every sub-chunk reachable via constant-pool
// *CompiledFunction values. Same recipe as PropagateScriptDir — and
// for the same reason: a nested function/closure that references
// `__args__` runs against its own chunk, so that chunk must carry
// the args too. The args slice is shared across chunks (no per-chunk
// copy), which is fine because the VM never mutates it; each
// OpLoadScriptArgs builds a fresh *Array around it.
func PropagateScriptArgs(chunk *BytecodeChunk, args []eval.Object) {
	if chunk == nil {
		return
	}
	seen := map[*BytecodeChunk]bool{}
	var walk func(c *BytecodeChunk)
	walk = func(cc *BytecodeChunk) {
		if cc == nil || seen[cc] {
			return
		}
		seen[cc] = true
		cc.ScriptArgs = args
		for _, k := range cc.Constants {
			if cf, ok := k.(*CompiledFunction); ok && cf.Chunk != nil {
				walk(cf.Chunk)
			}
		}
	}
	walk(chunk)
}

func newCompiler() *Compiler {
	return &Compiler{
		chunk:        &BytecodeChunk{},
		constIndex:   make(map[string]uint16),
		locals:       make(map[string]int),
		constSlots:   make(map[int]bool),
		upvalueIndex: make(map[string]uint16),
	}
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

// compileNode is the central type-switch. EVERY concrete ast.Node
// type that satisfies ast.Node must have an arm here — vmcoverage
// enforces this in CI. Stub arms call c.unimplemented(node) so the
// user gets a position-anchored error rather than a silent miss.
func (c *Compiler) compileNode(n ast.Node) error {
	// Defensive: ast slices occasionally contain a typed nil slot
	// (e.g. an optional `Defaults[i]` for a parameter without a
	// default value, or stub MethodDecl arms emitted by some parser
	// paths). The tree-walker's Eval(nil) silently returns NULL via
	// its default switch arm; mirror that so we don't blow up the
	// compile with a misleading "unknown ast node type <nil>" error
	// before the program ever has a chance to run.
	if n == nil {
		c.emitOp(OpPushNull, ast.Pos{})
		return nil
	}
	switch v := n.(type) {

	// ── Top level ───────────────────────────────────────────────────
	case *ast.Program:
		return c.compileProgram(v)

	// ── Literals (real) ─────────────────────────────────────────────
	case *ast.IntLiteral:
		return c.compileLiteral(&eval.Integer{Value: v.Value}, v.Pos)
	case *ast.FloatLiteral:
		return c.compileLiteral(&eval.Float{Value: v.Value}, v.Pos)
	case *ast.StringLiteral:
		return c.compileLiteral(&eval.String{Value: v.Value}, v.Pos)
	case *ast.BoolLiteral:
		if v.Value {
			c.emitOp(OpPushTrue, v.Pos)
		} else {
			c.emitOp(OpPushFalse, v.Pos)
		}
		return nil
	case *ast.NullLiteral:
		c.emitOp(OpPushNull, v.Pos)
		return nil
	case *ast.BytesLiteral:
		return c.compileLiteral(&eval.Bytes{Value: v.Value}, v.Pos)
	case *ast.InterpolatedString:
		return c.compileInterpolatedString(v)

	// ── Identifiers & calls ────────────────────────────────────────
	case *ast.Ident:
		return c.compileIdent(v)
	case *ast.CallExpr:
		return c.compileCall(v)

	// ── Stub arms — all return errUnimplemented ────────────────────
	case *ast.ArrayLiteral:
		return c.compileArrayLiteral(v)
	case *ast.HashLiteral:
		return c.compileHashLiteral(v)
	case *ast.TupleLiteral:
		return c.compileTupleLiteral(v)
	case *ast.StructLiteral:
		return c.compileStructLiteral(v)
	case *ast.FunctionLiteral:
		return c.compileFunctionLiteral(v)
	case *ast.PrefixExpr:
		return c.compilePrefix(v)
	case *ast.InfixExpr:
		return c.compileInfix(v)
	case *ast.IndexExpr:
		return c.compileIndex(v)
	case *ast.DotExpr:
		return c.compileDotExpr(v)
	case *ast.PipeExpr:
		return c.compilePipe(v)
	case *ast.UnwrapExpr:
		if err := c.compileNode(v.Value); err != nil {
			return err
		}
		c.emitOp(OpUnwrap, v.Pos)
		return nil
	case *ast.AssignStmt:
		return c.compileAssign(v)
	case *ast.LetStmt:
		return c.compileLet(v)
	case *ast.ConstStmt:
		return c.compileConst(v)
	case *ast.MultiAssignStmt:
		return c.compileMultiAssign(v)
	case *ast.MultiLetStmt:
		return c.compileMultiLet(v)
	case *ast.IndexAssignStmt:
		return c.compileIndexAssign(v)
	case *ast.DotAssignStmt:
		return c.compileDotAssign(v)
	case *ast.IfStmt:
		return c.compileIf(v)
	case *ast.WhileStmt:
		return c.compileWhile(v)
	case *ast.ForInStmt:
		return c.compileForIn(v)
	case *ast.BreakStmt:
		return c.compileBreak(v)
	case *ast.ContinueStmt:
		return c.compileContinue(v)
	case *ast.ReturnStmt:
		return c.compileReturn(v)
	case *ast.SwitchStmt:
		return c.compileSwitch(v)
	case *ast.SelectStmt:
		// SelectStmt is non-trivial to lower because the tree-walker
		// uses reflect.Select over a runtime-built case table, and
		// each case's body needs to bind recv'd values into locals
		// before executing. A faithful VM impl needs either a
		// dedicated OpSelect with an inline case-descriptor table
		// (variable-length operand, jump-table style) OR a runtime
		// helper that accepts per-case body closures. Neither is
		// hard, just substantial — see project_vm_bytecode.md for
		// the design sketch.
		//
		// Real-world impact is limited (mostly concurrent server /
		// event-loop code), so deferring is acceptable for now.
		return c.unimplementedAt(v.Pos, "SelectStmt (deferred — needs OpSelect + case-descriptor table)")
	case *ast.EnumDecl:
		return c.compileEnumDecl(v)
	case *ast.EnumPattern:
		return c.unimplementedAt(v.Pos, "EnumPattern")
	case *ast.StructDecl:
		return c.compileStructDecl(v)
	case *ast.MethodDecl:
		return c.unimplementedAt(v.Pos, "MethodDecl")
	case *ast.ImportStmt:
		return c.compileImport(v)
	}
	return fmt.Errorf("vm/compiler: unknown ast node type %T — add a case in compileNode and rerun vmcoverage", n)
}

// ── Real implementations ──────────────────────────────────────────────────────

// compileStmtList emits each statement followed by OpReturnIfError.
// The opcode normalises the stack (pops the statement's leftover
// value if any) AND bubbles Errors up to the enclosing chunk —
// matching the tree-walker's `if isError(result) return result`
// loop. Used everywhere a body of statements appears: program,
// function body, if/else arms, while body, for-in body, switch
// case bodies, default body.
//
// `keepLast`: when true, the final statement's value is LEFT on
// the stack (no trailing OpReturnIfError). Used by function bodies
// so the implicit return naturally takes the last statement's
// value — matching the tree-walker's "function returns the last
// statement's evaluated value" rule. Errors in intermediate
// statements still propagate via OpReturnIfError; if the LAST
// statement produces an Error, it lands on the stack and the
// caller's OpReturnIfError catches it.
func (c *Compiler) compileStmtList(stmts []ast.Node, keepLast bool) error {
	for i, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			return err
		}
		isLast := i == len(stmts)-1
		if isLast && keepLast {
			continue
		}
		c.emitOp(OpReturnIfError, ast.Pos{})
	}
	return nil
}

// compileProgram emits each statement's bytecode in order via
// compileStmtList so errors from any statement halt cleanly
// (matches the tree-walker's main-loop error propagation).
func (c *Compiler) compileProgram(p *ast.Program) error {
	// Pre-pass: hoist top-level `fn name(...)` declarations so
	// mutually-recursive references resolve at compile time even
	// when the callee is declared LATER in source order. The
	// tree-walker doesn't need this trick because eval-time lookup
	// is by name in the live env; the VM resolves names statically,
	// so any later-declared callee would be "undefined" without
	// pre-declaration.
	//
	// We allocate the slots upfront (initial cell.Value = nil at
	// runtime); the regular compileAssign / compileLet arms will
	// REUSE the pre-allocated slot when they encounter the
	// declaration in order, and the StoreLocal they emit fills the
	// cell with the actual function value. By the time any call
	// site that captured the slot as an upvalue actually invokes
	// the function, the cell has been populated.
	//
	// Limited to top-level by design: hoisting non-top-level (e.g.
	// `fn inner` declared inside another function) would require
	// the same machinery applied per-function, and is left for a
	// future pass if real code needs it.
	for _, stmt := range p.Statements {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if _, ok := s.Value.(*ast.FunctionLiteral); ok && s.Name != "_" {
				if _, exists := c.locals[s.Name]; !exists {
					c.locals[s.Name] = c.nextSlot
					c.nextSlot++
					if c.nextSlot > c.chunk.NumLocals {
						c.chunk.NumLocals = c.nextSlot
					}
				}
			}
		case *ast.LetStmt:
			if _, ok := s.Value.(*ast.FunctionLiteral); ok && s.Name != "_" {
				if _, exists := c.locals[s.Name]; !exists {
					c.locals[s.Name] = c.nextSlot
					c.nextSlot++
					if c.nextSlot > c.chunk.NumLocals {
						c.chunk.NumLocals = c.nextSlot
					}
				}
			}
		}
	}
	if err := c.compileStmtList(p.Statements, false); err != nil {
		return err
	}
	// M6 (audit follow-up, 2026-05-22): snapshot the top-level name
	// table onto the chunk. CompileAndRunModule reads this after
	// running the program to populate the *Module's env with live
	// bindings pointing at the persistent UpvalueCells. Skipped for
	// sub-chunks (per-function chunks) because they have no notion
	// of "module exports" — their locals are call-frame-local.
	if len(c.locals) > 0 {
		names := make(map[string]int, len(c.locals))
		for name, slot := range c.locals {
			names[name] = slot
		}
		c.chunk.TopLevelNames = names
	}
	return nil
}

// compileLiteral pools the object and emits PushConst. Dedups
// against the constIndex map so repeated literals share a slot.
func (c *Compiler) compileLiteral(obj eval.Object, pos ast.Pos) error {
	idx := c.poolConstant(obj)
	c.emitOp(OpPushConst, pos)
	c.emitU16(idx)
	return nil
}

// compileInterpolatedString handles the trivial case (a single
// literal segment) by lowering to a string constant. Embedded
// expressions are deferred until we have a string-builder opcode +
// the InfixExpr arm.
func (c *Compiler) compileInterpolatedString(s *ast.InterpolatedString) error {
	if len(s.Segments) == 1 && !s.Segments[0].IsExpr {
		return c.compileLiteral(&eval.String{Value: s.Segments[0].Text}, s.Pos)
	}
	return c.unimplementedAt(s.Pos, "InterpolatedString with embedded expressions")
}

// compileIdent emits a Load for the named binding. Resolution order:
//
//  1. THIS compiler's locals     → OpLoadLocal slot
//  2. Any enclosing compiler's   → register as an upvalue via
//     locals or upvalues           resolveUpvalue, OpGetUpvalue idx
//  3. Not found                  → compile-time error (the only
//     names the VM doesn't statically
//     resolve are builtins, which
//     CallExpr handles before reaching
//     this arm)
//
// resolveUpvalue may recurse upward through multiple compiler frames,
// registering an upvalue chain so an Ident reaching three levels out
// captures correctly. See upvalue.go for the runtime side.
func (c *Compiler) compileIdent(id *ast.Ident) error {
	if slot, ok := c.locals[id.Value]; ok {
		c.emitOp(OpLoadLocal, id.Pos)
		c.emitU16(uint16(slot))
		return nil
	}
	if idx, ok := c.resolveUpvalue(id.Value); ok {
		c.emitOp(OpGetUpvalue, id.Pos)
		c.emitU16(idx)
		return nil
	}
	// __args__ intercept. The tree-walker sees `__args__` on the
	// entry env (main.go does env.Set("__args__", ...) before Eval);
	// the VM has no env chain, so a free reference here means "load
	// the command-line args injected by the launcher". Sits AFTER
	// the locals + upvalues checks so `let __args__ = X` still
	// shadows the injected binding the way it would in the tree-
	// walker. Mirrors the _scriptDir intercept at OpCallBuiltin —
	// just at load instead of call. PropagateScriptArgs copies the
	// args slice onto every reachable sub-chunk, so nested closures
	// resolve correctly.
	if id.Value == "__args__" {
		c.emitOp(OpLoadScriptArgs, id.Pos)
		return nil
	}
	// Builtin used as a value (not a call) — e.g. `arr |> trim` or
	// `map(arr, double)`. Push the Builtin object as a constant so
	// callCallable / higher-order builtins receive a *Builtin.
	if b, ok := eval.Builtins[id.Value]; ok {
		idx := c.poolConstant(b)
		c.emitOp(OpPushConst, id.Pos)
		c.emitU16(idx)
		return nil
	}
	// Deferred-resolution path. The tree-walker resolves identifiers
	// lazily, so a name that only appears in a never-executed branch
	// must NOT be a compile-time error. Push an OpUndefinedName
	// referencing the name in the constant pool; at runtime, this
	// either resolves to a late-registered Builtin or raises a
	// kLex-shape "undefined name" error.
	nameIdx := c.poolConstant(&eval.String{Value: id.Value})
	c.emitOp(OpUndefinedName, id.Pos)
	c.emitU16(nameIdx)
	return nil
}

// resolveUpvalue walks the compiler-frame chain looking for `name`.
// On a hit, it registers an upvalue in EVERY frame between this
// compiler and the one where the name is locally defined — so the
// runtime capture chain stays intact through arbitrary nesting.
// Returns the upvalue index in THIS compiler's upvalue list.
//
// Returns (0, false) when name isn't found in any enclosing scope.
//
// The recursive structure mirrors Lua's compiler. Each call either:
//   - Returns false immediately if there's no outer.
//   - Finds name in outer's locals → registers an upvalue with
//     IsLocal=true pointing at that slot.
//   - Otherwise recurses into outer.resolveUpvalue → if that
//     succeeds, registers an upvalue with IsLocal=false pointing at
//     the outer's NEW upvalue slot. This propagates the capture up
//     the chain transparently.
func (c *Compiler) resolveUpvalue(name string) (uint16, bool) {
	if c.outer == nil {
		return 0, false
	}
	// Hit in the immediate parent's locals?
	if slot, ok := c.outer.locals[name]; ok {
		return c.addUpvalue(name, true, uint16(slot)), true
	}
	// Recurse: maybe parent itself captures this name from further out.
	if outerIdx, ok := c.outer.resolveUpvalue(name); ok {
		return c.addUpvalue(name, false, outerIdx), true
	}
	return 0, false
}

// addUpvalue records a new capture in c.upvalueRefs (deduped via
// upvalueIndex by name) and returns its index. Bounded by uint16
// like the rest of the operand-width family.
func (c *Compiler) addUpvalue(name string, isLocal bool, index uint16) uint16 {
	if idx, ok := c.upvalueIndex[name]; ok {
		return idx
	}
	idx := uint16(len(c.upvalueRefs))
	c.upvalueRefs = append(c.upvalueRefs, upvalueRef{IsLocal: isLocal, Index: index})
	c.upvalueIndex[name] = idx
	return idx
}

// compileAssign handles bare `x = expr`.
//
// Tree-walker semantics (see [[feedback-closure-env]]): walk the
// scope chain to find an existing binding and update it there, or
// create in the current scope if not found. The skeleton has a
// single flat scope today (no function frames, no for-in nested
// scopes), so "walk chain" collapses to "look up in c.locals."
//
// `x = _` is the canonical discard: evaluate RHS for side effects,
// don't store.
//
// Once function literals land we'll switch from a single flat map
// to a stack of compiler scopes; this arm's body becomes the
// per-scope variant of the same logic.
func (c *Compiler) compileAssign(a *ast.AssignStmt) error {
	// Special-case: `name = fn(...) { ... }` — pass the assignment
	// name into the function literal so its body can recurse through
	// the self-slot. Bare anonymous lambdas (RHS is a different
	// shape) fall through to the regular compile path.
	if fl, ok := a.Value.(*ast.FunctionLiteral); ok && a.Name != "_" {
		if err := c.compileFunctionLiteralNamed(fl, a.Name, a.Name); err != nil {
			return err
		}
	} else if err := c.compileNode(a.Value); err != nil {
		return err
	}
	if a.Name == "_" {
		// Discard: pop the value we just computed.
		c.emitOp(OpPop, a.Pos)
		return nil
	}
	// kLex bare `=` walks the scope chain (see [[feedback-closure-env]]):
	//   1. If the name exists in THIS scope's locals → mutate it.
	//   2. Else if it exists in an enclosing scope → mutate THROUGH
	//      that scope's cell via SetUpvalue, so the outer binding
	//      is updated (and any sibling closure capturing the same
	//      cell sees the change).
	//   3. Otherwise create a new binding in the current scope.
	//
	// LetStmt (compileLet) skips step 2 by design — `let` ALWAYS
	// binds in the current scope, never reaches outward.
	if slot, ok := c.locals[a.Name]; ok {
		// Const reassignment is a RUNTIME error in kLex (so safe()
		// can catch it). The compile-time constSlots flag stays as
		// extra defensive info, but we no longer refuse to emit
		// — OpStoreLocal checks the cell's IsConst at runtime.
		c.emitOp(OpStoreLocal, a.Pos)
		c.emitU16(uint16(slot))
		return nil
	}
	if idx, ok := c.resolveUpvalue(a.Name); ok {
		c.emitOp(OpSetUpvalue, a.Pos)
		c.emitU16(idx)
		return nil
	}
	// M6 (audit fix, 2026-05-23): new-slot allocation needs OpFreshCell
	// for the same reason compileLet does — a binding created inside a
	// loop body must get a fresh cell per iteration so closures capturing
	// it during that iteration don't alias subsequent iterations' values.
	// The existing-slot path above stays as plain OpStoreLocal: that's
	// mutation, not declaration, and existing captures intentionally see
	// the new value through the shared cell.
	slot := c.nextSlot
	c.locals[a.Name] = slot
	c.nextSlot++
	if c.nextSlot > c.chunk.NumLocals {
		c.chunk.NumLocals = c.nextSlot
	}
	c.emitOp(OpFreshCell, a.Pos)
	c.emitU16(uint16(slot))
	c.emitOp(OpStoreLocal, a.Pos)
	c.emitU16(uint16(slot))
	return nil
}

// compileLet handles `let x = expr`. In the tree-walker, let always
// creates in the CURRENT scope and never walks outer scopes (see
// [[feedback-let-keyword]]). In our flat-scope skeleton this is
// indistinguishable from AssignStmt — same map, same slot allocator.
// The distinction matters once we add nested scopes for function
// bodies / for-in bodies; until then `let` and bare `=` share an
// implementation by design.
func (c *Compiler) compileLet(l *ast.LetStmt) error {
	// Same self-slot threading as compileAssign — `let foo = fn(...) {…}`
	// inside another function's body must still allow recursive
	// self-reference. Anonymous RHS falls through.
	if fl, ok := l.Value.(*ast.FunctionLiteral); ok && l.Name != "_" {
		if err := c.compileFunctionLiteralNamed(fl, l.Name, l.Name); err != nil {
			return err
		}
	} else if err := c.compileNode(l.Value); err != nil {
		return err
	}
	if l.Name == "_" {
		c.emitOp(OpPop, l.Pos)
		return nil
	}
	slot, ok := c.locals[l.Name]
	if !ok {
		slot = c.nextSlot
		c.locals[l.Name] = slot
		c.nextSlot++
		if c.nextSlot > c.chunk.NumLocals {
			c.chunk.NumLocals = c.nextSlot
		}
	}
	// Emit OpFreshCell so each execution of this `let` binds a
	// brand-new UpvalueCell. Inside a loop body, this is what makes
	// `let x = i` followed by `async(fn() { … x … })` produce
	// closures that observe DIFFERENT x values per iteration. Without
	// the fresh cell, every iteration's closure would alias the same
	// cell and all would observe the loop's final value.
	//
	// Note we emit AFTER the RHS has already been computed (above);
	// OpFreshCell replaces locals[slot] but doesn't touch the stack,
	// so the pending RHS is still on top ready for OpStoreLocal.
	c.emitOp(OpFreshCell, l.Pos)
	c.emitU16(uint16(slot))
	// Const reassignment is enforced at runtime via the cell's
	// IsConst flag; the constSlots compile-time tracking is no longer
	// load-bearing for the error, but we keep it for diagnostics.
	c.emitOp(OpStoreLocal, l.Pos)
	c.emitU16(uint16(slot))
	return nil
}

// compilePrefix handles unary operators. kLex has exactly two:
//
//	`!x`  → Not    (Bool-only; type-checking is the VM's job)
//	`-x`  → Neg    (Int or Float; promotion is the VM's job)
//
// The compiler doesn't enforce type rules — it just emits the
// opcode. The VM handler reports a clean type error if the operand
// is the wrong shape, matching the tree-walker's behaviour exactly.
func (c *Compiler) compilePrefix(p *ast.PrefixExpr) error {
	if err := c.compileNode(p.Right); err != nil {
		return err
	}
	switch p.Operator {
	case "!":
		c.emitOp(OpNot, p.Pos)
	case "-":
		c.emitOp(OpNeg, p.Pos)
	default:
		return c.unimplementedAt(p.Pos, "PrefixExpr operator "+p.Operator)
	}
	return nil
}

// compileInfix handles binary operators. Three groups:
//
//   - Arithmetic: + - * / %      (Int+Int promote-aware; VM dispatches)
//   - Comparison: == != < > <= >= (returns Boolean)
//   - Logical:    && ||           (short-circuits — must compile specially)
//
// The short-circuit ops compile to JumpIfFalse / Jump rather than
// pushing both sides and emitting a single opcode. We don't have
// those opcodes wired yet (IfStmt is the next milestone), so until
// then && / || return unimplemented. The other operators map 1:1
// to opcodes via the Eq/Ne/Lt/etc table.
//
// The tree-walker's [`eval/eval_fast_int.go`](eval/eval_fast_int.go)
// fast path is the reference for unboxed-int semantics; we don't
// duplicate that optimisation here because the VM never boxes
// intermediate arithmetic values to begin with (it works on a
// stack of Go pointers, never re-binding to env slots between ops).
func (c *Compiler) compileInfix(n *ast.InfixExpr) error {
	if n.Operator == "&&" || n.Operator == "||" {
		return c.compileShortCircuit(n)
	}
	if err := c.compileNode(n.Left); err != nil {
		return err
	}
	if err := c.compileNode(n.Right); err != nil {
		return err
	}
	var op opcode
	switch n.Operator {
	case "+":
		op = OpAdd
	case "-":
		op = OpSub
	case "*":
		op = OpMul
	case "/":
		op = OpDiv
	case "%":
		op = OpMod
	case "==":
		op = OpEq
	case "!=":
		op = OpNe
	case "<":
		op = OpLt
	case "<=":
		op = OpLe
	case ">":
		op = OpGt
	case ">=":
		op = OpGe
	default:
		return c.unimplementedAt(n.Pos, "InfixExpr operator "+n.Operator)
	}
	c.emitOp(op, n.Pos)
	return nil
}

// compileSwitch lowers both forms of switch into a chain of
// equality tests + JumpIfFalse hops. Two shapes:
//
//	Value switch:       switch subj { case v1, v2 { body } default { } }
//	                    — every case value is compared with subj via ==.
//
//	Expression switch:  switch       { case bool_expr { body } default { } }
//	                    — every case value is itself a boolean expr.
//
// Within one case, multiple values OR together — any match runs
// the body. Cases are tried in source order; first match wins; no
// fallthrough (kLex's semantic).
//
// Layout (value switch with two values per case):
//
//	<compile subj>; StoreLocal subjSlot
//	case 0:
//	  try_v0_0:  LoadLocal subj; <v0_0>; Eq; JumpIfFalse → try_v0_1
//	             Jump → body_0
//	  try_v0_1:  LoadLocal subj; <v0_1>; Eq; JumpIfFalse → case_1
//	             Jump → body_0
//	body_0:      <body>; Jump → end
//	case_1:      ... (next case or default)
//	default:     <defaultBody>
//	end:
//
// Expression switch is identical minus the LoadLocal/Eq pair —
// each value is compiled directly as a bool expression.
//
// break inside a case body terminates the switch (matches the
// tree-walker). We don't push a loopContext because switch isn't a
// loop — break inside switch but outside any loop would compile-
// error today. If a real program needs that, we'll add a switch-
// context analog of loopContext.
//
// EnumPatterns inside case Values are not yet supported (enum
// milestone).
func (c *Compiler) compileSwitch(s *ast.SwitchStmt) error {
	subjSlot := -1
	if s.Subject != nil {
		if err := c.compileNode(s.Subject); err != nil {
			return err
		}
		subjSlot = c.allocAnonSlot()
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(subjSlot))
	}

	var endPatches []int

	for ci, sc := range s.Cases {
		// For each value in this case, emit a test that either
		// jumps to the body (on match) or falls through to the next
		// value's test (on miss). After the last value's test, a
		// miss falls through to nextCasePatch.
		var bodyJumpPatches []int  // each value's "match → body" Jump
		var nextValuePatches []int // each non-last value's JumpIfFalse

		// If the case carries an EnumPattern, the field bindings are
		// visible inside the body — track the binding slot list for
		// per-case storage. Only one pattern per case (kLex AST
		// doesn't allow mixing patterns with other values inside a
		// case), so we collect from a single arm.
		var enumBindings []int

		for vi, vNode := range sc.Values {
			// EnumPattern is a special shape: it pushes the pattern
			// value on the stack, then OpMatchVariant pops both
			// pattern and subject, pushes either (bindCount values +
			// True) or (False). The False path becomes JumpIfFalse
			// to the next case; the True path falls through to
			// binding stores.
			if ep, isPat := vNode.(*ast.EnumPattern); isPat {
				if subjSlot < 0 {
					return c.unimplementedAt(s.Pos, "enum pattern requires a switch subject")
				}
				if vi != 0 || len(sc.Values) != 1 {
					return c.unimplementedAt(s.Pos, "enum pattern must be the only value in its case")
				}
				// Compile the pattern. *ast.Ident → push as a String
				// (short form: name-only match). *ast.DotExpr → eval
				// to push an EnumVariant / EnumInstance (full form).
				if id, ok := ep.Pattern.(*ast.Ident); ok {
					strIdx := c.poolConstant(&eval.String{Value: id.Value})
					c.emitOp(OpPushConst, s.Pos)
					c.emitU16(strIdx)
				} else {
					if err := c.compileNode(ep.Pattern); err != nil {
						return err
					}
				}
				// Push subject below pattern? No — handler expects
				// [pattern, subject] with subject on TOP. We just
				// pushed pattern; now push subject.
				c.emitOp(OpLoadLocal, s.Pos)
				c.emitU16(uint16(subjSlot))
				// Match. Pushes either (N field values + True) or False.
				c.emitOp(OpMatchVariant, s.Pos)
				c.emitU16(uint16(len(ep.Bindings)))
				// Pre-allocate binding slots in pattern order — we
				// know them at compile time. Field values come off
				// the stack in declaration order (first-pushed at
				// bottom), so we StoreLocal in REVERSE order.
				//
				// Case-local scoping: bindings must NOT leak to the
				// outer scope. The tree-walker creates a fresh env
				// per case body, so an outer `radius = "before"`
				// stays "before" after `case Shape.Circle(radius)`
				// completes. To match that here, each binding gets a
				// FRESH anonymous slot for the body's lifetime; any
				// prior `c.locals[name]` entry is saved and restored
				// after the case body. (Restore happens further down
				// once we've also compiled the body.)
				enumBindings = make([]int, len(ep.Bindings))
				for i, name := range ep.Bindings {
					if name == "_" {
						enumBindings[i] = -1
						continue
					}
					slot := c.allocAnonSlot()
					enumBindings[i] = slot
				}
				// Branch on the bool sentinel.
				patch := c.emitJump(OpJumpIfFalse, s.Pos)
				nextValuePatches = append(nextValuePatches, patch)
				// Body-bound path: store fields in reverse order
				// from the stack, then fall through to body.
				for i := len(enumBindings) - 1; i >= 0; i-- {
					slot := enumBindings[i]
					if slot < 0 {
						c.emitOp(OpPop, s.Pos)
					} else {
						c.emitOp(OpStoreLocal, s.Pos)
						c.emitU16(uint16(slot))
					}
				}
				bodyPatch := c.emitJump(OpJump, s.Pos)
				bodyJumpPatches = append(bodyJumpPatches, bodyPatch)
				continue
			}
			if subjSlot >= 0 {
				// Value switch: subj == value
				c.emitOp(OpLoadLocal, s.Pos)
				c.emitU16(uint16(subjSlot))
				if err := c.compileNode(vNode); err != nil {
					return err
				}
				c.emitOp(OpEq, s.Pos)
			} else {
				// Expression switch: value is already a bool
				if err := c.compileNode(vNode); err != nil {
					return err
				}
			}
			isLast := vi == len(sc.Values)-1
			if isLast {
				// Final test: on miss, jump to next case (patched below).
				patch := c.emitJump(OpJumpIfFalse, s.Pos)
				nextValuePatches = append(nextValuePatches, patch)
				// On match, control reaches the unconditional Jump below.
				bodyPatch := c.emitJump(OpJump, s.Pos)
				bodyJumpPatches = append(bodyJumpPatches, bodyPatch)
			} else {
				// On miss, try the next value.
				skip := c.emitJump(OpJumpIfFalse, s.Pos)
				nextValuePatches = append(nextValuePatches, skip)
				// On match, jump to body (patched once we know its offset).
				toBody := c.emitJump(OpJump, s.Pos)
				bodyJumpPatches = append(bodyJumpPatches, toBody)
				// nextValuePatches[len-1] (the JumpIfFalse for THIS
				// test that skipped on miss) needs to land HERE — at
				// the start of the next test. So patch it now.
				c.patchJump(skip)
				// pop the just-patched entry to avoid double-patching
				nextValuePatches = nextValuePatches[:len(nextValuePatches)-1]
			}
		}

		// Body: patch all "match → body" jumps to land here.
		for _, p := range bodyJumpPatches {
			c.patchJump(p)
		}
		// Install enum-pattern bindings as named locals JUST for the
		// case body. Save any prior name → slot mapping so an outer
		// variable of the same name reappears after the case ends.
		// `pat.Bindings` only carries names; the matching slots live
		// in enumBindings (parallel index).
		type savedLocal struct {
			name      string
			hadEntry  bool
			priorSlot int
		}
		var saved []savedLocal
		if len(enumBindings) > 0 {
			// Find the pattern arm we matched on — there's at most one
			// per case (per ast contract), so the bindings list is the
			// first EnumPattern we saw above.
			for _, v := range sc.Values {
				if ep, ok := v.(*ast.EnumPattern); ok {
					saved = make([]savedLocal, len(ep.Bindings))
					for i, name := range ep.Bindings {
						saved[i] = savedLocal{name: name}
						if name == "_" {
							continue
						}
						if prior, had := c.locals[name]; had {
							saved[i].hadEntry = true
							saved[i].priorSlot = prior
						}
						c.locals[name] = enumBindings[i]
					}
					break
				}
			}
		}

		if err := c.compileStmtList(sc.Body, false); err != nil {
			return err
		}

		// Restore prior bindings so they're visible after the case.
		for _, sv := range saved {
			if sv.name == "_" {
				continue
			}
			if sv.hadEntry {
				c.locals[sv.name] = sv.priorSlot
			} else {
				delete(c.locals, sv.name)
			}
		}

		// After body: jump to end of switch.
		endPatches = append(endPatches, c.emitJump(OpJump, s.Pos))

		// Patch the final-value-miss to land at the start of the
		// next case (i.e. here, after this case's body+endJump).
		for _, p := range nextValuePatches {
			c.patchJump(p)
		}
		_ = ci
	}

	// Default body (if present).
	if s.HasDefault {
		if err := c.compileStmtList(s.Default, false); err != nil {
			return err
		}
	}

	// End-of-switch — patch every body's exit jump here.
	for _, p := range endPatches {
		c.patchJump(p)
	}
	return nil
}

// compileImport handles `import "path/to/file.lex" as alias`.
//
// Strategy: delegate the actual load+parse+eval to the tree-walker
// at RUNTIME via a new OpImport opcode. We can't load at compile
// time because:
//
//   - Module loading uses the importing chunk's runtime
//     environment (for KLEX_PATH, scriptDir, etc.). At compile
//     time we don't have an env to resolve against.
//   - Re-running the import at every VM startup is what kLex's
//     module cache (moduleCacheMu in eval) is built for — the
//     second call to import the same file is a cache hit.
//   - Imported modules' top-level fn bindings are *eval.Function
//     values; OpCall already delegates to eval.CallCallable for
//     that case. So the VM just needs to OBTAIN the Module, store
//     it under the alias, and the rest works.
//
// OpImport pushes the resulting *eval.Module onto the stack. We
// follow it with a StoreLocal of the alias slot — the standard
// "evaluate-then-bind" pattern used by AssignStmt.
func (c *Compiler) compileImport(s *ast.ImportStmt) error {
	pathIdx := c.poolConstant(&eval.String{Value: s.Path})
	aliasIdx := c.poolConstant(&eval.String{Value: s.Alias})
	c.emitOp(OpImport, s.Pos)
	c.emitU16(pathIdx)
	c.emitU16(aliasIdx)
	// OpImport leaves the *Module on the stack; bind to the alias.
	slot := c.allocSlotForName(s.Alias)
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(slot))
	return nil
}

// compileEnumDecl creates a runtime *eval.EnumDef value and binds
// it to the type name. Once bound, variants are accessed via
// `TypeName.VariantName` (DotExpr on an EnumDef) which produces
// either an EnumInstance (zero-field variant — usable directly) or
// an EnumVariant descriptor (data-carrying variant — callable to
// construct).
//
// Pattern-matching in `switch` is handled by compileSwitch's
// EnumPattern arm, which already routes through this def for the
// variant name lookup.
func (c *Compiler) compileEnumDecl(e *ast.EnumDecl) error {
	def := &eval.EnumDef{
		Name:     e.Name,
		Variants: make(map[string][]string, len(e.Variants)),
	}
	for _, v := range e.Variants {
		def.Variants[v.Name] = v.Fields
	}
	idx := c.poolConstant(def)
	c.emitOp(OpPushConst, e.Pos)
	c.emitU16(idx)
	slot := c.allocSlotForName(e.Name)
	if c.constSlots[slot] {
		return c.unimplementedAt(e.Pos, "cannot redeclare const as enum: "+e.Name)
	}
	c.emitOp(OpStoreLocal, e.Pos)
	c.emitU16(uint16(slot))
	return nil
}

// compileStructDecl creates a runtime *eval.StructDef value and
// binds it to the type name. Methods are compiled as
// *CompiledFunction values with `self` injected as parameter slot 0;
// they go into the def's MethodsAny map (polymorphic — see
// eval.StructDef). The dot-call dispatch site (OpCallMethod) reads
// from MethodsAny at runtime, so the method table travels with the
// def the same way the tree-walker's Methods map does.
//
// Method compile model
//
//   - Each MethodDecl turns into a FunctionLiteral whose first
//     parameter is the literal name "self", followed by the user-
//     declared params. The reused compileFunctionLiteralNamed handles
//     locals, upvalues, return semantics — methods inherit closures-
//     into-outer-scope for free.
//   - poolConstant gives us the *CompiledFunction; storing it under
//     MethodsAny preserves the def's identity (one shared def per
//     struct type, methods authored once).
func (c *Compiler) compileStructDecl(s *ast.StructDecl) error {
	// M5+M6 audit follow-up (2026-05-22): methods are now compiled
	// through the SAME closure machinery as regular fns
	// (compileFunctionLiteralNamed → OpPushConst or OpMakeClosure).
	// The struct def is pooled as a TEMPLATE with empty MethodsAny;
	// at runtime, after the def is pushed onto the stack, we emit
	// a per-method sequence:
	//
	//   <compile method body as anon fn>   ← pushes cf or closure
	//   OpInstallMethod <name-idx>          ← pops cf, attaches to def
	//
	// This fixes the latent bug where methods referencing module-
	// level fns (e.g. stdlib/observable.lex's `map` method calling
	// `newObservable`) recorded UpvalueRefs at compile time but had
	// cf.Upvalues == nil at runtime → "GetUpvalue idx 0 out of range".
	// The closure path populates Upvalues from the enclosing frame's
	// locals just like top-level closures.
	def := &eval.StructDef{
		Name:       s.Name,
		Fields:     s.Fields,
		Methods:    map[string]*eval.Function{}, // empty — VM uses MethodsAny
		MethodsAny: make(map[string]eval.Object, len(s.Methods)),
	}
	idx := c.poolConstant(def)
	c.emitOp(OpPushConst, s.Pos)
	c.emitU16(idx)

	// For each method, compile as a synthetic anonymous FunctionLiteral
	// (with self prepended) and install onto the def via OpInstallMethod.
	// Anonymous because methods don't recurse via their own bare name;
	// inside the body, recursive calls use `self.method(...)` which
	// dispatches through OpCallMethod, not the self-slot trick.
	for _, m := range s.Methods {
		fl := &ast.FunctionLiteral{
			Pos:    m.Pos,
			Params: append([]string{"self"}, m.Params...),
			// self is untyped; keep ParamTypes parallel to Params so the
			// annotation checker (which indexes them together) stays aligned.
			ParamTypes: append([]string{""}, m.ParamTypes...),
			ReturnType: m.ReturnType,
			Defaults:   append([]ast.Node{nil}, m.Defaults...),
			Variadic:   m.Variadic,
			Body:       m.Body,
		}
		// compileFunctionLiteralNamed with selfName="" produces an
		// anonymous *CompiledFunction. Crucially, if the body
		// references outer-scope names (newObservable, other
		// module-level fns, captured locals from an enclosing
		// function-with-methods), the sub-compiler registers them
		// as upvalues and the emit logic chooses OpMakeClosure over
		// OpPushConst — so the runtime cf has populated Upvalues.
		// selfName="" (methods recurse via self.method(), not a bare
		// self-slot), but displayName=m.Name so method type-errors and
		// stack traces name the method instead of "anonymous".
		if err := c.compileFunctionLiteralNamed(fl, "", m.Name); err != nil {
			return err
		}
		// The cf is now on top of the stack; the def is below it.
		// OpInstallMethod pops cf, sets def.MethodsAny[name] = cf,
		// leaves def on the stack for the next method.
		nameIdx := c.poolConstant(&eval.String{Value: m.Name})
		c.emitOp(OpInstallMethod, m.Pos)
		c.emitU16(nameIdx)
	}

	// def is still on stack with all methods installed. Bind it.
	slot := c.allocSlotForName(s.Name)
	if c.constSlots[slot] {
		return c.unimplementedAt(s.Pos, "cannot redeclare const as struct: "+s.Name)
	}
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(slot))
	return nil
}

// compileStructLiteral builds an instance: emits the def lookup,
// then alternating (name, value) pairs in source order, then
// OpMakeStruct with the pair count. The handler reads the def from
// the stack, validates each field name against def.Fields, and
// constructs the StructInstance.
//
// Why we don't pre-resolve the def at compile time: structs are
// runtime values bound via StoreLocal/Assign — the same struct name
// can shadow or be reassigned (though kLex idioms don't do this).
// Treating the def as a normal name lookup keeps the model
// uniform with everything else.
func (c *Compiler) compileStructLiteral(s *ast.StructLiteral) error {
	if len(s.Fields) > 0xFFFF {
		return c.unimplementedAt(s.Pos, "struct literal with > 65535 fields")
	}
	// Push the StructDef (looked up by name).
	if err := c.compileNode(&ast.Ident{Pos: s.Pos, Value: s.Name}); err != nil {
		return err
	}
	for _, fi := range s.Fields {
		// Push field name as a String constant.
		nameIdx := c.poolConstant(&eval.String{Value: fi.Name})
		c.emitOp(OpPushConst, s.Pos)
		c.emitU16(nameIdx)
		// Push value.
		if err := c.compileNode(fi.Value); err != nil {
			return err
		}
	}
	c.emitOp(OpMakeStruct, s.Pos)
	c.emitU16(uint16(len(s.Fields)))
	return nil
}

// compileDotExpr handles `expr.field` for struct instances (and
// will handle modules once ImportStmt lands — EvalGetField already
// branches on receiver type). Field name is stored as a String
// constant so OpGetField can read it in O(1) at runtime.
func (c *Compiler) compileDotExpr(d *ast.DotExpr) error {
	if err := c.compileNode(d.Left); err != nil {
		return err
	}
	nameIdx := c.poolConstant(&eval.String{Value: d.Property})
	c.emitOp(OpGetField, d.Pos)
	c.emitU16(nameIdx)
	return nil
}

// compileDotAssign handles `obj.field = value`. Same shape as
// compileIndexAssign but with the field name encoded as a
// constant-pool string rather than as a stack value.
func (c *Compiler) compileDotAssign(s *ast.DotAssignStmt) error {
	if err := c.compileNode(s.Left.Left); err != nil {
		return err
	}
	if err := c.compileNode(s.Value); err != nil {
		return err
	}
	nameIdx := c.poolConstant(&eval.String{Value: s.Left.Property})
	c.emitOp(OpSetField, s.Pos)
	c.emitU16(nameIdx)
	return nil
}

// compilePipe handles `left |> right`. kLex's pipe operator prepends
// the left value as the FIRST argument of the right-hand callable:
//
//	"hi"  |> trim                  →  trim("hi")
//	xs    |> map(double)           →  map(xs, double)
//	xs    |> filter(odd) |> len    →  len(filter(xs, odd))
//
// We share the call-shape dispatch with compileCall by emitting the
// same opcodes (CallBuiltin for known builtins, OpCall otherwise) —
// just with the left value spliced in as arg 0.
//
// Sharp edge to note: the tree-walker evaluates Left FIRST, then the
// extra args (left-to-right), then the callee. We preserve that
// ordering for builtins (left, then extra args, with builtin index
// in the operand — no separate callee evaluation). For user calls
// the callee must be on the stack BEFORE its args per OpCall's
// contract, which means evaluating callee first — that's a
// deliberate, documented divergence from the tree-walker's order on
// the user-call path. In practice it only matters if the callee
// expression has side effects that observe the args, which is
// vanishingly rare and never seen in any real kLex code.
func (c *Compiler) compilePipe(p *ast.PipeExpr) error {
	// Decompose `right` into (callee, extraArgs). If right is a
	// CallExpr we lift its function + args; otherwise the whole
	// expression is the callee with no extra args.
	var calleeNode ast.Node
	var extraArgs []ast.Node
	if call, ok := p.Right.(*ast.CallExpr); ok {
		calleeNode = call.Function
		extraArgs = call.Args
	} else {
		calleeNode = p.Right
	}

	argc := 1 + len(extraArgs)
	if argc > 255 {
		return fmt.Errorf("vm/compiler: too many piped arguments (%d) at line %d — argc is uint8", argc, p.Pos.Line)
	}

	// Builtin fast path — only fires when callee is a known builtin
	// Ident, just like compileCall.
	if calleeID, ok := calleeNode.(*ast.Ident); ok {
		if idx, isBuiltin := BuiltinIndex[calleeID.Value]; isBuiltin {
			if err := c.compileNode(p.Left); err != nil {
				return err
			}
			for _, arg := range extraArgs {
				if err := c.compileNode(arg); err != nil {
					return err
				}
			}
			c.emitOp(OpCallBuiltin, p.Pos)
			c.emitU16(idx)
			c.emitU8(uint8(argc))
			return nil
		}
	}

	// User-function path. OpCall expects [callable, arg0, …, argN]
	// so the callee must be on the stack FIRST. See the function-
	// header comment for the tree-walker-ordering note.
	if err := c.compileNode(calleeNode); err != nil {
		return err
	}
	if err := c.compileNode(p.Left); err != nil {
		return err
	}
	for _, arg := range extraArgs {
		if err := c.compileNode(arg); err != nil {
			return err
		}
	}
	c.emitOp(OpCall, p.Pos)
	c.emitU8(uint8(argc))
	return nil
}

// compileFunctionLiteral compiles a `fn(params) { body }` literal
// into a sibling BytecodeChunk and emits a PushConst that puts the
// resulting CompiledFunction on the stack.
//
// Approach
//
//  1. Create a fresh Compiler instance (newCompiler() — its own
//     chunk, constant pool, locals map, and loopStack).
//  2. Pre-bind every parameter as a local in slot 0..N-1. At call
//     time, the VM places the actual args into those slots before
//     execution begins, so the body code just LoadLocal-s by slot.
//  3. Compile each statement in the body.
//  4. Emit a tail PushNull + Return so a function that falls off
//     the bottom without a `return` still produces NULL — matches
//     the tree-walker's "missing return → null" semantics.
//  5. Wrap the resulting chunk in a CompiledFunction and pool it
//     as a constant on the OUTER chunk; emit PushConst <idx>.
//
// Variadic, closures, and parameter defaults are implemented:
//
//   - Variadic — n.Variadic flows through to CompiledFunction.Variadic;
//     OpCall's validateArity + bindCallArgs handle "rest" param
//     collection. OpMakeClosure also copies the flag (was a bug —
//     closures of variadic templates used to lose the flag).
//   - Closures — sub-compiler walks outer scopes via resolveUpvalue
//     and emits OpMakeClosure with upvalue descriptors when the
//     body captures anything; non-capturing functions still ship
//     as a PushConst of the CompiledFunction in the constant pool
//     (no per-call closure-allocation cost).
//   - Parameter defaults (constant-literal form) — n.Defaults is
//     walked by resolveConstantDefaults at function-construction
//     time. Each literal default (null / int / float / string /
//     bool / bytes) becomes an entry in CompiledFunction.DefaultValues
//     paralleling the param list; CompiledFunction.NumRequired
//     records the minimum argc. OpCall's validateArity accepts
//     [NumRequired, NumParams]; bindStackArgs / bindArgs fill
//     missing trailing slots from DefaultValues. Non-literal
//     defaults (e.g. `arr = []`, `n = computeIt()`) error at
//     compile time with a migration message — see
//     resolveConstantDefaults for the supported set.
//
// Still deferred:
//
//   - Non-constant defaults (general-expression defaults — would
//     require compiling each default as a sub-chunk evaluated at
//     call time). Tracked as a future upgrade; the on-disk wire
//     format already reserves the right shape via DefaultValues.
//
// Type annotations (n.ParamTypes / n.ReturnType) ARE enforced: they
// flow onto CompiledFunction.{Params,ParamTypes,ReturnType,TypeChecked}
// and chunk.ReturnType, checked in bindStackArgs/bindMethodArgs/bindArgs
// (args) and at OpReturn (return) via the shared eval.* checkers — full
// parity with the tree-walker.
func (c *Compiler) compileFunctionLiteral(f *ast.FunctionLiteral) error {
	return c.compileFunctionLiteralNamed(f, "", "")
}

// compileFunctionLiteralNamed is the inner form that knows the
// name the function is being assigned to (if any). Named functions
// get a "self slot" so recursion works without full closures:
//
//	fn fact(n) {           ── outer assigns CompiledFunction to local 'fact'
//	    if n <= 1 { return 1 }
//	    return n * fact(n - 1)   ── body's 'fact' resolves to sub-local self slot
//	}                                 (populated by OpCall at runtime)
//
// AssignStmt / LetStmt detect the FunctionLiteral RHS shape and
// call this directly with the assigned name. Bare anonymous
// `fn(x) { x }` literals call compileFunctionLiteral (selfName="")
// and get no self slot — recursion isn't possible for them anyway
// since they have no name to recurse through.
// selfName drives the recursion self-slot (non-empty → the function can
// call itself by that bare name). displayName is the name shown in
// diagnostics (stack traces, Inspect, type-annotation errors). They're
// usually identical, but methods pass selfName="" (no bare-name recursion —
// methods recurse via self.method()) with displayName=methodName so error
// messages still name the method instead of "anonymous".
func (c *Compiler) compileFunctionLiteralNamed(f *ast.FunctionLiteral, selfName, displayName string) error {
	sub := newCompiler()
	sub.outer = c // enables resolveUpvalue to walk back to outer scope
	for i, p := range f.Params {
		sub.locals[p] = i
	}
	sub.nextSlot = len(f.Params)
	sub.chunk.NumLocals = len(f.Params)

	selfSlot := -1
	if selfName != "" {
		selfSlot = sub.nextSlot
		sub.locals[selfName] = selfSlot
		sub.nextSlot++
		sub.chunk.NumLocals = sub.nextSlot
	}

	if err := sub.compileStmtList(f.Body, true); err != nil {
		return err
	}
	// With keepLast=true, the body's final statement may have left
	// a value on the stack — that's the implicit return (matches the
	// tree-walker's "function returns last statement's value"). When
	// the last statement produced nothing (e.g. AssignStmt), Return
	// uses an empty stack and returns NULL. Either way, no implicit
	// PushNull required.
	sub.emitOp(OpReturn, f.Pos)

	// Resolve default-value parameters. The parser enforces
	// "defaults-must-come-last", so n.Defaults[i] is nil for i <
	// numRequired and non-nil for i >= numRequired. We capture the
	// constant value of each default at compile time; non-constant
	// defaults are rejected with a migration message.
	//
	// Variadic functions intentionally ignore defaults — the rest
	// param collects any trailing args (including zero), so per-param
	// defaults don't combine cleanly. Tree-walker has the same
	// limitation.
	var defaultValues []eval.Object
	numRequired := len(f.Params)
	if !f.Variadic {
		dv, numReq, derr := resolveConstantDefaults(f, displayName)
		if derr != nil {
			return derr
		}
		defaultValues = dv
		numRequired = numReq
	} else if f.Variadic {
		numRequired = len(f.Params) - 1
		if numRequired < 0 {
			numRequired = 0
		}
	}

	// Carry the optional type annotations so the VM enforces them just
	// like the tree-walker. ReturnType also lives on the chunk so
	// OpReturn can read it via the loop-local `chunk` pointer.
	sub.chunk.ReturnType = f.ReturnType
	sub.chunk.FnName = displayName

	cf := &CompiledFunction{
		Name:          displayName,
		Chunk:         sub.chunk,
		NumParams:     len(f.Params),
		NumRequired:   numRequired,
		DefaultValues: defaultValues,
		Variadic:      f.Variadic,
		SelfSlot:      selfSlot,
		UpvalueRefs:   sub.upvalueRefs,
		Params:        f.Params,
		ParamTypes:    f.ParamTypes,
		ReturnType:    f.ReturnType,
		TypeChecked:   eval.HasTypeAnnotations(f.ParamTypes, f.ReturnType),
	}
	idx := c.poolConstant(cf)

	// Emit MakeClosure ONLY when the body captures anything. The
	// non-capturing case stays as a cheap PushConst (no per-call
	// allocation, no Upvalues slice to populate at runtime) — most
	// functions don't close over anything and we keep that fast path.
	if len(sub.upvalueRefs) == 0 {
		c.emitOp(OpPushConst, f.Pos)
		c.emitU16(idx)
	} else {
		c.emitOp(OpMakeClosure, f.Pos)
		c.emitU16(idx)
	}
	return nil
}

// resolveConstantDefaults walks f.Defaults and returns a parallel
// []eval.Object where each entry is either nil (the param is required)
// or a constant default value captured from a literal AST node.
// Returns (nil, numRequired, nil) when the function has no defaults
// at all — keeps the construction path allocation-free in the common
// case. Returns a CompileError when a default isn't a constant
// literal; the message points the user at the workaround.
//
// Supported literal AST nodes:
//
//   - *ast.NullLiteral   → eval.NULL
//   - *ast.BoolLiteral   → eval.TRUE / eval.FALSE
//   - *ast.IntLiteral    → *eval.Integer
//   - *ast.FloatLiteral  → *eval.Float
//   - *ast.StringLiteral → *eval.String
//   - *ast.BytesLiteral  → *eval.Bytes
//
// Anything else (CallExpr, ArrayLiteral, HashLiteral, Ident, ...)
// errors with a message suggesting the user rewrite as `x = null`
// and handle the fallback inside the body. This restriction lifts
// when DefaultValues becomes DefaultChunks (sub-chunks evaluated at
// call time) — tracked as a future upgrade.
func resolveConstantDefaults(f *ast.FunctionLiteral, fnName string) ([]eval.Object, int, *CompileError) {
	// Quick exit: no defaults at all.
	anyDefault := false
	for _, d := range f.Defaults {
		if d != nil {
			anyDefault = true
			break
		}
	}
	if !anyDefault {
		return nil, len(f.Params), nil
	}

	out := make([]eval.Object, len(f.Params))
	numRequired := len(f.Params)
	for i, d := range f.Defaults {
		if d == nil {
			continue
		}
		if numRequired == len(f.Params) {
			numRequired = i
		}
		val, ok := constantLiteralValue(d)
		if !ok {
			displayName := fnName
			if displayName == "" {
				displayName = "<anon>"
			}
			paramName := f.Params[i]
			return nil, 0, &CompileError{
				Pos: nodePos(d),
				Message: fmt.Sprintf(
					"vm: function %s parameter '%s' has a non-literal default expression — "+
						"only constant literals (null, ints, floats, strings, bools, bytes) "+
						"are supported under --vm right now. Workaround: declare as `%s = null` "+
						"and resolve the real default inside the body.",
					displayName, paramName, paramName),
			}
		}
		out[i] = val
	}
	return out, numRequired, nil
}

// constantLiteralValue returns the kLex Object that a literal AST
// node would evaluate to, or (_, false) if the node isn't a literal
// suitable for use as a compile-time default.
func constantLiteralValue(n ast.Node) (eval.Object, bool) {
	switch v := n.(type) {
	case *ast.NullLiteral:
		return eval.NULL, true
	case *ast.BoolLiteral:
		if v.Value {
			return eval.TRUE, true
		}
		return eval.FALSE, true
	case *ast.IntLiteral:
		return eval.NewInteger(int(v.Value)), true
	case *ast.FloatLiteral:
		return &eval.Float{Value: v.Value}, true
	case *ast.StringLiteral:
		return &eval.String{Value: v.Value}, true
	case *ast.BytesLiteral:
		// Copy the bytes so the default value can't be mutated by
		// holding-on-to-it tricks affecting later calls.
		buf := make([]byte, len(v.Value))
		copy(buf, v.Value)
		return &eval.Bytes{Value: buf}, true
	}
	return nil, false
}

// nodePos best-effort extracts the source position of an AST node,
// returning the zero Pos when the node type doesn't surface one.
// Used by resolveConstantDefaults so the error points at the
// offending default expression rather than the function as a whole.
func nodePos(n ast.Node) ast.Pos {
	switch v := n.(type) {
	case *ast.NullLiteral:
		return v.Pos
	case *ast.BoolLiteral:
		return v.Pos
	case *ast.IntLiteral:
		return v.Pos
	case *ast.FloatLiteral:
		return v.Pos
	case *ast.StringLiteral:
		return v.Pos
	case *ast.BytesLiteral:
		return v.Pos
	case *ast.Ident:
		return v.Pos
	case *ast.CallExpr:
		return v.Pos
	case *ast.ArrayLiteral:
		return v.Pos
	case *ast.HashLiteral:
		return v.Pos
	case *ast.InfixExpr:
		return v.Pos
	case *ast.PrefixExpr:
		return v.Pos
	}
	return ast.Pos{}
}

// compileArrayLiteral pushes each element in source order, then
// emits MakeArray with the count as a uint16 operand. The VM
// handler pops `count` values and assembles them into a *Array.
//
// Source order = stack push order = result element order. The
// MakeArray handler walks the slice top-down so element 0 ends up
// at stack[base], element 1 at stack[base+1], etc.
//
// uint16 limit: 65 535 elements per literal. Programs that exceed
// it (unlikely in source) should use makeArray() at runtime — same
// as today's tree-walker for very large arrays.
func (c *Compiler) compileArrayLiteral(a *ast.ArrayLiteral) error {
	if len(a.Elements) > 0xFFFF {
		return c.unimplementedAt(a.Pos, "array literal with > 65535 elements (use makeArray() at runtime)")
	}
	for _, el := range a.Elements {
		if err := c.compileNode(el); err != nil {
			return err
		}
	}
	c.emitOp(OpMakeArray, a.Pos)
	c.emitU16(uint16(len(a.Elements)))
	return nil
}

// compileHashLiteral pushes key, value, key, value, … in source
// order, then emits MakeHash with the PAIR count.
//
// The VM handler walks 2*N stack slots, calling toHashKey on each
// key to validate the type (kLex hashes accept string / integer /
// boolean keys; anything else is a TypeError matching the tree-
// walker's evalHashLiteral).
func (c *Compiler) compileHashLiteral(h *ast.HashLiteral) error {
	if len(h.Pairs) > 0xFFFF {
		return c.unimplementedAt(h.Pos, "hash literal with > 65535 pairs")
	}
	for _, p := range h.Pairs {
		if err := c.compileNode(p.Key); err != nil {
			return err
		}
		if err := c.compileNode(p.Value); err != nil {
			return err
		}
	}
	c.emitOp(OpMakeHash, h.Pos)
	c.emitU16(uint16(len(h.Pairs)))
	return nil
}

// compileTupleLiteral mirrors compileArrayLiteral but emits
// MakeTuple. Tuples are immutable + return-multi-value shape; the
// language uses them for `return a, b, c` and destructuring assign.
func (c *Compiler) compileTupleLiteral(t *ast.TupleLiteral) error {
	if len(t.Elements) > 0xFFFF {
		return c.unimplementedAt(t.Pos, "tuple literal with > 65535 elements")
	}
	for _, el := range t.Elements {
		if err := c.compileNode(el); err != nil {
			return err
		}
	}
	c.emitOp(OpMakeTuple, t.Pos)
	c.emitU16(uint16(len(t.Elements)))
	return nil
}

// compileIndex emits the bytecode for `container[index]`. Pushes
// container then index, emits OpIndex which dispatches through
// eval.EvalIndex — the same shared resolver the tree-walker uses,
// so type/bound rules are identical across both interpreters.
func (c *Compiler) compileIndex(i *ast.IndexExpr) error {
	if err := c.compileNode(i.Left); err != nil {
		return err
	}
	if err := c.compileNode(i.Index); err != nil {
		return err
	}
	c.emitOp(OpIndex, i.Pos)
	return nil
}

// compileReturn emits `return [expr]`. Today, at the top-level
// chunk (no function frames yet), OpReturn behaves identically to
// OpHalt — it pushes the value and exits the chunk. When user-
// defined functions land, OpReturn becomes "pop current frame,
// push return value onto caller's stack" and the same compiler arm
// keeps working unchanged.
//
// Bare `return` (no expression) pushes NULL — matching the tree-
// walker's behaviour that `return` without a value yields null.
func (c *Compiler) compileReturn(r *ast.ReturnStmt) error {
	if r.Value == nil {
		c.emitOp(OpPushNull, r.Pos)
	} else {
		if err := c.compileNode(r.Value); err != nil {
			return err
		}
	}
	c.emitOp(OpReturn, r.Pos)
	return nil
}

// compileConst is identical to compileLet structurally but marks
// the resulting slot in c.constSlots so any subsequent AssignStmt
// or LetStmt targeting the same name fails at COMPILE time. The
// tree-walker enforces this at runtime via env.CheckWritable; we
// catch it earlier because the slot-and-name resolution is static.
//
// `const _ = expr` evaluates for side effects and discards the
// value — no name to bind, no constness to track.
func (c *Compiler) compileConst(s *ast.ConstStmt) error {
	if err := c.compileNode(s.Value); err != nil {
		return err
	}
	if s.Name == "_" {
		c.emitOp(OpPop, s.Pos)
		return nil
	}
	if _, exists := c.locals[s.Name]; exists {
		return c.unimplementedAt(s.Pos, "cannot re-declare existing binding as const: "+s.Name)
	}
	slot := c.allocAnonSlot()
	c.locals[s.Name] = slot
	c.constSlots[slot] = true
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(slot))
	// Mark the cell as const at runtime. The compile-time
	// constSlots tracking still gates redeclaration, but reassign
	// from inner closures resolves through resolveUpvalue without
	// access to constSlots — so the runtime IsConst flag on the
	// shared UpvalueCell is the canonical guard. The name lives in
	// the constant pool for the runtime error message.
	nameIdx := c.poolConstant(&eval.String{Value: s.Name})
	c.emitOp(OpMarkConst, s.Pos)
	c.emitU16(uint16(slot))
	c.emitU16(nameIdx)
	return nil
}

// compileMultiAssign handles `a, b = expr` by stashing the RHS
// tuple in a temp slot, then for each name emitting:
//
//	LoadLocal tempSlot ; PushInt i ; OpIndex ; StoreLocal nameSlot
//
// The tree-walker requires the RHS to evaluate to a Tuple with
// exactly len(Names) elements. We don't have arity verification at
// compile time, so the runtime OpIndex out-of-bounds error stands
// in — same vmdiff-visible outcome.
//
// `a, _ = expr` discards the second slot: we still OpIndex (for any
// side effects + bounds check) and then OpPop.
// compileMultiLet handles `let a, b = expr`. Like compileLet, every name
// is bound in the CURRENT scope — never resolved through upvalues. If the
// name already has a slot in this scope we reuse it (re-binding the value
// in the existing slot); otherwise we allocate a fresh slot. OpFreshCell
// per name preserves the per-iteration closure-capture semantics that
// compileLet relies on inside loop bodies. `_` discards by popping.
func (c *Compiler) compileMultiLet(s *ast.MultiLetStmt) error {
	if err := c.compileNode(s.Value); err != nil {
		return err
	}
	c.emitOp(OpUnpackTuple, s.Pos)
	c.emitU16(uint16(len(s.Names)))
	// Bind in REVERSE order — OpUnpackTuple leaves the last element
	// on top of the stack, matching compileMultiAssign's ordering.
	for i := len(s.Names) - 1; i >= 0; i-- {
		name := s.Names[i]
		if name == "_" {
			c.emitOp(OpPop, s.Pos)
			continue
		}
		slot, ok := c.locals[name]
		if !ok {
			slot = c.nextSlot
			c.locals[name] = slot
			c.nextSlot++
			if c.nextSlot > c.chunk.NumLocals {
				c.chunk.NumLocals = c.nextSlot
			}
		}
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(slot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(slot))
	}
	return nil
}

func (c *Compiler) compileMultiAssign(s *ast.MultiAssignStmt) error {
	if err := c.compileNode(s.Value); err != nil {
		return err
	}
	// OpUnpackTuple enforces "RHS is a Tuple with exactly N
	// elements" — same rule the tree-walker enforces inside its
	// MultiAssignStmt arm. Pushes the elements in source order, so
	// the LAST element sits on top of the stack.
	c.emitOp(OpUnpackTuple, s.Pos)
	c.emitU16(uint16(len(s.Names)))
	// Bind in REVERSE order: pop the top (last element) into the
	// last name, then second-to-last, etc.
	for i := len(s.Names) - 1; i >= 0; i-- {
		name := s.Names[i]
		if name == "_" {
			c.emitOp(OpPop, s.Pos)
			continue
		}
		// Match compileAssign's resolution order: locals first (in-
		// scope mutation), then upvalues (walk outward — required
		// for the "outer const survives inner reassignment attempt"
		// case in constTest), then create a fresh local. Without
		// the upvalue check, multi-assigning to an outer const from
		// within a closure would silently bind a fresh shadow
		// instead of routing through OpSetUpvalue (and its IsConst
		// runtime guard).
		if slot, ok := c.locals[name]; ok {
			c.emitOp(OpStoreLocal, s.Pos)
			c.emitU16(uint16(slot))
			continue
		}
		if idx, ok := c.resolveUpvalue(name); ok {
			c.emitOp(OpSetUpvalue, s.Pos)
			c.emitU16(idx)
			continue
		}
		// M6: new-slot allocation needs OpFreshCell for per-iteration
		// closure-capture correctness. See compileAssign for the rule.
		slot := c.allocAnonSlot()
		c.locals[name] = slot
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(slot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(slot))
	}
	return nil
}

// compileIndexAssign handles `container[index] = value`. Pushes
// container, index, and value in source order, then emits
// OpIndexStore which routes through eval.EvalIndexAssign — the same
// resolver the tree-walker uses, so frozen-checks, bounds-checks,
// and hash-key validation are identical across both interpreters.
func (c *Compiler) compileIndexAssign(s *ast.IndexAssignStmt) error {
	if err := c.compileNode(s.Left.Left); err != nil {
		return err
	}
	if err := c.compileNode(s.Left.Index); err != nil {
		return err
	}
	if err := c.compileNode(s.Value); err != nil {
		return err
	}
	c.emitOp(OpIndexStore, s.Pos)
	return nil
}

// compileForIn lowers `for x in coll { body }` (and the two-var
// form `for k, v in coll`) to a while-loop over an integer index,
// using the `len` builtin and OpIndex for value access. Today this
// only supports indexable collections (Array, String, Bytes,
// Tuple). Hash iteration needs a different lowering (iterate
// keys()) and is deferred until we have a proper iterator opcode
// or a runtime-dispatching helper.
//
// Generated bytecode shape:
//
//	<collection>                       ── push collection
//	StoreLocal _iter                   ── stash at anonymous slot
//	PushInt 0; StoreLocal _idx         ── anonymous index counter
//	loop_top:
//	    LoadLocal _idx                 ── left  of `_idx < len(_iter)`
//	    LoadLocal _iter; CallBuiltin len 1   ── right
//	    Lt
//	    JumpIfFalse [end]
//	    LoadLocal _iter; LoadLocal _idx; Index  ── element
//	    StoreLocal <varSlot>           ── bind to user's loop var
//	    (two-var: also StoreLocal _idx → valueSlot's role swap, see below)
//	    <body>
//	    LoadLocal _idx; PushInt 1; Add; StoreLocal _idx
//	    Jump [loop_top]
//	end:
//
// Two-var convention (matches ast.ForInStmt docs):
//
//	for x in arr           → Variable="x"  ValueVar=""   ── bind arr[i] to x
//	for i, v in arr        → Variable="i"  ValueVar="v"  ── i = index, v = arr[i]
//
// Anonymous slots (_iter, _idx) are allocated by the compiler but
// not registered in c.locals, so user code can't accidentally
// reference them by name.
func (c *Compiler) compileForIn(s *ast.ForInStmt) error {
	lenIdx, ok := BuiltinIndex["len"]
	if !ok {
		return c.unimplementedAt(s.Pos, "for-in: builtin len missing from index — vmbuiltins regen needed?")
	}
	iterPrepIdx, ok := BuiltinIndex["_iterPrep"]
	if !ok {
		return c.unimplementedAt(s.Pos, "for-in: builtin _iterPrep missing from index — vmbuiltins regen needed?")
	}

	twoVar := s.ValueVar != ""

	// 1. Evaluate collection, call _iterPrep(coll, twoVar), unpack the
	//    returned (iterArray, isPairs) tuple into anonymous slots.
	//    _iterPrep normalises *Hash to an array of (k, v) 2-tuples and
	//    flips isPairs to TRUE; other indexable collections pass
	//    through with isPairs=FALSE. This lets the rest of the loop
	//    use the same integer-indexed lowering for every collection
	//    shape, with a single conditional in the bind step.
	if err := c.compileNode(s.Collection); err != nil {
		return err
	}
	if twoVar {
		c.emitOp(OpPushTrue, s.Pos)
	} else {
		c.emitOp(OpPushFalse, s.Pos)
	}
	c.emitOp(OpCallBuiltin, s.Pos)
	c.emitU16(iterPrepIdx)
	c.emitU8(2)
	// _iterPrep on an error type returns an Error; the surrounding
	// OpReturnIfError will catch it because the loop body is wrapped
	// in compileStmtList. But this expression is mid-statement, so
	// route through the explicit early-return chain — push the result
	// and let the next OpReturnIfError fire if it's an Error. We don't
	// have an OpReturnIfError emit here today; instead, the runtime
	// will treat the Error as an unexpected value, OpUnpackTuple will
	// fail with "expected Tuple" and surface the error. Either way the
	// program halts cleanly with the underlying message.
	c.emitOp(OpUnpackTuple, s.Pos)
	c.emitU16(2)
	// Stack now (top → bottom): isPairs, iterArray.
	isPairsSlot := c.allocAnonSlot()
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(isPairsSlot))
	iterSlot := c.allocAnonSlot()
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(iterSlot))

	// 2. _idx = 0.
	idxSlot := c.allocAnonSlot()
	c.emitOp(OpPushInt, s.Pos)
	c.emitI32(0)
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(idxSlot))

	// 3. Reserve slots for the user's loop variables.
	//
	// `_` is the formal discard placeholder. Allocating a real local
	// slot for it would let the body READ `_` (giving it whatever the
	// loop just bound), which contradicts the tree-walker's contract
	// that `_` is write-only — every read of `_` is a RuntimeError.
	// Routing `_` through an anonymous slot keeps the per-iteration
	// store cost the same but blocks any later `_` reference from
	// resolving to it (compileIdent only looks up named locals).
	var varSlot, valueSlot int
	if twoVar {
		// Variable holds the INDEX, ValueVar holds the ELEMENT.
		if s.Variable == "_" {
			varSlot = c.allocAnonSlot()
		} else {
			varSlot = c.allocSlotForName(s.Variable)
		}
		if s.ValueVar == "_" {
			valueSlot = c.allocAnonSlot()
		} else {
			valueSlot = c.allocSlotForName(s.ValueVar)
		}
	} else {
		if s.Variable == "_" {
			varSlot = c.allocAnonSlot()
		} else {
			varSlot = c.allocSlotForName(s.Variable)
		}
	}

	// 4. Loop top + condition: _idx < len(_iter).
	loopTop := len(c.chunk.Code)
	c.loopStack = append(c.loopStack, loopContext{})

	c.emitOp(OpLoadLocal, s.Pos)
	c.emitU16(uint16(idxSlot))
	c.emitOp(OpLoadLocal, s.Pos)
	c.emitU16(uint16(iterSlot))
	c.emitOp(OpCallBuiltin, s.Pos)
	c.emitU16(lenIdx)
	c.emitU8(1)
	c.emitOp(OpLt, s.Pos)
	exitPatch := c.emitJump(OpJumpIfFalse, s.Pos)

	// 5. Bind loop vars: element comes from _iter[_idx].
	//
	// For two-var iteration we branch on _isPairs at runtime:
	//   * isPairs=TRUE  (hash source, normalised to [[k, v]…]):
	//     unpack the 2-tuple into (varSlot, valueSlot).
	//   * isPairs=FALSE (array/string/bytes/tuple source):
	//     varSlot = idx, valueSlot = element.
	c.emitOp(OpLoadLocal, s.Pos)
	c.emitU16(uint16(iterSlot))
	c.emitOp(OpLoadLocal, s.Pos)
	c.emitU16(uint16(idxSlot))
	c.emitOp(OpIndex, s.Pos)
	if twoVar {
		// Now stack: [..., element]. Branch on isPairs.
		c.emitOp(OpLoadLocal, s.Pos)
		c.emitU16(uint16(isPairsSlot))
		// JumpIfFalse → array-binding path (consumes the bool).
		arrayBindPatch := c.emitJump(OpJumpIfFalse, s.Pos)

		// Hash-binding path: element is a 2-tuple (k, v).
		// OpUnpackTuple pushes elements in source order so v ends
		// on top; pop into valueSlot first, then varSlot.
		//
		// H2 (audit fix, 2026-05-23): OpFreshCell before each store
		// so closures captured during iteration N see a distinct
		// cell from iteration N+1's binding. Without the fresh cell,
		// every closure aliases the slot's only cell and observes
		// the loop's final value (`for i in [1,2,3] { push(out,
		// fn(){ return i }) }` would produce three closures all
		// returning 3 instead of 1, 2, 3 respectively).
		c.emitOp(OpUnpackTuple, s.Pos)
		c.emitU16(2)
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(valueSlot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(valueSlot))
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(varSlot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(varSlot))
		bindDonePatch := c.emitJump(OpJump, s.Pos)

		// Array-binding path: element → valueSlot, idx → varSlot.
		c.patchJump(arrayBindPatch)
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(valueSlot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(valueSlot))
		c.emitOp(OpLoadLocal, s.Pos)
		c.emitU16(uint16(idxSlot))
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(varSlot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(varSlot))

		c.patchJump(bindDonePatch)
	} else {
		// Single-var form. _iterPrep guarantees isPairs is FALSE
		// here (it errors on single-var iteration of a hash), so the
		// element is the value to bind directly. H2: OpFreshCell so
		// per-iteration closure-capture sees a fresh cell.
		c.emitOp(OpFreshCell, s.Pos)
		c.emitU16(uint16(varSlot))
		c.emitOp(OpStoreLocal, s.Pos)
		c.emitU16(uint16(varSlot))
	}

	// 6. Body.
	if err := c.compileStmtList(s.Body, false); err != nil {
		c.loopStack = c.loopStack[:len(c.loopStack)-1]
		return err
	}

	// 7. Increment: _idx = _idx + 1.
	// Continue jumps land HERE — so every iteration's index advances
	// even when the body shortcuts to continue.
	incLabel := len(c.chunk.Code)
	c.emitOp(OpLoadLocal, s.Pos)
	c.emitU16(uint16(idxSlot))
	c.emitOp(OpPushInt, s.Pos)
	c.emitI32(1)
	c.emitOp(OpAdd, s.Pos)
	c.emitOp(OpStoreLocal, s.Pos)
	c.emitU16(uint16(idxSlot))

	// 8. Jump back to loop top.
	c.emitOp(OpJump, s.Pos)
	backPatch := len(c.chunk.Code)
	c.emitI32(0)
	rel := int32(loopTop - (backPatch + 4))
	c.chunk.Code[backPatch+0] = byte(rel)
	c.chunk.Code[backPatch+1] = byte(rel >> 8)
	c.chunk.Code[backPatch+2] = byte(rel >> 16)
	c.chunk.Code[backPatch+3] = byte(rel >> 24)

	// 9. End: patch the exit JumpIfFalse, resolve continue patches
	//    (which land on the increment label so the next iteration's
	//    index advances), and resolve break patches.
	c.patchJump(exitPatch)
	ctx := c.loopStack[len(c.loopStack)-1]
	for _, p := range ctx.continuePatches {
		c.patchJumpTo(p, incLabel)
	}
	for _, p := range ctx.breakPatches {
		c.patchJump(p)
	}
	c.loopStack = c.loopStack[:len(c.loopStack)-1]
	return nil
}

// patchJumpTo is patchJump's explicit-destination cousin. patchJump
// resolves to "wherever we are now," which is right for breaks (they
// jump to end). For continues in for-in we want to jump to the
// increment block, which is BEFORE the end — hence this variant.
func (c *Compiler) patchJumpTo(patchAt, dest int) {
	rel := int32(dest - (patchAt + 4))
	c.chunk.Code[patchAt+0] = byte(rel)
	c.chunk.Code[patchAt+1] = byte(rel >> 8)
	c.chunk.Code[patchAt+2] = byte(rel >> 16)
	c.chunk.Code[patchAt+3] = byte(rel >> 24)
}

// allocAnonSlot reserves a local slot WITHOUT registering it in
// c.locals — used for compiler-internal temporaries (for-in's _iter
// / _idx, future try/catch saved state, etc.) that user code must
// not be able to reference by name.
func (c *Compiler) allocAnonSlot() int {
	slot := c.nextSlot
	c.nextSlot++
	if c.nextSlot > c.chunk.NumLocals {
		c.chunk.NumLocals = c.nextSlot
	}
	return slot
}

// allocSlotForName binds `name` to a slot in c.locals — allocating a
// new one if the name doesn't already exist. Used by for-in and the
// future MultiAssignStmt arm. AssignStmt / LetStmt have their own
// inlined allocator that also handles the "value already on stack"
// invariant; this helper just reserves storage.
func (c *Compiler) allocSlotForName(name string) int {
	if slot, ok := c.locals[name]; ok {
		return slot
	}
	slot := c.allocAnonSlot()
	c.locals[name] = slot
	return slot
}

// compileWhile emits the bytecode for `while cond { body }`.
//
// Layout:
//
//	loop_top:                    ── continue jumps here
//	    <compile cond>
//	    JumpIfFalse [loop_end]
//	    <compile body>
//	    Jump [loop_top]          ── backward jump; offset is negative
//	loop_end:                    ── break jumps here
//
// Continue jumps to loop_top (offset known at emit time).
// Break jumps to loop_end (offset only known after the loop body
// is fully emitted, so we collect break-patches in the loopContext
// and resolve them all in a final pass).
//
// The loopContext is pushed onto c.loopStack on entry, popped on
// exit — that's what makes nested loops work: break/continue always
// refer to the INNERMOST loop, which is the top of the stack.
func (c *Compiler) compileWhile(s *ast.WhileStmt) error {
	loopTop := len(c.chunk.Code)
	c.loopStack = append(c.loopStack, loopContext{})

	// Condition.
	if err := c.compileNode(s.Condition); err != nil {
		c.loopStack = c.loopStack[:len(c.loopStack)-1]
		return err
	}
	// Skip past loop body if condition is false.
	exitPatch := c.emitJump(OpJumpIfFalse, s.Pos)

	// Body.
	if err := c.compileStmtList(s.Body, false); err != nil {
		c.loopStack = c.loopStack[:len(c.loopStack)-1]
		return err
	}

	// Loop back to the top. emitJump writes a 0 placeholder; we patch
	// it manually here because the destination is BEHIND us, not
	// ahead.
	c.emitOp(OpJump, s.Pos)
	backPatch := len(c.chunk.Code)
	c.emitI32(0)
	// rel from end-of-operand to loopTop is negative.
	rel := int32(loopTop - (backPatch + 4))
	c.chunk.Code[backPatch+0] = byte(rel)
	c.chunk.Code[backPatch+1] = byte(rel >> 8)
	c.chunk.Code[backPatch+2] = byte(rel >> 16)
	c.chunk.Code[backPatch+3] = byte(rel >> 24)

	// Patch the JumpIfFalse exit to land here.
	c.patchJump(exitPatch)

	// Patch continues to the loop top (re-check condition), then
	// break patches to land at end-of-loop.
	ctx := c.loopStack[len(c.loopStack)-1]
	for _, p := range ctx.continuePatches {
		c.patchJumpTo(p, loopTop)
	}
	for _, p := range ctx.breakPatches {
		c.patchJump(p)
	}
	c.loopStack = c.loopStack[:len(c.loopStack)-1]
	return nil
}

// compileBreak emits a placeholder Jump and records its patch
// offset on the innermost loop's break list. The break offset gets
// resolved when compileWhile (or future compileForIn) finishes
// emitting the loop body and knows where loop_end is.
func (c *Compiler) compileBreak(b *ast.BreakStmt) error {
	if len(c.loopStack) == 0 {
		return c.unimplementedAt(b.Pos, "break outside loop")
	}
	patch := c.emitJump(OpJump, b.Pos)
	top := len(c.loopStack) - 1
	c.loopStack[top].breakPatches = append(c.loopStack[top].breakPatches, patch)
	return nil
}

// compileContinue emits a placeholder Jump and records its patch on
// the innermost loop's continuePatches list. The loop's compile
// arm patches them to the appropriate destination:
//
//   - compileWhile  → patches to loop top (re-check condition)
//   - compileForIn  → patches to the increment block (so the index
//     advances before the next iteration's check)
//
// Uniform patch-list model means break and continue work the same
// way structurally, and the per-loop arm chooses the destination.
func (c *Compiler) compileContinue(co *ast.ContinueStmt) error {
	if len(c.loopStack) == 0 {
		return c.unimplementedAt(co.Pos, "continue outside loop")
	}
	patch := c.emitJump(OpJump, co.Pos)
	top := len(c.loopStack) - 1
	c.loopStack[top].continuePatches = append(c.loopStack[top].continuePatches, patch)
	return nil
}

// compileShortCircuit emits `&&` and `||` so the RHS is evaluated only
// when its truth value is needed. Both forms leave the result on the
// stack (the kLex tree-walker has the same property — the operator
// returns a Bool).
//
// Encoding for `L && R`:
//
//	<compile L>                ── leaves Bool on stack
//	JumpIfFalsePeek [end]      ── if L is false, leave L on stack and jump
//	Pop                        ── L was true, discard it
//	<compile R>                ── leaves Bool on stack
//	end:
//
// Encoding for `L || R`:
//
//	<compile L>
//	JumpIfTruePeek [end]       ── if L is true, leave L on stack and jump
//	Pop
//	<compile R>
//	end:
//
// The Peek variants are crucial: we want the SHORT-CIRCUITED value
// (L) to BE the expression's result if the short-circuit fires. A
// regular JumpIfFalse / JumpIfTrue would pop the value, and we'd
// have to push a Bool back — wasteful and asymmetric with the
// fall-through path.
//
// Type-checking: the Peek variants check Bool-ness in the VM and
// raise a kLex TypeError if either operand isn't Bool. Matches the
// tree-walker's evalLogical exactly.
func (c *Compiler) compileShortCircuit(n *ast.InfixExpr) error {
	if err := c.compileNode(n.Left); err != nil {
		return err
	}
	var peekOp opcode
	switch n.Operator {
	case "&&":
		peekOp = OpJumpIfFalsePeek
	case "||":
		peekOp = OpJumpIfTruePeek
	default:
		return c.unimplementedAt(n.Pos, "short-circuit "+n.Operator)
	}
	patch := c.emitJump(peekOp, n.Pos)
	c.emitOp(OpPop, n.Pos)
	if err := c.compileNode(n.Right); err != nil {
		return err
	}
	c.patchJump(patch)
	return nil
}

// compileIf emits the bytecode for `if cond { body } [else { elseBody }]`.
//
// Layout for `if cond { B }` (no else):
//
//	<compile cond>            ── leaves bool on stack
//	JumpIfFalse [end-of-B]    ── pops bool, jumps if false
//	<compile B>
//	end-of-B:
//
// Layout for `if cond { B } else { E }`:
//
//	<compile cond>
//	JumpIfFalse [start-of-E]
//	<compile B>
//	Jump        [end-of-E]
//	start-of-E:
//	<compile E>
//	end-of-E:
//
// The two jump targets aren't known when the jump instruction is
// emitted — we emit a placeholder offset (0) and remember the byte
// position, then back-patch once the target is reached. That's the
// standard one-pass-compiler trick.
//
// Jump offsets are RELATIVE to the byte AFTER the i32 operand
// (i.e. relative to the next instruction's first byte). The VM
// adds the offset to pc, which already points past the operand by
// the time it executes the jump. This convention matches how Lua,
// CPython, and most stack VMs encode jumps and keeps both forward
// and backward jumps symmetric.
func (c *Compiler) compileIf(s *ast.IfStmt) error {
	if err := c.compileNode(s.Condition); err != nil {
		return err
	}
	// JumpIfFalse — pops the condition, jumps to else (or end) if false.
	jifPatch := c.emitJump(OpJumpIfFalse, s.Pos)

	// Then-body. Compile with keepLast=true so the branch's last
	// expression stays on the stack — required for the tree-walker
	// compatible "if expression returns a value" pattern that
	// reducer fns rely on (`fn(a,b){ if b>a {b} else {a} }`). When
	// the IfStmt is used as a statement (not its body's last value
	// consumed), the surrounding OpReturnIfError pops the result on
	// success and bubbles errors — same as for any other value-
	// producing statement.
	if err := c.compileStmtList(s.Body, true); err != nil {
		return err
	}

	if s.ElseBody == nil {
		// No else: the if-false path must push NULL so the stack is
		// balanced with the then-path's value. Two-step lowering:
		//   1. After then-body falls through, jump past the else stub.
		//   2. Else stub: PushNull. Both paths converge here.
		jumpPastNull := c.emitJump(OpJump, s.Pos)
		c.patchJump(jifPatch)
		c.emitOp(OpPushNull, s.Pos)
		c.patchJump(jumpPastNull)
		return nil
	}

	jumpPatch := c.emitJump(OpJump, s.Pos)
	c.patchJump(jifPatch)

	// Else-body — same keepLast=true reasoning as the then arm.
	if err := c.compileStmtList(s.ElseBody, true); err != nil {
		return err
	}
	// Patch the Jump to land right after the else-body.
	c.patchJump(jumpPatch)
	return nil
}

// emitJump emits a jump opcode (Jump or JumpIfFalse) with a
// placeholder i32 operand and returns the BYTE OFFSET of that
// placeholder. The caller stashes the returned offset and passes it
// to patchJump once the destination is known.
func (c *Compiler) emitJump(op opcode, pos ast.Pos) int {
	c.emitOp(op, pos)
	patchAt := len(c.chunk.Code)
	c.emitI32(0) // placeholder
	return patchAt
}

// patchJump rewrites the i32 operand at patchAt to be the signed
// offset from the END of the operand (patchAt+4) to the CURRENT end
// of the code stream. Used as the second half of the back-patch
// dance in compileIf and (future) compileWhile.
func (c *Compiler) patchJump(patchAt int) {
	// Destination is wherever we are now.
	dest := len(c.chunk.Code)
	// Relative offset from the byte AFTER the operand to the destination.
	rel := int32(dest - (patchAt + 4))
	// Little-endian, in-place rewrite of the placeholder.
	c.chunk.Code[patchAt+0] = byte(rel)
	c.chunk.Code[patchAt+1] = byte(rel >> 8)
	c.chunk.Code[patchAt+2] = byte(rel >> 16)
	c.chunk.Code[patchAt+3] = byte(rel >> 24)
}

// compileCall handles the call shapes the compiler supports today:
//
//	builtinName(arg1, arg2, …)   — CallBuiltin (fast indexed dispatch)
//	userFn(arg1, arg2, …)        — Call (run the callee's chunk)
//
// Dispatch rule: if the callee is a bare Ident matching a name in
// BuiltinIndex, emit CallBuiltin. Otherwise, compile the callee as
// an expression that should produce a CompiledFunction on the stack,
// then emit Call. The Ident-resolved-to-a-local path is the common
// user-function case (`fn double(x) { … }` → `double` is a local).
//
// We DELIBERATELY do not "fall back to builtin" for unknown idents
// — the compiler emits LoadLocal, and if the local doesn't exist
// the (yet-to-be-real) compile-time variable resolution will catch
// it. Today that path produces an unimplementedAt because
// compileIdent can't find the name; once functions land and tests
// pass, that diagnostic becomes a "undefined name" compile error.
func (c *Compiler) compileCall(call *ast.CallExpr) error {
	if len(call.Args) > 255 {
		return fmt.Errorf("vm/compiler: too many arguments (%d) at line %d — argc is uint8", len(call.Args), call.Pos.Line)
	}
	// Builtin fast path: callee is an Ident naming a registered
	// builtin. Skip the fast path when the name is locally shadowed
	// — `fn assert(...) {}` followed by `assert(x, y)` should dispatch
	// to the user's CompiledFunction, NOT the Go-side `assert` builtin.
	// Matches the tree-walker's env.Get order: locals + upvalues first,
	// builtins only when nothing in scope holds the name.
	if calleeID, ok := call.Function.(*ast.Ident); ok {
		if idx, isBuiltin := BuiltinIndex[calleeID.Value]; isBuiltin {
			_, locally := c.locals[calleeID.Value]
			_, captured := c.resolveUpvalue(calleeID.Value)
			if !locally && !captured {
				// Intrinsic short-circuits: map / filter / reduce
				// have dedicated opcodes that skip eval-side
				// callCallable + the ExternalCallable hook per
				// element. Same observable behaviour, ~one fewer
				// indirection per call to the user callback.
				// Falls through to OpCallBuiltin if the call shape
				// doesn't match (e.g. variadic abuse, wrong argc).
				switch calleeID.Value {
				case "map", "filter":
					if len(call.Args) == 2 {
						for _, arg := range call.Args {
							if err := c.compileNode(arg); err != nil {
								return err
							}
						}
						if calleeID.Value == "map" {
							c.emitOp(OpMap, call.Pos)
						} else {
							c.emitOp(OpFilter, call.Pos)
						}
						return nil
					}
				case "reduce":
					if len(call.Args) == 3 {
						for _, arg := range call.Args {
							if err := c.compileNode(arg); err != nil {
								return err
							}
						}
						c.emitOp(OpReduce, call.Pos)
						return nil
					}
				}
				for _, arg := range call.Args {
					if err := c.compileNode(arg); err != nil {
						return err
					}
				}
				c.emitOp(OpCallBuiltin, call.Pos)
				c.emitU16(idx)
				c.emitU8(uint8(len(call.Args)))
				return nil
			}
		}
	}
	// Method-dispatch / dot-call path: `receiver.name(args)`. The
	// runtime decides whether this is an actual method call on a
	// struct or a property-fetch-then-call (modules, hashes, fields
	// holding callables). We just compile the receiver + args + name
	// and let OpCallMethod sort it out.
	if dot, ok := call.Function.(*ast.DotExpr); ok {
		if err := c.compileNode(dot.Left); err != nil {
			return err
		}
		for _, arg := range call.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
		nameIdx := c.poolConstant(&eval.String{Value: dot.Property})
		c.emitOp(OpCallMethod, call.Pos)
		c.emitU16(nameIdx)
		c.emitU8(uint8(len(call.Args)))
		return nil
	}
	// User-function path. Compile the callee — the resulting value
	// must be a CompiledFunction (the VM enforces this at OpCall
	// dispatch and produces a clean TypeError otherwise).
	if err := c.compileNode(call.Function); err != nil {
		return err
	}
	for _, arg := range call.Args {
		if err := c.compileNode(arg); err != nil {
			return err
		}
	}
	c.emitOp(OpCall, call.Pos)
	c.emitU8(uint8(len(call.Args)))
	return nil
}

// ── Emit helpers ──────────────────────────────────────────────────────────────

// emitOp appends an opcode byte and records its source line.
func (c *Compiler) emitOp(op opcode, pos ast.Pos) {
	c.chunk.Code = append(c.chunk.Code, byte(op))
	c.chunk.Lines = append(c.chunk.Lines, pos.Line)
}

// emitU8 / emitU16 / emitI32 append an operand. Line table is
// extended in lockstep so VM error reporting can read the right line
// for any byte offset.
func (c *Compiler) emitU8(v uint8) {
	c.chunk.Code = append(c.chunk.Code, v)
	c.chunk.Lines = append(c.chunk.Lines, c.lastLine())
}

func (c *Compiler) emitU16(v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	c.chunk.Code = append(c.chunk.Code, buf[:]...)
	c.chunk.Lines = append(c.chunk.Lines, c.lastLine(), c.lastLine())
}

func (c *Compiler) emitI32(v int32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(v))
	c.chunk.Code = append(c.chunk.Code, buf[:]...)
	c.chunk.Lines = append(c.chunk.Lines, c.lastLine(), c.lastLine(), c.lastLine(), c.lastLine())
}

func (c *Compiler) lastLine() int {
	if n := len(c.chunk.Lines); n > 0 {
		return c.chunk.Lines[n-1]
	}
	return 0
}

// poolConstant adds obj to the constant pool and returns its index.
// Dedupes only "value-typed" constants — Integer, Float, String,
// Bool, Null — where Inspect() is a faithful identity. For entries
// whose Inspect() is NOT faithful we MUST NOT dedupe, or distinct
// values silently alias onto one shared slot:
//   - CompiledFunction / StructDef / EnumDef — two anonymous fns both
//     Inspect() as "fn:<anon>" but are completely different programs.
//   - Bytes — Inspect() is "bytes(N)" (length only, by design), so
//     every byte literal of the same length would otherwise collapse
//     to one constant (b"Hi!" aliasing b"\x00\x01\x02", etc.).
//
// Panics if the pool grows past uint16 range — that's a hard wire-
// format limit; programs hitting it need a PushConstW opcode that
// doesn't exist yet.
func (c *Compiler) poolConstant(obj eval.Object) uint16 {
	if !isValueDedupable(obj) {
		// Allocate fresh; never share. Don't even record in
		// constIndex — these never get hit by future lookups
		// anyway, and skipping the entry is slightly faster.
		if len(c.chunk.Constants) >= 0xFFFF {
			panic("vm/compiler: constant pool overflow (>65535) — extend wire format with PushConstW")
		}
		idx := uint16(len(c.chunk.Constants))
		c.chunk.Constants = append(c.chunk.Constants, obj)
		return idx
	}
	key := fmt.Sprintf("%s\x00%s", obj.Type(), obj.Inspect())
	if idx, ok := c.constIndex[key]; ok {
		return idx
	}
	if len(c.chunk.Constants) >= 0xFFFF {
		panic("vm/compiler: constant pool overflow (>65535) — extend wire format with PushConstW")
	}
	idx := uint16(len(c.chunk.Constants))
	c.chunk.Constants = append(c.chunk.Constants, obj)
	c.constIndex[key] = idx
	return idx
}

// isValueDedupable returns true for kLex types where Inspect() is a
// faithful identity — two values that Inspect() the same can safely
// share a constant pool slot. CompiledFunction / StructDef / EnumDef
// fail this — different definitions can produce identical Inspect()
// strings but must remain distinct constants. Bytes also fails: its
// Inspect() is the length-only "bytes(N)", so deduping would alias
// every same-length byte literal onto one backing slice.
func isValueDedupable(obj eval.Object) bool {
	switch obj.(type) {
	case *eval.Integer, *eval.Float, *eval.String, *eval.Boolean,
		*eval.Null:
		return true
	}
	return false
}

// ── Error helpers ─────────────────────────────────────────────────────────────

// CompileError carries the AST position alongside the failure
// message so callers can surface a clean "compile failed at line N"
// error to the user.
type CompileError struct {
	Pos     ast.Pos
	Message string
}

func (e *CompileError) Error() string {
	if e.Pos.Line == 0 {
		return "compile error: " + e.Message
	}
	return fmt.Sprintf("compile error at line %d: %s", e.Pos.Line, e.Message)
}

// unimplementedAt produces a position-anchored error for AST shapes
// the skeleton doesn't handle yet. The diff-runner pattern-matches
// the *CompileError type to mark a test "skipped" rather than
// "failed."
//
// We take pos explicitly (rather than reflecting on the node) because
// ast.Node is an interface that doesn't expose the embedded Pos
// uniformly — every call site already has the concrete type in hand,
// so passing v.Pos is one tap-away and keeps reflect out of the build.
func (c *Compiler) unimplementedAt(pos ast.Pos, what string) error {
	return &CompileError{
		Pos:     pos,
		Message: "VM compiler does not yet handle " + what,
	}
}
