package eval

// builtins_ui_widgets.go — Phase 1a UI widgets, shared between the
// desktop (GLFW/OpenGL) and browser (Canvas2D) backends.
//
// Every widget body in this file reads input via uiInput() (backed by
// gfx on desktop and gfxState on WASM), reads/writes UI state on the
// shared `uiCore` global, and draws through the `activeRenderer`
// interface. No reference to gfx, gfxState, gl, glfw, or syscall/js
// appears in this file — that's how it stays portable.
//
// Widgets covered (Phase 1a):
//   - infrastructure : uiNextFieldID, uiGetFocus, uiSetFocus,
//                      uiSetFont, uiResetFont
//   - theme          : makeTheme, uiTheme
//   - layout cursors : uiBeginRow + uiRowX/Y/H/Advance
//                      uiBeginCol + uiColX/Y/W/Advance
//   - widgets        : button, label, checkbox, slider, progressBar,
//                      toggle, radio
//
// uiBegin / uiEnd are NOT registered here — desktop keeps its richer
// versions (tooltip/dropdown/toast/cursor handling) in builtins_ui.go;
// WASM gets a minimal pair in builtins_ui_lifecycle_wasm.go.
//
// uiRegisterElement and uiCheckHover are defined here (not in
// builtins_ui.go) so the WASM uiEnd can call them without duplicating
// the logic per backend.

import (
	"fmt"
	"klex/ast"
	"math"
	"sort"
	"strings"
	"time"
)

// drawFocusRing paints a 2-px (or theme-specified) accent stroke just
// outside (x,y,w,h). Used by focusable widgets (textInput, textArea,
// buttons when keyboard-navigated) to make focus state visible.
func drawFocusRing(x, y, w, h, radius float32) {
	ring := uiCore.style.focusRingPx
	if ring <= 0 {
		ring = 2
	}
	activeRenderer.strokeRoundedRect(x-ring*0.5, y-ring*0.5, w+ring, h+ring, radius+ring*0.5, ring*0.5, uiCore.theme.accent)
}

// drawEmptyState paints centred dim text inside (x,y,w,h) — used by
// list / listMulti / table / treeView when their content array is
// empty so the container doesn't render as a blank panel.
func drawEmptyState(x, y, w, h float32, message string) {
	activeRenderer.drawText(message, int(x+w*0.5), int(y+h*0.5), true, 0.5, uiCore.theme.dimText)
}

// animateColor smooths the colour stored at `key` toward `target` and
// returns the current interpolated value. Frame-rate-INDEPENDENT —
// uses exponential decay scaled to the per-frame delta
// (uiCore.frameDt) so the perceived settle time is consistent across
// 30/60/120 Hz displays. The decay constant `tau` is the half-life;
// 60 ms feels snappy without being instant.
func animateColor(key string, target [4]float32) [4]float32 {
	if uiCore.animColors == nil {
		uiCore.animColors = make(map[string][4]float32)
	}
	cur, ok := uiCore.animColors[key]
	if !ok {
		uiCore.animColors[key] = target
		return target
	}
	const tau = 0.06 // 60 ms half-life
	dt := uiCore.frameDt
	if dt <= 0 {
		dt = 1.0 / 60.0 // fallback for the very first frame
	}
	k := float32(1.0 - math.Exp(-dt/tau))
	for i := 0; i < 4; i++ {
		cur[i] += (target[i] - cur[i]) * k
	}
	uiCore.animColors[key] = cur
	return cur
}

// uiDisabled returns true when the topmost pushDisabled(true) frame
// is active. Widgets check this to suppress hover/click and render
// at half opacity.
func uiDisabled() bool {
	n := len(uiCore.disabledStack)
	return n > 0 && uiCore.disabledStack[n-1]
}

// fadeIfDisabled returns the colour at half alpha when the disabled
// stack is active, otherwise the colour unchanged.
func fadeIfDisabled(c [4]float32) [4]float32 {
	if uiDisabled() {
		c[3] *= 0.45
	}
	return c
}

// ─── element registry helpers ────────────────────────────────────────────────

// uiRegisterElement stamps a widget's bounds into uiCore.elements so
// uiCheckHover can find it at end-of-frame. Lazy-initialises the map
// if a widget fires before uiBegin had a chance to (rare but possible
// with custom render flows).
func uiRegisterElement(id string, x, y, w, h float32) {
	if uiCore.elements == nil {
		uiCore.elements = make(map[string][4]float32)
	}
	uiCore.elements[id] = [4]float32{x, y, w, h}
	uiCore.elementOrder = append(uiCore.elementOrder, id)
	uiCore.lastElementID = id
}

// uiCheckHover returns the ID of the registered element under (mx,my)
// or "" if nothing is hovered. Takes float64 to match the desktop
// mouse-coordinate type (gfx.mouseX is float64). Iterates elementOrder
// in reverse so the last-registered element wins when widgets overlap —
// popup/overlay elements (dropdown menus, tooltips) are always
// registered after the content they cover, so they take priority.
func uiCheckHover(mx, my float64) string {
	fmx, fmy := float32(mx), float32(my)
	for i := len(uiCore.elementOrder) - 1; i >= 0; i-- {
		id := uiCore.elementOrder[i]
		if b, ok := uiCore.elements[id]; ok {
			if fmx >= b[0] && fmx <= b[0]+b[2] && fmy >= b[1] && fmy <= b[1]+b[3] {
				return id
			}
		}
	}
	return ""
}

// ─── helpers used by widget bodies ───────────────────────────────────────────

// extractScale parses an optional [size] argument used by every widget
// for text scaling. Returns (scale, errOrNil). The default is 0.5 to
// match the desktop contract (8px effective on the 16px monospace
// atlas; the wasm renderer interprets scale via its own base size).
func extractScale(name string, args []Object, idx int, dflt float32) (float32, Object) {
	if len(args) <= idx {
		return dflt, nil
	}
	switch v := args[idx].(type) {
	case *Integer:
		return float32(v.Value) / 100.0, nil
	case *Float:
		return float32(v.Value), nil
	default:
		return 0, typeError(name+": size must be a number", ast.Pos{})
	}
}

// requireIntArgs decodes a known-arity slice of *Integer args.
// Returns (values, errOrNil). Used so widget headers don't repeat the
// same five-way type-assertion boilerplate.
func requireIntArgs(name, msg string, args []Object) ([]int, Object) {
	out := make([]int, len(args))
	for i, a := range args {
		v, ok := a.(*Integer)
		if !ok {
			return nil, typeError(name+": "+msg, ast.Pos{})
		}
		out[i] = v.Value
	}
	return out, nil
}

// requireFloatArgs decodes a known-arity slice of numeric args, accepting
// either *Integer or *Float (a literal 10 and a computed 10.5 are both fine).
// Used by the layout-cursor builtins, whose documented signatures are `: float`
// and whose sibling advance functions (uiRowAdvance/uiColAdvance) already accept
// both. Returns the values as float32 ready for the float-native graphics layer.
func requireFloatArgs(name, msg string, args []Object) ([]float32, Object) {
	out := make([]float32, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case *Integer:
			out[i] = float32(v.Value)
		case *Float:
			out[i] = float32(v.Value)
		default:
			return nil, typeError(name+": "+msg, ast.Pos{})
		}
	}
	return out, nil
}

// rgbaFromArray converts a kLex [r,g,b,a] *Array into [4]float32.
// Accepts Integer or Float per slot.
func rgbaFromArray(a *Array) ([4]float32, bool) {
	if len(a.Elements) != 4 {
		return [4]float32{}, false
	}
	var c [4]float32
	for i, el := range a.Elements {
		switch n := el.(type) {
		case *Float:
			c[i] = float32(n.Value)
		case *Integer:
			c[i] = float32(n.Value)
		default:
			return [4]float32{}, false
		}
	}
	return c, true
}

// paletteToArray returns the current uiCore.theme as a 14-slot kLex
// array. Slot order matches the documented uiPalette layout.
func paletteToArray(p uiPalette) *Array {
	slots := [14][4]float32{
		p.widgetBg, p.widgetBgHover, p.widgetBgActive,
		p.widgetText, p.labelText, p.dimText,
		p.accent, p.accentBg,
		p.track, p.trackFill, p.handle,
		p.inputBg, p.inputFocusBg,
		p.shadow,
	}
	out := make([]Object, 14)
	for i, c := range slots {
		out[i] = &Array{Elements: []Object{
			&Float{Value: float64(c[0])},
			&Float{Value: float64(c[1])},
			&Float{Value: float64(c[2])},
			&Float{Value: float64(c[3])},
		}}
	}
	return &Array{Elements: out}
}

// arrayToPalette reads a 14-slot kLex array back into a uiPalette. The
// caller is responsible for length validation.
func arrayToPalette(a *Array) (uiPalette, bool) {
	read := func(i int) ([4]float32, bool) {
		el, ok := a.Elements[i].(*Array)
		if !ok {
			return [4]float32{}, false
		}
		return rgbaFromArray(el)
	}
	var p uiPalette
	slots := [14]*[4]float32{
		&p.widgetBg, &p.widgetBgHover, &p.widgetBgActive,
		&p.widgetText, &p.labelText, &p.dimText,
		&p.accent, &p.accentBg,
		&p.track, &p.trackFill, &p.handle,
		&p.inputBg, &p.inputFocusBg,
		&p.shadow,
	}
	for i, dst := range slots {
		c, ok := read(i)
		if !ok {
			return uiPalette{}, false
		}
		*dst = c
	}
	return p, true
}

// ─── init: register Phase 1a builtins ────────────────────────────────────────

