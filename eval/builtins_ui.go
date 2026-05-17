//go:build !js

package eval

import (
	"fmt"
	"klex/ast"
	"math"
	"strings"
	"time"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

var uiDropdownOpen string

// uiWrapText splits text at explicit newlines then word-wraps each segment to maxChars.
func uiWrapText(text string, maxChars int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
			} else if len(line)+1+len(word) <= maxChars {
				line += " " + word
			} else {
				lines = append(lines, line)
				line = word
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func init() {
	gfx.uiTheme = defaultUIPalette()

	// uiBegin() — reset UI state at the start of each draw loop
	Builtins["uiBegin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiBegin expects no arguments", ast.Pos{})
		}
		gfx.uiNextID = 0
		gfx.uiElements = make(map[string][4]float32)
		gfx.uiPendingTooltip.active = false
		gfx.uiTooltipMatchedThisFrame = false
		if gfx.uiListSelected == nil {
			gfx.uiListSelected = make(map[string]int)
		}
		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}
		return NULL
	}}

	// uiNextFieldID() → string
	// Returns the ID the next textInput widget will receive.
	// Call this immediately before textInput() to capture its ID for Tab focus management.
	Builtins["uiNextFieldID"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: fmt.Sprintf("txt_%d", gfx.uiNextID)}
	}}

	// uiGetFocus() → string
	// Returns the ID of the currently focused widget, or "" if none.
	Builtins["uiGetFocus"] = &Builtin{Fn: func(args []Object) Object {
		return &String{Value: gfx.uiActiveID}
	}}

	// uiSetFocus(id) — sets focus to the widget with the given ID.
	// Use in combination with uiNextFieldID() to implement Tab key navigation.
	Builtins["uiSetFocus"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiSetFocus expects 1 argument: id string", ast.Pos{})
		}
		id, ok := args[0].(*String)
		if !ok {
			return typeError("uiSetFocus: argument must be a string", ast.Pos{})
		}
		gfx.uiActiveID = id.Value
		return NULL
	}}

	// uiSetFont(font) — set the active font for all widget text (button labels, tabs, etc.)
	// Call once per frame (or once before window()) after loading a font with loadFont().
	// Reverts to the embedded monospace font when uiResetFont() is called.
	Builtins["uiSetFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiSetFont expects 1 argument: font", ast.Pos{})
		}
		fnt, ok := args[0].(*Font)
		if !ok {
			return typeError("uiSetFont: argument must be a font from loadFont()", ast.Pos{})
		}
		gfx.uiActiveFont = fnt
		return NULL
	}}

	// uiResetFont() — revert widget text back to the embedded monospace font.
	Builtins["uiResetFont"] = &Builtin{Fn: func(args []Object) Object {
		gfx.uiActiveFont = nil
		return NULL
	}}

	// uiEnd() — finalize UI frame, update hover state
	Builtins["uiEnd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiEnd expects no arguments", ast.Pos{})
		}
		// Render any deferred dropdown popup on top of all other widgets.
		if gfx.uiPendingDropdown.active {
			p := gfx.uiPendingDropdown
			gfx.uiPendingDropdown.active = false
			const itemH = 24.0
			menuH := float32(len(p.items)) * itemH
			drawRoundedRectSDF(p.fx, p.fy, p.fw, menuH, 4, gfx.uiTheme.inputBg, false, 0)
			savedFill := gfx.fillColor
			for i, itemText := range p.items {
				itemY := p.fy + float32(i)*itemH
				isItemHovered := gfx.mouseX >= float64(p.fx) && gfx.mouseX <= float64(p.fx+p.fw) &&
					gfx.mouseY >= float64(itemY) && gfx.mouseY <= float64(itemY+itemH)
				itemBg := gfx.uiTheme.inputBg
				if i == p.selectedIdx {
					itemBg = gfx.uiTheme.accentBg
				} else if isItemHovered {
					itemBg = gfx.uiTheme.widgetBgActive
				}
				drawRoundedRectSDF(p.fx, itemY, p.fw, itemH, 2, itemBg, false, 0)
				if i == p.selectedIdx {
					gfx.fillColor = gfx.uiTheme.widgetText
				} else if isItemHovered {
					gfx.fillColor = gfx.uiTheme.labelText
				} else {
					gfx.fillColor = gfx.uiTheme.dimText
				}
				drawText(itemText, int(p.fx+8), int(itemY+(itemH-p.charH)*0.5), false, p.textScale)
				uiRegisterElement(fmt.Sprintf("%s_opt_%d", p.id, i), p.fx, itemY, p.fw, itemH)
			}
			gfx.fillColor = savedFill
		}
		// Render active toasts — bottom-right corner, stacked upward.
		now := time.Since(gfx.startTime).Seconds()
		const (
			toastW      = 340.0
			toastH      = 44.0
			toastMargin = 16.0
			toastGap    = 8.0
			toastRadius = 6.0
			fadeDur     = 0.5
		)
		toastStyleColor := func(style string) [4]float32 {
			switch style {
			case "success":
				return [4]float32{0.20, 0.75, 0.35, 1}
			case "warn":
				return [4]float32{0.90, 0.55, 0.10, 1}
			case "error":
				return [4]float32{0.85, 0.20, 0.20, 1}
			default: // "info"
				return [4]float32{0.25, 0.55, 0.90, 1}
			}
		}
		live := gfx.uiToasts[:0]
		savedFillToast := gfx.fillColor
		for i, t := range gfx.uiToasts {
			remaining := t.expiresAt - now
			if remaining <= 0 {
				continue
			}
			live = append(live, gfx.uiToasts[i])
			alpha := float32(1.0)
			if remaining < fadeDur {
				alpha = float32(remaining / fadeDur)
			}
			slot := float32(len(live) - 1)
			tx := float32(gfx.winW) - toastW - toastMargin
			ty := float32(gfx.winH) - toastMargin - toastH - slot*(toastH+toastGap)
			bg := [4]float32{0.12, 0.12, 0.14, 0.95 * alpha}
			drawRoundedRectSDF(tx, ty, toastW, toastH, toastRadius, bg, false, 0)
			accent := toastStyleColor(t.style)
			accent[3] *= alpha
			drawRoundedRectSDF(tx, ty, 4, toastH, toastRadius, accent, false, 0)
			gfx.fillColor = [4]float32{1, 1, 1, alpha}
			charH := float32(gfx.fontCellH) * 0.5
			if gfx.uiActiveFont != nil {
				charH = gfx.uiActiveFont.LineH * 0.5
			}
			drawText(t.message, int(tx+14), int(ty+(toastH-charH)*0.5), false, 0.5)
		}
		gfx.uiToasts = live
		gfx.fillColor = savedFillToast

		// Render pending tooltip on top of everything.
		if gfx.uiPendingTooltip.active {
			const (
				tipPadX  = float32(10.0)
				tipPadY  = float32(6.0)
				tipScale = float32(0.5)
				tipR     = float32(4.0)
			)
			tip := gfx.uiPendingTooltip
			charH := uiCharH(tipScale)
			// Measure exact text width using glyph advances for proportional fonts.
			var textW float32
			if gfx.uiActiveFont != nil {
				for _, ch := range tip.text {
					if g, ok := gfx.uiActiveFont.glyphs[ch]; ok {
						textW += g.advance * tipScale
					} else {
						textW += gfx.uiActiveFont.fallback.advance * tipScale
					}
				}
			} else {
				textW = float32(len(tip.text)) * float32(gfx.fontCellW) * tipScale
			}
			tipW := textW + tipPadX*2
			tipH := charH + tipPadY*2
			// Position above-right of cursor; clamp to window edges.
			tx := tip.mx + 14
			ty := tip.my - tipH - 6
			if tx+tipW > float32(gfx.winW)-4 {
				tx = float32(gfx.winW) - tipW - 4
			}
			if ty < 4 {
				ty = tip.my + 20
			}
			savedFillTip := gfx.fillColor
			drawRoundedRectSDF(tx, ty, tipW, tipH, tipR, [4]float32{0.08, 0.08, 0.10, 1.0}, false, 0)
			drawRoundedRectSDF(tx, ty, tipW, tipH, tipR, gfx.uiTheme.accent, true, 0.5)
			gfx.fillColor = [4]float32{1, 1, 1, 1}
			drawText(tip.text, int(tx+tipPadX), int(ty+tipPadY), false, tipScale)
			gfx.fillColor = savedFillTip
		}
		if !gfx.uiTooltipMatchedThisFrame {
			// No tooltip() call matched this frame — mouse left all tooltip widgets.
			gfx.uiTooltipHoveredID = ""
		}

		mx, my := gfx.mouseX, gfx.mouseY
		gfx.uiHoveredID = uiCheckHover(mx, my)

		// Lazy-init standard cursors and set the right shape each frame.
		if gfx.cursorArrow == nil {
			gfx.cursorArrow = glfw.CreateStandardCursor(glfw.ArrowCursor)
			gfx.cursorIBeam = glfw.CreateStandardCursor(glfw.IBeamCursor)
			gfx.cursorHand = glfw.CreateStandardCursor(glfw.HandCursor)
			gfx.cursorResizeEW = glfw.CreateStandardCursor(glfw.HResizeCursor)
			gfx.cursorResizeNS = glfw.CreateStandardCursor(glfw.VResizeCursor)
		}
		hid := gfx.uiHoveredID
		switch {
		case strings.HasPrefix(hid, "txt_") || strings.HasPrefix(hid, "ta_"):
			gfx.win.SetCursor(gfx.cursorIBeam)
		case strings.HasPrefix(hid, "spl_v_"):
			gfx.win.SetCursor(gfx.cursorResizeEW)
		case strings.HasPrefix(hid, "spl_h_"):
			gfx.win.SetCursor(gfx.cursorResizeNS)
		case strings.HasPrefix(hid, "btn_") ||
			strings.HasPrefix(hid, "chk_") ||
			strings.HasPrefix(hid, "sld_") ||
			strings.HasPrefix(hid, "dd_") ||
			strings.HasPrefix(hid, "tgl_") ||
			strings.HasPrefix(hid, "rad_") ||
			strings.HasPrefix(hid, "list_") ||
			strings.HasPrefix(hid, "listm_") ||
			strings.HasPrefix(hid, "tab_") ||
			strings.HasPrefix(hid, "ns_") ||
			strings.HasPrefix(hid, "ctx_") ||
			strings.HasPrefix(hid, "tv_") ||
			strings.HasPrefix(hid, "tbl_") ||
			strings.HasPrefix(hid, "acc_"):
			gfx.win.SetCursor(gfx.cursorHand)
		default:
			gfx.win.SetCursor(gfx.cursorArrow)
		}
		return NULL
	}}

	// toast(message, [style], [duration]) — show an ephemeral notification.
	// style: "info" (default), "success", "warn", "error"
	// duration: seconds the toast is visible (default 3.0)
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
		expiresAt := time.Since(gfx.startTime).Seconds() + dur
		gfx.uiToasts = append(gfx.uiToasts, toastEntry{message: msg.Value, style: style, expiresAt: expiresAt})
		return NULL
	}}

	// tooltip(text) — show hover text for the widget drawn immediately before this call.
	// Appears after the cursor rests on the widget for 0.5 seconds.
	Builtins["tooltip"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("tooltip expects 1 argument: text", ast.Pos{})
		}
		txt, ok := args[0].(*String)
		if !ok {
			return typeError("tooltip: argument must be a string", ast.Pos{})
		}
		if gfx.uiLastElementID == "" || gfx.uiHoveredID != gfx.uiLastElementID {
			return NULL
		}
		gfx.uiTooltipMatchedThisFrame = true
		now := time.Since(gfx.startTime).Seconds()
		if gfx.uiTooltipHoveredID != gfx.uiLastElementID {
			gfx.uiTooltipHoveredID = gfx.uiLastElementID
			gfx.uiTooltipHoverStart = now
			return NULL
		}
		if now-gfx.uiTooltipHoverStart >= 0.5 {
			gfx.uiPendingTooltip = pendingTooltip{
				active: true,
				text:   txt.Value,
				mx:     float32(gfx.mouseX),
				my:     float32(gfx.mouseY),
			}
		}
		return NULL
	}}

	// image(img, x, y, w, h, [mode]) — draw an Image inside the UI.
	// mode: "fit" (default, letterbox+centre), "fill" (crop+clip), "stretch"
	Builtins["image"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("image expects 5-6 arguments: img, x, y, w, h, [mode]", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError("image: first argument must be an image from loadImage()", ast.Pos{})
		}
		if !allNumeric(args[1:5]) {
			return typeError("image: x, y, w, h must be numeric", ast.Pos{})
		}
		fx := float32(toFloat64(args[1]))
		fy := float32(toFloat64(args[2]))
		fw := float32(toFloat64(args[3]))
		fh := float32(toFloat64(args[4]))
		mode := "fit"
		if len(args) == 6 {
			m, ok := args[5].(*String)
			if !ok {
				return typeError("image: mode must be a string", ast.Pos{})
			}
			mode = m.Value
		}

		switch mode {
		case "stretch":
			drawImageGL(img, fx, fy, fw, fh)

		case "fill":
			scaleX := fw / float32(img.W)
			scaleY := fh / float32(img.H)
			scale := scaleX
			if scaleY > scale {
				scale = scaleY
			}
			dw := float32(img.W) * scale
			dh := float32(img.H) * scale
			dx := fx + (fw-dw)*0.5
			dy := fy + (fh-dh)*0.5
			imgClip := clipRect{fx, fy, fw, fh}
			if len(gfx.clipStack) > 0 {
				imgClip = intersectClip(gfx.clipStack[len(gfx.clipStack)-1], imgClip)
			}
			applyScissor(imgClip)
			drawImageGL(img, dx, dy, dw, dh)
			if len(gfx.clipStack) > 0 {
				applyScissor(gfx.clipStack[len(gfx.clipStack)-1])
			} else {
				gl.Disable(gl.SCISSOR_TEST)
			}

		default: // "fit"
			scaleX := fw / float32(img.W)
			scaleY := fh / float32(img.H)
			scale := scaleX
			if scaleY < scale {
				scale = scaleY
			}
			dw := float32(img.W) * scale
			dh := float32(img.H) * scale
			dx := fx + (fw-dw)*0.5
			dy := fy + (fh-dh)*0.5
			drawImageGL(img, dx, dy, dw, dh)
		}
		return NULL
	}}

	// button(label, x, y, w, h, ?size) → true if clicked this frame
	Builtins["button"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("button expects 5-6 arguments: label, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		w, ok4 := args[3].(*Integer)
		h, ok5 := args[4].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("button: label must be string, others must be integers", ast.Pos{})
		}

		var textScale float32 = 0.5
		if len(args) == 6 {
			if sizeInt, ok := args[5].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[5].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("button: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("btn_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)

		// Auto-expand button width so the label always fits with minimum padding.
		// This makes button sizing font-agnostic: callers don't need to know which
		// font is active or how wide each glyph is — the button just grows to fit.
		const btnLabelPad = float32(12) // minimum horizontal padding each side
		minW := uiTextWidth(label.Value, textScale) + btnLabelPad*2
		if fw < minW {
			fw = minW
		}

		isHovered := gfx.uiHoveredID == id
		isActive := gfx.uiActiveID == id && gfx.mouseDown
		isClicked := false

		if isHovered && gfx.mouseJustClicked {
			gfx.uiActiveID = id
			isClicked = true
		}

		bgColor := gfx.uiTheme.widgetBg
		if isHovered {
			bgColor = gfx.uiTheme.widgetBgHover
		}
		if isActive {
			bgColor = gfx.uiTheme.widgetBgActive
		}
		drawRoundedRectSDF(fx, fy, fw, fh, 4, bgColor, false, 0)

		savedFill := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.widgetText
		drawText(label.Value, int(fx+fw*0.5), int(fy+fh*0.5), true, textScale)
		gfx.fillColor = savedFill

		uiRegisterElement(id, fx, fy, fw, fh)
		return &Boolean{Value: isClicked}
	}}

	// label(text, x, y, ?size) → null
	Builtins["label"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return typeError("label expects 3-4 arguments: text, x, y, [size]", ast.Pos{})
		}
		text, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		if !ok1 || !ok2 || !ok3 {
			return typeError("label: text must be string, x/y must be integers", ast.Pos{})
		}

		var textScale float32 = 0.5
		if len(args) == 4 {
			if sizeInt, ok := args[3].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[3].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("label: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("lbl_%d", gfx.uiNextID)
		gfx.uiNextID++

		savedFill := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.labelText
		drawText(text.Value, int(x.Value), int(y.Value), false, textScale)
		gfx.fillColor = savedFill
		uiRegisterElement(id, float32(x.Value), float32(y.Value), 100, 20)
		return NULL
	}}

	// textInput(label, currentText, x, y, w, h, ?size) → new text
	Builtins["textInput"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("textInput expects 6-7 arguments: label, currentText, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		currentText, ok2 := args[1].(*String)
		x, ok3 := args[2].(*Integer)
		y, ok4 := args[3].(*Integer)
		w, ok5 := args[4].(*Integer)
		h, ok6 := args[5].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
			return typeError("textInput: label/text must be strings, others must be integers", ast.Pos{})
		}

		var textScale float32 = 0.5
		if len(args) == 7 {
			if sizeInt, ok := args[6].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[6].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("textInput: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("txt_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		isHovered := gfx.uiHoveredID == id
		isFocused := gfx.uiActiveID == id

		if isHovered && gfx.mouseJustClicked {
			gfx.uiActiveID = id
		}
		if gfx.mouseJustClicked && !isHovered && gfx.uiActiveID == id {
			gfx.uiActiveID = ""
		}

		newText := currentText.Value

		// ── Lazy-init cursor state ────────────────────────────────────────────
		if gfx.uiTextCursor == nil {
			gfx.uiTextCursor = make(map[string]int)
			gfx.uiTextAnchor = make(map[string]int)
			gfx.uiTextScroll = make(map[string]float32)
			gfx.uiTextBlink = make(map[string]float64)
			gfx.uiUndoStacks = make(map[string][]string)
			gfx.uiRedoStacks = make(map[string][]string)
		}
		cursor := gfx.uiTextCursor[id]
		anchor := gfx.uiTextAnchor[id]
		scroll := gfx.uiTextScroll[id]
		now := time.Since(gfx.startTime).Seconds()

		// runeLen returns the number of Unicode codepoints in s.
		runeLen := func(s string) int { return len([]rune(s)) }

		// Clamp in case text was changed programmatically between frames.
		rl := runeLen(newText)
		if cursor > rl { cursor = rl }
		if anchor > rl { anchor = rl }

		// deleteSelection removes the selected range using rune-safe slicing.
		deleteSelection := func(t string, cur, anc int) (string, int) {
			lo, hi := cur, anc
			if lo > hi { lo, hi = hi, lo }
			r := []rune(t)
			return string(append(append([]rune{}, r[:lo]...), r[hi:]...)), lo
		}
		hasSelection := cursor != anchor

		// ── Focus gained this frame — click to position ───────────────────────
		if isHovered && gfx.mouseJustClicked {
			isShiftClick := gfx.keys[glfw.KeyLeftShift] || gfx.keys[glfw.KeyRightShift]
			const pad = float32(5)
			clickOffset := float32(gfx.mouseX) - (fx + pad) + scroll
			newCursor := uiCharAtX(newText, clickOffset, textScale)
			if isShiftClick {
				cursor = newCursor
			} else {
				cursor = newCursor
				anchor = newCursor
			}
			gfx.uiTextBlink[id] = now
		}

		if isFocused {
			cmdOrCtrl := gfx.keys[glfw.KeyLeftSuper] || gfx.keys[glfw.KeyRightSuper] ||
				gfx.keys[glfw.KeyLeftControl] || gfx.keys[glfw.KeyRightControl]
			isShift := gfx.keys[glfw.KeyLeftShift] || gfx.keys[glfw.KeyRightShift]

			// ── Forward delete (Delete key) ───────────────────────────────────
			if gfx.justPressed[glfw.KeyDelete] {
				if hasSelection {
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
				} else if cursor < runeLen(newText) {
					r := []rune(newText)
					newText = string(append(append([]rune{}, r[:cursor]...), r[cursor+1:]...))
				}
				gfx.uiTextBlink[id] = now
				hasSelection = false
			}

			// ── Backspace ─────────────────────────────────────────────────────
			if gfx.uiBackspaceCount > 0 {
				if hasSelection {
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
					hasSelection = false
				} else {
					for i := 0; i < gfx.uiBackspaceCount; i++ {
						if cursor > 0 {
							r := []rune(newText)
							newText = string(append(append([]rune{}, r[:cursor-1]...), r[cursor:]...))
							cursor--
						}
					}
					anchor = cursor
				}
				gfx.uiTextBlink[id] = now
			}

			// ── Character insertion ───────────────────────────────────────────
			for _, ch := range gfx.charBuf {
				if ch >= 32 { // accept all printable Unicode
					if hasSelection {
						newText, cursor = deleteSelection(newText, cursor, anchor)
						anchor = cursor
						hasSelection = false
					}
					r := []rune(newText)
					r = append(append(append([]rune{}, r[:cursor]...), ch), r[cursor:]...)
					newText = string(r)
					cursor++
					anchor = cursor
				}
			}
			if len(gfx.charBuf) > 0 {
				gfx.uiTextBlink[id] = now
			}

			// ── Arrow keys ────────────────────────────────────────────────────
			if gfx.justPressed[glfw.KeyLeft] {
				if hasSelection && !isShift {
					if cursor > anchor { cursor = anchor }
					anchor = cursor
				} else if cmdOrCtrl {
					r := []rune(newText)
					i := cursor
					for i > 0 && r[i-1] == ' ' { i-- }
					for i > 0 && r[i-1] != ' ' { i-- }
					cursor = i
					if !isShift { anchor = cursor }
				} else {
					if cursor > 0 { cursor-- }
					if !isShift { anchor = cursor }
				}
				gfx.uiTextBlink[id] = now
			}
			if gfx.justPressed[glfw.KeyRight] {
				rl2 := runeLen(newText)
				if hasSelection && !isShift {
					if cursor < anchor { cursor = anchor }
					anchor = cursor
				} else if cmdOrCtrl {
					r := []rune(newText)
					i := cursor
					for i < rl2 && r[i] != ' ' { i++ }
					for i < rl2 && r[i] == ' ' { i++ }
					cursor = i
					if !isShift { anchor = cursor }
				} else {
					if cursor < rl2 { cursor++ }
					if !isShift { anchor = cursor }
				}
				gfx.uiTextBlink[id] = now
			}

			// ── Home / End ────────────────────────────────────────────────────
			if gfx.justPressed[glfw.KeyHome] {
				cursor = 0
				if !isShift { anchor = 0 }
				gfx.uiTextBlink[id] = now
			}
			if gfx.justPressed[glfw.KeyEnd] {
				cursor = runeLen(newText)
				if !isShift { anchor = cursor }
				gfx.uiTextBlink[id] = now
			}

			// ── Ctrl+A (select all) ───────────────────────────────────────────
			if cmdOrCtrl && gfx.justPressed[glfw.KeyA] {
				anchor = 0
				cursor = runeLen(newText)
				gfx.uiTextBlink[id] = now
			}

			// ── Clipboard ─────────────────────────────────────────────────────
			if cmdOrCtrl && gfx.justPressed[glfw.KeyV] {
				clip := gfx.win.GetClipboardString()
				filteredRunes := []rune{}
				for _, ch := range clip {
					if ch >= 32 {
						filteredRunes = append(filteredRunes, ch)
					}
				}
				if len(filteredRunes) > 0 {
					if hasSelection {
						newText, cursor = deleteSelection(newText, cursor, anchor)
						anchor = cursor
						hasSelection = false
					}
					r := []rune(newText)
					r = append(append(append([]rune{}, r[:cursor]...), filteredRunes...), r[cursor:]...)
					newText = string(r)
					cursor += len(filteredRunes)
					anchor = cursor
					gfx.uiTextBlink[id] = now
				}
			}
			if cmdOrCtrl && gfx.justPressed[glfw.KeyC] {
				if hasSelection {
					lo, hi := cursor, anchor
					if lo > hi { lo, hi = hi, lo }
					gfx.win.SetClipboardString(string([]rune(newText)[lo:hi]))
				} else {
					gfx.win.SetClipboardString(newText)
				}
			}
			if cmdOrCtrl && gfx.justPressed[glfw.KeyX] {
				if hasSelection {
					lo, hi := cursor, anchor
					if lo > hi { lo, hi = hi, lo }
					gfx.win.SetClipboardString(string([]rune(newText)[lo:hi]))
					newText, cursor = deleteSelection(newText, cursor, anchor)
					anchor = cursor
					hasSelection = false
				} else {
					gfx.win.SetClipboardString(newText)
					newText = ""
					cursor = 0
					anchor = 0
				}
				gfx.uiTextBlink[id] = now
			}

			// ── Undo / Redo ───────────────────────────────────────────────────
			isWordBoundary := false
			for _, ch := range gfx.charBuf {
				if ch == ' ' || ch == '.' || ch == ',' || ch == ';' || ch == ':' ||
					ch == '!' || ch == '?' || ch == '/' || ch == '\\' ||
					ch == '(' || ch == ')' || ch == '[' || ch == ']' {
					isWordBoundary = true
					break
				}
			}
			isDeleteOp := gfx.uiBackspaceCount > 0 || gfx.justPressed[glfw.KeyDelete]
			isPaste := cmdOrCtrl && gfx.justPressed[glfw.KeyV]
			isCut := cmdOrCtrl && gfx.justPressed[glfw.KeyX]

			if (isWordBoundary || isDeleteOp || isPaste || isCut) && newText != currentText.Value {
				stack := gfx.uiUndoStacks[id]
				stack = append(stack, currentText.Value)
				if len(stack) > 50 { stack = stack[1:] }
				gfx.uiUndoStacks[id] = stack
				gfx.uiRedoStacks[id] = nil
			}

			isUndo := cmdOrCtrl && gfx.justPressed[glfw.KeyZ] && !isShift
			isRedo := (cmdOrCtrl && gfx.justPressed[glfw.KeyZ] && isShift) ||
				(cmdOrCtrl && gfx.justPressed[glfw.KeyY])

			if isUndo {
				undoStack := gfx.uiUndoStacks[id]
				if len(undoStack) > 0 {
					redoStack := gfx.uiRedoStacks[id]
					redoStack = append(redoStack, newText)
					gfx.uiRedoStacks[id] = redoStack
					newText = undoStack[len(undoStack)-1]
					gfx.uiUndoStacks[id] = undoStack[:len(undoStack)-1]
					cursor = runeLen(newText)
					anchor = cursor
					gfx.uiTextBlink[id] = now
				}
			}
			if isRedo {
				redoStack := gfx.uiRedoStacks[id]
				if len(redoStack) > 0 {
					undoStack := gfx.uiUndoStacks[id]
					undoStack = append(undoStack, newText)
					gfx.uiUndoStacks[id] = undoStack
					newText = redoStack[len(redoStack)-1]
					gfx.uiRedoStacks[id] = redoStack[:len(redoStack)-1]
					cursor = runeLen(newText)
					anchor = cursor
					gfx.uiTextBlink[id] = now
				}
			}
		}

		// Clamp cursor after all edits.
		rl = runeLen(newText)
		if cursor > rl { cursor = rl }
		if anchor > rl { anchor = rl }

		// ── Scroll to keep cursor visible ─────────────────────────────────────
		const pad = float32(5)
		visibleW := fw - pad*2
		cursorX := uiMeasurePrefix(newText, cursor, textScale)
		if cursorX-scroll < 0 {
			scroll = cursorX
		}
		if cursorX-scroll > visibleW {
			scroll = cursorX - visibleW
		}

		// Persist cursor state.
		gfx.uiTextCursor[id] = cursor
		gfx.uiTextAnchor[id] = anchor
		gfx.uiTextScroll[id] = scroll

		// ── Draw ──────────────────────────────────────────────────────────────
		bgColor := gfx.uiTheme.inputBg
		if isFocused {
			bgColor = gfx.uiTheme.inputFocusBg
		}
		drawRoundedRectSDF(fx, fy, fw, fh, 3, bgColor, false, 0)

		var charH float32
		if gfx.uiActiveFont != nil {
			charH = gfx.uiActiveFont.LineH * textScale
		} else {
			charH = float32(gfx.fontCellH) * textScale
		}
		textY := int(fy + (fh-charH)*0.5)

		if label.Value != "" {
			drawText(label.Value, int(fx)-80, textY, false, textScale)
		}

		// Clip text + selection + caret to the field interior (stack-aware).
		tiClip := clipRect{fx + pad, fy, visibleW + pad, fh}
		if len(gfx.clipStack) > 0 {
			tiClip = intersectClip(gfx.clipStack[len(gfx.clipStack)-1], tiClip)
		}
		applyScissor(tiClip)

		savedFill := gfx.fillColor

		// Selection highlight.
		if cursor != anchor {
			lo, hi := cursor, anchor
			if lo > hi {
				lo, hi = hi, lo
			}
			selX0 := fx + pad + uiMeasurePrefix(newText, lo, textScale) - scroll
			selX1 := fx + pad + uiMeasurePrefix(newText, hi, textScale) - scroll
			selW := selX1 - selX0
			if selW > 0 {
				selY := fy + (fh-charH)*0.5
				drawRoundedRectSDF(selX0, selY, selW, charH, 0, gfx.uiTheme.accentBg, false, 0)
			}
		}

		// Text.
		gfx.fillColor = gfx.uiTheme.widgetText
		drawText(newText, int(fx+pad)-int(scroll), textY, false, textScale)
		gfx.fillColor = savedFill

		// Blinking caret.
		if isFocused {
			blinkStart := gfx.uiTextBlink[id]
			elapsed := now - blinkStart
			caretVisible := elapsed < 0.5 || math.Mod(elapsed, 1.0) < 0.5
			if caretVisible {
				caretX := fx + pad + cursorX - scroll
				caretY := fy + (fh-charH)*0.5
				drawRoundedRectSDF(caretX, caretY, 1.5, charH, 0, gfx.uiTheme.accent, false, 0)
			}
		}

		// Restore previous clip state after textInput internal drawing.
		if len(gfx.clipStack) > 0 {
			applyScissor(gfx.clipStack[len(gfx.clipStack)-1])
		} else {
			gl.Disable(gl.SCISSOR_TEST)
		}

		uiRegisterElement(id, fx, fy, fw, fh)
		return &String{Value: newText}
	}}

	// list(label, items[], x, y, w, h, ?size) → selected item string (with scrollbar)
	Builtins["list"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("list expects 6-7 arguments: label, items, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		x, ok3 := args[2].(*Integer)
		y, ok4 := args[3].(*Integer)
		w, ok5 := args[4].(*Integer)
		h, ok6 := args[5].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
			return typeError("list: label must be string, items must be array, others must be integers", ast.Pos{})
		}

		var textScale float32 = 0.5
		if len(args) == 7 {
			if sizeInt, ok := args[6].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[6].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("list: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("list_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		numItems := len(itemsObj.Elements)
		if numItems == 0 {
			return NULL
		}

		const itemHeightPx = 24.0
		visibleItems := int(fh / itemHeightPx)
		if visibleItems < 1 {
			visibleItems = 1
		}

		scrollPos := gfx.uiListScroll[id]

		if gfx.uiScrollDelta != 0 {
			mx, my := gfx.mouseX, gfx.mouseY
			if mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(fy) && my <= float64(fy+fh) {
				scrollPos -= int(gfx.uiScrollDelta * 3)
				if scrollPos < 0 {
					scrollPos = 0
				}
				if scrollPos > numItems-visibleItems {
					scrollPos = numItems - visibleItems
				}
				gfx.uiListScroll[id] = scrollPos
			}
		}

		selectedIdx := gfx.uiListSelected[id]
		if selectedIdx >= numItems {
			selectedIdx = 0
		}

		if label.Value != "" {
			drawText(label.Value, int(fx), int(fy-20), false, textScale)
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		scrollbarW := float32(12.0)
		if numItems > visibleItems {
			scrollbarX := fx + fw - scrollbarW - 2
			scrollbarH := fh * float32(visibleItems) / float32(numItems)
			scrollbarY := fy + float32(scrollPos)*fh/float32(numItems)
			drawRoundedRectSDF(scrollbarX, scrollbarY, scrollbarW, scrollbarH, 2, gfx.uiTheme.accentBg, false, 0)
		}

		charH := uiCharH(textScale)
		savedFillList := gfx.fillColor

		for i := 0; i < visibleItems && (scrollPos+i) < numItems; i++ {
			itemIdx := scrollPos + i
			itemY := fy + float32(i)*itemHeightPx
			itemText := ""

			if str, ok := itemsObj.Elements[itemIdx].(*String); ok {
				itemText = str.Value
			} else {
				itemText = itemsObj.Elements[itemIdx].Inspect()
			}

			listW := fw - 15
			isItemHovered := gfx.mouseX >= float64(fx) && gfx.mouseX <= float64(fx+listW) &&
				gfx.mouseY >= float64(itemY) && gfx.mouseY <= float64(itemY+itemHeightPx)

			if isItemHovered && gfx.mouseJustClicked {
				selectedIdx = itemIdx
				gfx.uiListSelected[id] = itemIdx
			}

			itemBgColor := gfx.uiTheme.track
			if itemIdx == selectedIdx {
				itemBgColor = gfx.uiTheme.accentBg
			} else if isItemHovered {
				itemBgColor = gfx.uiTheme.widgetBg
			}
			drawRoundedRectSDF(fx, itemY, listW, itemHeightPx, 2, itemBgColor, false, 0)

			if itemIdx == selectedIdx {
				gfx.fillColor = gfx.uiTheme.widgetText
			} else if isItemHovered {
				gfx.fillColor = gfx.uiTheme.labelText
			} else {
				gfx.fillColor = gfx.uiTheme.dimText
			}
			drawText(itemText, int(fx+5), int(itemY+(itemHeightPx-charH)*0.5), false, textScale)

			uiRegisterElement(fmt.Sprintf("%s_item_%d", id, itemIdx), fx, itemY, listW, itemHeightPx)
		}
		gfx.fillColor = savedFillList

		if selectedIdx >= 0 && selectedIdx < numItems {
			return itemsObj.Elements[selectedIdx]
		}
		return NULL
	}}

	// listMulti(label, items, selected, x, y, w, h, ?size) → selected[]
	// selected is a bool array (one entry per item). Click toggles. Returns updated array.
	Builtins["listMulti"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("listMulti expects 7-8 arguments: label, items, selected, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		selObj, ok3 := args[2].(*Array)
		x, ok4 := args[3].(*Integer)
		y, ok5 := args[4].(*Integer)
		w, ok6 := args[5].(*Integer)
		h, ok7 := args[6].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
			return typeError("listMulti: wrong argument types", ast.Pos{})
		}

		var textScale float32 = 0.5
		if len(args) == 8 {
			if sizeInt, ok := args[7].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[7].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("listMulti: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("listm_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		numItems := len(itemsObj.Elements)

		// Build a working bool slice, extending/trimming to match items length.
		sel := make([]bool, numItems)
		for i := 0; i < numItems && i < len(selObj.Elements); i++ {
			if b, ok := selObj.Elements[i].(*Boolean); ok {
				sel[i] = b.Value
			}
		}

		const itemH = 24.0
		visibleItems := int(fh / itemH)
		if visibleItems < 1 {
			visibleItems = 1
		}

		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}
		scrollPos := gfx.uiListScroll[id]

		if gfx.uiScrollDelta != 0 {
			mx, my := gfx.mouseX, gfx.mouseY
			if mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(fy) && my <= float64(fy+fh) {
				scrollPos -= int(gfx.uiScrollDelta * 3)
				if scrollPos < 0 {
					scrollPos = 0
				}
				maxScroll := numItems - visibleItems
				if maxScroll < 0 {
					maxScroll = 0
				}
				if scrollPos > maxScroll {
					scrollPos = maxScroll
				}
				gfx.uiListScroll[id] = scrollPos
			}
		}

		if label.Value != "" {
			savedFillLbl := gfx.fillColor
			gfx.fillColor = gfx.uiTheme.labelText
			drawText(label.Value, int(fx), int(fy-20), false, textScale)
			gfx.fillColor = savedFillLbl
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		listW := fw - 15
		scrollbarW := float32(12.0)
		if numItems > visibleItems {
			scrollbarX := fx + fw - scrollbarW - 2
			scrollbarH := fh * float32(visibleItems) / float32(numItems)
			scrollbarY := fy + float32(scrollPos)*fh/float32(numItems)
			drawRoundedRectSDF(scrollbarX, scrollbarY, scrollbarW, scrollbarH, 2, gfx.uiTheme.accentBg, false, 0)
		}

		charH := uiCharH(textScale)
		savedFillM := gfx.fillColor
		const tickW = float32(4.0)

		for i := 0; i < visibleItems && (scrollPos+i) < numItems; i++ {
			itemIdx := scrollPos + i
			itemY := fy + float32(i)*itemH

			itemText := ""
			if str, ok := itemsObj.Elements[itemIdx].(*String); ok {
				itemText = str.Value
			} else {
				itemText = itemsObj.Elements[itemIdx].Inspect()
			}

			isHovered := gfx.mouseX >= float64(fx) && gfx.mouseX <= float64(fx+listW) &&
				gfx.mouseY >= float64(itemY) && gfx.mouseY <= float64(itemY+itemH)

			if isHovered && gfx.mouseJustClicked {
				sel[itemIdx] = !sel[itemIdx]
			}

			isSelected := sel[itemIdx]

			var rowBg [4]float32
			if isSelected {
				rowBg = gfx.uiTheme.accentBg
			} else if isHovered {
				rowBg = gfx.uiTheme.widgetBg
			} else {
				rowBg = gfx.uiTheme.track
			}
			drawRoundedRectSDF(fx, itemY, listW, itemH, 2, rowBg, false, 0)

			// Left accent tick for selected rows.
			if isSelected {
				drawRoundedRectSDF(fx, itemY, tickW, itemH, 2, gfx.uiTheme.accent, false, 0)
			}

			if isSelected {
				gfx.fillColor = gfx.uiTheme.widgetText
			} else if isHovered {
				gfx.fillColor = gfx.uiTheme.labelText
			} else {
				gfx.fillColor = gfx.uiTheme.dimText
			}
			drawText(itemText, int(fx+tickW+5), int(itemY+(itemH-charH)*0.5), false, textScale)

			uiRegisterElement(fmt.Sprintf("%s_item_%d", id, itemIdx), fx, itemY, listW, itemH)
		}
		gfx.fillColor = savedFillM

		// Return updated bool array.
		out := make([]Object, numItems)
		for i, v := range sel {
			out[i] = &Boolean{Value: v}
		}
		return &Array{Elements: out}
	}}

	// checkbox(label, x, y, checked, ?size) → bool
	Builtins["checkbox"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return typeError("checkbox expects 4-5 arguments: label, x, y, checked, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		checked, ok4 := args[3].(*Boolean)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("checkbox: label must be string, x/y must be integers, checked must be bool", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 5 {
			if sizeInt, ok := args[4].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[4].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("checkbox: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("chk_%d", gfx.uiNextID)
		gfx.uiNextID++

		const boxSize = 18.0
		fx, fy := float32(x.Value), float32(y.Value)
		isHovered := gfx.uiHoveredID == id
		newChecked := checked.Value
		if isHovered && gfx.mouseJustClicked {
			newChecked = !newChecked
		}

		boxColor := gfx.uiTheme.widgetBgActive
		if newChecked {
			boxColor = gfx.uiTheme.widgetBgHover
		} else if isHovered {
			boxColor = gfx.uiTheme.widgetBg
		}
		drawRoundedRectSDF(fx, fy, boxSize, boxSize, 3, boxColor, false, 0)
		if newChecked {
			drawRoundedRectSDF(fx+4, fy+4, boxSize-8, boxSize-8, 2, gfx.uiTheme.handle, false, 0)
		}

		charH := uiCharH(textScale)
		savedFillChk := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.labelText
		drawText(label.Value, int(fx+boxSize+6), int(fy+(boxSize-charH)*0.5), false, textScale)
		gfx.fillColor = savedFillChk

		hitW := boxSize + 6 + float32(len(label.Value))*float32(gfx.fontCellW)*textScale
		uiRegisterElement(id, fx, fy, hitW, boxSize)
		return &Boolean{Value: newChecked}
	}}

	// slider(label, x, y, w, value, min, max, ?size) → float
	Builtins["slider"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("slider expects 7-8 arguments: label, x, y, w, value, min, max, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		w, ok4 := args[3].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("slider: label must be string, x/y/w must be integers", ast.Pos{})
		}
		if !canArithmetic(args[4].Type()) || !canArithmetic(args[5].Type()) || !canArithmetic(args[6].Type()) {
			return typeError("slider: value, min, max must be numeric", ast.Pos{})
		}
		value := toFloat64(args[4])
		minVal := toFloat64(args[5])
		maxVal := toFloat64(args[6])
		var textScale float32 = 0.5
		if len(args) == 8 {
			if sizeInt, ok := args[7].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[7].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("slider: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("sld_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		const trackH = 6.0
		const handleR = 8.0
		trackY := fy + handleR - trackH*0.5

		isDragging := gfx.uiActiveID == id && gfx.mouseDown
		isHit := gfx.mouseX >= float64(fx-handleR) && gfx.mouseX <= float64(fx+fw+handleR) &&
			gfx.mouseY >= float64(fy) && gfx.mouseY <= float64(fy+handleR*2)

		if isHit && gfx.mouseJustClicked {
			gfx.uiActiveID = id
			isDragging = true
		}
		if !gfx.mouseDown && gfx.uiActiveID == id {
			gfx.uiActiveID = ""
		}

		if isDragging {
			t := (gfx.mouseX - float64(fx)) / float64(fw)
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			value = minVal + t*(maxVal-minVal)
		}
		if value < minVal {
			value = minVal
		}
		if value > maxVal {
			value = maxVal
		}

		t := float32(0)
		if maxVal > minVal {
			t = float32((value - minVal) / (maxVal - minVal))
		}

		if label.Value != "" {
			charH := uiCharH(textScale)
			savedFillSld := gfx.fillColor
			gfx.fillColor = gfx.uiTheme.labelText
			drawText(label.Value+": "+fmt.Sprintf("%.1f", value), int(fx), int(fy-charH-4), false, textScale)
			gfx.fillColor = savedFillSld
		}

		drawRoundedRectSDF(fx, trackY, fw, trackH, trackH*0.5, gfx.uiTheme.track, false, 0)
		if t > 0.001 {
			drawRoundedRectSDF(fx, trackY, fw*t, trackH, trackH*0.5, gfx.uiTheme.trackFill, false, 0)
		}

		hx := fx + fw*t
		handleColor := gfx.uiTheme.handle
		if isDragging {
			handleColor = gfx.uiTheme.widgetText
		}
		drawRoundedRectSDF(hx-handleR, fy, handleR*2, handleR*2, handleR, handleColor, false, 0)

		uiRegisterElement(id, fx-handleR, fy, fw+handleR*2, handleR*2)
		return &Float{Value: value}
	}}

	// progressBar(x, y, w, h, value, max) → null
	Builtins["progressBar"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 {
			return typeError("progressBar expects 6 arguments: x, y, w, h, value, max", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		w, ok3 := args[2].(*Integer)
		h, ok4 := args[3].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("progressBar: x, y, w, h must be integers", ast.Pos{})
		}
		if !canArithmetic(args[4].Type()) || !canArithmetic(args[5].Type()) {
			return typeError("progressBar: value and max must be numeric", ast.Pos{})
		}
		value := toFloat64(args[4])
		maxVal := toFloat64(args[5])

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		t := float32(0)
		if maxVal > 0 {
			t = float32(value / maxVal)
		}
		if t > 1 {
			t = 1
		}
		if t < 0 {
			t = 0
		}

		r := fh * 0.5
		drawRoundedRectSDF(fx, fy, fw, fh, r, gfx.uiTheme.track, false, 0)
		if t > 0.001 {
			fillW := fw * t
			if fillW < fh {
				fillW = fh
			}
			drawRoundedRectSDF(fx, fy, fillW, fh, r, gfx.uiTheme.trackFill, false, 0)
		}
		return NULL
	}}

	// dropdown(label, items[], x, y, w, ?size) → selected item
	// Call after other widgets so the open menu renders on top.
	Builtins["dropdown"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("dropdown expects 5-6 arguments: label, items, x, y, w, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		itemsObj, ok2 := args[1].(*Array)
		x, ok3 := args[2].(*Integer)
		y, ok4 := args[3].(*Integer)
		w, ok5 := args[4].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("dropdown: label must be string, items must be array, x/y/w must be integers", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 6 {
			if sizeInt, ok := args[5].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[5].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("dropdown: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("dd_%d", gfx.uiNextID)
		gfx.uiNextID++

		numItems := len(itemsObj.Elements)
		if numItems == 0 {
			return &String{Value: ""}
		}

		if gfx.uiListSelected == nil {
			gfx.uiListSelected = make(map[string]int)
		}
		selectedIdx := gfx.uiListSelected[id]
		if selectedIdx >= numItems {
			selectedIdx = 0
		}

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		const headerH = 28.0
		const itemH = 24.0

		isOpen := uiDropdownOpen == id
		isHovered := gfx.uiHoveredID == id

		// Compute popup position once — upward-flip when the menu would overflow
		// the window bottom. Shared by the outside-click guard and item handler.
		popupMenuH := float32(numItems) * itemH
		popupMenuY := fy + headerH
		if popupMenuY+popupMenuH > float32(gfx.winH) {
			popupMenuY = fy - popupMenuH
			if popupMenuY < 0 {
				popupMenuY = 0
			}
		}

		if isHovered && gfx.mouseJustClicked {
			if isOpen {
				uiDropdownOpen = ""
				isOpen = false
			} else {
				uiDropdownOpen = id
				isOpen = true
			}
		} else if isOpen && gfx.mouseJustClicked {
			mx, my := gfx.mouseX, gfx.mouseY
			inHeader := mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(fy) && my <= float64(fy+headerH)
			inMenu := mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(popupMenuY) && my <= float64(popupMenuY+popupMenuH)
			if !inHeader && !inMenu {
				uiDropdownOpen = ""
				isOpen = false
			}
		}

		var charH float32
		if gfx.uiActiveFont != nil {
			charH = gfx.uiActiveFont.LineH * textScale
		} else {
			charH = float32(gfx.fontCellH) * textScale
		}
		savedFillDD := gfx.fillColor
		if label.Value != "" {
			drawText(label.Value, int(fx), int(fy-charH-4), false, textScale)
		}

		headerColor := gfx.uiTheme.widgetBgActive
		if isOpen || isHovered {
			headerColor = gfx.uiTheme.widgetBg
		}
		drawRoundedRectSDF(fx, fy, fw, headerH, 4, headerColor, false, 0)

		textY := int(fy + (headerH-charH)*0.5)
		gfx.fillColor = gfx.uiTheme.widgetText
		if selectedIdx < numItems {
			selText := ""
			if str, ok := itemsObj.Elements[selectedIdx].(*String); ok {
				selText = str.Value
			} else {
				selText = itemsObj.Elements[selectedIdx].Inspect()
			}
			drawText(selText, int(fx+8), textY, false, textScale)
		}
		gfx.fillColor = gfx.uiTheme.dimText
		drawText("v", int(fx+fw-float32(gfx.fontCellW)*textScale-8), textY, false, textScale)
		uiRegisterElement(id, fx, fy, fw, headerH)

		if isOpen {
			// Handle item clicks inline (needed for correct return value this frame),
			// then defer all drawing to uiEnd() so the popup renders on top.
			// popupMenuY / popupMenuH already account for the upward-flip.
			for i := 0; i < numItems; i++ {
				itemY := popupMenuY + float32(i)*itemH
				isItemHovered := gfx.mouseX >= float64(fx) && gfx.mouseX <= float64(fx+fw) &&
					gfx.mouseY >= float64(itemY) && gfx.mouseY <= float64(itemY+itemH)
				if isItemHovered && gfx.mouseJustClicked {
					selectedIdx = i
					gfx.uiListSelected[id] = i
					uiDropdownOpen = ""
					isOpen = false
				}
			}
			if isOpen {
				items := make([]string, numItems)
				for i := 0; i < numItems; i++ {
					if str, ok := itemsObj.Elements[i].(*String); ok {
						items[i] = str.Value
					} else {
						items[i] = itemsObj.Elements[i].Inspect()
					}
				}
				gfx.uiPendingDropdown = pendingDropdown{
					active:      true,
					id:          id,
					fx:          fx,
					fy:          popupMenuY,
					fw:          fw,
					items:       items,
					selectedIdx: selectedIdx,
					charH:       charH,
					textScale:   textScale,
				}
			}
		}
		gfx.fillColor = savedFillDD

		if selectedIdx >= 0 && selectedIdx < numItems {
			return itemsObj.Elements[selectedIdx]
		}
		return &String{Value: ""}
	}}

	// toggle(label, x, y, on, [size]) → bool
	Builtins["toggle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return typeError("toggle expects 4-5 arguments: label, x, y, on, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		onObj, ok4 := args[3].(*Boolean)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("toggle: label must be string, x/y must be integers, on must be bool", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 5 {
			if sizeInt, ok := args[4].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[4].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("toggle: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("tgl_%d", gfx.uiNextID)
		gfx.uiNextID++

		on := onObj.Value
		const trackW = 44.0
		const trackH = 22.0
		const handleR = 9.0

		fx, fy := float32(x.Value), float32(y.Value)
		isHovered := gfx.uiHoveredID == id
		if isHovered && gfx.mouseJustClicked {
			on = !on
		}

		// Pill track
		var trackColor [4]float32
		if on {
			trackColor = gfx.uiTheme.widgetBgHover
		} else {
			trackColor = gfx.uiTheme.widgetBgActive
		}
		drawRoundedRectSDF(fx, fy, trackW, trackH, trackH*0.5, trackColor, false, 0)

		// Handle — slides left (off) or right (on)
		var hx float32
		if on {
			hx = fx + trackW - handleR - 3
		} else {
			hx = fx + handleR + 3
		}
		hy := fy + trackH*0.5

		if on {
			glow := gfx.uiTheme.handle
			glow[3] = 0.15
			drawRoundedRectSDF(hx-handleR-4, hy-handleR-4, (handleR+4)*2, (handleR+4)*2, handleR+4, glow, false, 0)
		}
		var handleColor [4]float32
		if on {
			handleColor = gfx.uiTheme.handle
		} else {
			handleColor = gfx.uiTheme.trackFill
		}
		drawRoundedRectSDF(hx-handleR, hy-handleR, handleR*2, handleR*2, handleR, handleColor, false, 0)

		// Label
		savedFill := gfx.fillColor
		if on {
			gfx.fillColor = gfx.uiTheme.widgetText
		} else {
			gfx.fillColor = gfx.uiTheme.dimText
		}
		charH := float32(gfx.fontCellH) * textScale
		drawText(label.Value, int(fx+trackW+10), int(fy+(trackH-charH)*0.5), false, textScale)
		gfx.fillColor = savedFill

		uiRegisterElement(id, fx, fy, trackW, trackH)
		return &Boolean{Value: on}
	}}

	// radio(label, x, y, value, groupValue, [size]) → string
	// Returns value if clicked, otherwise returns groupValue unchanged.
	// Call once per option in the group; chain the returned groupValue through each call.
	Builtins["radio"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("radio expects 5-6 arguments: label, x, y, value, groupValue, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		value, ok4 := args[3].(*String)
		groupValue, ok5 := args[4].(*String)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("radio: label/value/groupValue must be strings, x/y must be integers", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 6 {
			if sizeInt, ok := args[5].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[5].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("radio: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("rad_%d", gfx.uiNextID)
		gfx.uiNextID++

		selected := value.Value == groupValue.Value
		const r = 9.0
		fx, fy := float32(x.Value), float32(y.Value)
		cx := fx + r
		cy := fy + r

		isHovered := gfx.uiHoveredID == id
		newGroupValue := groupValue.Value
		if isHovered && gfx.mouseJustClicked {
			newGroupValue = value.Value
			selected = true
		}

		// Outer ring
		var ringColor [4]float32
		if selected {
			ringColor = gfx.uiTheme.accent
		} else if isHovered {
			ringColor = gfx.uiTheme.accentBg
		} else {
			ringColor = gfx.uiTheme.widgetBg
		}
		drawRoundedRectSDF(cx-r, cy-r, r*2, r*2, r, ringColor, false, 0)

		// Inner dot when selected
		if selected {
			const dotR = 4.5
			drawRoundedRectSDF(cx-dotR, cy-dotR, dotR*2, dotR*2, dotR, gfx.uiTheme.handle, false, 0)
		}

		// Label
		savedFill := gfx.fillColor
		charH := float32(gfx.fontCellH) * textScale
		charW := float32(gfx.fontCellW) * textScale
		if selected {
			gfx.fillColor = gfx.uiTheme.widgetText
		} else {
			gfx.fillColor = gfx.uiTheme.dimText
		}
		drawText(label.Value, int(cx+r+8), int(cy-charH*0.5), false, textScale)
		gfx.fillColor = savedFill

		hitW := r*2 + 8 + float32(len(label.Value))*charW
		uiRegisterElement(id, fx, fy, hitW, r*2)
		return &String{Value: newGroupValue}
	}}

	// numericStepper(label, x, y, w, value, min, max, [size]) → int
	Builtins["numericStepper"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("numericStepper expects 7-8 arguments: label, x, y, w, value, min, max, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		x, ok2 := args[1].(*Integer)
		y, ok3 := args[2].(*Integer)
		w, ok4 := args[3].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("numericStepper: label must be string, x/y/w must be integers", ast.Pos{})
		}
		if !canArithmetic(args[4].Type()) || !canArithmetic(args[5].Type()) || !canArithmetic(args[6].Type()) {
			return typeError("numericStepper: value, min, max must be numeric", ast.Pos{})
		}
		value := int(toFloat64(args[4]))
		minVal := int(toFloat64(args[5]))
		maxVal := int(toFloat64(args[6]))

		var textScale float32 = 0.5
		if len(args) == 8 {
			if sizeInt, ok := args[7].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[7].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("numericStepper: size must be a number", ast.Pos{})
			}
		}

		baseID := gfx.uiNextID
		gfx.uiNextID++

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		charH := uiCharH(textScale)
		charW := float32(gfx.fontCellW) * textScale
		h := charH + 14.0
		const btnW = 32.0

		minusID := fmt.Sprintf("ns_m_%d", baseID)
		plusID := fmt.Sprintf("ns_p_%d", baseID)

		isMinusHovered := gfx.uiHoveredID == minusID
		isPlusHovered := gfx.uiHoveredID == plusID

		if isMinusHovered && gfx.mouseJustClicked {
			value--
			if value < minVal {
				value = minVal
			}
		}
		if isPlusHovered && gfx.mouseJustClicked {
			value++
			if value > maxVal {
				value = maxVal
			}
		}

		// Minus button
		var minusBg [4]float32
		if isMinusHovered {
			minusBg = gfx.uiTheme.accentBg
		} else {
			minusBg = gfx.uiTheme.widgetBgActive
		}
		drawRoundedRectSDF(fx, fy, btnW, h, 4, minusBg, false, 0)

		// Value display
		drawRoundedRectSDF(fx+btnW, fy, fw-btnW*2, h, 0, gfx.uiTheme.inputBg, false, 0)

		// Plus button
		var plusBg [4]float32
		if isPlusHovered {
			plusBg = gfx.uiTheme.accentBg
		} else {
			plusBg = gfx.uiTheme.widgetBgActive
		}
		drawRoundedRectSDF(fx+fw-btnW, fy, btnW, h, 4, plusBg, false, 0)

		savedFill := gfx.fillColor

		// Draw − and + symbols
		gfx.fillColor = gfx.uiTheme.widgetText
		drawText("-", int(fx+btnW*0.5-charW*0.5), int(fy+(h-charH)*0.5), false, textScale)
		drawText("+", int(fx+fw-btnW+btnW*0.5-charW*0.5), int(fy+(h-charH)*0.5), false, textScale)

		// Draw value centred in the middle panel
		valStr := fmt.Sprintf("%d", value)
		gfx.fillColor = gfx.uiTheme.widgetText
		midW := fw - btnW*2
		valX := int(fx + btnW + (midW-float32(len(valStr))*charW)*0.5)
		drawText(valStr, valX, int(fy+(h-charH)*0.5), false, textScale)

		// Label above
		if label.Value != "" {
			gfx.fillColor = gfx.uiTheme.labelText
			drawText(label.Value, int(fx), int(fy-charH-4), false, textScale)
		}

		gfx.fillColor = savedFill

		uiRegisterElement(minusID, fx, fy, btnW, h)
		uiRegisterElement(plusID, fx+fw-btnW, fy, btnW, h)
		return &Integer{Value: value}
	}}

	// tabs(x, y, w, items[], activeIdx, [size]) → int
	Builtins["tabs"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("tabs expects 5-6 arguments: x, y, w, items, activeIdx, [size]", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		w, ok3 := args[2].(*Integer)
		itemsObj, ok4 := args[3].(*Array)
		activeObj, ok5 := args[4].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("tabs: x/y/w/activeIdx must be integers, items must be array", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 6 {
			if sizeInt, ok := args[5].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[5].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("tabs: size must be a number", ast.Pos{})
			}
		}

		numTabs := len(itemsObj.Elements)
		if numTabs == 0 {
			return &Integer{Value: 0}
		}

		activeIdx := int(activeObj.Value)
		if activeIdx < 0 {
			activeIdx = 0
		}
		if activeIdx >= numTabs {
			activeIdx = numTabs - 1
		}

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		charH := float32(gfx.fontCellH) * textScale
		charW := float32(gfx.fontCellW) * textScale
		tabH := charH + 18.0
		tabW := fw / float32(numTabs)
		baseID := gfx.uiNextID
		gfx.uiNextID++

		// Shared strip background — what makes this a tab bar, not buttons
		drawRoundedRectSDF(fx, fy, fw, tabH, 0, gfx.uiTheme.inputBg, false, 0)

		// Full-width accent line at the bottom of the bar
		accentLine := gfx.uiTheme.accent
		accentLine[3] = 0.55
		drawRoundedRectSDF(fx, fy+tabH-2, fw, 2, 0, accentLine, false, 0)

		savedFill := gfx.fillColor

		for i := 0; i < numTabs; i++ {
			id := fmt.Sprintf("tab_%d_%d", baseID, i)
			tx := fx + float32(i)*tabW

			tabText := ""
			if str, ok := itemsObj.Elements[i].(*String); ok {
				tabText = str.Value
			} else {
				tabText = itemsObj.Elements[i].Inspect()
			}

			isActive := i == activeIdx
			isHovered := gfx.uiHoveredID == id
			if isHovered && gfx.mouseJustClicked {
				activeIdx = i
			}

			if isActive {
				// Active tab: lifted fill that covers the bottom accent line
				drawRoundedRectSDF(tx+1, fy+1, tabW-2, tabH-1, 0, gfx.uiTheme.widgetBg, false, 0)
				// Top accent stripe — the clear "this tab is active" signal
				drawRoundedRectSDF(tx+4, fy+1, tabW-8, 3, 1, gfx.uiTheme.accent, false, 0)
				gfx.fillColor = gfx.uiTheme.widgetText
			} else if isHovered {
				drawRoundedRectSDF(tx+1, fy+1, tabW-2, tabH-3, 0, gfx.uiTheme.track, false, 0)
				gfx.fillColor = gfx.uiTheme.labelText
			} else {
				gfx.fillColor = gfx.uiTheme.dimText
			}

			// Thin vertical divider between tabs (skip after last)
			if i < numTabs-1 {
				drawRoundedRectSDF(tx+tabW-1, fy+5, 1, tabH-10, 0, gfx.uiTheme.widgetBgActive, false, 0)
			}

			textX := int(tx + (tabW-float32(len(tabText))*charW)*0.5)
			textY := int(fy + (tabH-charH)*0.5)
			drawText(tabText, textX, textY, false, textScale)
			uiRegisterElement(id, tx, fy, tabW, tabH)
		}

		gfx.fillColor = savedFill
		return &Integer{Value: activeIdx}
	}}

	// textArea(label, text, x, y, w, h, [size]) → string
	Builtins["textArea"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("textArea expects 6-7 arguments: label, text, x, y, w, h, [size]", ast.Pos{})
		}
		label, ok1 := args[0].(*String)
		currentText, ok2 := args[1].(*String)
		x, ok3 := args[2].(*Integer)
		y, ok4 := args[3].(*Integer)
		w, ok5 := args[4].(*Integer)
		h, ok6 := args[5].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
			return typeError("textArea: label/text must be strings, x/y/w/h must be integers", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 7 {
			if sizeInt, ok := args[6].(*Integer); ok {
				textScale = float32(sizeInt.Value) / 100.0
			} else if sizeFloat, ok := args[6].(*Float); ok {
				textScale = float32(sizeFloat.Value)
			} else {
				return typeError("textArea: size must be a number", ast.Pos{})
			}
		}

		id := fmt.Sprintf("ta_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		isHovered := gfx.uiHoveredID == id
		isFocused := gfx.uiActiveID == id

		if isHovered && gfx.mouseJustClicked {
			gfx.uiActiveID = id
			isFocused = true
		} else if !isHovered && gfx.mouseJustClicked && gfx.uiActiveID == id {
			gfx.uiActiveID = ""
			isFocused = false
		}

		newText := currentText.Value
		if isFocused {
			for i := 0; i < gfx.uiBackspaceCount; i++ {
				runes := []rune(newText)
				if len(runes) > 0 {
					newText = string(runes[:len(runes)-1])
				}
			}
			if gfx.justPressed[glfw.KeyEnter] {
				newText += "\n"
			}
			for _, ch := range gfx.charBuf {
				if ch >= 32 && ch < 127 {
					newText += string(ch)
				}
			}
			cmdOrCtrl := gfx.keys[glfw.KeyLeftSuper] || gfx.keys[glfw.KeyRightSuper] ||
				gfx.keys[glfw.KeyLeftControl] || gfx.keys[glfw.KeyRightControl]
			if cmdOrCtrl && gfx.justPressed[glfw.KeyV] {
				clip := gfx.win.GetClipboardString()
				for _, ch := range clip {
					if ch == '\n' || (ch >= 32 && ch < 127) {
						newText += string(ch)
					}
				}
			}
			if cmdOrCtrl && gfx.justPressed[glfw.KeyC] {
				gfx.win.SetClipboardString(newText)
			}
			if cmdOrCtrl && gfx.justPressed[glfw.KeyX] {
				gfx.win.SetClipboardString(newText)
				newText = ""
			}
		}

		bgColor := gfx.uiTheme.inputBg
		if isFocused {
			bgColor = gfx.uiTheme.inputFocusBg
		}
		drawRoundedRectSDF(fx, fy, fw, fh, 4, bgColor, false, 0)

		var charH float32
		if gfx.uiActiveFont != nil {
			charH = gfx.uiActiveFont.LineH * textScale
		} else {
			charH = float32(gfx.fontCellH) * textScale
		}
		charW := float32(gfx.fontCellW) * textScale
		if label.Value != "" {
			drawText(label.Value, int(fx), int(fy-charH-4), false, textScale)
		}

		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}

		lines := strings.Split(newText, "\n")
		lineH := charH + 4.0
		visibleLines := int(fh / lineH)
		if visibleLines < 1 {
			visibleLines = 1
		}
		maxScroll := len(lines) - visibleLines
		if maxScroll < 0 {
			maxScroll = 0
		}

		scrollOff := gfx.uiListScroll[id]
		if scrollOff > maxScroll {
			scrollOff = maxScroll
		}

		if gfx.uiScrollDelta != 0 &&
			gfx.mouseX >= float64(fx) && gfx.mouseX <= float64(fx+fw) &&
			gfx.mouseY >= float64(fy) && gfx.mouseY <= float64(fy+fh) {
			scrollOff -= int(gfx.uiScrollDelta * 3)
			if scrollOff < 0 {
				scrollOff = 0
			}
			if scrollOff > maxScroll {
				scrollOff = maxScroll
			}
			gfx.uiListScroll[id] = scrollOff
		}

		// Keep cursor (end of text) visible when focused
		if isFocused {
			lastLineIdx := len(lines) - 1
			if lastLineIdx < scrollOff {
				scrollOff = lastLineIdx
				gfx.uiListScroll[id] = scrollOff
			}
			if lastLineIdx >= scrollOff+visibleLines {
				scrollOff = lastLineIdx - visibleLines + 1
				gfx.uiListScroll[id] = scrollOff
			}
		}

		savedFillTA := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.widgetText
		for i := 0; i < visibleLines && (scrollOff+i) < len(lines); i++ {
			lineY := fy + float32(i)*lineH + 4.0
			drawText(lines[scrollOff+i], int(fx+6), int(lineY), false, textScale)
		}
		gfx.fillColor = savedFillTA

		// Blinking cursor at end of last line
		if isFocused {
			lastLineIdx := len(lines) - 1
			cursorLineInView := lastLineIdx - scrollOff
			if cursorLineInView >= 0 && cursorLineInView < visibleLines {
				lastLine := lines[lastLineIdx]
				cursorX := fx + 6.0 + float32(len(lastLine))*charW
				cursorY := fy + float32(cursorLineInView)*lineH + 4.0
				if int(time.Since(gfx.startTime).Seconds()*2)%2 == 0 {
					drawRoundedRectSDF(cursorX, cursorY, 2, charH, 1, gfx.uiTheme.handle, false, 0)
				}
			}
		}

		if maxScroll > 0 {
			sbW := float32(8.0)
			sbX := fx + fw - sbW - 2
			thumbH := fh * float32(visibleLines) / float32(len(lines))
			if thumbH < 12 {
				thumbH = 12
			}
			thumbY := fy + (fh-thumbH)*float32(scrollOff)/float32(maxScroll)
			drawRoundedRectSDF(sbX, fy+2, sbW, fh-4, sbW*0.5, gfx.uiTheme.track, false, 0)
			drawRoundedRectSDF(sbX, thumbY, sbW, thumbH, sbW*0.5, gfx.uiTheme.widgetBgHover, false, 0)
		}

		uiRegisterElement(id, fx, fy, fw, fh)
		return &String{Value: newText}
	}}

	// getTypedChars() → string  — returns printable characters typed this frame
	Builtins["getTypedChars"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("getTypedChars expects no arguments", ast.Pos{})
		}
		out := ""
		for _, ch := range gfx.charBuf {
			if ch >= 32 && ch < 127 {
				out += string(ch)
			}
		}
		return &String{Value: out}
	}}

	// table(headers[], rows[][], x, y, w, h, ?size) → int — scrollable data grid; returns selected row index (-1 = none)
	Builtins["table"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 6 || len(args) > 7 {
			return typeError("table expects 6-7 arguments: headers, rows, x, y, w, h, [size]", ast.Pos{})
		}
		headers, ok1 := args[0].(*Array)
		rowsArr, ok2 := args[1].(*Array)
		x, ok3 := args[2].(*Integer)
		y, ok4 := args[3].(*Integer)
		w, ok5 := args[4].(*Integer)
		h, ok6 := args[5].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
			return typeError("table: headers/rows must be arrays, x/y/w/h must be integers", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 7 {
			if s, ok := args[6].(*Integer); ok {
				textScale = float32(s.Value) / 100.0
			} else if s, ok := args[6].(*Float); ok {
				textScale = float32(s.Value)
			}
		}

		id := fmt.Sprintf("tbl_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		numCols := len(headers.Elements)
		numRows := len(rowsArr.Elements)
		if numCols == 0 {
			return &Integer{Value: -1}
		}

		charH := uiCharH(textScale)
		headerH := charH + 10.0
		rowH := charH + 8.0
		colW := fw / float32(numCols)

		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}
		if gfx.uiListSelected == nil {
			gfx.uiListSelected = make(map[string]int)
		}
		selectedRow := gfx.uiListSelected[id]
		scrollOff := gfx.uiListScroll[id]

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

		if gfx.uiScrollDelta != 0 {
			mx, my := gfx.mouseX, gfx.mouseY
			if mx >= float64(fx) && mx <= float64(fx+fw) && my >= float64(fy) && my <= float64(fy+fh) {
				scrollOff -= int(gfx.uiScrollDelta * 3)
				if scrollOff < 0 {
					scrollOff = 0
				}
				if scrollOff > maxScroll {
					scrollOff = maxScroll
				}
				gfx.uiListScroll[id] = scrollOff
			}
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)
		drawRoundedRectSDF(fx, fy, fw, headerH, 4, gfx.uiTheme.widgetBgActive, false, 0)

		savedFillTbl := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.widgetText
		for c, hdrObj := range headers.Elements {
			hdr := ""
			if s, ok := hdrObj.(*String); ok {
				hdr = s.Value
			}
			drawText(hdr, int(fx+float32(c)*colW+6), int(fy+(headerH-charH)*0.5), false, textScale)
			if c > 0 {
				hdrDiv := gfx.uiTheme.accentBg
				hdrDiv[3] = 0.6
				drawRoundedRectSDF(fx+float32(c)*colW, fy, 1, headerH, 0, hdrDiv, false, 0)
			}
		}

		mx, my := gfx.mouseX, gfx.mouseY
		for i := 0; i < visibleRows; i++ {
			rowIdx := i + scrollOff
			if rowIdx >= numRows {
				break
			}
			ry := fy + headerH + float32(i)*rowH

			var bg [4]float32
			if rowIdx == selectedRow {
				bg = gfx.uiTheme.accentBg
			} else if i%2 == 0 {
				bg = gfx.uiTheme.inputBg
			} else {
				bg = gfx.uiTheme.track
			}
			drawRoundedRectSDF(fx, ry, fw, rowH, 0, bg, false, 0)

			if gfx.mouseJustClicked && mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(ry) && my <= float64(ry+rowH) {
				selectedRow = rowIdx
				gfx.uiListSelected[id] = rowIdx
			}

			if row, ok := rowsArr.Elements[rowIdx].(*Array); ok {
				if rowIdx == selectedRow {
					gfx.fillColor = gfx.uiTheme.widgetText
				} else {
					gfx.fillColor = gfx.uiTheme.labelText
				}
				for c := 0; c < numCols && c < len(row.Elements); c++ {
					cell := row.Elements[c].Inspect()
					if s, ok2 := row.Elements[c].(*String); ok2 {
						cell = s.Value
					}
					drawText(cell, int(fx+float32(c)*colW+6), int(ry+(rowH-charH)*0.5), false, textScale)
					if c > 0 {
						rowDiv := gfx.uiTheme.widgetBg
						rowDiv[3] = 0.4
						drawRoundedRectSDF(fx+float32(c)*colW, ry, 1, rowH, 0, rowDiv, false, 0)
					}
				}
			}
		}

		if maxScroll > 0 {
			sbW := float32(8.0)
			sbX := fx + fw - sbW - 2
			sbH := fh - headerH
			thumbH := sbH * float32(visibleRows) / float32(numRows)
			if thumbH < 12 {
				thumbH = 12
			}
			thumbY := fy + headerH + (sbH-thumbH)*float32(scrollOff)/float32(maxScroll)
			drawRoundedRectSDF(sbX, fy+headerH, sbW, sbH, sbW*0.5, gfx.uiTheme.track, false, 0)
			drawRoundedRectSDF(sbX, thumbY, sbW, thumbH, sbW*0.5, gfx.uiTheme.widgetBgHover, false, 0)
		}

		gfx.fillColor = savedFillTbl
		uiRegisterElement(id, fx, fy, fw, fh)
		return &Integer{Value: selectedRow}
	}}

	// accordion(x, y, w, sections[], openIdx, ?size) → int
	// Draws stacked clickable section headers. Returns open section index (-1 = all closed).
	// Caller renders content at y + (openIdx+1)*sectionH using the returned openIdx.
	Builtins["accordion"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("accordion expects 5-6 arguments: x, y, w, sections, openIdx, [size]", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		w, ok3 := args[2].(*Integer)
		sections, ok4 := args[3].(*Array)
		openIdxObj, ok5 := args[4].(*Integer)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("accordion: x/y/w/openIdx must be integers, sections must be array", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 6 {
			if s, ok := args[5].(*Integer); ok {
				textScale = float32(s.Value) / 100.0
			} else if s, ok := args[5].(*Float); ok {
				textScale = float32(s.Value)
			}
		}

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		openIdx := openIdxObj.Value
		charH := uiCharH(textScale)
		hdrH := charH + 14.0
		mx, my := gfx.mouseX, gfx.mouseY
		savedFillAcc := gfx.fillColor

		for i, secObj := range sections.Elements {
			lbl := ""
			if s, ok := secObj.(*String); ok {
				lbl = s.Value
			}
			hy := fy + float32(i)*hdrH
			isOpen := openIdx == i

			var bg [4]float32
			if isOpen {
				bg = gfx.uiTheme.widgetBg
			} else if mx >= float64(fx) && mx <= float64(fx+fw) && my >= float64(hy) && my <= float64(hy+hdrH) {
				bg = gfx.uiTheme.widgetBgActive
			} else {
				bg = gfx.uiTheme.track
			}
			drawRoundedRectSDF(fx, hy, fw, hdrH, 4, bg, false, 0)

			if isOpen {
				drawRoundedRectSDF(fx, hy, 3, hdrH, 0, gfx.uiTheme.accent, false, 0)
				gfx.fillColor = gfx.uiTheme.widgetText
			} else {
				gfx.fillColor = gfx.uiTheme.labelText
			}

			sym := ">"
			if isOpen {
				sym = "v"
			}
			drawText(sym, int(fx+8), int(hy+(hdrH-charH)*0.5), false, textScale)
			drawText(lbl, int(fx+26), int(hy+(hdrH-charH)*0.5), false, textScale)

			if gfx.mouseJustClicked && mx >= float64(fx) && mx <= float64(fx+fw) &&
				my >= float64(hy) && my <= float64(hy+hdrH) {
				if isOpen {
					openIdx = -1
				} else {
					openIdx = i
				}
			}
		}
		gfx.fillColor = savedFillAcc
		return &Integer{Value: openIdx}
	}}

	// contextMenu(x, y, items[], visible, ?size) → int
	// Returns selected item index, -1 if nothing clicked, -2 if clicked outside (dismiss).
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
		// Detect whether this is the first frame the menu is visible so we can
		// skip the outside-dismiss check (the opening click would fire it immediately).
		isFirstFrame := gfx.uiMenuOpenFrame < gfx.frameCount-1
		gfx.uiMenuOpenFrame = gfx.frameCount
		var textScale float32 = 0.5
		if len(args) == 5 {
			if s, ok := args[4].(*Integer); ok {
				textScale = float32(s.Value) / 100.0
			} else if s, ok := args[4].(*Float); ok {
				textScale = float32(s.Value)
			}
		}

		numItems := len(items.Elements)
		if numItems == 0 {
			return &Integer{Value: -1}
		}

		charH := uiCharH(textScale)
		itemH := charH + 10.0

		menuW := float32(120.0)
		for _, item := range items.Elements {
			if s, ok := item.(*String); ok {
				w := float32(len(s.Value))*float32(gfx.fontCellW)*textScale + 24.0
				if w > menuW {
					menuW = w
				}
			}
		}

		fx := float32(x.Value)
		fy := float32(y.Value)
		menuH := itemH*float32(numItems) + 4.0

		drawRoundedRectSDF(fx+3, fy+3, menuW, menuH, 4, gfx.uiTheme.shadow, false, 0)
		menuBg := gfx.uiTheme.widgetBgActive
		menuBg[3] = 0.97
		drawRoundedRectSDF(fx, fy, menuW, menuH, 4, menuBg, false, 0)
		drawRoundedRectSDF(fx, fy, menuW, menuH, 4, gfx.uiTheme.accent, true, 1.5)

		mx, my := gfx.mouseX, gfx.mouseY
		result := -1
		savedFillCtx := gfx.fillColor

		for i, item := range items.Elements {
			lbl := ""
			if s, ok := item.(*String); ok {
				lbl = s.Value
			}
			iy := fy + 2 + float32(i)*itemH
			isOver := mx >= float64(fx) && mx <= float64(fx+menuW) &&
				my >= float64(iy) && my <= float64(iy+itemH)

			if isOver {
				drawRoundedRectSDF(fx+2, iy, menuW-4, itemH, 3, gfx.uiTheme.widgetBg, false, 0)
				if gfx.mouseJustClicked {
					result = i
				}
				gfx.fillColor = gfx.uiTheme.widgetText
			} else {
				gfx.fillColor = gfx.uiTheme.labelText
			}
			drawText(lbl, int(fx+10), int(iy+(itemH-charH)*0.5), false, textScale)
		}
		gfx.fillColor = savedFillCtx

		if gfx.mouseJustClicked && result == -1 && !isFirstFrame {
			outside := mx < float64(fx) || mx > float64(fx+menuW) ||
				my < float64(fy) || my > float64(fy+menuH)
			if outside {
				result = -2
			}
		}

		uiRegisterElement(fmt.Sprintf("ctx_%d", gfx.uiNextID), fx, fy, menuW, menuH)
		return &Integer{Value: result}
	}}

	// colorPicker(x, y, w, r, g, b, a) → [r, g, b, a]
	// Four RGBA sliders with live preview swatch. Values are 0.0–1.0.
	Builtins["colorPicker"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 7 {
			return typeError("colorPicker expects 7 arguments: x, y, w, r, g, b, a", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		w, ok3 := args[2].(*Integer)
		getF := func(o Object) (float64, bool) {
			if v, ok := o.(*Float); ok {
				return v.Value, true
			}
			if v, ok := o.(*Integer); ok {
				return float64(v.Value), true
			}
			return 0, false
		}
		r, ok4 := getF(args[3])
		g, ok5 := getF(args[4])
		b, ok6 := getF(args[5])
		a, ok7 := getF(args[6])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
			return typeError("colorPicker: x/y/w must be integers, r/g/b/a must be numbers", ast.Pos{})
		}

		baseID := gfx.uiNextID
		gfx.uiNextID += 5

		fx, fy, fw := float32(x.Value), float32(y.Value), float32(w.Value)
		var textScale float32 = 0.5
		charH := uiCharH(textScale)
		trackH := float32(5.0)
		handleR := float32(7.0)
		gap := charH + 28.0

		cpSlider := func(idN int, lbl string, value float64, fillColor [4]float32, sy float32) float64 {
			trackY := sy + handleR - trackH*0.5
			if gfx.mouseDown {
				mx, my := gfx.mouseX, gfx.mouseY
				if mx >= float64(fx-handleR) && mx <= float64(fx+fw+handleR) &&
					my >= float64(sy) && my <= float64(sy+handleR*2) {
					t := (float32(mx) - fx) / fw
					if t < 0 {
						t = 0
					}
					if t > 1 {
						t = 1
					}
					value = float64(t)
				}
			}
			t := float32(value)
			savedFillCP := gfx.fillColor
			gfx.fillColor = gfx.uiTheme.labelText
			drawText(lbl+": "+fmt.Sprintf("%.2f", value), int(fx), int(sy-charH-2), false, textScale)
			gfx.fillColor = savedFillCP
			drawRoundedRectSDF(fx, trackY, fw, trackH, trackH*0.5, gfx.uiTheme.track, false, 0)
			if t > 0.01 {
				drawRoundedRectSDF(fx, trackY, fw*t, trackH, trackH*0.5, fillColor, false, 0)
			}
			hx := fx + fw*t
			drawRoundedRectSDF(hx-handleR, sy, handleR*2, handleR*2, handleR, gfx.uiTheme.handle, false, 0)
			uiRegisterElement(fmt.Sprintf("cp_%d_%d", baseID, idN), fx, sy, fw, handleR*2)
			return value
		}

		r = cpSlider(0, "R", r, [4]float32{float32(r), 0.1, 0.1, 1}, fy)
		g = cpSlider(1, "G", g, [4]float32{0.1, float32(g), 0.1, 1}, fy+gap)
		b = cpSlider(2, "B", b, [4]float32{0.1, 0.1, float32(b), 1}, fy+gap*2)
		a = cpSlider(3, "A", a, [4]float32{0.55, 0.55, 0.55, float32(a)}, fy+gap*3)

		swatchY := fy + gap*4
		swatchH := charH + 12.0
		drawRoundedRectSDF(fx, swatchY, fw, swatchH, 4, [4]float32{float32(r), float32(g), float32(b), float32(a)}, false, 0)
		drawText("preview", int(fx+6), int(swatchY+(swatchH-charH)*0.5), false, textScale)

		return &Array{Elements: []Object{
			&Float{Value: r},
			&Float{Value: g},
			&Float{Value: b},
			&Float{Value: a},
		}}
	}}

	// modal(title, message, buttons[]) → string
	// Full-screen dimmed overlay with a centred dialog. Returns the clicked button label
	// or "" each frame until a button is clicked. Call AFTER all other widgets (before
	// uiEnd()) so it renders on top. Registers a full-screen hit element to block
	// background widget hover on subsequent frames.
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
		var charH float32
		if gfx.uiActiveFont != nil {
			charH = gfx.uiActiveFont.LineH * textScale
		} else {
			charH = float32(gfx.fontCellH) * textScale
		}
		charW := float32(gfx.fontCellW) * textScale

		ww := float32(gfx.winW)
		wh := float32(gfx.winH)

		// Dialog geometry
		const dialogW float32 = 400
		const titleH float32 = 40
		const btnAreaH float32 = 52
		const pad float32 = 18

		// Word-wrap message into lines
		maxChars := int((dialogW - pad*2) / charW)
		if maxChars < 10 {
			maxChars = 10
		}
		msgLines := uiWrapText(message.Value, maxChars)
		lineH := charH + 5
		msgH := float32(len(msgLines))*lineH + pad*2

		dialogH := titleH + msgH + btnAreaH
		dx := (ww - dialogW) * 0.5
		dy := (wh - dialogH) * 0.5

		// 1. Full-screen dim overlay
		drawRoundedRectSDF(0, 0, ww, wh, 0, [4]float32{0, 0, 0, 0.60}, false, 0)

		// 2. Drop shadow
		drawRoundedRectSDF(dx+4, dy+4, dialogW, dialogH, 8, gfx.uiTheme.shadow, false, 0)

		// 3. Dialog body
		drawRoundedRectSDF(dx, dy, dialogW, dialogH, 8, gfx.uiTheme.inputBg, false, 0)

		// 4. Title bar — rounded top, square bottom
		titleBg := gfx.uiTheme.widgetBg
		drawRoundedRectSDF(dx, dy, dialogW, titleH, 8, titleBg, false, 0)
		drawRoundedRectSDF(dx, dy+8, dialogW, titleH-8, 0, titleBg, false, 0)
		// Accent stripe on left edge
		drawRoundedRectSDF(dx, dy, 4, titleH, 4, gfx.uiTheme.accent, false, 0)

		savedFill := gfx.fillColor
		gfx.fillColor = gfx.uiTheme.widgetText
		drawText(title.Value, int(dx+16), int(dy+(titleH-charH)*0.5), false, textScale)

		// 5. Message lines
		gfx.fillColor = gfx.uiTheme.labelText
		for i, line := range msgLines {
			ly := dy + titleH + pad + float32(i)*lineH
			drawText(line, int(dx+pad), int(ly), false, textScale)
		}

		// 6. Buttons — right-aligned row
		mx, my := gfx.mouseX, gfx.mouseY
		const btnH float32 = 30
		const btnGap float32 = 8
		btnY := dy + titleH + msgH + (btnAreaH-btnH)*0.5

		btnWidths := make([]float32, numBtns)
		totalBtnW := float32(0)
		for i, b := range buttons.Elements {
			lbl := ""
			if s, ok := b.(*String); ok {
				lbl = s.Value
			}
			bw := float32(len(lbl))*charW + 28
			if bw < 72 {
				bw = 72
			}
			btnWidths[i] = bw
			totalBtnW += bw
			if i > 0 {
				totalBtnW += btnGap
			}
		}

		bx := dx + dialogW - pad - totalBtnW
		result := ""
		for i, b := range buttons.Elements {
			lbl := ""
			if s, ok := b.(*String); ok {
				lbl = s.Value
			}
			bw := btnWidths[i]

			isOver := mx >= float64(bx) && mx <= float64(bx+bw) &&
				my >= float64(btnY) && my <= float64(btnY+btnH)

			var btnBg [4]float32
			if isOver {
				btnBg = gfx.uiTheme.accent
			} else {
				btnBg = gfx.uiTheme.widgetBg
			}
			drawRoundedRectSDF(bx, btnY, bw, btnH, 4, btnBg, false, 0)

			if isOver {
				gfx.fillColor = gfx.uiTheme.widgetText
			} else {
				gfx.fillColor = gfx.uiTheme.labelText
			}
			drawText(lbl, int(bx+bw*0.5), int(btnY+(btnH-charH)*0.5), true, textScale)

			if isOver && gfx.mouseJustClicked {
				result = lbl
			}
			bx += bw + btnGap
		}

		gfx.fillColor = savedFill

		// Consume the click so deferred overlays (dropdowns in uiEnd) don't also react.
		if gfx.mouseJustClicked {
			gfx.mouseJustClicked = false
		}

		// Full-screen hit element blocks background widget hover on the next frame.
		uiRegisterElement("__modal__", 0, 0, ww, wh)

		return &String{Value: result}
	}}

	// treeView(x, y, w, h, labels[], levels[], expanded[], ?size) → [selectedIdx, expanded[]]
	// labels[]: display text; levels[]: indent depth (0=root); expanded[]: bool per node.
	// Returns [selectedIdx, newExpanded[]]. Reassign: result = treeView(...); sel = result[0]; exp = result[1]
	Builtins["treeView"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("treeView expects 7-8 arguments: x, y, w, h, labels, levels, expanded, [size]", ast.Pos{})
		}
		x, ok1 := args[0].(*Integer)
		y, ok2 := args[1].(*Integer)
		w, ok3 := args[2].(*Integer)
		h, ok4 := args[3].(*Integer)
		labels, ok5 := args[4].(*Array)
		levels, ok6 := args[5].(*Array)
		expandedArr, ok7 := args[6].(*Array)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
			return typeError("treeView: x/y/w/h must be integers, labels/levels/expanded must be arrays", ast.Pos{})
		}
		var textScale float32 = 0.5
		if len(args) == 8 {
			if s, ok := args[7].(*Integer); ok {
				textScale = float32(s.Value) / 100.0
			} else if s, ok := args[7].(*Float); ok {
				textScale = float32(s.Value)
			}
		}

		id := fmt.Sprintf("tv_%d", gfx.uiNextID)
		gfx.uiNextID++

		fx, fy, fw, fh := float32(x.Value), float32(y.Value), float32(w.Value), float32(h.Value)
		n := len(labels.Elements)
		charH := uiCharH(textScale)
		rowH := charH + 8.0

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
			if i < len(newExpanded) {
				if b, ok := newExpanded[i].(*Boolean); ok {
					return b.Value
				}
			}
			return false
		}

		visible := []int{}
		hiddenLevel := -1
		for i := 0; i < n; i++ {
			lv := getLevel(i)
			if hiddenLevel >= 0 && lv > hiddenLevel {
				continue
			}
			if hiddenLevel >= 0 && lv <= hiddenLevel {
				hiddenLevel = -1
			}
			visible = append(visible, i)
			hasChildren := i+1 < n && getLevel(i+1) > lv
			if hasChildren && !nodeExpanded(i) {
				hiddenLevel = lv
			}
		}

		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}
		if gfx.uiListSelected == nil {
			gfx.uiListSelected = make(map[string]int)
		}
		scrollOff := gfx.uiListScroll[id]
		selectedIdx := gfx.uiListSelected[id]

		visibleCount := len(visible)
		visibleRows := int(fh / rowH)
		maxScroll := visibleCount - visibleRows
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollOff > maxScroll {
			scrollOff = maxScroll
		}

		if gfx.uiScrollDelta != 0 {
			mx, my := gfx.mouseX, gfx.mouseY
			if mx >= float64(fx) && mx <= float64(fx+fw) && my >= float64(fy) && my <= float64(fy+fh) {
				scrollOff -= int(gfx.uiScrollDelta * 3)
				if scrollOff < 0 {
					scrollOff = 0
				}
				if scrollOff > maxScroll {
					scrollOff = maxScroll
				}
				gfx.uiListScroll[id] = scrollOff
			}
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		mx, my := gfx.mouseX, gfx.mouseY
		savedFillTV := gfx.fillColor

		for vi := 0; vi < visibleRows && (vi+scrollOff) < visibleCount; vi++ {
			nodeIdx := visible[vi+scrollOff]
			lv := getLevel(nodeIdx)
			indent := float32(lv) * 16.0
			hasChildren := nodeIdx+1 < n && getLevel(nodeIdx+1) > lv
			ry := fy + float32(vi)*rowH
			isSelected := nodeIdx == selectedIdx
			isHovered := mx >= float64(fx) && mx <= float64(fx+fw) && my >= float64(ry) && my <= float64(ry+rowH)

			if isSelected {
				drawRoundedRectSDF(fx, ry, fw, rowH, 0, gfx.uiTheme.accentBg, false, 0)
			} else if isHovered {
				drawRoundedRectSDF(fx, ry, fw, rowH, 0, gfx.uiTheme.track, false, 0)
			}

			toggleX := fx + 4 + indent
			textX := toggleX + 18.0

			if hasChildren {
				sym := "+"
				if nodeExpanded(nodeIdx) {
					sym = "-"
				}
				drawRoundedRectSDF(toggleX, ry+(rowH-12)*0.5, 12, 12, 2, gfx.uiTheme.widgetBg, false, 0)
				gfx.fillColor = gfx.uiTheme.labelText
				drawText(sym, int(toggleX+3), int(ry+(rowH-charH)*0.5), false, textScale)

				ty := ry + (rowH-12)*0.5
				if gfx.mouseJustClicked && mx >= float64(toggleX) && mx <= float64(toggleX+12) &&
					my >= float64(ty) && my <= float64(ty+12) {
					if nodeExpanded(nodeIdx) {
						newExpanded[nodeIdx] = FALSE
					} else {
						newExpanded[nodeIdx] = TRUE
					}
				}
			}

			lbl := ""
			if nodeIdx < len(labels.Elements) {
				if s, ok := labels.Elements[nodeIdx].(*String); ok {
					lbl = s.Value
				}
			}
			if isSelected {
				gfx.fillColor = gfx.uiTheme.widgetText
			} else if lv == 0 {
				gfx.fillColor = gfx.uiTheme.labelText
			} else {
				gfx.fillColor = gfx.uiTheme.dimText
			}
			drawText(lbl, int(textX), int(ry+(rowH-charH)*0.5), false, textScale)

			if gfx.mouseJustClicked && mx >= float64(textX) && mx <= float64(fx+fw) &&
				my >= float64(ry) && my <= float64(ry+rowH) {
				selectedIdx = nodeIdx
				gfx.uiListSelected[id] = nodeIdx
			}
		}
		gfx.fillColor = savedFillTV

		if maxScroll > 0 {
			sbW := float32(8.0)
			sbX := fx + fw - sbW - 2
			thumbH := fh * float32(visibleRows) / float32(visibleCount)
			if thumbH < 12 {
				thumbH = 12
			}
			thumbY := fy + (fh-thumbH)*float32(scrollOff)/float32(maxScroll)
			drawRoundedRectSDF(sbX, fy, sbW, fh, sbW*0.5, gfx.uiTheme.track, false, 0)
			drawRoundedRectSDF(sbX, thumbY, sbW, thumbH, sbW*0.5, gfx.uiTheme.widgetBgHover, false, 0)
		}

		uiRegisterElement(id, fx, fy, fw, fh)
		return &Array{Elements: []Object{
			&Integer{Value: selectedIdx},
			&Array{Elements: newExpanded},
		}}
	}}

	// makeTheme() → array — returns the default 14-slot palette array ready to modify.
	// Slot order: 0=widgetBg, 1=widgetBgHover, 2=widgetBgActive, 3=widgetText,
	// 4=labelText, 5=dimText, 6=accent, 7=accentBg, 8=track, 9=trackFill,
	// 10=handle, 11=inputBg, 12=inputFocusBg, 13=shadow. Each slot is a [r, g, b, a] array.
	Builtins["makeTheme"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("makeTheme expects no arguments", ast.Pos{})
		}
		p := defaultUIPalette()
		slots := [14][4]float32{
			p.widgetBg, p.widgetBgHover, p.widgetBgActive,
			p.widgetText, p.labelText, p.dimText,
			p.accent, p.accentBg,
			p.track, p.trackFill, p.handle,
			p.inputBg, p.inputFocusBg,
			p.shadow,
		}
		elems := make([]Object, 14)
		for i, c := range slots {
			elems[i] = &Array{Elements: []Object{
				&Float{Value: float64(c[0])},
				&Float{Value: float64(c[1])},
				&Float{Value: float64(c[2])},
				&Float{Value: float64(c[3])},
			}}
		}
		return &Array{Elements: elems}
	}}

	// uiTheme(palette) — install a 14-slot palette from makeTheme() as the global widget theme.
	Builtins["uiTheme"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiTheme expects 1 argument: palette array from makeTheme()", ast.Pos{})
		}
		palette, ok := args[0].(*Array)
		if !ok || len(palette.Elements) != 14 {
			return typeError("uiTheme: argument must be a 14-element array from makeTheme()", ast.Pos{})
		}
		readColor := func(idx int) ([4]float32, bool) {
			el, ok := palette.Elements[idx].(*Array)
			if !ok || len(el.Elements) != 4 {
				return [4]float32{}, false
			}
			var c [4]float32
			for i, v := range el.Elements {
				switch n := v.(type) {
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
		slots := [14]*[4]float32{
			&gfx.uiTheme.widgetBg, &gfx.uiTheme.widgetBgHover, &gfx.uiTheme.widgetBgActive,
			&gfx.uiTheme.widgetText, &gfx.uiTheme.labelText, &gfx.uiTheme.dimText,
			&gfx.uiTheme.accent, &gfx.uiTheme.accentBg,
			&gfx.uiTheme.track, &gfx.uiTheme.trackFill, &gfx.uiTheme.handle,
			&gfx.uiTheme.inputBg, &gfx.uiTheme.inputFocusBg,
			&gfx.uiTheme.shadow,
		}
		for i, slot := range slots {
			c, ok := readColor(i)
			if !ok {
				return typeError(fmt.Sprintf("uiTheme: slot %d must be a 4-element [r,g,b,a] array", i), ast.Pos{})
			}
			*slot = c
		}
		return NULL
	}}

	// scrollArea(x, y, w, h, contentH) → float — draws panel + scrollbar; returns scroll offset in pixels.
	// Use pushClip(x,y,w,h) before drawing content and popClip() after.
	Builtins["scrollArea"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("scrollArea expects 5 arguments: x, y, w, h, contentH", ast.Pos{})
		}
		getNum := func(o Object) (float64, bool) {
			if v, ok := o.(*Integer); ok {
				return float64(v.Value), true
			}
			if v, ok := o.(*Float); ok {
				return v.Value, true
			}
			return 0, false
		}
		xf, ok1 := getNum(args[0])
		yf, ok2 := getNum(args[1])
		wf, ok3 := getNum(args[2])
		hf, ok4 := getNum(args[3])
		cf, ok5 := getNum(args[4])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("scrollArea: all arguments must be numbers", ast.Pos{})
		}

		id := fmt.Sprintf("sa_%d", gfx.uiNextID)
		gfx.uiNextID++

		if gfx.uiListScroll == nil {
			gfx.uiListScroll = make(map[string]int)
		}

		fx, fy, fw, fh := float32(xf), float32(yf), float32(wf), float32(hf)
		contentH := float32(cf)
		scrollOff := float32(gfx.uiListScroll[id])
		maxScroll := contentH - fh
		if maxScroll < 0 {
			maxScroll = 0
		}
		if scrollOff > maxScroll {
			scrollOff = maxScroll
		}

		if gfx.uiScrollDelta != 0 {
			mx, my := gfx.mouseX, gfx.mouseY
			if mx >= float64(fx) && mx <= float64(fx+fw) && my >= float64(fy) && my <= float64(fy+fh) {
				scrollOff -= float32(gfx.uiScrollDelta) * 20.0
				if scrollOff < 0 {
					scrollOff = 0
				}
				if scrollOff > maxScroll {
					scrollOff = maxScroll
				}
				gfx.uiListScroll[id] = int(scrollOff)
			}
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		if maxScroll > 0 {
			sbW := float32(8.0)
			sbX := fx + fw - sbW - 2
			thumbH := fh * fh / contentH
			if thumbH < 16 {
				thumbH = 16
			}
			thumbY := fy + (fh-thumbH)*scrollOff/maxScroll
			drawRoundedRectSDF(sbX, fy+2, sbW, fh-4, sbW*0.5, gfx.uiTheme.track, false, 0)
			drawRoundedRectSDF(sbX, thumbY, sbW, thumbH, sbW*0.5, gfx.uiTheme.widgetBgHover, false, 0)

			if gfx.mouseDown {
				mx, my := gfx.mouseX, gfx.mouseY
				if mx >= float64(sbX) && mx <= float64(sbX+sbW) && my >= float64(fy) && my <= float64(fy+fh) {
					t := (float32(my) - fy) / fh
					if t < 0 {
						t = 0
					}
					if t > 1 {
						t = 1
					}
					scrollOff = t * maxScroll
					gfx.uiListScroll[id] = int(scrollOff)
				}
			}
		}

		uiRegisterElement(id, fx, fy, fw, fh)
		return &Float{Value: float64(scrollOff)}
	}}

	// splitter(pos, x, y, length, orient, min, max, [thickness]) → int
	// Interactive resize handle between two panels.
	// orient: "v" = vertical bar (drags left/right), "h" = horizontal bar (drags up/down).
	// pos is the current bar position along the drag axis; x,y are the region origin;
	// length is the bar extent in the perpendicular direction.
	// Returns updated pos clamped to [min, max].
	Builtins["splitter"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 7 || len(args) > 8 {
			return typeError("splitter expects 7-8 arguments: pos, x, y, length, orient, min, max, [thickness]", ast.Pos{})
		}
		getInt := func(o Object) (int64, bool) {
			if v, ok := o.(*Integer); ok {
				return int64(v.Value), true
			}
			return 0, false
		}
		pos, ok1 := getInt(args[0])
		rx, ok2 := getInt(args[1])
		ry, ok3 := getInt(args[2])
		length, ok4 := getInt(args[3])
		orient, ok5 := args[4].(*String)
		minVal, ok6 := getInt(args[5])
		maxVal, ok7 := getInt(args[6])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
			return typeError("splitter: pos/x/y/length/min/max must be integers, orient must be string", ast.Pos{})
		}
		if orient.Value != "v" && orient.Value != "h" {
			return typeError(`splitter: orient must be "v" (vertical bar) or "h" (horizontal bar)`, ast.Pos{})
		}
		thickness := int64(6)
		if len(args) == 8 {
			if t, ok := getInt(args[7]); ok {
				thickness = t
			} else {
				return typeError("splitter: thickness must be an integer", ast.Pos{})
			}
		}

		isVertical := orient.Value == "v"
		prefix := "spl_v_"
		if !isVertical {
			prefix = "spl_h_"
		}
		id := fmt.Sprintf("%s%d", prefix, gfx.uiNextID)
		gfx.uiNextID++

		half := float32(thickness) * 0.5
		var hx, hy, hw, hh float32
		if isVertical {
			hx = float32(pos) - half
			hy = float32(ry)
			hw = float32(thickness)
			hh = float32(length)
		} else {
			hx = float32(rx)
			hy = float32(pos) - half
			hw = float32(length)
			hh = float32(thickness)
		}

		isDragging := gfx.uiActiveID == id && gfx.mouseDown
		isHit := gfx.mouseX >= float64(hx) && gfx.mouseX <= float64(hx+hw) &&
			gfx.mouseY >= float64(hy) && gfx.mouseY <= float64(hy+hh)

		if isHit && gfx.mouseJustClicked {
			gfx.uiActiveID = id
			isDragging = true
		}
		if !gfx.mouseDown && gfx.uiActiveID == id {
			gfx.uiActiveID = ""
		}

		if isDragging {
			if isVertical {
				pos = int64(gfx.mouseX)
			} else {
				pos = int64(gfx.mouseY)
			}
			if pos < minVal {
				pos = minVal
			}
			if pos > maxVal {
				pos = maxVal
			}
		}

		barColor := gfx.uiTheme.track
		if isDragging || isHit {
			barColor = gfx.uiTheme.accent
		}
		const lineW = float32(2.0)
		if isVertical {
			drawRoundedRectSDF(float32(pos)-lineW*0.5, float32(ry), lineW, float32(length), 1, barColor, false, 0)
		} else {
			drawRoundedRectSDF(float32(rx), float32(pos)-lineW*0.5, float32(length), lineW, 1, barColor, false, 0)
		}

		uiRegisterElement(id, hx, hy, hw, hh)
		return &Integer{Value: int(pos)}
	}}

	// ── Layout cursors ────────────────────────────────────────────────────────

	// uiBeginRow(x, y, h, gap) — initialise a horizontal layout cursor.
	// Subsequent calls to uiRowX()/uiRowY()/uiRowH() return the current slot
	// position. Call uiRowAdvance(w) after each widget to move right.
	Builtins["uiBeginRow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return typeError("uiBeginRow expects 4 arguments: x, y, h, gap", ast.Pos{})
		}
		getNum := func(o Object) (float32, bool) {
			if v, ok := o.(*Integer); ok {
				return float32(v.Value), true
			}
			if v, ok := o.(*Float); ok {
				return float32(v.Value), true
			}
			return 0, false
		}
		x, ok1 := getNum(args[0])
		y, ok2 := getNum(args[1])
		h, ok3 := getNum(args[2])
		gap, ok4 := getNum(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("uiBeginRow: all arguments must be numbers", ast.Pos{})
		}
		gfx.uiRowCurX = x
		gfx.uiRowY = y
		gfx.uiRowH = h
		gfx.uiRowGap = gap
		return NULL
	}}

	// uiRowX() → int — current X position of the row cursor.
	Builtins["uiRowX"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiRowX expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiRowCurX)}
	}}

	// uiRowY() → int — Y position of the row (constant for the row's lifetime).
	Builtins["uiRowY"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiRowY expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiRowY)}
	}}

	// uiRowH() → int — height of the row (constant for the row's lifetime).
	Builtins["uiRowH"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiRowH expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiRowH)}
	}}

	// uiRowAdvance(w) — advance the row cursor right by w + gap.
	Builtins["uiRowAdvance"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiRowAdvance expects 1 argument: w", ast.Pos{})
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
		gfx.uiRowCurX += w + gfx.uiRowGap
		return NULL
	}}

	// uiBeginCol(x, y, w, gap) — initialise a vertical layout cursor.
	// Subsequent calls to uiColX()/uiColY()/uiColW() return the current slot
	// position. Call uiColAdvance(h) after each widget to move down.
	Builtins["uiBeginCol"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return typeError("uiBeginCol expects 4 arguments: x, y, w, gap", ast.Pos{})
		}
		getNum := func(o Object) (float32, bool) {
			if v, ok := o.(*Integer); ok {
				return float32(v.Value), true
			}
			if v, ok := o.(*Float); ok {
				return float32(v.Value), true
			}
			return 0, false
		}
		x, ok1 := getNum(args[0])
		y, ok2 := getNum(args[1])
		w, ok3 := getNum(args[2])
		gap, ok4 := getNum(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("uiBeginCol: all arguments must be numbers", ast.Pos{})
		}
		gfx.uiColX = x
		gfx.uiColCurY = y
		gfx.uiColW = w
		gfx.uiColGap = gap
		return NULL
	}}

	// uiColX() → int — X position of the column (constant for the column's lifetime).
	Builtins["uiColX"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiColX expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiColX)}
	}}

	// uiColY() → int — current Y position of the column cursor.
	Builtins["uiColY"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiColY expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiColCurY)}
	}}

	// uiColW() → int — width of the column (constant for the column's lifetime).
	Builtins["uiColW"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiColW expects no arguments", ast.Pos{})
		}
		return &Integer{Value: int(gfx.uiColW)}
	}}

	// uiColAdvance(h) — advance the column cursor down by h + gap.
	Builtins["uiColAdvance"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("uiColAdvance expects 1 argument: h", ast.Pos{})
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
		gfx.uiColCurY += h + gfx.uiColGap
		return NULL
	}}
}

