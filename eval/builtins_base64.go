package eval

import (
	"encoding/base64"
	"fmt"
	"klex/ast"
)

func init() {
	// _base64Encode(s) → string  — standard base64 with padding
	Builtins["_base64Encode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_base64Encode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_base64Encode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: base64.StdEncoding.EncodeToString([]byte(s.Value))}
	}}

	// _base64Decode(s) → (decoded_string, err)  — standard base64
	Builtins["_base64Decode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_base64Decode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_base64Decode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		b, err := base64.StdEncoding.DecodeString(s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(b)}, &Null{}}}
	}}

	// _base64UrlEncode(s) → string  — URL-safe base64, no padding
	Builtins["_base64UrlEncode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_base64UrlEncode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_base64UrlEncode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: base64.RawURLEncoding.EncodeToString([]byte(s.Value))}
	}}

	// _base64UrlDecode(s) → (decoded_string, err)  — URL-safe base64, no padding
	Builtins["_base64UrlDecode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_base64UrlDecode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_base64UrlDecode: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		b, err := base64.RawURLEncoding.DecodeString(s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(b)}, &Null{}}}
	}}
}
