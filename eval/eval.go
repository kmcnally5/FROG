package eval

// eval.go is the evaluator — the third and final stage of the interpreter.
//
// It receives the AST built by the parser and "executes" it by walking the
// tree recursively. For each node type, Eval() either returns a value or a
// signal (ReturnValue, BreakSignal, ContinueSignal, Error).
//
// This architecture is called a TREE-WALKING INTERPRETER. It is the simplest
// possible execution model: no bytecode, no compilation, no virtual machine.
// The tradeoff is that it's slower than compiled approaches, but for a learning
// project the simplicity is worth it — you can trace exactly what happens.
//
// Error propagation:
// Errors bubble up the call stack like exceptions, but without try/catch.
// Every Eval() call checks `if isError(result) { return result }` immediately
// after getting a value. This means an error at any depth instantly unwinds
// all the way to the top-level program loop, which prints it and stops.
//
// Signal propagation (return, break, continue):
// These work the same way — they are special Object types that bubble up
// through Eval() calls until the right handler catches them:
//   - ReturnValue is unwrapped by the function-call handler
//   - BreakSignal / ContinueSignal are caught by the while-loop handler

import (
	"bufio"
	"fmt"
	"io"
	"klex/ast"
	"klex/lexer"
	"klex/parser"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// stdinReader is shared across all input() calls so buffered bytes are not lost.
var stdinReader = bufio.NewReader(os.Stdin)

// importingFiles tracks which files are currently mid-import to detect cycles.
// It is package-level because import evaluation recurses through Eval itself.
var importingFiles = map[string]bool{}

// KLexVersion is the interpreter version, set by main.go at startup.
// Exposed as the __version__ builtin so FROG programs can read it.
var KLexVersion = "unknown"

// Output is the writer used by println. Defaults to os.Stdout.
// Override this to redirect output (e.g. in WASM builds).
var Output io.Writer = os.Stdout

// Builtins are the built-in functions available in every kLex program.
// They live outside the environment chain so they are always accessible.
// When you call println("hello"), the evaluator looks up "println" in the
// environment, finds this Builtin object, and calls its Fn.
var Builtins = map[string]*Builtin{
	// __version__ returns the interpreter version string.
	"__version__": {Fn: func(args []Object) Object {
		return &String{Value: KLexVersion}
	}},
	"println": {Fn: func(args []Object) Object {
		for _, arg := range args {
			fmt.Fprintln(Output, arg.Inspect())
		}
		return NULL
	}},
	"len": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("len expects 1 argument", ast.Pos{})
		}
		switch arg := args[0].(type) {
		case *Array:
			return &Integer{Value: len(arg.Elements)}
		case *String:
			// Count Unicode code points, not bytes, so len("café") == 4.
			return &Integer{Value: len([]rune(arg.Value))}
		case *Hash:
			return &Integer{Value: len(arg.Pairs)}
		default:
			return typeError(fmt.Sprintf("len not defined for %s", args[0].Type()), ast.Pos{})
		}
	}},
	// keys returns an array of all keys in a hash.
	// Note: Go maps have no guaranteed order, so the key order is non-deterministic.
	"keys": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keys expects 1 argument", ast.Pos{})
		}
		hash, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("keys expects hash, got %s", args[0].Type()), ast.Pos{})
		}
		out := make([]Object, 0, len(hash.Pairs))
		for _, pair := range hash.Pairs {
			out = append(out, pair.Key)
		}
		return &Array{Elements: out}
	}},
	// values returns an array of all values in a hash.
	// Order matches keys() — both iterate the same underlying map in the same pass,
	// but Go map iteration is non-deterministic so do not rely on order across calls.
	"values": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("values expects 1 argument", ast.Pos{})
		}
		hash, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("values expects hash, got %s", args[0].Type()), ast.Pos{})
		}
		out := make([]Object, 0, len(hash.Pairs))
		for _, pair := range hash.Pairs {
			out = append(out, pair.Value)
		}
		return &Array{Elements: out}
	}},
	// hasKey returns true if the hash contains the given key, false otherwise.
	// hasKey(h, "name") — avoids the null-check pattern of h["name"] == null.
	"hasKey": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("hasKey expects 2 arguments", ast.Pos{})
		}
		hash, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("hasKey: first argument must be hash, got %s", args[0].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		_, exists := hash.Pairs[hk]
		return &Boolean{Value: exists}
	}},
	// delete removes a key from a hash in place (mutates the hash).
	"delete": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("delete expects 2 arguments", ast.Pos{})
		}
		hash, ok := args[0].(*Hash)
		if !ok {
			return typeError(fmt.Sprintf("delete expects hash as first argument, got %s", args[0].Type()), ast.Pos{})
		}
		hk, err := toHashKey(args[1], ast.Pos{})
		if err != nil {
			return err
		}
		delete(hash.Pairs, hk)
		return NULL
	}},
	// print outputs a value without a trailing newline.
	// Useful for building output on a single line across multiple calls.
	"print": {Fn: func(args []Object) Object {
		for _, arg := range args {
			fmt.Print(arg.Inspect())
		}
		return NULL
	}},
	// type returns the runtime type name of any value as a string.
	// Useful for debugging: type(42) → "INTEGER", type("hi") → "STRING"
	"type": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("type expects 1 argument", ast.Pos{})
		}
		return &String{Value: string(args[0].Type())}
	}},
	// str converts any value to its string representation.
	// This is how you turn an integer into a string for building output.
	// str(42) → "42", str(true) → "true", str(null) → "null"
	"str": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("str expects 1 argument", ast.Pos{})
		}
		return &String{Value: args[0].Inspect()}
	}},
	// int converts a string or float to an integer.
	// int("42") → 42, int(3.9) → 3 (truncates toward zero)
	"int": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("int expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return v
		case *Float:
			return &Integer{Value: int(v.Value)}
		case *String:
			n, err := strconv.Atoi(v.Value)
			if err != nil {
				// Check whether it looks like a float to give a more helpful message.
				if _, ferr := strconv.ParseFloat(v.Value, 64); ferr == nil {
					return runtimeError(fmt.Sprintf("int: %q looks like a float — use int(float(%q))", v.Value, v.Value), ast.Pos{})
				}
				return runtimeError(fmt.Sprintf("int: cannot convert %q to integer", v.Value), ast.Pos{})
			}
			return &Integer{Value: n}
		default:
			return typeError(fmt.Sprintf("int: cannot convert %s to integer", args[0].Type()), ast.Pos{})
		}
	}},
	// split breaks a string into an array of substrings on a separator.
	// split("a,b,c", ",") → ["a", "b", "c"]
	"split": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("split expects 2 arguments", ast.Pos{})
		}
		str, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("split: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		sep, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("split: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		parts := strings.Split(str.Value, sep.Value)
		elements := make([]Object, len(parts))
		for i, p := range parts {
			elements[i] = &String{Value: p}
		}
		return &Array{Elements: elements}
	}},
	// join concatenates an array of strings into a single string with a separator.
	// join(["a", "b", "c"], ",") → "a,b,c"
	// All array elements must be strings — mixing types is a TypeError.
	"join": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("join expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("join: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		sep, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("join: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		parts := make([]string, len(arr.Elements))
		for i, el := range arr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("join: array element %d must be string, got %s", i, el.Type()), ast.Pos{})
			}
			parts[i] = s.Value
		}
		return &String{Value: strings.Join(parts, sep.Value)}
	}},
	// pop returns a NEW array with the last element removed — it does not mutate.
	// Consistent with push: both operations return new arrays rather than modifying in place.
	// Calling pop on an empty array returns an empty array.
	"pop": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("pop expects 1 argument", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("pop: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		if len(arr.Elements) == 0 {
			return &Array{Elements: []Object{}}
		}
		newElements := make([]Object, len(arr.Elements)-1)
		copy(newElements, arr.Elements)
		return &Array{Elements: newElements}
	}},
	// push returns a NEW array with the element appended — it does not mutate.
	// This is intentional: immutable array operations are safer and more predictable.
	"push": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("push expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("push: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		newElements := make([]Object, len(arr.Elements)+1)
		copy(newElements, arr.Elements)
		newElements[len(arr.Elements)] = args[1]
		return &Array{Elements: newElements}
	}},
	// upper returns a copy of a string with all letters converted to uppercase.
	"upper": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("upper expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("upper: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.ToUpper(s.Value)}
	}},
	// lower returns a copy of a string with all letters converted to lowercase.
	"lower": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("lower expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("lower: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.ToLower(s.Value)}
	}},
	// float converts an integer or string to a float.
	// float(3) → 3.0, float("2.5") → 2.5
	"float": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("float expects 1 argument", ast.Pos{})
		}
		switch v := args[0].(type) {
		case *Integer:
			return &Float{Value: float64(v.Value)}
		case *Float:
			return v
		case *String:
			f, err := strconv.ParseFloat(v.Value, 64)
			if err != nil {
				return runtimeError(fmt.Sprintf("float: cannot convert %q to float", v.Value), ast.Pos{})
			}
			return &Float{Value: f}
		default:
			return typeError(fmt.Sprintf("float: cannot convert %s to float", args[0].Type()), ast.Pos{})
		}
	}},
	// range generates an array of integers.
	// range(stop)             → [0, 1, ..., stop-1]
	// range(start, stop)      → [start, start+1, ..., stop-1]
	// range(start, stop, step)→ [start, start+step, ...] up to but not including stop
	// A negative step counts down. Returns an empty array if the range is empty.
	"range": {Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 3 {
			return runtimeError("range expects 1, 2, or 3 arguments", ast.Pos{})
		}
		toInt := func(o Object, name string) (int, Object) {
			i, ok := o.(*Integer)
			if !ok {
				return 0, typeError(fmt.Sprintf("range: %s must be integer, got %s", name, o.Type()), ast.Pos{})
			}
			return i.Value, nil
		}
		var start, stop, step int
		switch len(args) {
		case 1:
			var err Object
			stop, err = toInt(args[0], "stop")
			if err != nil {
				return err
			}
			start, step = 0, 1
		case 2:
			var err Object
			start, err = toInt(args[0], "start")
			if err != nil {
				return err
			}
			stop, err = toInt(args[1], "stop")
			if err != nil {
				return err
			}
			step = 1
		case 3:
			var err Object
			start, err = toInt(args[0], "start")
			if err != nil {
				return err
			}
			stop, err = toInt(args[1], "stop")
			if err != nil {
				return err
			}
			step, err = toInt(args[2], "step")
			if err != nil {
				return err
			}
			if step == 0 {
				return runtimeError("range: step cannot be zero", ast.Pos{})
			}
		}
		var elements []Object
		for i := start; (step > 0 && i < stop) || (step < 0 && i > stop); i += step {
			elements = append(elements, &Integer{Value: i})
		}
		if elements == nil {
			elements = []Object{}
		}
		return &Array{Elements: elements}
	}},
	// trim removes leading and trailing whitespace from a string.
	"trim": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("trim expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("trim: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: strings.TrimSpace(s.Value)}
	}},
	// replace returns a copy of str with all occurrences of old replaced by new.
	// replace("hello world", "world", "kLex") → "hello kLex"
	"replace": {Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("replace expects 3 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		old, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		new, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("replace: third argument must be string, got %s", args[2].Type()), ast.Pos{})
		}
		return &String{Value: strings.ReplaceAll(s.Value, old.Value, new.Value)}
	}},
	// indexOf returns the index of the first occurrence of substr in str, or -1 if not found.
	// Operates on Unicode code points, consistent with string indexing.
	// indexOf("hello", "ll") → 2,  indexOf("hello", "x") → -1
	"indexOf": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("indexOf expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("indexOf: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		sub, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("indexOf: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		runes := []rune(s.Value)
		subRunes := []rune(sub.Value)
		if len(subRunes) == 0 {
			return &Integer{Value: 0}
		}
		for i := 0; i <= len(runes)-len(subRunes); i++ {
			match := true
			for j, r := range subRunes {
				if runes[i+j] != r {
					match = false
					break
				}
			}
			if match {
				return &Integer{Value: i}
			}
		}
		return &Integer{Value: -1}
	}},
	// startsWith returns true if str begins with the given prefix.
	// startsWith("hello", "he") → true
	"startsWith": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("startsWith expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("startsWith: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		prefix, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("startsWith: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		return &Boolean{Value: strings.HasPrefix(s.Value, prefix.Value)}
	}},
	// endsWith returns true if str ends with the given suffix.
	// endsWith("hello", "lo") → true
	"endsWith": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("endsWith expects 2 arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("endsWith: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		suffix, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("endsWith: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		return &Boolean{Value: strings.HasSuffix(s.Value, suffix.Value)}
	}},
	// env returns the value of an environment variable as a string, or null if unset.
	"env": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("env expects 1 argument", ast.Pos{})
		}
		name, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("env: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		val, set := os.LookupEnv(name.Value)
		if !set {
			return NULL
		}
		return &String{Value: val}
	}},
	// readFile reads the entire contents of a file and returns it as a string.
	// On failure (file not found, permission denied, etc.) it returns a runtime error.
	// Use safe(readFile, path) to handle the error without crashing.
	"readFile": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("readFile expects 1 argument", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("readFile: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		data, err := os.ReadFile(path.Value)
		if err != nil {
			return runtimeError(fmt.Sprintf("readFile: %s", err.Error()), ast.Pos{})
		}
		return &String{Value: string(data)}
	}},
	// writeFile writes a string to a file, creating it if it does not exist and
	// truncating it if it does. On failure returns a runtime error.
	"writeFile": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("writeFile expects 2 arguments", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("writeFile: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("writeFile: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		err := os.WriteFile(path.Value, []byte(content.Value), 0644)
		if err != nil {
			return runtimeError(fmt.Sprintf("writeFile: %s", err.Error()), ast.Pos{})
		}
		return NULL
	}},
	// appendFile appends a string to a file, creating it if it does not exist.
	// On failure returns a runtime error.
	"appendFile": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("appendFile expects 2 arguments", ast.Pos{})
		}
		path, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("appendFile: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		content, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("appendFile: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}
		f, err := os.OpenFile(path.Value, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return runtimeError(fmt.Sprintf("appendFile: %s", err.Error()), ast.Pos{})
		}
		defer f.Close()
		if _, err = f.WriteString(content.Value); err != nil {
			return runtimeError(fmt.Sprintf("appendFile: %s", err.Error()), ast.Pos{})
		}
		return NULL
	}},
	// exec runs an external binary and returns its stdout as a string.
	// The first argument is the command name or path; the second is an array
	// of string arguments. On non-zero exit or any OS error, returns a runtime
	// error — use safe(exec, cmd, args) to handle failures without crashing.
	"exec": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("exec expects 2 arguments", ast.Pos{})
		}
		cmdName, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("exec: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		arr, ok := args[1].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("exec: second argument must be array, got %s", args[1].Type()), ast.Pos{})
		}
		cmdArgs := make([]string, len(arr.Elements))
		for i, el := range arr.Elements {
			s, ok := el.(*String)
			if !ok {
				return typeError(fmt.Sprintf("exec: args[%d] must be string, got %s", i, el.Type()), ast.Pos{})
			}
			cmdArgs[i] = s.Value
		}
		out, err := exec.Command(cmdName.Value, cmdArgs...).Output()
		if err != nil {
			return runtimeError(fmt.Sprintf("exec: %s", err.Error()), ast.Pos{})
		}
		return &String{Value: string(out)}
	}},
	// input prints an optional prompt and reads one line from stdin.
	// The trailing newline is stripped. Returns the line as a string.
	"input": {Fn: func(args []Object) Object {
		if len(args) > 1 {
			return runtimeError("input expects 0 or 1 arguments", ast.Pos{})
		}
		if len(args) == 1 {
			prompt, ok := args[0].(*String)
			if !ok {
				return typeError(fmt.Sprintf("input: argument must be string, got %s", args[0].Type()), ast.Pos{})
			}
			fmt.Print(prompt.Value)
		}
		line, err := stdinReader.ReadString('\n')
		if err != nil {
			// EOF with partial content is fine — return what we have.
			if line == "" {
				return &String{Value: ""}
			}
		}
		line = strings.TrimRight(line, "\r\n")
		return &String{Value: line}
	}},
	// channel creates a new channel for passing values between async tasks.
	// channel()    — unbuffered: send blocks until a receiver is ready.
	// channel(n)   — buffered with capacity n: send blocks only when the buffer is full.
	"channel": {Fn: func(args []Object) Object {
		capacity := 0
		if len(args) == 1 {
			n, ok := args[0].(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("channel: capacity must be an integer, got %s", args[0].Type()), ast.Pos{})
			}
			if n.Value < 0 {
				return runtimeError("channel: capacity must be non-negative", ast.Pos{})
			}
			capacity = n.Value
		} else if len(args) > 1 {
			return runtimeError("channel expects 0 or 1 arguments", ast.Pos{})
		}
		return &Channel{ch: make(chan Object, capacity), done: make(chan struct{})}
	}},

	// send transmits a value to a channel.
	// Blocks until a receiver is ready (unbuffered) or buffer has space (buffered).
	// Returns null on success.
	// Returns false (Boolean) if the channel has been cancelled via cancel() or
	// by a consumer breaking out of a for-in loop — the caller should stop sending.
	// Returns a RuntimeError if the channel's data side is already closed.
	"send": {Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("send expects 2 arguments", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("send: first argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		var result Object = NULL
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = runtimeError("send: channel is closed", ast.Pos{})
				}
			}()
			select {
			case ch.ch <- args[1]:
				result = NULL
			case <-ch.done:
				result = FALSE
			}
		}()
		return result
	}},

	// cancel signals that the consumer of a channel is done and no more values
	// should be sent. Any blocked or future send() call on this channel returns
	// false instead of blocking. cancel() is idempotent — calling it twice is safe.
	// This is the explicit form; breaking out of a for-in loop over a channel
	// also cancels it automatically.
	"cancel": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("cancel expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("cancel: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		func() {
			defer func() { recover() }()
			close(ch.done)
		}()
		return NULL
	}},

	// isError returns true if val is a RuntimeError or TypeError produced by the
	// evaluator. Use this inside stream pipeline stages to detect errors returned
	// by callback functions via safe().
	"isError": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("isError expects 1 argument", ast.Pos{})
		}
		_, ok := args[0].(*Error)
		return &Boolean{Value: ok}
	}},

	// assert checks that condition is true.
	// assert(condition)          — fails with "assert: condition is false"
	// assert(condition, message) — fails with the given message
	// condition must be bool; a non-bool condition is a TypeError.
	// On success returns null. On failure raises a RuntimeError that propagates
	// normally — catchable with safe() like any other error.
	"assert": {Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("assert expects 1 or 2 arguments", ast.Pos{})
		}
		cond, ok := args[0].(*Boolean)
		if !ok {
			return typeError(fmt.Sprintf("assert: condition must be bool, got %s", args[0].Type()), ast.Pos{})
		}
		if cond.Value {
			return NULL
		}
		msg := "assert: condition is false"
		if len(args) == 2 {
			s, ok := args[1].(*String)
			if !ok {
				return typeError(fmt.Sprintf("assert: message must be string, got %s", args[1].Type()), ast.Pos{})
			}
			msg = s.Value
		}
		return runtimeError(msg, ast.Pos{})
	}},

	// recv receives the next value from a channel.
	// Returns (value, true) when a value is available.
	// Returns (null, false) when the channel is closed and empty.
	// Blocks until a value is available or the channel is closed.
	"recv": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("recv expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("recv: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		val, open := <-ch.ch
		if !open {
			return &Tuple{Elements: []Object{NULL, FALSE}}
		}
		return &Tuple{Elements: []Object{val, TRUE}}
	}},

	// close signals that no more values will be sent on the channel.
	// Receivers will drain any buffered values then get (null, false) from recv.
	// Returns null on success. RuntimeError if the channel is already closed.
	"close": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("close expects 1 argument", ast.Pos{})
		}
		ch, ok := args[0].(*Channel)
		if !ok {
			return typeError(fmt.Sprintf("close: argument must be a channel, got %s", args[0].Type()), ast.Pos{})
		}
		var result Object = NULL
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = runtimeError("close: channel is already closed", ast.Pos{})
				}
			}()
			close(ch.ch)
		}()
		return result
	}},

	// sleep pauses execution for the given number of milliseconds.
	// sleep(500) waits half a second. Always returns null.
	"sleep": {Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("sleep expects 1 argument", ast.Pos{})
		}
		ms, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("sleep: argument must be an integer (milliseconds), got %s", args[0].Type()), ast.Pos{})
		}
		time.Sleep(time.Duration(ms.Value) * time.Millisecond)
		return NULL
	}},
}