// ── Text cursor helpers ───────────────────────────────────────────────────────

// uiMeasurePrefix returns the pixel width of the first n runes of text
// using whichever font is active (monospace or proportional) at the given scale.
func uiMeasurePrefix(text string, n int, scale float32) float32 {
	if n <= 0 || len(text) == 0 {
		return 0
	}
	if gfx.uiActiveFont != nil {
		var w float32
		count := 0
		for _, ch := range text { // range iterates runes
			if count >= n {
				break
			}
			if g, ok := gfx.uiActiveFont.glyphs[ch]; ok {
				w += g.advance * scale
			} else {
				w += gfx.uiActiveFont.fallback.advance * scale
			}
			count++
		}
		return w
	}
	runeLen := len([]rune(text))
	if n > runeLen {
		n = runeLen
	}
	return float32(n*gfx.fontCellW) * scale
}

// uiCharAtX returns the rune index closest to pixel offset px from the
// left edge of the text. Uses midpoint snapping so clicks land on the nearest
// character boundary.
func uiCharAtX(text string, px float32, scale float32) int {
	runeLen := len([]rune(text))
	if runeLen == 0 {
		return 0
	}
	for i := 1; i <= runeLen; i++ {
		mid := (uiMeasurePrefix(text, i-1, scale) + uiMeasurePrefix(text, i, scale)) * 0.5
		if px < mid {
			return i - 1
		}
	}
	return runeLen
}

