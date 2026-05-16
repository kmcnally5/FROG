//go:build windows

package eval

import "klex/ast"

func init() {
	Builtins["window"] = &Builtin{Fn: func(args []Object) Object {
		return runtimeError("window: graphics not supported on Windows", ast.Pos{})
	}}
	for _, name := range []string{
		"background", "fill", "noFill", "stroke", "noStroke", "strokeWeight", "blendMode",
		"rect", "circle", "line", "triangle", "arc", "gradient",
		"frameCount", "mouseX", "mouseY", "mouseDown", "mouseClicked",
		"mouseRightClicked", "mouseRightDown", "mouseScrollX", "mouseScrollY",
		"pushClip", "popClip", "winWidth", "winHeight",
		"loadFont", "textFont", "textWidth",
		"droppedFiles",
	} {
		n := name
		Builtins[n] = &Builtin{Fn: func(args []Object) Object {
			return runtimeError(n+": graphics not supported on Windows", ast.Pos{})
		}}
	}
}