// init registers builtins that need to call Eval (higher-order functions).
// These cannot be in the Builtins var literal because Go's initialisation
// cycle checker sees: Builtins → closure → Eval → (indirectly) Builtins.
// init() runs after all functions are fully defined, so no cycle exists.
func init() {
	// filter returns a new array containing only the elements for which
	// the function returns true. Example: filter([1,2,3,4], fn(x) { x > 2 }) → [3, 4]
	Builtins["filter"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("filter expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("filter: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if numRequired(fn) != 1 {
				return runtimeError(fmt.Sprintf("filter: function must take 1 argument, got %d required", numRequired(fn)), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			return typeError(fmt.Sprintf("filter: second argument must be function, got %s", args[1].Type()), ast.Pos{})
		}
		out := []Object{}
		for _, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{el})
			if err != nil {
				return err
			}
			b, ok := result.(*Boolean)
			if !ok {
				return typeError(fmt.Sprintf("filter: function must return bool, got %s", result.Type()), ast.Pos{})
			}
			if b.Value {
				out = append(out, el)
			}
		}
		return &Array{Elements: out}
	}}

	// reduce folds an array into a single value by repeatedly applying a function.
	// The function receives (accumulator, currentElement) and returns the new accumulator.
	// Example: reduce([1,2,3], fn(acc, x) { acc + x }, 0) → 6
	Builtins["reduce"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("reduce expects 3 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("reduce: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if numRequired(fn) != 2 {
				return runtimeError(fmt.Sprintf("reduce: function must take 2 arguments, got %d required", numRequired(fn)), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			return typeError(fmt.Sprintf("reduce: second argument must be function, got %s", args[1].Type()), ast.Pos{})
		}
		accumulator := args[2] // start with the initial value
		for _, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{accumulator, el})
			if err != nil {
				return err
			}
			accumulator = result
		}
		return accumulator
	}}

	Builtins["map"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("map expects 2 arguments", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("map: first argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		switch fn := args[1].(type) {
		case *Function:
			if numRequired(fn) != 1 {
				return runtimeError(fmt.Sprintf("map: function must take 1 argument, got %d required", numRequired(fn)), ast.Pos{})
			}
		case *Builtin:
			// arity cannot be checked ahead of time; the builtin will error if called wrong
		default:
			return typeError(fmt.Sprintf("map: second argument must be function, got %s", args[1].Type()), ast.Pos{})
		}
		out := make([]Object, len(arr.Elements))
		for i, el := range arr.Elements {
			result, err := callCallable(args[1], []Object{el})
			if err != nil {
				return err
			}
			out[i] = result
		}
		return &Array{Elements: out}
	}}

	// error creates a first-class Error value with a user-defined code and message.
	// error("NOT_FOUND", "key was missing")
	// The returned Error is NOT a propagation signal — it stays in the environment
	// and can be inspected via .code, .message, and .is(code).
	Builtins["error"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("error() expects 2 arguments: code, message", ast.Pos{})
		}
		code, ok1 := args[0].(*String)
		msg, ok2 := args[1].(*String)
		if !ok1 {
			return typeError("error() first argument (code) must be a string", ast.Pos{})
		}
		if !ok2 {
			return typeError("error() second argument (message) must be a string", ast.Pos{})
		}
		return &Error{IsUserError: true, Code: code.Value, Message: msg.Value}
	}}

	// safe calls a function (user-defined or builtin) and turns any runtime error
	// into a (null, ErrorObject) tuple instead of letting the error propagate and
	// crash the program.
	// Usage: val, err = safe(fn, arg1, arg2, ...)
	// On success returns (result, null); on error returns (null, Error).
	// The Error carries a .code ("RUNTIME_ERROR" or "TYPE_ERROR") and .message.
	// If the function already returns a tuple it is passed through unchanged.
	Builtins["safe"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("safe expects at least 1 argument", ast.Pos{})
		}
		callArgs := args[1:]
		var result Object
		switch fn := args[0].(type) {
		case *Function:
			var err Object
			result, err = applyFunction(fn, callArgs)
			if err != nil {
				e := err.(*Error)
				code := "RUNTIME_ERROR"
				if e.Kind == TypeError {
					code = "TYPE_ERROR"
				}
				return &Tuple{Elements: []Object{NULL, &Error{IsUserError: true, Code: code, Message: e.Message}}}
			}
		case *Builtin:
			result = fn.Fn(callArgs)
			if isError(result) {
				e := result.(*Error)
				code := "RUNTIME_ERROR"
				if e.Kind == TypeError {
					code = "TYPE_ERROR"
				}
				return &Tuple{Elements: []Object{NULL, &Error{IsUserError: true, Code: code, Message: e.Message}}}
			}
		default:
			return typeError(fmt.Sprintf("safe: first argument must be function, got %s", args[0].Type()), ast.Pos{})
		}
		if t, ok := result.(*Tuple); ok {
			return t
		}
		return &Tuple{Elements: []Object{result, NULL}}
	}}

	// async launches a function in a background goroutine and returns a Task
	// immediately. Accepts both user-defined functions and builtins.
	// Usage: task = async(fn, arg1, arg2, ...)
	// Constraint: the function must not mutate shared mutable state (arrays,
	// hashes) that the calling goroutine also accesses — communicate only via
	// the return value, which await() delivers.
	// Note: do not call input() from async — it shares a global stdin reader.
	Builtins["async"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("async expects at least 1 argument", ast.Pos{})
		}
		fnArgs := args[1:]
		task := &Task{done: make(chan struct{})}
		switch fn := args[0].(type) {
		case *Function:
			go func() {
				result, err := applyFunction(fn, fnArgs)
				if err != nil {
					task.result = err
				} else {
					task.result = result
				}
				close(task.done)
			}()
		case *Builtin:
			go func() {
				task.result = fn.Fn(fnArgs)
				close(task.done)
			}()
		default:
			return typeError(fmt.Sprintf("async: first argument must be a function, got %s", args[0].Type()), ast.Pos{})
		}
		return task
	}}

	// await blocks until the given task completes and returns its result.
	// If the task's function produced an error, await propagates it.
	// Usage: result = await(task)
	Builtins["await"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("await expects 1 argument", ast.Pos{})
		}
		task, ok := args[0].(*Task)
		if !ok {
			return typeError(fmt.Sprintf("await: argument must be a task, got %s", args[0].Type()), ast.Pos{})
		}
		<-task.done
		return task.result
	}}
}