// ── UI helpers ───────────────────────────────────────────────────────────────

func uiRegisterElement(id string, x, y, w, h float32) {
	if gfx.uiElements == nil {
		gfx.uiElements = make(map[string][4]float32)
	}
	gfx.uiElements[id] = [4]float32{x, y, w, h}
	gfx.uiLastElementID = id
}

func uiCheckHover(mx, my float64) string {
	fmx, fmy := float32(mx), float32(my)
	for id, bounds := range gfx.uiElements {
		x, y, w, h := bounds[0], bounds[1], bounds[2], bounds[3]
		if fmx >= x && fmx <= x+w && fmy >= y && fmy <= y+h {
			return id
		}
	}
	return ""
}

// uiCharH returns the line height for the active font at the given scale.
// Use this everywhere charH was computed from gfx.fontCellH so that widgets
// centre their text correctly whether the embedded or a TrueType font is active.
func uiCharH(scale float32) float32 {
	if gfx.uiActiveFont != nil {
		return gfx.uiActiveFont.LineH * scale
	}
	return float32(gfx.fontCellH) * scale
}

// uiTextWidth returns the rendered pixel width of text at scale using whichever
// font is currently active. This must match drawText / drawTextProp exactly so
// that width calculations and rendering stay in sync.
func uiTextWidth(text string, scale float32) float32 {
	if gfx.uiActiveFont != nil {
		var w float32
		for _, ch := range text {
			if g, ok := gfx.uiActiveFont.glyphs[ch]; ok {
				w += g.advance
			} else {
				w += gfx.uiActiveFont.fallback.advance
			}
		}
		return w * scale
	}
	return float32(len([]rune(text))*gfx.fontCellW) * scale
}

