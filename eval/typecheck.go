package eval

// typecheck.go contains type predicates and error constructors.
//
// Keeping these separate from eval.go serves two purposes:
//  1. The rules for what types are valid for each operation are in one place,
//     making them easy to find and change.
//  2. eval.go stays focused on control flow and doesn't get cluttered with
//     type validation logic.
//
// kLex is STRICTLY TYPED — there is no implicit coercion. You cannot add
// an integer and a string. You cannot use an integer as a boolean condition.
// Every operator checks its operands' types before doing anything with them.
// This is enforced here.

import (
	"fmt"
	"klex/ast"
)

// canArithmetic returns true if the type can be used with +, -, *, /
func canArithmetic(t ObjectType) bool { return t == INTEGER_OBJ || t == FLOAT_OBJ }

// canCompare returns true if the type supports <, >, <=, >=
// Integers, floats, and strings all support ordering comparison.
// String comparison is lexicographic (Unicode code point order).
func canCompare(t ObjectType) bool {
	return t == INTEGER_OBJ || t == FLOAT_OBJ || t == STRING_OBJ
}

// canLogical returns true if the type can be used with &&, ||, !
// Only booleans are valid here — integers are NOT truthy in kLex.
// `if 1 { }` is a TypeError, not `if true { }`.
func canLogical(t ObjectType) bool { return t == BOOLEAN_OBJ }

// typeMismatchError is used when an operator gets operands of the wrong types.
// Example: `1 + true` → "operator + not defined for INTEGER and BOOLEAN"
//
// Agentic hook: every internal error constructed via this helper fires
// the registered on_error_bubble hook (if any). Mirror of the VM's
// bubbleError fire so the hook receives every internal error regardless
// of which interpreter is running. The hook's re-entry guard handles
// the case where the hook itself constructs an error.
func typeMismatchError(op string, left, right ObjectType, pos ast.Pos) *Error {
	e := &Error{
		Kind:    TypeError,
		Pos:     pos,
		Message: fmt.Sprintf("operator %s not defined for %s and %s", op, left, right),
	}
	FireErrorBubbleHook(e, nil)
	return e
}

// typeError is a general-purpose type error for situations that don't fit
// the "operator mismatch" pattern — e.g. "unhashable type", "not a function".
// See typeMismatchError for the agentic-hook rationale.
func typeError(msg string, pos ast.Pos) *Error {
	e := &Error{Kind: TypeError, Pos: pos, Message: msg}
	FireErrorBubbleHook(e, nil)
	return e
}

// runtimeError is for errors that are type-correct but fail at runtime —
// e.g. division by zero, out-of-bounds index, undefined variable.
// See typeMismatchError for the agentic-hook rationale.
func runtimeError(msg string, pos ast.Pos) *Error {
	e := &Error{Kind: RuntimeErr, Pos: pos, Message: msg}
	FireErrorBubbleHook(e, nil)
	return e
}

// undefinedIdentifierMessage returns a "this name doesn't exist" error
// message — but enriched when the name is a reserved word from another
// popular language. A bare "undefined variable: var" gets interpreted
// literally by both programmers coming from Go/Rust/JS AND by LLMs
// reading kLex errors during iteration (see the Gemma 4 coding probe).
// Surfacing the actual issue ("`var` is not a kLex keyword — use bare
// assignment") helps both audiences self-correct quickly.
func undefinedIdentifierMessage(name string) string {
	switch name {
	case "var":
		return "'var' is not a kLex keyword. Declare variables with bare assignment (`x = 0`) or `let` for current-scope-only (`let x = 0`)"
	case "func", "function":
		return "'" + name + "' is not a kLex keyword. Define functions with `fn name(args) { ... }`"
	case "def":
		return "'def' is not a kLex keyword. Define functions with `fn name(args) { ... }` (kLex syntax, not Python)"
	case "class":
		return "'class' is not a kLex keyword. Define data types with `struct Name { field1, field2 }` instead"
	case "interface":
		return "'interface' is not a kLex keyword. kLex has no interface declarations — duck-typing on struct/hash shapes is the convention"
	case "void":
		return "'void' is not a kLex keyword. Functions return null implicitly when no `return` is hit — no return-type annotation needed"
	case "static", "final", "public", "private", "protected":
		return "'" + name + "' is not a kLex keyword. kLex has no visibility modifiers — every top-level name in a module is exported"
	case "nil":
		return "'nil' is not a kLex keyword. Use `null` (kLex follows JavaScript/Java naming, not Go/Ruby)"
	case "None":
		return "'None' is not a kLex keyword. Use `null` (kLex syntax, not Python)"
	case "True":
		return "'True' is not a kLex keyword. Use lowercase `true` (kLex syntax, not Python)"
	case "False":
		return "'False' is not a kLex keyword. Use lowercase `false` (kLex syntax, not Python)"
	case "undefined":
		return "'undefined' is not a kLex keyword. Use `null` (kLex syntax, not JavaScript)"
	case "new":
		return "'new' is not a kLex keyword. Construct structs with `StructName { field: value, ... }` — no `new` required"
	case "throw", "raise":
		return "'" + name + "' is not a kLex keyword. kLex has no exceptions; return `error(code, message)` as the second value of a `(value, err)` tuple"
	case "try", "catch", "except":
		return "'" + name + "' is not a kLex keyword. kLex has no try/catch — use `safe(fn, ...args)` to catch panics, or `?` to propagate (value, err) tuples"
	case "elif":
		return "'elif' is not a kLex keyword. Chain conditionals with `} else if cond {` (kLex syntax, not Python)"
	}
	return "undefined variable: " + name
}

