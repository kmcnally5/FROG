package eval

import (
	"crypto/rand"
	"fmt"
	"klex/ast"
)

func init() {
	// _uuid() → string  — generates a random UUID v4.
	// Uses crypto/rand for proper randomness, not math/rand.
	// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	Builtins["_uuid"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("_uuid expects no arguments", ast.Pos{})
		}
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return runtimeError(fmt.Sprintf("_uuid: failed to generate random bytes: %s", err.Error()), ast.Pos{})
		}
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // variant bits
		return &String{Value: fmt.Sprintf(
			"%08x-%04x-%04x-%04x-%012x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:],
		)}
	}}
}
