package eval

// eval_fast_int.go — unboxed integer expression evaluation.
//
// The tree-walking interpreter's hot loop allocates a fresh *Integer
// for every intermediate arithmetic value. For an expression like
// `(x * 2) % 4 == 0`, naive Eval boxes three Integers (one per
// arithmetic op) and one Boolean — even though every intermediate is
// thrown away on the next line. Per-element, in a 1 M-iteration loop,
// that's millions of small allocations and the matching GC work.
//
// evalIntFast computes an integer-shaped expression entirely in a Go
// `int` register, without boxing intermediates. The caller boxes once
// at the boundary (via intObj) or, for comparisons, returns the
// TRUE/FALSE singleton with no allocation at all.
//
// This is a strict subset of the general Eval path. It's deliberately
// narrow:
//
//   - Operands MUST be integer-shaped end to end. The moment we see a
//     Float, Function call, IndexExpr, or anything else, we bail and
//     the caller falls back to the general path.
//   - Identifier resolution checks env.Get and only succeeds if the
//     binding is a *Integer.
//   - Division and modulo by zero produce a kLex runtime error and
//     propagate via the third return value.
//
// Future extension: a sibling evalFloatFast for the float-arithmetic
// case. Skipped today because the benchmark workloads we care about
// are integer-dominated (loop counters, indices, byte ops, accumulators).

import (
	"klex/ast"
)

// evalIntFast attempts to evaluate node as an integer without allocating
// any intermediate *Integer objects.
//
// Returns:
//
//	(val, true,  nil)   — succeeded; val is the integer result.
//	(0,   false, nil)   — node is not integer-shaped; caller falls back
//	                      to the general Eval path.
//	(0,   false, errObj) — a kLex runtime/type error occurred; caller
//	                      MUST return errObj unchanged so propagation works.
func evalIntFast(node ast.Node, env *Environment) (int, bool, Object) {
	switch n := node.(type) {

	case *ast.IntLiteral:
		return n.Value, true, nil

	case *ast.Ident:
		// Look up the binding. We accept *Integer only — anything else
		// (Float, Boolean, String, function, etc.) bounces us back to
		// the general path so type rules still apply correctly.
		val, ok := env.Get(n.Value)
		if !ok {
			return 0, false, nil
		}
		if i, ok := val.(*Integer); ok {
			return i.Value, true, nil
		}
		return 0, false, nil

	case *ast.PrefixExpr:
		if n.Operator != "-" {
			return 0, false, nil
		}
		v, ok, err := evalIntFast(n.Right, env)
		if err != nil {
			return 0, false, err
		}
		if !ok {
			return 0, false, nil
		}
		return -v, true, nil

	case *ast.InfixExpr:
		// Only integer arithmetic ops are fast-pathable. && / || are
		// boolean-only and already short-circuited; comparison operators
		// are handled separately by the caller (which boxes the result
		// as a Boolean, not an Integer).
		switch n.Operator {
		case "+", "-", "*", "/", "%":
		default:
			return 0, false, nil
		}
		lv, lok, lerr := evalIntFast(n.Left, env)
		if lerr != nil {
			return 0, false, lerr
		}
		if !lok {
			return 0, false, nil
		}
		rv, rok, rerr := evalIntFast(n.Right, env)
		if rerr != nil {
			return 0, false, rerr
		}
		if !rok {
			return 0, false, nil
		}
		switch n.Operator {
		case "+":
			return lv + rv, true, nil
		case "-":
			return lv - rv, true, nil
		case "*":
			return lv * rv, true, nil
		case "/":
			if rv == 0 {
				return 0, false, runtimeError("division by zero — guard the right operand with `if y != 0` before using `/`", n.Pos)
			}
			return lv / rv, true, nil
		case "%":
			if rv == 0 {
				return 0, false, runtimeError("modulo by zero — guard the right operand with `if y != 0` before using `%`", n.Pos)
			}
			return lv % rv, true, nil
		}
	}
	return 0, false, nil
}

// evalIntCompareFast handles the bool-returning case: `intExpr CMP intExpr`
// where CMP is ==, !=, <, >, <=, >=. Both operands must be int-fast for
// the path to fire; otherwise the caller falls back to the general
// comparison path (which also handles strings and floats).
//
// Saves: one *Integer box per operand, one Boolean box (returns the
// singleton). For an expression like `y % 4 == 0`, this saves three
// allocations per evaluation — and `y % 4` itself is recursively
// fast-pathed, so the *Integer for the modulo result is also elided.
func evalIntCompareFast(node *ast.InfixExpr, env *Environment) (Object, bool) {
	lv, lok, lerr := evalIntFast(node.Left, env)
	if lerr != nil {
		return lerr, true
	}
	if !lok {
		return nil, false
	}
	rv, rok, rerr := evalIntFast(node.Right, env)
	if rerr != nil {
		return rerr, true
	}
	if !rok {
		return nil, false
	}
	var res bool
	switch node.Operator {
	case "==":
		res = lv == rv
	case "!=":
		res = lv != rv
	case "<":
		res = lv < rv
	case ">":
		res = lv > rv
	case "<=":
		res = lv <= rv
	case ">=":
		res = lv >= rv
	default:
		return nil, false
	}
	if res {
		return TRUE, true
	}
	return FALSE, true
}
