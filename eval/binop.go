package eval

// binop.go — shared implementation of every binary operator's
// runtime semantics. Exported so the bytecode VM can dispatch
// arithmetic and comparison opcodes through the SAME code path the
// tree-walking evaluator uses for InfixExpr.
//
// Why centralise here
//
// Before this file existed, the InfixExpr branch in Eval() carried
// the full type/value dispatch inline. The VM would need to re-
// implement it, and the two implementations would drift — a small
// rule change (e.g. "bytes + bytes is now defined") landed in one
// place but not the other, vmdiff would catch it days later, and
// the bug would always be subtle.
//
// EvalBinaryOp is the single source of truth. Eval()'s InfixExpr
// branch is a thin shell that handles operand evaluation and the
// short-circuit operators (&& / ||), then delegates here. The VM's
// OpAdd / OpSub / OpEq / etc. handlers each call this with the
// right operator string and pre-popped operands.
//
// Returns an Object: a value on success or an *Error on failure
// (TypeError / RuntimeError). Callers test via isError() — the
// existing convention.

import (
	"klex/ast"
	"path/filepath"
	"sync/atomic"
)

// EvalBinaryOp applies `op` to two already-evaluated operands and
// returns the result, or an error Object on type mismatch / division
// by zero / unknown operator.
//
// Supported operators:
//
//	Arithmetic:  + - * / %   (Int+Int promote-aware; String+String, Bytes+Bytes for +)
//	Comparison:  == != < > <= >=
//
// && / || are NOT here — they short-circuit and have to evaluate
// their right operand conditionally, which only makes sense at the
// AST level (Eval) or with control-flow opcodes (VM). The vm/ side
// will compile && and || into Jump / JumpIfFalse sequences, not a
// binary opcode.
//
// pos is the source position used for error reporting; pass the
// InfixExpr's Pos from Eval and the chunk's per-byte line from the
// VM. If pos is the zero value the error simply omits the line
// number, which is the existing convention across kLex's builtins.
func EvalBinaryOp(left, right Object, op string, pos ast.Pos) Object {
	switch op {

	case "+":
		// String / Bytes concatenation FIRST — both are typed
		// "additions" that pre-date arithmetic in the dispatch table.
		if left.Type() == STRING_OBJ && right.Type() == STRING_OBJ {
			return &String{Value: left.(*String).Value + right.(*String).Value}
		}
		if left.Type() == BYTES_OBJ && right.Type() == BYTES_OBJ {
			lb := left.(*Bytes).Value
			rb := right.(*Bytes).Value
			out := make([]byte, len(lb)+len(rb))
			copy(out, lb)
			copy(out[len(lb):], rb)
			return &Bytes{Value: out}
		}
		if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
			return typeMismatchError("+", left.Type(), right.Type(), pos)
		}
		if left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ {
			return intObj(left.(*Integer).Value + right.(*Integer).Value)
		}
		return &Float{Value: toFloat64(left) + toFloat64(right)}

	case "-", "*", "/", "%":
		// % is integer-only; everything else promotes to float when
		// either side is float.
		if op == "%" {
			if left.Type() != INTEGER_OBJ || right.Type() != INTEGER_OBJ {
				return typeMismatchError("%", left.Type(), right.Type(), pos)
			}
			if right.(*Integer).Value == 0 {
				return runtimeError("modulo by zero — guard the right operand with `if y != 0` before using `%`", pos)
			}
			return intObj(left.(*Integer).Value % right.(*Integer).Value)
		}
		if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
			return typeMismatchError(op, left.Type(), right.Type(), pos)
		}
		// Fast path: both ints — skip the toFloat64 round-trip.
		if li, lok := left.(*Integer); lok {
			if ri, rok := right.(*Integer); rok {
				switch op {
				case "-":
					return intObj(li.Value - ri.Value)
				case "*":
					return intObj(li.Value * ri.Value)
				case "/":
					if ri.Value == 0 {
						return runtimeError("division by zero — guard the right operand with `if y != 0` before using `/`", pos)
					}
					return intObj(li.Value / ri.Value)
				}
			}
		}
		// Float path: at least one operand is Float — promote both.
		lf, rf := toFloat64(left), toFloat64(right)
		switch op {
		case "-":
			return &Float{Value: lf - rf}
		case "*":
			return &Float{Value: lf * rf}
		case "/":
			if rf == 0 {
				return runtimeError("division by zero — guard the right operand with `if y != 0` before using `/`", pos)
			}
			return &Float{Value: lf / rf}
		}

	case "==":
		return evalEquals(left, right, pos)

	case "!=":
		eq := evalEquals(left, right, pos)
		if isError(eq) {
			return eq
		}
		if eq == TRUE {
			return FALSE
		}
		return TRUE

	case "<", ">", "<=", ">=":
		return evalNumericCompare(left, right, op, pos)
	}
	return runtimeError("EvalBinaryOp: unknown operator '"+op+"'", pos)
}