func init() {
	// makeTheme — get the default UI palette as a 14-slot array to customise.
	//
	// Returns the 14 [r, g, b, a] colour slots the widgets read, so you can tweak
	// individual slots and apply them with uiTheme. Slot order: 0 widgetBg,
	// 1 widgetBgHover, 2 widgetBgActive, 3 widgetText, 4 labelText, 5 dimText,
	// 6 accent, 7 accentBg, 8 track, 9 trackFill, 10 handle, 11 inputBg,
	// 12 inputFocusBg, 13 shadow.
	//
	// @sig     makeTheme() -> array
	// @returns a 14-element array of [r, g, b, a] colour arrays (0.0–1.0)
	// @errors  RuntimeError if called with any arguments
	// @example no-run t = makeTheme()
	// @since   0.1.0
	// @see     uiTheme, setTheme
	Builtins["makeTheme"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("makeTheme expects no arguments", ast.Pos{})
		}
		return paletteToArray(defaultUIPalette())
	}}

	// pushDisabled — push a disabled state so following widgets are greyed out.
	//
	// While the top of the stack is true, interactive widgets drawn after it
	// render at half opacity and ignore hover/click. The stack lets nested forms
	// re-enable sections (push false inside). Always pair with popDisabled.
	//
	// @sig     pushDisabled(disabled: bool) -> null
	// @param   disabled  true to disable following widgets, false to re-enable
	// @returns null
	// @errors  TypeError unless given a single boolean argument
	// @example no-run pushDisabled(true)
	// @since   0.1.0
	// @see     popDisabled
	Builtins["pushDisabled"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("pushDisabled expects 1 argument: bool", ast.Pos{})
		}
		b, ok := args[0].(*Boolean)
		if !ok {
			return typeError("pushDisabled: argument must be a bool", ast.Pos{})
		}
		uiCore.disabledStack = append(uiCore.disabledStack, b.Value)
		return NULL
	}}
	// popDisabled — pop the most recent pushDisabled state.
	//
	// Restores the disabled state to what it was before the matching pushDisabled.
	// Safe to call on an empty stack (no-op).
	//
	// @sig     popDisabled() -> null
	// @returns null
	// @errors  none
	// @example no-run popDisabled()
	// @since   0.1.0
	// @see     pushDisabled
	Builtins["popDisabled"] = &Builtin{Fn: func(args []Object) Object {
		if n := len(uiCore.disabledStack); n > 0 {
			uiCore.disabledStack = uiCore.disabledStack[:n-1]
		}
		return NULL
	}}

	// setTheme — install a named preset theme (colours + style tokens).
	//
	// Sets both the colour palette and the style tokens (radii, spacing, font
	// base) in one call. Presets: "nebula" (deep violet + ice cyan, the original
	// look), "light" (clean light, blue accent), "dark" (modern dark, blue
	// accent), "highContrast" (black/white + yellow, for accessibility). Use
	// uiTheme for fine-grained colour overrides on top.
	//
	// @sig     setTheme(name: string) -> null
	// @param   name  "nebula", "light", "dark", or "highContrast"
	// @returns null
	// @errors  TypeError if name isn't a string; RuntimeError if the preset name is unknown
	// @example no-run setTheme("dark")
	// @since   0.1.0
	// @see     uiTheme, makeTheme
	Builtins["setTheme"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("setTheme expects 1 argument: name (string)", ast.Pos{})
		}
		nameObj, ok := args[0].(*String)
		if !ok {
			return typeError("setTheme: name must be a string", ast.Pos{})
		}
		palette, style, ok := themePreset(nameObj.Value)
		if !ok {
			return runtimeError("setTheme: unknown preset "+nameObj.Value+" (try \"nebula\", \"light\", \"dark\", \"highContrast\")", ast.Pos{})
		}
		uiCore.theme = palette
		uiCore.style = style
		return NULL
	}}

	// uiTheme — apply a 14-slot colour palette to the widgets.
	//
	// Takes a palette from makeTheme (optionally with slots tweaked) and makes it
	// the active widget colour scheme. For a complete preset including style
	// tokens, use setTheme instead.
	//
	// @sig     uiTheme(palette: array) -> null
	// @param   palette  a 14-element array of [r, g, b, a] arrays, as from makeTheme
	// @returns null
	// @errors  TypeError unless given a 14-element array of 4-number colour arrays
	// @example no-run uiTheme(t)
	// @since   0.1.0
	// @see     makeTheme, setTheme
	Builtins["uiTheme"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiTheme expects 1 argument: palette array from makeTheme()", ast.Pos{})
		}
		a, ok := args[0].(*Array)
		if !ok || len(a.Elements) != 14 {
			return typeError("uiTheme: argument must be a 14-element array from makeTheme()", ast.Pos{})
		}
		p, ok := arrayToPalette(a)
		if !ok {
			return typeError("uiTheme: every slot must be a 4-element [r,g,b,a] array of numbers", ast.Pos{})
		}
		uiCore.theme = p
		return NULL
	}}

	// uiNextFieldID — the ID the next textInput/textArea will be assigned.
	//
	// Call it immediately before a text field to capture that field's ID, then
	// pass the ID to uiSetFocus to implement Tab navigation between fields.
	//
	// @sig     uiNextFieldID() -> string
	// @returns the ID the next text field will receive this frame
	// @errors  none
	// @example no-run id = uiNextFieldID()
	// @since   0.1.0
	// @see     uiGetFocus, uiSetFocus, textInput
	Builtins["uiNextFieldID"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: fmt.Sprintf("txt_%d", uiCore.nextID)}
	}}

	// uiGetFocus — the ID of the currently focused widget, or "" if none.
	//
	// @sig     uiGetFocus() -> string
	// @returns the focused widget's ID, or "" when nothing is focused
	// @errors  none
	// @example no-run focused = uiGetFocus()
	// @since   0.1.0
	// @see     uiSetFocus, uiNextFieldID
	Builtins["uiGetFocus"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: uiCore.activeID}
	}}

	// uiSetFocus — move keyboard focus to the widget with the given ID.
	//
	// Combine with uiNextFieldID to drive Tab navigation between text fields.
	//
	// @sig     uiSetFocus(id: string) -> null
	// @param   id  the widget ID to focus (from uiNextFieldID)
	// @returns null
	// @errors  TypeError unless given a single string argument
	// @example no-run uiSetFocus(id)
	// @since   0.1.0
	// @see     uiGetFocus, uiNextFieldID
	Builtins["uiSetFocus"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiSetFocus expects 1 argument: id (string)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError("uiSetFocus: id must be a string", ast.Pos{})
		}
		uiCore.activeID = s.Value
		return NULL
	}}

	// uiSetFont — use a loaded font for all subsequent widget text.
	//
	// Pass a font from loadFont; button labels, tabs, list rows etc. then render
	// with it. Call once per frame (or once before window). Revert with
	// uiResetFont. (On the browser/WASM backend the renderer always uses its CSS
	// font, so this is currently a no-op there.)
	//
	// @sig     uiSetFont(font: Font) -> null
	// @param   font  a font handle from loadFont
	// @returns null
	// @errors  TypeError unless given a single Font argument
	// @example no-run uiSetFont(font)
	// @since   0.1.0
	// @see     uiResetFont, loadFont
	Builtins["uiSetFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiSetFont expects 1 argument: font (from loadFont)", ast.Pos{})
		}
		f, ok := args[0].(*Font)
		if !ok {
			return typeError("uiSetFont: argument must be a Font (loadFont result)", ast.Pos{})
		}
		uiCore.activeFont = f
		return NULL
	}}
	// uiResetFont — revert widget text to the built-in monospace font.
	//
	// Undoes uiSetFont, so following widgets use the embedded default font again.
	//
	// @sig     uiResetFont() -> null
	// @returns null
	// @errors  none
	// @example no-run uiResetFont()
	// @since   0.1.0
	// @see     uiSetFont
	Builtins["uiResetFont"] = &Builtin{Fn: func(args []Object) Object {
		uiCore.activeFont = nil
		return NULL
	}}

	// ── Layout cursors ───────────────────────────────────────────────

	// uiBeginRow — start a left-to-right layout row at (x, y).
	//
	// Sets up a cursor: widgets read uiRowX/uiRowY/uiRowH for their position and
	// height, and uiRowAdvance steps the cursor right by a width plus `gap`. Lets
	// you lay out a toolbar without hand-computing x offsets.
	//
	// @sig     uiBeginRow(x: number, y: number, h: number, gap: number) -> null
	// @param   x    starting x in pixels
	// @param   y    row y in pixels
	// @param   h    row height in pixels
	// @param   gap  pixels inserted between items by uiRowAdvance
	// @returns null
	// @errors  TypeError unless given 4 numeric arguments
	// @example no-run uiBeginRow(10.0, 10.0, 30.0, 8.0)
	// @since   0.1.0
	// @see     uiRowX, uiRowAdvance, uiBeginCol
	Builtins["uiBeginRow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return typeError("uiBeginRow expects 4 arguments: x, y, h, gap", ast.Pos{})
		}
		// Accept int OR float — matches the documented `: float` signature and
		// the sibling uiRowAdvance. Previously this required ints, which silently
		// aborted render loops when a caller passed computed float coords (the
		// SecretHunter post-scan freeze, found via --record-tape 2026-05-31).
		vals, err := requireFloatArgs("uiBeginRow", "x, y, h, gap must be numbers", args)
		if err != nil {
			return err
		}
		uiCore.rowCurX = vals[0]
		uiCore.rowY = vals[1]
		uiCore.rowH = vals[2]
		uiCore.rowGap = vals[3]
		return NULL
	}}
	// uiRowX — the current x of the active row cursor (see uiBeginRow).
	//
	// @sig     uiRowX() -> int
	// @returns the row cursor's current x in pixels
	// @errors  none
	// @example no-run button("ok", uiRowX(), uiRowY(), 60, uiRowH())
	// @since   0.1.0
	// @see     uiBeginRow, uiRowAdvance
	Builtins["uiRowX"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.rowCurX)} }}
	// uiRowY — the y of the active row (see uiBeginRow).
	//
	// @sig     uiRowY() -> int
	// @returns the row's y in pixels
	// @errors  none
	// @example no-run y = uiRowY()
	// @since   0.1.0
	// @see     uiBeginRow, uiRowX
	Builtins["uiRowY"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.rowY)} }}
	// uiRowH — the height of the active row (see uiBeginRow).
	//
	// @sig     uiRowH() -> int
	// @returns the row's height in pixels
	// @errors  none
	// @example no-run h = uiRowH()
	// @since   0.1.0
	// @see     uiBeginRow, uiRowX
	Builtins["uiRowH"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.rowH)} }}
	// uiRowAdvance — move the row cursor right by width plus the row gap.
	//
	// Call after placing an item to position the next one in the row.
	//
	// @sig     uiRowAdvance(width: number) -> null
	// @param   width  the width of the item just placed, in pixels
	// @returns null
	// @errors  TypeError unless given a single numeric argument
	// @example no-run uiRowAdvance(60.0)
	// @since   0.1.0
	// @see     uiBeginRow, uiRowX
	Builtins["uiRowAdvance"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiRowAdvance expects 1 argument: width", ast.Pos{})
		}
		var w float32
		switch v := args[0].(type) {
		case *Integer:
			w = float32(v.Value)
		case *Float:
			w = float32(v.Value)
		default:
			return typeError("uiRowAdvance: argument must be a number", ast.Pos{})
		}
		uiCore.rowCurX += w + uiCore.rowGap
		return NULL
	}}

	// uiBeginCol — start a top-to-bottom layout column at (x, y).
	//
	// The vertical counterpart to uiBeginRow: widgets read uiColX/uiColY/uiColW,
	// and uiColAdvance steps the cursor down by a height plus `gap`. Lets you
	// stack a form without hand-computing y offsets.
	//
	// @sig     uiBeginCol(x: number, y: number, w: number, gap: number) -> null
	// @param   x    column x in pixels
	// @param   y    starting y in pixels
	// @param   w    column width in pixels
	// @param   gap  pixels inserted between items by uiColAdvance
	// @returns null
	// @errors  TypeError unless given 4 numeric arguments
	// @example no-run uiBeginCol(10.0, 10.0, 200.0, 8.0)
	// @since   0.1.0
	// @see     uiColX, uiColAdvance, uiBeginRow
	Builtins["uiBeginCol"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return typeError("uiBeginCol expects 4 arguments: x, y, w, gap", ast.Pos{})
		}
		// Accept int OR float — matches the documented `: float` signature and
		// the sibling uiColAdvance (see uiBeginRow note above).
		vals, err := requireFloatArgs("uiBeginCol", "x, y, w, gap must be numbers", args)
		if err != nil {
			return err
		}
		uiCore.colX = vals[0]
		uiCore.colCurY = vals[1]
		uiCore.colW = vals[2]
		uiCore.colGap = vals[3]
		return NULL
	}}
	// uiColX — the x of the active column (see uiBeginCol).
	//
	// @sig     uiColX() -> int
	// @returns the column's x in pixels
	// @errors  none
	// @example no-run x = uiColX()
	// @since   0.1.0
	// @see     uiBeginCol, uiColAdvance
	Builtins["uiColX"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.colX)} }}
	// uiColY — the current y of the active column cursor (see uiBeginCol).
	//
	// @sig     uiColY() -> int
	// @returns the column cursor's current y in pixels
	// @errors  none
	// @example no-run label("name", uiColX(), uiColY())
	// @since   0.1.0
	// @see     uiBeginCol, uiColAdvance
	Builtins["uiColY"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.colCurY)} }}
	// uiColW — the width of the active column (see uiBeginCol).
	//
	// @sig     uiColW() -> int
	// @returns the column's width in pixels
	// @errors  none
	// @example no-run w = uiColW()
	// @since   0.1.0
	// @see     uiBeginCol, uiColX
	Builtins["uiColW"] = &Builtin{Fn: func(args []Object) Object { return &Integer{Value: int(uiCore.colW)} }}
	// uiColAdvance — move the column cursor down by height plus the column gap.
	//
	// Call after placing an item to position the next one below it.
	//
	// @sig     uiColAdvance(height: number) -> null
	// @param   height  the height of the item just placed, in pixels
	// @returns null
	// @errors  TypeError unless given a single numeric argument
	// @example no-run uiColAdvance(30.0)
	// @since   0.1.0
	// @see     uiBeginCol, uiColY
	Builtins["uiColAdvance"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiColAdvance expects 1 argument: height", ast.Pos{})
		}
		var h float32
		switch v := args[0].(type) {
		case *Integer:
			h = float32(v.Value)
		case *Float:
			h = float32(v.Value)
		default:
			return typeError("uiColAdvance: argument must be a number", ast.Pos{})
		}
		uiCore.colCurY += h + uiCore.colGap
		return NULL
	}}

	// button — draw a clickable button, returning true the frame it's clicked.
	//
	// An immediate-mode widget: call it every frame inside the draw loop and act
	// on the returned bool. The box auto-grows to fit the label, and hover/press
	// visuals follow the active theme. The optional size scales the label text.
	//
	// @sig     button(label: string, x: int, y: int, w: int, h: int, [size: number]) -> bool
	// @param   label  the button caption
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   w      minimum width in pixels (grows to fit the label)
	// @param   h      height in pixels
	// @param   size   label text scale (default 0.5)
	// @returns true only on the frame the button is clicked, false otherwise
	// @errors  TypeError if label isn't a string, x/y/w/h aren't integers, or size isn't numeric; RuntimeError unless given 5 or 6 arguments
	// @example no-run if button("Save", 20, 20, 80, 30) { save() }
	// @since   0.1.0
	// @see     label, checkbox, toggle
	Builtins["button"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("button expects 5-6 arguments: label, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok := args[0].(*String)
		if !ok {
			return typeError("button: label must be a string", ast.Pos{})
		}
		coords, err := requireIntArgs("button", "x/y/w/h must be integers", args[1:5])
		if err != nil {
			return err
		}
		scale, err := extractScale("button", args, 5, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("btn_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])

		// Auto-grow to fit the label so widget sizing is font-agnostic.
		const labelPad = float32(12)
		minW := activeRenderer.textWidth(label.Value, scale) + labelPad*2
		if fw < minW {
			fw = minW
		}

		disabled := uiDisabled()
		in := uiInput()
		hovered := uiCore.hoveredID == id && !disabled
		active := uiCore.activeID == id && in.mouseDown && !disabled
		clicked := false
		if hovered && in.mouseClicked {
			uiCore.activeID = id
			clicked = true
		}

		target := uiCore.theme.widgetBg
		if hovered {
			target = uiCore.theme.widgetBgHover
		}
		if active {
			target = uiCore.theme.widgetBgActive
		}
		bg := animateColor(id+"::bg", target)
		r := uiCore.style.radiusMedium

		// Soft shadow below — heavier on hover, suppressed when pressed
		// (the button visually "sinks in"). Skipped when disabled.
		// Dark themes need much higher alpha + blur than light themes;
		// the per-theme `shadow` slot already accounts for that — this
		// just multiplies for state.
		if !disabled {
			shadow := uiCore.theme.shadow
			shadowAlpha := float32(0.85)
			offsetY := float32(4)
			blur := float32(14)
			switch {
			case active:
				shadowAlpha = 0.40
				offsetY = 2
				blur = 6
			case hovered:
				shadowAlpha = 1.10 // boosted past 1.0 — clamps in cssColor
				if shadowAlpha > 1 {
					shadowAlpha = 1
				}
				offsetY = 7
				blur = 22
			}
			shadow[3] *= shadowAlpha
			activeRenderer.dropShadow(fx, fy, fw, fh, r, offsetY, blur, shadow)
		}

		activeRenderer.fillRoundedRect(fx, fy, fw, fh, r, fadeIfDisabled(bg))
		// Subtle 1px inner outline at low alpha gives the button "edge"
		// even in flat themes — improves the perceived elevation.
		edge := uiCore.theme.widgetBgActive
		edge[3] = 0.4
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, r, 0.5, fadeIfDisabled(edge))
		activeRenderer.drawText(label.Value, int(fx+fw*0.5), int(fy+fh*0.5), true, scale, fadeIfDisabled(uiCore.theme.widgetText))
		uiRegisterElement(id, fx, fy, fw, fh)
		if clicked && UiEventHookActive() {
			FireUiEventHook("click", "button", label.Value, NULL, int(in.mouseX), int(in.mouseY))
		}
		return &Boolean{Value: clicked}
	}}

	// label — draw a line of static text at (x, y).
	//
	// Non-interactive caption text in the theme's label colour. The optional size
	// scales the text.
	//
	// @sig     label(text: string, x: int, y: int, [size: number]) -> null
	// @param   text  the text to draw
	// @param   x     left position in pixels
	// @param   y     top position in pixels
	// @param   size  text scale (default 0.5)
	// @returns null
	// @errors  TypeError if text isn't a string, x/y aren't integers, or size isn't numeric; RuntimeError unless given 3 or 4 arguments
	// @example no-run label("Settings", 20, 20)
	// @since   0.1.0
	// @see     button, text
	Builtins["label"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return typeError("label expects 3-4 arguments: text, x, y, [size]", ast.Pos{})
		}
		text, ok := args[0].(*String)
		if !ok {
			return typeError("label: text must be a string", ast.Pos{})
		}
		coords, err := requireIntArgs("label", "x/y must be integers", args[1:3])
		if err != nil {
			return err
		}
		scale, err := extractScale("label", args, 3, 0.5)
		if err != nil {
			return err
		}
		activeRenderer.drawText(text.Value, coords[0], coords[1], false, scale, uiCore.theme.labelText)
		return NULL
	}}

	// checkbox — draw a labelled checkbox, returning its new checked state.
	//
	// Immediate-mode: pass the current checked state in and store the value it
	// returns — it flips on the frame the box is clicked, otherwise returns the
	// state unchanged.
	//
	// @sig     checkbox(label: string, x: int, y: int, checked: bool, [size: number]) -> bool
	// @param   label    the text beside the box
	// @param   x        left position in pixels
	// @param   y        top position in pixels
	// @param   checked  the current checked state
	// @param   size     label text scale (default 0.5)
	// @returns the checked state after this frame (toggled if clicked)
	// @errors  TypeError on wrong argument types; RuntimeError unless given 4 or 5 arguments
	// @example no-run showGrid = checkbox("Show grid", 20, 60, showGrid)
	// @since   0.1.0
	// @see     toggle, radio, button
	Builtins["checkbox"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return typeError("checkbox expects 4-5 arguments: label, x, y, checked, [size]", ast.Pos{})
		}
		label, ok := args[0].(*String)
		if !ok {
			return typeError("checkbox: label must be a string", ast.Pos{})
		}
		coords, err := requireIntArgs("checkbox", "x/y must be integers", args[1:3])
		if err != nil {
			return err
		}
		checkedArg, ok := args[3].(*Boolean)
		if !ok {
			return typeError("checkbox: checked must be a boolean", ast.Pos{})
		}
		scale, err := extractScale("checkbox", args, 4, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("chk_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		const boxSize = float32(18)
		labelPad := float32(8)
		labelW := activeRenderer.textWidth(label.Value, scale)
		totalW := boxSize + labelPad + labelW
		totalH := boxSize

		disabled := uiDisabled()
		in := uiInput()
		hovered := uiCore.hoveredID == id && !disabled
		checked := checkedArg.Value
		changed := false
		if hovered && in.mouseClicked {
			checked = !checked
			changed = true
			uiCore.activeID = id
		}

		boxTarget := uiCore.theme.inputBg
		if hovered {
			boxTarget = uiCore.theme.widgetBgHover
		}
		boxBg := animateColor(id+"::bg", boxTarget)
		boxR := uiCore.style.radiusSmall
		activeRenderer.fillRoundedRect(fx, fy, boxSize, boxSize, boxR, fadeIfDisabled(boxBg))
		activeRenderer.strokeRoundedRect(fx, fy, boxSize, boxSize, boxR, 0.5, fadeIfDisabled(uiCore.theme.dimText))
		if checked {
			// Filled inner mark.
			inset := float32(4)
			activeRenderer.fillRoundedRect(fx+inset, fy+inset, boxSize-inset*2, boxSize-inset*2, boxR*0.5, fadeIfDisabled(uiCore.theme.accent))
		}
		activeRenderer.drawText(label.Value, int(fx+boxSize+labelPad), int(fy+boxSize*0.5), false, scale, fadeIfDisabled(uiCore.theme.labelText))
		uiRegisterElement(id, fx, fy, totalW, totalH)
		if changed && UiEventHookActive() {
			FireUiEventHook("toggle", "checkbox", label.Value, &Boolean{Value: checked}, int(in.mouseX), int(in.mouseY))
		}
		return &Boolean{Value: checked}
	}}

	// slider — draw a draggable slider, returning its current value.
	//
	// Immediate-mode: pass the current value and store what it returns — while the
	// handle is dragged the value tracks the mouse between min and max. The label
	// is drawn above the track.
	//
	// @sig     slider(label: string, x: int, y: int, w: int, value: number, min: number, max: number, [size: number]) -> float
	// @param   label  caption drawn above the track ("" for none)
	// @param   x      left position in pixels
	// @param   y      top position in pixels (label sits here, track below)
	// @param   w      track width in pixels
	// @param   value  the current value
	// @param   min    minimum value (left end)
	// @param   max    maximum value (right end; must be > min)
	// @param   size   label text scale (default 0.5)
	// @returns the value after this frame (follows the handle while dragged)
	// @errors  TypeError on wrong argument types; RuntimeError if max <= min or given the wrong argument count
	// @example no-run volume = slider("Volume", 20, 60, 200, volume, 0.0, 1.0)
	// @since   0.1.0
	// @see     numericStepper, progressBar
	Builtins["slider"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("slider expects 7-8 arguments: label, x, y, w, value, min, max, [size]", ast.Pos{})
		}
		_, ok := args[0].(*String) // label rendered above the track
		if !ok {
			return typeError("slider: label must be a string", ast.Pos{})
		}
		label := args[0].(*String).Value
		coords, err := requireIntArgs("slider", "x/y/w must be integers", args[1:4])
		if err != nil {
			return err
		}
		readF := func(o Object) (float64, bool) {
			switch v := o.(type) {
			case *Float:
				return v.Value, true
			case *Integer:
				return float64(v.Value), true
			}
			return 0, false
		}
		value, ok1 := readF(args[4])
		minV, ok2 := readF(args[5])
		maxV, ok3 := readF(args[6])
		if !ok1 || !ok2 || !ok3 {
			return typeError("slider: value/min/max must be numbers", ast.Pos{})
		}
		if maxV <= minV {
			return runtimeError("slider: max must be greater than min", ast.Pos{})
		}
		scale, err := extractScale("slider", args, 7, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("sld_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw := float32(coords[2])
		const trackH = float32(6)
		const handleR = float32(8)
		trackY := fy + 20 // label sits above the track
		hitY := trackY - handleR
		hitH := trackH + handleR*2

		// Draw label.
		if label != "" {
			activeRenderer.drawText(label, int(fx), int(fy), false, scale, uiCore.theme.labelText)
		}

		disabled := uiDisabled()
		in := uiInput()
		hovered := uiCore.hoveredID == id && !disabled
		if hovered && in.mouseClicked {
			uiCore.activeID = id
		}
		originalValue := value
		if !disabled && uiCore.activeID == id && in.mouseDown {
			t := float64(in.mouseX-fx) / float64(fw)
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			value = minV + t*(maxV-minV)
		}
		if !in.mouseDown && uiCore.activeID == id {
			uiCore.activeID = ""
		}

		// Track + fill.
		t := (value - minV) / (maxV - minV)
		activeRenderer.fillRoundedRect(fx, trackY, fw, trackH, trackH*0.5, fadeIfDisabled(uiCore.theme.track))
		activeRenderer.fillRoundedRect(fx, trackY, fw*float32(t), trackH, trackH*0.5, fadeIfDisabled(uiCore.theme.accent))
		// Handle — outline + drop pseudo-shadow via faint dark backing.
		hx := fx + fw*float32(t) - handleR
		hy := trackY + trackH*0.5 - handleR
		activeRenderer.fillRoundedRect(hx, hy, handleR*2, handleR*2, handleR, fadeIfDisabled(uiCore.theme.handle))
		activeRenderer.strokeRoundedRect(hx, hy, handleR*2, handleR*2, handleR, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))

		uiRegisterElement(id, fx, hitY, fw, hitH)
		if value != originalValue && UiEventHookActive() {
			FireUiEventHook("drag", "slider", label, &Float{Value: value}, int(in.mouseX), int(in.mouseY))
		}
		return &Float{Value: value}
	}}

	// progressBar — draw a horizontal progress bar (display-only).
	//
	// Fills the track from the left in the accent colour by where `value` sits
	// between min and max (clamped to that range). Non-interactive — for an
	// adjustable control use slider.
	//
	// @sig     progressBar(x: int, y: int, w: int, h: int, value: number, min: number, max: number) -> null
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   w      width in pixels
	// @param   h      height in pixels
	// @param   value  the current value
	// @param   min    value mapped to an empty bar
	// @param   max    value mapped to a full bar (must be > min)
	// @returns null
	// @errors  TypeError on wrong argument types; RuntimeError if max <= min or not given 7 arguments
	// @example no-run progressBar(20, 120, 200, 8, done, 0.0, total)
	// @since   0.1.0
	// @see     slider, spinner
	Builtins["progressBar"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 7 {
			return typeError("progressBar expects 7 arguments: x, y, w, h, value, min, max", ast.Pos{})
		}
		coords, err := requireIntArgs("progressBar", "x/y/w/h must be integers", args[0:4])
		if err != nil {
			return err
		}
		readF := func(o Object) (float64, bool) {
			switch v := o.(type) {
			case *Float:
				return v.Value, true
			case *Integer:
				return float64(v.Value), true
			}
			return 0, false
		}
		value, ok1 := readF(args[4])
		minV, ok2 := readF(args[5])
		maxV, ok3 := readF(args[6])
		if !ok1 || !ok2 || !ok3 {
			return typeError("progressBar: value/min/max must be numbers", ast.Pos{})
		}
		if maxV <= minV {
			return runtimeError("progressBar: max must be greater than min", ast.Pos{})
		}
		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		t := (value - minV) / (maxV - minV)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, fh*0.5, uiCore.theme.track)
		activeRenderer.fillRoundedRect(fx, fy, fw*float32(t), fh, fh*0.5, uiCore.theme.accent)
		return NULL
	}}

	// toggle — draw an on/off switch (pill), returning its new state.
	//
	// Like checkbox but rendered as a sliding pill switch. Immediate-mode: pass
	// the current state and store the returned value — it flips on the frame it's
	// clicked.
	//
	// @sig     toggle(label: string, x: int, y: int, on: bool, [size: number]) -> bool
	// @param   label  the text beside the switch
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   on     the current on/off state
	// @param   size   label text scale (default 0.5)
	// @returns the state after this frame (flipped if clicked)
	// @errors  TypeError on wrong argument types; RuntimeError unless given 4 or 5 arguments
	// @example no-run darkMode = toggle("Dark mode", 20, 90, darkMode)
	// @since   0.1.0
	// @see     checkbox, radio
	Builtins["toggle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return typeError("toggle expects 4-5 arguments: label, x, y, on, [size]", ast.Pos{})
		}
		label, ok := args[0].(*String)
		if !ok {
			return typeError("toggle: label must be a string", ast.Pos{})
		}
		coords, err := requireIntArgs("toggle", "x/y must be integers", args[1:3])
		if err != nil {
			return err
		}
		onArg, ok := args[3].(*Boolean)
		if !ok {
			return typeError("toggle: on must be a boolean", ast.Pos{})
		}
		scale, err := extractScale("toggle", args, 4, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("tgl_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		const pillW = float32(36)
		const pillH = float32(18)
		labelPad := float32(8)
		labelW := activeRenderer.textWidth(label.Value, scale)
		totalW := pillW + labelPad + labelW

		disabled := uiDisabled()
		in := uiInput()
		hovered := uiCore.hoveredID == id && !disabled
		on := onArg.Value
		changed := false
		if hovered && in.mouseClicked {
			on = !on
			changed = true
			uiCore.activeID = id
		}

		// Track colour animates between off and on.
		trackTarget := uiCore.theme.track
		if on {
			trackTarget = uiCore.theme.accent
		}
		track := animateColor(id+"::track", trackTarget)
		activeRenderer.fillRoundedRect(fx, fy, pillW, pillH, pillH*0.5, fadeIfDisabled(track))
		// Knob slides between left and right end of the track — also animated.
		knobR := pillH * 0.5
		knobTargetX := fx + knobR
		if on {
			knobTargetX = fx + pillW - knobR
		}
		knobPosKey := id + "::knobX"
		// Reuse animColors as a 1-d animator by stuffing the x into [0].
		knobCur := animateColor(knobPosKey, [4]float32{knobTargetX, 0, 0, 0})
		knobX := knobCur[0]
		activeRenderer.fillRoundedRect(knobX-knobR+1, fy+1, knobR*2-2, pillH-2, knobR-1, fadeIfDisabled(uiCore.theme.handle))
		activeRenderer.strokeRoundedRect(knobX-knobR+1, fy+1, knobR*2-2, pillH-2, knobR-1, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))
		activeRenderer.drawText(label.Value, int(fx+pillW+labelPad), int(fy+pillH*0.5), false, scale, fadeIfDisabled(uiCore.theme.labelText))
		uiRegisterElement(id, fx, fy, totalW, pillH)
		if changed && UiEventHookActive() {
			FireUiEventHook("toggle", "toggle", label.Value, &Boolean{Value: on}, int(in.mouseX), int(in.mouseY))
		}
		return &Boolean{Value: on}
	}}

	// radio — draw one option of a radio group, returning the group's selection.
	//
	// Each option's `value` is its identity; `groupValue` is the currently
	// selected one. Returns `value` if this option was clicked, otherwise
	// `groupValue` unchanged — so chain one variable through every option:
	// g = radio("A", …, "a", g); g = radio("B", …, "b", g). The option whose
	// value equals groupValue draws as selected.
	//
	// @sig     radio(label: string, x: int, y: int, value: string, groupValue: string, [size: number]) -> string
	// @param   label       the text beside this option
	// @param   x           left position in pixels
	// @param   y           top position in pixels
	// @param   value       this option's identifier
	// @param   groupValue  the group's currently selected value
	// @param   size        label text scale (default 0.5)
	// @returns this option's value if it was clicked, otherwise groupValue
	// @errors  TypeError if label/value/groupValue aren't strings or x/y aren't integers; RuntimeError unless given 5 or 6 arguments
	// @example no-run shape = radio("Circle", 20, 60, "circle", shape)
	// @since   0.1.0
	// @see     checkbox, toggle, dropdown
	Builtins["radio"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("radio expects 5-6 arguments: label, x, y, value, groupValue, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		valueArg, ok4 := args[3].(*String)
		groupArg, ok5 := args[4].(*String)
		if !ok1 || !ok4 || !ok5 {
			return typeError("radio: label/value/groupValue must be strings", ast.Pos{})
		}
		coords, err := requireIntArgs("radio", "x/y must be integers", args[1:3])
		if err != nil {
			return err
		}
		scale, err := extractScale("radio", args, 5, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("rad_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		const dotSize = float32(18)
		labelPad := float32(8)
		labelW := activeRenderer.textWidth(label.Value, scale)
		totalW := dotSize + labelPad + labelW

		in := uiInput()
		hovered := uiCore.hoveredID == id
		selected := valueArg.Value == groupArg.Value
		newGroup := groupArg.Value
		changed := false
		if hovered && in.mouseClicked && newGroup != valueArg.Value {
			newGroup = valueArg.Value
			selected = true
			changed = true
			uiCore.activeID = id
		}

		bg := uiCore.theme.inputBg
		if hovered {
			bg = uiCore.theme.widgetBgHover
		}
		// Circle = rounded rect with r=half-size; matches every backend.
		activeRenderer.fillRoundedRect(fx, fy, dotSize, dotSize, dotSize*0.5, bg)
		activeRenderer.strokeRoundedRect(fx, fy, dotSize, dotSize, dotSize*0.5, 0.5, uiCore.theme.dimText)
		if selected {
			inset := float32(5)
			activeRenderer.fillRoundedRect(fx+inset, fy+inset, dotSize-inset*2, dotSize-inset*2, (dotSize-inset*2)*0.5, uiCore.theme.accent)
		}
		lh := activeRenderer.lineHeight(scale)
		activeRenderer.drawText(label.Value, int(fx+dotSize+labelPad), int(fy+(dotSize-lh)*0.5), false, scale, uiCore.theme.labelText)
		uiRegisterElement(id, fx, fy, totalW, dotSize)
		if changed && UiEventHookActive() {
			FireUiEventHook("select", "radio", label.Value, &String{Value: newGroup}, int(in.mouseX), int(in.mouseY))
		}
		return &String{Value: newGroup}
	}}

	// ════════════════════════════════════════════════════════════════
	// Phase 1b — composites + charts
	// ════════════════════════════════════════════════════════════════

	// tabs — draw a row of tabs, returning the selected index.
	//
	// Immediate-mode: pass the current active index and store the returned one —
	// it changes to a tab's index on the frame that tab is clicked. Tabs auto-size
	// to their labels, distributing slack or shrinking to fit the width.
	//
	// @sig     tabs(x: int, y: int, w: int, items: array, activeIdx: int, [size: number]) -> int
	// @param   x          left position in pixels
	// @param   y          top position in pixels
	// @param   w          total width in pixels for the tab strip
	// @param   items      array of tab label strings
	// @param   activeIdx  the currently selected tab index
	// @param   size       label text scale (default 0.5)
	// @returns the selected tab index after this frame
	// @errors  TypeError if x/y/w aren't integers, items isn't an array, or activeIdx isn't an integer; RuntimeError unless given 5 or 6 arguments
	// @example no-run tab = tabs(0, 0, 400, ["Files", "Edit", "View"], tab)
	// @since   0.1.0
	// @see     accordion, dropdown
	Builtins["tabs"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("tabs expects 5-6 arguments: x, y, w, items, activeIdx, [size]", ast.Pos{})
		}
		coords, err := requireIntArgs("tabs", "x/y/w must be integers", args[0:3])
		if err != nil {
			return err
		}
		itemsObj, ok := args[3].(*Array)
		if !ok {
			return typeError("tabs: items must be an array of strings", ast.Pos{})
		}
		activeIdxObj, ok := args[4].(*Integer)
		if !ok {
			return typeError("tabs: activeIdx must be an integer", ast.Pos{})
		}
		scale, err := extractScale("tabs", args, 5, 0.5)
		if err != nil {
			return err
		}

		baseID := uiCore.nextID
		uiCore.nextID++

		fx, fy, fw := float32(coords[0]), float32(coords[1]), float32(coords[2])
		const tabH = float32(34)
		n := len(itemsObj.Elements)
		if n == 0 {
			return activeIdxObj
		}
		activeIdx := activeIdxObj.Value
		originalIdx := activeIdx

		// Auto-size: each tab gets its natural label width + padding.
		// If total < fw, slack distributes evenly. If total > fw, every
		// tab shrinks proportionally so all stay visible.
		labels := make([]string, n)
		natural := make([]float32, n)
		const tabPad = float32(20)
		totalNat := float32(0)
		for i := 0; i < n; i++ {
			if s, ok := itemsObj.Elements[i].(*String); ok {
				labels[i] = s.Value
			} else {
				labels[i] = itemsObj.Elements[i].Inspect()
			}
			natural[i] = activeRenderer.textWidth(labels[i], scale) + tabPad*2
			if natural[i] < 60 {
				natural[i] = 60
			}
			totalNat += natural[i]
		}
		widths := make([]float32, n)
		if totalNat <= fw {
			extra := (fw - totalNat) / float32(n)
			for i := 0; i < n; i++ {
				widths[i] = natural[i] + extra
			}
		} else {
			squash := fw / totalNat
			for i := 0; i < n; i++ {
				widths[i] = natural[i] * squash
			}
		}

		in := uiInput()
		// Base underline along the full width.
		activeRenderer.fillRoundedRect(fx, fy+tabH-2, fw, 2, 0, uiCore.theme.widgetBgActive)

		tx := fx
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("tab_%d_%d", baseID, i)
			tabW := widths[i]

			isActive := i == activeIdx
			hovered := uiCore.hoveredID == id
			if hovered && in.mouseClicked {
				activeIdx = i
				uiCore.activeID = id
			}

			textColor := uiCore.theme.dimText
			if isActive {
				activeRenderer.fillRoundedRect(tx+1, fy+1, tabW-2, tabH-1, 0, uiCore.theme.widgetBg)
				// Bottom accent bar (active indicator).
				activeRenderer.fillRoundedRect(tx, fy+tabH-3, tabW, 3, uiCore.style.radiusSmall, uiCore.theme.accent)
				textColor = uiCore.theme.widgetText
			} else if hovered {
				activeRenderer.fillRoundedRect(tx+1, fy+1, tabW-2, tabH-3, 0, uiCore.theme.track)
				textColor = uiCore.theme.labelText
			}
			activeRenderer.drawText(labels[i], int(tx+tabW*0.5), int(fy+tabH*0.5), true, scale, textColor)
			uiRegisterElement(id, tx, fy, tabW, tabH)
			tx += tabW
		}
		if activeIdx != originalIdx && UiEventHookActive() {
			pickedLabel := ""
			if str, ok := itemsObj.Elements[activeIdx].(*String); ok {
				pickedLabel = str.Value
			}
			FireUiEventHook("select", "tabs", pickedLabel, &Integer{Value: activeIdx}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: activeIdx}
	}}

	// numericStepper — draw a −/value/+ integer stepper, returning the new value.
	//
	// Immediate-mode: pass the current integer and store what it returns — the −
	// and + buttons step it by 1, clamped to [min, max]. For a continuous range
	// use slider instead.
	//
	// @sig     numericStepper(label: string, x: int, y: int, w: int, value: int, min: int, max: int, [size: number]) -> int
	// @param   label  caption drawn above the control ("" for none)
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   w      total width in pixels
	// @param   value  the current integer value
	// @param   min    minimum value (− stops here)
	// @param   max    maximum value (+ stops here)
	// @param   size   text scale (default 0.5)
	// @returns the value after this frame (±1 if a button was clicked)
	// @errors  TypeError on wrong argument types; RuntimeError unless given 7 or 8 arguments
	// @example no-run count = numericStepper("Count", 20, 60, 120, count, 0, 10)
	// @since   0.1.0
	// @see     slider, textInput
	Builtins["numericStepper"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("numericStepper expects 7-8 arguments: label, x, y, w, value, min, max, [size]", ast.Pos{})
		}
		label, ok := args[0].(*String)
		if !ok {
			return typeError("numericStepper: label must be a string", ast.Pos{})
		}
		coords, err := requireIntArgs("numericStepper", "x/y/w must be integers", args[1:4])
		if err != nil {
			return err
		}
		readI := func(o Object) (int, bool) {
			switch v := o.(type) {
			case *Integer:
				return v.Value, true
			case *Float:
				return int(v.Value), true
			}
			return 0, false
		}
		value, ok1 := readI(args[4])
		minV, ok2 := readI(args[5])
		maxV, ok3 := readI(args[6])
		if !ok1 || !ok2 || !ok3 {
			return typeError("numericStepper: value/min/max must be numeric", ast.Pos{})
		}
		original := value
		scale, err := extractScale("numericStepper", args, 7, 0.5)
		if err != nil {
			return err
		}

		baseID := uiCore.nextID
		uiCore.nextID++

		fx, fy, fw := float32(coords[0]), float32(coords[1]), float32(coords[2])
		const stepH = float32(30)
		const btnW = float32(32)

		minusID := fmt.Sprintf("ns_m_%d", baseID)
		plusID := fmt.Sprintf("ns_p_%d", baseID)

		in := uiInput()
		// Label above.
		if label.Value != "" {
			activeRenderer.drawText(label.Value, int(fx), int(fy), false, scale, uiCore.theme.labelText)
		}
		labelOffset := float32(0)
		if label.Value != "" {
			labelOffset = 20
		}
		boxY := fy + labelOffset

		r := uiCore.style.radiusMedium

		// [−] button.
		mBg := uiCore.theme.widgetBg
		if uiCore.hoveredID == minusID {
			mBg = uiCore.theme.widgetBgHover
		}
		activeRenderer.fillRoundedRect(fx, boxY, btnW, stepH, r, mBg)
		activeRenderer.strokeRoundedRect(fx, boxY, btnW, stepH, r, 0.5, uiCore.theme.widgetBgActive)
		activeRenderer.drawText("-", int(fx+btnW*0.5), int(boxY+stepH*0.5), true, scale, uiCore.theme.widgetText)
		uiRegisterElement(minusID, fx, boxY, btnW, stepH)

		// Value field.
		fieldX := fx + btnW + 2
		fieldW := fw - btnW*2 - 4
		activeRenderer.fillRoundedRect(fieldX, boxY, fieldW, stepH, r, uiCore.theme.inputBg)
		activeRenderer.strokeRoundedRect(fieldX, boxY, fieldW, stepH, r, 0.5, uiCore.theme.widgetBgActive)
		activeRenderer.drawText(fmt.Sprintf("%d", value), int(fieldX+fieldW*0.5), int(boxY+stepH*0.5), true, scale, uiCore.theme.widgetText)

		// [+] button.
		pBg := uiCore.theme.widgetBg
		if uiCore.hoveredID == plusID {
			pBg = uiCore.theme.widgetBgHover
		}
		plusX := fx + fw - btnW
		activeRenderer.fillRoundedRect(plusX, boxY, btnW, stepH, r, pBg)
		activeRenderer.strokeRoundedRect(plusX, boxY, btnW, stepH, r, 0.5, uiCore.theme.widgetBgActive)
		activeRenderer.drawText("+", int(plusX+btnW*0.5), int(boxY+stepH*0.5), true, scale, uiCore.theme.widgetText)
		uiRegisterElement(plusID, plusX, boxY, btnW, stepH)

		if in.mouseClicked {
			if uiCore.hoveredID == minusID && value > minV {
				value--
			} else if uiCore.hoveredID == plusID && value < maxV {
				value++
			}
		}
		if value != original && UiEventHookActive() {
			FireUiEventHook("step", "numericStepper", label.Value, &Integer{Value: value}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: value}
	}}

	// accordion — draw collapsible sections, returning the open one's index.
	//
	// Immediate-mode: pass the currently open section index and store what it
	// returns — clicking a header opens that section (and, by convention, clicking
	// the open one collapses it, giving -1). Only one section is open at a time.
	//
	// @sig     accordion(x: int, y: int, w: int, sections: array, openIdx: int, [size: number]) -> int
	// @param   x         left position in pixels
	// @param   y         top position in pixels
	// @param   w         width in pixels
	// @param   sections  array of section header strings
	// @param   openIdx   index of the currently open section (-1 for none)
	// @param   size      text scale (default 0.5)
	// @returns the open section index after this frame
	// @errors  TypeError if x/y/w aren't integers or sections isn't an array; RuntimeError unless given 5 or 6 arguments
	// @example no-run open = accordion(20, 20, 240, ["General", "Advanced"], open)
	// @since   0.1.0
	// @see     tabs, treeView
	Builtins["accordion"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("accordion expects 5-6 arguments: x, y, w, sections, openIdx, [size]", ast.Pos{})
		}
		coords, err := requireIntArgs("accordion", "x/y/w must be integers", args[0:3])
		if err != nil {
			return err
		}
		sections, ok := args[3].(*Array)
		if !ok {
			return typeError("accordion: sections must be an array of strings", ast.Pos{})
		}
		openIdxObj, ok := args[4].(*Integer)
		if !ok {
			return typeError("accordion: openIdx must be an integer", ast.Pos{})
		}
		scale, err := extractScale("accordion", args, 5, 0.5)
		if err != nil {
			return err
		}

		fx, fy, fw := float32(coords[0]), float32(coords[1]), float32(coords[2])
		openIdx := openIdxObj.Value
		originalOpenIdx := openIdx
		const hdrH = float32(28)

		in := uiInput()
		var clickedLabel string

		for i, secObj := range sections.Elements {
			lbl := ""
			if s, ok := secObj.(*String); ok {
				lbl = s.Value
			}
			hy := fy + float32(i)*hdrH
			isOpen := openIdx == i
			hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= hy && in.mouseY <= hy+hdrH

			var bg [4]float32
			switch {
			case isOpen:
				bg = uiCore.theme.widgetBg
			case hovered:
				bg = uiCore.theme.widgetBgActive
			default:
				bg = uiCore.theme.track
			}
			activeRenderer.fillRoundedRect(fx, hy, fw, hdrH, uiCore.style.radiusMedium, bg)
			activeRenderer.strokeRoundedRect(fx, hy, fw, hdrH, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
			if isOpen {
				activeRenderer.fillRoundedRect(fx, hy, 3, hdrH, 0, uiCore.theme.accent)
			}
			sym := ">"
			if isOpen {
				sym = "v"
			}
			textColor := uiCore.theme.labelText
			if isOpen {
				textColor = uiCore.theme.widgetText
			}
			activeRenderer.drawText(sym, int(fx+8), int(hy+hdrH*0.5), false, scale, textColor)
			activeRenderer.drawText(lbl, int(fx+26), int(hy+hdrH*0.5), false, scale, textColor)

			if hovered && in.mouseClicked {
				if isOpen {
					openIdx = -1
				} else {
					openIdx = i
				}
				clickedLabel = lbl
			}
		}
		if openIdx != originalOpenIdx && UiEventHookActive() {
			kind := "expand"
			if openIdx == -1 {
				kind = "collapse"
			}
			FireUiEventHook(kind, "accordion", clickedLabel, &Integer{Value: openIdx}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: openIdx}
	}}

	// modal — draw a centred dialog over a dimmed screen, returning a clicked button.
	//
	// A full-screen dim plus a centred dialog with a title, message, and a row of
	// buttons. Returns the clicked button's label, or "" if none was clicked this
	// frame — drive your own "is the dialog open" flag with that. Call it AFTER all
	// other widgets so it renders on top.
	//
	// @sig     modal(title: string, message: string, buttons: array) -> string
	// @param   title    the dialog title-bar text
	// @param   message  the body text (single line; pre-format your own wrapping)
	// @param   buttons  array of button label strings, right-aligned
	// @returns the clicked button's label, or "" if none was clicked
	// @errors  TypeError if title/message aren't strings or buttons isn't an array; RuntimeError unless given 3 arguments
	// @example no-run if modal("Quit?", "Discard changes?", ["Cancel", "Quit"]) == "Quit" { exit() }
	// @since   0.1.0
	// @see     toast, tooltip, button
	Builtins["modal"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return typeError("modal expects 3 arguments: title, message, buttons", ast.Pos{})
		}
		title, ok1 := args[0].(*String)
		message, ok2 := args[1].(*String)
		buttons, ok3 := args[2].(*Array)
		if !ok1 || !ok2 || !ok3 {
			return typeError("modal: title/message must be strings, buttons must be array", ast.Pos{})
		}
		numBtns := len(buttons.Elements)
		if numBtns == 0 {
			return &String{Value: ""}
		}

		const textScale float32 = 0.62
		in := uiInput()
		ww, wh := float32(in.winW), float32(in.winH)

		const dialogW float32 = 420
		const titleH float32 = 40
		const btnAreaH float32 = 52
		const pad float32 = 18
		const lineH float32 = 22

		// Plain message render — no word-wrap here; caller can preformat.
		// Future: when uiWrapText lands on the shared path, wire it in.
		msgLines := []string{message.Value}
		msgH := float32(len(msgLines))*lineH + pad*2
		dialogH := titleH + msgH + btnAreaH
		dx := (ww - dialogW) * 0.5
		dy := (wh - dialogH) * 0.5

		// Dim overlay.
		activeRenderer.fillRoundedRect(0, 0, ww, wh, 0, [4]float32{0, 0, 0, 0.60})
		// Drop shadow under the dialog — heavy blur for that
		// "important overlay" feel. Multiplier on theme.shadow lets
		// the alpha scale with theme (dark themes set the base higher).
		modalR := uiCore.style.radiusLarge
		shadow := uiCore.theme.shadow
		shadow[3] *= 1.0
		activeRenderer.dropShadow(dx, dy, dialogW, dialogH, modalR, 14, 48, shadow)
		// Dialog body.
		activeRenderer.fillRoundedRect(dx, dy, dialogW, dialogH, modalR, uiCore.theme.inputBg)
		activeRenderer.strokeRoundedRect(dx, dy, dialogW, dialogH, modalR, 0.5, uiCore.theme.widgetBgActive)
		// Title bar.
		activeRenderer.fillRoundedRect(dx, dy, dialogW, titleH, modalR, uiCore.theme.widgetBg)
		activeRenderer.drawText(title.Value, int(dx+pad), int(dy+titleH*0.5), false, textScale, uiCore.theme.widgetText)

		// Message lines.
		for i, line := range msgLines {
			activeRenderer.drawText(line, int(dx+pad), int(dy+titleH+pad+float32(i)*lineH), false, textScale, uiCore.theme.labelText)
		}

		// Buttons — right-aligned at bottom.
		const btnH float32 = 32
		const btnGap float32 = 8
		btnY := dy + dialogH - btnAreaH + (btnAreaH-btnH)*0.5
		btnWidths := make([]float32, numBtns)
		totalBtnW := float32(0)
		for i, b := range buttons.Elements {
			lbl := ""
			if s, ok := b.(*String); ok {
				lbl = s.Value
			}
			w := activeRenderer.textWidth(lbl, textScale) + 24
			if w < 80 {
				w = 80
			}
			btnWidths[i] = w
			totalBtnW += w
		}
		totalBtnW += btnGap * float32(numBtns-1)
		bx := dx + dialogW - pad - totalBtnW

		result := ""
		for i, b := range buttons.Elements {
			lbl := ""
			if s, ok := b.(*String); ok {
				lbl = s.Value
			}
			bw := btnWidths[i]
			isOver := in.mouseX >= bx && in.mouseX <= bx+bw && in.mouseY >= btnY && in.mouseY <= btnY+btnH

			bg := uiCore.theme.widgetBg
			textColor := uiCore.theme.labelText
			if isOver {
				bg = uiCore.theme.accent
				textColor = uiCore.theme.widgetText
			}
			activeRenderer.fillRoundedRect(bx, btnY, bw, btnH, uiCore.style.radiusMedium, bg)
			activeRenderer.strokeRoundedRect(bx, btnY, bw, btnH, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
			activeRenderer.drawText(lbl, int(bx+bw*0.5), int(btnY+btnH*0.5), true, textScale, textColor)

			if isOver && in.mouseClicked {
				result = lbl
			}
			bx += bw + btnGap
		}

		// Block background widget hover next frame via full-screen hit element.
		uiRegisterElement("__modal__", 0, 0, ww, wh)
		if result != "" && UiEventHookActive() {
			FireUiEventHook("click", "modal", result, &String{Value: title.Value}, int(in.mouseX), int(in.mouseY))
		}
		return &String{Value: result}
	}}

	// splitter — draw a draggable pane divider, returning its position.
	//
	// Immediate-mode: pass the current divider position and store what it returns
	// — while dragged it follows the mouse along its axis, clamped to [min, max].
	// orient "v" makes a vertical divider you drag horizontally (pos is an x); "h"
	// makes a horizontal one you drag vertically (pos is a y).
	//
	// @sig     splitter(pos: int, x: int, y: int, length: int, orient: string, min: int, max: int, [thickness: int]) -> int
	// @param   pos        the divider's current position along its drag axis (px)
	// @param   x          the divider's x (its fixed coordinate when horizontal)
	// @param   y          the divider's y (its fixed coordinate when vertical)
	// @param   length     how long the divider is, in pixels
	// @param   orient     "v" (vertical bar, drags in x) or "h" (horizontal bar, drags in y)
	// @param   min        minimum position (px)
	// @param   max        maximum position (px)
	// @param   thickness  bar thickness in pixels (default 6)
	// @returns the divider position after this frame
	// @errors  TypeError on wrong argument types or an orient other than "v"/"h"; RuntimeError unless given 7 or 8 arguments
	// @example no-run splitX = splitter(splitX, 0, 0, 400, "v", 100, 600)
	// @since   0.1.0
	// @see     scrollArea, table
	Builtins["splitter"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("splitter expects 7-8 arguments: pos, x, y, length, orient, min, max, [thickness]", ast.Pos{})
		}
		readI := func(o Object) (int, bool) {
			if v, ok := o.(*Integer); ok {
				return v.Value, true
			}
			return 0, false
		}
		pos, ok1 := readI(args[0])
		rx, ok2 := readI(args[1])
		ry, ok3 := readI(args[2])
		length, ok4 := readI(args[3])
		orient, ok5 := args[4].(*String)
		minV, ok6 := readI(args[5])
		maxV, ok7 := readI(args[6])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
			return typeError("splitter: pos/x/y/length/min/max must be integers, orient must be string", ast.Pos{})
		}
		if orient.Value != "v" && orient.Value != "h" {
			return typeError(`splitter: orient must be "v" or "h"`, ast.Pos{})
		}
		thickness := 6
		if len(args) == 8 {
			t, ok := readI(args[7])
			if !ok {
				return typeError("splitter: thickness must be an integer", ast.Pos{})
			}
			thickness = t
		}
		originalPos := pos
		isVertical := orient.Value == "v"

		prefix := "spl_v_"
		if !isVertical {
			prefix = "spl_h_"
		}
		id := fmt.Sprintf("%s%d", prefix, uiCore.nextID)
		uiCore.nextID++

		half := float32(thickness) * 0.5
		var hx, hy, hw, hh float32
		if isVertical {
			hx, hy = float32(pos)-half, float32(ry)
			hw, hh = float32(thickness), float32(length)
		} else {
			hx, hy = float32(rx), float32(pos)-half
			hw, hh = float32(length), float32(thickness)
		}

		in := uiInput()
		hovered := uiCore.hoveredID == id
		if hovered && in.mouseClicked {
			uiCore.activeID = id
		}
		if uiCore.activeID == id {
			if in.mouseDown {
				if isVertical {
					pos = int(in.mouseX)
				} else {
					pos = int(in.mouseY)
				}
				if pos < minV {
					pos = minV
				}
				if pos > maxV {
					pos = maxV
				}
			} else {
				uiCore.activeID = ""
			}
		}

		bg := uiCore.theme.track
		if hovered || uiCore.activeID == id {
			bg = uiCore.theme.accent
		}
		// Recompute the visual rect from the (possibly updated) pos.
		if isVertical {
			hx = float32(pos) - half
		} else {
			hy = float32(pos) - half
		}
		activeRenderer.fillRoundedRect(hx, hy, hw, hh, 2, bg)
		uiRegisterElement(id, hx, hy, hw, hh)

		if pos != originalPos && UiEventHookActive() {
			FireUiEventHook("drag", "splitter", orient.Value, &Integer{Value: pos}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: pos}
	}}

	// image — draw a loaded image inside the UI at (x, y) scaled to w×h.
	//
	// The UI-layer wrapper around a loadImage result (the drawImage equivalent for
	// widget layouts). The image is stretched to the given rectangle. The optional
	// mode is reserved for future "fit"/"fill" behaviours and currently ignored.
	//
	// @sig     image(img: image, x: int, y: int, w: int, h: int, [mode: string]) -> null
	// @param   img   an image from loadImage
	// @param   x     left position in pixels
	// @param   y     top position in pixels
	// @param   w     draw width in pixels
	// @param   h     draw height in pixels
	// @param   mode  reserved for future scaling modes (currently ignored)
	// @returns null
	// @errors  TypeError if img isn't an image or x/y/w/h aren't integers; RuntimeError unless given 5 or 6 arguments
	// @example no-run image(logo, 20, 20, 64, 64)
	// @since   0.1.0
	// @see     loadImage, drawImage
	Builtins["image"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("image expects 5-6 arguments: img, x, y, w, h, [mode]", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError("image: first argument must be an image (from loadImage)", ast.Pos{})
		}
		coords, err := requireIntArgs("image", "x/y/w/h must be integers", args[1:5])
		if err != nil {
			return err
		}
		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		activeRenderer.drawImage(img, fx, fy, fw, fh)
		return NULL
	}}

	// ── Charts ───────────────────────────────────────────────────────

	readSeries := func(name string, a *Array) []float32 {
		out := make([]float32, len(a.Elements))
		for i, el := range a.Elements {
			switch v := el.(type) {
			case *Float:
				out[i] = float32(v.Value)
			case *Integer:
				out[i] = float32(v.Value)
			default:
				_ = name // keep linter quiet if we make this richer later
			}
		}
		return out
	}
	seriesRange := func(s []float32) (float32, float32) {
		if len(s) == 0 {
			return 0, 1
		}
		lo, hi := s[0], s[0]
		for _, v := range s {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		if hi == lo {
			hi = lo + 1
		}
		return lo, hi
	}

	// sparkline — draw a tiny inline line chart: just the line, no axes or background.
	//
	// Auto-scales to the data's range, drawn in the accent colour. Ideal for
	// compact dashboard tiles. Needs at least 2 points (fewer draws nothing).
	//
	// @sig     sparkline(data: array, x: int, y: int, w: int, h: int) -> null
	// @param   data  array of numbers to plot
	// @param   x     left position in pixels
	// @param   y     top position in pixels
	// @param   w     width in pixels
	// @param   h     height in pixels
	// @returns null
	// @errors  TypeError if data isn't an array or x/y/w/h aren't integers; RuntimeError unless given 5 arguments
	// @example no-run sparkline(history, 10, 10, 120, 32)
	// @since   0.1.0
	// @see     lineChart, barChart
	Builtins["sparkline"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("sparkline expects 5 arguments: data, x, y, w, h", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("sparkline: data must be an array of numbers", ast.Pos{})
		}
		coords, err := requireIntArgs("sparkline", "x/y/w/h must be integers", args[1:5])
		if err != nil {
			return err
		}
		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		series := readSeries("sparkline", data)
		if len(series) < 2 {
			return NULL
		}
		lo, hi := seriesRange(series)
		step := fw / float32(len(series)-1)
		for i := 0; i < len(series)-1; i++ {
			x1 := fx + float32(i)*step
			y1 := fy + fh - (series[i]-lo)/(hi-lo)*fh
			x2 := fx + float32(i+1)*step
			y2 := fy + fh - (series[i+1]-lo)/(hi-lo)*fh
			activeRenderer.drawLine(x1, y1, x2, y2, 2, uiCore.theme.accent)
		}
		return NULL
	}}

	// lineChart — draw a line chart with background, axes, and a filled area.
	//
	// Plots data across the rectangle in the current fill colour (line + 25%-alpha
	// area), with a dot per point and an accent border. If min/max are omitted the
	// y-range is derived from the data; pass them to pin the scale.
	//
	// @sig     lineChart(data: array, x: int, y: int, w: int, h: int, [min: number], [max: number]) -> null
	// @param   data  array of numbers to plot
	// @param   x     left position in pixels
	// @param   y     top position in pixels
	// @param   w     width in pixels
	// @param   h     height in pixels
	// @param   min   value mapped to the bottom (default: data minimum)
	// @param   max   value mapped to the top (default: data maximum)
	// @returns null
	// @errors  TypeError if data isn't an array, x/y/w/h aren't integers, or min/max aren't numbers; RuntimeError unless given 5 or 7 arguments
	// @example no-run lineChart(durations, 10, 10, 400, 150)
	// @since   0.1.0
	// @see     barChart, sparkline, pieChart
	Builtins["lineChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 && len(args) != 7 {
			return typeError("lineChart expects 5 or 7 arguments: data, x, y, w, h, [min, max]", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("lineChart: data must be an array of numbers", ast.Pos{})
		}
		coords, err := requireIntArgs("lineChart", "x/y/w/h must be integers", args[1:5])
		if err != nil {
			return err
		}
		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		series := readSeries("lineChart", data)
		if len(series) < 2 {
			return NULL
		}
		var lo, hi float32
		if len(args) == 7 {
			readF := func(o Object) (float32, bool) {
				switch v := o.(type) {
				case *Float:
					return float32(v.Value), true
				case *Integer:
					return float32(v.Value), true
				}
				return 0, false
			}
			loV, ok1 := readF(args[5])
			hiV, ok2 := readF(args[6])
			if !ok1 || !ok2 {
				return typeError("lineChart: min/max must be numbers", ast.Pos{})
			}
			lo, hi = loV, hiV
		} else {
			lo, hi = seriesRange(series)
		}
		// Background.
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, 4, uiCore.theme.inputBg)
		// Axis lines (subtle).
		axis := [4]float32{1, 1, 1, 0.08}
		activeRenderer.drawLine(fx, fy+fh, fx+fw, fy+fh, 1, axis)
		activeRenderer.drawLine(fx, fy, fx, fy+fh, 1, axis)
		// Filled area: build a closed polygon under the line.
		step := fw / float32(len(series)-1)
		pts := make([]float32, 0, (len(series)+2)*2)
		pts = append(pts, fx, fy+fh)
		for i, v := range series {
			px := fx + float32(i)*step
			py := fy + fh - (v-lo)/(hi-lo)*fh
			pts = append(pts, px, py)
		}
		pts = append(pts, fx+fw, fy+fh)
		fillCol := uiCore.theme.accent
		fillCol[3] = 0.25
		activeRenderer.fillPolygon(pts, fillCol)
		// Line + dots.
		for i := 0; i < len(series)-1; i++ {
			x1 := fx + float32(i)*step
			y1 := fy + fh - (series[i]-lo)/(hi-lo)*fh
			x2 := fx + float32(i+1)*step
			y2 := fy + fh - (series[i+1]-lo)/(hi-lo)*fh
			activeRenderer.drawLine(x1, y1, x2, y2, 2, uiCore.theme.accent)
		}
		for i, v := range series {
			cx := fx + float32(i)*step
			cy := fy + fh - (v-lo)/(hi-lo)*fh
			activeRenderer.fillRoundedRect(cx-3, cy-3, 6, 6, 3, uiCore.theme.accent)
		}
		return NULL
	}}

	// barChart — draw a vertical bar chart with a track column behind each bar.
	//
	// Bars use the current fill colour, each with a 15% gap. The baseline anchors
	// at 0 unless you pass min; if max is omitted it's derived from the data.
	//
	// @sig     barChart(data: array, x: int, y: int, w: int, h: int, [min: number], [max: number]) -> null
	// @param   data  array of numbers, one bar each
	// @param   x     left position in pixels
	// @param   y     top position in pixels
	// @param   w     width in pixels
	// @param   h     height in pixels
	// @param   min   value mapped to a zero-height bar (default 0)
	// @param   max   value mapped to a full-height bar (default: data maximum)
	// @returns null
	// @errors  TypeError if data isn't an array, x/y/w/h aren't integers, or min/max aren't numbers; RuntimeError unless given 5 or 7 arguments
	// @example no-run barChart(counts, 10, 170, 400, 120)
	// @since   0.1.0
	// @see     lineChart, pieChart, sparkline
	Builtins["barChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 && len(args) != 7 {
			return typeError("barChart expects 5 or 7 arguments: data, x, y, w, h, [min, max]", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("barChart: data must be an array of numbers", ast.Pos{})
		}
		coords, err := requireIntArgs("barChart", "x/y/w/h must be integers", args[1:5])
		if err != nil {
			return err
		}
		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		series := readSeries("barChart", data)
		if len(series) == 0 {
			return NULL
		}
		var lo, hi float32
		if len(args) == 7 {
			readF := func(o Object) (float32, bool) {
				switch v := o.(type) {
				case *Float:
					return float32(v.Value), true
				case *Integer:
					return float32(v.Value), true
				}
				return 0, false
			}
			loV, ok1 := readF(args[5])
			hiV, ok2 := readF(args[6])
			if !ok1 || !ok2 {
				return typeError("barChart: min/max must be numbers", ast.Pos{})
			}
			lo, hi = loV, hiV
		} else {
			lo, hi = seriesRange(series)
			if lo > 0 {
				lo = 0 // anchor at zero for sensible bar heights
			}
		}
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, 4, uiCore.theme.inputBg)
		// Bars with gap.
		const gapRatio = float32(0.15)
		slot := fw / float32(len(series))
		barW := slot * (1 - gapRatio)
		for i, v := range series {
			bx := fx + float32(i)*slot + (slot-barW)*0.5
			t := (v - lo) / (hi - lo)
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			bh := t * fh
			by := fy + fh - bh
			// Ghost track behind bar.
			activeRenderer.fillRoundedRect(bx, fy, barW, fh, 0, uiCore.theme.track)
			activeRenderer.fillRoundedRect(bx, by, barW, bh, 0, uiCore.theme.accent)
		}
		return NULL
	}}

	// pieChart — draw a pie (or donut) chart centred at (cx, cy).
	//
	// Each data value is one slice, coloured by the matching [r, g, b, a] entry in
	// colors. Slices start at 12 o'clock and sweep clockwise; zero values are
	// skipped. A positive innerRadius draws a donut instead of a full pie.
	//
	// @sig     pieChart(data: array, colors: array, cx: int, cy: int, radius: int, [innerRadius: int]) -> null
	// @param   data         array of slice values
	// @param   colors       array of [r, g, b, a] colours, one per slice
	// @param   cx           centre x in pixels
	// @param   cy           centre y in pixels
	// @param   radius       outer radius in pixels
	// @param   innerRadius  inner radius for a donut (default 0 = full pie)
	// @returns null
	// @errors  TypeError if data/colors aren't arrays or cx/cy/radius aren't integers; RuntimeError unless given 5 or 6 arguments
	// @example no-run pieChart(counts, colours, 400, 300, 80)
	// @since   0.1.0
	// @see     barChart, lineChart
	Builtins["pieChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 && len(args) != 6 {
			return typeError("pieChart expects 5 or 6 arguments: data, colors, cx, cy, radius, [innerRadius]", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("pieChart: data must be an array of numbers", ast.Pos{})
		}
		colors, ok := args[1].(*Array)
		if !ok {
			return typeError("pieChart: colors must be an array of [r,g,b,a] arrays", ast.Pos{})
		}
		readI := func(o Object) (int, bool) {
			if v, ok := o.(*Integer); ok {
				return v.Value, true
			}
			return 0, false
		}
		cx, ok1 := readI(args[2])
		cy, ok2 := readI(args[3])
		radius, ok3 := readI(args[4])
		if !ok1 || !ok2 || !ok3 {
			return typeError("pieChart: cx/cy/radius must be integers", ast.Pos{})
		}
		_ = 0 // innerRadius reserved for donut variant — Phase 1b draws filled pie only

		series := readSeries("pieChart", data)
		total := float32(0)
		for _, v := range series {
			if v > 0 {
				total += v
			}
		}
		if total <= 0 {
			return NULL
		}
		angle := float32(-3.14159265 / 2) // start at top (12 o'clock)
		for i, v := range series {
			if v <= 0 {
				continue
			}
			sweep := (v / total) * (2 * 3.14159265)
			var col [4]float32
			if i < len(colors.Elements) {
				if a, ok := colors.Elements[i].(*Array); ok {
					if c, ok := rgbaFromArray(a); ok {
						col = c
					}
				}
			}
			activeRenderer.fillArc(float32(cx), float32(cy), float32(radius), angle, angle+sweep, col)
			angle += sweep
		}
		return NULL
	}}

	// ════════════════════════════════════════════════════════════════
	// Phase 2 — scroll family + datagrid
	// ════════════════════════════════════════════════════════════════

	// drawScrollbar draws a track + thumb on the right edge of
	// (fx, fy, fw, fh) AND handles click/drag on the thumb. Returns the
	// (possibly updated) scrollOff so the caller can persist it. Units
	// for scrollOff/maxScroll are whatever the caller uses (lines,
	// items, pixels); the drag math is linear on `t ∈ [0, 1]`.
	drawScrollbar := func(id string, fx, fy, fw, fh, scrollOff, maxScroll, thumbExtent float32) float32 {
		if maxScroll <= 0 {
			return scrollOff
		}
		const sbW = float32(10)
		sbX := fx + fw - sbW - 3
		sbTrackY := fy + 3
		sbTrackH := fh - 6
		thumbH := sbTrackH * thumbExtent
		if thumbH < 20 {
			thumbH = 20
		}
		sbID := id + "_sb"
		in := uiInput()
		// Hit-test the WHOLE track so the user can grab anywhere along it.
		overTrack := in.mouseX >= sbX-2 && in.mouseX <= sbX+sbW+2 &&
			in.mouseY >= sbTrackY && in.mouseY <= sbTrackY+sbTrackH
		if overTrack && in.mouseClicked {
			uiCore.activeID = sbID
		}
		if uiCore.activeID == sbID {
			if in.mouseDown {
				t := (in.mouseY - sbTrackY - thumbH*0.5) / (sbTrackH - thumbH)
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}
				scrollOff = t * maxScroll
			} else {
				uiCore.activeID = ""
			}
		}
		thumbY := sbTrackY + (sbTrackH-thumbH)*scrollOff/maxScroll
		thumbCol := uiCore.theme.widgetBgHover
		if uiCore.activeID == sbID || overTrack {
			thumbCol = uiCore.theme.handle
		}
		activeRenderer.fillRoundedRect(sbX, sbTrackY, sbW, sbTrackH, sbW*0.5, uiCore.theme.track)
		activeRenderer.fillRoundedRect(sbX, thumbY, sbW, thumbH, sbW*0.5, thumbCol)
		return scrollOff
	}

	// scrollArea — draw a scrollbar for a viewport, returning the scroll offset.
	//
	// Manages a vertical scrollbar over the (x, y, w, h) viewport for content
	// `contentH` pixels tall, handling wheel and thumb drag. Returns the current
	// scroll offset in pixels: clip to the viewport with pushClip/popClip and draw
	// your content translated by -offset.
	//
	// @sig     scrollArea(x: number, y: number, w: number, h: number, contentH: number) -> float
	// @param   x         viewport left in pixels
	// @param   y         viewport top in pixels
	// @param   w         viewport width in pixels
	// @param   h         viewport height in pixels
	// @param   contentH  total content height in pixels
	// @returns the current scroll offset in pixels
	// @errors  TypeError unless given 5 numeric arguments
	// @example no-run off = scrollArea(0, 0, 300, 400, contentHeight)
	// @since   0.1.0
	// @see     list, table, splitter
	Builtins["scrollArea"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("scrollArea expects 5 arguments: x, y, w, h, contentH", ast.Pos{})
		}
		readF := func(o Object) (float32, bool) {
			switch v := o.(type) {
			case *Integer:
				return float32(v.Value), true
			case *Float:
				return float32(v.Value), true
			}
			return 0, false
		}
		fxv, ok1 := readF(args[0])
		fyv, ok2 := readF(args[1])
		fwv, ok3 := readF(args[2])
		fhv, ok4 := readF(args[3])
		fc, ok5 := readF(args[4])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("scrollArea: all arguments must be numbers", ast.Pos{})
		}
		fx, fy, fw, fh := fxv, fyv, fwv, fhv

		id := fmt.Sprintf("sa_%d", uiCore.nextID)
		uiCore.nextID++
		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}

		scrollOff := float32(uiCore.listScroll[id])
		originalOff := scrollOff
		maxScroll := fc - fh
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollOff > maxScroll {
			scrollOff = maxScroll
		}
		in := uiInput()
		hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+fh
		if hovered && in.scrollY != 0 {
			scrollOff += float32(in.scrollY)
			if scrollOff < 0 {
				scrollOff = 0
			}
			if scrollOff > maxScroll {
				scrollOff = maxScroll
			}
			uiCore.listScroll[id] = int(scrollOff)
		}
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, uiCore.theme.inputBg)
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
		thumbExtent := float32(1.0)
		if fc > 0 {
			thumbExtent = fh / fc
			if thumbExtent > 1 {
				thumbExtent = 1
			}
		}
		scrollOff = drawScrollbar(id, fx, fy, fw, fh, scrollOff, maxScroll, thumbExtent)
		uiCore.listScroll[id] = int(scrollOff)
		uiRegisterElement(id, fx, fy, fw, fh)
		if scrollOff != originalOff && UiEventHookActive() {
			FireUiEventHook("scroll", "scrollArea", "", &Float{Value: float64(scrollOff)}, int(in.mouseX), int(in.mouseY))
		}
		return &Float{Value: float64(scrollOff)}
	}}

	// list — draw a scrollable single-select list, returning the selected item.
	//
	// Shows items in a scrollable box with mouse wheel and keyboard navigation
	// (Up/Down to move, Enter to confirm); click to select. Selection and scroll
	// are tracked internally per list. Returns the currently selected element, or
	// null when the list is empty.
	//
	// @sig     list(label: string, items: array, x: int, y: int, w: int, h: int, [size: number]) -> any
	// @param   label  caption drawn above the box ("" for none)
	// @param   items  array of items to show (strings, or any value via its display form)
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   w      width in pixels
	// @param   h      height in pixels
	// @param   size   text scale (default 0.5)
	// @returns the selected item, or null if the list is empty
	// @errors  TypeError if label isn't a string, items isn't an array, or x/y/w/h aren't integers; RuntimeError unless given 6 or 7 arguments
	// @example no-run choice = list("Files", names, 20, 40, 200, 160)
	// @since   0.1.0
	// @see     listMulti, dropdown, scrollArea
	Builtins["list"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("list expects 6-7 arguments: label, items, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		if !ok1 || !ok2 {
			return typeError("list: label must be string, items must be array", ast.Pos{})
		}
		coords, err := requireIntArgs("list", "x/y/w/h must be integers", args[2:6])
		if err != nil {
			return err
		}
		scale, err := extractScale("list", args, 6, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("list_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		// Row height scales with the active font so larger text doesn't
		// crowd the row. Add spacingS top + bottom for breathing room.
		itemH := activeRenderer.lineHeight(scale) + uiCore.style.spacingS*2
		n := len(itemsObj.Elements)
		if n == 0 {
			// Empty state — still draw the container so layout
			// doesn't collapse.
			if label.Value != "" {
				activeRenderer.drawText(label.Value, int(fx), int(fy-20), false, scale, uiCore.theme.labelText)
			}
			activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, uiCore.theme.inputBg)
			activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
			drawEmptyState(fx, fy, fw, fh, "No items")
			uiRegisterElement(id, fx, fy, fw, fh)
			return NULL
		}
		visible := int(fh / itemH)
		if visible < 1 {
			visible = 1
		}

		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		scrollPos := uiCore.listScroll[id]
		selectedIdx := uiCore.listSelected[id]
		if selectedIdx >= n {
			selectedIdx = 0
		}
		originalSel := selectedIdx

		disabled := uiDisabled()
		in := uiInput()
		hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+fh && !disabled
		focused := uiCore.activeID == id && !disabled
		enterFired := false
		if hovered && in.scrollY != 0 {
			step := 1
			if in.scrollY < 0 {
				step = -1
			}
			scrollPos += step
			if scrollPos < 0 {
				scrollPos = 0
			}
			if scrollPos > n-visible {
				scrollPos = n - visible
			}
			if scrollPos < 0 {
				scrollPos = 0
			}
			uiCore.listScroll[id] = scrollPos
		}

		// Keyboard nav: list takes focus on click; Up/Down move
		// selection (with key-repeat), Enter fires a "select" event.
		if focused {
			for i := 0; i < in.upCount; i++ {
				if selectedIdx > 0 {
					selectedIdx--
				}
			}
			for i := 0; i < in.downCount; i++ {
				if selectedIdx < n-1 {
					selectedIdx++
				}
			}
			if in.enterPressed {
				enterFired = true
			}
			// Auto-scroll to keep selection visible.
			if selectedIdx < scrollPos {
				scrollPos = selectedIdx
			}
			if selectedIdx >= scrollPos+visible {
				scrollPos = selectedIdx - visible + 1
			}
			uiCore.listScroll[id] = scrollPos
			uiCore.listSelected[id] = selectedIdx
		}

		if label.Value != "" {
			activeRenderer.drawText(label.Value, int(fx), int(fy-20), false, scale, fadeIfDisabled(uiCore.theme.labelText))
		}
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, fadeIfDisabled(uiCore.theme.inputBg))
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))
		if focused {
			drawFocusRing(fx, fy, fw, fh, uiCore.style.radiusMedium)
		}

		listW := fw - 15
		lh := activeRenderer.lineHeight(scale)
		activeRenderer.pushClip(fx, fy, fw, fh)
		for i := 0; i < visible && scrollPos+i < n; i++ {
			idx := scrollPos + i
			iy := fy + float32(i)*itemH
			itemText := ""
			if s, ok := itemsObj.Elements[idx].(*String); ok {
				itemText = s.Value
			} else {
				itemText = itemsObj.Elements[idx].Inspect()
			}
			isOver := in.mouseX >= fx && in.mouseX <= fx+listW && in.mouseY >= iy && in.mouseY <= iy+itemH && !disabled
			if isOver && in.mouseClicked {
				selectedIdx = idx
				uiCore.listSelected[id] = idx
				uiCore.activeID = id // focus list for keyboard nav
			}
			bg := uiCore.theme.track
			textCol := uiCore.theme.dimText
			switch {
			case idx == selectedIdx:
				bg = uiCore.theme.accentBg
				textCol = uiCore.theme.widgetText
			case isOver:
				bg = uiCore.theme.widgetBg
				textCol = uiCore.theme.labelText
			}
			activeRenderer.fillRoundedRect(fx, iy, listW, itemH, 2, fadeIfDisabled(bg))
			activeRenderer.drawText(itemText, int(fx+8), int(iy+(itemH-lh)*0.5), false, scale, fadeIfDisabled(textCol))
			uiRegisterElement(fmt.Sprintf("%s_item_%d", id, idx), fx, iy, listW, itemH)
		}
		activeRenderer.popClip()

		if enterFired && UiEventHookActive() && selectedIdx >= 0 && selectedIdx < n {
			FireUiEventHook("select", "list", label.Value, itemsObj.Elements[selectedIdx], int(in.mouseX), int(in.mouseY))
		}

		// Scrollbar.
		if n > visible {
			thumbExtent := float32(visible) / float32(n)
			maxScroll := float32(n - visible)
			newOff := drawScrollbar(id, fx, fy, fw, fh, float32(scrollPos), maxScroll, thumbExtent)
			if int(newOff) != scrollPos {
				scrollPos = int(newOff)
				uiCore.listScroll[id] = scrollPos
			}
		}
		uiRegisterElement(id, fx, fy, fw, fh)

		if selectedIdx != originalSel && UiEventHookActive() && selectedIdx >= 0 && selectedIdx < n {
			FireUiEventHook("select", "list", label.Value, itemsObj.Elements[selectedIdx], int(in.mouseX), int(in.mouseY))
		}
		if selectedIdx >= 0 && selectedIdx < n {
			return itemsObj.Elements[selectedIdx]
		}
		return NULL
	}}

	// listMulti — draw a scrollable multi-select list, returning the selection array.
	//
	// Like list but each row has a checkbox. Pass a parallel bool array marking
	// which items are selected, and store the array it returns — clicking a row
	// toggles its flag.
	//
	// @sig     listMulti(label: string, items: array, selected: array, x: int, y: int, w: int, h: int, [size: number]) -> array
	// @param   label     caption above the box ("" for none)
	// @param   items     array of items to show
	// @param   selected  parallel array of bools marking selected items
	// @param   x         left position in pixels
	// @param   y         top position in pixels
	// @param   w         width in pixels
	// @param   h         height in pixels
	// @param   size      text scale (default 0.5)
	// @returns the updated array of selection bools (one per item)
	// @errors  TypeError if label isn't a string, items/selected aren't arrays, or x/y/w/h aren't integers; RuntimeError unless given 7 or 8 arguments
	// @example no-run picks = listMulti("Tags", tags, picks, 20, 40, 200, 160)
	// @since   0.1.0
	// @see     list, checkbox
	Builtins["listMulti"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("listMulti expects 7-8 arguments: label, items, selected, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		selObj, ok3 := args[2].(*Array)
		if !ok1 || !ok2 || !ok3 {
			return typeError("listMulti: label must be string, items + selected must be arrays", ast.Pos{})
		}
		coords, err := requireIntArgs("listMulti", "x/y/w/h must be integers", args[3:7])
		if err != nil {
			return err
		}
		scale, err := extractScale("listMulti", args, 7, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("listm_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		itemH := activeRenderer.lineHeight(scale) + uiCore.style.spacingS*2
		n := len(itemsObj.Elements)
		if n == 0 {
			if label.Value != "" {
				activeRenderer.drawText(label.Value, int(fx), int(fy-20), false, scale, uiCore.theme.labelText)
			}
			activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, uiCore.theme.inputBg)
			activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
			drawEmptyState(fx, fy, fw, fh, "No items")
			return selObj
		}
		// Normalise selected array length.
		sel := make([]bool, n)
		for i := 0; i < n && i < len(selObj.Elements); i++ {
			if b, ok := selObj.Elements[i].(*Boolean); ok {
				sel[i] = b.Value
			}
		}
		visible := int(fh / itemH)
		if visible < 1 {
			visible = 1
		}

		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		scrollPos := uiCore.listScroll[id]

		disabled := uiDisabled()
		in := uiInput()
		hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+fh && !disabled
		focused := uiCore.activeID == id && !disabled
		// Internal cursor (for keyboard nav). Stored alongside listScroll.
		cursorKey := id + "_cur"
		cursor := uiCore.listSelected[cursorKey]
		if cursor >= n {
			cursor = 0
		}
		if hovered && in.scrollY != 0 {
			step := 1
			if in.scrollY < 0 {
				step = -1
			}
			scrollPos += step
			if scrollPos < 0 {
				scrollPos = 0
			}
			if scrollPos > n-visible {
				scrollPos = n - visible
			}
			if scrollPos < 0 {
				scrollPos = 0
			}
			uiCore.listScroll[id] = scrollPos
		}

		// Keyboard nav: Up/Down move the cursor, Enter toggles the row.
		if focused {
			for i := 0; i < in.upCount; i++ {
				if cursor > 0 {
					cursor--
				}
			}
			for i := 0; i < in.downCount; i++ {
				if cursor < n-1 {
					cursor++
				}
			}
			if in.enterPressed {
				sel[cursor] = !sel[cursor]
				changedKB := true
				_ = changedKB
			}
			if cursor < scrollPos {
				scrollPos = cursor
			}
			if cursor >= scrollPos+visible {
				scrollPos = cursor - visible + 1
			}
			uiCore.listScroll[id] = scrollPos
			uiCore.listSelected[cursorKey] = cursor
		}

		if label.Value != "" {
			activeRenderer.drawText(label.Value, int(fx), int(fy-20), false, scale, fadeIfDisabled(uiCore.theme.labelText))
		}
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, fadeIfDisabled(uiCore.theme.inputBg))
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))
		if focused {
			drawFocusRing(fx, fy, fw, fh, uiCore.style.radiusMedium)
		}

		listW := fw - 15
		lh := activeRenderer.lineHeight(scale)
		activeRenderer.pushClip(fx, fy, fw, fh)
		changed := false
		for i := 0; i < visible && scrollPos+i < n; i++ {
			idx := scrollPos + i
			iy := fy + float32(i)*itemH
			itemText := ""
			if s, ok := itemsObj.Elements[idx].(*String); ok {
				itemText = s.Value
			} else {
				itemText = itemsObj.Elements[idx].Inspect()
			}
			isOver := in.mouseX >= fx && in.mouseX <= fx+listW && in.mouseY >= iy && in.mouseY <= iy+itemH && !disabled
			if isOver && in.mouseClicked {
				sel[idx] = !sel[idx]
				changed = true
				cursor = idx
				uiCore.activeID = id
				uiCore.listSelected[cursorKey] = cursor
			}
			bg := uiCore.theme.track
			textCol := uiCore.theme.dimText
			if sel[idx] {
				bg = uiCore.theme.accentBg
				textCol = uiCore.theme.widgetText
				// Left strip accent.
				activeRenderer.fillRoundedRect(fx, iy, 3, itemH, 0, uiCore.theme.accent)
			} else if idx == cursor && focused {
				// Keyboard cursor highlight (subtler than full selection).
				bg = uiCore.theme.widgetBg
				textCol = uiCore.theme.widgetText
			} else if isOver {
				bg = uiCore.theme.widgetBg
				textCol = uiCore.theme.labelText
			}
			activeRenderer.fillRoundedRect(fx, iy, listW, itemH, 2, fadeIfDisabled(bg))
			if sel[idx] {
				activeRenderer.fillRoundedRect(fx, iy, 3, itemH, 0, fadeIfDisabled(uiCore.theme.accent))
			}
			activeRenderer.drawText(itemText, int(fx+8), int(iy+(itemH-lh)*0.5), false, scale, fadeIfDisabled(textCol))
			uiRegisterElement(fmt.Sprintf("%s_item_%d", id, idx), fx, iy, listW, itemH)
		}
		activeRenderer.popClip()

		if n > visible {
			thumbExtent := float32(visible) / float32(n)
			maxScroll := float32(n - visible)
			newOff := drawScrollbar(id, fx, fy, fw, fh, float32(scrollPos), maxScroll, thumbExtent)
			if int(newOff) != scrollPos {
				scrollPos = int(newOff)
				uiCore.listScroll[id] = scrollPos
			}
		}
		uiRegisterElement(id, fx, fy, fw, fh)

		out := make([]Object, n)
		for i, v := range sel {
			out[i] = &Boolean{Value: v}
		}
		if changed && UiEventHookActive() {
			FireUiEventHook("toggle", "listMulti", label.Value, NULL, int(in.mouseX), int(in.mouseY))
		}
		return &Array{Elements: out}
	}}

	// treeView — draw a collapsible tree, returning the selection and expand state.
	//
	// The tree is a flat list: labels[i] at indent levels[i], with expanded[i] a
	// bool for whether that node is open. Clicking a row's twisty toggles its
	// expanded flag; clicking the label selects it. Returns a two-element
	// [selectedIdx, expanded] tuple — feed `expanded` back in next frame.
	//
	// @sig     treeView(x: int, y: int, w: int, h: int, labels: array, levels: array, expanded: array, [size: number]) -> tuple
	// @param   x         left position in pixels
	// @param   y         top position in pixels
	// @param   w         width in pixels
	// @param   h         height in pixels
	// @param   labels    array of node label strings
	// @param   levels    parallel array of integer indent levels
	// @param   expanded  parallel array of bools for each node's open state
	// @param   size      text scale (default 0.5)
	// @returns a [selectedIndex, expanded] tuple — pass expanded back next frame
	// @errors  TypeError if x/y/w/h aren't integers or labels/levels/expanded aren't arrays; RuntimeError unless given 7 or 8 arguments
	// @example no-run sel, expanded = treeView(20, 20, 240, 300, labels, levels, expanded)
	// @since   0.1.0
	// @see     accordion, list
	Builtins["treeView"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("treeView expects 7-8 arguments: x, y, w, h, labels, levels, expanded, [size]", ast.Pos{})
		}
		coords, err := requireIntArgs("treeView", "x/y/w/h must be integers", args[0:4])
		if err != nil {
			return err
		}
		labels, ok1 := args[4].(*Array)
		levels, ok2 := args[5].(*Array)
		expandedArr, ok3 := args[6].(*Array)
		if !ok1 || !ok2 || !ok3 {
			return typeError("treeView: labels/levels/expanded must be arrays", ast.Pos{})
		}
		scale, err := extractScale("treeView", args, 7, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("tv_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		n := len(labels.Elements)
		rowH := activeRenderer.lineHeight(scale) + uiCore.style.spacingS*1.5

		// Make a defensive copy of expanded[] padded to n.
		newExpanded := make([]Object, n)
		for i := 0; i < n; i++ {
			if i < len(expandedArr.Elements) {
				newExpanded[i] = expandedArr.Elements[i]
			} else {
				newExpanded[i] = FALSE
			}
		}
		getLevel := func(i int) int {
			if i < len(levels.Elements) {
				if v, ok := levels.Elements[i].(*Integer); ok {
					return v.Value
				}
			}
			return 0
		}
		nodeExpanded := func(i int) bool {
			if b, ok := newExpanded[i].(*Boolean); ok {
				return b.Value
			}
			return false
		}

		// Compute visible rows honouring expanded/collapsed state.
		visible := []int{}
		skipUntilLevel := -1
		for i := 0; i < n; i++ {
			lvl := getLevel(i)
			if skipUntilLevel >= 0 && lvl > skipUntilLevel {
				continue
			}
			skipUntilLevel = -1
			visible = append(visible, i)
			if !nodeExpanded(i) {
				skipUntilLevel = lvl
			}
		}

		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		scrollPos := uiCore.listScroll[id]
		selected := uiCore.listSelected[id]
		originalSel := selected
		visN := len(visible)
		visibleRows := int(fh / rowH)
		if visibleRows < 1 {
			visibleRows = 1
		}

		in := uiInput()
		hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+fh
		if hovered && in.scrollY != 0 {
			step := 1
			if in.scrollY < 0 {
				step = -1
			}
			scrollPos += step
			if scrollPos < 0 {
				scrollPos = 0
			}
			if scrollPos > visN-visibleRows {
				scrollPos = visN - visibleRows
			}
			if scrollPos < 0 {
				scrollPos = 0
			}
			uiCore.listScroll[id] = scrollPos
		}

		activeRenderer.fillRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, uiCore.theme.inputBg)
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
		if n == 0 {
			drawEmptyState(fx, fy, fw, fh, "No items")
			uiRegisterElement(id, fx, fy, fw, fh)
			return &Array{Elements: []Object{&Integer{Value: -1}, expandedArr}}
		}
		lhTV := activeRenderer.lineHeight(scale)
		activeRenderer.pushClip(fx, fy, fw, fh)
		toggleIdx := -1
		for r := 0; r < visibleRows && scrollPos+r < visN; r++ {
			i := visible[scrollPos+r]
			ry := fy + float32(r)*rowH
			lvl := getLevel(i)
			indent := float32(8 + lvl*16)

			lbl := ""
			if s, ok := labels.Elements[i].(*String); ok {
				lbl = s.Value
			}

			// Detect children (next node has a deeper level).
			hasChildren := false
			if i+1 < n {
				if getLevel(i+1) > lvl {
					hasChildren = true
				}
			}

			isOver := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= ry && in.mouseY <= ry+rowH
			if isOver && in.mouseClicked {
				selected = i
				if hasChildren {
					toggleIdx = i
				}
			}

			bg := uiCore.theme.inputBg
			textCol := uiCore.theme.labelText
			switch {
			case i == selected:
				bg = uiCore.theme.accentBg
				textCol = uiCore.theme.widgetText
			case isOver:
				bg = uiCore.theme.widgetBg
				textCol = uiCore.theme.widgetText
			}
			activeRenderer.fillRoundedRect(fx, ry, fw-15, rowH, 0, bg)

			sym := " "
			if hasChildren {
				if nodeExpanded(i) {
					sym = "v"
				} else {
					sym = ">"
				}
			}
			activeRenderer.drawText(sym, int(fx+indent-12), int(ry+(rowH-lhTV)*0.5), false, scale, uiCore.theme.accent)
			activeRenderer.drawText(lbl, int(fx+indent), int(ry+(rowH-lhTV)*0.5), false, scale, textCol)
		}
		activeRenderer.popClip()

		if toggleIdx >= 0 {
			if b, ok := newExpanded[toggleIdx].(*Boolean); ok {
				newExpanded[toggleIdx] = &Boolean{Value: !b.Value}
			} else {
				newExpanded[toggleIdx] = TRUE
			}
		}
		uiCore.listSelected[id] = selected

		if visN > visibleRows {
			thumbExtent := float32(visibleRows) / float32(visN)
			maxScroll := float32(visN - visibleRows)
			newOff := drawScrollbar(id, fx, fy, fw, fh, float32(scrollPos), maxScroll, thumbExtent)
			if int(newOff) != scrollPos {
				scrollPos = int(newOff)
				uiCore.listScroll[id] = scrollPos
			}
		}
		uiRegisterElement(id, fx, fy, fw, fh)

		if selected != originalSel && UiEventHookActive() && selected >= 0 && selected < n {
			lbl := ""
			if s, ok := labels.Elements[selected].(*String); ok {
				lbl = s.Value
			}
			FireUiEventHook("select", "treeView", lbl, &Integer{Value: selected}, int(in.mouseX), int(in.mouseY))
		}

		return &Array{Elements: []Object{&Integer{Value: selected}, &Array{Elements: newExpanded}}}
	}}

	// ════════════════════════════════════════════════════════════════
	// Phase 3 — popup machinery (dropdown / tooltip / toast)
	// ════════════════════════════════════════════════════════════════

	// dropdown — draw a dropdown select, returning the chosen item.
	//
	// The header shows the current selection (passed as `label`); clicking opens a
	// popup menu, which is drawn last so it overlays everything. Returns the chosen
	// item's text when a new one is picked, otherwise the current label.
	//
	// @sig     dropdown(label: string, items: array, x: int, y: int, w: int, [size: number]) -> string
	// @param   label  the currently selected text (shown in the header)
	// @param   items  array of selectable item strings
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   w      width in pixels
	// @param   size   text scale (default 0.5)
	// @returns the selected item's text (the new pick, or the unchanged label)
	// @errors  TypeError if label isn't a string or items isn't an array; RuntimeError unless given 5 or 6 arguments
	// @example no-run mode = dropdown(mode, ["Fast", "Pretty"], 20, 20, 140)
	// @since   0.1.0
	// @see     list, radio, contextMenu
	Builtins["dropdown"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("dropdown expects 5-6 arguments: label, items, x, y, w, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		if !ok1 || !ok2 {
			return typeError("dropdown: label must be string, items must be array", ast.Pos{})
		}
		coords, err := requireIntArgs("dropdown", "x/y/w must be integers", args[2:5])
		if err != nil {
			return err
		}
		scale, err := extractScale("dropdown", args, 5, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("dd_%d", uiCore.nextID)
		uiCore.nextID++
		n := len(itemsObj.Elements)
		if n == 0 {
			return &String{Value: ""}
		}
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		selectedIdx := uiCore.listSelected[id]
		if selectedIdx >= n {
			selectedIdx = 0
		}
		originalIdx := selectedIdx

		fx, fy, fw := float32(coords[0]), float32(coords[1]), float32(coords[2])
		const headerH = float32(32)
		const itemH = float32(26)
		charH := activeRenderer.lineHeight(scale) // matches the actual rendered text height

		// Auto-size: if any item is wider than the requested w, grow the
		// header to match — keeps the longest entry from clipping.
		const ddPad = float32(28)
		for _, item := range itemsObj.Elements {
			if s, ok := item.(*String); ok {
				if want := activeRenderer.textWidth(s.Value, scale) + ddPad; want > fw {
					fw = want
				}
			}
		}

		disabled := uiDisabled()
		isOpen := uiCore.dropdownOpen == id && !disabled
		hovered := uiCore.hoveredID == id && !disabled

		in := uiInput()
		// Decide popup placement — flip upward when overflowing the window bottom.
		popupMenuH := float32(n) * itemH
		popupMenuY := fy + headerH
		if popupMenuY+popupMenuH > float32(in.winH) {
			popupMenuY = fy - popupMenuH
			if popupMenuY < 0 {
				popupMenuY = 0
			}
		}

		// Toggle on header click; close on click outside while open.
		if !disabled && hovered && in.mouseClicked {
			if isOpen {
				uiCore.dropdownOpen = ""
				isOpen = false
			} else {
				uiCore.dropdownOpen = id
				isOpen = true
			}
		} else if isOpen && in.mouseClicked {
			inHeader := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+headerH
			inMenu := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= popupMenuY && in.mouseY <= popupMenuY+popupMenuH
			if !inHeader && !inMenu {
				uiCore.dropdownOpen = ""
				isOpen = false
			}
		}

		// Optional label above the header — back off by the actual line
		// height + 4 px clearance so descenders don't kiss the box top.
		if label.Value != "" {
			activeRenderer.drawText(label.Value, int(fx), int(fy-charH-4), false, scale, uiCore.theme.labelText)
		}

		// Header.
		headerColor := uiCore.theme.widgetBg
		if hovered {
			headerColor = uiCore.theme.widgetBgHover
		}
		if isOpen {
			headerColor = uiCore.theme.widgetBgActive
		}
		ddR := uiCore.style.radiusMedium
		activeRenderer.fillRoundedRect(fx, fy, fw, headerH, ddR, fadeIfDisabled(headerColor))
		activeRenderer.strokeRoundedRect(fx, fy, fw, headerH, ddR, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))

		selText := ""
		if selectedIdx < n {
			if s, ok := itemsObj.Elements[selectedIdx].(*String); ok {
				selText = s.Value
			} else {
				selText = itemsObj.Elements[selectedIdx].Inspect()
			}
		}
		lhDD := activeRenderer.lineHeight(scale)
		ddTextY := int(fy + (headerH-lhDD)*0.5)
		activeRenderer.drawText(selText, int(fx+8), ddTextY, false, scale, fadeIfDisabled(uiCore.theme.widgetText))
		activeRenderer.drawText("v", int(fx+fw-16), ddTextY, false, scale, fadeIfDisabled(uiCore.theme.dimText))
		uiRegisterElement(id, fx, fy, fw, headerH)

		// When open, handle item clicks inline AND queue popup draw for uiEnd.
		if isOpen {
			for i := 0; i < n; i++ {
				iy := popupMenuY + float32(i)*itemH
				isItemHovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= iy && in.mouseY <= iy+itemH
				if isItemHovered && in.mouseClicked {
					selectedIdx = i
					uiCore.listSelected[id] = i
					uiCore.dropdownOpen = ""
					isOpen = false
				}
			}
			if isOpen {
				items := make([]string, n)
				for i := 0; i < n; i++ {
					if s, ok := itemsObj.Elements[i].(*String); ok {
						items[i] = s.Value
					} else {
						items[i] = itemsObj.Elements[i].Inspect()
					}
				}
				uiCore.pendingDropdown = pendingDropdown{
					active:      true,
					id:          id,
					fx:          fx,
					fy:          popupMenuY,
					fw:          fw,
					items:       items,
					selectedIdx: selectedIdx,
					charH:       charH,
					textScale:   scale,
				}
			}
		}

		if selectedIdx != originalIdx && UiEventHookActive() {
			picked := itemsObj.Elements[selectedIdx]
			FireUiEventHook("select", "dropdown", label.Value, picked, int(in.mouseX), int(in.mouseY))
		}
		if selectedIdx >= 0 && selectedIdx < n {
			return itemsObj.Elements[selectedIdx]
		}
		return &String{Value: ""}
	}}

	// tooltip — attach hover text to the widget drawn just before this call.
	//
	// Call immediately after a widget; the tooltip appears once the cursor has
	// rested on that widget for ~0.5s. The popup itself is drawn on top in uiEnd.
	//
	// @sig     tooltip(text: string) -> null
	// @param   text  the tooltip text for the preceding widget
	// @returns null
	// @errors  TypeError unless given a single string argument
	// @example no-run button("Save", 20, 20, 80, 30)  tooltip("Write to disk (Ctrl+S)")
	// @since   0.1.0
	// @see     modal, toast
	Builtins["tooltip"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("tooltip expects 1 argument: text", ast.Pos{})
		}
		txt, ok := args[0].(*String)
		if !ok {
			return typeError("tooltip: argument must be a string", ast.Pos{})
		}
		// Only fire when the cursor is over the most recently registered widget.
		if uiCore.lastElementID == "" || uiCore.hoveredID != uiCore.lastElementID {
			return NULL
		}
		uiCore.tooltipMatchedThisFrame = true
		now := time.Since(uiCore.startTime).Seconds()
		if uiCore.tooltipHoveredID != uiCore.lastElementID {
			uiCore.tooltipHoveredID = uiCore.lastElementID
			uiCore.tooltipHoverStart = now
			return NULL
		}
		if now-uiCore.tooltipHoverStart >= 0.5 {
			in := uiInput()
			uiCore.pendingTooltip = pendingTooltip{
				active: true,
				text:   txt.Value,
				mx:     in.mouseX,
				my:     in.mouseY,
			}
		}
		return NULL
	}}

	// contextMenu — draw a right-click popup menu, returning the chosen index.
	//
	// You own the `visible` flag (typically set true on a right-click and stored
	// with the menu position). Returns the clicked item index, -1 if nothing was
	// clicked, or -2 on an outside click — treat -2 as "dismiss" and set visible
	// false. When visible is false it draws nothing and returns -1.
	//
	// @sig     contextMenu(x: int, y: int, items: array, visible: bool, [size: number]) -> int
	// @param   x        menu left position in pixels
	// @param   y        menu top position in pixels
	// @param   items    array of menu item strings
	// @param   visible  whether the menu is currently shown
	// @param   size     text scale (default 0.5)
	// @returns the clicked item index, -1 if none, or -2 on an outside (dismiss) click
	// @errors  TypeError if x/y aren't integers, items isn't an array, or visible isn't a bool; RuntimeError unless given 4 or 5 arguments
	// @example no-run pick = contextMenu(menuX, menuY, ["Cut", "Copy", "Paste"], menuOpen)
	// @since   0.1.0
	// @see     dropdown, modal
	Builtins["contextMenu"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return typeError("contextMenu expects 4-5 arguments: x, y, items, visible, [size]", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		items, ok3 := args[2].(*Array)
		visibleObj, ok4 := args[3].(*Boolean)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("contextMenu: x/y must be integers, items must be array, visible must be bool", ast.Pos{})
		}
		if !visibleObj.Value {
			return &Integer{Value: -1}
		}
		scale, err := extractScale("contextMenu", args, 4, 0.5)
		if err != nil {
			return err
		}
		n := len(items.Elements)
		if n == 0 {
			return &Integer{Value: -1}
		}

		in := uiInput()
		// Detect first frame so the opening click doesn't immediately
		// trigger the outside-dismiss path.
		isFirstFrame := uiCore.menuOpenFrame < in.frameCount-1
		uiCore.menuOpenFrame = in.frameCount

		// Auto-size to widest item label.
		menuW := float32(120)
		for _, item := range items.Elements {
			if s, ok := item.(*String); ok {
				w := activeRenderer.textWidth(s.Value, scale) + 24
				if w > menuW {
					menuW = w
				}
			}
		}
		itemH := scale*20 + 10
		fx, fy := float32(x.Value), float32(y.Value)
		menuH := itemH*float32(n) + 4

		// Drop shadow + body + outline.
		ctxR := uiCore.style.radiusMedium
		shadow := uiCore.theme.shadow
		shadow[3] = 0.55
		activeRenderer.dropShadow(fx, fy, menuW, menuH, ctxR, 4, 12, shadow)
		menuBg := uiCore.theme.widgetBgActive
		menuBg[3] = 0.97
		activeRenderer.fillRoundedRect(fx, fy, menuW, menuH, ctxR, menuBg)
		activeRenderer.strokeRoundedRect(fx, fy, menuW, menuH, ctxR, 0.75, uiCore.theme.accent)

		result := -1
		lhCtx := activeRenderer.lineHeight(scale)
		for i, item := range items.Elements {
			lbl := ""
			if s, ok := item.(*String); ok {
				lbl = s.Value
			}
			iy := fy + 2 + float32(i)*itemH
			isOver := in.mouseX >= fx && in.mouseX <= fx+menuW && in.mouseY >= iy && in.mouseY <= iy+itemH

			textCol := uiCore.theme.labelText
			if isOver {
				activeRenderer.fillRoundedRect(fx+2, iy, menuW-4, itemH, 3, uiCore.theme.widgetBg)
				textCol = uiCore.theme.widgetText
				if in.mouseClicked {
					result = i
				}
			}
			activeRenderer.drawText(lbl, int(fx+10), int(iy+(itemH-lhCtx)*0.5), false, scale, textCol)
		}
		if in.mouseClicked && result == -1 && !isFirstFrame {
			outside := in.mouseX < fx || in.mouseX > fx+menuW || in.mouseY < fy || in.mouseY > fy+menuH
			if outside {
				result = -2
			}
		}
		uiRegisterElement(fmt.Sprintf("ctx_%d", uiCore.nextID), fx, fy, menuW, menuH)
		if result >= 0 && UiEventHookActive() {
			pickedLabel := ""
			if s, ok := items.Elements[result].(*String); ok {
				pickedLabel = s.Value
			}
			FireUiEventHook("select", "contextMenu", pickedLabel, &Integer{Value: result}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: result}
	}}

	// getTypedChars — the printable characters typed this frame.
	//
	// Returns the text entered since the last frame (Unicode-safe), or "" if
	// nothing was typed. The building block for custom text entry; the built-in
	// textInput/textArea widgets consume this for you.
	//
	// @sig     getTypedChars() -> string
	// @returns the characters typed this frame ("" if none)
	// @errors  RuntimeError if called with any arguments
	// @example no-run buffer = buffer + getTypedChars()
	// @since   0.1.0
	// @see     textInput, textArea, keyPressed
	Builtins["getTypedChars"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("getTypedChars expects no arguments", ast.Pos{})
		}
		return &String{Value: uiInput().typedChars}
	}}

	// lineHeight — the pixel height of one line of widget text at a given scale.
	//
	// Lets you vertically centre text by hand, e.g.
	// label(s, x, y + (h - lineHeight()) / 2) drops a label dead-centre in an
	// h-tall box. Scale defaults to 0.5 (the widget default).
	//
	// @sig     lineHeight([scale: number]) -> int
	// @param   scale  text scale to measure at (default 0.5)
	// @returns one line's height in pixels at that scale
	// @errors  TypeError if scale isn't numeric; RuntimeError if given more than 1 argument
	// @example no-run h = lineHeight()
	// @since   0.1.0
	// @see     label, text, fontCharHeight
	Builtins["lineHeight"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) > 1 {
			return typeError("lineHeight expects 0 or 1 arguments: [scale]", ast.Pos{})
		}
		var scale float32 = 0.5
		if len(args) == 1 {
			switch v := args[0].(type) {
			case *Float:
				scale = float32(v.Value)
			case *Integer:
				scale = float32(v.Value)
			default:
				return typeError("lineHeight: scale must be a number", ast.Pos{})
			}
		}
		return &Integer{Value: int(activeRenderer.lineHeight(scale) + 0.5)}
	}}

	// colorPicker — draw four RGBA sliders with a preview, returning the colour.
	//
	// Immediate-mode: pass the current r, g, b, a (each 0.0–1.0) and store the
	// returned [r, g, b, a] array — dragging a channel slider updates that
	// component. A live swatch shows the result.
	//
	// @sig     colorPicker(x: int, y: int, w: int, r: number, g: number, b: number, a: number) -> array
	// @param   x  left position in pixels
	// @param   y  top position in pixels
	// @param   w  slider width in pixels
	// @param   r  current red (0.0–1.0)
	// @param   g  current green (0.0–1.0)
	// @param   b  current blue (0.0–1.0)
	// @param   a  current alpha (0.0–1.0)
	// @returns the [r, g, b, a] colour after this frame
	// @errors  TypeError if x/y/w aren't integers or r/g/b/a aren't numbers; RuntimeError unless given 7 arguments
	// @example no-run col = colorPicker(20, 40, 200, col[0], col[1], col[2], col[3])
	// @since   0.1.0
	// @see     slider, makeTheme
	Builtins["colorPicker"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 7 {
			return typeError("colorPicker expects 7 arguments: x, y, w, r, g, b, a", ast.Pos{})
		}
		coords, err := requireIntArgs("colorPicker", "x/y/w must be integers", args[0:3])
		if err != nil {
			return err
		}
		readF := func(o Object) (float64, bool) {
			switch v := o.(type) {
			case *Float:
				return v.Value, true
			case *Integer:
				return float64(v.Value), true
			}
			return 0, false
		}
		r, ok1 := readF(args[3])
		g, ok2 := readF(args[4])
		b, ok3 := readF(args[5])
		a, ok4 := readF(args[6])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("colorPicker: r/g/b/a must be numbers", ast.Pos{})
		}

		baseID := uiCore.nextID
		uiCore.nextID += 5
		fx, fy, fw := float32(coords[0]), float32(coords[1]), float32(coords[2])
		const trackH = float32(5)
		const handleR = float32(7)
		const scale = float32(0.5)
		lh := activeRenderer.lineHeight(scale)
		gap := lh + 28

		in := uiInput()
		// cpSlider draws one channel slider and returns the (possibly updated)
		// value. Drag detection: any frame the left mouse is held and the
		// cursor is in the row's vertical band, the value follows the mouse.
		cpSlider := func(idN int, lbl string, value float64, fillCol [4]float32, sy float32) float64 {
			trackY := sy + handleR - trackH*0.5
			id := fmt.Sprintf("cp_%d_%d", baseID, idN)
			hit := in.mouseY >= sy && in.mouseY <= sy+handleR*2 &&
				in.mouseX >= fx-handleR && in.mouseX <= fx+fw+handleR
			if hit && in.mouseClicked {
				uiCore.activeID = id
			}
			if uiCore.activeID == id && in.mouseDown {
				t := (in.mouseX - fx) / fw
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}
				value = float64(t)
			}
			if !in.mouseDown && uiCore.activeID == id {
				uiCore.activeID = ""
			}
			t := float32(value)
			activeRenderer.drawText(fmt.Sprintf("%s: %.2f", lbl, value), int(fx), int(sy-lh-2), false, scale, uiCore.theme.labelText)
			activeRenderer.fillRoundedRect(fx, trackY, fw, trackH, trackH*0.5, uiCore.theme.track)
			if t > 0.01 {
				activeRenderer.fillRoundedRect(fx, trackY, fw*t, trackH, trackH*0.5, fillCol)
			}
			hx := fx + fw*t
			activeRenderer.fillRoundedRect(hx-handleR, sy, handleR*2, handleR*2, handleR, uiCore.theme.handle)
			uiRegisterElement(id, fx, sy, fw, handleR*2)
			return value
		}

		origR, origG, origB, origA := r, g, b, a
		r = cpSlider(0, "R", r, [4]float32{float32(r), 0.1, 0.1, 1}, fy)
		g = cpSlider(1, "G", g, [4]float32{0.1, float32(g), 0.1, 1}, fy+gap)
		b = cpSlider(2, "B", b, [4]float32{0.1, 0.1, float32(b), 1}, fy+gap*2)
		a = cpSlider(3, "A", a, [4]float32{0.55, 0.55, 0.55, float32(a)}, fy+gap*3)

		swatchY := fy + gap*4
		swatchH := lh + 12
		activeRenderer.fillRoundedRect(fx, swatchY, fw, swatchH, 4, [4]float32{float32(r), float32(g), float32(b), float32(a)})
		activeRenderer.drawText("preview", int(fx+6), int(swatchY+(swatchH-lh)*0.5), false, scale, uiCore.theme.widgetText)

		result := &Array{Elements: []Object{
			&Float{Value: r},
			&Float{Value: g},
			&Float{Value: b},
			&Float{Value: a},
		}}
		if (r != origR || g != origG || b != origB || a != origA) && UiEventHookActive() {
			FireUiEventHook("drag", "colorPicker", "", result, int(in.mouseX), int(in.mouseY))
		}
		return result
	}}

	// toast — queue an ephemeral notification, shown bottom-right.
	//
	// Fire-and-forget: call it once (e.g. on a click) and the message animates in
	// bottom-right, then fades after `duration` seconds — uiEnd renders the active
	// toasts. style ("info", "success", "warn", "error") tints it. Don't call it
	// every frame, or you'll queue a new toast each frame.
	//
	// @sig     toast(message: string, [style: string], [duration: number]) -> null
	// @param   message   the notification text
	// @param   style     "info" (default), "success", "warn", or "error"
	// @param   duration  seconds before it fades (default 3.0)
	// @returns null
	// @errors  TypeError if message/style aren't strings or duration isn't a number; RuntimeError unless given 1 to 3 arguments
	// @example no-run if button("Save", 20, 20, 80, 30) { toast("Saved", "success") }
	// @since   0.1.0
	// @see     modal, tooltip
	Builtins["toast"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 3 {
			return typeError("toast expects 1-3 arguments: message, [style], [duration]", ast.Pos{})
		}
		msg, ok := args[0].(*String)
		if !ok {
			return typeError("toast: message must be a string", ast.Pos{})
		}
		style := "info"
		if len(args) >= 2 {
			s, ok := args[1].(*String)
			if !ok {
				return typeError("toast: style must be a string", ast.Pos{})
			}
			style = s.Value
		}
		dur := 3.0
		if len(args) == 3 {
			switch d := args[2].(type) {
			case *Integer:
				dur = float64(d.Value)
			case *Float:
				dur = d.Value
			default:
				return typeError("toast: duration must be a number", ast.Pos{})
			}
		}
		expiresAt := time.Since(uiCore.startTime).Seconds() + dur
		uiCore.toasts = append(uiCore.toasts, toastEntry{message: msg.Value, style: style, expiresAt: expiresAt})
		return NULL
	}}

	// ════════════════════════════════════════════════════════════════
	// Phase 4 — text editing (textInput + textArea)
	// ════════════════════════════════════════════════════════════════
	//
	// First-pass scope: cursor positioning, typed-char insert,
	// backspace/delete, arrow navigation, Home/End, shift+selection,
	// horizontal scroll, blinking caret. Deferred to a Phase 4.5
	// follow-up: clipboard (async on WASM), undo/redo, word-jump,
	// double-click word selection.

	// ensureTextMaps lazy-initialises the per-widget editor maps in
	// uiCore. Called by textInput / textArea before any read.
	ensureTextMaps := func() {
		if uiCore.textCursor == nil {
			uiCore.textCursor = make(map[string]int)
			uiCore.textAnchor = make(map[string]int)
			uiCore.textScroll = make(map[string]float32)
			uiCore.textBlink = make(map[string]float64)
			uiCore.undoStacks = make(map[string][]string)
			uiCore.redoStacks = make(map[string][]string)
		}
	}

	// pushUndo records `text` onto id's undo stack and clears the redo
	// stack. Capped at 100 entries to bound memory.
	pushUndo := func(id, text string) {
		s := uiCore.undoStacks[id]
		if len(s) > 0 && s[len(s)-1] == text {
			return // dedupe consecutive identical states
		}
		s = append(s, text)
		if len(s) > 100 {
			s = s[len(s)-100:]
		}
		uiCore.undoStacks[id] = s
		uiCore.redoStacks[id] = nil
	}
	// popUndo returns the previous saved state (and pushes `current`
	// onto redo). Returns "" and false if the stack is empty.
	popUndo := func(id, current string) (string, bool) {
		s := uiCore.undoStacks[id]
		if len(s) == 0 {
			return "", false
		}
		prev := s[len(s)-1]
		uiCore.undoStacks[id] = s[:len(s)-1]
		uiCore.redoStacks[id] = append(uiCore.redoStacks[id], current)
		return prev, true
	}
	popRedo := func(id, current string) (string, bool) {
		s := uiCore.redoStacks[id]
		if len(s) == 0 {
			return "", false
		}
		next := s[len(s)-1]
		uiCore.redoStacks[id] = s[:len(s)-1]
		uiCore.undoStacks[id] = append(uiCore.undoStacks[id], current)
		return next, true
	}

	// isWordChar — used by Ctrl/Cmd+Left/Right word-jump.
	isWordChar := func(r rune) bool {
		return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
	// wordJumpLeft / wordJumpRight return the new cursor index after a
	// word-boundary jump from `pos` in `text`.
	wordJumpLeft := func(text string, pos int) int {
		r := []rune(text)
		if pos > len(r) {
			pos = len(r)
		}
		// Skip preceding non-word, then preceding word.
		for pos > 0 && !isWordChar(r[pos-1]) {
			pos--
		}
		for pos > 0 && isWordChar(r[pos-1]) {
			pos--
		}
		return pos
	}
	wordJumpRight := func(text string, pos int) int {
		r := []rune(text)
		// Skip current word, then trailing non-word.
		for pos < len(r) && isWordChar(r[pos]) {
			pos++
		}
		for pos < len(r) && !isWordChar(r[pos]) {
			pos++
		}
		return pos
	}
	// selectionSlice returns the [lo,hi) rune-range of the selection.
	selectionSlice := func(text string, cur, anc int) string {
		lo, hi := cur, anc
		if lo > hi {
			lo, hi = hi, lo
		}
		r := []rune(text)
		if lo < 0 {
			lo = 0
		}
		if hi > len(r) {
			hi = len(r)
		}
		return string(r[lo:hi])
	}

	// deleteSelection returns (newText, newCursor) with the range
	// [min(cur,anc), max(cur,anc)) excised. Rune-safe.
	deleteSelection := func(t string, cur, anc int) (string, int) {
		lo, hi := cur, anc
		if lo > hi {
			lo, hi = hi, lo
		}
		r := []rune(t)
		if lo < 0 {
			lo = 0
		}
		if hi > len(r) {
			hi = len(r)
		}
		return string(append(append([]rune{}, r[:lo]...), r[hi:]...)), lo
	}

	// runeCount returns the number of Unicode codepoints in s.
	runeCount := func(s string) int { return len([]rune(s)) }

	// textInput — draw a single-line editable text field, returning its contents.
	//
	// Immediate-mode: pass the current string and store what it returns. Handles
	// focus on click, caret, typing, backspace/delete, arrow/Home/End navigation,
	// shift-selection, word-jump, undo/redo and horizontal scroll. The label is
	// drawn above the field.
	//
	// @sig     textInput(label: string, currentText: string, x: int, y: int, w: int, h: int, [size: number]) -> string
	// @param   label        caption above the field ("" for none)
	// @param   currentText  the field's current text
	// @param   x            left position in pixels
	// @param   y            top position in pixels
	// @param   w            width in pixels
	// @param   h            height in pixels
	// @param   size         text scale (default 0.5)
	// @returns the field's text after this frame's edits
	// @errors  TypeError if label/currentText aren't strings or x/y/w/h aren't integers; RuntimeError unless given 6 or 7 arguments
	// @example no-run name = textInput("Name", name, 20, 40, 200, 28)
	// @since   0.1.0
	// @see     textArea, uiNextFieldID, getTypedChars
	Builtins["textInput"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("textInput expects 6-7 arguments: label, currentText, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		currentText, ok2 := args[1].(*String)
		if !ok1 || !ok2 {
			return typeError("textInput: label and currentText must be strings", ast.Pos{})
		}
		coords, err := requireIntArgs("textInput", "x/y/w/h must be integers", args[2:6])
		if err != nil {
			return err
		}
		scale, err := extractScale("textInput", args, 6, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("txt_%d", uiCore.nextID)
		uiCore.nextID++
		ensureTextMaps()

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])

		disabled := uiDisabled()
		in := uiInput()
		hovered := uiCore.hoveredID == id && !disabled
		focused := uiCore.activeID == id && !disabled
		now := time.Since(uiCore.startTime).Seconds()

		if disabled {
			// Disabled fields can't be focused or receive input.
			if uiCore.activeID == id {
				uiCore.activeID = ""
			}
		} else {
			// Click in / click out — focus management.
			if hovered && in.mouseClicked {
				uiCore.activeID = id
				focused = true
			} else if !hovered && in.mouseClicked && uiCore.activeID == id {
				uiCore.activeID = ""
				focused = false
			}
		}

		newText := currentText.Value
		cursor := uiCore.textCursor[id]
		anchor := uiCore.textAnchor[id]
		scroll := uiCore.textScroll[id]
		rl := runeCount(newText)
		if cursor > rl {
			cursor = rl
		}
		if anchor > rl {
			anchor = rl
		}

		// xAtCursor estimates the pixel offset of the cursor within the field.
		xAtCursor := func(t string, pos int) float32 {
			r := []rune(t)
			if pos < 0 {
				pos = 0
			}
			if pos > len(r) {
				pos = len(r)
			}
			return activeRenderer.textWidth(string(r[:pos]), scale)
		}
		// cursorFromX is the inverse — picks the closest rune position for px.
		cursorFromX := func(t string, px float32) int {
			r := []rune(t)
			best := 0
			bestDist := float32(1e9)
			for i := 0; i <= len(r); i++ {
				w := activeRenderer.textWidth(string(r[:i]), scale)
				d := w - px
				if d < 0 {
					d = -d
				}
				if d < bestDist {
					bestDist = d
					best = i
				}
			}
			return best
		}

		const pad = float32(6)
		fieldL := fx + pad
		fieldR := fx + fw - pad

		// Click positions the cursor (with shift extending the selection).
		if focused && hovered && in.mouseClicked {
			clickX := in.mouseX - fieldL + scroll
			newCursor := cursorFromX(newText, clickX)
			if in.shift {
				cursor = newCursor
			} else {
				cursor = newCursor
				anchor = newCursor
			}
			uiCore.textBlink[id] = now
		}
		// Drag selection: mouse held and moving updates cursor, keeps anchor.
		if focused && hovered && in.mouseDown && !in.mouseClicked {
			clickX := in.mouseX - fieldL + scroll
			cursor = cursorFromX(newText, clickX)
			uiCore.textBlink[id] = now
		}

		hasSelection := cursor != anchor
		cmdOrCtrl := in.cmd || in.ctrl

		if focused {
			// Mirror the live selection into the clipboard buffer every
			// frame so the browser's native copy/cut event (which fires
			// one frame before this widget body runs) already has the
			// text. No-op on desktop. See uiPublishSelection.
			if hasSelection {
				uiPublishSelection(selectionSlice(newText, cursor, anchor))
			} else {
				uiPublishSelection("")
			}
			// ── Editor shortcuts (Cmd/Ctrl + …) ───────────────────
			if cmdOrCtrl && in.keyA {
				cursor = runeCount(newText)
				anchor = 0
				uiCore.textBlink[id] = now
				hasSelection = cursor != anchor
			}
			if cmdOrCtrl && in.keyC && hasSelection {
				uiClipboardWrite(selectionSlice(newText, cursor, anchor))
			}
			if cmdOrCtrl && in.keyX && hasSelection {
				uiClipboardWrite(selectionSlice(newText, cursor, anchor))
				pushUndo(id, newText)
				newText, cursor = deleteSelection(newText, cursor, anchor)
				anchor = cursor
				hasSelection = false
				uiCore.textBlink[id] = now
			}
			// Undo / redo. Shift+Z = redo (standard Mac), Y = redo too.
			if cmdOrCtrl && in.keyZ && !in.shift {
				if prev, ok := popUndo(id, newText); ok {
					newText = prev
					if cursor > runeCount(newText) {
						cursor = runeCount(newText)
					}
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}
			if cmdOrCtrl && ((in.keyZ && in.shift) || in.keyY) {
				if next, ok := popRedo(id, newText); ok {
					newText = next
					if cursor > runeCount(newText) {
						cursor = runeCount(newText)
					}
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}

			// Paste (browser fired the event, or desktop synthesised it).
			if in.clipPaste != "" {
				pushUndo(id, newText)
				if cursor != anchor {
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				}
				// textInput is single-line — collapse embedded newlines.
				pasted := strings.ReplaceAll(in.clipPaste, "\n", " ")
				pasted = strings.ReplaceAll(pasted, "\r", "")
				r := []rune(newText)
				ins := []rune(pasted)
				newR := append(append([]rune{}, r[:cursor]...), ins...)
				newR = append(newR, r[cursor:]...)
				newText = string(newR)
				cursor += len(ins)
				anchor = cursor
				uiCore.textBlink[id] = now
				hasSelection = false
			}

			// Forward delete.
			if in.deleteCount > 0 {
				if hasSelection {
					pushUndo(id, newText)
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				} else {
					pushUndo(id, newText)
					for i := 0; i < in.deleteCount; i++ {
						r := []rune(newText)
						if cursor < len(r) {
							newText = string(append(append([]rune{}, r[:cursor]...), r[cursor+1:]...))
						}
					}
				}
				uiCore.textBlink[id] = now
			}
			// Backspace.
			if in.backspaceCount > 0 {
				if hasSelection {
					pushUndo(id, newText)
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				} else {
					pushUndo(id, newText)
					for i := 0; i < in.backspaceCount; i++ {
						r := []rune(newText)
						if cursor > 0 {
							newText = string(append(append([]rune{}, r[:cursor-1]...), r[cursor:]...))
							cursor--
						}
					}
				}
				uiCore.textBlink[id] = now
			}
			// Cursor left (word-jump when Cmd/Ctrl held).
			if in.leftCount > 0 {
				if cmdOrCtrl {
					cursor = wordJumpLeft(newText, cursor)
				} else {
					for i := 0; i < in.leftCount; i++ {
						if cursor > 0 {
							cursor--
						}
					}
				}
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Cursor right (word-jump when Cmd/Ctrl held).
			if in.rightCount > 0 {
				if cmdOrCtrl {
					cursor = wordJumpRight(newText, cursor)
				} else {
					for i := 0; i < in.rightCount; i++ {
						if cursor < runeCount(newText) {
							cursor++
						}
					}
				}
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Home / End.
			if in.homePressed {
				cursor = 0
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			if in.endPressed {
				cursor = runeCount(newText)
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Typed printable chars — insert at cursor, replacing selection.
			// Suppress when a modifier is held (else Cmd+A inserts "a").
			if in.typedChars != "" && !cmdOrCtrl {
				filtered := make([]rune, 0, len(in.typedChars))
				for _, r := range in.typedChars {
					if r >= 32 { // strip control chars
						filtered = append(filtered, r)
					}
				}
				if len(filtered) > 0 {
					pushUndo(id, newText)
					if cursor != anchor {
						newText, cursor = deleteSelection(newText, cursor, anchor)
						anchor = cursor
					}
					r := []rune(newText)
					newR := append(append([]rune{}, r[:cursor]...), filtered...)
					newR = append(newR, r[cursor:]...)
					newText = string(newR)
					cursor += len(filtered)
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}
		}

		// Re-clamp after mutations.
		rl = runeCount(newText)
		if cursor > rl {
			cursor = rl
		}
		if anchor > rl {
			anchor = rl
		}

		// Auto-scroll to keep cursor visible.
		fieldW := fieldR - fieldL
		cursorX := xAtCursor(newText, cursor)
		if cursorX-scroll < 0 {
			scroll = cursorX
		} else if cursorX-scroll > fieldW {
			scroll = cursorX - fieldW
		}
		if scroll < 0 {
			scroll = 0
		}

		// Persist edits back to uiCore.
		uiCore.textCursor[id] = cursor
		uiCore.textAnchor[id] = anchor
		uiCore.textScroll[id] = scroll

		// ── Drawing ──────────────────────────────────────────────────
		// In modern style, when the field is empty and unfocused, the
		// label floats inside as a placeholder. When focused or with
		// content, the label sits above.
		showFloatingLabel := label.Value != "" && (focused || newText != "")
		if showFloatingLabel {
			activeRenderer.drawText(label.Value, int(fx), int(fy-18), false, scale*0.85, uiCore.theme.labelText)
		}
		bg := uiCore.theme.inputBg
		if focused {
			bg = uiCore.theme.inputFocusBg
		} else if hovered {
			bg = uiCore.theme.widgetBg
		}
		fieldR2 := uiCore.style.radiusMedium
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, fieldR2, fadeIfDisabled(bg))
		if !focused {
			activeRenderer.strokeRoundedRect(fx, fy, fw, fh, fieldR2, 0.5, fadeIfDisabled(uiCore.theme.widgetBgActive))
		}
		if focused {
			drawFocusRing(fx, fy, fw, fh, fieldR2)
		}

		// Clip to the field interior so scrolled text doesn't spill.
		activeRenderer.pushClip(fieldL, fy, fieldR-fieldL, fh)

		// Selection highlight — sized to the text strip.
		if focused && cursor != anchor {
			loX := xAtCursor(newText, anchor) - scroll
			hiX := xAtCursor(newText, cursor) - scroll
			if loX > hiX {
				loX, hiX = hiX, loX
			}
			selLH := activeRenderer.lineHeight(scale)
			selInset := activeRenderer.textTopInset(scale)
			selTop := fy + (fh-selLH)*0.5 + selInset
			activeRenderer.fillRoundedRect(fieldL+loX, selTop, hiX-loX, selLH-selInset, 0, uiCore.theme.accentBg)
		}

		// Text — vertically centred. drawText(centered=false) treats y
		// as the TOP of the glyph box, so back off half the line height
		// from field-centre.
		lh := activeRenderer.lineHeight(scale)
		textY := int(fy + (fh-lh)*0.5)
		if newText == "" && !focused && label.Value != "" {
			// Placeholder — the label dimmed inside the field.
			activeRenderer.drawText(label.Value, int(fieldL), textY, false, scale, fadeIfDisabled(uiCore.theme.dimText))
		} else {
			activeRenderer.drawText(newText, int(fieldL-scroll), textY, false, scale, fadeIfDisabled(uiCore.theme.widgetText))
		}

		// Blinking caret — shifted down by textTopInset so it sits
		// alongside the visible glyphs rather than floating above them
		// (Canvas2D textBaseline="top" has internal leading above glyphs).
		if focused {
			blinkBase := uiCore.textBlink[id]
			elapsed := now - blinkBase
			if elapsed-float64(int(elapsed)) < 0.5 || elapsed < 0.05 {
				caretX := fieldL + xAtCursor(newText, cursor) - scroll
				topInset := activeRenderer.textTopInset(scale)
				caretTop := fy + (fh-lh)*0.5 + topInset
				activeRenderer.fillRoundedRect(caretX, caretTop, 1.5, lh-topInset, 0, uiCore.theme.accent)
			}
		}
		activeRenderer.popClip()
		uiRegisterElement(id, fx, fy, fw, fh)

		if newText != currentText.Value && UiEventHookActive() {
			FireUiEventHook("text", "textInput", label.Value, &String{Value: newText}, int(in.mouseX), int(in.mouseY))
		}
		return &String{Value: newText}
	}}

	// textArea — draw a multi-line editable text box, returning its contents.
	//
	// The multi-line counterpart to textInput: Enter inserts a newline, the wheel
	// scrolls vertically, and selection spans lines. Wide lines are clipped on the
	// right (no horizontal scroll). The two optional trailing args are
	// order-independent — a number sets the text scale, and the string "klex"
	// turns on kLex syntax highlighting.
	//
	// @sig     textArea(label: string, text: string, x: int, y: int, w: int, h: int, [size: number], [syntax: string]) -> string
	// @param   label   caption above the box ("" for none)
	// @param   text    the box's current text
	// @param   x       left position in pixels
	// @param   y       top position in pixels
	// @param   w       width in pixels
	// @param   h       height in pixels
	// @param   size    text scale (default 0.5)
	// @param   syntax  "klex" to enable kLex syntax highlighting
	// @returns the box's text after this frame's edits
	// @errors  TypeError if label/text aren't strings, x/y/w/h aren't integers, or the optional args aren't a number/string; RuntimeError unless given 6 to 8 arguments
	// @example no-run code = textArea("Source", code, 20, 40, 400, 300, "klex")
	// @since   0.1.0
	// @see     textInput, getTypedChars
	Builtins["textArea"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 8 {
			return typeError("textArea expects 6-8 arguments: label, text, x, y, w, h, [size], [syntax]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		currentText, ok2 := args[1].(*String)
		if !ok1 || !ok2 {
			return typeError("textArea: label and text must be strings", ast.Pos{})
		}
		coords, err := requireIntArgs("textArea", "x/y/w/h must be integers", args[2:6])
		if err != nil {
			return err
		}
		// Optional trailing args, order-independent: a number sets the font
		// scale; a string selects a syntax-highlight mode ("klex"). Either,
		// both, or neither may be supplied.
		scale := float32(0.5)
		syntax := ""
		for k := 6; k < len(args); k++ {
			switch a := args[k].(type) {
			case *Float:
				scale = float32(a.Value)
			case *Integer:
				scale = float32(a.Value)
			case *String:
				syntax = a.Value
			default:
				return typeError("textArea: optional args must be a size (number) and/or a syntax mode (string)", ast.Pos{})
			}
		}
		highlight := syntax == "klex"

		id := fmt.Sprintf("ta_%d", uiCore.nextID)
		uiCore.nextID++
		ensureTextMaps()
		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])

		in := uiInput()
		hovered := uiCore.hoveredID == id
		focused := uiCore.activeID == id
		now := time.Since(uiCore.startTime).Seconds()

		if hovered && in.mouseClicked {
			uiCore.activeID = id
			focused = true
		} else if !hovered && in.mouseClicked && uiCore.activeID == id {
			uiCore.activeID = ""
			focused = false
		}

		newText := currentText.Value
		cursor := uiCore.textCursor[id]
		anchor := uiCore.textAnchor[id]
		scrollLine := uiCore.listScroll[id]

		lh := activeRenderer.lineHeight(scale)
		const pad = float32(6)
		fieldL := fx + pad
		fieldR := fx + fw - pad
		fieldT := fy + pad
		fieldB := fy + fh - pad
		visibleLines := int((fieldB - fieldT) / lh)
		if visibleLines < 1 {
			visibleLines = 1
		}
		// wrapW reserves room for the scrollbar thumb on the right.
		wrapW := fieldR - fieldL - 14
		var wlines []wrappedLine
		wlines = softWrapTextWithOffsets(newText, wrapW, scale)

		// Helpers for visual-line ↔ rune-index conversion using wlines.
		// runeIdxToLineCol maps a rune offset to (visual line index, col within that line).
		// lineColToRuneIdx maps (visual line index, col) back to a rune offset.
		// Both closures capture wlines by reference so they stay correct after rewraps.
		runeIdxToLineCol := func(_ string, idx int) (int, int) {
			ln := taLineForOffset(wlines, idx)
			return ln, idx - wlines[ln].startRune
		}
		lineColToRuneIdx := func(_ string, line, col int) int {
			if len(wlines) == 0 {
				return 0
			}
			if line < 0 {
				line = 0
			}
			if line >= len(wlines) {
				line = len(wlines) - 1
			}
			if col > wlines[line].runeCount {
				col = wlines[line].runeCount
			}
			return wlines[line].startRune + col
		}

		// Wheel scroll (per-line).
		if hovered && in.scrollY != 0 {
			step := 1
			if in.scrollY < 0 {
				step = -1
			}
			scrollLine += step
		}
		maxScroll := len(wlines) - visibleLines
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollLine < 0 {
			scrollLine = 0
		}
		if scrollLine > maxScroll {
			scrollLine = maxScroll
		}

		// xAtCol estimates pixel offset of `col` within the given line.
		xAtCol := func(line string, col int) float32 {
			r := []rune(line)
			if col < 0 {
				col = 0
			}
			if col > len(r) {
				col = len(r)
			}
			return activeRenderer.textWidth(string(r[:col]), scale)
		}
		colFromX := func(line string, px float32) int {
			r := []rune(line)
			best := 0
			bestDist := float32(1e9)
			for i := 0; i <= len(r); i++ {
				w := activeRenderer.textWidth(string(r[:i]), scale)
				d := w - px
				if d < 0 {
					d = -d
				}
				if d < bestDist {
					bestDist = d
					best = i
				}
			}
			return best
		}

		// Click positions the cursor.
		if focused && hovered && in.mouseClicked {
			clickLine := int((in.mouseY-fieldT)/lh) + scrollLine
			if clickLine < 0 {
				clickLine = 0
			}
			if clickLine >= len(wlines) {
				clickLine = len(wlines) - 1
			}
			clickCol := colFromX(wlines[clickLine].text, in.mouseX-fieldL)
			newCursor := lineColToRuneIdx(newText, clickLine, clickCol)
			if in.shift {
				cursor = newCursor
			} else {
				cursor = newCursor
				anchor = newCursor
			}
			uiCore.textBlink[id] = now
		}
		// Drag selection: mouse held and moving updates cursor, keeps anchor.
		if focused && hovered && in.mouseDown && !in.mouseClicked {
			dragLine := int((in.mouseY-fieldT)/lh) + scrollLine
			if dragLine < 0 {
				dragLine = 0
			}
			if dragLine >= len(wlines) {
				dragLine = len(wlines) - 1
			}
			dragCol := colFromX(wlines[dragLine].text, in.mouseX-fieldL)
			cursor = lineColToRuneIdx(newText, dragLine, dragCol)
			uiCore.textBlink[id] = now
		}

		hasSelection := cursor != anchor
		cmdOrCtrl := in.cmd || in.ctrl
		prevCursor := cursor

		if focused {
			// Mirror the live selection into the clipboard buffer every
			// frame so the browser's native copy/cut event (which fires
			// one frame before this widget body runs) already has the
			// text. No-op on desktop. See uiPublishSelection.
			if hasSelection {
				uiPublishSelection(selectionSlice(newText, cursor, anchor))
			} else {
				uiPublishSelection("")
			}
			// ── Editor shortcuts ──────────────────────────────────
			if cmdOrCtrl && in.keyA {
				cursor = runeCount(newText)
				anchor = 0
				uiCore.textBlink[id] = now
				hasSelection = cursor != anchor
			}
			if cmdOrCtrl && in.keyC && hasSelection {
				uiClipboardWrite(selectionSlice(newText, cursor, anchor))
			}
			if cmdOrCtrl && in.keyX && hasSelection {
				uiClipboardWrite(selectionSlice(newText, cursor, anchor))
				pushUndo(id, newText)
				newText, cursor = deleteSelection(newText, cursor, anchor)
				anchor = cursor
				hasSelection = false
				uiCore.textBlink[id] = now
			}
			if cmdOrCtrl && in.keyZ && !in.shift {
				if prev, ok := popUndo(id, newText); ok {
					newText = prev
					if cursor > runeCount(newText) {
						cursor = runeCount(newText)
					}
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}
			if cmdOrCtrl && ((in.keyZ && in.shift) || in.keyY) {
				if next, ok := popRedo(id, newText); ok {
					newText = next
					if cursor > runeCount(newText) {
						cursor = runeCount(newText)
					}
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}
			// Paste — preserve newlines (textArea contract).
			if in.clipPaste != "" {
				pushUndo(id, newText)
				if cursor != anchor {
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				}
				r := []rune(newText)
				ins := []rune(in.clipPaste)
				newR := append(append([]rune{}, r[:cursor]...), ins...)
				newR = append(newR, r[cursor:]...)
				newText = string(newR)
				cursor += len(ins)
				anchor = cursor
				uiCore.textBlink[id] = now
				hasSelection = false
			}

			// Forward delete.
			if in.deleteCount > 0 {
				if hasSelection {
					pushUndo(id, newText)
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				} else {
					pushUndo(id, newText)
					for i := 0; i < in.deleteCount; i++ {
						r := []rune(newText)
						if cursor < len(r) {
							newText = string(append(append([]rune{}, r[:cursor]...), r[cursor+1:]...))
						}
					}
				}
				uiCore.textBlink[id] = now
			}
			// Backspace.
			if in.backspaceCount > 0 {
				if hasSelection {
					pushUndo(id, newText)
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				} else {
					pushUndo(id, newText)
					for i := 0; i < in.backspaceCount; i++ {
						r := []rune(newText)
						if cursor > 0 {
							newText = string(append(append([]rune{}, r[:cursor-1]...), r[cursor:]...))
							cursor--
						}
					}
				}
				uiCore.textBlink[id] = now
			}
			// Horizontal arrows (word-jump with Cmd/Ctrl).
			if in.leftCount > 0 {
				if cmdOrCtrl {
					cursor = wordJumpLeft(newText, cursor)
				} else {
					for i := 0; i < in.leftCount; i++ {
						if cursor > 0 {
							cursor--
						}
					}
				}
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			if in.rightCount > 0 {
				if cmdOrCtrl {
					cursor = wordJumpRight(newText, cursor)
				} else {
					for i := 0; i < in.rightCount; i++ {
						if cursor < runeCount(newText) {
							cursor++
						}
					}
				}
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Vertical arrows — move between visual (wrapped) lines.
			if in.upCount > 0 || in.downCount > 0 {
				curLine, curCol := runeIdxToLineCol(newText, cursor)
				targetLine := curLine + in.downCount - in.upCount
				if targetLine < 0 {
					targetLine = 0
				}
				if targetLine >= len(wlines) {
					targetLine = len(wlines) - 1
				}
				targetCol := curCol
				if targetCol > wlines[targetLine].runeCount {
					targetCol = wlines[targetLine].runeCount
				}
				cursor = lineColToRuneIdx(newText, targetLine, targetCol)
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Home / End within current visual line.
			if in.homePressed {
				lineIdx, _ := runeIdxToLineCol(newText, cursor)
				cursor = lineColToRuneIdx(newText, lineIdx, 0)
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			if in.endPressed {
				lineIdx, _ := runeIdxToLineCol(newText, cursor)
				cursor = lineColToRuneIdx(newText, lineIdx, wlines[lineIdx].runeCount)
				if !in.shift {
					anchor = cursor
				}
				uiCore.textBlink[id] = now
			}
			// Enter inserts newline.
			if in.enterPressed {
				pushUndo(id, newText)
				if cursor != anchor {
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				}
				r := []rune(newText)
				newR := append(append([]rune{}, r[:cursor]...), '\n')
				newR = append(newR, r[cursor:]...)
				newText = string(newR)
				cursor++
				anchor = cursor
				uiCore.textBlink[id] = now
			}
			// Typed printable chars (suppressed when modifier held).
			if in.typedChars != "" && !cmdOrCtrl {
				filtered := make([]rune, 0, len(in.typedChars))
				for _, r := range in.typedChars {
					if r >= 32 {
						filtered = append(filtered, r)
					}
				}
				if len(filtered) > 0 {
					pushUndo(id, newText)
					if cursor != anchor {
						newText, cursor = deleteSelection(newText, cursor, anchor)
						anchor = cursor
					}
					r := []rune(newText)
					newR := append(append([]rune{}, r[:cursor]...), filtered...)
					newR = append(newR, r[cursor:]...)
					newText = string(newR)
					cursor += len(filtered)
					anchor = cursor
					uiCore.textBlink[id] = now
				}
			}
		}

		// Re-clamp + auto-scroll cursor into view.
		rl := runeCount(newText)
		if cursor > rl {
			cursor = rl
		}
		if anchor > rl {
			anchor = rl
		}
		wlines = softWrapTextWithOffsets(newText, wrapW, scale)
		maxScroll = len(wlines) - visibleLines
		if maxScroll < 0 {
			maxScroll = 0
		}
		if cursor != prevCursor {
			curLine, _ := runeIdxToLineCol(newText, cursor)
			if curLine < scrollLine {
				scrollLine = curLine
			}
			if curLine >= scrollLine+visibleLines {
				scrollLine = curLine - visibleLines + 1
			}
		}
		if scrollLine < 0 {
			scrollLine = 0
		}

		uiCore.textCursor[id] = cursor
		uiCore.textAnchor[id] = anchor
		uiCore.listScroll[id] = scrollLine

		// ── Drawing ──────────────────────────────────────────────────
		showFloatingLabel := label.Value != "" && (focused || newText != "")
		if showFloatingLabel {
			activeRenderer.drawText(label.Value, int(fx), int(fy-18), false, scale*0.85, uiCore.theme.labelText)
		}
		bg := uiCore.theme.inputBg
		if focused {
			bg = uiCore.theme.inputFocusBg
		} else if hovered {
			bg = uiCore.theme.widgetBg
		}
		taR := uiCore.style.radiusMedium
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, taR, bg)
		if !focused {
			activeRenderer.strokeRoundedRect(fx, fy, fw, fh, taR, 0.5, uiCore.theme.widgetBgActive)
		}
		if focused {
			drawFocusRing(fx, fy, fw, fh, taR)
		}
		// Empty-field placeholder — label rendered dim inside, top-left.
		if newText == "" && !focused && label.Value != "" {
			activeRenderer.drawText(label.Value, int(fieldL), int(fieldT), false, scale, uiCore.theme.dimText)
		}
		activeRenderer.pushClip(fieldL, fieldT, fieldR-fieldL, fieldB-fieldT)

		topInset := activeRenderer.textTopInset(scale)

		// Selection highlight — drawn per visible line.
		if focused && cursor != anchor {
			lo, hi := anchor, cursor
			if lo > hi {
				lo, hi = hi, lo
			}
			loLine, loCol := runeIdxToLineCol(newText, lo)
			hiLine, hiCol := runeIdxToLineCol(newText, hi)
			for ln := loLine; ln <= hiLine; ln++ {
				if ln < scrollLine || ln >= scrollLine+visibleLines {
					continue
				}
				lineText := wlines[ln].text
				startCol := 0
				endCol := wlines[ln].runeCount
				if ln == loLine {
					startCol = loCol
				}
				if ln == hiLine {
					endCol = hiCol
				}
				if startCol == endCol {
					continue
				}
				selX := fieldL + xAtCol(lineText, startCol)
				selW := xAtCol(lineText, endCol) - xAtCol(lineText, startCol)
				selY := fieldT + float32(ln-scrollLine)*lh + topInset
				activeRenderer.fillRoundedRect(selX, selY, selW, lh-topInset, 0, uiCore.theme.accentBg)
			}
		}

		// Lines of text — single colour, or per-token colours when syntax
		// highlighting is enabled. The full text is tokenised once into a
		// per-rune category slice; each visual line then draws consecutive
		// same-category runs in their token colour.
		var hlCats []synCat
		if highlight {
			hlCats = highlightKLex([]rune(newText))
		}
		for i := 0; i < visibleLines && scrollLine+i < len(wlines); i++ {
			ly := fieldT + float32(i)*lh
			wl := wlines[scrollLine+i]
			if !highlight {
				activeRenderer.drawText(wl.text, int(fieldL), int(ly), false, scale, uiCore.theme.widgetText)
				continue
			}
			lineRunes := []rune(wl.text)
			base := wl.startRune
			normal := uiCore.theme.widgetText
			bg := uiCore.theme.inputBg
			x := fieldL
			r := 0
			for r < len(lineRunes) {
				cat := catAt(hlCats, base+r)
				s := r + 1
				for s < len(lineRunes) && catAt(hlCats, base+s) == cat {
					s++
				}
				runText := string(lineRunes[r:s])
				activeRenderer.drawText(runText, int(x), int(ly), false, scale, syntaxColor(cat, bg, normal))
				x += activeRenderer.textWidth(runText, scale)
				r = s
			}
		}

		// Blinking caret — shifted down by textTopInset so it sits
		// alongside the visible glyphs rather than floating above them.
		if focused {
			blinkBase := uiCore.textBlink[id]
			elapsed := now - blinkBase
			if elapsed-float64(int(elapsed)) < 0.5 || elapsed < 0.05 {
				cLine, cCol := runeIdxToLineCol(newText, cursor)
				if cLine >= scrollLine && cLine < scrollLine+visibleLines {
					caretX := fieldL + xAtCol(wlines[cLine].text, cCol)
					caretY := fieldT + float32(cLine-scrollLine)*lh + topInset
					activeRenderer.fillRoundedRect(caretX, caretY, 1.5, lh-topInset, 0, uiCore.theme.accent)
				}
			}
		}
		activeRenderer.popClip()

		// Vertical scrollbar — draggable thumb.
		if maxScroll > 0 {
			sbW := float32(10)
			sbX := fx + fw - sbW - 3
			sbTrackY := fy + 3
			sbTrackH := fh - 6
			thumbH := sbTrackH * float32(visibleLines) / float32(len(wlines))
			if thumbH < 20 {
				thumbH = 20
			}
			sbID := id + "_sb"
			// Hit-test against the WHOLE track (not just the thumb) so
			// the user can grab anywhere along it.
			overTrack := in.mouseX >= sbX-2 && in.mouseX <= sbX+sbW+2 &&
				in.mouseY >= sbTrackY && in.mouseY <= sbTrackY+sbTrackH
			if overTrack && in.mouseClicked {
				uiCore.activeID = sbID
			}
			if uiCore.activeID == sbID {
				if in.mouseDown {
					// Map mouse Y → fractional scroll position.
					t := (in.mouseY - sbTrackY - thumbH*0.5) / (sbTrackH - thumbH)
					if t < 0 {
						t = 0
					}
					if t > 1 {
						t = 1
					}
					scrollLine = int(t * float32(maxScroll))
					if scrollLine < 0 {
						scrollLine = 0
					}
					if scrollLine > maxScroll {
						scrollLine = maxScroll
					}
					uiCore.listScroll[id] = scrollLine
				} else {
					uiCore.activeID = ""
				}
			}
			thumbY := sbTrackY + (sbTrackH-thumbH)*float32(scrollLine)/float32(maxScroll)
			thumbCol := uiCore.theme.widgetBgHover
			if uiCore.activeID == sbID || overTrack {
				thumbCol = uiCore.theme.handle
			}
			activeRenderer.fillRoundedRect(sbX, sbTrackY, sbW, sbTrackH, sbW*0.5, uiCore.theme.track)
			activeRenderer.fillRoundedRect(sbX, thumbY, sbW, thumbH, sbW*0.5, thumbCol)
		}

		uiRegisterElement(id, fx, fy, fw, fh)
		if newText != currentText.Value && UiEventHookActive() {
			FireUiEventHook("text", "textArea", label.Value, &String{Value: newText}, int(in.mouseX), int(in.mouseY))
		}
		return &String{Value: newText}
	}}

	// table — draw a scrollable data grid, returning the selected row index.
	//
	// `headers` is an array of column titles; `rows` is an array of rows, each an
	// array of cell values. Supports click-to-select, scrolling, and click-to-sort
	// by column. Returns the selected row index (-1 if none).
	//
	// @sig     table(headers: array, rows: array, x: int, y: int, w: int, h: int, [size: number]) -> int
	// @param   headers  array of column header strings
	// @param   rows     array of rows, each an array of cell values
	// @param   x        left position in pixels
	// @param   y        top position in pixels
	// @param   w        width in pixels
	// @param   h        height in pixels
	// @param   size     text scale (default 0.5)
	// @returns the selected row index, or -1 if no row is selected
	// @errors  TypeError if headers/rows aren't arrays or x/y/w/h aren't integers; RuntimeError unless given 6 or 7 arguments
	// @example no-run sel = table(["Name", "Size"], rows, 20, 40, 400, 300)
	// @since   0.1.0
	// @see     list, scrollArea, treeView
	Builtins["table"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("table expects 6-7 arguments: headers, rows, x, y, w, h, [size]", ast.Pos{})
		}
		headers, ok1 := args[0].(*Array)
		rowsArr, ok2 := args[1].(*Array)
		if !ok1 || !ok2 {
			return typeError("table: headers and rows must be arrays", ast.Pos{})
		}
		coords, err := requireIntArgs("table", "x/y/w/h must be integers", args[2:6])
		if err != nil {
			return err
		}
		scale, err := extractScale("table", args, 6, 0.5)
		if err != nil {
			return err
		}

		id := fmt.Sprintf("tbl_%d", uiCore.nextID)
		uiCore.nextID++

		fx, fy := float32(coords[0]), float32(coords[1])
		fw, fh := float32(coords[2]), float32(coords[3])
		numCols := len(headers.Elements)
		numRows := len(rowsArr.Elements)
		if numCols == 0 {
			return &Integer{Value: -1}
		}
		// Row + header heights scale with the font so larger text fits.
		lhTbl := activeRenderer.lineHeight(scale)
		rowH := lhTbl + uiCore.style.spacingS*2
		headerH := rowH + 4
		colW := fw / float32(numCols)

		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		if uiCore.tableSortCol == nil {
			uiCore.tableSortCol = make(map[string]int)
			uiCore.tableSortDir = make(map[string]int)
		}
		selectedRow := uiCore.listSelected[id]
		originalSel := selectedRow
		scrollOff := uiCore.listScroll[id]
		sortCol, hasSortCol := uiCore.tableSortCol[id]
		sortDir, _ := uiCore.tableSortDir[id]
		if !hasSortCol {
			sortCol = -1
		}
		contentH := fh - headerH
		visibleRows := int(contentH / rowH)
		if visibleRows < 1 {
			visibleRows = 1
		}
		maxScroll := numRows - visibleRows
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollOff > maxScroll {
			scrollOff = maxScroll
		}

		in := uiInput()
		hovered := in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= fy && in.mouseY <= fy+fh
		if hovered && in.scrollY != 0 {
			step := 1
			if in.scrollY < 0 {
				step = -1
			}
			scrollOff += step
			if scrollOff < 0 {
				scrollOff = 0
			}
			if scrollOff > maxScroll {
				scrollOff = maxScroll
			}
			uiCore.listScroll[id] = scrollOff
		}

		r := uiCore.style.radiusMedium
		activeRenderer.fillRoundedRect(fx, fy, fw, fh, r, uiCore.theme.inputBg)
		activeRenderer.strokeRoundedRect(fx, fy, fw, fh, r, 0.5, uiCore.theme.widgetBgActive)
		// Header row.
		activeRenderer.fillRoundedRect(fx, fy, fw, headerH, r, uiCore.theme.widgetBgActive)

		// Detect header click → toggle sort. Click same column cycles
		// asc → desc → asc; click different column resets to asc on it.
		if numRows > 0 && in.mouseClicked &&
			in.mouseY >= fy && in.mouseY <= fy+headerH &&
			in.mouseX >= fx && in.mouseX <= fx+fw {
			clickedCol := int((in.mouseX - fx) / colW)
			if clickedCol >= 0 && clickedCol < numCols {
				if clickedCol == sortCol {
					if sortDir >= 0 {
						sortDir = -1
					} else {
						sortDir = 1
					}
				} else {
					sortCol = clickedCol
					sortDir = 1
				}
				uiCore.tableSortCol[id] = sortCol
				uiCore.tableSortDir[id] = sortDir
			}
		}

		// Header text + dividers + sort indicator.
		for c := 0; c < numCols; c++ {
			hdr := ""
			if s, ok := headers.Elements[c].(*String); ok {
				hdr = s.Value
			}
			// Header background tint on the sorted column.
			if c == sortCol && sortDir != 0 {
				tint := uiCore.theme.accent
				tint[3] = 0.18
				activeRenderer.fillRoundedRect(fx+float32(c)*colW, fy, colW, headerH, 0, tint)
			}
			activeRenderer.drawText(hdr, int(fx+float32(c)*colW+8), int(fy+(headerH-lhTbl)*0.5), false, scale, uiCore.theme.widgetText)
			// Sort arrow indicator on the sorted column.
			if c == sortCol && sortDir != 0 {
				arrow := "▲"
				if sortDir < 0 {
					arrow = "▼"
				}
				activeRenderer.drawText(arrow, int(fx+float32(c+1)*colW-22), int(fy+(headerH-lhTbl)*0.5), false, scale*0.85, uiCore.theme.accent)
			}
			if c > 0 {
				div := uiCore.theme.accentBg
				div[3] = 0.6
				activeRenderer.fillRoundedRect(fx+float32(c)*colW, fy, 1, headerH, 0, div)
			}
		}

		if numRows == 0 {
			drawEmptyState(fx, fy+headerH, fw, fh-headerH, "No rows")
			uiRegisterElement(id, fx, fy, fw, fh)
			return &Integer{Value: -1}
		}

		// Build the display permutation. perm[i] is the ORIGINAL index
		// of the row to display at visual position i.
		perm := make([]int, numRows)
		for i := range perm {
			perm[i] = i
		}
		if sortCol >= 0 && sortCol < numCols && sortDir != 0 {
			cellNum := func(o Object) (float64, bool) {
				switch v := o.(type) {
				case *Float:
					return v.Value, true
				case *Integer:
					return float64(v.Value), true
				case *String:
					var f float64
					if _, err := fmt.Sscanf(v.Value, "%g", &f); err == nil {
						return f, true
					}
				}
				return 0, false
			}
			cellStr := func(o Object) string {
				if s, ok := o.(*String); ok {
					return s.Value
				}
				return o.Inspect()
			}
			sort.SliceStable(perm, func(i, j int) bool {
				ra, _ := rowsArr.Elements[perm[i]].(*Array)
				rb, _ := rowsArr.Elements[perm[j]].(*Array)
				if ra == nil || rb == nil || sortCol >= len(ra.Elements) || sortCol >= len(rb.Elements) {
					return false
				}
				af, aok := cellNum(ra.Elements[sortCol])
				bf, bok := cellNum(rb.Elements[sortCol])
				if aok && bok {
					if sortDir > 0 {
						return af < bf
					}
					return af > bf
				}
				as := cellStr(ra.Elements[sortCol])
				bs := cellStr(rb.Elements[sortCol])
				if sortDir > 0 {
					return as < bs
				}
				return as > bs
			})
		}

		// Keyboard nav — table takes focus on row click. Up/Down step
		// selection through the SORTED order (so visual continuity is
		// preserved), but selectedRow stays in original-index units.
		focused := uiCore.activeID == id
		if focused && (in.upCount > 0 || in.downCount > 0) {
			// Find current display position of selectedRow.
			curVis := -1
			for i, p := range perm {
				if p == selectedRow {
					curVis = i
					break
				}
			}
			if curVis < 0 {
				curVis = 0
			}
			for i := 0; i < in.upCount; i++ {
				if curVis > 0 {
					curVis--
				}
			}
			for i := 0; i < in.downCount; i++ {
				if curVis < numRows-1 {
					curVis++
				}
			}
			selectedRow = perm[curVis]
			uiCore.listSelected[id] = selectedRow
			// Keep selection in view.
			if curVis < scrollOff {
				scrollOff = curVis
			}
			if curVis >= scrollOff+visibleRows {
				scrollOff = curVis - visibleRows + 1
			}
			uiCore.listScroll[id] = scrollOff
		}

		// Body rows (clipped). i is visual row position; perm[i+scrollOff]
		// is the original row index to display there.
		activeRenderer.pushClip(fx, fy+headerH, fw, fh-headerH)
		for i := 0; i < visibleRows; i++ {
			visIdx := i + scrollOff
			if visIdx >= numRows {
				break
			}
			origIdx := perm[visIdx]
			ry := fy + headerH + float32(i)*rowH

			bg := uiCore.theme.inputBg
			textCol := uiCore.theme.labelText
			if origIdx == selectedRow {
				bg = uiCore.theme.accentBg
				textCol = uiCore.theme.widgetText
			} else if i%2 == 1 {
				bg = uiCore.theme.track
			}
			activeRenderer.fillRoundedRect(fx, ry, fw, rowH, 0, bg)

			if in.mouseClicked && in.mouseX >= fx && in.mouseX <= fx+fw && in.mouseY >= ry && in.mouseY <= ry+rowH {
				selectedRow = origIdx
				uiCore.listSelected[id] = origIdx
				uiCore.activeID = id // focus for keyboard nav
			}

			if row, ok := rowsArr.Elements[origIdx].(*Array); ok {
				for c := 0; c < numCols && c < len(row.Elements); c++ {
					cell := row.Elements[c].Inspect()
					if s, ok2 := row.Elements[c].(*String); ok2 {
						cell = s.Value
					}
					activeRenderer.drawText(cell, int(fx+float32(c)*colW+8), int(ry+(rowH-lhTbl)*0.5), false, scale, textCol)
					if c > 0 {
						div := uiCore.theme.widgetBg
						div[3] = 0.4
						activeRenderer.fillRoundedRect(fx+float32(c)*colW, ry, 1, rowH, 0, div)
					}
				}
			}
		}
		activeRenderer.popClip()

		if numRows > visibleRows {
			thumbExtent := float32(visibleRows) / float32(numRows)
			newOff := drawScrollbar(id, fx, fy+headerH, fw, fh-headerH, float32(scrollOff), float32(maxScroll), thumbExtent)
			if int(newOff) != scrollOff {
				scrollOff = int(newOff)
				uiCore.listScroll[id] = scrollOff
			}
		}
		uiRegisterElement(id, fx, fy, fw, fh)
		if selectedRow != originalSel && UiEventHookActive() {
			FireUiEventHook("select", "table", "", &Integer{Value: selectedRow}, int(in.mouseX), int(in.mouseY))
		}
		return &Integer{Value: selectedRow}
	}}
}
