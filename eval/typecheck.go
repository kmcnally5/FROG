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
func typeMismatchError(op string, left, right ObjectType, pos ast.Pos) *Error {
	return &Error{
		Kind:    TypeError,
		Pos:     pos,
		Message: fmt.Sprintf("operator %s not defined for %s and %s", op, left, right),
	}
}

// typeError is a general-purpose type error for situations that don't fit
// the "operator mismatch" pattern — e.g. "unhashable type", "not a function".
func typeError(msg string, pos ast.Pos) *Error {
	return &Error{Kind: TypeError, Pos: pos, Message: msg}
}

// runtimeError is for errors that are type-correct but fail at runtime —
// e.g. division by zero, out-of-bounds index, undefined variable.
func runtimeError(msg string, pos ast.Pos) *Error {
	return &Error{Kind: RuntimeErr, Pos: pos, Message: msg}
}
