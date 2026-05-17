//go:build !js

package eval

import (
	"klex/ast"
	"math"

	"github.com/go-gl/gl/v4.1-core/gl"
)

// vec2 is a 2D point used by the vector path tessellator.
type vec2 struct{ x, y float32 }

func init() {
	// beginPath() → null
	// Clear the current path and reset the pen. Must be called before building a new path.
	Builtins["beginPath"] = &Builtin{Fn: func(args []Object) Object {
		gfx.pathPts = gfx.pathPts[:0]
		gfx.pathHasStart = false
		return NULL
	}}

	// moveTo(x, y) → null
	// Move the pen to (x, y) without drawing. Starts a new contour.
	Builtins["moveTo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 || !allNumeric(args) {
			return runtimeError("moveTo expects 2 numeric arguments: x, y", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		gfx.pathPts = append(gfx.pathPts, vec2{x, y})
		gfx.pathPenX, gfx.pathPenY = x, y
		gfx.pathStartX, gfx.pathStartY = x, y
		gfx.pathHasStart = true
		return NULL
	}}

	// lineTo(x, y) → null
	// Add a straight line from the current pen to (x, y).
	Builtins["lineTo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 || !allNumeric(args) {
			return runtimeError("lineTo expects 2 numeric arguments: x, y", ast.Pos{})
		}
		if !gfx.pathHasStart {
			return runtimeError("lineTo: call moveTo first", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		gfx.pathPts = append(gfx.pathPts, vec2{x, y})
		gfx.pathPenX, gfx.pathPenY = x, y
		return NULL
	}}

	// bezierTo(cp1x, cp1y, cp2x, cp2y, x, y) → null
	// Add a cubic Bézier from the current pen to (x, y) via control points (cp1x,cp1y) and (cp2x,cp2y).
	Builtins["bezierTo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 || !allNumeric(args) {
			return runtimeError("bezierTo expects 6 numeric arguments: cp1x, cp1y, cp2x, cp2y, x, y", ast.Pos{})
		}
		if !gfx.pathHasStart {
			return runtimeError("bezierTo: call moveTo first", ast.Pos{})
		}
		p0 := vec2{gfx.pathPenX, gfx.pathPenY}
		p1 := vec2{float32(toFloat64(args[0])), float32(toFloat64(args[1]))}
		p2 := vec2{float32(toFloat64(args[2])), float32(toFloat64(args[3]))}
		p3 := vec2{float32(toFloat64(args[4])), float32(toFloat64(args[5]))}
		pathSubdivideCubic(p0, p1, p2, p3, &gfx.pathPts, 0.25)
		gfx.pathPenX, gfx.pathPenY = p3.x, p3.y
		return NULL
	}}

	// quadTo(cpx, cpy, x, y) → null
	// Add a quadratic Bézier from the current pen to (x, y) via control point (cpx, cpy).
	// Internally elevated to a cubic for uniform tessellation.
	Builtins["quadTo"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 || !allNumeric(args) {
			return runtimeError("quadTo expects 4 numeric arguments: cpx, cpy, x, y", ast.Pos{})
		}
		if !gfx.pathHasStart {
			return runtimeError("quadTo: call moveTo first", ast.Pos{})
		}
		p0 := vec2{gfx.pathPenX, gfx.pathPenY}
		qp := vec2{float32(toFloat64(args[0])), float32(toFloat64(args[1]))}
		p3 := vec2{float32(toFloat64(args[2])), float32(toFloat64(args[3]))}
		// Degree elevation: quadratic → cubic preserving the curve exactly.
		p1 := vec2{p0.x + (qp.x-p0.x)*2/3, p0.y + (qp.y-p0.y)*2/3}
		p2 := vec2{p3.x + (qp.x-p3.x)*2/3, p3.y + (qp.y-p3.y)*2/3}
		pathSubdivideCubic(p0, p1, p2, p3, &gfx.pathPts, 0.25)
		gfx.pathPenX, gfx.pathPenY = p3.x, p3.y
		return NULL
	}}

	// closePath() → null
	// Close the current contour by adding a line back to the last moveTo point.
	Builtins["closePath"] = &Builtin{Fn: func(args []Object) Object {
		if !gfx.pathHasStart {
			return NULL
		}
		gfx.pathPts = append(gfx.pathPts, vec2{gfx.pathStartX, gfx.pathStartY})
		gfx.pathPenX, gfx.pathPenY = gfx.pathStartX, gfx.pathStartY
		return NULL
	}}

	// fillPath() → null
	// Fill the current path with the current fill colour.
	// Uses GPU stencil even-odd rule — handles concave and complex shapes correctly.
	Builtins["fillPath"] = &Builtin{Fn: func(args []Object) Object {
		if len(gfx.pathPts) >= 3 {
			pathFill(gfx.pathPts, gfx.fillColor)
		}
		return NULL
	}}

	// strokePath() → null
	// Stroke the current path with the current stroke colour and strokeWeight.
	Builtins["strokePath"] = &Builtin{Fn: func(args []Object) Object {
		if len(gfx.pathPts) >= 2 {
			pathStroke(gfx.pathPts, gfx.strokeColor, gfx.strokeWidth)
		}
		return NULL
	}}
}

