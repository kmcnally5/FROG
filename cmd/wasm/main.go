//go:build wasm

package main

import (
	"fmt"
	"klex/eval"
	"klex/lexer"
	"klex/parser"
	"strings"
	"syscall/js"
)

var globalEnv *eval.Environment

func init() {
	globalEnv = eval.NewEnv()
	js.Global().Set("klex_eval", js.FuncOf(jsEval))
	js.Global().Set("klex_reset", js.FuncOf(jsReset))
	js.Global().Set("klex_depth", js.FuncOf(jsDepth))
}

func main() {
	c := make(chan struct{})
	<-c
}

func jsEval(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{"output": "", "error": "no input", "isError": true}
	}

	input := strings.TrimSpace(args[0].String())
	if input == "" {
		return map[string]interface{}{"output": "", "error": "", "isError": false}
	}

	// Redirect eval.Output to a strings.Builder — no os.Pipe needed
	var outBuf strings.Builder
	eval.Output = &outBuf

	var parseErrors []string

	l := lexer.New(input)
	p := parser.New(l)
	program := p.ParseProgram()

	for _, e := range program.Errors {
		parseErrors = append(parseErrors, fmt.Sprintf("ParseError: %s", e))
	}

	eval.Eval(program, globalEnv)


	errStr := strings.Join(parseErrors, "\n")
	return map[string]interface{}{
		"output":  outBuf.String(),
		"error":   errStr,
		"isError": len(parseErrors) > 0,
	}
}

func jsReset(this js.Value, args []js.Value) interface{} {
	globalEnv = eval.NewEnv()
	return nil
}

func jsDepth(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return 0
	}
	return depth(args[0].String())
}

func depth(input string) int {
	d := 0
	inString := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if ch == '"' && (i == 0 || input[i-1] != '\\') {
			inString = !inString
		}
		if inString {
			continue
		}
		if ch == '{' || ch == '(' {
			d++
		} else if ch == '}' || ch == ')' {
			d--
		}
	}
	if d < 0 {
		return 0
	}
	return d
}
