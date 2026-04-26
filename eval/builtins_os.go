package eval

import (
	"fmt"
	"klex/ast"
	"os"
)

func init() {
	// _osGetenv(key) → string or null
	// Returns the value of the named environment variable, or null if unset.
	Builtins["_osGetenv"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_osGetenv expects 1 argument", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osGetenv: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		val, set := os.LookupEnv(k.Value)
		if !set {
			return &Null{}
		}
		return &String{Value: val}
	}}

	// _osSetenv(key, val) → (null, err)
	Builtins["_osSetenv"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_osSetenv expects 2 arguments", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osSetenv: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		v, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_osSetenv: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		if err := os.Setenv(k.Value, v.Value); err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&Null{}, &Null{}}}
	}}

	// _osCwd() → (path_string, err)
	Builtins["_osCwd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osCwd expects no arguments", ast.Pos{})
		}
		dir, err := os.Getwd()
		if err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: dir}, &Null{}}}
	}}

	// _osHostname() → (hostname_string, err)
	Builtins["_osHostname"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osHostname expects no arguments", ast.Pos{})
		}
		h, err := os.Hostname()
		if err != nil {
			return &Tuple{Elements: []Object{&Null{}, &String{Value: err.Error()}}}
		}
		return &Tuple{Elements: []Object{&String{Value: h}, &Null{}}}
	}}

	// _osPid() → integer  — current process ID
	Builtins["_osPid"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osPid expects no arguments", ast.Pos{})
		}
		return &Integer{Value: os.Getpid()}
	}}

	// _osArgs() → array of strings
	// Returns all command-line arguments. Index 0 is the binary, index 1 is
	// the script path, index 2 onward are user-supplied arguments.
	Builtins["_osArgs"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_osArgs expects no arguments", ast.Pos{})
		}
		elems := make([]Object, len(os.Args))
		for i, a := range os.Args {
			elems[i] = &String{Value: a}
		}
		return &Array{Elements: elems}
	}}

	// _osExit(code) — terminates the process with the given exit code.
	// Does not return.
	Builtins["_osExit"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_osExit expects 1 argument", ast.Pos{})
		}
		code, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("_osExit: argument must be integer, got %s", args[0].Type()), ast.Pos{})
		}
		os.Exit(code.Value)
		return &Null{} // unreachable
	}}
}