// -------------------- FUNCTION TYPE ANNOTATIONS --------------------
//
// kLex function parameters and return values may carry optional type
// annotations (type-first syntax: `fn add(int a, int b) : int { ... }`).
// Annotations are OPT-IN per function (the parser enforces all-or-nothing on
// the parameter list) and, when present, ENFORCED at call time here.
//
// This extends kLex's strict-typing rule from operators to function contracts:
// a wrong-typed argument fails loudly AT THE CALL BOUNDARY, naming the
// parameter — instead of surfacing as a confusing operator error deep inside
// the body. Unannotated functions pay nothing: the call paths gate every check
// behind fn.TypeChecked.
//
// Design decisions (Karl, 2026-06-02):
//   - Strict null: null satisfies only `null` or `any`. There is no implicit
//     nullability — `T?` optional syntax is deferred future work.
//   - Both params AND return values are enforced.
//   - A non-keyword annotation is treated as a user struct/enum type name and
//     matched against the value's concrete type name; anything else fails
//     (this catches typos and genuine type mismatches alike).

// HasTypeAnnotations reports whether a function carries any enforceable
// annotation — any non-empty param type, or a return type. Computed once at
// function construction (tree-walker *Function and VM *CompiledFunction both)
// and cached in a TypeChecked flag so the common unannotated case skips the
// per-call check entirely.
func HasTypeAnnotations(paramTypes []string, returnType string) bool {
	if returnType != "" {
		return true
	}
	for _, t := range paramTypes {
		if t != "" {
			return true
		}
	}
	return false
}

// AnnotationAccepts reports whether a runtime value satisfies a parameter or
// return-type annotation. typeName is the source-level annotation — either a
// built-in type keyword (with common aliases) or a user-defined struct/enum
// name. Callers skip this for unannotated ("") slots before reaching here.
//
// Exported so the bytecode VM (package vm) shares this exact vocabulary —
// there is ONE definition of what each type keyword accepts, used by both
// interpreters.
func AnnotationAccepts(typeName string, obj Object) bool {
	switch typeName {
	case "any":
		return true
	case "int", "integer":
		return obj.Type() == INTEGER_OBJ
	case "float":
		return obj.Type() == FLOAT_OBJ
	case "number", "num":
		return obj.Type() == INTEGER_OBJ || obj.Type() == FLOAT_OBJ
	case "string", "str":
		return obj.Type() == STRING_OBJ
	case "bool", "boolean":
		return obj.Type() == BOOLEAN_OBJ
	case "array":
		return obj.Type() == ARRAY_OBJ
	case "hash", "map":
		return obj.Type() == HASH_OBJ || obj.Type() == CONCURRENT_HASH_OBJ
	case "function", "fn":
		t := obj.Type()
		return t == FUNCTION_OBJ || t == BUILTIN_OBJ || t == COMPILED_FUNCTION_OBJ
	case "null":
		return obj.Type() == NULL_OBJ
	case "bytes":
		return obj.Type() == BYTES_OBJ
	case "tuple":
		return obj.Type() == TUPLE_OBJ
	case "channel":
		return obj.Type() == CHANNEL_OBJ
	case "task":
		return obj.Type() == TASK_OBJ
	case "error":
		return obj.Type() == ERROR_OBJ
	case "image":
		return obj.Type() == IMAGE_OBJ
	case "font":
		return obj.Type() == FONT_OBJ
	}
	// Not a built-in keyword → treat as a user struct/enum type name and match
	// the value's concrete type. A non-instance value (or a name mismatch)
	// fails: it's either a real type error or a typo'd annotation.
	switch v := obj.(type) {
	case *StructInstance:
		return v.Def.Name == typeName
	case *EnumInstance:
		return v.TypeName == typeName
	}
	return false
}

