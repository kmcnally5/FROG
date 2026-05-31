//go:build js && wasm

package eval

import (
	"fmt"
	"math"
	"syscall/js"
)

// builtins_ui_renderer_wasm.go — Canvas2D renderer adapter for the
// shared uiRenderer interface. Translates the abstract draw calls into
// the same Canvas2D primitives used by eval/builtins_graphics_wasm.go
// (rect path with arcTo corners for rounded rects, fillText/measureText
// for text).
//
// canvas2DRenderer is stateless; it consults gfxState.ctx for the live
// 2d context. All colour and font state is set per-call (saved and
// restored around fillText so widget colours don't leak into other
// graphics builtins).

type canvas2DRenderer struct{}

// cssColor formats an RGBA tuple as a Canvas2D-acceptable string.
// Uses rgba() to preserve alpha; widget code does pass <1.0 alphas via
// the theme palette (e.g. selection highlights).
func cssColor(c [4]float32) string {
	r := int(c[0]*255 + 0.5)
	g := int(c[1]*255 + 0.5)
	b := int(c[2]*255 + 0.5)
	if c[3] >= 1.0 {
		return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", r, g, b, c[3])
}

// roundRectPath constructs a rounded-rect path on ctx. Used by both
// fill and stroke variants. arcTo corners match Canvas2D's native
// roundRect (which not every browser still ships with — arcTo is the
// safe lowest-common-denominator).
func roundRectPath(ctx js.Value, x, y, w, h, r float32) {
	// Clamp radius so it can't exceed the rectangle's half-size.
	maxR := w
	if h < maxR {
		maxR = h
	}
	maxR = maxR / 2
	if r > maxR {
		r = maxR
	}
	if r < 0 {
		r = 0
	}
	ctx.Call("beginPath")
	ctx.Call("moveTo", x+r, y)
	ctx.Call("lineTo", x+w-r, y)
	ctx.Call("arcTo", x+w, y, x+w, y+r, r)
	ctx.Call("lineTo", x+w, y+h-r)
	ctx.Call("arcTo", x+w, y+h, x+w-r, y+h, r)
	ctx.Call("lineTo", x+r, y+h)
	ctx.Call("arcTo", x, y+h, x, y+h-r, r)
	ctx.Call("lineTo", x, y+r)
	ctx.Call("arcTo", x, y, x+r, y, r)
	ctx.Call("closePath")
}

func (canvas2DRenderer) fillRoundedRect(x, y, w, h, r float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	roundRectPath(ctx, x, y, w, h, r)
	ctx.Set("fillStyle", cssColor(color))
	ctx.Call("fill")
}

func (canvas2DRenderer) strokeRoundedRect(x, y, w, h, r, strokeHalfW float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	roundRectPath(ctx, x, y, w, h, r)
	ctx.Set("strokeStyle", cssColor(color))
	ctx.Set("lineWidth", strokeHalfW*2)
	ctx.Call("stroke")
}

// uiFontBasePx is the fallback default font size when no theme style
// is loaded. Real widgets read uiCore.style.fontBasePx instead — see
// effectiveFontBasePx() below.
const uiFontBasePx = 16.0

// uiSystemFontStack is the default CSS font shorthand suffix when no
// uiCore.activeFont is loaded. Modern system stack — picks the host
// OS's native UI font (San Francisco on mac, Segoe UI on Windows,
// Roboto on Android/ChromeOS). Falls back to sans-serif as a last resort.
const uiSystemFontStack = `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`

// effectiveFontBasePx returns the live base font size — uiCore.style
// when set (non-zero), else the const uiFontBasePx fallback so calls
// before theme init still produce sensible sizes.
func effectiveFontBasePx() float32 {
	if uiCore.style.fontBasePx > 0 {
		return uiCore.style.fontBasePx
	}
	return uiFontBasePx
}

func (canvas2DRenderer) drawText(text string, x, y int, centered bool, scale float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	if text == "" {
		return
	}

	cssFont, _ := activeFontCSS(scale)
	if cssFont == "" {
		size := float64(scale) * float64(effectiveFontBasePx())
		if size < 1 {
			size = 1
		}
		cssFont = fmt.Sprintf("%dpx %s", int(size+0.5), uiSystemFontStack)
	}
	ctx.Set("font", cssFont)
	ctx.Set("fillStyle", cssColor(color))

	// Match desktop drawText contract: when centered=true, (x,y) is
	// the centre of the text box; otherwise (x,y) is the top-left.
	// Canvas2D's textBaseline=middle + textAlign=center delivers
	// centred; left+top for the corner case.
	if centered {
		ctx.Set("textAlign", "center")
		ctx.Set("textBaseline", "middle")
	} else {
		ctx.Set("textAlign", "left")
		ctx.Set("textBaseline", "top")
	}
	ctx.Call("fillText", text, x, y)
}

// activeFontCSS returns ("<px>px <family>", lineHeightPx) for the
// currently active proportional font, or ("", 0) when no font is set
// (callers fall back to the default sans-serif at uiFontBasePx).
func activeFontCSS(scale float32) (string, float32) {
	f := uiCore.activeFont
	if f == nil {
		return "", 0
	}
	h, ok := lookupFont(f)
	if !ok {
		return "", 0
	}
	size := h.ptSize * scale
	if size < 1 {
		size = 1
	}
	return fmt.Sprintf("%dpx %s", int(size+0.5), h.family), size
}

func (canvas2DRenderer) lineHeight(scale float32) float32 {
	if _, lh := activeFontCSS(scale); lh > 0 {
		return lh
	}
	return effectiveFontBasePx() * scale
}

// textTopInsetCache memoises the computed inset per CSS font string so
// measureText runs at most once per unique (font-family, font-size). A map
// (rather than a single last-value slot) keeps widgets that alternate
// between two scales — e.g. a label and a heading — from thrashing it.
var textTopInsetCache = map[string]float32{}

// textTopInset returns the gap between the em-square top (the Y
// coordinate passed to fillText with textBaseline="top") and the top of
// the actual rendered cap-height glyph pixels.
//
// It is computed live from Canvas2D font metrics so it's correct for any
// system font and scale:
//  1. m1.alphabeticBaseline (measured from textBaseline="top") gives the
//     distance from the em-square top down to the alphabetic baseline.
//  2. m2.actualBoundingBoxAscent (from textBaseline="alphabetic") gives
//     the cap height above the alphabetic baseline.
//  3. Their difference is the gap from the em-square top to cap-height
//     top — exactly how far down the cursor must shift to sit alongside
//     the text rather than floating above it.
//
// The result is cached by CSS font string; measureText is called at most
// once per unique (font-family, font-size) combination.
func (canvas2DRenderer) textTopInset(scale float32) float32 {
	cssFont, _ := activeFontCSS(scale)
	if cssFont == "" {
		size := float64(scale) * float64(effectiveFontBasePx())
		if size < 1 {
			size = 1
		}
		cssFont = fmt.Sprintf("%dpx %s", int(size+0.5), uiSystemFontStack)
	}
	if v, ok := textTopInsetCache[cssFont]; ok {
		return v
	}

	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return effectiveFontBasePx() * scale * 0.2
	}

	ctx.Set("font", cssFont)

	// Step 1: distance from "top" baseline (em-square top) to alphabetic baseline.
	ctx.Set("textBaseline", "top")
	m1 := ctx.Call("measureText", "H")
	alphaFromTop := m1.Get("alphabeticBaseline")

	// Step 2: cap height above the alphabetic baseline.
	ctx.Set("textBaseline", "alphabetic")
	m2 := ctx.Call("measureText", "H")
	capFromAlpha := m2.Get("actualBoundingBoxAscent")

	var inset float32
	if !alphaFromTop.IsUndefined() && !capFromAlpha.IsUndefined() {
		// Use Abs: Chrome returns alphabeticBaseline as positive (going down from
		// textBaseline="top"), Safari returns it negative (spec-correct: positive = up).
		// Without Abs, Safari's negative value makes v deeply negative → inset = 0.
		v := float32(math.Abs(alphaFromTop.Float()) - capFromAlpha.Float())
		if v > 0 {
			inset = v
		}
	} else {
		// Fallback for browsers that don't yet expose alphabeticBaseline
		// (pre-2022 Safari): approximate with 20% of font size.
		inset = effectiveFontBasePx() * scale * 0.2
	}

	textTopInsetCache[cssFont] = inset
	return inset
}