// numRequired returns the count of parameters that have no default value.
// Since the parser enforces that defaults come last, this is simply the
// number of leading nil entries in fn.Defaults.
func numRequired(fn *Function) int {
	for i, d := range fn.Defaults {
		if d != nil {
			return i
		}
	}
	return len(fn.Params)
}

// arityError builds a clear argument-count error message that accounts for
// optional (defaulted) parameters.
func arityError(name string, fn *Function, got int, pos ast.Pos) *Error {
	req := numRequired(fn)
	total := len(fn.Params)
	var msg string
	if req == total {
		msg = fmt.Sprintf("%s expects %d argument(s), got %d", name, total, got)
	} else {
		msg = fmt.Sprintf("%s expects %d to %d argument(s), got %d", name, req, total, got)
	}
	return runtimeError(msg, pos)
}

// bindArgs binds call arguments to parameter names in env, filling any
// missing trailing arguments from their default expressions evaluated in
// the function's closure environment (fn.Env).
func bindArgs(fn *Function, args []Object, env *Environment) Object {
	for i, param := range fn.Params {
		if i < len(args) {
			env.Set(param, args[i])
		} else {
			// Missing arg — evaluate default in the closure env.
			defVal := Eval(fn.Defaults[i], fn.Env)
			if isError(defVal) {
				return defVal
			}
			env.Set(param, defVal)
		}
	}
	return nil
}

