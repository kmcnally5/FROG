package eval

import (
	"fmt"
	"klex/ast"
	"math"
)

// toFloat64Local is a local helper for vector ops that accepts both
// Integer and Float kLex values. (Other builtin files have a private
// toFloat64; we keep ours scoped to this file to avoid coupling.)
func toFloat64Local(o Object) (float64, bool) {
	switch v := o.(type) {
	case *Integer:
		return float64(v.Value), true
	case *Float:
		return v.Value, true
	}
	return 0, false
}

func init() {
	// cosineSim — cosine similarity of two equal-length numeric vectors.
	//
	// Ranges from -1.0 (opposite) through 0.0 (orthogonal) to 1.0 (same
	// direction). The standard similarity metric for embeddings — kNN search in
	// stdlib/ai/vector_store.lex ranks results with it. A zero-magnitude vector
	// returns 0.
	//
	// @sig     cosineSim(a: array, b: array) -> float
	// @param   a  a numeric vector
	// @param   b  a numeric vector of the same length as a
	// @returns the cosine similarity, in [-1.0, 1.0]
	// @errors  TypeError if either isn't a numeric array; RuntimeError on a length mismatch
	// @example cosineSim([1.0, 0.0], [0.0, 1.0])   → 0
	// @example cosineSim([2.0, 0.0], [3.0, 0.0])   → 1
	// @since   0.1.0
	// @see     dotProduct, vecNorm
	Builtins["cosineSim"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("cosineSim expects 2 arguments (vec1, vec2)", ast.Pos{})
		}
		a, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("cosineSim: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		b, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("cosineSim: second argument must be array, got %s", args[1].Type()), ast.Pos{})
		}
		if len(a.Elements) != len(b.Elements) {
			return runtimeError(fmt.Sprintf("cosineSim: dimension mismatch — %d vs %d",
				len(a.Elements), len(b.Elements)), ast.Pos{})
		}
		if len(a.Elements) == 0 {
			return &Float{Value: 0}
		}
		var dot, na, nb float64
		for i, av := range a.Elements {
			af, ok := toFloat64Local(av)
			if !ok {
				return typeError(fmt.Sprintf("cosineSim: vec1[%d] must be number, got %s", i, av.Type()), ast.Pos{})
			}
			bf, ok := toFloat64Local(b.Elements[i])
			if !ok {
				return typeError(fmt.Sprintf("cosineSim: vec2[%d] must be number, got %s", i, b.Elements[i].Type()), ast.Pos{})
			}
			dot += af * bf
			na += af * af
			nb += bf * bf
		}
		if na == 0 || nb == 0 {
			return &Float{Value: 0}
		}
		return &Float{Value: dot / (math.Sqrt(na) * math.Sqrt(nb))}
	}}

	// dotProduct — the inner product of two equal-length numeric vectors.
	//
	// Faster than cosineSim (no normalisation step) — use it when your vectors are
	// already normalised, or when you want raw rather than cosine-corrected
	// similarity.
	//
	// @sig     dotProduct(a: array, b: array) -> float
	// @param   a  a numeric vector
	// @param   b  a numeric vector of the same length as a
	// @returns the sum of a[i]*b[i] over all i
	// @errors  TypeError if either isn't a numeric array; RuntimeError on a length mismatch
	// @example dotProduct([1.0, 2.0, 3.0], [4.0, 5.0, 6.0])   → 32
	// @since   0.1.0
	// @see     cosineSim, vecNorm
	Builtins["dotProduct"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("dotProduct expects 2 arguments (vec1, vec2)", ast.Pos{})
		}
		a, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("dotProduct: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		b, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("dotProduct: second argument must be array, got %s", args[1].Type()), ast.Pos{})
		}
		if len(a.Elements) != len(b.Elements) {
			return runtimeError(fmt.Sprintf("dotProduct: dimension mismatch — %d vs %d",
				len(a.Elements), len(b.Elements)), ast.Pos{})
		}
		var dot float64
		for i, av := range a.Elements {
			af, ok := toFloat64Local(av)
			if !ok {
				return typeError(fmt.Sprintf("dotProduct: vec1[%d] must be number, got %s", i, av.Type()), ast.Pos{})
			}
			bf, ok := toFloat64Local(b.Elements[i])
			if !ok {
				return typeError(fmt.Sprintf("dotProduct: vec2[%d] must be number, got %s", i, b.Elements[i].Type()), ast.Pos{})
			}
			dot += af * bf
		}
		return &Float{Value: dot}
	}}

	// vecNorm — the L2 (Euclidean) norm of a numeric vector.
	//
	// The geometric length of the vector. Normalising by it (`v / vecNorm(v)`)
	// makes dotProduct equivalent to cosineSim.
	//
	// @sig     vecNorm(v: array) -> float
	// @param   v  a numeric vector
	// @returns sqrt of the sum of squares of v's elements
	// @errors  TypeError if v isn't a numeric array
	// @example vecNorm([3.0, 4.0])   → 5
	// @since   0.1.0
	// @see     cosineSim, dotProduct
	Builtins["vecNorm"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("vecNorm expects 1 argument (vec)", ast.Pos{})
		}
		v, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("vecNorm: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		var sum float64
		for i, el := range v.Elements {
			f, ok := toFloat64Local(el)
			if !ok {
				return typeError(fmt.Sprintf("vecNorm: vec[%d] must be number, got %s", i, el.Type()), ast.Pos{})
			}
			sum += f * f
		}
		return &Float{Value: math.Sqrt(sum)}
	}}
}