// EvalIndex resolves `container[index]` for every container shape
// the language supports. Centralised so the VM's OpIndex handler
// hits the SAME type rules and bounds checks as the tree-walker's
// IndexExpr arm (see eval.go ~line 2750). If a future container
// type (e.g. AtomicHash) gets index support, it lands here and
// both interpreters pick it up.
//
// Returns the indexed element or a kLex error Object.
func EvalIndex(container, index Object, pos ast.Pos) Object {
	switch l := container.(type) {
	case *Array:
		idx, ok := index.(*Integer)
		if !ok {
			return typeError(fmtType("array index must be integer, got ", index.Type()), pos)
		}
		if idx.Value < 0 || idx.Value >= len(l.Elements) {
			return runtimeError(fmtBounds("index out of bounds (array length ", idx.Value, len(l.Elements)), pos)
		}
		return l.Elements[idx.Value]
	case *Hash:
		hk, err := toHashKey(index, pos)
		if err != nil {
			return err
		}
		pair, ok := l.Get(hk)
		if !ok {
			return NULL
		}
		return pair.Value
	case *ConcurrentHash:
		hk, err := toHashKey(index, pos)
		if err != nil {
			return err
		}
		val, ok := l.M.Load(hk)
		if !ok {
			return NULL
		}
		pair, ok := val.(HashPair)
		if !ok {
			return NULL
		}
		return pair.Value
	case *String:
		idx, ok := index.(*Integer)
		if !ok {
			return typeError(fmtType("string index must be integer, got ", index.Type()), pos)
		}
		r, inBounds := l.RuneAt(idx.Value)
		if !inBounds {
			return runtimeError(fmtBounds("index out of bounds (string length ", idx.Value, l.RuneLen()), pos)
		}
		return &String{Value: string(r)}
	case *Bytes:
		idx, ok := index.(*Integer)
		if !ok {
			return typeError(fmtType("bytes index must be integer, got ", index.Type()), pos)
		}
		if idx.Value < 0 || idx.Value >= len(l.Value) {
			return runtimeError(fmtBounds("index out of bounds (bytes length ", idx.Value, len(l.Value)), pos)
		}
		return intObj(int(l.Value[idx.Value]))
	case *Tuple:
		idx, ok := index.(*Integer)
		if !ok {
			return typeError(fmtType("tuple index must be integer, got ", index.Type()), pos)
		}
		if idx.Value < 0 || idx.Value >= len(l.Elements) {
			return runtimeError(fmtBounds("index out of bounds (tuple length ", idx.Value, len(l.Elements)), pos)
		}
		return l.Elements[idx.Value]
	case *StructInstance:
		return typeError("cannot use bracket access on struct "+l.Def.Name+" — use dot notation: struct.field", pos)
	}
	return typeError("index operator not supported for "+string(container.Type()), pos)
}