func (canvas2DRenderer) textWidth(text string, scale float32) float32 {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		// No window yet — produce a reasonable estimate so widgets
		// that compute auto-sizing before the canvas exists don't
		// crash. Approximates the average glyph width at the given
		// scale.
		return float32(len([]rune(text))) * 0.55 * effectiveFontBasePx() * scale
	}
	cssFont, _ := activeFontCSS(scale)
	if cssFont == "" {
		size := float64(scale) * float64(effectiveFontBasePx())
		if size < 1 {
			size = 1
		}
		cssFont = fmt.Sprintf("%dpx %s", int(size+0.5), uiSystemFontStack)
	}
	ctx.Set("font", cssFont)
	m := ctx.Call("measureText", text)
	w := m.Get("width")
	if w.IsUndefined() {
		return float32(len([]rune(text))) * 0.55 * effectiveFontBasePx() * scale
	}
	return float32(w.Float())
}

func (canvas2DRenderer) drawLine(x1, y1, x2, y2, lineWidth float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	ctx.Set("strokeStyle", cssColor(color))
	ctx.Set("lineWidth", lineWidth)
	ctx.Call("beginPath")
	ctx.Call("moveTo", x1, y1)
	ctx.Call("lineTo", x2, y2)
	ctx.Call("stroke")
}

