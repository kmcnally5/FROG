//go:build !js

package eval

import (
	"klex/ast"
	"math"

	"github.com/go-gl/gl/v4.1-core/gl"
)

func init() {
	// lineChart(data, x, y, w, h, [min, max]) → null
	// Draws a line chart. Uses fill() colour for the line and area fill (at 25% alpha).
	// If min/max are omitted they are derived from the data.
	Builtins["lineChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 7 {
			return typeError("lineChart expects 5-7 arguments: data, x, y, w, h, [min, max]", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("lineChart: data must be an array", ast.Pos{})
		}
		if !allNumeric(args[1:5]) {
			return typeError("lineChart: x, y, w, h must be numeric", ast.Pos{})
		}
		fx := float32(toFloat64(args[1]))
		fy := float32(toFloat64(args[2]))
		fw := float32(toFloat64(args[3]))
		fh := float32(toFloat64(args[4]))

		n := len(data.Elements)
		if n == 0 {
			return NULL
		}

		vals := make([]float64, n)
		for i, el := range data.Elements {
			vals[i] = toFloat64(el)
		}

		minVal, maxVal := vals[0], vals[0]
		for _, v := range vals {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
		if len(args) >= 6 {
			minVal = toFloat64(args[5])
		}
		if len(args) == 7 {
			maxVal = toFloat64(args[6])
		}
		if maxVal == minVal {
			maxVal = minVal + 1
		}

		screenX := func(i int) float32 {
			if n == 1 {
				return fx + fw*0.5
			}
			return fx + float32(i)/float32(n-1)*fw
		}
		screenY := func(v float64) float32 {
			t := (v - minVal) / (maxVal - minVal)
			return fy + fh - float32(t)*fh
		}

		// Background.
		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		// Subtle axis lines.
		axisColor := [4]float32{1, 1, 1, 0.08}
		drawPrimitive(gl.LINES, []float32{fx, fy + fh, fx + fw, fy + fh}, axisColor)
		drawPrimitive(gl.LINES, []float32{fx, fy, fx, fy + fh}, axisColor)

		baseline := fy + fh

		// Area fill (triangle strip below the line).
		fillVerts := make([]float32, 0, n*4)
		for i, v := range vals {
			fillVerts = append(fillVerts, screenX(i), baseline, screenX(i), screenY(v))
		}
		fillColor := gfx.fillColor
		fillColor[3] *= 0.25
		drawPrimitive(gl.TRIANGLE_STRIP, fillVerts, fillColor)

		// Line.
		lineVerts := make([]float32, 0, n*2)
		for i, v := range vals {
			lineVerts = append(lineVerts, screenX(i), screenY(v))
		}
		drawPrimitive(gl.LINE_STRIP, lineVerts, gfx.fillColor)

		// Dot at each data point.
		dotColor := gfx.fillColor
		for i, v := range vals {
			px, py := screenX(i), screenY(v)
			drawRoundedRectSDF(px-2.5, py-2.5, 5, 5, 2.5, dotColor, false, 0)
		}

		// Border.
		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.accent, true, 0.5)
		return NULL
	}}

	// barChart(data, x, y, w, h, [min, max]) → null
	// Draws a vertical bar chart. Uses fill() colour for bars.
	// Gap between bars is 20% of bar width.
	Builtins["barChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 7 {
			return typeError("barChart expects 5-7 arguments: data, x, y, w, h, [min, max]", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("barChart: data must be an array", ast.Pos{})
		}
		if !allNumeric(args[1:5]) {
			return typeError("barChart: x, y, w, h must be numeric", ast.Pos{})
		}
		fx := float32(toFloat64(args[1]))
		fy := float32(toFloat64(args[2]))
		fw := float32(toFloat64(args[3]))
		fh := float32(toFloat64(args[4]))

		n := len(data.Elements)
		if n == 0 {
			return NULL
		}

		vals := make([]float64, n)
		for i, el := range data.Elements {
			vals[i] = toFloat64(el)
		}

		minVal := 0.0
		maxVal := vals[0]
		for _, v := range vals {
			if v > maxVal {
				maxVal = v
			}
		}
		if len(args) >= 6 {
			minVal = toFloat64(args[5])
		}
		if len(args) == 7 {
			maxVal = toFloat64(args[6])
		}
		if maxVal == minVal {
			maxVal = minVal + 1
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.inputBg, false, 0)

		axisColor := [4]float32{1, 1, 1, 0.08}
		drawPrimitive(gl.LINES, []float32{fx, fy + fh, fx + fw, fy + fh}, axisColor)

		totalBarW := fw / float32(n)
		gapW := totalBarW * 0.15
		barW := totalBarW - gapW

		barColor := gfx.fillColor
		dimColor := barColor
		dimColor[3] *= 0.35

		for i, v := range vals {
			t := float64(0)
			if maxVal > minVal {
				t = math.Max(0, math.Min(1, (v-minVal)/(maxVal-minVal)))
			}
			bh := float32(t) * fh
			bx := fx + float32(i)*totalBarW + gapW*0.5
			by := fy + fh - bh
			// Dim track bar.
			drawRoundedRectSDF(bx, fy, barW, fh, 2, dimColor, false, 0)
			// Filled bar.
			if bh > 0 {
				drawRoundedRectSDF(bx, by, barW, bh, 2, barColor, false, 0)
			}
		}

		drawRoundedRectSDF(fx, fy, fw, fh, 4, gfx.uiTheme.accent, true, 0.5)
		return NULL
	}}

	// sparkline(data, x, y, w, h) → null
	// Minimal inline line chart — no background, no axes, just the line.
	// Auto-scales to the data range. Uses fill() colour.
	Builtins["sparkline"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return typeError("sparkline expects 5 arguments: data, x, y, w, h", ast.Pos{})
		}
		data, ok := args[0].(*Array)
		if !ok {
			return typeError("sparkline: data must be an array", ast.Pos{})
		}
		if !allNumeric(args[1:]) {
			return typeError("sparkline: x, y, w, h must be numeric", ast.Pos{})
		}
		fx := float32(toFloat64(args[1]))
		fy := float32(toFloat64(args[2]))
		fw := float32(toFloat64(args[3]))
		fh := float32(toFloat64(args[4]))

		n := len(data.Elements)
		if n < 2 {
			return NULL
		}

		vals := make([]float64, n)
		for i, el := range data.Elements {
			vals[i] = toFloat64(el)
		}
		minVal, maxVal := vals[0], vals[0]
		for _, v := range vals {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
		if maxVal == minVal {
			maxVal = minVal + 1
		}

		lineVerts := make([]float32, 0, n*2)
		for i, v := range vals {
			t := (v - minVal) / (maxVal - minVal)
			px := fx + float32(i)/float32(n-1)*fw
			py := fy + fh - float32(t)*fh
			lineVerts = append(lineVerts, px, py)
		}
		drawPrimitive(gl.LINE_STRIP, lineVerts, gfx.fillColor)
		return NULL
	}}

	// pieChart(data, colors, cx, cy, radius, [innerRadius]) → null
	// Draws a pie chart centred at (cx, cy). colors is an array of [r,g,b,a] arrays,
	// one per data element. Optional innerRadius > 0 draws a donut chart.
	Builtins["pieChart"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 5 || len(args) > 6 {
			return typeError("pieChart expects 5-6 arguments: data, colors, cx, cy, radius, [innerRadius]", ast.Pos{})
		}
		data, ok1 := args[0].(*Array)
		colorsArr, ok2 := args[1].(*Array)
		if !ok1 || !ok2 {
			return typeError("pieChart: data and colors must be arrays", ast.Pos{})
		}
		if !allNumeric(args[2:5]) {
			return typeError("pieChart: cx, cy, radius must be numeric", ast.Pos{})
		}
		cx := float32(toFloat64(args[2]))
		cy := float32(toFloat64(args[3]))
		radius := float32(toFloat64(args[4]))
		innerRadius := float32(0)
		if len(args) == 6 {
			innerRadius = float32(toFloat64(args[5]))
		}

		n := len(data.Elements)
		if n == 0 {
			return NULL
		}

		vals := make([]float64, n)
		total := 0.0
		for i, el := range data.Elements {
			v := math.Max(0, toFloat64(el))
			vals[i] = v
			total += v
		}
		if total == 0 {
			return NULL
		}

		sliceColor := func(i int) [4]float32 {
			if i < len(colorsArr.Elements) {
				if ca, ok := colorsArr.Elements[i].(*Array); ok && len(ca.Elements) >= 3 {
					r := float32(toFloat64(ca.Elements[0]))
					g := float32(toFloat64(ca.Elements[1]))
					b := float32(toFloat64(ca.Elements[2]))
					a := float32(1.0)
					if len(ca.Elements) >= 4 {
						a = float32(toFloat64(ca.Elements[3]))
					}
					return [4]float32{r, g, b, a}
				}
			}
			return gfx.fillColor
		}

		const segsPerCircle = 80
		angle := -math.Pi / 2 // start at top

		for i, v := range vals {
			if v == 0 {
				continue
			}
			fraction := v / total
			sweep := fraction * math.Pi * 2
			steps := int(fraction * segsPerCircle)
			if steps < 3 {
				steps = 3
			}
			col := sliceColor(i)

			if innerRadius <= 0 {
				// Solid slice — triangle fan.
				verts := make([]float32, 0, (steps+2)*2)
				verts = append(verts, cx, cy)
				for s := 0; s <= steps; s++ {
					a := angle + float64(s)/float64(steps)*sweep
					verts = append(verts, cx+float32(math.Cos(a))*radius, cy+float32(math.Sin(a))*radius)
				}
				drawPrimitive(gl.TRIANGLE_FAN, verts, col)
			} else {
				// Donut slice — triangle strip between inner and outer arc.
				verts := make([]float32, 0, (steps+1)*4)
				for s := 0; s <= steps; s++ {
					a := angle + float64(s)/float64(steps)*sweep
					ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
					verts = append(verts,
						cx+ca*innerRadius, cy+sa*innerRadius,
						cx+ca*radius, cy+sa*radius,
					)
				}
				drawPrimitive(gl.TRIANGLE_STRIP, verts, col)
			}

			// Thin separator line between slices for clarity.
			endA := angle + sweep
			sep := [4]float32{0, 0, 0, 0.4}
			drawPrimitive(gl.LINES, []float32{
				cx + float32(math.Cos(angle))*innerRadius, cy + float32(math.Sin(angle))*innerRadius,
				cx + float32(math.Cos(angle))*radius, cy + float32(math.Sin(angle))*radius,
				cx + float32(math.Cos(endA))*innerRadius, cy + float32(math.Sin(endA))*innerRadius,
				cx + float32(math.Cos(endA))*radius, cy + float32(math.Sin(endA))*radius,
			}, sep)

			angle += sweep
		}
		return NULL
	}}
}
