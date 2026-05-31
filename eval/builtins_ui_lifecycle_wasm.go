//go:build js && wasm

package eval

// builtins_ui_lifecycle_wasm.go — uiBegin / uiEnd for the WASM build.
// Phase 3 adds: pending dropdown popup, tooltip hover-and-pause
// rendering, and toast notifications. OS cursor handling and key
// repeat are still desktop-only; Phase 4 will revisit when textInput
// lands.

import (
	"fmt"
	"klex/ast"
	"time"
)

// toastStyleColors returns the (bg, text) colours for a toast style.
func toastStyleColors(style string) ([4]float32, [4]float32) {
	switch style {
	case "success":
		return [4]float32{0.20, 0.55, 0.30, 0.95}, [4]float32{1, 1, 1, 1}
	case "warn":
		return [4]float32{0.65, 0.45, 0.10, 0.95}, [4]float32{1, 1, 1, 1}
	case "error":
		return [4]float32{0.70, 0.20, 0.20, 0.95}, [4]float32{1, 1, 1, 1}
	default: // info
		return [4]float32{0.15, 0.35, 0.65, 0.95}, [4]float32{1, 1, 1, 1}
	}
}

func init() {
	// uiBegin() — reset per-frame UI state. Must be the first widget
	// call in each draw frame. Mirrors the lifecycle the desktop
	// uiBegin establishes.
	Builtins["uiBegin"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiBegin expects no arguments", ast.Pos{})
		}
		uiCore.nextID = 0
		// Reuse the map across frames — clearing keeps the backing store
		// and avoids a per-frame allocation + GC churn.
		if uiCore.elements == nil {
			uiCore.elements = make(map[string][4]float32)
		} else {
			for k := range uiCore.elements {
				delete(uiCore.elements, k)
			}
		}
		uiCore.elementOrder = uiCore.elementOrder[:0]
		uiCore.lastElementID = ""
		uiCore.pendingTooltip.active = false
		uiCore.tooltipMatchedThisFrame = false
		// Tick per-frame timing for animations.
		now := time.Now()
		if !uiCore.prevFrameTime.IsZero() {
			uiCore.frameDt = now.Sub(uiCore.prevFrameTime).Seconds()
			// Clamp the dt — a tab switch or paused window can leave
			// a huge dt that would over-step animations.
			if uiCore.frameDt > 0.1 {
				uiCore.frameDt = 0.1
			}
		}
		uiCore.prevFrameTime = now
		if uiCore.listSelected == nil {
			uiCore.listSelected = make(map[string]int)
		}
		if uiCore.listScroll == nil {
			uiCore.listScroll = make(map[string]int)
		}
		return NULL
	}}

	// uiEnd() — close out the UI frame. For Phase 1a this is just the
	// hover check: walk the element registry and update uiCore.hoveredID
	// based on the live mouse position. Phase 3 adds dropdown / tooltip
	// deferred draws here; Phase 1b will add toast rendering.
	Builtins["uiEnd"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return typeError("uiEnd expects no arguments", ast.Pos{})
		}
		in := uiInput()

		// 1. Deferred dropdown popup — rendered last so it floats above
		//    every other widget. The dropdown body queued this if the
		//    menu is currently open.
		if uiCore.pendingDropdown.active {
			p := uiCore.pendingDropdown
			uiCore.pendingDropdown.active = false
			itemH := float32(26)
			menuH := float32(len(p.items)) * itemH
			// Shadow under the popup — sells the "floating above" depth.
			ddShadow := uiCore.theme.shadow
			ddShadow[3] *= 0.95
			activeRenderer.dropShadow(p.fx, p.fy, p.fw, menuH, uiCore.style.radiusMedium, 8, 24, ddShadow)
			activeRenderer.fillRoundedRect(p.fx, p.fy, p.fw, menuH, uiCore.style.radiusMedium, uiCore.theme.inputBg)
			activeRenderer.strokeRoundedRect(p.fx, p.fy, p.fw, menuH, uiCore.style.radiusMedium, 0.5, uiCore.theme.widgetBgActive)
			lhDD := activeRenderer.lineHeight(p.textScale)
			for i, itemText := range p.items {
				iy := p.fy + float32(i)*itemH
				isItemHovered := in.mouseX >= p.fx && in.mouseX <= p.fx+p.fw &&
					in.mouseY >= iy && in.mouseY <= iy+itemH
				itemBg := uiCore.theme.inputBg
				textCol := uiCore.theme.dimText
				switch {
				case i == p.selectedIdx:
					itemBg = uiCore.theme.accentBg
					textCol = uiCore.theme.widgetText
				case isItemHovered:
					itemBg = uiCore.theme.widgetBgActive
					textCol = uiCore.theme.labelText
				}
				activeRenderer.fillRoundedRect(p.fx, iy, p.fw, itemH, 2, itemBg)
				activeRenderer.drawText(itemText, int(p.fx+8), int(iy+(itemH-lhDD)*0.5), false, p.textScale, textCol)
				uiRegisterElement(fmt.Sprintf("%s_opt_%d", p.id, i), p.fx, iy, p.fw, itemH)
			}
		}

		// 2. Active toasts — bottom-right, stacked upward, fade out
		//    over the last 0.5s of life.
		now := time.Since(uiCore.startTime).Seconds()
		keep := uiCore.toasts[:0]
		for _, t := range uiCore.toasts {
			if t.expiresAt > now {
				keep = append(keep, t)
			}
		}
		uiCore.toasts = keep
		const toastW = float32(280)
		const toastH = float32(40)
		const toastGap = float32(8)
		const toastPad = float32(16)
		lhToast := activeRenderer.lineHeight(0.5)
		for i, t := range uiCore.toasts {
			alpha := float32(1)
			remaining := t.expiresAt - now
			if remaining < 0.5 {
				alpha = float32(remaining * 2)
				if alpha < 0 {
					alpha = 0
				}
			}
			bg, textCol := toastStyleColors(t.style)
			bg[3] *= alpha
			textCol[3] *= alpha
			tx := float32(in.winW) - toastW - toastPad
			ty := float32(in.winH) - toastPad - float32(i+1)*(toastH+toastGap)
			activeRenderer.fillRoundedRect(tx, ty, toastW, toastH, uiCore.style.radiusMedium, bg)
			activeRenderer.drawText(t.message, int(tx+12), int(ty+(toastH-lhToast)*0.5), false, 0.5, textCol)
		}

		// 3. Pending tooltip — render after toasts so it can sit on top.
		if uiCore.pendingTooltip.active {
			p := uiCore.pendingTooltip
			uiCore.pendingTooltip.active = false
			textScale := float32(0.45)
			padX := float32(10)
			tw := activeRenderer.textWidth(p.text, textScale) + padX*2
			th := float32(26)
			tx := p.mx + 12
			ty := p.my + 12
			if tx+tw > float32(in.winW) {
				tx = float32(in.winW) - tw - 4
			}
			if ty+th > float32(in.winH) {
				ty = p.my - th - 8
			}
			tipR := uiCore.style.radiusMedium
			tipShadow := uiCore.theme.shadow
			tipShadow[3] *= 0.85
			activeRenderer.dropShadow(tx, ty, tw, th, tipR, 5, 14, tipShadow)
			activeRenderer.fillRoundedRect(tx, ty, tw, th, tipR, [4]float32{0.1, 0.1, 0.12, 0.96})
			lhTip := activeRenderer.lineHeight(textScale)
			activeRenderer.drawText(p.text, int(tx+padX), int(ty+(th-lhTip)*0.5), false, textScale, [4]float32{1, 1, 1, 1})
		}

		// 4. Reset tooltip hover timer if nothing matched this frame —
		//    cursor moved away from any widget that had attached tooltip.
		if !uiCore.tooltipMatchedThisFrame {
			uiCore.tooltipHoveredID = ""
			uiCore.tooltipHoverStart = 0
		}

		// 5. Hover hit-test for next frame.
		uiCore.hoveredID = uiCheckHover(float64(in.mouseX), float64(in.mouseY))
		return NULL
	}}
}