// fmtType / fmtBounds are tiny formatters used by EvalIndex's error
// paths. Kept here rather than reaching for fmt.Sprintf because the
// hot path benefits from avoiding the package overhead. Either way
// these are error-path-only.
func fmtType(prefix string, t ObjectType) string {
	return prefix + string(t)
}

func fmtBounds(prefix string, idx, length int) string {
	// Format matches the tree-walker's inline error to keep
	// vmdiff at byte parity: "index <idx> out of bounds (array length <len>)".
	// Callers pass the prefix without the index ("index out of bounds (array length ");
	// we insert <idx> between "index " and "out of bounds".
	const sentinel = "index out of bounds (array length "
	if prefix == sentinel {
		return "index " + itoaForBounds(idx) + " out of bounds (array length " + itoaForBounds(length) + ")"
	}
	const sentinelBytes = "index out of bounds (bytes length "
	if prefix == sentinelBytes {
		return "index " + itoaForBounds(idx) + " out of bounds (bytes length " + itoaForBounds(length) + ")"
	}
	const sentinelString = "index out of bounds (string length "
	if prefix == sentinelString {
		return "index " + itoaForBounds(idx) + " out of bounds (string length " + itoaForBounds(length) + ")"
	}
	const sentinelTuple = "index out of bounds (tuple length "
	if prefix == sentinelTuple {
		return "index " + itoaForBounds(idx) + " out of bounds (tuple length " + itoaForBounds(length) + ")"
	}
	// Fallback for any caller that uses a different prefix shape:
	// append the index at the end so behaviour is at least defined.
	return prefix + itoaForBounds(length) + ") got index " + itoaForBounds(idx)
}

