package eval

import (
	"fmt"
	"klex/ast"
	"strings"
)

func init() {
	// format(fmtStr, arg...) — printf-style string formatting.
	//
	// Format verbs:
	//   %d         integer decimal
	//   %f %e %g   float (decimal / scientific / shortest); accepts INTEGER too
	//   %s         string (strict — use %v for any type)
	//   %t         boolean
	//   %v         any value (calls Inspect())
	//   %x %X      integer hex (lower / upper)
	//   %o         integer octal
	//   %b         integer binary
	//   %%         literal percent sign
	//
	// Width, precision, and flags (-+0 space) follow standard printf conventions.
	// Too few or too many arguments is a RuntimeError.
	// Wrong type for a verb is a RuntimeError.
	Builtins["format"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("format expects at least 1 argument", ast.Pos{})
		}
		fmtStr, ok := args[0].(*String)
		if !ok {
			return typeError("format: first argument must be a string", ast.Pos{})
		}
		result, err := applyFormat(fmtStr.Value, args[1:])
		if err != nil {
			return runtimeError(err.Error(), ast.Pos{})
		}
		return &String{Value: result}
	}}
}

// applyFormat walks fmtStr, substitutes each %... specifier with the
// corresponding argument, and returns the assembled string.
func applyFormat(fmtStr string, args []Object) (string, error) {
	var out strings.Builder
	argIdx := 0
	i := 0

	for i < len(fmtStr) {
		if fmtStr[i] != '%' {
			out.WriteByte(fmtStr[i])
			i++
			continue
		}

		specStart := i
		i++ // move past '%'

		if i >= len(fmtStr) {
			return "", fmt.Errorf("format: trailing '%%'")
		}

		// %% → literal percent, no argument consumed
		if fmtStr[i] == '%' {
			out.WriteByte('%')
			i++
			continue
		}

		// Flags: any combination of - + space 0
		for i < len(fmtStr) && isFmtFlag(fmtStr[i]) {
			i++
		}
		// Width
		for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
			i++
		}
		// Precision
		if i < len(fmtStr) && fmtStr[i] == '.' {
			i++
			for i < len(fmtStr) && fmtStr[i] >= '0' && fmtStr[i] <= '9' {
				i++
			}
		}

		if i >= len(fmtStr) {
			return "", fmt.Errorf("format: incomplete format specifier")
		}

		verb := fmtStr[i]
		spec := fmtStr[specStart : i+1] // full specifier including leading '%'
		i++

		if argIdx >= len(args) {
			return "", fmt.Errorf("format: not enough arguments (specifier %q needs argument %d)", spec, argIdx+1)
		}
		arg := args[argIdx]
		argIdx++

		piece, err := formatArg(spec, verb, arg)
		if err != nil {
			return "", err
		}
		out.WriteString(piece)
	}

	if argIdx < len(args) {
		return "", fmt.Errorf("format: %d unused argument(s)", len(args)-argIdx)
	}

	return out.String(), nil
}

// formatArg formats a single argument against its specifier.
func formatArg(spec string, verb byte, arg Object) (string, error) {
	switch verb {

	case 'd':
		n, ok := arg.(*Integer)
		if !ok {
			return "", fmt.Errorf("format: %%d requires INTEGER, got %s", arg.Type())
		}
		return fmt.Sprintf(spec, n.Value), nil

	case 'f', 'e', 'E', 'g', 'G':
		switch v := arg.(type) {
		case *Integer:
			return fmt.Sprintf(spec, float64(v.Value)), nil
		case *Float:
			return fmt.Sprintf(spec, v.Value), nil
		default:
			return "", fmt.Errorf("format: %%%c requires INTEGER or FLOAT, got %s", verb, arg.Type())
		}

	case 's':
		s, ok := arg.(*String)
		if !ok {
			return "", fmt.Errorf("format: %%s requires STRING, got %s (use %%v for any type)", arg.Type())
		}
		return fmt.Sprintf(spec, s.Value), nil

	case 't':
		b, ok := arg.(*Boolean)
		if !ok {
			return "", fmt.Errorf("format: %%t requires BOOLEAN, got %s", arg.Type())
		}
		return fmt.Sprintf(spec, b.Value), nil

	case 'v':
		// %v accepts any type; rewrite verb to %s so fmt.Sprintf handles
		// width and alignment flags correctly against the inspect string.
		sSpec := spec[:len(spec)-1] + "s"
		return fmt.Sprintf(sSpec, arg.Inspect()), nil

	case 'x', 'X':
		n, ok := arg.(*Integer)
		if !ok {
			return "", fmt.Errorf("format: %%%c requires INTEGER, got %s", verb, arg.Type())
		}
		return fmt.Sprintf(spec, n.Value), nil

	case 'o':
		n, ok := arg.(*Integer)
		if !ok {
			return "", fmt.Errorf("format: %%o requires INTEGER, got %s", arg.Type())
		}
		return fmt.Sprintf(spec, n.Value), nil

	case 'b':
		n, ok := arg.(*Integer)
		if !ok {
			return "", fmt.Errorf("format: %%b requires INTEGER, got %s", arg.Type())
		}
		return fmt.Sprintf(spec, n.Value), nil

	default:
		return "", fmt.Errorf("format: unknown verb '%%%c'", verb)
	}
}

func isFmtFlag(c byte) bool {
	return c == '-' || c == '+' || c == ' ' || c == '0'
}
