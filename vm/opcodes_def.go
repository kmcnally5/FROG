// Code in this file is HAND-WRITTEN. It is the single source of truth
// for every VM opcode.
//
// After editing, run:
//
//	go generate ./vm/...
//
// which regenerates vm/opcodes_gen.go (the const block + name table +
// stack-effect table). DO NOT edit opcodes_gen.go by hand — your
// changes will be erased on the next regen.

//go:generate go run ./cmd/vmgen

package vm

// operandKind tags how an opcode's immediate operand is encoded in
// the instruction stream. The compiler emits each kind with a known
// width; the disassembler and stack-balance verifier read the same
// table to walk the stream consistently.
type operandKind int

const (
	opKindNone       operandKind = iota
	opKindInt8                   // signed 8-bit literal
	opKindUint8                  // unsigned 8-bit (small index, argc)
	opKindUint16                 // unsigned 16-bit (constant pool / builtin index)
	opKindInt32                  // signed 32-bit (jump offsets, large literals)
	opKindConstIdx               // index into the function's constant pool
	opKindBuiltinIdx             // index into the global builtin table
	opKindLocalIdx               // index into the current frame's local slot array
)

// operandDef names an immediate operand and gives its on-wire encoding.
type operandDef struct {
	Name string
	Kind operandKind
}

// opcodeDef is one VM opcode. Everything generated about the opcode
// — its numeric value, name, encoding, stack effect, disassembly
// format — flows from this struct.
//
// StackIn / StackOut are the number of values popped / pushed. A
// negative number means "variable, computed from operands" (used by
// CallBuiltin where argc is an operand).
type opcodeDef struct {
	Name        string
	Operands    []operandDef
	StackIn     int
	StackOut    int
	Description string
}

