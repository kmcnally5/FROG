//go:build !js

package eval

import (
	"fmt"
	"klex/ast"
	"strings"
	"time"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// uiDropdownOpen moved to uiCore.dropdownOpen.

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
	uiCore.theme = defaultUIPalette()

	// uiBegin — start a UI frame, resetting per-frame widget state.
	//
	// Call once at the top of each draw loop, before any widget. It resets the
	// widget ID counter and the hover/element registry so the immediate-mode
	// widgets work correctly. Always pair with uiEnd at the end of the frame.
	//
	// @sig     uiBegin() -> null
	// @returns null
	// @errors  TypeError if called with any arguments
	// @example no-run window(640, 480, "app", fn(f) { uiBegin()  /* widgets */  uiEnd() })
	// @since   0.1.0
	// @see     uiEnd, button, window
	Builtins["uiBegin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiBegin expects no arguments", ast.Pos{})
		}
		uiCore.nextID = 0
		uiCore.elements = make(map[string][4]float32)
		uiCore.elementOrder = uiCore.elementOrder[:0]
		uiCore.pendingTooltip.active = false
		uiCore.tooltipMatchedThisFrame = false
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		return NULL
	}}

	// uiNextFieldID() → string
	// Returns the ID the next textInput widget will receive.
	// Call this immediately before textInput() to capture its ID for Tab focus management.

	// uiGetFocus() → string
	// Returns the ID of the currently focused widget, or "" if none.

	// uiSetFocus(id) — sets focus to the widget with the given ID.
	// Use in combination with uiNextFieldID() to implement Tab key navigation.

	// uiSetFont(font) — set the active font for all widget text (button labels, tabs, etc.)
	// Call once per frame (or once before window()) after loading a font with loadFont().
	// Reverts to the embedded monospace font when uiResetFont() is called.

	// uiResetFont() — revert widget text back to the embedded monospace font.

	// uiEnd — finish a UI frame: update hover state and draw deferred popups.
	//
	// Call once at the end of each draw loop, after every widget. It computes which
	// widget the cursor is over (so next frame's hover is correct) and renders
	// overlays that must sit on top — open dropdown menus, tooltips, toasts. Always
	// pair with uiBegin.
	//
	// @sig     uiEnd() -> null
	// @returns null
	// @errors  TypeError if called with any arguments
	// @example no-run window(640, 480, "app", fn(f) { uiBegin()  /* widgets */  uiEnd() })
	// @since   0.1.0
	// @see     uiBegin, tooltip, toast, dropdown
	Builtins["uiEnd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiEnd expects no arguments", ast.Pos{})
		}
		// Render any deferred dropdown popup on top of all other widgets.
		if uiCore.pendingDropdown.active {
			p := uiCore.pendingDropdown
			uiCore.pendingDropdown.active = false
			const itemH = 24.0
			menuH := float32(len(p.items)) * itemH
			drawRoundedRectSDF(p.fx, p.fy, p.fw, menuH, 4, uiCore.theme.inputBg, false, 0)
			savedFill := gfx.fillColor
			for i, itemText := range p.items {
				itemY := p.fy + float32(i)*itemH
				isItemHovered := gfx.mouseX >= float64(p.fx) && gfx.mouseX <= float64(p.fx+p.fw) &&
					gfx.mouseY >= float64(itemY) && gfx.mouseY <= float64(itemY+itemH)
				itemBg := uiCore.theme.inputBg
				if i == p.selectedIdx {
					itemBg = uiCore.theme.accentBg
				} else if isItemHovered {
					itemBg = uiCore.theme.widgetBgActive
				}
				drawRoundedRectSDF(p.fx, itemY, p.fw, itemH, 2, itemBg, false, 0)
				if i == p.selectedIdx {
					gfx.fillColor = uiCore.theme.widgetText
				} else if isItemHovered {
					gfx.fillColor = uiCore.theme.labelText
				} else {
					gfx.fillColor = uiCore.theme.dimText
				}
				drawText(itemText, int(p.fx+8), int(itemY+(itemH-p.charH)*0.5), false, p.textScale)
				uiRegisterElement(fmt.Sprintf("%s_opt_%d", p.id, i), p.fx, itemY, p.fw, itemH)
			}
			gfx.fillColor = savedFill
		}
		// Render active toasts — bottom-right corner, stacked upward.
		now := time.Since(uiCore.startTime).Seconds()
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
		live := uiCore.toasts[:0]
		savedFillToast := gfx.fillColor
		for i, t := range uiCore.toasts {
			remaining := t.expiresAt - now
			if remaining <= 0 {
				continue
			}
			live = append(live, uiCore.toasts[i])
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
			if uiCore.activeFont != nil {
				charH = uiCore.activeFont.LineH * 0.5
			}
			drawText(t.message, int(tx+14), int(ty+(toastH-charH)*0.5), false, 0.5)
		}
		uiCore.toasts = live
		gfx.fillColor = savedFillToast

		// Render pending tooltip on top of everything.
		if uiCore.pendingTooltip.active {
			const (
				tipPadX  = float32(10.0)
				tipPadY  = float32(6.0)
				tipScale = float32(0.5)
				tipR     = float32(4.0)
			)
			tip := uiCore.pendingTooltip
			charH := uiCharH(tipScale)
			// Measure exact text width using glyph advances for proportional fonts.
			var textW float32
			if uiCore.activeFont != nil {
				for _, ch := range tip.text {
					if g, ok := uiCore.activeFont.glyphs[ch]; ok {
						textW += g.advance * tipScale
					} else {
						textW += uiCore.activeFont.fallback.advance * tipScale
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
			drawRoundedRectSDF(tx, ty, tipW, tipH, tipR, uiCore.theme.accent, true, 0.5)
			gfx.fillColor = [4]float32{1, 1, 1, 1}
			drawText(tip.text, int(tx+tipPadX), int(ty+tipPadY), false, tipScale)
			gfx.fillColor = savedFillTip
		}
		if !uiCore.tooltipMatchedThisFrame {
			// No tooltip() call matched this frame — mouse left all tooltip widgets.
			uiCore.tooltipHoveredID = ""
		}

		mx, my := gfx.mouseX, gfx.mouseY
		uiCore.hoveredID = uiCheckHover(mx, my)

		// Lazy-init standard cursors and set the right shape each frame.
		if gfx.cursorArrow == nil {
			gfx.cursorArrow = glfw.CreateStandardCursor(glfw.ArrowCursor)
			gfx.cursorIBeam = glfw.CreateStandardCursor(glfw.IBeamCursor)
			gfx.cursorHand = glfw.CreateStandardCursor(glfw.HandCursor)
			gfx.cursorResizeEW = glfw.CreateStandardCursor(glfw.HResizeCursor)
			gfx.cursorResizeNS = glfw.CreateStandardCursor(glfw.VResizeCursor)
		}
		hid := uiCore.hoveredID
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

	// tooltip(text) — show hover text for the widget drawn immediately before this call.
	// Appears after the cursor rests on the widget for 0.5 seconds.

	// image(img, x, y, w, h, [mode]) — draw an Image inside the UI.
	// mode: "fit" (default, letterbox+centre), "fill" (crop+clip), "stretch"

	// "fit"

	// button(label, x, y, w, h, ?size) → true if clicked this frame

	// Auto-expand button width so the label always fits with minimum padding.
	// This makes button sizing font-agnostic: callers don't need to know which
	// font is active or how wide each glyph is — the button just grows to fit.
	// minimum horizontal padding each side

	// label(text, x, y, ?size) → null

	// textInput moved to eval/builtins_ui_widgets.go (Phase 4 + 4.5).

	// list(label, items[], x, y, w, h, ?size) → selected item string (with scrollbar)

	// listMulti(label, items, selected, x, y, w, h, ?size) → selected[]
	// selected is a bool array (one entry per item). Click toggles. Returns updated array.

	// Build a working bool slice, extending/trimming to match items length.

	// Left accent tick for selected rows.

	// Return updated bool array.

	// checkbox(label, x, y, checked, ?size) → bool

	// slider(label, x, y, w, value, min, max, ?size) → float

	// progressBar(x, y, w, h, value, max) → null

	// dropdown(label, items[], x, y, w, ?size) → selected item
	// Call after other widgets so the open menu renders on top.

	// Compute popup position once — upward-flip when the menu would overflow
	// the window bottom. Shared by the outside-click guard and item handler.

	// Handle item clicks inline (needed for correct return value this frame),
	// then defer all drawing to uiEnd() so the popup renders on top.
	// popupMenuY / popupMenuH already account for the upward-flip.

	// toggle(label, x, y, on, [size]) → bool

	// Pill track

	// Handle — slides left (off) or right (on)

	// Label

	// radio(label, x, y, value, groupValue, [size]) → string
	// Returns value if clicked, otherwise returns groupValue unchanged.
	// Call once per option in the group; chain the returned groupValue through each call.

	// Outer ring

	// Inner dot when selected

	// Label

	// numericStepper(label, x, y, w, value, min, max, [size]) → int

	// Minus button

	// Value display

	// Plus button

	// Draw − and + symbols

	// Draw value centred in the middle panel

	// Label above

	// tabs(x, y, w, items[], activeIdx, [size]) → int

	// Shared strip background — what makes this a tab bar, not buttons

	// Full-width accent line at the bottom of the bar

	// Active tab: lifted fill that covers the bottom accent line

	// Top accent stripe — the clear "this tab is active" signal

	// Thin vertical divider between tabs (skip after last)

	// textArea moved to eval/builtins_ui_widgets.go (Phase 4 + 4.5).

	// getTypedChars, table moved to eval/builtins_ui_widgets.go (Phase 3.5 / Phase 2).

	// accordion, contextMenu moved to eval/builtins_ui_widgets.go (Phase 1b / Phase 3.5).

	// colorPicker(x, y, w, r, g, b, a) → [r, g, b, a]
	// Four RGBA sliders with live preview swatch. Values are 0.0–1.0.

	// modal moved to eval/builtins_ui_widgets.go (Phase 1b).

	// treeView moved to eval/builtins_ui_widgets.go (Phase 2).

	// makeTheme() → array — returns the default 14-slot palette array ready to modify.
	// Slot order: 0=widgetBg, 1=widgetBgHover, 2=widgetBgActive, 3=widgetText,
	// 4=labelText, 5=dimText, 6=accent, 7=accentBg, 8=track, 9=trackFill,
	// 10=handle, 11=inputBg, 12=inputFocusBg, 13=shadow. Each slot is a [r, g, b, a] array.
	// makeTheme, uiTheme moved to eval/builtins_ui_widgets.go (Phase 1a).

	// scrollArea, splitter moved to eval/builtins_ui_widgets.go (Phase 2 + Phase 1b).

	// ── Layout cursors ────────────────────────────────────────────────────────
	// uiBeginRow / uiRowX/Y/H/Advance and uiBeginCol / uiColX/Y/W/Advance
	// are now registered in eval/builtins_ui_widgets.go (shared between
	// desktop and WASM). The desktop-only versions previously here were
	// dead code — overridden by the shared init order.
}

// ── Text cursor helpers ───────────────────────────────────────────────────────

// uiMeasurePrefix returns the pixel width of the first n runes of text
// using whichever font is active (monospace or proportional) at the given scale.
func uiMeasurePrefix(text string, n int, scale float32) float32 {
	if n <= 0 || len(text) == 0 {
		return 0
	}
	if uiCore.activeFont != nil {
		var w float32
		count := 0
		for _, ch := range text { // range iterates runes
			if count >= n {
				break
			}
			if g, ok := uiCore.activeFont.glyphs[ch]; ok {
				w += g.advance * scale
			} else {
				w += uiCore.activeFont.fallback.advance * scale
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
// uiRegisterElement + uiCheckHover moved to eval/builtins_ui_widgets.go
// so the WASM build (which has no builtins_ui.go) can use them.

// uiCharH returns the line height for the active font at the given scale.
// Use this everywhere charH was computed from gfx.fontCellH so that widgets
// centre their text correctly whether the embedded or a TrueType font is active.
func uiCharH(scale float32) float32 {
	if uiCore.activeFont != nil {
		return uiCore.activeFont.LineH * scale
	}
	return float32(gfx.fontCellH) * scale
}

// uiTextWidth returns the rendered pixel width of text at scale using whichever
// font is currently active on the desktop GL renderer.
// NOTE: wrap functions (softWrapText, wrapOneLine, etc.) use
// activeRenderer.textWidth() instead so they work on both GL and Canvas2D.
func uiTextWidth(text string, scale float32) float32 {
	if uiCore.activeFont != nil {
		var w float32
		for _, ch := range text {
			if g, ok := uiCore.activeFont.glyphs[ch]; ok {
				w += g.advance
			} else {
				w += uiCore.activeFont.fallback.advance
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
	if uiCore.activeFont != nil {
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

// drawTextProp renders text using uiCore.activeFont (proportional SDF atlas).
// Mirrors drawText's centering contract: when centered=true, (x,y) is the
// centre of the text block, matching how button/tab widgets position their labels.
func drawTextProp(text string, x, y int, centered bool, scale float32) {
	fnt := uiCore.activeFont

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
			penX, fy, g.u0, 0,
			penX+qw, fy, g.u1, 0,
			penX+qw, fy+lineH, g.u1, 1,
			penX, fy, g.u0, 0,
			penX+qw, fy+lineH, g.u1, 1,
			penX, fy+lineH, g.u0, 1,
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
