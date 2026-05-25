package vm

// function.go — CompiledFunction is the VM's runtime representation
// of a user-defined function.
//
// The tree-walker has its own `eval.Function` type that holds the
// AST body. The VM can't use it because we need bytecode, not AST.
// Rather than retrofit `eval.Function` to hold either form, we
// introduce a sibling type and keep the two evaluators completely
// independent on the callable-value front.
//
// CompiledFunction satisfies eval.Object so it lives in the same
// type space as every other kLex value: it can be stored in
// constants, locals, arrays, hashes, channels — anywhere a Function
// can. The Type() distinguishes it from `eval.Function` so callers
// that need to specialise (e.g. builtins that introspect callables)
// can branch.
//
// Closures: implemented via upvalue cells. UpvalueRefs (below)
// records the COMPILE-TIME descriptor list of captured names;
// Upvalues holds the RUNTIME captured cells populated by
// OpMakeClosure. See [[vm-bytecode-project]] § "Closures: how they
// work" for the full mechanism. The compiler's resolveUpvalue walks
// outward through enclosing scopes and registers an upvalue at every
// intermediate frame so deep nesting captures correctly.

import (
	"klex/eval"
)

// CompiledFunctionType is the ObjectType tag for CompiledFunction
// values. Distinct from FUNCTION_OBJ (the tree-walker's Function)
// so callers can branch on which interpreter the callable came
// from. Both are callable from the VM via OpCall.
//
// M2 (audit fix, 2026-05-22): the canonical constant now lives on
// eval as COMPILED_FUNCTION_OBJ — eval-side dispatchers can do
// `fn.Type() == eval.COMPILED_FUNCTION_OBJ` directly without going
// through IsExternalCallable. This alias is kept for vm-package
// callers' readability.
const CompiledFunctionType = eval.COMPILED_FUNCTION_OBJ

// CompiledFunction is a user-defined function whose body has been
// compiled to bytecode. The chunk is fully self-contained — its
// constant pool, line table, and local-slot layout were resolved
// at compile time.
//
// NumParams is the declared parameter count (including the variadic
// "rest" param if Variadic is true). NumRequired is the number of
// params that must be supplied by the caller — equal to NumParams
// when no defaults exist, smaller when trailing params have defaults.
type CompiledFunction struct {
	Name      string // for stack-trace / disassembly purposes; "" for anonymous
	Chunk     *BytecodeChunk
	NumParams int

	// NumRequired is the minimum argc the caller must supply. For
	// non-variadic functions: NumParams when no defaults, NumParams -
	// trailing-defaults-count otherwise. For variadic functions:
	// NumParams - 1 (the rest-param accepts zero or more, so the
	// minimum required is the count of pre-rest params). Mirrors
	// fn.NumRequired on the tree-walker's *eval.Function.
	NumRequired int

	// DefaultValues is the per-parameter default-value table. Parallel
	// to the parameter list: DefaultValues[i] is the constant default
	// for param i, or nil if no default. The compiler restricts
	// defaults to literal expressions (NullLiteral, IntLiteral,
	// FloatLiteral, StringLiteral, BoolLiteral, BytesLiteral) — that
	// covers every default in the stdlib today. Non-literal defaults
	// (e.g. `n = computeIt()`, `arr = []`) produce a compile error
	// with a migration message. The upgrade path is to swap this
	// field for `DefaultChunks []*BytecodeChunk` evaluated at call
	// time; doing so won't break callers because the OpCall path
	// already routes through bindStackArgs / bindArgs.
	//
	// nil slice when the function has no defaulted parameters at all
	// (the common case) — saves an allocation on the construction
	// path and an empty-slice nil-check at call time.
	DefaultValues []eval.Object

	// Variadic indicates the last declared parameter collects any
	// extra positional arguments as an array. Set at compile time
	// from FunctionLiteral.Variadic. Required-argument count is
	// `NumParams - 1` (the rest-param itself is satisfied by any
	// number of trailing args, including zero). Call sites use
	// validateArity + bindCallArgs to handle both shapes uniformly.
	Variadic bool

	// SelfSlot is the local slot the VM should populate with the
	// CompiledFunction itself at call time, supporting recursive
	// references inside the function body without a full closure
	// pass. -1 means "no self-binding" (anonymous functions).
	SelfSlot int

	// UpvalueRefs is the COMPILE-TIME descriptor list of values
	// this function captures from its enclosing scope. Populated by
	// the sub-compiler when an Ident inside the body resolves to a
	// name in an outer compiler.
	//
	// At runtime, OpMakeClosure walks this list against the
	// CALLER'S locals/upvalues to populate the closure's Upvalues
	// slice. Templates stored as constants share this same
	// descriptor; the per-call Upvalues slice differs.
	UpvalueRefs []upvalueRef

	// Upvalues is the RUNTIME captured cells — populated by
	// OpMakeClosure when a closure value is created. Empty for the
	// constant-pool TEMPLATE that OpMakeClosure copies from. The
	// cells are shared with the enclosing frame, so mutations
	// through the closure propagate back.
	Upvalues []*UpvalueCell
}

func (cf *CompiledFunction) Type() eval.ObjectType { return CompiledFunctionType }

// Inspect produces a kLex-side debug string. Mirrors the
// tree-walker's *eval.Function output so println(fn) is consistent
// across both interpreters: "fn:double" or "fn:<anon>".
func (cf *CompiledFunction) Inspect() string {
	if cf.Name != "" {
		return "fn:" + cf.Name
	}
	return "fn:<anon>"
}