// itoaForBounds is a tiny int-to-string helper. Stays out of strconv
// to keep error paths import-light.
func itoaForBounds(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// EvalMakeHash builds a *Hash from a flat slice of alternating
// key, value, key, value… entries. Each key is validated through
// toHashKey so the kLex rule "hash keys must be string / int /
// float / bool" is enforced exactly as the tree-walker enforces it
// for HashLiteral. Used by the VM's OpMakeHash handler.
//
// Returns the *Hash on success or a *Error from toHashKey on the
// first invalid key.
func EvalMakeHash(flatKV []Object, pos ast.Pos) Object {
	if len(flatKV)%2 != 0 {
		return runtimeError("EvalMakeHash: odd number of operands — caller bug", pos)
	}
	pairs := make(map[HashKey]HashPair, len(flatKV)/2)
	for i := 0; i < len(flatKV); i += 2 {
		key, val := flatKV[i], flatKV[i+1]
		hk, err := toHashKey(key, pos)
		if err != nil {
			return err
		}
		pairs[hk] = HashPair{Key: key, Value: val}
	}
	return &Hash{Pairs: pairs}
}

// EvalIndexAssign mutates container[index] = value. Mirrors the
// tree-walker's IndexAssignStmt arm exactly: handles *Hash,
// *ConcurrentHash, *Array, with frozen-checks and bounds-checks.
// Strings and Bytes are immutable so they're not in the dispatch.
//
// Returns the assigned value (matches the tree-walker's "assignment
// is an expression that yields its RHS" convention) or an *Error.
func EvalIndexAssign(container, index, value Object, pos ast.Pos) Object {
	switch o := container.(type) {
	case *Hash:
		if o.frozen {
			return runtimeError("cannot mutate frozen hash", pos)
		}
		hk, err := toHashKey(index, pos)
		if err != nil {
			return err
		}
		o.Set(hk, HashPair{Key: index, Value: value})
		return value
	case *ConcurrentHash:
		hk, err := toHashKey(index, pos)
		if err != nil {
			return err
		}
		// Swap reports whether a prior value existed; bump count if not.
		if _, loaded := o.M.Swap(hk, HashPair{Key: index, Value: value}); !loaded {
			atomic.AddInt64(&o.Cnt, 1)
		}
		return value
	case *Array:
		if o.frozen {
			return runtimeError("cannot mutate frozen array", pos)
		}
		idx, ok := index.(*Integer)
		if !ok {
			return typeError(fmtType("array index must be integer, got ", index.Type()), pos)
		}
		if idx.Value < 0 || idx.Value >= len(o.Elements) {
			return runtimeError(fmtBounds("index out of bounds (array length ", idx.Value, len(o.Elements)), pos)
		}
		o.Elements[idx.Value] = value
		return value
	}
	return typeError("index assignment not supported for "+string(container.Type()), pos)
}

// EvalMakeStruct builds a *StructInstance from a flat slice of
// alternating name, value, name, value entries. Each name must
// appear in def.Fields (otherwise → TypeError). Missing fields are
// silently NULL — matches the tree-walker's lenient init behaviour
// where partial struct literals are allowed.
//
// flatNV is consumed as a slice of (name, value) pairs that the VM
// already popped from the stack. The VM passes them in the order
// they were pushed (= source order).
func EvalMakeStruct(def *StructDef, flatNV []Object, pos ast.Pos) Object {
	if len(flatNV)%2 != 0 {
		return runtimeError("EvalMakeStruct: odd number of operands — caller bug", pos)
	}
	known := make(map[string]bool, len(def.Fields))
	for _, f := range def.Fields {
		known[f] = true
	}
	fields := make(map[string]Object, len(def.Fields))
	for i := 0; i < len(flatNV); i += 2 {
		nameObj, val := flatNV[i], flatNV[i+1]
		ns, ok := nameObj.(*String)
		if !ok {
			return typeError("EvalMakeStruct: field name must be string — compiler bug", pos)
		}
		if !known[ns.Value] {
			return typeError("struct "+def.Name+" has no field "+ns.Value, pos)
		}
		fields[ns.Value] = val
	}
	return &StructInstance{Def: def, Fields: fields}
}

// EvalGetField reads a field from a StructInstance, an Enum
// variant descriptor, or a property from a Module. Mirrors the
// tree-walker's DotExpr arm exactly.
func EvalGetField(receiver Object, name string, pos ast.Pos) Object {
	switch r := receiver.(type) {
	case *StructInstance:
		if val, ok := r.GetField(name); ok {
			return val
		}
		return typeError("struct "+r.Def.Name+" has no field "+name, pos)

	case *EnumDef:
		// Variant lookup. Zero-field variants become EnumInstance
		// directly (no call needed). Data-carrying variants return
		// an EnumVariant descriptor that's callable to construct.
		fields, ok := r.Variants[name]
		if !ok {
			return typeError("enum "+r.Name+" has no variant "+name, pos)
		}
		if len(fields) == 0 {
			return &EnumInstance{
				TypeName:    r.Name,
				VariantName: name,
				FieldNames:  nil,
				Fields:      map[string]Object{},
			}
		}
		return &EnumVariant{TypeName: r.Name, VariantName: name, Fields: fields}

	case *EnumInstance:
		if val, ok := r.Fields[name]; ok {
			return val
		}
		return typeError("enum variant "+r.TypeName+"."+r.VariantName+" has no field "+name, pos)

	case *Module:
		if val, ok := r.Env.Get(name); ok {
			return val
		}
		return typeError("module "+r.Name+" has no member "+name, pos)

	case *Error:
		// User errors (from `error(code, msg)` or `safe()`) expose
		// code / message / errorType / traceback / is(). Matches
		// the tree-walker's DotExpr arm exactly.
		if !r.IsUserError {
			return typeError("dot access not supported on internal-error "+name, pos)
		}
		switch name {
		case "code":
			return &String{Value: r.Code}
		case "message":
			return &String{Value: r.Message}
		case "errorType":
			return &String{Value: r.ErrorType}
		case "traceback":
			return &String{Value: r.Traceback}
		case "is":
			// Return a closure that compares a string against this
			// error's code. The capture is the *Error itself; the
			// Builtin closes over it and uses the captured Code
			// at call time.
			captured := r
			return &Builtin{Fn: func(args []Object) Object {
				if len(args) != 1 {
					return runtimeError("is() expects 1 argument", pos)
				}
				s, ok := args[0].(*String)
				if !ok {
					return typeError("is() argument must be a string", pos)
				}
				return boolObj(captured.Code == s.Value)
			}}
		}
		return runtimeError("error has no property "+name, pos)

	case *Null:
		return typeError("cannot access ."+name+" on null — check for null before dot access", pos)
	}
	return typeError("dot access not supported on "+string(receiver.Type()), pos)
}

// EvalSetField mutates a StructInstance's field. Frozen instances
// reject mutation with a clean error. Unknown fields are rejected.
// Returns the assigned value, matching the tree-walker's "assignment
// is an expression yielding its RHS" convention used elsewhere.
func EvalSetField(receiver Object, name string, value Object, pos ast.Pos) Object {
	inst, ok := receiver.(*StructInstance)
	if !ok {
		return typeError("dot assignment not supported on "+string(receiver.Type()), pos)
	}
	if inst.frozen {
		return runtimeError("cannot mutate frozen struct "+inst.Def.Name, pos)
	}
	// Only declared fields are settable — adding unknown fields
	// silently is a footgun; the tree-walker rejects.
	found := false
	for _, f := range inst.Def.Fields {
		if f == name {
			found = true
			break
		}
	}
	if !found {
		return typeError("struct "+inst.Def.Name+" has no field "+name, pos)
	}
	inst.SetField(name, value)
	return value
}

// ImportModule resolves and loads a .lex file the same way the
// tree-walker's `import "path" as alias` does, returning the
// resulting *Module. Wraps the existing ImportStmt arm in Eval so
// the loader / cache / cycle-detection logic is single-source.
//
// On any error (file not found, parse error, etc.) returns
// (nil, *Error). The caller (vm/) converts that into a halt + Go
// error at the OpImport site.
//
// Used by the bytecode VM's OpImport handler. A fresh env is used
// so the imported module doesn't pollute any caller-side scope;
// the only output is the returned *Module.
func ImportModule(path, alias string) (Object, Object) {
	// H4 (audit fix, 2026-05-22): fast path for cache hits — skip
	// the throwaway NewEnv() and synthetic *ast.ImportStmt that the
	// slow path requires. Resolution uses scriptDir="" which still
	// covers KLEX_PATH / binary-dir / binary-parent paths; the
	// "next to importing script" step doesn't fire here because
	// ImportModule callers (the VM) don't pass a scriptDir hint —
	// but those same callers cache-hit on the absolute path
	// produced by the original importer's resolution, so the cache
	// lookup still succeeds for repeated imports of stdlib paths.
	// On cache miss, falls through to the env-based loader.
	if resolved, _, ok := resolveImportPathFromDir(path, ""); ok {
		if absPath, err := filepath.Abs(resolved); err == nil {
			moduleCacheMu.RLock()
			cachedEnv, hit := moduleCache[absPath]
			moduleCacheMu.RUnlock()
			if hit {
				return &Module{Name: alias, Env: cachedEnv}, nil
			}
		}
	}
	env := NewEnv()
	stmt := &ast.ImportStmt{Path: path, Alias: alias}
	result := Eval(stmt, env)
	if isError(result) {
		return nil, result
	}
	return result, nil
}

// CallCallable is the exported entry into the tree-walker's function
// invocation path. Used by the bytecode VM to dispatch through the
// same applyFunction logic when its OpCall handler receives an
// *eval.Function — the case that arises when an imported module's
// (eval-compiled) function is called from VM-compiled code.
//
// Returns (result, errorObject). errorObject is non-nil on any
// kLex error (TypeError, RuntimeError, etc.) — the VM converts it
// to a Go error and halts execution at the call site.
func CallCallable(fn Object, args []Object) (Object, Object) {
	return callCallable(fn, args)
}

// ExternalCallable is a hook for non-eval packages (the bytecode VM)
// to dispatch their own callable Object types when eval-side code
// (`safe`, `map`, `filter`, `reduce`, `async`, etc.) tries to invoke
// them. eval-only code switches on *Function / *Builtin and would
// fall into the "not callable" default arm for a *vm.CompiledFunction
// without this hook.
//
// The hook is set by `vm`'s init() — eval never imports vm (would
// be a cycle), so we can't reference the VM type directly.
//
// Returns (result, dispatched). dispatched==false means the hook
// didn't recognise fn — the caller should fall back to its normal
// "not callable" error path. result may itself be an *Error object
// when the call ran but produced an error (kLex error-as-value).
var ExternalCallable = func(fn Object, args []Object) (result Object, dispatched bool) {
	// M2+L3 (audit fix, 2026-05-22): default no-op stub. Returns
	// dispatched=false so callers fall back to their normal
	// "not callable" handling. The vm package overrides this at
	// init() to dispatch *CompiledFunction. Default-stub pattern
	// removes the per-call `if hook != nil` nil-check from every
	// callCallable / evalCall / parallel / async path.
	return nil, false
}

// ExternalCallableAsync is the async-specialised variant of
// ExternalCallable: same dispatch role, but the implementation is
// expected to clone any *primitive* captured state (closure upvalues,
// in the VM's case) before invoking, so the task gets the
// snapshot-semantics async() promises. Reference types stay shared
// (kLex's documented async-snapshot rule — mutations to Hash/Array/
// Struct contents ARE observed by the caller). If nil, callers fall
// back to ExternalCallable and accept that primitive upvalues
// alias the caller's cell (the loop-var-race that hashConcurrentTest
// section 4 exposes when `i` is in outer scope).
var ExternalCallableAsync = func(fn Object, args []Object) (result Object, dispatched bool) {
	// M2+L3 (audit fix): default no-op stub. Async builtin's hook
	// fallthrough still works — when async() sees this default,
	// the call routes through ExternalCallable instead.
	return nil, false
}

// MarkExternalUpvaluesShared is the M5 (audit follow-up 2026-05-22)
// companion hook to MarkSharedRecursive. eval-side code can't
// reach into a VM *CompiledFunction's captured upvalue cells, but
// when async() is dispatched on a VM closure, those upvalues' Values
// will be read by the spawned goroutine. The vm package installs
// this hook to walk a callable's reachable state and mark Hashes
// shared BEFORE the goroutine starts. Safe no-op for non-VM
// callables; the vm-side implementation type-asserts and bails out
// cleanly when fn isn't a *CompiledFunction.
//
// MUST be called from the SPAWNING goroutine (before `go func()`),
// not from inside the goroutine — otherwise concurrent MarkShared
// walks from sibling spawners race on Hash.Pairs iteration.
var MarkExternalUpvaluesShared = func(fn Object) {
	// M2+L3 (audit fix): default no-op stub. The vm package
	// overrides this at init() to walk *CompiledFunction's
	// upvalues. Default no-op lets eval-side async() spawn paths
	// skip the nil-check on the hot path.
}

// VMCompileAndRunModule is the M6 hook (audit follow-up 2026-05-22)
// for delegating module top-level evaluation to the bytecode VM
// when --vm mode is active. When set, eval's *ast.ImportStmt arm
// tries this hook AFTER parsing succeeds but BEFORE its own tree-
// walker Eval(prog, modEnv) fallback. The hook compiles `prog`,
// runs the resulting chunk, and returns an *Environment whose
// Get path consults LIVE getters pointing at the persistent
// UpvalueCells of every top-level binding — matching tree-walker
// semantics where external readers of `mod.something` see ongoing
// mutations by the module's own internal functions.
//
// Returns (env, nil) on success. Returns (nil, nil) on compile
// error so the loader falls back to tree-walker Eval (keeps M6
// incremental — modules using compiler-unimplemented features
// like interpolated strings with embedded expressions still
// work). Returns (nil, *Error) on a genuine runtime error in the
// module's top-level code; the loader propagates that to the
// importing site.
//
// scriptDir is the absolute directory of the source file (the
// loader has already resolved this); the hook records it on the
// chunk so the VM's _scriptDir intercept resolves correctly
// inside the module's own compiled functions.
//
// nil by default — main.go (or vmdiff's vm-pass setup) installs
// the hook by assigning vm.CompileAndRunModule. Pure-tree-walker
// invocations leave it unset and never pay the VM compile cost.
var VMCompileAndRunModule func(prog *ast.Program, scriptDir string) (env *Environment, errObj Object)

// VMRunScript is the WASM hook for delegating isolated full-program
// execution to the bytecode VM. cmd/wasm/main.go wires this at init()
// time. When set, the runScript builtin (builtins_eval_wasm.go) tries
// the VM first and falls back to tree-walker Eval on compile error.
//
// Returns (result, true) when the VM compiled and ran the program —
// result may itself be an *Error if the program failed at runtime.
// Returns (nil, false) when the compiler rejected the program (e.g.
// unimplemented AST arm) — the caller falls through to Eval.
//
// nil by default — non-WASM and pure-tree-walker builds leave it unset.
var VMRunScript func(prog *ast.Program) (Object, bool)

// IsExternalCallable is a companion predicate to ExternalCallable. It
// lets eval-side type checks recognise types that ExternalCallable
// would dispatch (notably *CompiledFunction from the VM) WITHOUT
// invoking them. Higher-order builtins (map / filter / reduce / apply
// / parallelArrayForEach …) use this when they need to validate
// "this argument is callable" up-front for a clean error message.
//
// Set by vm's init() in the same way as ExternalCallable. nil-safe:
// callers should treat absence as "no external callables registered".
// IsExternalCallable was a function-pointer hook installed by vm
// (M2 audit: replaced 2026-05-22 by a direct type check against
// COMPILED_FUNCTION_OBJ inside IsCallable below). Kept as a
// settable var so any external code that historically assigned to
// it doesn't break the build; eval itself no longer consults the
// hook. New external callable kinds should add their type tags to
// the IsCallable type-switch.
var IsExternalCallable func(fn Object) bool

// IsCallable returns true if fn can be invoked via callCallable —
// covering *Function, *Builtin, and *CompiledFunction (the VM's
// callable). Direct type-tag check; no hook indirection (saves
// ~2-3ns per call on hot paths like map/filter/reduce argument
// validation).
func IsCallable(fn Object) bool {
	if fn == nil {
		return false
	}
	switch fn.(type) {
	case *Function, *Builtin:
		return true
	}
	return fn.Type() == COMPILED_FUNCTION_OBJ
}

// EvalUnaryOp applies a prefix operator to a single operand. kLex
// has exactly two: `!` (Bool-only) and `-` (Int or Float). Exported
// for the same reason as EvalBinaryOp — one source of truth.
func EvalUnaryOp(operand Object, op string, pos ast.Pos) Object {
	switch op {
	case "!":
		if !canLogical(operand.Type()) {
			return typeMismatchError("!", operand.Type(), operand.Type(), pos)
		}
		return boolObj(!operand.(*Boolean).Value)
	case "-":
		if !canArithmetic(operand.Type()) {
			return typeMismatchError("-", operand.Type(), operand.Type(), pos)
		}
		if f, ok := operand.(*Float); ok {
			return &Float{Value: -f.Value}
		}
		return intObj(-operand.(*Integer).Value)
	}
	return runtimeError("EvalUnaryOp: unknown operator '"+op+"'", pos)
}
