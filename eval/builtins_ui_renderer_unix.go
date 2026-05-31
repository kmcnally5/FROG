//go:build !js

package eval

// builtins_ui_renderer_unix.go — desktop renderer adapter for the
// shared uiRenderer interface. Wraps the existing OpenGL / SDF helpers
// (drawRoundedRectSDF, drawText, uiTextWidth, gfx.fillColor) so shared
// widget bodies can draw identically to the legacy path.
//
// glRenderer is intentionally stateless except for borrowing
// gfx.fillColor as the colour-spilling channel for drawText (the
// existing helper reads gfx.fillColor; we save/restore around each
// call so the widget's `color` argument wins without leaking).

import (
	"math"

	"github.com/go-gl/gl/v4.1-core/gl"
)

type glRenderer struct{}

func (glRenderer) fillRoundedRect(x, y, w, h, r float32, color [4]float32) {
	drawRoundedRectSDF(x, y, w, h, r, color, false, 0)
}

func (glRenderer) strokeRoundedRect(x, y, w, h, r, strokeHalfW float32, color [4]float32) {
	drawRoundedRectSDF(x, y, w, h, r, color, true, strokeHalfW)
}

func (glRenderer) drawText(text string, x, y int, centered bool, scale float32, color [4]float32) {
	saved := gfx.fillColor
	gfx.fillColor = color
	drawText(text, x, y, centered, scale)
	gfx.fillColor = saved
}

func (glRenderer) textWidth(text string, scale float32) float32 {
	return uiTextWidth(text, scale)
}

func (glRenderer) lineHeight(scale float32) float32 {
	return uiCharH(scale)
}

func (glRenderer) textTopInset(_ float32) float32 {
	return 0
}

func (glRenderer) drawLine(x1, y1, x2, y2, lineWidth float32, color [4]float32) {
	saved := gfx.strokeWidth
	gfx.strokeWidth = lineWidth
	drawPrimitive(gl.LINES, []float32{x1, y1, x2, y2}, color)
	gfx.strokeWidth = saved
}

func (glRenderer) fillPolygon(points []float32, color [4]float32) {
	if len(points) < 6 {
		return
	}
	drawPrimitive(gl.TRIANGLE_FAN, points, color)
}

func (glRenderer) fillArc(cx, cy, r, startAngle, endAngle float32, color [4]float32) {
	// Approximate the sector with a triangle fan: centre vertex first,
	// then `segments` points along the arc. Segment count scales with
	// sweep so big slices stay smooth without over-tesselating thin ones.
	sweep := endAngle - startAngle
	if sweep <= 0 {
		return
	}
	segments := int(math.Ceil(float64(sweep) * 16.0))
	if segments < 3 {
		segments = 3
	}
	pts := make([]float32, 0, 2*(segments+2))
	pts = append(pts, cx, cy)
	for i := 0; i <= segments; i++ {
		t := float32(i) / float32(segments)
		a := startAngle + sweep*t
		pts = append(pts, cx+r*float32(math.Cos(float64(a))), cy+r*float32(math.Sin(float64(a))))
	}
	drawPrimitive(gl.TRIANGLE_FAN, pts, color)
}

func (glRenderer) drawImage(img *Image, x, y, w, h float32) {
	if img == nil {
		return
	}
	drawImageGL(img, x, y, w, h)
}

func (glRenderer) pushClip(x, y, w, h float32) {
	newClip := clipRect{x: x, y: y, w: w, h: h}
	if len(gfx.clipStack) > 0 {
		newClip = intersectClip(gfx.clipStack[len(gfx.clipStack)-1], newClip)
	}
	gfx.clipStack = append(gfx.clipStack, newClip)
	applyScissor(newClip)
}

func (glRenderer) popClip() {
	if len(gfx.clipStack) == 0 {
		return
	}
	gfx.clipStack = gfx.clipStack[:len(gfx.clipStack)-1]
	if len(gfx.clipStack) > 0 {
		applyScissor(gfx.clipStack[len(gfx.clipStack)-1])
	} else {
		gl.Disable(gl.SCISSOR_TEST)
	}
}

// dropShadow on desktop GL — approximates a soft shadow with three
// concentric offset rectangles at decreasing alpha. Not a true
// gaussian, but cheap and reads as "elevated" against any background.
func (glRenderer) dropShadow(x, y, w, h, r, offsetY, blur float32, color [4]float32) {
	if color[3] <= 0 {
		return
	}
	// 3 layers, each offset slightly further and faded.
	for i, layer := range []float32{0.25, 0.17, 0.10} {
		c := color
		c[3] *= layer
		expand := blur * float32(i+1) * 0.5
		drawRoundedRectSDF(
			x-expand, y+offsetY-expand,
			w+expand*2, h+expand*2,
			r+expand, c, false, 0,
		)
	}
}

func init() {
	activeRenderer = glRenderer{}
}
