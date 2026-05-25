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
