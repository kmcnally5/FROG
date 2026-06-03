//go:build js && wasm

package eval

// builtins_eval_wasm.go — playground-specific builtins for the WASM build.
//
// runScript  — evaluates a kLex source string in a fresh Environment and
//              returns a hash {output, error, isError}. Used by the kLex
//              Playground IDE so user scripts run isolated from the
//              playground's own state. The caller owns output capture;
//              eval.Output is temporarily redirected to a strings.Builder
//              for the duration of the call and restored afterwards.
//
// openURL    — calls window.open(url, "_blank") via syscall/js so the
//              playground's "View full docs" button can open a GitHub
//              anchor in a new tab without a full round-trip.
//
// Both builtins are intentionally WASM-only (//go:build js && wasm):
//   - runScript needs the render loop's single-goroutine guarantee to
//     safely redirect the shared Output pointer.
//   - openURL has no meaningful desktop semantics.
//
// vm/builtins_gen_wasm.go (generated, //go:build js && wasm) includes
// these two builtins, keeping the WASM builtin count correct. Regenerate
// with `go generate ./vm/...` after adding or removing WASM-only builtins.

import (
	"encoding/base64"
	"klex/ast"
	"klex/lexer"
	"klex/parser"
	"strings"
	"syscall/js"
)

func init() {
	// runScript — evaluate a kLex source string in isolation (browser playground only).
	//
	// WASM-only: runs `src` as a complete program in a fresh environment, capturing
	// its printed output. Returns a hash with "output" (printed text), "error" (the
	// message, "" on success) and "isError" (bool). Definitions don't persist
	// between calls. Scripts that call window() are rejected — the canvas belongs to
	// the playground IDE. Not available on desktop.
	//
	// @sig     runScript(src: string) -> hash
	// @param   src  the kLex source to evaluate
	// @returns a hash with keys "output", "error", and "isError"
	// @errors  TypeError if src isn't a string; RuntimeError unless given 1 argument
	// @example no-run result = runScript("println(2 + 2)")
	// @since   0.1.0
	// @see     openURL
	Builtins["runScript"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("runScript expects 1 argument (src)", ast.Pos{})
		}
		srcObj, ok := args[0].(*String)
		if !ok {
			return typeError("runScript: src must be string, got "+string(args[0].Type()), ast.Pos{})
		}

		src := srcObj.Value

		var outBuf strings.Builder
		prevOutput := Output
		defer func() { Output = prevOutput }()
		Output = &outBuf

		l := lexer.New(src)
		p := parser.New(l)
		program := p.ParseProgram()

		var errParts []string
		for _, e := range program.Errors {
			errParts = append(errParts, "ParseError: "+e)
		}

		// Reject window() after parsing so comments and string literals
		// containing "window(" don't trigger a false positive.
		if hasWindowCall(program) {
			return makeRunResult("", "window() scripts use the canvas — the playground doesn't support canvas output yet.\nTry a pure-output script using println() instead.", true)
		}

		var result Object
		if len(errParts) == 0 && VMRunScript != nil {
			// VM path: faster for full programs. Falls back to the
			// tree-walker when the compiler hits an unimplemented AST
			// arm (ok=false), keeping incremental rollout safe.
			if vmResult, ok := VMRunScript(program); ok {
				result = vmResult
			} else {
				result = Eval(program, NewEnv())
			}
		} else {
			result = Eval(program, NewEnv())
		}

		if errObj, isErr := result.(*Error); isErr && len(errParts) == 0 {
			errParts = append(errParts, errObj.Message)
		}

		errStr := strings.Join(errParts, "\n")
		return makeRunResult(outBuf.String(), errStr, errStr != "")
	}}

	// openURL — open a URL in a new browser tab (browser playground only).
	//
	// WASM-only: calls window.open(url, "_blank"), used by the playground's
	// documentation links. Not available on desktop.
	//
	// @sig     openURL(url: string) -> null
	// @param   url  the URL to open in a new tab
	// @returns null
	// @errors  TypeError if url isn't a string; RuntimeError unless given 1 argument
	// @example no-run openURL("https://github.com/kmcnally5/FROG")
	// @since   0.1.0
	// @see     runScript
	Builtins["openURL"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("openURL expects 1 argument (url)", ast.Pos{})
		}
		urlObj, ok := args[0].(*String)
		if !ok {
			return typeError("openURL: url must be string, got "+string(args[0].Type()), ast.Pos{})
		}
		js.Global().Call("open", urlObj.Value, "_blank")
		return NULL
	}}

	// _wasmGetHash() → string
	// Returns window.location.hash without the leading '#'.
	Builtins["_wasmGetHash"] = &Builtin{Fn: func(args []Object) Object {
		hash := js.Global().Get("location").Get("hash").String()
		if len(hash) > 0 && hash[0] == '#' {
			hash = hash[1:]
		}
		return &String{Value: hash}
	}}

	// _wasmSetHash(hash: string) → null
	// Sets window.location.hash (without triggering a page navigation).
	Builtins["_wasmSetHash"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_wasmSetHash expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("_wasmSetHash: argument must be string", ast.Pos{})
		}
		js.Global().Get("location").Set("hash", s.Value)
		return NULL
	}}

	// _wasmGetHref() → string
	// Returns window.location.href — the full current page URL including hash.
	Builtins["_wasmGetHref"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: js.Global().Get("location").Get("href").String()}
	}}

	// _wasmCopyToClipboard(text: string) → null
	// Writes text to the clipboard via navigator.clipboard (fire-and-forget).
	Builtins["_wasmCopyToClipboard"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_wasmCopyToClipboard expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("_wasmCopyToClipboard: argument must be string", ast.Pos{})
		}
		clipboard := js.Global().Get("navigator").Get("clipboard")
		if !clipboard.IsUndefined() && !clipboard.IsNull() {
			clipboard.Call("writeText", s.Value)
		}
		return NULL
	}}

	// _wasmBase64Encode(text: string) → string
	// URL-safe base64-encodes text (handles full UTF-8).
	Builtins["_wasmBase64Encode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_wasmBase64Encode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("_wasmBase64Encode: argument must be string", ast.Pos{})
		}
		return &String{Value: base64.URLEncoding.EncodeToString([]byte(s.Value))}
	}}

	// _wasmBase64Decode(encoded: string) → string
	// Decodes a URL-safe base64 string. Returns a RuntimeError on bad input
	// (use safe() in kLex to catch it as a (result, err) pair).
	Builtins["_wasmBase64Decode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("_wasmBase64Decode expects 1 argument", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("_wasmBase64Decode: argument must be string", ast.Pos{})
		}
		decoded, err := base64.URLEncoding.DecodeString(s.Value)
		if err != nil {
			return runtimeError("_wasmBase64Decode: invalid base64: "+err.Error(), ast.Pos{})
		}
		return &String{Value: string(decoded)}
	}}

	// _wasmRequestPaste() → null
	// Reads the clipboard asynchronously and injects the text into the UI
	// paste buffer (gfxState.pasteBuf) so the focused widget picks it up
	// on the next frame — identical to the Cmd+V flow in the keyboard handler.
	Builtins["_wasmRequestPaste"] = &Builtin{Fn: func(args []Object) Object {
		clip := js.Global().Get("navigator").Get("clipboard")
		if clip.IsUndefined() || clip.IsNull() {
			return NULL
		}
		var readFn js.Func
		readFn = js.FuncOf(func(this js.Value, cbArgs []js.Value) interface{} {
			readFn.Release()
			if len(cbArgs) > 0 {
				text := cbArgs[0].String()
				gfxState.mu.Lock()
				gfxState.pasteBuf = text
				gfxState.mu.Unlock()
			}
			return nil
		})
		clip.Call("readText").Call("then", readFn)
		return NULL
	}}

	// _wasmCheckRunFlag() → bool
	// Reads and clears window.__klex_run_requested — set by the Cmd+Enter
	// keydown handler in index.html. Returns true once per key press.
	Builtins["_wasmCheckRunFlag"] = &Builtin{Fn: func(args []Object) Object {
		flag := js.Global().Get("__klex_run_requested")
		if flag.IsUndefined() || !flag.Bool() {
			return FALSE
		}
		js.Global().Set("__klex_run_requested", false)
		return TRUE
	}}
}

