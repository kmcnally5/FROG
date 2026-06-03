package eval

import (
	"fmt"
	"klex/ast"
	"strconv"
	"strings"
	"unicode/utf8"
)

func init() {
	// substr — a substring of str by rune index.
	//
	// Returns the characters from start up to (not including) end. Indices are
	// 0-based and count Unicode code points (runes), not bytes. Omit end to run
	// to the end of the string.
	//
	// @sig     substr(str: string, start: int, [end: int]) -> string
	// @param   str    the source string
	// @param   start  0-based rune index to start from (inclusive)
	// @param   end    0-based rune index to stop at (exclusive); defaults to len(str)
	// @returns the substring between start and end
	// @errors  TypeError if the arguments aren't (string, int[, int]); RuntimeError if start or end is out of bounds
	// @example substr("hello world", 6)     → world
	// @example substr("hello world", 0, 5)  → hello
	// @since   0.1.0
	// @see     slice, split, len
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

	// slice — a sub-range of an array or bytes value (a new copy).
	//
	// Returns elements from start up to (not including) end as a new value of the
	// same type as the input (array → array, bytes → bytes). Indices are 0-based.
	// Omit end to run to the end. The input is not modified.
	//
	// @sig     slice(seq: array | bytes, start: int, [end: int]) -> array | bytes
	// @param   seq    the array or bytes value to slice
	// @param   start  0-based index to start from (inclusive)
	// @param   end    0-based index to stop at (exclusive); defaults to len(seq)
	// @returns a new array or bytes (matching seq) holding seq[start:end]
	// @errors  TypeError if seq isn't an array/bytes or the indices aren't ints; RuntimeError if out of bounds
	// @example slice([1, 2, 3, 4, 5], 2)     → [3, 4, 5]
	// @example slice([1, 2, 3, 4, 5], 1, 4)  → [2, 3, 4]
	// @since   0.1.0
	// @see     substr, concat, len
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

	// parseInt — safely parse a string as an integer, returning (value, err).
	//
	// Trims surrounding whitespace, then parses. Unlike int(), a failure is
	// returned as an error in the tuple rather than raised — use it for untrusted
	// input (CSV, HTTP, database). A float-looking string is rejected with a hint
	// to use parseFloat.
	//
	// @sig     parseInt(str: string) -> (int, error)
	// @param   str  the string to parse
	// @returns (n, null) on success; (null, error) with code "PARSE_ERROR" on failure
	// @errors  TypeError if str is not a string; the parse failure itself is returned in the tuple, not raised
	// @example parseInt("42")   → (42, null)
	// @since   0.1.0
	// @see     parseFloat, int, safe
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

	// parseFloat — safely parse a string as a float, returning (value, err).
	//
	// Trims surrounding whitespace, then parses. Unlike float(), a failure is
	// returned as an error in the tuple rather than raised — use it for untrusted
	// input (CSV, HTTP, database).
	//
	// @sig     parseFloat(str: string) -> (float, error)
	// @param   str  the string to parse
	// @returns (f, null) on success; (null, error) with code "PARSE_ERROR" on failure
	// @errors  TypeError if str is not a string; the parse failure itself is returned in the tuple, not raised
	// @example parseFloat("3.14")   → (3.14, null)
	// @since   0.1.0
	// @see     parseInt, float, safe
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

	// ord — the Unicode code point of a string's first character.
	//
	// Reads the first rune of c and returns its code point. Multi-character
	// inputs use only the first rune (ord("Ab") is 65). Pairs with chr:
	// chr(ord(c)) == c for any single-rune string.
	//
	// @sig     ord(c: string) -> int
	// @param   c  a non-empty string; only its first rune is read
	// @returns the Unicode code point of the first character
	// @errors  TypeError if c is not a string; RuntimeError if c is empty or not valid UTF-8
	// @example ord("A")   → 65
	// @example ord("☃")   → 9731
	// @since   0.1.0
	// @see     chr
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

	// chr — the one-character string for a Unicode code point.
	//
	// Returns a single-rune string for code point n. Pairs with ord:
	// chr(ord(c)) == c for any single-rune string.
	//
	// @sig     chr(n: int) -> string
	// @param   n  a valid Unicode code point: 0..0x10FFFF, excluding the surrogate range 0xD800..0xDFFF
	// @returns a one-character string containing rune n
	// @errors  TypeError if n is not an integer; RuntimeError if n is not a valid code point
	// @example chr(65)     → A
	// @example chr(9731)   → ☃
	// @since   0.1.0
	// @see     ord
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
