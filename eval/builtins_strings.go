package eval

import (
	"fmt"
	"klex/ast"
	"strconv"
	"strings"
	"unicode/utf8"
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
		n := s.RuneLen()
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
		return &String{Value: s.RuneSubstring(start, end)}
	}}

	// slice(arr, start) — returns a new array from start to the end of arr.
	// slice(arr, start, end) — returns a new array from start up to (not including) end.
	// Indices are 0-based. A RuntimeError is raised if start or end is out of bounds.
	//
	//   slice([1,2,3,4,5], 2)     → [3, 4, 5]
	//   slice([1,2,3,4,5], 1, 4)  → [2, 3, 4]
	Builtins["slice"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("slice expects 2 or 3 arguments (arr|bytes, start [, end])", ast.Pos{})
		}
		startObj, ok := args[1].(*Integer)
		if !ok {
			return runtimeError(fmt.Sprintf("slice: start must be an integer, got %s", args[1].Type()), ast.Pos{})
		}
		// Same start/end normalisation for both Array and Bytes.
		readEnd := func(n int) (int, Object) {
			start := int(startObj.Value)
			end := n
			if len(args) == 3 {
				endObj, ok := args[2].(*Integer)
				if !ok {
					return 0, runtimeError(fmt.Sprintf("slice: end must be an integer, got %s", args[2].Type()), ast.Pos{})
				}
				end = int(endObj.Value)
			}
			if start < 0 || start > n {
				return 0, runtimeError(fmt.Sprintf("slice: start index %d out of bounds (length %d)", start, n), ast.Pos{})
			}
			if end < start || end > n {
				return 0, runtimeError(fmt.Sprintf("slice: end index %d out of bounds (start %d, length %d)", end, start, n), ast.Pos{})
			}
			return end, nil
		}
		switch src := args[0].(type) {
		case *Array:
			end, errObj := readEnd(len(src.Elements))
			if errObj != nil {
				return errObj
			}
			result := make([]Object, end-int(startObj.Value))
			copy(result, src.Elements[int(startObj.Value):end])
			return &Array{Elements: result}
		case *Bytes:
			end, errObj := readEnd(len(src.Value))
			if errObj != nil {
				return errObj
			}
			start := int(startObj.Value)
			result := make([]byte, end-start)
			copy(result, src.Value[start:end])
			return &Bytes{Value: result}
		default:
			return runtimeError(fmt.Sprintf("slice: first argument must be an array or bytes, got %s", args[0].Type()), ast.Pos{})
		}
	}}

	// parseInt(str) → (int, err)
	//
	// Safely parses a string as an integer. Trims leading/trailing whitespace.
	// Returns (value, null) on success, (null, error) on failure.
	// Use this instead of int() when the input is untrusted (CSV, HTTP, database).
	//
	// Example:
	//   n, err = parseInt("42")
	//   if err != null { println("bad number: {err.message}")  return }
	//   println(n + 1)   // 43
	Builtins["parseInt"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("parseInt expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("parseInt: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		trimmed := strings.TrimSpace(s.Value)
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			if _, ferr := strconv.ParseFloat(trimmed, 64); ferr == nil {
				return &Tuple{Elements: []Object{NULL, &Error{
					IsUserError: true,
					Code:        "PARSE_ERROR",
					Message:     fmt.Sprintf("parseInt: %q looks like a float — use parseFloat() then convert", s.Value),
				}}}
			}
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "PARSE_ERROR",
				Message:     fmt.Sprintf("parseInt: cannot parse %q as integer", s.Value),
			}}}
		}
		return &Tuple{Elements: []Object{&Integer{Value: n}, NULL}}
	}}

	// parseFloat(str) → (float, err)
	//
	// Safely parses a string as a float. Trims leading/trailing whitespace.
	// Returns (value, null) on success, (null, error) on failure.
	// Use this instead of float() when the input is untrusted (CSV, HTTP, database).
	//
	// Example:
	//   f, err = parseFloat("3.14")
	//   if err != null { println("bad number: {err.message}")  return }
	//   println(f * 2.0)   // 6.28
	Builtins["parseFloat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("parseFloat expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("parseFloat: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		trimmed := strings.TrimSpace(s.Value)
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "PARSE_ERROR",
				Message:     fmt.Sprintf("parseFloat: cannot parse %q as float", s.Value),
			}}}
		}
		return &Tuple{Elements: []Object{&Float{Value: f}, NULL}}
	}}

	// ord(c) → int
	//
	// Returns the Unicode code point of the first character of c. The argument
	// must be a non-empty string; supplying an empty string is a RuntimeError.
	// Multi-character inputs use only the first rune — `ord("Ab")` returns 65.
	//
	// Pairs with chr(): chr(ord(c)) == c for any single-rune string.
	// Use this in place of stdlib/encoding.lex's ord, which scanned a 95-char
	// ASCII table per call (O(95) per char).
	//
	// Examples:
	//   ord("A")  → 65
	//   ord(" ")  → 32
	//   ord("☃")  → 9731
	Builtins["ord"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("ord expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("ord: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		if len(s.Value) == 0 {
			return runtimeError("ord: argument must be a non-empty string", ast.Pos{})
		}
		r, _ := utf8.DecodeRuneInString(s.Value)
		if r == utf8.RuneError {
			return runtimeError("ord: argument contains invalid UTF-8", ast.Pos{})
		}
		return intObj(int(r))
	}}

	// chr(n) → string
	//
	// Returns a single-character string containing the rune for code point n.
	// n must be a valid Unicode code point — negative values, values in the
	// surrogate range (0xD800–0xDFFF), and values >= 0x110000 are RuntimeErrors.
	//
	// Pairs with ord(): chr(ord(c)) == c for any single-rune string.
	//
	// Examples:
	//   chr(65)    → "A"
	//   chr(9731)  → "☃"
	Builtins["chr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("chr expects 1 argument (int)", ast.Pos{})
		}
		n, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("chr: argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		v := n.Value
		if v < 0 || v >= 0x110000 || (v >= 0xD800 && v <= 0xDFFF) {
			return runtimeError(fmt.Sprintf("chr: %d is not a valid Unicode code point", v), ast.Pos{})
		}
		return &String{Value: string(rune(v))}
	}}
}

