//go:build windows

package eval

import "klex/ast"

func init() {
	for _, name := range []string{"lineChart", "barChart", "sparkline", "pieChart"} {
		n := name
		Builtins[n] = &Builtin{Fn: func(args []Object) Object {
			return runtimeError(n+": charts not supported on Windows", ast.Pos{})
		}}
	}
}