func (canvas2DRenderer) fillPolygon(points []float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	if len(points) < 6 {
		return
	}
	ctx.Set("fillStyle", cssColor(color))
	ctx.Call("beginPath")
	ctx.Call("moveTo", points[0], points[1])
	for i := 2; i < len(points); i += 2 {
		ctx.Call("lineTo", points[i], points[i+1])
	}
	ctx.Call("closePath")
	ctx.Call("fill")
}

func (canvas2DRenderer) fillArc(cx, cy, r, startAngle, endAngle float32, color [4]float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	if endAngle <= startAngle {
		return
	}
	ctx.Set("fillStyle", cssColor(color))
	ctx.Call("beginPath")
	ctx.Call("moveTo", cx, cy)
	// Canvas2D's arc takes (cx, cy, r, startAngle, endAngle, anticlockwise).
	// Angles use the same convention as fillArc — radians, 0 = right.
	ctx.Call("arc", cx, cy, r, startAngle, endAngle, false)
	ctx.Call("closePath")
	ctx.Call("fill")
}

func (canvas2DRenderer) drawImage(img *Image, x, y, w, h float32) {
	if img == nil {
		return
	}
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	jsImg, ok := imageRegistry[img.TextureID]
	if !ok || jsImg.IsUndefined() || jsImg.IsNull() {
		return
	}
	ctx.Call("drawImage", jsImg, x, y, w, h)
}

func (canvas2DRenderer) pushClip(x, y, w, h float32) {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	ctx.Call("save")
	ctx.Call("beginPath")
	ctx.Call("rect", x, y, w, h)
	ctx.Call("clip")
	gfxState.clipDepth++
}

func (canvas2DRenderer) popClip() {
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	if gfxState.clipDepth == 0 {
		return
	}
	ctx.Call("restore")
	gfxState.clipDepth--
}

// dropShadow on Canvas2D — uses the native ctx.shadow* properties.
// Trick: draw the shape filled in the shadow colour with the shadow
// filter enabled; the visible shape is the shadow itself, offset and
// blurred. The actual UI element renders on top afterwards via the
// caller's normal fillRoundedRect call, hiding the underlying fill.
func (canvas2DRenderer) dropShadow(x, y, w, h, r, offsetY, blur float32, color [4]float32) {
	if color[3] <= 0 {
		return
	}
	ctx := gfxState.ctx
	if ctx.IsUndefined() || ctx.IsNull() {
		return
	}
	ctx.Call("save")
	ctx.Set("shadowColor", cssColor(color))
	ctx.Set("shadowOffsetX", 0)
	ctx.Set("shadowOffsetY", offsetY)
	ctx.Set("shadowBlur", blur)
	roundRectPath(ctx, x, y, w, h, r)
	// Fill colour matches shadow colour so the underlying rectangle
	// itself is invisible against the shadow — only the offset/blur
	// silhouette shows. The caller draws the real element on top.
	ctx.Set("fillStyle", cssColor(color))
	ctx.Call("fill")
	ctx.Call("restore")
}

// init wires canvas2DRenderer in as the active renderer for the WASM
// build. Runs before any kLex script executes — package init order
// guarantees this since eval is a leaf dependency.
func init() {
	activeRenderer = canvas2DRenderer{}
}