// pathSubdivideCubic recursively subdivides a cubic Bézier until each segment is within
// tol pixels of flat, using the Anti-Grain Geometry flatness check.
func pathSubdivideCubic(p0, p1, p2, p3 vec2, out *[]vec2, tol float32) {
	ux := 3*p1.x - 2*p0.x - p3.x
	uy := 3*p1.y - 2*p0.y - p3.y
	vx := 3*p2.x - 2*p3.x - p0.x
	vy := 3*p2.y - 2*p3.y - p0.y
	t2 := tol * tol * 16
	if ux*ux+uy*uy < t2 && vx*vx+vy*vy < t2 {
		*out = append(*out, p3)
		return
	}
	// De Casteljau split at t=0.5.
	q0 := vec2{(p0.x + p1.x) * 0.5, (p0.y + p1.y) * 0.5}
	q1 := vec2{(p1.x + p2.x) * 0.5, (p1.y + p2.y) * 0.5}
	q2 := vec2{(p2.x + p3.x) * 0.5, (p2.y + p3.y) * 0.5}
	r0 := vec2{(q0.x + q1.x) * 0.5, (q0.y + q1.y) * 0.5}
	r1 := vec2{(q1.x + q2.x) * 0.5, (q1.y + q2.y) * 0.5}
	s := vec2{(r0.x + r1.x) * 0.5, (r0.y + r1.y) * 0.5}
	pathSubdivideCubic(p0, q0, r0, s, out, tol)
	pathSubdivideCubic(s, r1, q2, p3, out, tol)
}

// pathFill fills an arbitrary polygon (including concave) using the GPU stencil even-odd method.
// Pass 1: triangle fan into stencil with INVERT — interior pixels toggle to non-zero.
// Pass 2: bounding quad painted where stencil≠0, ZERO clears stencil as it draws (self-cleaning).
func pathFill(pts []vec2, color [4]float32) {
	if len(pts) < 3 {
		return
	}

	minX, minY := pts[0].x, pts[0].y
	maxX, maxY := pts[0].x, pts[0].y
	for _, p := range pts[1:] {
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
	}

	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
	gl.UseProgram(gfx.shaderProg)
	gl.UniformMatrix4fv(gfx.projLoc, 1, false, &mvp[0])
	gl.BindVertexArray(gfx.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.vbo)

	fan := make([]float32, 0, len(pts)*2)
	for _, p := range pts {
		fan = append(fan, p.x, p.y)
	}

	// Pass 1: stencil only — INVERT each sample hit by the fan.
	gl.ColorMask(false, false, false, false)
	gl.Enable(gl.STENCIL_TEST)
	gl.StencilFunc(gl.ALWAYS, 0, 0xFF)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.INVERT)
	gl.BufferData(gl.ARRAY_BUFFER, len(fan)*4, gl.Ptr(fan), gl.STREAM_DRAW)
	gl.Uniform4f(gfx.colorLoc, 0, 0, 0, 0)
	gl.DrawArrays(gl.TRIANGLE_FAN, 0, int32(len(fan)/2))

	// Pass 2: paint bounding quad where stencil≠0, clear stencil as we go.
	gl.ColorMask(true, true, true, true)
	gl.StencilFunc(gl.NOTEQUAL, 0, 0xFF)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.ZERO)
	bbox := []float32{
		minX, minY,
		maxX, minY,
		maxX, maxY,
		minX, minY,
		maxX, maxY,
		minX, maxY,
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(bbox)*4, gl.Ptr(bbox), gl.STREAM_DRAW)
	gl.Uniform4f(gfx.colorLoc, color[0], color[1], color[2], color[3])
	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	gl.Disable(gl.STENCIL_TEST)
	gl.BindVertexArray(0)
}

// pathStroke renders the path outline as expanded quads (one per segment).
func pathStroke(pts []vec2, color [4]float32, strokeW float32) {
	if len(pts) < 2 {
		return
	}
	halfW := strokeW / 2
	verts := make([]float32, 0, (len(pts)-1)*12)
	for i := 1; i < len(pts); i++ {
		p0, p1 := pts[i-1], pts[i]
		dx := p1.x - p0.x
		dy := p1.y - p0.y
		l := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if l < 1e-6 {
			continue
		}
		nx := -dy / l * halfW
		ny := dx / l * halfW
		verts = append(verts,
			p0.x+nx, p0.y+ny,
			p0.x-nx, p0.y-ny,
			p1.x+nx, p1.y+ny,
			p0.x-nx, p0.y-ny,
			p1.x-nx, p1.y-ny,
			p1.x+nx, p1.y+ny,
		)
	}
	if len(verts) > 0 {
		drawPrimitive(gl.TRIANGLES, verts, color)
	}
}