// CheckArgAnnotations validates supplied positional arguments against a
// parallel (params, paramTypes) annotation list. Slice-based rather than
// tied to *Function so the VM can pass *CompiledFunction's fields through the
// same single implementation. Callers gate on a TypeChecked flag and run the
// arity check first. Unannotated ("") params are skipped. For a variadic
// function the trailing rest parameter's annotation applies to EACH collected
// element (mirroring Go's `nums ...int`). Returns nil on success, else a
// TypeError located at pos.
//
// paramTypes is assumed parallel to params (the parser guarantees this); a
// defensive length guard avoids any out-of-range panic on the hot path.
func CheckArgAnnotations(params, paramTypes []string, variadic bool, args []Object, fnName string, pos ast.Pos) *Error {
	n := len(params)
	if n == 0 || len(paramTypes) < n {
		return nil
	}
	for i, arg := range args {
		var typeName, paramName string
		switch {
		case variadic && i >= n-1:
			typeName, paramName = paramTypes[n-1], params[n-1]
		case i < n:
			typeName, paramName = paramTypes[i], params[i]
		default:
			// More args than params on a non-variadic fn — the arity
			// check already rejected this; nothing left to validate.
			return nil
		}
		if typeName == "" {
			continue
		}
		if !AnnotationAccepts(typeName, arg) {
			return typeError(fmt.Sprintf("%s: parameter %q expects %s, got %s",
				fnName, paramName, typeName, arg.Type()), pos)
		}
	}
	return nil
}

// CheckReturnAnnotation validates a function's result against its return-type
// annotation. Slice-friendly companion to CheckArgAnnotations, shared with the
// VM. An empty returnType means no annotation (skip). A function declared with
// a return type that falls off its body (implicit null) or returns the wrong
// type fails here. Returns nil on success, else a TypeError at pos.
func CheckReturnAnnotation(returnType string, result Object, fnName string, pos ast.Pos) *Error {
	if returnType == "" {
		return nil
	}
	if !AnnotationAccepts(returnType, result) {
		return typeError(fmt.Sprintf("%s: declared return type %s, but returned %s",
			fnName, returnType, result.Type()), pos)
	}
	return nil
}

// CheckDefaultAnnotation validates a defaulted parameter value against its
// annotation — used when a caller omits an argument and the default fills the
// slot. Without this, `fn f(string x = 42)` would silently bind an int to a
// string-annotated parameter. Checked at bind time (not definition time) so
// the tree-walker and VM agree on WHEN the error surfaces. Empty typeName or a
// satisfying value → nil; otherwise a TypeError at pos.
func CheckDefaultAnnotation(typeName, paramName string, defVal Object, fnName string, pos ast.Pos) *Error {
	if typeName == "" || AnnotationAccepts(typeName, defVal) {
		return nil
	}
	return typeError(fmt.Sprintf("%s: parameter %q default value expects %s, got %s",
		fnName, paramName, typeName, defVal.Type()), pos)
}

// checkArgTypes / checkReturnType are the tree-walker's *Function-shaped
// wrappers over the exported slice-based checkers above. Kept so eval call
// sites read cleanly; both delegate to the single shared implementation.
func checkArgTypes(fn *Function, args []Object, fnName string, pos ast.Pos) *Error {
	return CheckArgAnnotations(fn.Params, fn.ParamTypes, fn.Variadic, args, fnName, pos)
}

func checkReturnType(fn *Function, result Object, fnName string, pos ast.Pos) *Error {
	return CheckReturnAnnotation(fn.ReturnType, result, fnName, pos)
}

// fnDisplayName is the name used in type-error messages — the function's own
// name, or "anonymous" for lambdas.
func fnDisplayName(fn *Function) string {
	if fn.Name == "" {
		return "anonymous"
	}
	return fn.Name
}