// drawText renders a string using the SDF font atlas.
// x, y is the top-left origin of the first character when centered=false.
// When centered=true, x, y is the centre point and text is centred both axes.
func drawText(text string, x, y int, centered bool, scale float32) {
	if gfx.uiActiveFont != nil {
		drawTextProp(text, x, y, centered, scale)
		return
	}
	if gfx.fontTex == 0 {
		return
	}

	fx, fy := float32(x), float32(y)
	charW := float32(gfx.fontCellW) * scale
	charH := float32(gfx.fontCellH) * scale

	runeCount := len([]rune(text))
	if centered {
		fx -= float32(runeCount*gfx.fontCellW) * scale / 2
		fy -= charH / 2
	}

	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])

	gl.UseProgram(gfx.texProg)
	gl.UniformMatrix4fv(gfx.texProjLoc, 1, false, &mvp[0])
	gl.Uniform4f(gfx.texTintLoc, gfx.fillColor[0], gfx.fillColor[1], gfx.fillColor[2], gfx.fillColor[3])
	gl.Uniform1i(gfx.texTextModeLoc, 1)
	gl.Uniform1i(gfx.texTexLoc, 0)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, gfx.fontTex)

	gl.BindVertexArray(gfx.texVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.texVBO)

	verts := make([]float32, 0, len([]rune(text))*24)
	pos := 0 // rune position for X — not byte offset
	for _, r := range text {
		drawR := r
		if r < 32 || r >= 128 {
			drawR = '?' // visible replacement for non-ASCII in monospace
		}

		idx := int(drawR) - 32
		tx := float32(idx) / 96.0
		tw := float32(1.0 / 96.0)

		cx := fx + float32(pos)*charW
		cy := fy

		verts = append(verts,
			cx, cy, tx, 0,
			cx+charW, cy, tx+tw, 0,
			cx+charW, cy+charH, tx+tw, 1,
			cx, cy, tx, 0,
			cx+charW, cy+charH, tx+tw, 1,
			cx, cy+charH, tx, 1,
		)
		pos++
	}
	if len(verts) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.DYNAMIC_DRAW)
		gl.DrawArrays(gl.TRIANGLES, 0, int32(len(verts)/4))
	}

	gl.BindVertexArray(0)
}

