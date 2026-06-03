package eval

import (
	"fmt"
	"klex/ast"
	"sort"
)

func init() {
	// sort — a new array sorted in ascending order (stable).
	//
	// Works on arrays of integers, floats, or strings (numbers may mix with each
	// other; strings can't mix with numbers). Stable: equal elements keep their
	// relative order. The input is not modified.
	//
	// @sig     sort(arr: array) -> array
	// @param   arr  an array of all-numeric or all-string elements
	// @returns a new array sorted ascending
	// @errors  TypeError if elements aren't mutually comparable (e.g. a string mixed with numbers)
	// @example sort([3, 1, 4, 1, 5])   → [1, 1, 3, 4, 5]
	// @since   0.1.0
	// @see     sortBy
	Builtins["sort"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sort expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("sort: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		if len(arr.Elements) < 2 {
			out := make([]Object, len(arr.Elements))
			copy(out, arr.Elements)
			return &Array{Elements: out}
		}

		out := make([]Object, len(arr.Elements))
		copy(out, arr.Elements)

		var sortErr Object
		sort.SliceStable(out, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			a, b := out[i], out[j]
			switch la := a.(type) {
			case *Integer:
				switch rb := b.(type) {
				case *Integer:
					return la.Value < rb.Value
				case *Float:
					return float64(la.Value) < rb.Value
				}
			case *Float:
				switch rb := b.(type) {
				case *Float:
					return la.Value < rb.Value
				case *Integer:
					return la.Value < float64(rb.Value)
				}
			case *String:
				if rb, ok := b.(*String); ok {
					return la.Value < rb.Value
				}
			}
			sortErr = typeError(fmt.Sprintf("sort: cannot compare %s and %s", a.Type(), b.Type()), ast.Pos{})
			return false
		})

		if sortErr != nil {
			return sortErr
		}
		return &Array{Elements: out}
	}}

	// sortBy — a new array sorted by a comparator function (stable).
	//
	// compareFn(a, b) returns true when a should come before b. Stable: equal
	// elements keep their relative order. The input is not modified.
	//
	// @sig     sortBy(arr: array, compareFn: function) -> array
	// @param   arr        the array to sort
	// @param   compareFn  fn(a, b) returning true when a should precede b
	// @returns a new array ordered by compareFn
	// @errors  TypeError if arr isn't an array or compareFn returns a non-bool; any error compareFn raises propagates
	// @example sortBy([3, 1, 2], fn(a, b) { a < b })   → [1, 2, 3]
	// @since   0.1.0
	// @see     sort
	Builtins["sortBy"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("sortBy expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("sortBy: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		if len(arr.Elements) < 2 {
			out := make([]Object, len(arr.Elements))
			copy(out, arr.Elements)
			return &Array{Elements: out}
		}

		out := make([]Object, len(arr.Elements))
		copy(out, arr.Elements)

		var sortErr Object
		sort.SliceStable(out, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			result, err := callCallable(args[1], []Object{out[i], out[j]})
			if err != nil {
				sortErr = err
				return false
			}
			b, ok := result.(*Boolean)
			if !ok {
				sortErr = typeError(fmt.Sprintf("sortBy: compareFn must return bool, got %s", result.Type()), ast.Pos{})
				return false
			}
			return b.Value
		})

		if sortErr != nil {
			return sortErr
		}
		return &Array{Elements: out}
	}}
}
