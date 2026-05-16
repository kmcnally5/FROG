//go:build windows

package eval

import "klex/ast"

func init() {
	for _, name := range []string{
		"uiBegin", "uiEnd", "uiSetFont", "uiResetFont", "makeTheme", "uiTheme",
		"button", "label", "textInput", "list",
		"checkbox", "slider", "progressBar", "dropdown", "tabs", "textArea",
		"toggle", "radio", "numericStepper", "getTypedChars",
		"table", "accordion", "contextMenu", "colorPicker", "modal", "treeView", "scrollArea",
		"uiBeginRow", "uiRowX", "uiRowY", "uiRowH", "uiRowAdvance",
		"uiBeginCol", "uiColX", "uiColY", "uiColW", "uiColAdvance",
		"toast", "image", "tooltip", "listMulti", "splitter",
	} {
		n := name
		Builtins[n] = &Builtin{Fn: func(args []Object) Object {
			return runtimeError(n+": UI not supported on Windows", ast.Pos{})
		}}
	}
}