// drawTextProp renders text using gfx.uiActiveFont (proportional SDF atlas).
// Mirrors drawText's centering contract: when centered=true, (x,y) is the
// centre of the text block, matching how button/tab widgets position their labels.
func drawTextProp(text string, x, y int, centered bool, scale float32) {
	fnt := gfx.uiActiveFont

	// Deferred GPU upload on first widget use.
	if fnt.pixels != nil {
		var texID uint32
		gl.GenTextures(1, &texID)
		gl.BindTexture(gl.TEXTURE_2D, texID)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
			fnt.atlasW, fnt.atlasHpx, 0,
			gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(fnt.pixels))
		gl.BindTexture(gl.TEXTURE_2D, 0)
		fnt.TextureID = texID
		fnt.pixels = nil
	}
	if fnt.TextureID == 0 {
		return
	}

	fx, fy := float32(x), float32(y)
	lineH := fnt.LineH * scale

	if centered {
		var totalW float32
		for _, ch := range text {
			if g, ok := fnt.glyphs[ch]; ok {
				totalW += g.advance
			} else {
				totalW += fnt.fallback.advance
			}
		}
		fx -= totalW * scale / 2
		fy -= lineH / 2
	}

	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
	gl.UseProgram(gfx.texProg)
	gl.UniformMatrix4fv(gfx.texProjLoc, 1, false, &mvp[0])
	gl.Uniform4f(gfx.texTintLoc, gfx.fillColor[0], gfx.fillColor[1], gfx.fillColor[2], gfx.fillColor[3])
	gl.Uniform1i(gfx.texTextModeLoc, 1)
	gl.Uniform1i(gfx.texTexLoc, 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, fnt.TextureID)
	gl.BindVertexArray(gfx.texVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.texVBO)

	verts := make([]float32, 0, len([]rune(text))*24)
	penX := fx
	for _, ch := range text {
		g, ok := fnt.glyphs[ch]
		if !ok {
			g = fnt.fallback
		}
		qw := g.advance * scale
		verts = append(verts,
			penX, fy,          g.u0, 0,
			penX+qw, fy,       g.u1, 0,
			penX+qw, fy+lineH, g.u1, 1,
			penX, fy,          g.u0, 0,
			penX+qw, fy+lineH, g.u1, 1,
			penX, fy+lineH,    g.u0, 1,
		)
		penX += qw
	}
	if len(verts) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.DYNAMIC_DRAW)
		gl.DrawArrays(gl.TRIANGLES, 0, int32(len(verts)/4))
	}

	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
}
