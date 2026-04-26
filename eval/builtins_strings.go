package eval

import (
	"fmt"
	"klex/ast"
)

func init() {
	// substr(str, start) — returns the substring from start to the end of str.
	// substr(str, start, end) — returns the substring from start up to (not including) end.
	// Indices are 0-based. A RuntimeError is raised if start or end is out of bounds.
	//
	//   substr("hello world", 6)     → "world"
	//   substr("hello world", 0, 5)  → "hello"
	Builtins["substr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("substr expects 2 or 3 arguments (str, start [, end])", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return runtimeError(fmt.Sprintf("substr: first argument must be a string, got %s", args[0].Type()), ast.Pos{})
		}
		startObj, ok := args[1].(*Integer)
		if !ok {
			return runtimeError(fmt.Sprintf("substr: start must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		runes := []rune(s.Value)
		n := len(runes)
		start := int(startObj.Value)
		end := n
		if len(args) == 3 {
			endObj, ok := args[2].(*Integer)
			if !ok {
				return runtimeError(fmt.Sprintf("substr: end must be an integer, got %s", args[2].Type()), ast.Pos{})
			}
			end = int(endObj.Value)
		}
		if start < 0 || start > n {
			return runtimeError(fmt.Sprintf("substr: start index %d out of bounds (length %d)", start, n), ast.Pos{})
		}
		if end < start || end > n {
			return runtimeError(fmt.Sprintf("substr: end index %d out of bounds (start %d, length %d)", end, start, n), ast.Pos{})
		}
		return &String{Value: string(runes[start:end])}
	}}

	// slice(arr, start) — returns a new array from start to the end of arr.
	// slice(arr, start, end) — returns a new array from start up to (not including) end.
	// Indices are 0-based. A RuntimeError is raised if start or end is out of bounds.
	//
	//   slice([1,2,3,4,5], 2)     → [3, 4, 5]
	//   slice([1,2,3,4,5], 1, 4)  → [2, 3, 4]
	Builtins["slice"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("slice expects 2 or 3 arguments (arr, start [, end])", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return runtimeError(fmt.Sprintf("slice: first argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		startObj, ok := args[1].(*Integer)
		if !ok {
			return runtimeError(fmt.Sprintf("slice: start must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		n := len(arr.Elements)
		start := int(startObj.Value)
		end := n
		if len(args) == 3 {
			endObj, ok := args[2].(*Integer)
			if !ok {
				return runtimeError(fmt.Sprintf("slice: end must be an integer, got %s", args[2].Type()), ast.Pos{})
			}
			end = int(endObj.Value)
		}
		if start < 0 || start > n {
			return runtimeError(fmt.Sprintf("slice: start index %d out of bounds (length %d)", start, n), ast.Pos{})
		}
		if end < start || end > n {
			return runtimeError(fmt.Sprintf("slice: end index %d out of bounds (start %d, length %d)", end, start, n), ast.Pos{})
		}
		result := make([]Object, end-start)
		copy(result, arr.Elements[start:end])
		return &Array{Elements: result}
	}}
}