// hasWindowCall reports whether prog contains a direct call to window().
// Walks the parsed AST so comments and string literals are not matched.
func hasWindowCall(prog *ast.Program) bool {
	var check func(n ast.Node) bool
	check = func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			if ident, ok := node.Function.(*ast.Ident); ok && ident.Value == "window" {
				return true
			}
			if check(node.Function) {
				return true
			}
			for _, a := range node.Args {
				if check(a) {
					return true
				}
			}
		case *ast.AssignStmt:
			return check(node.Value)
		case *ast.LetStmt:
			return check(node.Value)
		case *ast.ConstStmt:
			return check(node.Value)
		case *ast.ReturnStmt:
			return check(node.Value)
		case *ast.MultiAssignStmt:
			return check(node.Value)
		case *ast.MultiLetStmt:
			return check(node.Value)
		case *ast.InfixExpr:
			return check(node.Left) || check(node.Right)
		case *ast.PrefixExpr:
			return check(node.Right)
		case *ast.UnwrapExpr:
			return check(node.Value)
		case *ast.TupleLiteral:
			for _, e := range node.Elements {
				if check(e) {
					return true
				}
			}
		case *ast.IfStmt:
			if check(node.Condition) {
				return true
			}
			for _, s := range node.Body {
				if check(s) {
					return true
				}
			}
			for _, s := range node.ElseBody {
				if check(s) {
					return true
				}
			}
		case *ast.WhileStmt:
			if check(node.Condition) {
				return true
			}
			for _, s := range node.Body {
				if check(s) {
					return true
				}
			}
		case *ast.ForInStmt:
			if check(node.Collection) {
				return true
			}
			for _, s := range node.Body {
				if check(s) {
					return true
				}
			}
		case *ast.FunctionLiteral:
			for _, s := range node.Body {
				if check(s) {
					return true
				}
			}
		case *ast.SwitchStmt:
			if check(node.Subject) {
				return true
			}
			for _, c := range node.Cases {
				for _, s := range c.Body {
					if check(s) {
						return true
					}
				}
			}
			for _, s := range node.Default {
				if check(s) {
					return true
				}
			}
		}
		return false
	}
	for _, s := range prog.Statements {
		if check(s) {
			return true
		}
	}
	return false
}

// makeRunResult builds the {output, error, isError} hash returned by runScript.
func makeRunResult(output, errStr string, isErr bool) *Hash {
	strPair := func(k, v string) (HashKey, HashPair) {
		kObj := &String{Value: k}
		vObj := &String{Value: v}
		return HashKey{Type: STRING_OBJ, Value: k}, HashPair{Key: kObj, Value: vObj}
	}
	h := &Hash{Pairs: make(map[HashKey]HashPair, 3)}
	hk, hp := strPair("output", output)
	h.Pairs[hk] = hp
	hk, hp = strPair("error", errStr)
	h.Pairs[hk] = hp
	boolVal := Object(FALSE)
	if isErr {
		boolVal = TRUE
	}
	h.Pairs[HashKey{Type: STRING_OBJ, Value: "isError"}] = HashPair{
		Key:   &String{Value: "isError"},
		Value: boolVal,
	}
	return h
}