// opcodeDefs is the full ordered list of VM opcodes. Order determines
// numeric value (Halt = 0, PushNull = 1, …) so reordering or removing
// entries is a wire-format change — bump a version constant if you do
// that mid-flight. Appending new opcodes at the end is always safe.
//
// Stay minimal for now: this is the set the compiler+VM bring-up will
// implement first. Later passes will add closure ops, channel ops,
// async/await ops, struct field access, etc.
var opcodeDefs = []opcodeDef{
	{Name: "Halt", StackIn: 0, StackOut: 0,
		Description: "Stop execution and return the top of stack (or NULL if empty)."},
	{Name: "Pop", StackIn: 1, StackOut: 0,
		Description: "Discard the top of stack. Emitted after expression statements whose value isn't used."},

	// ── Constants & singletons ─────────────────────────────────────
	{Name: "PushNull", StackIn: 0, StackOut: 1,
		Description: "Push the NULL singleton."},
	{Name: "PushTrue", StackIn: 0, StackOut: 1,
		Description: "Push the TRUE singleton."},
	{Name: "PushFalse", StackIn: 0, StackOut: 1,
		Description: "Push the FALSE singleton."},
	{Name: "PushConst",
		Operands:    []operandDef{{Name: "idx", Kind: opKindConstIdx}},
		StackIn:     0, StackOut: 1,
		Description: "Push the function's constant pool[idx]."},
	{Name: "PushInt",
		Operands:    []operandDef{{Name: "value", Kind: opKindInt32}},
		StackIn:     0, StackOut: 1,
		Description: "Push an inline int32 literal, boxed via intObj (small-int pool aware)."},

	// ── Locals ──────────────────────────────────────────────────────
	{Name: "LoadLocal",
		Operands:    []operandDef{{Name: "slot", Kind: opKindLocalIdx}},
		StackIn:     0, StackOut: 1,
		Description: "Push the value of the local at the given slot."},
	{Name: "StoreLocal",
		Operands:    []operandDef{{Name: "slot", Kind: opKindLocalIdx}},
		StackIn:     1, StackOut: 0,
		Description: "Pop the top of stack and store it into the local at the given slot."},

	// ── Arithmetic ──────────────────────────────────────────────────
	{Name: "Add", StackIn: 2, StackOut: 1,
		Description: "+. Int+Int, Float+Float, String+String, Bytes+Bytes; mixed Int/Float promotes."},
	{Name: "Sub", StackIn: 2, StackOut: 1, Description: "-."},
	{Name: "Mul", StackIn: 2, StackOut: 1, Description: "*."},
	{Name: "Div", StackIn: 2, StackOut: 1,
		Description: "/. Divide by zero raises a kLex runtime error."},
	{Name: "Mod", StackIn: 2, StackOut: 1,
		Description: "%. Integer-only; modulo by zero raises a kLex runtime error."},
	{Name: "Neg", StackIn: 1, StackOut: 1,
		Description: "Unary -."},

	// ── Comparison (return Boolean) ─────────────────────────────────
	{Name: "Eq", StackIn: 2, StackOut: 1, Description: "==. Cross-type comparison follows the kLex strict rules."},
	{Name: "Ne", StackIn: 2, StackOut: 1, Description: "!=."},
	{Name: "Lt", StackIn: 2, StackOut: 1, Description: "<. Numbers and strings (lexicographic)."},
	{Name: "Le", StackIn: 2, StackOut: 1, Description: "<=."},
	{Name: "Gt", StackIn: 2, StackOut: 1, Description: ">."},
	{Name: "Ge", StackIn: 2, StackOut: 1, Description: ">=."},

	// ── Logical ─────────────────────────────────────────────────────
	{Name: "Not", StackIn: 1, StackOut: 1,
		Description: "!. Bool-only — integers are not truthy in kLex."},

	// ── Collections ─────────────────────────────────────────────────
	{Name: "MakeArray",
		Operands: []operandDef{{Name: "count", Kind: opKindUint16}},
		StackIn:  -1, StackOut: 1,
		Description: "Pop `count` values (left-to-right pushed → bottom-to-top), push Array literal."},
	{Name: "MakeHash",
		Operands: []operandDef{{Name: "pairs", Kind: opKindUint16}},
		StackIn:  -1, StackOut: 1,
		Description: "Pop `pairs*2` values (key,value pushed in source order), push Hash literal."},
	{Name: "MakeTuple",
		Operands: []operandDef{{Name: "count", Kind: opKindUint16}},
		StackIn:  -1, StackOut: 1,
		Description: "Pop `count` values, push Tuple literal. Used by `return a, b, c` and similar."},
	{Name: "Index",
		StackIn: 2, StackOut: 1,
		Description: "Pop [container, index], push element. Container may be Array, String, Bytes, Hash, Tuple."},
	{Name: "IndexStore",
		StackIn: 3, StackOut: 0,
		Description: "Pop [container, index, value]; mutate container[index] = value. Statement-level — leaves nothing on stack. Routes through eval.EvalIndexAssign for shared semantics."},
	{Name: "UnpackTuple",
		Operands: []operandDef{{Name: "expected", Kind: opKindUint16}},
		StackIn:  1, StackOut: -1, // pops Tuple, pushes `expected` elements
		Description: "Pop a Tuple, push its elements in order (so the last element ends up on top). Errors if the value isn't a Tuple OR its arity doesn't match `expected`. Used by MultiAssignStmt to enforce kLex's strict tuple-unpack rule."},
	{Name: "Unwrap",
		StackIn: 1, StackOut: 1,
		Description: "Pop a 2-tuple (value, err). If err != null, early-return from the current chunk with err. Otherwise push value. Implements the postfix `?` operator — the standard kLex error-propagation idiom."},
	{Name: "ReturnIfError",
		StackIn: 0, StackOut: 0, // peek + maybe-pop — net 0 / -1
		Description: "Peek top of stack. If empty: no-op. If *Error: early-return from the current chunk with that error (mirrors how the tree-walker bubbles Errors through statement sequences). Otherwise: pop the value and continue. Emitted between statements inside function bodies, the top-level program, and inside if/while/for/switch bodies so that an error from any statement propagates the way the tree-walker propagates it via isError()."},

	// ── Closure / upvalues ─────────────────────────────────────────
	{Name: "GetUpvalue",
		Operands: []operandDef{{Name: "idx", Kind: opKindUint16}},
		StackIn:  0, StackOut: 1,
		Description: "Push the value of the current closure's upvalue[idx]. Read through the captured cell; sees any mutation made through the same cell elsewhere."},
	{Name: "SetUpvalue",
		Operands: []operandDef{{Name: "idx", Kind: opKindUint16}},
		StackIn:  1, StackOut: 0,
		Description: "Pop tos and store it into the current closure's upvalue[idx] cell. Mutation is visible to the enclosing frame + every other closure capturing the same cell."},
	{Name: "MakeClosure",
		Operands: []operandDef{{Name: "templateIdx", Kind: opKindConstIdx}},
		StackIn:  0, StackOut: 1,
		Description: "Build a closure value: copy the CompiledFunction template at constants[templateIdx], populate its Upvalues from the current frame's locals/upvalues per the template's UpvalueRefs, push the closure. Replaces PushConst for FunctionLiterals that capture anything."},

	// ── Structs ────────────────────────────────────────────────────
	{Name: "MakeStruct",
		Operands: []operandDef{{Name: "fieldCount", Kind: opKindUint16}},
		StackIn:  -1, StackOut: 1, // pops def + 2N (name,value) pairs
		Description: "Build a StructInstance. Pops in stack order: top → valueN, nameN, ..., value1, name1, def (StructDef). Validates each name is in def.Fields and emits a TypeError otherwise."},
	{Name: "GetField",
		Operands: []operandDef{{Name: "name", Kind: opKindConstIdx}},
		StackIn:  1, StackOut: 1,
		Description: "Pop a StructInstance, push instance.Fields[name]. Errors if name isn't a declared field. name is a string-constant index."},
	{Name: "SetField",
		Operands: []operandDef{{Name: "name", Kind: opKindConstIdx}},
		StackIn:  2, StackOut: 0,
		Description: "Pop [instance, value], mutate instance.Fields[name] = value. Errors on unknown field or frozen instance. Statement-level — leaves nothing on stack."},

	// ── Imports ────────────────────────────────────────────────────
	{Name: "Import",
		Operands: []operandDef{
			{Name: "pathConst", Kind: opKindConstIdx},
			{Name: "aliasConst", Kind: opKindConstIdx},
		},
		StackIn:  0, StackOut: 1,
		Description: "Resolve `pathConst` (a String) via the tree-walker's import machinery and push the resulting *eval.Module. Caller follows with StoreLocal under the alias. aliasConst is currently informational (used for the Module.Name field on construction)."},

	// ── Enum pattern matching ──────────────────────────────────────
	{Name: "MatchVariant",
		Operands: []operandDef{{Name: "bindCount", Kind: opKindUint16}},
		StackIn:  2, StackOut: -1, // pops [pattern, subject]; pushes either (bindCount fields + true) or (false)
		Description: "Pop [pattern, subject]. If subject is an EnumInstance whose variant matches pattern, push bindCount field values in declaration order then push True. Otherwise push False (no field values). pattern is *String (short form: match by variant name), *EnumVariant (full form: match type + variant name), or *EnumInstance (zero-field full-form pattern)."},

	// ── Builtin call ────────────────────────────────────────────────
	{Name: "CallBuiltin",
		Operands: []operandDef{
			{Name: "builtin", Kind: opKindBuiltinIdx},
			{Name: "argc", Kind: opKindUint8},
		},
		StackIn:     -1, // argc args popped; resolved at execution time
		StackOut:    1,
		Description: "Pop argc args, invoke builtinTable[builtin], push result. Errors propagate."},
	{Name: "Call",
		Operands: []operandDef{{Name: "argc", Kind: opKindUint8}},
		StackIn:  -1, // argc args + 1 callable popped
		StackOut: 1,
		Description: "User-defined function call. Pop argc args + 1 callable (CompiledFunction). Execute the callable's chunk with args bound to slots 0..argc-1. Push return value."},

	// ── Control flow ────────────────────────────────────────────────
	{Name: "Jump",
		Operands:    []operandDef{{Name: "offset", Kind: opKindInt32}},
		StackIn:     0, StackOut: 0,
		Description: "Unconditional relative jump (offset is signed bytes from end of this instruction)."},
	{Name: "JumpIfFalse",
		Operands:    []operandDef{{Name: "offset", Kind: opKindInt32}},
		StackIn:     1, StackOut: 0,
		Description: "Pop top of stack (must be Bool); jump if false."},
	{Name: "JumpIfFalsePeek",
		Operands:    []operandDef{{Name: "offset", Kind: opKindInt32}},
		StackIn:     0, StackOut: 0,
		Description: "PEEK top of stack (must be Bool); jump if false, leave tos intact. For && short-circuit: the false value remains as the expression result."},
	{Name: "JumpIfTruePeek",
		Operands:    []operandDef{{Name: "offset", Kind: opKindInt32}},
		StackIn:     0, StackOut: 0,
		Description: "PEEK top of stack (must be Bool); jump if true, leave tos intact. For || short-circuit: the true value remains as the expression result."},
	{Name: "Return", StackIn: 1, StackOut: 0,
		Description: "Pop tos and return it from the current function."},

	// ── Higher-order intrinsics ────────────────────────────────────
	// `map(arr, fn)`, `filter(arr, fn)`, `reduce(arr, fn, init)` are
	// the inner-loop primitives that dominate kLex's "vector
	// transformation" idiom. The naive lowering via OpCallBuiltin
	// pays a per-element cross-interpreter dispatch tax:
	//
	//   builtin map → eval.callCallable → ExternalCallable hook →
	//   vm.execute → return → repeat
	//
	// for every element. On a 1M-element chain that's millions of
	// type assertions, interface boxings, and hook indirections.
	//
	// These intrinsic opcodes collapse the loop into a single
	// dispatch (one switch arm). The callback's *CompiledFunction
	// is type-asserted ONCE outside the loop, the inner loop calls
	// execute() directly with reused pooled locals, and the result
	// container is allocated once up front. Falls back to the
	// generic builtin path if the callback isn't a *CompiledFunction
	// (a *Function from an imported module or a *Builtin used as
	// callback), so semantics stay identical to the eval-side
	// builtins.
	{Name: "Map", StackIn: 2, StackOut: 1,
		Description: "Pop fn + arr; push map(arr, fn) — new Array of same length where each element is fn(arr[i])."},
	{Name: "Filter", StackIn: 2, StackOut: 1,
		Description: "Pop fn + arr; push filter(arr, fn) — new Array containing only elements where fn(el) returned true."},
	{Name: "Reduce", StackIn: 3, StackOut: 1,
		Description: "Pop init + fn + arr; push reduce(arr, fn, init) — left fold of arr through fn starting from init."},

	// ── Fresh cell allocation ──────────────────────────────────────
	// Replace locals[slot] with a brand-new empty UpvalueCell.
	// Emitted by `let` (compileLet) so each *execution* of a let
	// binds a fresh shared cell — critical for `let` inside a loop
	// body, where closures captured on different iterations must
	// see independent values for the same name. Without this op,
	// every iteration's closure would alias the SAME cell, and all
	// closures would observe the last iteration's value (the
	// classic loop-var-capture bug).
	//
	// Subsequent OpStoreLocal stores into the fresh cell; subsequent
	// OpMakeClosure captures the fresh cell pointer. Closures from
	// PRIOR iterations keep their reference to the OLD cell — its
	// Value field still holds whatever was stored before, and Go's
	// GC keeps it alive for as long as any closure references it.
	{Name: "FreshCell",
		Operands:    []operandDef{{Name: "slot", Kind: opKindLocalIdx}},
		StackIn:     0, StackOut: 0,
		Description: "Replace locals[slot] with a fresh empty UpvalueCell. Used by `let` to make each binding a per-execution cell so closures from different iterations of an enclosing loop don't alias each other."},

	// ── Const marking ──────────────────────────────────────────────
	// Emitted by ConstStmt right after the initial StoreLocal so the
	// runtime can flip the cell's IsConst flag. Subsequent stores via
	// OpStoreLocal / OpSetUpvalue check IsConst and produce a kLex
	// runtime error ("cannot reassign constant <name>") that safe()
	// catches — matching tree-walker semantics. The name is provided
	// for the error message; the slot index is needed because
	// OpMarkConst targets the cell that was just written (not the
	// top of stack, which has already been consumed by StoreLocal).
	{Name: "MarkConst",
		Operands: []operandDef{
			{Name: "slot", Kind: opKindLocalIdx},
			{Name: "nameIdx", Kind: opKindConstIdx},
		},
		StackIn:     0, StackOut: 0,
		Description: "Mark the local at the given slot as const (read-only). The name (constants[nameIdx], a *String) is recorded on the cell for use in the runtime error if anything tries to reassign it later."},

	// ── Method dispatch ────────────────────────────────────────────
	// `receiver.name(args)` where the parser parsed the callee as a
	// DotExpr. The runtime decides between method dispatch and the
	// fallback "fetch property and call it as a function" based on
	// the receiver's actual type:
	//
	//   * receiver is *StructInstance AND def.MethodsAny[name] is a
	//     CompiledFunction → call the method, injecting receiver as
	//     slot 0 and the supplied args as slots 1..argc.
	//   * receiver is *StructInstance AND def.Methods[name] is a
	//     tree-walker *Function → dispatch through the
	//     ExternalCallable hook (eval.callCallable) so we get the
	//     same behaviour as the tree-walker.
	//   * otherwise → field access (`OpGetField` semantics) followed
	//     by an ordinary OpCall. Covers module-function calls
	//     (`math.sqrt(2)`) and hash-stored callables.
	{Name: "CallMethod",
		Operands: []operandDef{
			{Name: "nameIdx", Kind: opKindConstIdx},
			{Name: "argc", Kind: opKindUint8},
		},
		StackIn:  -1, // pops argc args + 1 receiver
		StackOut: 1,
		Description: "Dispatch `receiver.name(args)`. Looks up name in receiver.Def.MethodsAny (VM-compiled methods) first, then receiver.Def.Methods (tree-walker methods via ExternalCallable), then falls back to property-fetch + call (module/hash/closure-in-field). Push the return value."},

	// ── Deferred name resolution ───────────────────────────────────
	// kLex's tree-walker resolves identifiers lazily (only when the
	// statement that mentions them actually executes). The VM
	// compiler walks the whole program ahead of time, so unknown
	// names that only sit in dead-or-rare branches would fail at
	// compile time and refuse to load the program at all. Emitting
	// this opcode instead pushes the resolution to runtime: when (and
	// only when) the instruction is executed, the VM produces a
	// kLex-shape "undefined name" runtime error that the surrounding
	// OpReturnIfError chain bubbles like any other error.
	{Name: "UndefinedName",
		Operands:    []operandDef{{Name: "nameIdx", Kind: opKindConstIdx}},
		StackIn:     0, StackOut: 1,
		Description: "Raise a runtime 'undefined name N' error. The name is read from the constant pool slot N (a *String). Used when the compiler couldn't resolve an identifier at compile time — matches the tree-walker's deferred-resolution semantics."},

	// ── Struct-method installation (M5+M6 fix, 2026-05-22) ────────
	// Pops a *CompiledFunction (typically produced by OpMakeClosure
	// at the call site, with its Upvalues captured from the current
	// frame's locals) and installs it as the method `name` on the
	// *StructDef on the stack below. Leaves the def on the stack so
	// subsequent OpInstallMethod calls can chain.
	//
	// Why this exists: previously compileStructDecl baked method
	// CompiledFunctions into the StructDef as constant-pool
	// templates with UpvalueRefs but no runtime Upvalues. A method
	// that references module-level names (e.g. `newObservable`
	// inside stdlib/observable.lex's `map` method) compiled
	// correctly but failed at call time with "OpGetUpvalue idx 0
	// out of range" because cf.Upvalues was nil. OpInstallMethod
	// fixes this by going through the same OpMakeClosure path
	// regular fns use — the closure on the stack has populated
	// Upvalues when it lands here.
	{Name: "InstallMethod",
		Operands:    []operandDef{{Name: "nameIdx", Kind: opKindConstIdx}},
		StackIn:     1, // pops the closure
		StackOut:    0, // def stays on the stack (net +0)
		Description: "Pop the *CompiledFunction on top and install it as method `name` (constants[nameIdx], a *String) on the *StructDef on the stack below. Leaves the def on the stack. Used by compileStructDecl to give methods proper closure semantics — they capture module-level state the same way top-level functions do."},

	// ── Runtime-injected identifier: __args__ ──────────────────────
	// Mirrors the _scriptDir intercept pattern, but at load (not
	// call). The tree-walker sets `__args__` on the entry env so
	// `len(__args__)`, `__args__[0]`, etc. resolve via env.Get. The
	// VM has no env chain, so the compiler emits this opcode for any
	// free reference to `__args__` and the dispatch loop builds a
	// fresh *Array around the current chunk's ScriptArgs slice. The
	// slice is propagated to every sub-chunk by PropagateScriptArgs,
	// so nested functions / closures / methods see the same args.
	// Allocating the *Array per-load (rather than caching one on the
	// chunk) matches tree-walker semantics: each read of `__args__`
	// yields a distinct array value, and mutations to one don't
	// surprise other readers.
	{Name: "LoadScriptArgs",
		StackIn: 0, StackOut: 1,
		Description: "Push a fresh *Array wrapping the current chunk's ScriptArgs. Emitted by the compiler for free references to `__args__`. Mirrors the tree-walker's env.Get(\"__args__\") path."},
}
