package eval

import (
	"fmt"
	"klex/ast"
)

// objectToStringSlice converts a kLex Array of strings to a Go []string.
// Returns a RuntimeError Object on failure.
//
// Lives in its own file with no build tag so it's available to every
// platform — process spawning (non-js), MCP bridges, and any future
// callers all share this helper.
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
