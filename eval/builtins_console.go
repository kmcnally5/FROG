package eval

import (
	"fmt"
	"klex/ast"
)

func init() {
	Builtins["color_red"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[31m"}
	}}

	Builtins["color_green"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[32m"}
	}}

	Builtins["color_blue"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[34m"}
	}}

	Builtins["color_yellow"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[33m"}
	}}

	Builtins["color_magenta"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[35m"}
	}}

	Builtins["color_cyan"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[36m"}
	}}

	Builtins["color_white"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[37m"}
	}}

	Builtins["color_black"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[30m"}
	}}

	Builtins["color_bold"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[1m"}
	}}

	Builtins["color_dim"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[2m"}
	}}

	Builtins["color_underline"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[4m"}
	}}

	Builtins["color_reset"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[0m"}
	}}

	Builtins["color_bg_red"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[41m"}
	}}

	Builtins["color_bg_green"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[42m"}
	}}

	Builtins["color_bg_blue"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[44m"}
	}}

	Builtins["color_bg_yellow"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[43m"}
	}}

	Builtins["color_bg_magenta"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[45m"}
	}}

	Builtins["color_bg_cyan"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[46m"}
	}}

	Builtins["color_bg_white"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[47m"}
	}}

	Builtins["color_bg_black"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[40m"}
	}}
	Builtins["colorize"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError(fmt.Sprintf("colorize: expected 2 arguments, got %d", len(args)), ast.Pos{})
		}

		text, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("colorize: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		colorCode, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("colorize: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}

		result := fmt.Sprintf("%s%s\033[0m", colorCode.Value, text.Value)
		return &String{Value: result}
	}}
}
