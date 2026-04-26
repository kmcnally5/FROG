package eval

import (
	"bytes"
	"fmt"
	"klex/ast"
	"os/exec"
	"strings"
)

func init() {
	// _processRun(cmd, args) → (stdout, err)
	// Runs cmd with args, captures stdout. stderr is folded into err on failure.
	// args must be an Array of strings (may be empty).
	Builtins["_processRun"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_processRun expects 2 arguments", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processRun: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, err := objectToStringSlice("_processRun", args[1])
		if err != nil {
			return err
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command(cmd.Value, argv...)
		c.Stdout = &stdout
		c.Stderr = &stderr
		if runErr := c.Run(); runErr != nil {
			msg := runErr.Error()
			if se := strings.TrimSpace(stderr.String()); se != "" {
				msg = se
			}
			return &Tuple{Elements: []Object{&Null{}, &String{Value: msg}}}
		}
		return &Tuple{Elements: []Object{&String{Value: stdout.String()}, &Null{}}}
	}}

	// _processExec(cmd, args) → (stdout, stderr, exitCode, err)
	// Runs cmd with args, captures stdout and stderr separately.
	// exitCode is the integer exit code (-1 if the process could not be started).
	// err is non-null only when the process could not be started at all.
	Builtins["_processExec"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("_processExec expects 2 arguments", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processExec: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		argv, err := objectToStringSlice("_processExec", args[1])
		if err != nil {
			return err
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command(cmd.Value, argv...)
		c.Stdout = &stdout
		c.Stderr = &stderr
		runErr := c.Run()
		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// process could not start
				return &Tuple{Elements: []Object{
					&Null{}, &Null{},
					&Integer{Value: -1},
					&String{Value: runErr.Error()},
				}}
			}
		}
		return &Tuple{Elements: []Object{
			&String{Value: stdout.String()},
			&String{Value: stderr.String()},
			&Integer{Value: exitCode},
			&Null{},
		}}
	}}

	// _processShell(cmd) → (stdout, err)
	// Runs cmd as a shell command via /bin/sh -c. stderr is folded into err.
	Builtins["_processShell"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_processShell expects 1 argument", ast.Pos{})
		}
		cmd, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("_processShell: cmd must be string, got %s", args[0].Type()), ast.Pos{})
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command("/bin/sh", "-c", cmd.Value)
		c.Stdout = &stdout
		c.Stderr = &stderr
		if runErr := c.Run(); runErr != nil {
			msg := runErr.Error()
			if se := strings.TrimSpace(stderr.String()); se != "" {
				msg = se
			}
			return &Tuple{Elements: []Object{&Null{}, &String{Value: msg}}}
		}
		return &Tuple{Elements: []Object{&String{Value: stdout.String()}, &Null{}}}
	}}
}

// objectToStringSlice converts a kLex Array of strings to a Go []string.
// Returns a RuntimeError Object on failure.
func objectToStringSlice(caller string, obj Object) ([]string, Object) {
	arr, ok := obj.(*Array)
	if !ok {
		return nil, typeError(fmt.Sprintf("%s: args must be an array, got %s", caller, obj.Type()), ast.Pos{})
	}
	result := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		s, ok := el.(*String)
		if !ok {
			return nil, typeError(fmt.Sprintf("%s: args[%d] must be string, got %s", caller, i, el.Type()), ast.Pos{})
		}
		result[i] = s.Value
	}
	return result, nil
}