// applyFunction calls a user-defined Function with the given arguments and
// returns the result. It is used by higher-order builtins (map, filter, reduce)
// that need to invoke kLex functions from inside Go code.
// Returns (result, nil) on success, or (nil, *Error) on failure.
func applyFunction(fn *Function, args []Object) (Object, Object) {
	env := &Environment{
		store: make(map[string]Object),
		outer: fn.Env,
	}
	if fn.Variadic {
		required := len(fn.Params) - 1
		if len(args) < required {
			return nil, runtimeError(
				fmt.Sprintf("function expects at least %d argument(s), got %d", required, len(args)),
				ast.Pos{},
			)
		}
		for i := 0; i < required; i++ {
			env.Set(fn.Params[i], args[i])
		}
		env.Set(fn.Params[required], &Array{Elements: args[required:]})
	} else {
		req := numRequired(fn)
		if len(args) < req || len(args) > len(fn.Params) {
			return nil, arityError("function", fn, len(args), ast.Pos{})
		}
		if errObj := bindArgs(fn, args, env); errObj != nil {
			return nil, errObj
		}
	}
	var result Object = NULL
	for _, node := range fn.Body {
		result = Eval(node, env)
		if isReturn(result) {
			return result.(*ReturnValue).Value, nil
		}
		if isError(result) {
			return nil, result
		}
	}
	return result, nil
}

// callCallable invokes either a user-defined Function or a Builtin with the
// given arguments. Returns (result, nil) on success, (nil, *Error) on failure.
// Used by map/filter/reduce so they accept both function types uniformly.
func callCallable(fn Object, args []Object) (Object, Object) {
	switch f := fn.(type) {
	case *Function:
		return applyFunction(f, args)
	case *Builtin:
		result := f.Fn(args)
		if isError(result) {
			return nil, result
		}
		return result, nil
	default:
		return nil, typeError(fmt.Sprintf("not callable: %s", fn.Type()), ast.Pos{})
	}
}

// toFloat64 extracts the numeric value from an Integer or Float as float64.
// Must only be called after canArithmetic/canCompare has confirmed the type is
// INTEGER_OBJ or FLOAT_OBJ — the fallback 0 is unreachable in correct usage.
func toFloat64(o Object) float64 {
	switch v := o.(type) {
	case *Integer:
		return float64(v.Value)
	case *Float:
		return v.Value
	}
	panic("toFloat64: called with non-numeric type " + string(o.Type()))
}

// -------------------- HASH KEY --------------------

// toHashKey converts a kLex Object into a HashKey suitable for use as a Go
// map key. Only string, integer, and boolean values are hashable.
//
// We combine the Type and a string representation so that integer 1 and
// string "1" produce different keys — {"1": "a", 1: "b"} has two entries.
func toHashKey(obj Object, pos ast.Pos) (HashKey, Object) {
	switch o := obj.(type) {
	case *String:
		return HashKey{Type: STRING_OBJ, Value: o.Value}, nil
	case *Integer:
		return HashKey{Type: INTEGER_OBJ, Value: fmt.Sprintf("%d", o.Value)}, nil
	case *Float:
		return HashKey{Type: FLOAT_OBJ, Value: strconv.FormatFloat(o.Value, 'f', -1, 64)}, nil
	case *Boolean:
		v := "false"
		if o.Value {
			v = "true"
		}
		return HashKey{Type: BOOLEAN_OBJ, Value: v}, nil
	default:
		return HashKey{}, typeError(fmt.Sprintf("unhashable type: %s", obj.Type()), pos)
	}
}

// -------------------- HELPERS --------------------

func isError(obj Object) bool {
	if e, ok := obj.(*Error); ok {
		return !e.IsUserError
	}
	return false
}

// IsError is the exported form of isError for use by main.
func IsError(obj Object) bool { return isError(obj) }

func isReturn(obj Object) bool {
	return obj != nil && obj.Type() == RETURN_OBJ
}

func isBreak(obj Object) bool {
	return obj != nil && obj.Type() == BREAK_OBJ
}

func isContinue(obj Object) bool {
	return obj != nil && obj.Type() == CONTINUE_OBJ
}

// toBool extracts the bool value from a Boolean object.
// Returns (false, false) if the object is not a Boolean — the caller then
// produces a TypeError. This is how kLex enforces that conditions must be bool.
func toBool(obj Object) (bool, bool) {
	b, ok := obj.(*Boolean)
	if !ok {
		return false, false
	}
	return b.Value, true
}

// evalEquals handles == comparisons.
// Special rules:
//   - null == null  → true  (null is a real value, not an error)
//   - null == T     → false (not a TypeError — enables null-check patterns like x == null)
//   - T == U (different non-null types) → TypeError
//   - Same-type values → compare by value
func evalEquals(left, right Object, pos ast.Pos) Object {
	if left.Type() == NULL_OBJ || right.Type() == NULL_OBJ {
		return &Boolean{Value: left.Type() == NULL_OBJ && right.Type() == NULL_OBJ}
	}

	// EnumInstance can be compared to EnumVariant for switch pattern matching:
	// Shape.Circle(5.0) == Shape.Circle  →  true (same type and variant name)
	// The check is symmetric: both orderings are valid.
	if li, ok := left.(*EnumInstance); ok {
		switch r := right.(type) {
		case *EnumVariant:
			return &Boolean{Value: li.TypeName == r.TypeName && li.VariantName == r.VariantName}
		case *EnumInstance:
			// handled below after the type check
		default:
			return FALSE
		}
	}
	if lv, ok := left.(*EnumVariant); ok {
		switch r := right.(type) {
		case *EnumInstance:
			return &Boolean{Value: lv.TypeName == r.TypeName && lv.VariantName == r.VariantName}
		case *EnumVariant:
			return &Boolean{Value: lv.TypeName == r.TypeName && lv.VariantName == r.VariantName}
		default:
			return FALSE
		}
	}

	if left.Type() != right.Type() {
		return typeError(fmt.Sprintf("cannot compare %s and %s", left.Type(), right.Type()), pos)
	}

	switch l := left.(type) {
	case *Integer:
		return &Boolean{Value: l.Value == right.(*Integer).Value}
	case *Float:
		return &Boolean{Value: l.Value == right.(*Float).Value}
	case *Boolean:
		return &Boolean{Value: l.Value == right.(*Boolean).Value}
	case *String:
		return &Boolean{Value: l.Value == right.(*String).Value}
	case *Array, *Hash, *Function:
		// Reference types compare by identity (pointer equality), not by contents.
		return &Boolean{Value: left == right}
	case *EnumInstance:
		r := right.(*EnumInstance)
		if l.TypeName != r.TypeName || l.VariantName != r.VariantName {
			return FALSE
		}
		for name, lv := range l.Fields {
			rv, ok := r.Fields[name]
			if !ok {
				return FALSE
			}
			eq := evalEquals(lv, rv, pos)
			if isError(eq) {
				return eq
			}
			if !eq.(*Boolean).Value {
				return FALSE
			}
		}
		return TRUE
	}

	return FALSE
}

// evalOrderCompare handles <, >, <=, >= for integers, floats, and strings.
// Mixed integer/float is allowed (integer is promoted to float).
// String comparison is lexicographic (Unicode code point order).
// Any other type combination is a TypeError.
func evalNumericCompare(left, right Object, op string, pos ast.Pos) Object {
	if !canCompare(left.Type()) || !canCompare(right.Type()) {
		return typeMismatchError(op, left.Type(), right.Type(), pos)
	}
	if left.Type() == STRING_OBJ && right.Type() == STRING_OBJ {
		l := left.(*String).Value
		r := right.(*String).Value
		switch op {
		case "<":
			return &Boolean{Value: l < r}
		case ">":
			return &Boolean{Value: l > r}
		case "<=":
			return &Boolean{Value: l <= r}
		case ">=":
			return &Boolean{Value: l >= r}
		}
	}
	l := toFloat64(left)
	r := toFloat64(right)
	switch op {
	case "<":
		return &Boolean{Value: l < r}
	case ">":
		return &Boolean{Value: l > r}
	case "<=":
		return &Boolean{Value: l <= r}
	case ">=":
		return &Boolean{Value: l >= r}
	}
	return runtimeError("unknown comparison operator: "+op, pos)
}

