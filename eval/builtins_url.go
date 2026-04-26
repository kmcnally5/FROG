package eval

import (
	"fmt"
	"klex/ast"
	"net/url"
)

func init() {
	// _urlEncode(s) → string  — percent-encodes a query string component.
	// Spaces become +, special characters become %XX.
	Builtins["_urlEncode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_urlEncode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_urlEncode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: url.QueryEscape(s.Value)}
	}}

	// _urlDecode(s) → (decoded_string, err)  — decodes a percent-encoded string.
	Builtins["_urlDecode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_urlDecode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_urlDecode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		decoded, err := url.QueryUnescape(s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: decoded}, &Null{}}}
	}}
}
