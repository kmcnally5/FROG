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
	// cosineSim(a, b) → float
	//
	// Compute the cosine similarity of two equal-length numeric arrays.
	// Range: -1.0 (opposite) … 0.0 (orthogonal) … 1.0 (identical
	// direction). The standard similarity metric for sentence/document
	// embeddings — k-nearest-neighbour searches in stdlib/ai/vector_store.lex
	// use this to rank results.
	//
	// Both arrays must have the same length and contain only numbers
	// (Integer or Float; mixed is fine). Zero-magnitude vectors return 0.
	//
	//   v1 = [1.0, 0.0, 0.0]
	//   v2 = [0.0, 1.0, 0.0]
	//   cosineSim(v1, v2)  // 0.0
	//   cosineSim(v1, v1)  // 1.0
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

	// dotProduct(a, b) → float
	//
	// Inner product of two equal-length numeric arrays. Faster than
	// cosineSim (no normalisation step) — useful when your vectors are
	// already normalised, or when you want raw similarity rather than
	// cosine-corrected similarity.
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

	// vecNorm(v) → float
	//
	// L2 (Euclidean) norm of a numeric array — the geometric length of
	// the vector. Useful for normalising vectors (`v / vecNorm(v)`) so
	// dotProduct becomes equivalent to cosineSim.
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