// evalLogical handles && and || with proper short-circuit evaluation.
// For &&: if left is false, return false immediately without evaluating right.
// For ||: if left is true, return true immediately without evaluating right.
func evalLogical(n *ast.InfixExpr, env *Environment) Object {
	left := Eval(n.Left, env)
	if isError(left) {
		return left
	}
	if !canLogical(left.Type()) {
		return typeError(fmt.Sprintf("operator %s requires bool, got %s", n.Operator, left.Type()), n.Pos)
	}
	lval := left.(*Boolean).Value
	if n.Operator == "&&" && !lval {
		return FALSE
	}
	if n.Operator == "||" && lval {
		return TRUE
	}
	right := Eval(n.Right, env)
	if isError(right) {
		return right
	}
	if !canLogical(right.Type()) {
		return typeError(fmt.Sprintf("operator %s requires bool, got %s", n.Operator, right.Type()), n.Pos)
	}
	return &Boolean{Value: right.(*Boolean).Value}
}

// -------------------- EVAL CALL --------------------

// evalCall handles function invocation — both user-defined functions and builtins.
//
// For user-defined functions:
//  1. A new Environment is created with its outer set to the function's closure env.
//  2. Parameters are bound to argument values in that new env.
//  3. The function body is evaluated in that new env.
//  4. If a ReturnValue signal is encountered, it is unwrapped here.
//
// This is why kLex has lexical scoping: the function body always runs inside
// the environment where the function was DEFINED (fn.Env), not where it was CALLED.
func evalCall(c *ast.CallExpr, env *Environment) Object {
	// If this is a dot call (obj.method(...)), resolve the receiver first so we
	// can bind `self` inside the function's environment.
	var selfReceiver Object
	if dotExpr, ok := c.Function.(*ast.DotExpr); ok {
		recv := Eval(dotExpr.Left, env)
		if isError(recv) {
			return recv
		}
		switch recv.(type) {
		case *StructInstance:
			selfReceiver = recv
		}
	}

	fnObj := Eval(c.Function, env)
	if isError(fnObj) {
		return fnObj
	}

	// Evaluate all arguments before calling — arguments are eager, not lazy.
	args := []Object{}
	for _, argNode := range c.Args {
		val := Eval(argNode, env)
		if isError(val) {
			return val
		}
		args = append(args, val)
	}

	switch fn := fnObj.(type) {
	case *Builtin:
		return fn.Fn(args)

	case *Function:
		name := fn.Name
		if name == "" {
			name = "anonymous"
		}
		newEnv := &Environment{
			store: make(map[string]Object),
			outer: fn.Env,
		}
		if selfReceiver != nil {
			newEnv.Set("self", selfReceiver)
		}
		if fn.Variadic {
			required := len(fn.Params) - 1
			if len(args) < required {
				return runtimeError(
					fmt.Sprintf("%s expects at least %d argument(s), got %d", name, required, len(args)),
					c.Pos,
				)
			}
			for i := 0; i < required; i++ {
				newEnv.Set(fn.Params[i], args[i])
			}
			rest := args[required:]
			newEnv.Set(fn.Params[required], &Array{Elements: rest})
		} else {
			req := numRequired(fn)
			if len(args) < req || len(args) > len(fn.Params) {
				return arityError(name, fn, len(args), c.Pos)
			}
			if errObj := bindArgs(fn, args, newEnv); errObj != nil {
				return errObj
			}
		}

		var result Object = NULL
		for _, node := range fn.Body {
			result = Eval(node, newEnv)
			if isReturn(result) {
				return result.(*ReturnValue).Value // unwrap the ReturnValue signal
			}
			if isError(result) {
				err := result.(*Error)
				err.Stack = append(err.Stack, Frame{FnName: fn.Name, CallPos: c.Pos})
				return err
			}
		}
		return result

	case *EnumVariant:
		if len(args) != len(fn.Fields) {
			return runtimeError(fmt.Sprintf("%s.%s expects %d argument(s), got %d",
				fn.TypeName, fn.VariantName, len(fn.Fields), len(args)), c.Pos)
		}
		fields := make(map[string]Object, len(fn.Fields))
		for i, name := range fn.Fields {
			fields[name] = args[i]
		}
		return &EnumInstance{
			TypeName:    fn.TypeName,
			VariantName: fn.VariantName,
			FieldNames:  fn.Fields,
			Fields:      fields,
		}

	default:
		return typeError(fmt.Sprintf("not a function, got %s", fnObj.Type()), c.Pos)
	}
}

// -------------------- MAIN EVAL --------------------

