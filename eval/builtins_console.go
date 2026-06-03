package eval

import (
	"fmt"
	"klex/ast"
)

func init() {
	// color_red — the ANSI escape code for red foreground text.
	//
	// @sig     color_red() -> string
	// @returns the escape sequence that turns following text red
	// @errors  none
	// @example no-run print(color_red() + "error" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_red"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[31m"}
	}}

	// color_green — the ANSI escape code for green foreground text.
	//
	// @sig     color_green() -> string
	// @returns the escape sequence that turns following text green
	// @errors  none
	// @example no-run print(color_green() + "ok" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_green"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[32m"}
	}}

	// color_blue — the ANSI escape code for blue foreground text.
	//
	// @sig     color_blue() -> string
	// @returns the escape sequence that turns following text blue
	// @errors  none
	// @example no-run print(color_blue() + "info" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_blue"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[34m"}
	}}

	// color_yellow — the ANSI escape code for yellow foreground text.
	//
	// @sig     color_yellow() -> string
	// @returns the escape sequence that turns following text yellow
	// @errors  none
	// @example no-run print(color_yellow() + "warn" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_yellow"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[33m"}
	}}

	// color_magenta — the ANSI escape code for magenta foreground text.
	//
	// @sig     color_magenta() -> string
	// @returns the escape sequence that turns following text magenta
	// @errors  none
	// @example no-run print(color_magenta() + "x" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_magenta"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[35m"}
	}}

	// color_cyan — the ANSI escape code for cyan foreground text.
	//
	// @sig     color_cyan() -> string
	// @returns the escape sequence that turns following text cyan
	// @errors  none
	// @example no-run print(color_cyan() + "x" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_cyan"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[36m"}
	}}

	// color_white — the ANSI escape code for white foreground text.
	//
	// @sig     color_white() -> string
	// @returns the escape sequence that turns following text white
	// @errors  none
	// @example no-run print(color_white() + "x" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_white"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[37m"}
	}}

	// color_black — the ANSI escape code for black foreground text.
	//
	// @sig     color_black() -> string
	// @returns the escape sequence that turns following text black
	// @errors  none
	// @example no-run print(color_bg_white() + color_black() + " x " + color_reset())
	// @since   0.1.0
	// @see     colorize, color_reset
	Builtins["color_black"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[30m"}
	}}

	// color_bold — the ANSI escape code for bold (bright) text.
	//
	// @sig     color_bold() -> string
	// @returns the escape sequence that turns following text bold
	// @errors  none
	// @example no-run print(color_bold() + "title" + color_reset())
	// @since   0.1.0
	// @see     color_dim, color_underline, color_reset
	Builtins["color_bold"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[1m"}
	}}

	// color_dim — the ANSI escape code for dim (faint) text.
	//
	// @sig     color_dim() -> string
	// @returns the escape sequence that turns following text faint
	// @errors  none
	// @example no-run print(color_dim() + "muted" + color_reset())
	// @since   0.1.0
	// @see     color_bold, color_reset
	Builtins["color_dim"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[2m"}
	}}

	// color_underline — the ANSI escape code for underlined text.
	//
	// @sig     color_underline() -> string
	// @returns the escape sequence that underlines following text
	// @errors  none
	// @example no-run print(color_underline() + "link" + color_reset())
	// @since   0.1.0
	// @see     color_bold, color_reset
	Builtins["color_underline"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[4m"}
	}}

	// color_reset — the ANSI escape code that clears all colour/style attributes.
	//
	// Print this after coloured text so later output isn't affected. colorize()
	// appends it for you.
	//
	// @sig     color_reset() -> string
	// @returns the escape sequence that resets all attributes to default
	// @errors  none
	// @example no-run print(color_red() + "error" + color_reset())
	// @since   0.1.0
	// @see     colorize, color_red
	Builtins["color_reset"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[0m"}
	}}

	// color_bg_red — the ANSI escape code for a red background.
	//
	// @sig     color_bg_red() -> string
	// @returns the escape sequence that sets the background to red
	// @errors  none
	// @example no-run print(color_bg_red() + " ! " + color_reset())
	// @since   0.1.0
	// @see     color_red, color_reset
	Builtins["color_bg_red"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[41m"}
	}}

	// color_bg_green — the ANSI escape code for a green background.
	//
	// @sig     color_bg_green() -> string
	// @returns the escape sequence that sets the background to green
	// @errors  none
	// @example no-run print(color_bg_green() + " ok " + color_reset())
	// @since   0.1.0
	// @see     color_green, color_reset
	Builtins["color_bg_green"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[42m"}
	}}

	// color_bg_blue — the ANSI escape code for a blue background.
	//
	// @sig     color_bg_blue() -> string
	// @returns the escape sequence that sets the background to blue
	// @errors  none
	// @example no-run print(color_bg_blue() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_blue, color_reset
	Builtins["color_bg_blue"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[44m"}
	}}

	// color_bg_yellow — the ANSI escape code for a yellow background.
	//
	// @sig     color_bg_yellow() -> string
	// @returns the escape sequence that sets the background to yellow
	// @errors  none
	// @example no-run print(color_bg_yellow() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_yellow, color_reset
	Builtins["color_bg_yellow"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[43m"}
	}}

	// color_bg_magenta — the ANSI escape code for a magenta background.
	//
	// @sig     color_bg_magenta() -> string
	// @returns the escape sequence that sets the background to magenta
	// @errors  none
	// @example no-run print(color_bg_magenta() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_magenta, color_reset
	Builtins["color_bg_magenta"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[45m"}
	}}

	// color_bg_cyan — the ANSI escape code for a cyan background.
	//
	// @sig     color_bg_cyan() -> string
	// @returns the escape sequence that sets the background to cyan
	// @errors  none
	// @example no-run print(color_bg_cyan() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_cyan, color_reset
	Builtins["color_bg_cyan"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[46m"}
	}}

	// color_bg_white — the ANSI escape code for a white background.
	//
	// @sig     color_bg_white() -> string
	// @returns the escape sequence that sets the background to white
	// @errors  none
	// @example no-run print(color_bg_white() + color_black() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_white, color_reset
	Builtins["color_bg_white"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[47m"}
	}}

	// color_bg_black — the ANSI escape code for a black background.
	//
	// @sig     color_bg_black() -> string
	// @returns the escape sequence that sets the background to black
	// @errors  none
	// @example no-run print(color_bg_black() + color_white() + " x " + color_reset())
	// @since   0.1.0
	// @see     color_black, color_reset
	Builtins["color_bg_black"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: "\033[40m"}
	}}

	// colorize — wrap text in a colour code and a reset, ready to print.
	//
	// Prepends colorCode and appends the reset escape so the colour applies to
	// exactly this text and nothing after it. Pair with the color_* builtins.
	//
	// @sig     colorize(text: string, colorCode: string) -> string
	// @param   text       the text to colour
	// @param   colorCode  an ANSI code from a color_* builtin
	// @returns text wrapped in colorCode … reset
	// @errors  TypeError if either argument is not a string
	// @example no-run println(colorize("done", color_green()))
	// @since   0.1.0
	// @see     color_red, color_reset
	Builtins["colorize"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 {
			return runtimeError(fmt.Sprintf("colorize: expected 2 arguments (text, colorCode), got %d — e.g. colorize(\"hello\", color_red())", len(args)), ast.Pos{})
		}

		text, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("colorize: first argument must be string, got %s", args[0].Type()), ast.Pos{})
		}

		colorCode, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("colorize: second argument must be string, got %s", args[1].Type()), ast.Pos{})
		}

		result := fmt.Sprintf("%s%s\033[0m", colorCode.Value, text.Value)
		return &String{Value: result}
	}}
}
