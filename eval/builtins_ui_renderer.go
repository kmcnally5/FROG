package eval

// builtins_ui_renderer.go — target-agnostic UI drawing surface.
//
// Shared widget bodies (eval/builtins_ui_widgets.go) call methods on a
// `uiRenderer` instead of touching GL / Canvas2D directly. Each backend
// registers its concrete implementation into `activeRenderer` from its
// init() — glRenderer on desktop (eval/builtins_ui_renderer_unix.go),
// canvas2DRenderer in the browser (eval/builtins_ui_renderer_wasm.go).
//
// The interface is deliberately small. It covers exactly what the
// Phase 1a widgets need: filled/stroked rounded rectangles, monospace
// or proportional text, and a text-width measurement that respects the
// active font. Phase 2+ will extend this incrementally — adding e.g.
// images, clipping, gradients — only when a widget being ported needs
// them.

// uiRenderer is the abstract drawing surface used by every shared
// widget. Implementations live in target-specific files; widgets see
// only the interface.
//
// Coordinate convention: pixel-space, origin top-left, Y grows down —
// matches both GL (after the ortho projection in window()) and the
// browser Canvas2D default. Colours are RGBA [0..1].
type uiRenderer interface {
	// fillRoundedRect paints a solid rounded-corner rectangle at
	// (x,y) of size (w,h) with corner radius r in `color`.
	fillRoundedRect(x, y, w, h, r float32, color [4]float32)

	// strokeRoundedRect outlines a rounded rectangle with a stroke of
	// half-width `strokeHalfW`. The stroke is drawn centred on the
	// edge — half inside, half outside the (x,y,w,h) bounds.
	strokeRoundedRect(x, y, w, h, r, strokeHalfW float32, color [4]float32)

	// drawText draws `text` anchored at (x,y). When centered is true,
	// (x,y) marks the *centre* of the text box (matching the desktop
	// drawText contract); otherwise (x,y) marks the top-left. `scale`
	// multiplies the active font size. The text is filled in `color`.
	drawText(text string, x, y int, centered bool, scale float32, color [4]float32)

	// textWidth returns the horizontal advance of `text` at `scale` in
	// pixels. Used by widgets that auto-size to fit their label.
	textWidth(text string, scale float32) float32

	// lineHeight returns the visual height of a single line of text at
	// `scale` in pixels. Widgets that left-align text inside a bounded
	// box (e.g. textInput) use this to compute a y that vertically
	// centres the text — the drawText contract for centered=false
	// places y at the TOP of the text, not the middle.
	lineHeight(scale float32) float32

	// textTopInset returns the gap between the em-square top (the Y
	// that drawText receives with centered=false) and the first visible
	// glyph pixel. Canvas2D's textBaseline="top" sits at the em-square
	// top, but actual glyphs start ~1-3 px lower due to internal
	// leading. The desktop GL renderer returns 0 because its SDF font
	// metrics are already calibrated to the cursor. Use this inset to
	// shift the text cursor down so it sits alongside the rendered text
	// rather than floating above it.
	textTopInset(scale float32) float32

	// drawLine paints a straight stroked line segment from (x1,y1) to
	// (x2,y2) at `lineWidth` pixels wide. Used by chart axes and
	// sparklines.
	drawLine(x1, y1, x2, y2, lineWidth float32, color [4]float32)

	// fillPolygon paints a filled polygon from a flat point list
	// [x1,y1,x2,y2,…]. Caller is responsible for closing the loop —
	// implementations join the last vertex back to the first
	// automatically. Used by chart area fills and pie slices when
	// the slice approximation needs an explicit path.
	fillPolygon(points []float32, color [4]float32)

	// fillArc paints a filled circular sector ("pie slice") centred
	// at (cx,cy) with radius r, sweeping clockwise from `startAngle`
	// to `endAngle` (radians, 0 = right, π/2 = down — Canvas2D
	// convention).
	fillArc(cx, cy, r, startAngle, endAngle float32, color [4]float32)

	// drawImage blits a kLex Image handle onto the canvas at (x,y)
	// scaled to (w,h). The Image type carries platform-specific data
	// (GL texture id on desktop, HTMLImageElement js.Value on WASM);
	// each backend dereferences accordingly.
	drawImage(img *Image, x, y, w, h float32)

	// pushClip restricts subsequent drawing to (x,y,w,h). Nested
	// pushClip calls intersect with the enclosing clip on desktop;
	// on Canvas2D each push is a save+clip and the corresponding
	// popClip restores. Every pushClip MUST be paired with a popClip.
	pushClip(x, y, w, h float32)

	// popClip restores the previous clip rectangle (or removes
	// clipping entirely if the stack is empty afterwards).
	popClip()

	// dropShadow paints a soft shadow below a rectangular region
	// (x,y,w,h,r) — offset downward by offsetY and blurred by blur
	// pixels. Used to give elevated UI elements (buttons on hover,
	// dropdown popups, modals) a sense of depth. Call BEFORE drawing
	// the actual shape on top.
	//
	// Implementation notes:
	//   - Canvas2D uses ctx.shadowOffsetY / shadowBlur / shadowColor.
	//   - Desktop GL approximates with a faded offset rounded rect
	//     (no true gaussian blur without a custom shader, but
	//     visually close enough for shadow-below use cases).
	dropShadow(x, y, w, h, r, offsetY, blur float32, color [4]float32)
}

// activeRenderer is the process-global concrete renderer. Set exactly
// once per process by the target-specific init() that wins the build
// tag. Nil before init() returns; widget code reaches this only inside
// a Builtin Fn (i.e. after the package has finished init), so a
// non-nil check is unnecessary.
var activeRenderer uiRenderer