// Eval is the central dispatch function. It receives any AST node and returns
// the Object that node evaluates to. It is called recursively — evaluating a
// program evaluates each statement, evaluating an infix expression evaluates
// both sides, and so on.
func Eval(node ast.Node, env *Environment) Object {
	switch n := node.(type) {

	// ---------------- PROGRAM ----------------
	// Evaluate each statement in order. Stop and print if an error occurs.
	// The final statement's value is the program's result (unused in practice).
	// Leaked control-flow signals (return/break/continue outside their valid
	// context) are programming errors — convert them to RuntimeErrors rather
	// than silently swallowing them.
	case *ast.Program:
		var result Object = NULL
		for _, stmt := range n.Statements {
			result = Eval(stmt, env)
			if isError(result) {
				fmt.Println(result.Inspect())
				return result
			}
			if isReturn(result) {
				err := runtimeError("return outside function", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
			if isBreak(result) {
				err := runtimeError("break outside loop", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
			if isContinue(result) {
				err := runtimeError("continue outside loop", ast.Pos{})
				fmt.Println(err.Inspect())
				return err
			}
		}
		return result

	// ---------------- ASSIGNMENT ----------------
	// Evaluate the right-hand side, then store the result in the environment.
	// If the value is an anonymous function, stamp its name onto it now —
	// this is what enables recursion (the function can refer to itself by name
	// because the name is in the outer env when the body eventually runs).
	case *ast.AssignStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if n.Name == "_" {
			return val // discard — evaluate for side effects, do not store
		}
		if errObj := env.CheckWritable(n.Name); errObj != nil {
			return errObj
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		env.Assign(n.Name, val)
		return val

	case *ast.LetStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if n.Name == "_" {
			return val // discard — evaluate for side effects, do not store
		}
		// let only creates in current scope — only block if const in THIS scope.
		if env.consts != nil && env.consts[n.Name] {
			return runtimeError("cannot reassign constant "+n.Name, n.Pos)
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		env.Set(n.Name, val)
		return val

	// ---------------- CONST DECLARATION ----------------
	case *ast.ConstStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		if fn, ok := val.(*Function); ok && fn.Name == "" {
			fn.Name = n.Name
		}
		env.SetConst(n.Name, val)
		return val

	// ---------------- ENUM DECLARATION ----------------
	// Build an EnumDef and bind it in the environment under the enum name.
	case *ast.EnumDecl:
		def := &EnumDef{
			Name:     n.Name,
			Variants: make(map[string][]string, len(n.Variants)),
		}
		for _, v := range n.Variants {
			def.Variants[v.Name] = v.Fields
		}
		env.Assign(n.Name, def)
		return def

	// ---------------- STRUCT DECLARATION ----------------
	// Build a StructDef and bind it in the environment under the struct name.
	case *ast.StructDecl:
		def := &StructDef{
			Name:    n.Name,
			Fields:  n.Fields,
			Methods: make(map[string]*Function),
		}
		for _, m := range n.Methods {
			def.Methods[m.Name] = &Function{
				Name:     m.Name,
				Params:   m.Params,
				Defaults: m.Defaults,
				Variadic: m.Variadic,
				Body:     m.Body,
				Env:      env,
			}
		}
		env.Assign(n.Name, def)
		return def

	// ---------------- STRUCT LITERAL ----------------
	// Look up the StructDef, validate field names, evaluate values, create instance.
	case *ast.StructLiteral:
		defObj, ok := env.Get(n.Name)
		if !ok {
			return runtimeError(fmt.Sprintf("undefined struct type %q", n.Name), n.Pos)
		}
		def, ok := defObj.(*StructDef)
		if !ok {
			return typeError(fmt.Sprintf("%q is not a struct type", n.Name), n.Pos)
		}
		// Check all provided fields are declared.
		declared := make(map[string]bool, len(def.Fields))
		for _, f := range def.Fields {
			declared[f] = true
		}
		provided := make(map[string]bool, len(n.Fields))
		for _, fi := range n.Fields {
			if !declared[fi.Name] {
				return runtimeError(fmt.Sprintf("struct %s has no field %q", def.Name, fi.Name), n.Pos)
			}
			if provided[fi.Name] {
				return runtimeError(fmt.Sprintf("field %q set more than once in struct literal", fi.Name), n.Pos)
			}
			provided[fi.Name] = true
		}
		// All declared fields must be initialised.
		for _, f := range def.Fields {
			if !provided[f] {
				return runtimeError(fmt.Sprintf("struct %s: field %q not initialised", def.Name, f), n.Pos)
			}
		}
		// Evaluate field values.
		fields := make(map[string]Object, len(n.Fields))
		for _, fi := range n.Fields {
			val := Eval(fi.Value, env)
			if isError(val) {
				return val
			}
			fields[fi.Name] = val
		}
		return &StructInstance{Def: def, Fields: fields}

	// ---------------- IDENTIFIER ----------------
	// Look up the variable name in the current environment chain.
	case *ast.Ident:
		if n.Value == "_" {
			return runtimeError("_ is a discard — its value cannot be read", n.Pos)
		}
		val, ok := env.Get(n.Value)
		if !ok {
			return runtimeError("undefined variable: "+n.Value, n.Pos)
		}
		return val

	// ---------------- FUNCTION LITERAL ----------------
	// Capture the current environment as the closure. The function object
	// remembers where it was created, not where it will be called.
	case *ast.FunctionLiteral:
		return &Function{
			Params:    n.Params,
			Defaults:  n.Defaults,
			Variadic:  n.Variadic,
			Body:      n.Body,
			Env:       env, // closure captured here
		}

	// ---------------- SWITCH ----------------
	// Value switch:      switch expr { case val, val { } default { } }
	// Expression switch: switch       { case bool_expr { } default { } }
	// Cases are tried in order; first match wins, no fallthrough.
	case *ast.SwitchStmt:
		var subject Object
		if n.Subject != nil {
			subject = Eval(n.Subject, env)
			if isError(subject) {
				return subject
			}
		}
		for _, sc := range n.Cases {
			matched := false
			matchEnv := env
			for _, valNode := range sc.Values {
				if pat, ok := valNode.(*ast.EnumPattern); ok {
					if subject == nil {
						return runtimeError("enum pattern requires a switch subject", pat.Pos)
					}
					inst, ok := subject.(*EnumInstance)
					if !ok {
						break
					}
					patVal := Eval(pat.Pattern, env)
					if isError(patVal) {
						return patVal
					}
					var patType, patVariant string
					switch pv := patVal.(type) {
					case *EnumVariant:
						patType, patVariant = pv.TypeName, pv.VariantName
					case *EnumInstance:
						patType, patVariant = pv.TypeName, pv.VariantName
					default:
						return runtimeError(fmt.Sprintf("enum pattern must reference an enum variant, got %s", patVal.Type()), pat.Pos)
					}
					if inst.TypeName != patType || inst.VariantName != patVariant {
						break
					}
					if len(pat.Bindings) != len(inst.FieldNames) {
						return runtimeError(fmt.Sprintf(
							"%s.%s has %d field(s) but pattern binds %d",
							inst.TypeName, inst.VariantName, len(inst.FieldNames), len(pat.Bindings),
						), pat.Pos)
					}
					childEnv := &Environment{store: make(map[string]Object), outer: env}
					for i, name := range pat.Bindings {
						childEnv.Set(name, inst.Fields[inst.FieldNames[i]])
					}
					matched = true
					matchEnv = childEnv
					break
				}
				val := Eval(valNode, env)
				if isError(val) {
					return val
				}
				if subject != nil {
					// Value switch: use the same equality logic as the == operator.
					eq := evalEquals(subject, val, n.Pos)
					if isError(eq) {
						return eq
					}
					if eq.(*Boolean).Value {
						matched = true
						break
					}
				} else {
					// Expression switch: each value must be a boolean.
					b, ok := val.(*Boolean)
					if !ok {
						return typeError(
							fmt.Sprintf("switch case expression must be boolean, got %s", val.Type()),
							n.Pos,
						)
					}
					if b.Value {
						matched = true
						break
					}
				}
			}
			if matched {
				var result Object = NULL
				for _, stmt := range sc.Body {
					result = Eval(stmt, matchEnv)
					if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
						return result
					}
				}
				return result
			}
		}
		// No case matched — run default if present.
		if n.HasDefault {
			var result Object = NULL
			for _, stmt := range n.Default {
				result = Eval(stmt, env)
				if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}
		// No default. If the subject is an enum instance, the unmatched variant
		// is a programming error — not a silent null. Require either a matching
		// case or an explicit default {}.
		if subject != nil {
			if inst, ok := subject.(*EnumInstance); ok {
				return runtimeError(
					fmt.Sprintf("switch: %s.%s not handled — add a case or a default",
						inst.TypeName, inst.VariantName),
					n.Pos,
				)
			}
		}
		return NULL

	// ---------------- SELECT ----------------
	// Blocks until one channel operation can proceed, then runs that case's body.
	// Uses reflect.Select so multiple channels can be waited on simultaneously.
	// If multiple cases are ready at once, one is chosen at random (Go semantics).
	// A default case makes the select non-blocking.
	case *ast.SelectStmt:
		// Build the reflect.SelectCase slice in lock-step with n.Cases so we
		// can map the chosen index back to the right body and bindings.
		reflCases := make([]reflect.SelectCase, 0, len(n.Cases))
		for _, sc := range n.Cases {
			switch sc.Kind {
			case ast.SelectRecv:
				chObj := Eval(sc.Chan, env)
				if isError(chObj) {
					return chObj
				}
				ch, ok := chObj.(*Channel)
				if !ok {
					return typeError(fmt.Sprintf("select recv: expected channel, got %s", chObj.Type()), sc.Pos)
				}
				reflCases = append(reflCases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(ch.ch),
				})
			case ast.SelectSend:
				chObj := Eval(sc.Chan, env)
				if isError(chObj) {
					return chObj
				}
				ch, ok := chObj.(*Channel)
				if !ok {
					return typeError(fmt.Sprintf("select send: expected channel, got %s", chObj.Type()), sc.Pos)
				}
				val := Eval(sc.SendVal, env)
				if isError(val) {
					return val
				}
				// reflect.Select requires the Send value to match the channel's
				// element type exactly. chan Object has element type Object
				// (interface), so we must wrap the concrete value in an interface
				// reflect.Value rather than using the concrete type directly.
				var iface Object = val
				reflCases = append(reflCases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.ch),
					Send: reflect.ValueOf(&iface).Elem(),
				})
			case ast.SelectDefault:
				reflCases = append(reflCases, reflect.SelectCase{
					Dir: reflect.SelectDefault,
				})
			}
		}

		chosen, recvVal, recvOK := reflect.Select(reflCases)
		sc := n.Cases[chosen]

		// Build a fresh scope for the chosen case body.
		caseEnv := &Environment{store: make(map[string]Object), outer: env}

		// For recv cases, bind the received value and ok flag using Assign() so
		// that existing outer-scope variables are updated (same semantics as a
		// regular val, ok = recv(ch) assignment statement).
		if sc.Kind == ast.SelectRecv {
			var val Object
			if recvOK {
				val = recvVal.Interface().(Object)
			} else {
				val = NULL
			}
			ok := &Boolean{Value: recvOK}
			if len(sc.Vars) >= 1 && sc.Vars[0] != "_" {
				if errObj := env.CheckWritable(sc.Vars[0]); errObj != nil {
					return errObj
				}
				env.Assign(sc.Vars[0], val)
			}
			if len(sc.Vars) >= 2 && sc.Vars[1] != "_" {
				if errObj := env.CheckWritable(sc.Vars[1]); errObj != nil {
					return errObj
				}
				env.Assign(sc.Vars[1], ok)
			}
		}

		var result Object = NULL
		for _, stmt := range sc.Body {
			result = Eval(stmt, caseEnv)
			if isReturn(result) || isError(result) || isBreak(result) || isContinue(result) {
				return result
			}
		}
		return result

	// ---------------- FOR IN ----------------
	// Evaluates the collection, then iterates over each element, binding it
	// to the loop variable in a fresh inner scope for each iteration.
	// The loop variable does not leak into the outer scope after the loop ends.
	// break and continue work exactly as they do in while loops.
	case *ast.ForInStmt:
		collection := Eval(n.Collection, env)
		if isError(collection) {
			return collection
		}

		runBody := func(bindings map[string]Object) Object {
			iterEnv := &Environment{store: bindings, outer: env}
			for _, stmt := range n.Body {
				result := Eval(stmt, iterEnv)
				if isError(result) || isReturn(result) {
					return result
				}
				if isBreak(result) {
					return NULL
				}
				if isContinue(result) {
					break
				}
			}
			return nil // nil = keep going
		}

		var result Object = NULL

		addBinding := func(m map[string]Object, name string, val Object) {
			if name != "_" {
				m[name] = val
			}
		}

		switch coll := collection.(type) {
		case *Array:
			for i, el := range coll.Elements {
				bindings := map[string]Object{}
				if n.ValueVar == "" {
					addBinding(bindings, n.Variable, el)
				} else {
					addBinding(bindings, n.Variable, &Integer{Value: i})
					addBinding(bindings, n.ValueVar, el)
				}
				if r := runBody(bindings); r != nil {
					if isBreak(r) {
						return NULL
					}
					return r
				}
			}
		case *Hash:
			if n.ValueVar == "" {
				return typeError("for-in over a hash requires two variables: for k, v in hash", n.Pos)
			}
			for _, pair := range coll.Pairs {
				bindings := map[string]Object{}
				addBinding(bindings, n.Variable, pair.Key)
				addBinding(bindings, n.ValueVar, pair.Value)
				if r := runBody(bindings); r != nil {
					if isBreak(r) {
						return NULL
					}
					return r
				}
			}
		case *Channel:
			if n.ValueVar != "" {
				return typeError("for-in over a channel does not support two variables", n.Pos)
			}
			for val := range coll.ch {
				if r := runBody(map[string]Object{n.Variable: val}); r != nil {
					if isBreak(r) {
						// Signal the producer that the consumer is done.
						// Closing done is idempotent via recover.
						func() {
							defer func() { recover() }()
							close(coll.done)
						}()
						return NULL
					}
					return r
				}
			}
		default:
			return typeError(fmt.Sprintf("for-in requires array, hash, or channel, got %s", collection.Type()), n.Pos)
		}

		return result

	// ---------------- WHILE ----------------
	// Re-evaluate the condition before each iteration.
	// BreakSignal exits the loop. ContinueSignal skips remaining body statements
	// so the outer for{} loop re-evaluates the condition on the next iteration.
	// ReturnValue and errors pass through unchanged — they unwind the call stack.
	case *ast.WhileStmt:
		var result Object = NULL
		for {
			cond := Eval(n.Condition, env)
			if isError(cond) {
				return cond
			}
			b, ok := toBool(cond)
			if !ok {
				return typeError(
					fmt.Sprintf("while condition must be bool, got %s (%s)", cond.Type(), cond.Inspect()),
					n.Pos,
				)
			}
			if !b {
				break
			}

			for _, stmt := range n.Body {
				result = Eval(stmt, env)
				if isError(result) {
					return result
				}
				if isReturn(result) {
					return result
				}
				if isBreak(result) {
					return NULL
				}
				if isContinue(result) {
					break // skip remaining stmts, outer for{} re-evaluates condition
				}
			}
		}
		return result

	// ---------------- RETURN ----------------
	// Wrap the value in ReturnValue so it can bubble up through Eval() calls
	// until evalCall() unwraps it.
	case *ast.ReturnStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		return &ReturnValue{Value: val}

	// Break and continue produce signal objects that bubble up to the while handler.
	case *ast.BreakStmt:
		return &BreakSignal{}

	case *ast.ContinueStmt:
		return &ContinueSignal{}

	// ---------------- IF ----------------
	case *ast.IfStmt:
		cond := Eval(n.Condition, env)
		if isError(cond) {
			return cond
		}
		b, ok := toBool(cond)
		if !ok {
			return typeError(
				fmt.Sprintf("if condition must be bool, got %s (%s)", cond.Type(), cond.Inspect()),
				n.Pos,
			)
		}

		if b {
			var result Object = NULL
			for _, stmt := range n.Body {
				result = Eval(stmt, env)
				if isError(result) || isReturn(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}

		if n.ElseBody != nil {
			var result Object = NULL
			for _, stmt := range n.ElseBody {
				result = Eval(stmt, env)
				if isError(result) || isReturn(result) || isBreak(result) || isContinue(result) {
					return result
				}
			}
			return result
		}

		return NULL

	// ---------------- TUPLE LITERAL ----------------
	// Produced by `return a, b` — evaluates each element and wraps them in a Tuple.
	case *ast.TupleLiteral:
		elements := make([]Object, len(n.Elements))
		for i, el := range n.Elements {
			val := Eval(el, env)
			if isError(val) {
				return val
			}
			elements[i] = val
		}
		return &Tuple{Elements: elements}

	// ---------------- MULTI ASSIGN ----------------
	// Unpacks a Tuple into multiple variables: val, err = divide(10, 2)
	// The RHS must evaluate to a Tuple with exactly the right number of elements.
	case *ast.MultiAssignStmt:
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		tuple, ok := val.(*Tuple)
		if !ok {
			return runtimeError(
				fmt.Sprintf("cannot unpack %s into %d variables — right side must return multiple values", val.Type(), len(n.Names)),
				n.Pos,
			)
		}
		if len(tuple.Elements) != len(n.Names) {
			return runtimeError(
				fmt.Sprintf("cannot unpack %d values into %d variables", len(tuple.Elements), len(n.Names)),
				n.Pos,
			)
		}
		for i, name := range n.Names {
			if name != "_" {
				if errObj := env.CheckWritable(name); errObj != nil {
					return errObj
				}
				env.Assign(name, tuple.Elements[i])
			}
		}
		return tuple

	// ---------------- ARRAY LITERAL ----------------
	// Evaluate each element expression and collect the results into an Array.
	case *ast.ArrayLiteral:
		elements := []Object{}
		for _, el := range n.Elements {
			val := Eval(el, env)
			if isError(val) {
				return val
			}
			elements = append(elements, val)
		}
		return &Array{Elements: elements}

	// ---------------- HASH LITERAL ----------------
	// Evaluate each key and value, convert the key to a HashKey, store the pair.
	case *ast.HashLiteral:
		pairs := make(map[HashKey]HashPair)
		for _, p := range n.Pairs {
			key := Eval(p.Key, env)
			if isError(key) {
				return key
			}
			hk, err := toHashKey(key, n.Pos)
			if err != nil {
				return err
			}
			val := Eval(p.Value, env)
			if isError(val) {
				return val
			}
			pairs[hk] = HashPair{Key: key, Value: val}
		}
		return &Hash{Pairs: pairs}

	// ---------------- INDEX EXPRESSION ----------------
	// Handles both arr[i] and map["key"].
	// The runtime type of `left` determines which path is taken.
	case *ast.IndexExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(n.Index, env)
		if isError(index) {
			return index
		}
		switch l := left.(type) {
		case *Array:
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("array index must be integer, got %s", index.Type()), n.Pos)
			}
			if idx.Value < 0 || idx.Value >= len(l.Elements) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (array length %d)", idx.Value, len(l.Elements)), n.Pos)
			}
			return l.Elements[idx.Value]
		case *Hash:
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			pair, ok := l.Pairs[hk]
			if !ok {
				// Missing key returns null, not an error — enables `m["k"] == null` checks.
				return NULL
			}
			return pair.Value
		case *String:
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("string index must be integer, got %s", index.Type()), n.Pos)
			}
			runes := []rune(l.Value)
			if idx.Value < 0 || idx.Value >= len(runes) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (string length %d)", idx.Value, len(runes)), n.Pos)
			}
			return &String{Value: string(runes[idx.Value])}
		case *StructInstance:
			return typeError(fmt.Sprintf("cannot use bracket access on struct %s — use dot notation: struct.field", l.Def.Name), n.Pos)
		default:
			return typeError(fmt.Sprintf("index operator not supported for %s", left.Type()), n.Pos)
		}

	// ---------------- INDEX ASSIGNMENT ----------------
	// Handles arr[i] = val and map["key"] = val.
	// Both arrays and hashes are pointer types in Go, so mutation is visible
	// everywhere the same object is referenced.
	case *ast.IndexAssignStmt:
		obj := Eval(n.Left.Left, env) // the thing being indexed (array or hash)
		if isError(obj) {
			return obj
		}
		index := Eval(n.Left.Index, env)
		if isError(index) {
			return index
		}
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		switch o := obj.(type) {
		case *Hash:
			hk, err := toHashKey(index, n.Pos)
			if err != nil {
				return err
			}
			o.Pairs[hk] = HashPair{Key: index, Value: val}
			return val
		case *Array:
			idx, ok := index.(*Integer)
			if !ok {
				return typeError(fmt.Sprintf("array index must be integer, got %s", index.Type()), n.Pos)
			}
			if idx.Value < 0 || idx.Value >= len(o.Elements) {
				return runtimeError(fmt.Sprintf("index %d out of bounds (array length %d)", idx.Value, len(o.Elements)), n.Pos)
			}
			o.Elements[idx.Value] = val
			return val
		default:
			return typeError(fmt.Sprintf("index assignment not supported for %s", obj.Type()), n.Pos)
		}

	// ---------------- IMPORT ----------------
	// Loads a kLex file, evaluates it in a fresh environment, and binds
	// the resulting module to the alias name in the current scope.
	//
	// Resolution order:
	//   1. n.Path as-is (relative to CWD — for local project files)
	//   2. $KLEX_PATH/n.Path (for stdlib and shared libraries)
	//
	// This means `import "math.lex" as math` finds a local math.lex first,
	// then falls back to $KLEX_PATH/math.lex — so local files can override stdlib.
	case *ast.ImportStmt:
		// Resolve path: try local first, then KLEX_PATH.
		var resolvedPath string
		src, readErr := os.ReadFile(n.Path)
		if readErr != nil {
			klexPath := os.Getenv("KLEX_PATH")
			if klexPath == "" {
				return runtimeError(fmt.Sprintf("cannot import %q: file not found (KLEX_PATH not set)", n.Path), n.Pos)
			}
			resolvedPath = klexPath + "/" + n.Path
			src, readErr = os.ReadFile(resolvedPath)
			if readErr != nil {
				return runtimeError(fmt.Sprintf("cannot import %q: not found locally or in KLEX_PATH (%s)", n.Path, klexPath), n.Pos)
			}
		} else {
			resolvedPath = n.Path
		}

		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			absPath = resolvedPath
		}
		if importingFiles[absPath] {
			return runtimeError(fmt.Sprintf("import cycle detected: %q is already being imported", n.Path), n.Pos)
		}
		importingFiles[absPath] = true

		l := lexer.New(string(src))
		p := parser.New(l)
		program := p.ParseProgram()
		if len(program.Errors) > 0 {
			delete(importingFiles, absPath)
			return runtimeError(fmt.Sprintf("parse error in %q: %s", n.Path, program.Errors[0]), n.Pos)
		}
		modEnv := NewEnv()
		result := Eval(program, modEnv)
		delete(importingFiles, absPath) // clear before checking error so re-import after failure works
		if isError(result) {
			return result
		}
		mod := &Module{Name: n.Alias, Env: modEnv}
		env.Assign(n.Alias, mod)
		return mod

	// ---------------- DOT EXPRESSION ----------------
	// Looks up a property name in a module's environment.
	// math.add → finds "add" in math's env and returns it.
	// The returned value can be anything — a function, a number, a string.
	case *ast.DotExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		switch obj := left.(type) {
		case *Module:
			val, ok := obj.Env.store[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("module %q has no property %q", obj.Name, n.Property), n.Pos)
			}
			return val
		case *StructInstance:
			// Field access.
			if val, ok := obj.Fields[n.Property]; ok {
				return val
			}
			// Method access — return the function itself (self injected at call time).
			if fn, ok := obj.Def.Methods[n.Property]; ok {
				return fn
			}
			return runtimeError(fmt.Sprintf("struct %s has no field or method %q", obj.Def.Name, n.Property), n.Pos)
		case *EnumDef:
			fields, ok := obj.Variants[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("enum %s has no variant %q", obj.Name, n.Property), n.Pos)
			}
			// Zero-field variants are instances — no call required.
			if len(fields) == 0 {
				return &EnumInstance{
					TypeName:    obj.Name,
					VariantName: n.Property,
					FieldNames:  nil,
					Fields:      map[string]Object{},
				}
			}
			// Data-carrying variants return a descriptor; calling it produces an instance.
			return &EnumVariant{TypeName: obj.Name, VariantName: n.Property, Fields: fields}
		case *EnumInstance:
			val, ok := obj.Fields[n.Property]
			if !ok {
				return runtimeError(fmt.Sprintf("enum variant %s.%s has no field %q",
					obj.TypeName, obj.VariantName, n.Property), n.Pos)
			}
			return val
		case *Error:
			if !obj.IsUserError {
				return typeError(fmt.Sprintf("dot access not supported on %s", left.Type()), n.Pos)
			}
			switch n.Property {
			case "code":
				return &String{Value: obj.Code}
			case "message":
				return &String{Value: obj.Message}
			case "is":
				captured := obj
				return &Builtin{Fn: func(args []Object) Object {
					if len(args) != 1 {
						return runtimeError("is() expects 1 argument", n.Pos)
					}
					s, ok := args[0].(*String)
					if !ok {
						return typeError("is() argument must be a string", n.Pos)
					}
					return &Boolean{Value: captured.Code == s.Value}
				}}
			default:
				return runtimeError(fmt.Sprintf("error has no property %q", n.Property), n.Pos)
			}
		case *Null:
			return typeError(fmt.Sprintf("cannot access .%s on null — check for null before dot access", n.Property), n.Pos)
		case *Hash:
			return typeError(fmt.Sprintf("cannot use dot access on hash — use bracket notation: hash[\"%s\"]", n.Property), n.Pos)
		default:
			return typeError(fmt.Sprintf("dot access not supported on %s", left.Type()), n.Pos)
		}

	case *ast.DotAssignStmt:
		obj := Eval(n.Left.Left, env)
		if isError(obj) {
			return obj
		}
		val := Eval(n.Value, env)
		if isError(val) {
			return val
		}
		switch target := obj.(type) {
		case *StructInstance:
			if _, ok := target.Fields[n.Left.Property]; !ok {
				return runtimeError(fmt.Sprintf("struct %s has no field %q", target.Def.Name, n.Left.Property), n.Pos)
			}
			target.Fields[n.Left.Property] = val
		default:
			return typeError(fmt.Sprintf("dot assignment not supported on %s", obj.Type()), n.Pos)
		}
		return val

	// ---------------- LITERALS ----------------
	// Leaf nodes — they just produce their value directly.
	case *ast.NullLiteral:
		return NULL

	case *ast.BoolLiteral:
		return &Boolean{Value: n.Value}

	case *ast.IntLiteral:
		return &Integer{Value: n.Value}

	case *ast.FloatLiteral:
		return &Float{Value: n.Value}

	case *ast.StringLiteral:
		return &String{Value: n.Value}

	case *ast.InterpolatedString:
		var result []byte
		for _, seg := range n.Segments {
			if !seg.IsExpr {
				result = append(result, seg.Text...)
			} else {
				val := Eval(seg.Expr, env)
				if isError(val) {
					return val
				}
				result = append(result, val.Inspect()...)
			}
		}
		return &String{Value: string(result)}

	// ---------------- CALL ----------------
	case *ast.CallExpr:
		return evalCall(n, env)

	// ---------------- PREFIX ----------------
	// Unary operators: ! (logical not).
	case *ast.PrefixExpr:
		val := Eval(n.Right, env)
		if isError(val) {
			return val
		}
		switch n.Operator {
		case "!":
			if !canLogical(val.Type()) {
				return typeMismatchError("!", val.Type(), val.Type(), n.Pos)
			}
			return &Boolean{Value: !val.(*Boolean).Value}
		case "-":
			if !canArithmetic(val.Type()) {
				return typeMismatchError("-", val.Type(), val.Type(), n.Pos)
			}
			if f, ok := val.(*Float); ok {
				return &Float{Value: -f.Value}
			}
			return &Integer{Value: -val.(*Integer).Value}
		}
		return runtimeError("unknown prefix operator: "+n.Operator, n.Pos)

	// ---------------- INFIX ----------------
	// Binary operators. && and || short-circuit and are handled first.
	// All other operators evaluate both sides eagerly.
	case *ast.InfixExpr:
		if n.Operator == "&&" || n.Operator == "||" {
			return evalLogical(n, env)
		}
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(n.Right, env)
		if isError(right) {
			return right
		}

		switch n.Operator {
		case "+":
			if left.Type() == STRING_OBJ && right.Type() == STRING_OBJ {
				return &String{Value: left.(*String).Value + right.(*String).Value}
			}
			if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
				return typeMismatchError("+", left.Type(), right.Type(), n.Pos)
			}
			if left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ {
				return &Integer{Value: left.(*Integer).Value + right.(*Integer).Value}
			}
			return &Float{Value: toFloat64(left) + toFloat64(right)}

		case "-", "*", "/", "%":
			// % is integer-only; the others promote to float when either operand is float.
			if n.Operator == "%" {
				if left.Type() != INTEGER_OBJ || right.Type() != INTEGER_OBJ {
					return typeMismatchError("%", left.Type(), right.Type(), n.Pos)
				}
				if right.(*Integer).Value == 0 {
					return runtimeError("modulo by zero", n.Pos)
				}
				return &Integer{Value: left.(*Integer).Value % right.(*Integer).Value}
			}
			if !canArithmetic(left.Type()) || !canArithmetic(right.Type()) {
				return typeMismatchError(n.Operator, left.Type(), right.Type(), n.Pos)
			}
			bothInt := left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ
			lf, rf := toFloat64(left), toFloat64(right)
			switch n.Operator {
			case "-":
				if bothInt {
					return &Integer{Value: left.(*Integer).Value - right.(*Integer).Value}
				}
				return &Float{Value: lf - rf}
			case "*":
				if bothInt {
					return &Integer{Value: left.(*Integer).Value * right.(*Integer).Value}
				}
				return &Float{Value: lf * rf}
			case "/":
				if bothInt {
					if right.(*Integer).Value == 0 {
						return runtimeError("division by zero", n.Pos)
					}
					return &Integer{Value: left.(*Integer).Value / right.(*Integer).Value}
				}
				if rf == 0 {
					return runtimeError("division by zero", n.Pos)
				}
				return &Float{Value: lf / rf}
			}

		case "==":
			result := evalEquals(left, right, n.Pos)
			if isError(result) {
				return result
			}
			return result

		case "!=":
			result := evalEquals(left, right, n.Pos)
			if isError(result) {
				return result
			}
			return &Boolean{Value: !result.(*Boolean).Value}

		case "<", ">", "<=", ">=":
			return evalNumericCompare(left, right, n.Operator, n.Pos)

		}

		return runtimeError("unknown operator: "+n.Operator, n.Pos)

	// ---------------- PIPE ----------------
	// left |> right — pipes left as the first argument of the right-hand callable.
	// If right is a CallExpr, left is prepended to its argument list.
	// If right is a bare reference, it is called with left as the only argument.
	case *ast.PipeExpr:
		left := Eval(n.Left, env)
		if isError(left) {
			return left
		}

		// Build args: piped value is always first.
		pipeArgs := []Object{left}

		// Determine the callable and any extra arguments from the right side.
		var fnNode ast.Node
		var extraArgs []ast.Node
		if call, ok := n.Right.(*ast.CallExpr); ok {
			fnNode = call.Function
			extraArgs = call.Args
		} else {
			fnNode = n.Right
		}

		for _, argNode := range extraArgs {
			val := Eval(argNode, env)
			if isError(val) {
				return val
			}
			pipeArgs = append(pipeArgs, val)
		}

		fnObj := Eval(fnNode, env)
		if isError(fnObj) {
			return fnObj
		}

		result, errObj := callCallable(fnObj, pipeArgs)
		if errObj != nil {
			return errObj
		}
		return result

	}

	return NULL
}
