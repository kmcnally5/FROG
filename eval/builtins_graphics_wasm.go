//go:build js && wasm

package eval

// builtins_graphics_wasm.go — Canvas2D backend for kLex's graphics API
// in the browser. Parallel to eval/builtins_graphics.go (the GLFW/OpenGL
// implementation) which is //go:build !js.
//
// Implemented: window/render loop, transform stack (push/popMatrix,
// rotate, scale, translate), shapes (rect, circle, ellipse, arc, line,
// point, polygon, triangle, roundedRect), fill/stroke state, background,
// blendMode, gradient, shadows (shadow/noShadow), clipping (push/popClip),
// text with custom fonts (loadFont/textFont), image loading + drawing
// (loadImage/drawImage/imageSize), RGBA pixel round-trip
// (imageFromRgba/imageToRgba — which unlocks the builtins_image_fx.go
// filters in the browser), drawParticles, mouse + keyboard input, and
// drag-and-drop (droppedFiles, wired to the canvas drop listener below).
//
// saveImage is registered but intentionally raises a clear "no filesystem
// in the browser" RuntimeError — pixel export from the browser is out of
// scope by design; run on desktop to write image files.
//
// The WASM backend now covers every desktop graphics builtin. Keep this
// list honest: it is derived from the Builtins[…] registrations in this
// file vs builtins_graphics.go, NOT from memory.
//
// The render loop uses the same cooperative-async pattern as the worker
// bridge: window() calls requestAnimationFrame, blocks the eval
// goroutine on a Go channel, and a js.FuncOf rAF callback wakes the
// channel. Each frame's drawFn runs synchronously between rAF ticks.
//
// Canvas convention: the host HTML page MUST contain
//   <canvas id="klex-canvas"></canvas>
// before klex_eval runs. window() resizes it and grabs the 2d context.

import (
	"fmt"
	"klex/ast"
	"math"
	"strings"
	"sync"
	"syscall/js"
	"time"
)

// gfxState holds all live Canvas2D state. Populated by window(); read
// by every drawing builtin. Single global because kLex graphics is
// single-window single-context by design (matches the GLFW backend).
var gfxState struct {
	mu sync.Mutex

	canvas js.Value
	ctx    js.Value // CanvasRenderingContext2D
	width  int
	height int

	// Fill / stroke / line width — mirrors what canvas already tracks
	// so we can implement noFill()/noStroke() (canvas has no notion of
	// "disabled fill"; we just skip the call).
	fillCSS    string
	strokeCSS  string
	fillEnabled   bool
	strokeEnabled bool
	strokeWeight  float64

	// Frame accounting.
	frameCount    int
	frameRateCap  float64 // 0 = uncapped (vsync), >0 = throttle to N fps
	lastFrameNs   int64   // wall-clock ns at end of previous frame (for throttle)

	// Active pushClip nesting depth — incremented on push, decremented
	// on pop. Used to no-op a mismatched popClip rather than corrupt
	// canvas state by calling restore() with no matching save().
	clipDepth int

	// Drag-and-drop queue. Filenames are not portable in the browser,
	// so droppedFiles() returns data: URLs that loadImage can consume
	// directly. FileReader.readAsDataURL fills this asynchronously from
	// the drop event; the kLex builtin drains + returns it.
	droppedFiles []string

	// Input state — mutated by JS event handlers under mu.
	mouseX             float64
	mouseY             float64
	mouseDown          bool
	mouseClicked       bool // one-shot per frame
	mouseRightDown     bool
	mouseRightClicked  bool // one-shot per frame
	scrollDeltaY       float64 // per-frame wheel accumulation
	scrollDeltaX       float64
	keys               map[string]bool
	keysPressed        map[string]bool
	charBuf            []rune // typed characters this frame, drained by drawFn via getTypedChars

	// Key-repeat counts — accumulated each frame for special keys
	// the text widgets care about. Reset after drawFn alongside the
	// other one-shot inputs. Mirrors the desktop gfx.uiBackspaceCount
	// family on gfx.
	backspaceCount int
	deleteCount    int
	leftCount      int
	rightCount     int
	upCount        int
	downCount      int

	// Clipboard paste buffer — filled by the document `paste` event
	// listener with the text payload from event.clipboardData. The
	// shared text widgets consume it via uiInput().clipPaste; the
	// buffer is cleared after drawFn (one-shot per frame).
	pasteBuf string // one-shot per frame

	// Render loop frame ack channel — written by rAF callback.
	frameAck chan struct{}

	initialised bool
}

// init registers every Canvas2D builtin. Called automatically when
// the eval package loads under GOOS=js GOARCH=wasm.
func init() {
	gfxState.keys = make(map[string]bool)
	gfxState.keysPressed = make(map[string]bool)
	gfxState.fillEnabled = true
	gfxState.strokeEnabled = true
	gfxState.fillCSS = "#ffffff"
	gfxState.strokeCSS = "#000000"
	gfxState.strokeWeight = 1.0
	gfxState.frameAck = make(chan struct{}, 1)

	// ── window(w, h, title, drawFn) ──────────────────────────────────────
	//
	// Open the canvas (must exist in the host page with id "klex-canvas"),
	// run drawFn each frame via requestAnimationFrame, block until the
	// canvas is removed from the DOM or the script returns from drawFn
	// with a sentinel false. drawFn receives frameCount as its single arg.
	Builtins["window"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("window expects 4 arguments (width, height, title, drawFn)", ast.Pos{})
		}
		wArg, ok := args[0].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("window: width must be int, got %s", args[0].Type()), ast.Pos{})
		}
		hArg, ok := args[1].(*Integer)
		if !ok {
			return typeError(fmt.Sprintf("window: height must be int, got %s", args[1].Type()), ast.Pos{})
		}
		titleArg, ok := args[2].(*String)
		if !ok {
			return typeError(fmt.Sprintf("window: title must be string, got %s", args[2].Type()), ast.Pos{})
		}
		drawFn := args[3]
		if _, ok := drawFn.(*Function); !ok {
			if _, ok := drawFn.(*Builtin); !ok {
				return typeError(fmt.Sprintf("window: drawFn must be a function, got %s", drawFn.Type()), ast.Pos{})
			}
		}

		gfxState.mu.Lock()
		if gfxState.initialised {
			gfxState.mu.Unlock()
			return runtimeError("window: already open — only one window per kLex program", ast.Pos{})
		}

		canvas := js.Global().Get("document").Call("getElementById", "klex-canvas")
		if canvas.IsNull() || canvas.IsUndefined() {
			gfxState.mu.Unlock()
			return runtimeError("window: no <canvas id=\"klex-canvas\"> in host page", ast.Pos{})
		}

		// HiDPI: scale the canvas backing store by devicePixelRatio so
		// drawing renders at the display's native resolution. Widget
		// coordinates remain in CSS pixels via ctx.scale(dpr, dpr).
		dpr := js.Global().Get("devicePixelRatio").Float()
		if dpr <= 0 {
			dpr = 1.0
		}
		cssW := wArg.Value
		cssH := hArg.Value
		// CSS-size (visual layout) stays at the requested w/h.
		canvas.Get("style").Set("width", fmt.Sprintf("%dpx", cssW))
		canvas.Get("style").Set("height", fmt.Sprintf("%dpx", cssH))
		// Backing-store size scales by devicePixelRatio AND wasmSuperSample
		// (SSAA) so Canvas2D's greyscale-only text reads crisp on standard-DPI
		// screens. The CSS display size stays at cssW×cssH, so the browser
		// downscales the larger backing store on screen.
		backingScale := dpr * wasmSuperSample
		canvas.Set("width", int(float64(cssW)*backingScale))
		canvas.Set("height", int(float64(cssH)*backingScale))
		js.Global().Get("document").Set("title", titleArg.Value)

		ctx := canvas.Call("getContext", "2d")
		// Draw coordinates stay in CSS pixels (widget convention); the combined
		// DPR×supersample transform maps them onto the backing store.
		ctx.Call("scale", backingScale, backingScale)

		gfxState.canvas = canvas
		gfxState.ctx = ctx
		gfxState.width = cssW
		gfxState.height = cssH
		gfxState.initialised = true
		gfxState.mu.Unlock()

		// Reset on EVERY exit path — clean break, a drawFn error returned
		// from inside the loop, or a panic. Without this, the in-loop error
		// return below skipped the reset and left initialised=true, wedging
		// the next window() call with "already open" (only visible once you
		// run sketches back-to-back, e.g. the graphics playground).
		defer func() {
			gfxState.mu.Lock()
			gfxState.initialised = false
			gfxState.mu.Unlock()
		}()

		installInputHandlers()

		// rAF callback drains into frameAck so the eval goroutine
		// (parked in <-frameAck below) can wake up and run the next
		// drawFn invocation. js.FuncOf is intentionally not Released
		// — one Func for the lifetime of the window is fine.
		rafCallback := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			select {
			case gfxState.frameAck <- struct{}{}:
			default:
			}
			return nil
		})

		// Render loop. Yields cooperatively at every <-frameAck — the
		// Go-WASM scheduler returns control to JS while we wait, which
		// lets the browser actually paint and fire input events.
		raf := js.Global().Get("requestAnimationFrame")
		for {
			// Ask the browser for the next paint.
			raf.Invoke(rafCallback)
			<-gfxState.frameAck

			// Cooperative stop: an external host (e.g. the graphics
			// playground's Stop button) can set window.__klex_stop_requested
			// to break the loop cleanly, without the sketch's drawFn having
			// to return false. Read-and-cleared so the next run starts
			// fresh. Inert for hosts that never set the flag.
			if js.Global().Get("__klex_stop_requested").Truthy() {
				js.Global().Set("__klex_stop_requested", false)
				break
			}

			// Bump frame counter. Per-frame one-shot inputs
			// (mouseClicked, keysPressed) MUST stay set through drawFn
			// so widget hit-tests can see the click event; they're
			// cleared AFTER drawFn returns. Mirrors the desktop
			// builtins_graphics.go render-loop ordering.
			gfxState.mu.Lock()
			gfxState.frameCount++
			frame := gfxState.frameCount
			gfxState.mu.Unlock()

			// Invoke drawFn(frame). Uses the existing callable
			// dispatch path so closures, Functions, and Builtins all
			// work.
			result, _ := callCallable(drawFn, []Object{&Integer{Value: frame}})

			// Clear the one-shot inputs now that drawFn has had a
			// chance to consume them.
			gfxState.mu.Lock()
			gfxState.mouseClicked = false
			gfxState.mouseRightClicked = false
			gfxState.scrollDeltaY = 0
			gfxState.scrollDeltaX = 0
			gfxState.charBuf = gfxState.charBuf[:0]
			gfxState.backspaceCount = 0
			gfxState.deleteCount = 0
			gfxState.leftCount = 0
			gfxState.rightCount = 0
			gfxState.upCount = 0
			gfxState.downCount = 0
			gfxState.pasteBuf = ""
			for k := range gfxState.keysPressed {
				delete(gfxState.keysPressed, k)
			}
			gfxState.mu.Unlock()

			if isError(result) {
				return result
			}
			// drawFn returning explicit false exits the loop cleanly.
			if b, ok := result.(*Boolean); ok && !b.Value {
				break
			}

			// Throttle to frameRate(fps) if set. rAF normally targets
			// vsync (typically 60 fps); a cap below that holds the
			// frame open by sleeping the difference. A cap above
			// vsync has no effect — we can't render faster than rAF
			// fires anyway.
			gfxState.mu.Lock()
			fpsCap := gfxState.frameRateCap
			last := gfxState.lastFrameNs
			gfxState.mu.Unlock()
			now := time.Now().UnixNano()
			if fpsCap > 0 && last > 0 {
				targetNs := int64(1e9 / fpsCap)
				elapsedNs := now - last
				if elapsedNs < targetNs {
					time.Sleep(time.Duration(targetNs - elapsedNs))
				}
			}
			gfxState.mu.Lock()
			gfxState.lastFrameNs = time.Now().UnixNano()
			gfxState.mu.Unlock()

			// Tiny yield so any pending input events can flow through
			// while the canvas is being painted by the browser.
			runtimeGosched()
		}

		return NULL
	}}

	Builtins["winWidth"] = nullaryInt(func() int {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return gfxState.width
	}, "winWidth")
	Builtins["winHeight"] = nullaryInt(func() int {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return gfxState.height
	}, "winHeight")
	Builtins["frameCount"] = nullaryInt(func() int {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return gfxState.frameCount
	}, "frameCount")

	// ── State: fill / stroke / strokeWeight / noFill / noStroke ────────
	Builtins["fill"] = &Builtin{Fn: func(args []Object) Object {
		col, err := parseColor("fill", args)
		if err != nil {
			return err
		}
		gfxState.mu.Lock()
		gfxState.fillCSS = col
		gfxState.fillEnabled = true
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["stroke"] = &Builtin{Fn: func(args []Object) Object {
		col, err := parseColor("stroke", args)
		if err != nil {
			return err
		}
		gfxState.mu.Lock()
		gfxState.strokeCSS = col
		gfxState.strokeEnabled = true
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["strokeWeight"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("strokeWeight expects 1 argument", ast.Pos{})
		}
		w, ok := toFloat(args[0])
		if !ok {
			return typeError(fmt.Sprintf("strokeWeight: argument must be number, got %s", args[0].Type()), ast.Pos{})
		}
		gfxState.mu.Lock()
		gfxState.strokeWeight = w
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["noFill"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		gfxState.fillEnabled = false
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["noStroke"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		gfxState.strokeEnabled = false
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["background"] = &Builtin{Fn: func(args []Object) Object {
		col, err := parseColor("background", args)
		if err != nil {
			return err
		}
		ctx, ok := getCtx("background")
		if !ok {
			return runtimeError("background: window() must be called first", ast.Pos{})
		}
		ctx.Set("fillStyle", col)
		ctx.Call("fillRect", 0, 0, gfxState.width, gfxState.height)
		// Restore the saved fillStyle so subsequent fill()s aren't surprised.
		applyFillStyle(ctx)
		return NULL
	}}

	// gradient fills a rectangle with a two-colour linear gradient — the
	// Canvas2D parallel to the desktop GPU gradient in builtins_graphics.go.
	// Same signature and semantics: color1→color2 runs left→right for dir
	// "h" and top→bottom for "v". The active transform and clip apply
	// automatically (Canvas gradient coords are in current user space).
	Builtins["gradient"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 7 {
			return typeError("gradient expects 7 arguments: x, y, w, h, color1, color2, dir", ast.Pos{})
		}
		x, okx := toFloat(args[0])
		y, oky := toFloat(args[1])
		w, okw := toFloat(args[2])
		h, okh := toFloat(args[3])
		if !okx || !oky || !okw || !okh {
			return typeError("gradient: x, y, w, h must be numeric", ast.Pos{})
		}
		toColor4 := func(o Object) ([4]float32, bool) {
			arr, ok := o.(*Array)
			if !ok || len(arr.Elements) != 4 {
				return [4]float32{}, false
			}
			var c [4]float32
			for i, el := range arr.Elements {
				f, ok := toFloat(el)
				if !ok {
					return [4]float32{}, false
				}
				c[i] = float32(f)
			}
			return c, true
		}
		c1, ok1 := toColor4(args[4])
		c2, ok2 := toColor4(args[5])
		if !ok1 || !ok2 {
			return typeError("gradient: color1 and color2 must be [r,g,b,a] float arrays", ast.Pos{})
		}
		dirObj, ok3 := args[6].(*String)
		if !ok3 || (dirObj.Value != "h" && dirObj.Value != "v") {
			return typeError(`gradient: dir must be "h" (horizontal) or "v" (vertical)`, ast.Pos{})
		}
		ctx, ok := getCtx("gradient")
		if !ok {
			return runtimeError("gradient: window() must be called first", ast.Pos{})
		}
		x1, y1 := x+w, y // horizontal: left → right
		if dirObj.Value == "v" {
			x1, y1 = x, y+h // vertical: top → bottom
		}
		g := ctx.Call("createLinearGradient", x, y, x1, y1)
		g.Call("addColorStop", 0, cssColor(c1))
		g.Call("addColorStop", 1, cssColor(c2))
		ctx.Set("fillStyle", g)
		ctx.Call("fillRect", x, y, w, h)
		// Restore the saved fillStyle so subsequent fill()s aren't surprised.
		applyFillStyle(ctx)
		return NULL
	}}

	// ── Transform stack ────────────────────────────────────────────────
	Builtins["pushMatrix"] = ctxNullary("pushMatrix", "save")
	Builtins["popMatrix"] = ctxNullary("popMatrix", "restore")
	Builtins["translate"] = ctxTwoFloats("translate", "translate")
	Builtins["rotate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("rotate expects 1 argument (angle)", ast.Pos{})
		}
		a, ok := toFloat(args[0])
		if !ok {
			return typeError(fmt.Sprintf("rotate: argument must be number, got %s", args[0].Type()), ast.Pos{})
		}
		ctx, ok := getCtx("rotate")
		if !ok {
			return runtimeError("rotate: window() must be called first", ast.Pos{})
		}
		ctx.Call("rotate", a)
		return NULL
	}}
	Builtins["scale"] = ctxTwoFloats("scale", "scale")

	// ── Shapes ─────────────────────────────────────────────────────────
	Builtins["rect"] = &Builtin{Fn: func(args []Object) Object {
		return drawRect(args, 0)
	}}
	Builtins["roundedRect"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return runtimeError("roundedRect expects 5 arguments (x, y, w, h, r)", ast.Pos{})
		}
		r, ok := toFloat(args[4])
		if !ok {
			return typeError(fmt.Sprintf("roundedRect: r must be number, got %s", args[4].Type()), ast.Pos{})
		}
		return drawRect(args[:4], r)
	}}
	Builtins["circle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("circle expects 3 arguments (x, y, r)", ast.Pos{})
		}
		x, ok1 := toFloat(args[0])
		y, ok2 := toFloat(args[1])
		r, ok3 := toFloat(args[2])
		if !ok1 || !ok2 || !ok3 {
			return typeError("circle: all arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx("circle")
		if !ok {
			return runtimeError("circle: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("arc", x, y, r, 0.0, 2*math.Pi)
		strokeAndFill(ctx)
		return NULL
	}}
	Builtins["ellipse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("ellipse expects 4 arguments (x, y, rx, ry)", ast.Pos{})
		}
		x, ok1 := toFloat(args[0])
		y, ok2 := toFloat(args[1])
		rx, ok3 := toFloat(args[2])
		ry, ok4 := toFloat(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("ellipse: all arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx("ellipse")
		if !ok {
			return runtimeError("ellipse: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("ellipse", x, y, rx, ry, 0.0, 0.0, 2*math.Pi)
		strokeAndFill(ctx)
		return NULL
	}}
	Builtins["line"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("line expects 4 arguments (x1, y1, x2, y2)", ast.Pos{})
		}
		x1, ok1 := toFloat(args[0])
		y1, ok2 := toFloat(args[1])
		x2, ok3 := toFloat(args[2])
		y2, ok4 := toFloat(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("line: all arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx("line")
		if !ok {
			return runtimeError("line: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("moveTo", x1, y1)
		ctx.Call("lineTo", x2, y2)
		applyStrokeStyle(ctx)
		ctx.Call("stroke")
		return NULL
	}}
	Builtins["triangle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 {
			return runtimeError("triangle expects 6 arguments (x1, y1, x2, y2, x3, y3)", ast.Pos{})
		}
		vals := make([]float64, 6)
		for i := 0; i < 6; i++ {
			v, ok := toFloat(args[i])
			if !ok {
				return typeError("triangle: all arguments must be numbers", ast.Pos{})
			}
			vals[i] = v
		}
		ctx, ok := getCtx("triangle")
		if !ok {
			return runtimeError("triangle: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("moveTo", vals[0], vals[1])
		ctx.Call("lineTo", vals[2], vals[3])
		ctx.Call("lineTo", vals[4], vals[5])
		ctx.Call("closePath")
		strokeAndFill(ctx)
		return NULL
	}}

	// arc(x, y, r, startAngle, endAngle) — sweep an arc in radians.
	// Matches the desktop signature; respects fill + stroke state.
	Builtins["arc"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 {
			return runtimeError("arc expects 5 arguments (x, y, r, startAngle, endAngle)", ast.Pos{})
		}
		vals := make([]float64, 5)
		for i := 0; i < 5; i++ {
			v, ok := toFloat(args[i])
			if !ok {
				return typeError("arc: all arguments must be numbers", ast.Pos{})
			}
			vals[i] = v
		}
		ctx, ok := getCtx("arc")
		if !ok {
			return runtimeError("arc: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("arc", vals[0], vals[1], vals[2], vals[3], vals[4])
		strokeAndFill(ctx)
		return NULL
	}}

	// point(x, y) — single pixel-ish dot. Uses current stroke colour at
	// the current strokeWeight. Canvas2D has no native point primitive;
	// drawing a 1×1 fillRect at the stroke colour matches the desktop
	// backend's pixel-set behaviour.
	Builtins["point"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("point expects 2 arguments (x, y)", ast.Pos{})
		}
		x, ok1 := toFloat(args[0])
		y, ok2 := toFloat(args[1])
		if !ok1 || !ok2 {
			return typeError("point: arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx("point")
		if !ok {
			return runtimeError("point: window() must be called first", ast.Pos{})
		}
		gfxState.mu.Lock()
		size := gfxState.strokeWeight
		col := gfxState.strokeCSS
		gfxState.mu.Unlock()
		if size < 1 {
			size = 1
		}
		half := size / 2
		ctx.Set("fillStyle", col)
		ctx.Call("fillRect", x-half, y-half, size, size)
		applyFillStyle(ctx) // restore so subsequent fill()-based draws aren't surprised
		return NULL
	}}

	// drawParticles(xs, ys, rs, gs, bs, alphas, count, pointSize) → null
	//
	// Browser parallel to the desktop GPU particle batch. SoA layout:
	// xs/ys are positions; rs/gs/bs/alphas are per-particle colour
	// components (0–1). Particles with alpha < 0.01 are skipped. Each live
	// particle is a filled dot of diameter pointSize. The active transform
	// and clip apply automatically.
	//
	// Perf note: Canvas2D has no batched point draw, so this is one fill
	// per live particle — fine for hundreds/low-thousands; the desktop GL
	// build (single draw call) will outrun it at very high counts.
	Builtins["drawParticles"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 8 {
			return runtimeError("drawParticles expects 8 arguments: xs,ys,rs,gs,bs,alphas,count,pointSize", ast.Pos{})
		}
		xs, ok0 := args[0].(*Array)
		ys, ok1 := args[1].(*Array)
		rs, ok2 := args[2].(*Array)
		gs, ok3 := args[3].(*Array)
		bs, ok4 := args[4].(*Array)
		alphas, ok5 := args[5].(*Array)
		if !ok0 || !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return typeError("drawParticles: xs,ys,rs,gs,bs,alphas must be arrays", ast.Pos{})
		}
		cf, okc := toFloat(args[6])
		ps, okp := toFloat(args[7])
		if !okc || !okp {
			return typeError("drawParticles: count and pointSize must be numbers", ast.Pos{})
		}
		count := int(cf)
		// Clamp to the shortest array so a malformed SoA call can't trap
		// the WASM runtime on an out-of-range index.
		for _, a := range []*Array{xs, ys, rs, gs, bs, alphas} {
			if len(a.Elements) < count {
				count = len(a.Elements)
			}
		}
		half := ps / 2
		if count <= 0 || half <= 0 {
			return NULL
		}
		ctx, ok := getCtx("drawParticles")
		if !ok {
			return runtimeError("drawParticles: window() must be called first", ast.Pos{})
		}
		drew := false
		for i := 0; i < count; i++ {
			a, _ := toFloat(alphas.Elements[i])
			if a < 0.01 {
				continue
			}
			x, _ := toFloat(xs.Elements[i])
			y, _ := toFloat(ys.Elements[i])
			r, _ := toFloat(rs.Elements[i])
			g, _ := toFloat(gs.Elements[i])
			b, _ := toFloat(bs.Elements[i])
			ctx.Set("fillStyle", cssColor([4]float32{float32(r), float32(g), float32(b), float32(a)}))
			ctx.Call("beginPath")
			ctx.Call("arc", x, y, half, 0.0, 2*math.Pi)
			ctx.Call("fill")
			drew = true
		}
		if drew {
			// Restore the saved fillStyle so later fill()-based draws aren't surprised.
			applyFillStyle(ctx)
		}
		return NULL
	}}

	// polygon(points) — points is a flat [x1, y1, x2, y2, ...] array.
	// Requires at least 3 points (6 numbers). Respects fill + stroke.
	Builtins["polygon"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("polygon expects 1 argument (flat array [x1,y1,x2,y2,...])", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("polygon: points must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		if len(arr.Elements) < 6 || len(arr.Elements)%2 != 0 {
			return runtimeError(fmt.Sprintf("polygon: need at least 3 points (6 even-count values), got %d values", len(arr.Elements)), ast.Pos{})
		}
		coords := make([]float64, len(arr.Elements))
		for i, el := range arr.Elements {
			v, ok := toFloat(el)
			if !ok {
				return typeError(fmt.Sprintf("polygon: points[%d] must be a number, got %s", i, el.Type()), ast.Pos{})
			}
			coords[i] = v
		}
		ctx, ok := getCtx("polygon")
		if !ok {
			return runtimeError("polygon: window() must be called first", ast.Pos{})
		}
		ctx.Call("beginPath")
		ctx.Call("moveTo", coords[0], coords[1])
		for i := 2; i < len(coords); i += 2 {
			ctx.Call("lineTo", coords[i], coords[i+1])
		}
		ctx.Call("closePath")
		strokeAndFill(ctx)
		return NULL
	}}

	// ── Shadows (Canvas2D shadow* properties) ──────────────────────────
	//
	// shadow(ox, oy, blur)            — opaque-black shadow at default
	// shadow(ox, oy, blur, r, g, b, a) — coloured shadow
	//
	// Affects subsequent rect/circle/roundedRect/text draws until
	// noShadow() is called. Matches the desktop signature.
	Builtins["shadow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 7 {
			return runtimeError("shadow expects 3 or 7 arguments (offsetX, offsetY, blur [, r, g, b, a])", ast.Pos{})
		}
		ox, ok1 := toFloat(args[0])
		oy, ok2 := toFloat(args[1])
		blur, ok3 := toFloat(args[2])
		if !ok1 || !ok2 || !ok3 {
			return typeError("shadow: offset/blur must be numbers", ast.Pos{})
		}
		col := "rgba(0,0,0,1)"
		if len(args) == 7 {
			c, perr := parseColor("shadow", args[3:])
			if perr != nil {
				return perr
			}
			col = c
		}
		ctx, ok := getCtx("shadow")
		if !ok {
			return runtimeError("shadow: window() must be called first", ast.Pos{})
		}
		ctx.Set("shadowOffsetX", ox)
		ctx.Set("shadowOffsetY", oy)
		ctx.Set("shadowBlur", blur)
		ctx.Set("shadowColor", col)
		return NULL
	}}
	Builtins["noShadow"] = &Builtin{Fn: func(args []Object) Object {
		ctx, ok := getCtx("noShadow")
		if !ok {
			return runtimeError("noShadow: window() must be called first", ast.Pos{})
		}
		ctx.Set("shadowOffsetX", 0)
		ctx.Set("shadowOffsetY", 0)
		ctx.Set("shadowBlur", 0)
		ctx.Set("shadowColor", "rgba(0,0,0,0)")
		return NULL
	}}

	// ── Clipping (Canvas2D save/clip/restore) ──────────────────────────
	//
	// pushClip(x, y, w, h) saves the canvas state, sets a rect clip,
	// and stays in effect until popClip() restores. Nesting is supported
	// — each pushClip is a save, each popClip is a restore. Mismatched
	// pop without a push is a no-op (canvas state stack is empty).
	Builtins["pushClip"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("pushClip expects 4 arguments (x, y, w, h)", ast.Pos{})
		}
		x, ok1 := toFloat(args[0])
		y, ok2 := toFloat(args[1])
		w, ok3 := toFloat(args[2])
		h, ok4 := toFloat(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("pushClip: all arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx("pushClip")
		if !ok {
			return runtimeError("pushClip: window() must be called first", ast.Pos{})
		}
		ctx.Call("save")
		ctx.Call("beginPath")
		ctx.Call("rect", x, y, w, h)
		ctx.Call("clip")
		gfxState.mu.Lock()
		gfxState.clipDepth++
		gfxState.mu.Unlock()
		return NULL
	}}
	Builtins["popClip"] = &Builtin{Fn: func(args []Object) Object {
		ctx, ok := getCtx("popClip")
		if !ok {
			return runtimeError("popClip: window() must be called first", ast.Pos{})
		}
		gfxState.mu.Lock()
		if gfxState.clipDepth == 0 {
			gfxState.mu.Unlock()
			return NULL // mismatched popClip is a no-op — matches desktop
		}
		gfxState.clipDepth--
		gfxState.mu.Unlock()
		ctx.Call("restore")
		return NULL
	}}

	// ── blendMode(mode) ────────────────────────────────────────────────
	//
	// Maps kLex blend-mode names onto Canvas2D's globalCompositeOperation.
	// The common subset that works the same on every backend.
	Builtins["blendMode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("blendMode expects 1 argument (mode name)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("blendMode: mode must be string, got %s", args[0].Type()), ast.Pos{})
		}
		ctx, ok := getCtx("blendMode")
		if !ok {
			return runtimeError("blendMode: window() must be called first", ast.Pos{})
		}
		mode := canvasBlendMode(s.Value)
		ctx.Set("globalCompositeOperation", mode)
		return NULL
	}}

	// ── Text (default canvas font; loadFont/SDF deferred) ──────────────
	// loadFont(path[, ptSize]) — async font load via the FontFace API.
	//
	// Same channel-block pattern as loadImage: build a FontFace, call
	// its load() Promise, then/catch into a Go channel, eval goroutine
	// resumes when the Promise settles. The browser-side family name
	// is synthesised as "klex_font_<id>" so users can call loadFont
	// repeatedly without family collisions.
	//
	// Returns *Font on success or *Error on failure (matches desktop
	// `loadFont(path) -> Font` shape; callers check
	// `if type(font) == "ERROR"`).
	Builtins["loadFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("loadFont expects 1 or 2 arguments (path [, ptSize])", ast.Pos{})
		}
		pathStr, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("loadFont: path must be string, got %s", args[0].Type()), ast.Pos{})
		}
		ptSize := 16.0
		if len(args) == 2 {
			sz, ok := toFloat(args[1])
			if !ok {
				return typeError(fmt.Sprintf("loadFont: ptSize must be number, got %s", args[1].Type()), ast.Pos{})
			}
			ptSize = sz
		}

		fontFaceCtor := js.Global().Get("FontFace")
		if fontFaceCtor.IsUndefined() {
			return &Error{IsUserError: true, Code: "FONT_LOAD_FAILED",
				Message: "FontFace API not available in this host"}
		}

		fontRegistryMu.Lock()
		fontNextID++
		id := fontNextID
		family := fmt.Sprintf("klex_font_%d", id)
		fontRegistryMu.Unlock()

		src := fmt.Sprintf(`url(%q)`, pathStr.Value)
		fontFace := fontFaceCtor.New(family, src)

		type result struct{ err string }
		ch := make(chan result, 1)

		onResolved := js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
			js.Global().Get("document").Get("fonts").Call("add", fontFace)
			ch <- result{}
			return nil
		})
		onRejected := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			msg := "font load failed: " + pathStr.Value
			if len(args) > 0 {
				m := args[0].Get("message")
				if !m.IsUndefined() {
					msg = msg + " — " + m.String()
				}
			}
			ch <- result{err: msg}
			return nil
		})
		fontFace.Call("load").Call("then", onResolved, onRejected)

		r := <-ch
		if r.err != "" {
			return &Error{IsUserError: true, Code: "FONT_LOAD_FAILED", Message: r.err}
		}

		fontRegistryMu.Lock()
		fontRegistry[id] = fontHandle{family: family, ptSize: float32(ptSize)}
		fontRegistryMu.Unlock()

		// LineH ≈ 1.2 × ptSize matches typical CSS line-height defaults.
		// Canvas2D doesn't expose a portable font-line-height metric, so
		// this is the standard rule-of-thumb approximation.
		return &Font{TextureID: id, LineH: float32(ptSize) * 1.2}
	}}

	// textFont(font, str, x, y[, scale]) — draw with a Font from loadFont.
	Builtins["textFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return runtimeError("textFont expects 4 or 5 arguments (font, str, x, y [, scale])", ast.Pos{})
		}
		fnt, ok := args[0].(*Font)
		if !ok {
			return typeError(fmt.Sprintf("textFont: font must be a Font from loadFont, got %s", args[0].Type()), ast.Pos{})
		}
		s, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("textFont: str must be string, got %s", args[1].Type()), ast.Pos{})
		}
		x, ok1 := toFloat(args[2])
		y, ok2 := toFloat(args[3])
		if !ok1 || !ok2 {
			return typeError("textFont: x and y must be numbers", ast.Pos{})
		}
		scale := 1.0
		if len(args) == 5 {
			sc, ok := toFloat(args[4])
			if !ok {
				return typeError("textFont: scale must be number", ast.Pos{})
			}
			scale = sc
		}
		h, ok := lookupFont(fnt)
		if !ok {
			return runtimeError("textFont: font not registered (loadFont must be called in this WASM build)", ast.Pos{})
		}
		ctx, ok := getCtx("textFont")
		if !ok {
			return runtimeError("textFont: window() must be called first", ast.Pos{})
		}
		px := float64(h.ptSize) * scale
		if px < 1 {
			px = 1
		}
		ctx.Set("font", fmt.Sprintf("%gpx %q", px, h.family))
		applyFillStyle(ctx)
		ctx.Set("textBaseline", "top")
		ctx.Call("fillText", s.Value, x, y)
		return NULL
	}}

	// textWidth — measure a string. Accepts either form:
	//   textWidth(str [, scale])           default monospace
	//   textWidth(font, str [, scale])     Font from loadFont
	// Dispatched on the first arg's type so existing string-only callers
	// keep working.
	Builtins["textWidth"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 {
			return runtimeError("textWidth expects (str [, scale]) or (font, str [, scale])", ast.Pos{})
		}
		ctx, ok := getCtx("textWidth")
		if !ok {
			return runtimeError("textWidth: window() must be called first", ast.Pos{})
		}

		// Font-bound form: textWidth(font, str [, scale])
		if fnt, isFont := args[0].(*Font); isFont {
			if len(args) < 2 || len(args) > 3 {
				return runtimeError("textWidth expects (font, str [, scale])", ast.Pos{})
			}
			s, ok := args[1].(*String)
			if !ok {
				return typeError(fmt.Sprintf("textWidth: str must be string, got %s", args[1].Type()), ast.Pos{})
			}
			scale := 1.0
			if len(args) == 3 {
				sc, ok := toFloat(args[2])
				if !ok {
					return typeError("textWidth: scale must be number", ast.Pos{})
				}
				scale = sc
			}
			h, ok := lookupFont(fnt)
			if !ok {
				return runtimeError("textWidth: font not registered", ast.Pos{})
			}
			px := float64(h.ptSize) * scale
			if px < 1 {
				px = 1
			}
			ctx.Set("font", fmt.Sprintf("%gpx %q", px, h.family))
			metrics := ctx.Call("measureText", s.Value)
			return &Float{Value: metrics.Get("width").Float()}
		}

		// String-only form: textWidth(str [, scale]) — default monospace.
		if len(args) > 2 {
			return runtimeError("textWidth: too many arguments", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("textWidth: first arg must be Font or string, got %s", args[0].Type()), ast.Pos{})
		}
		scale := 1.0
		if len(args) == 2 {
			sc, ok := toFloat(args[1])
			if !ok {
				return typeError("textWidth: scale must be number", ast.Pos{})
			}
			scale = sc
		}
		px := int(8 * scale)
		if px < 1 {
			px = 1
		}
		ctx.Set("font", fmt.Sprintf("%dpx monospace", px))
		metrics := ctx.Call("measureText", s.Value)
		return &Float{Value: metrics.Get("width").Float()}
	}}

	// fontCharWidth / fontCharHeight — embedded monospace metrics at
	// scale 1. The desktop backend uses an 8×8 bitmap font; the WASM
	// backend uses Canvas2D's "monospace" at 8px which is close enough
	// for layout math. Returning the constant 8 matches the desktop
	// contract so cross-platform .lex code computes the same coords.
	Builtins["fontCharWidth"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("fontCharWidth expects no arguments", ast.Pos{})
		}
		return &Integer{Value: 8}
	}}
	Builtins["fontCharHeight"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("fontCharHeight expects no arguments", ast.Pos{})
		}
		return &Integer{Value: 8}
	}}

	// droppedFiles() — drain and return the list of files dropped onto
	// the canvas since the last call. The desktop backend returns OS
	// paths; the browser has no concept of paths, so we return data:
	// URLs instead. Those work directly with loadImage(), which is the
	// canonical way to handle drag-and-drop images:
	//
	//   for url in droppedFiles() {
	//       let img = loadImage(url)
	//       if type(img) != "ERROR" { drawImage(img, x, y) }
	//   }
	//
	// FileReader.readAsDataURL is async, so dropped files land in the
	// queue a few ms after the drop event fires; calling droppedFiles()
	// on the same frame as the drop may miss them. Polling each frame
	// (the typical pattern) drains them by the next frame at worst.
	Builtins["droppedFiles"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError("droppedFiles expects no arguments", ast.Pos{})
		}
		gfxState.mu.Lock()
		files := gfxState.droppedFiles
		gfxState.droppedFiles = nil
		gfxState.mu.Unlock()
		out := make([]Object, len(files))
		for i, f := range files {
			out[i] = &String{Value: f}
		}
		return &Array{Elements: out}
	}}

	// frameRate(fps) — cap the render loop to fps frames per second.
	// 0 means uncapped (run at the browser's vsync rate, typically 60).
	Builtins["frameRate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("frameRate expects 1 argument (fps)", ast.Pos{})
		}
		fps, ok := toFloat(args[0])
		if !ok {
			return typeError("frameRate: fps must be number", ast.Pos{})
		}
		gfxState.mu.Lock()
		gfxState.frameRateCap = fps
		gfxState.mu.Unlock()
		return NULL
	}}

	Builtins["text"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 4 {
			return runtimeError("text expects 3 or 4 arguments (str, x, y, scale?)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("text: str must be string, got %s", args[0].Type()), ast.Pos{})
		}
		x, ok1 := toFloat(args[1])
		y, ok2 := toFloat(args[2])
		if !ok1 || !ok2 {
			return typeError("text: x and y must be numbers", ast.Pos{})
		}
		scale := 1.0
		if len(args) == 4 {
			sc, ok := toFloat(args[3])
			if !ok {
				return typeError(fmt.Sprintf("text: scale must be number, got %s", args[3].Type()), ast.Pos{})
			}
			scale = sc
		}
		ctx, ok := getCtx("text")
		if !ok {
			return runtimeError("text: window() must be called first", ast.Pos{})
		}
		// 8px monospace at scale 1 matches the GLFW backend's embedded
		// font default.
		px := int(8 * scale)
		if px < 1 {
			px = 1
		}
		ctx.Set("font", fmt.Sprintf("%dpx monospace", px))
		applyFillStyle(ctx)
		ctx.Set("textBaseline", "top")
		ctx.Call("fillText", s.Value, x, y)
		return NULL
	}}

	// ── Image loading + drawing ────────────────────────────────────────
	//
	// loadImage(path) blocks the eval goroutine on a channel until
	// HTMLImageElement.onload (success) or onerror (failure) fires.
	// Same cooperative-async pattern as worker bridges + the render
	// loop — Go scheduler yields while we wait, JS event loop runs,
	// the load completes, the callback wakes the channel, eval resumes.
	//
	// The loaded js.Value HTMLImageElement is stored in jsImageRegistry
	// keyed by a synthetic uint32 (held in *Image.TextureID). Under the
	// GLFW backend TextureID is a real GL texture; here we just reuse
	// the field as an opaque numeric handle. drawImage looks up the
	// js.Value and hands it to ctx.drawImage().
	//
	// On failure loadImage returns an *Error directly (not a tuple) —
	// matches the desktop signature `loadImage(path) -> image`. Callers
	// can check `if type(img) == "ERROR"` (the natural kLex pattern for
	// fallible builtins that return a single value).
	Builtins["loadImage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("loadImage expects 1 argument (path)", ast.Pos{})
		}
		pathStr, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("loadImage: path must be string, got %s", args[0].Type()), ast.Pos{})
		}

		jsImg := js.Global().Get("Image").New()
		type result struct {
			w, h int
			err  string
		}
		ch := make(chan result, 1)

		onload := js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
			ch <- result{
				w: jsImg.Get("naturalWidth").Int(),
				h: jsImg.Get("naturalHeight").Int(),
			}
			return nil
		})
		onerror := js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
			ch <- result{err: "load failed: " + pathStr.Value}
			return nil
		})
		jsImg.Set("onload", onload)
		jsImg.Set("onerror", onerror)
		// Same-origin URLs work without crossOrigin. Setting it to
		// "anonymous" upgrades to a CORS request — fine when the
		// server allows it, fails (with no usable error message) when
		// it doesn't. Leave unset for the common case; cross-origin
		// images can be handled by the user in their host HTML.
		jsImg.Set("src", pathStr.Value)

		r := <-ch
		if r.err != "" {
			return &Error{
				IsUserError: true,
				Code:        "IMAGE_LOAD_FAILED",
				Message:     r.err,
			}
		}

		imageRegistryMu.Lock()
		imageNextID++
		id := imageNextID
		imageRegistry[id] = jsImg
		imageRegistryMu.Unlock()

		return &Image{TextureID: id, W: r.w, H: r.h}
	}}

	// drawImage(img, x, y) | drawImage(img, x, y, w, h)
	Builtins["drawImage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 5 {
			return runtimeError("drawImage expects 3 or 5 arguments (img, x, y [, w, h])", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError(fmt.Sprintf("drawImage: img must be an image, got %s", args[0].Type()), ast.Pos{})
		}
		x, ok1 := toFloat(args[1])
		y, ok2 := toFloat(args[2])
		if !ok1 || !ok2 {
			return typeError("drawImage: x and y must be numbers", ast.Pos{})
		}
		imageRegistryMu.Lock()
		jsImg, found := imageRegistry[img.TextureID]
		imageRegistryMu.Unlock()
		if !found {
			return runtimeError("drawImage: image not registered (was it loaded in this WASM build?)", ast.Pos{})
		}
		ctx, ok := getCtx("drawImage")
		if !ok {
			return runtimeError("drawImage: window() must be called first", ast.Pos{})
		}
		if len(args) == 3 {
			ctx.Call("drawImage", jsImg, x, y)
		} else {
			w, ok1 := toFloat(args[3])
			h, ok2 := toFloat(args[4])
			if !ok1 || !ok2 {
				return typeError("drawImage: w and h must be numbers", ast.Pos{})
			}
			ctx.Call("drawImage", jsImg, x, y, w, h)
		}
		return NULL
	}}

	// imageSize(img) → (width, height)
	Builtins["imageSize"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("imageSize expects 1 argument (img)", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError(fmt.Sprintf("imageSize: argument must be an image, got %s", args[0].Type()), ast.Pos{})
		}
		return &Tuple{Elements: []Object{
			&Integer{Value: img.W},
			&Integer{Value: img.H},
		}}
	}}

	// imageFromRgba(bytes, width, height) → image
	//
	// Browser parallel to the desktop builtin. Wraps raw RGBA8 pixels
	// (row-major, width*height*4 bytes) into a drawable kLex Image: the
	// pixels are painted onto an offscreen <canvas> via putImageData, and
	// that canvas is registered as the image's draw source so drawImage()
	// can blit it. The pixels are also kept on the Image so imageToRgba()
	// round-trips without a readback. Bytes are copied — later caller
	// mutations don't affect the image.
	Builtins["imageFromRgba"] = &Builtin{Fn: func(args []Object) (res Object) {
		if len(args) != 3 {
			return runtimeError("imageFromRgba expects (bytes, width, height)", ast.Pos{})
		}
		bs, bok := args[0].(*Bytes)
		w, wok := args[1].(*Integer)
		h, hok := args[2].(*Integer)
		if !bok || !wok || !hok {
			return typeError("imageFromRgba expects (bytes: bytes, width: int, height: int)", ast.Pos{})
		}
		if w.Value <= 0 || h.Value <= 0 {
			return runtimeError("imageFromRgba: width and height must be positive", ast.Pos{})
		}
		expected := w.Value * h.Value * 4
		if len(bs.Value) != expected {
			return runtimeError(fmt.Sprintf(
				"imageFromRgba: bytes length %d does not match width*height*4 = %d",
				len(bs.Value), expected), ast.Pos{})
		}
		defer func() {
			if r := recover(); r != nil {
				res = runtimeError(fmt.Sprintf("imageFromRgba: offscreen canvas build failed: %v", r), ast.Pos{})
			}
		}()
		pix := make([]byte, len(bs.Value))
		copy(pix, bs.Value)

		doc := js.Global().Get("document")
		canvas := doc.Call("createElement", "canvas")
		canvas.Set("width", w.Value)
		canvas.Set("height", h.Value)
		cctx := canvas.Call("getContext", "2d")
		imgData := cctx.Call("createImageData", w.Value, h.Value)
		// ImageData.data is a Uint8ClampedArray; view its backing buffer
		// as a Uint8Array so CopyBytesToJS accepts it, then write pixels in.
		u8 := js.Global().Get("Uint8Array").New(imgData.Get("data").Get("buffer"))
		js.CopyBytesToJS(u8, pix)
		cctx.Call("putImageData", imgData, 0, 0)

		imageRegistryMu.Lock()
		imageNextID++
		id := imageNextID
		imageRegistry[id] = canvas
		imageRegistryMu.Unlock()

		return &Image{TextureID: id, W: w.Value, H: h.Value, pixels: pix}
	}}

	// imageToRgba(img) → bytes
	//
	// Browser parallel to the desktop builtin. Returns the image's raw
	// RGBA8 pixels (row-major, width*height*4 bytes). Source priority:
	//   1. img.pixels — present for images made via imageFromRgba.
	//   2. the registered draw source (HTMLImageElement from loadImage, or
	//      an offscreen canvas) — drawn to a scratch canvas and read back
	//      with getImageData.
	// getImageData throws on a cross-origin-tainted canvas; the recover
	// surfaces that as a clean kLex RuntimeError instead of a WASM trap.
	Builtins["imageToRgba"] = &Builtin{Fn: func(args []Object) (res Object) {
		if len(args) != 1 {
			return runtimeError("imageToRgba expects 1 argument: img", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError(fmt.Sprintf("imageToRgba: argument must be an image, got %s", args[0].Type()), ast.Pos{})
		}
		if img.W <= 0 || img.H <= 0 {
			return runtimeError("imageToRgba: image has no dimensions", ast.Pos{})
		}
		expected := img.W * img.H * 4
		if len(img.pixels) >= expected {
			// Defensive copy so caller mutations don't leak into the image.
			out := make([]byte, expected)
			copy(out, img.pixels)
			return &Bytes{Value: out}
		}
		imageRegistryMu.Lock()
		src, found := imageRegistry[img.TextureID]
		imageRegistryMu.Unlock()
		if !found {
			return runtimeError("imageToRgba: image has no pixel data (was it loaded in this WASM build?)", ast.Pos{})
		}
		defer func() {
			if r := recover(); r != nil {
				res = runtimeError(fmt.Sprintf("imageToRgba: pixel read failed (cross-origin image?): %v", r), ast.Pos{})
			}
		}()
		doc := js.Global().Get("document")
		canvas := doc.Call("createElement", "canvas")
		canvas.Set("width", img.W)
		canvas.Set("height", img.H)
		cctx := canvas.Call("getContext", "2d")
		cctx.Call("drawImage", src, 0, 0)
		imgData := cctx.Call("getImageData", 0, 0, img.W, img.H)
		u8 := js.Global().Get("Uint8Array").New(imgData.Get("data").Get("buffer"))
		out := make([]byte, expected)
		js.CopyBytesToGo(out, u8)
		return &Bytes{Value: out}
	}}

	// saveImage(img, path) → null
	//
	// Desktop writes the image to disk; browsers have no filesystem, so in
	// the WASM build this is intentionally unsupported. It is registered
	// (rather than left undefined) purely so the failure is a clear,
	// actionable message instead of an "undefined name" error — the same
	// pattern _httpServe uses for browser-impossible operations. Pixel
	// export in the browser is out of scope by design; run on desktop to
	// save image files.
	Builtins["saveImage"] = &Builtin{Fn: func(args []Object) Object {
		return runtimeError("saveImage is not available in the browser — there is no filesystem. Run the script on a desktop kLex build to write image files.", ast.Pos{})
	}}

	// ── Input: mouse + keyboard ────────────────────────────────────────
	Builtins["mouseX"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return &Float{Value: gfxState.mouseX}
	}}
	Builtins["mouseY"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return &Float{Value: gfxState.mouseY}
	}}
	Builtins["mouseDown"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.mouseDown {
			return TRUE
		}
		return FALSE
	}}
	Builtins["mouseClicked"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.mouseClicked {
			return TRUE
		}
		return FALSE
	}}
	Builtins["mouseRightDown"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.mouseRightDown {
			return TRUE
		}
		return FALSE
	}}
	Builtins["mouseRightClicked"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.mouseRightClicked {
			return TRUE
		}
		return FALSE
	}}
	Builtins["mouseScrollY"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return &Float{Value: gfxState.scrollDeltaY}
	}}
	Builtins["mouseScrollX"] = &Builtin{Fn: func(args []Object) Object {
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		return &Float{Value: gfxState.scrollDeltaX}
	}}
	Builtins["keyDown"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keyDown expects 1 argument (key name)", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("keyDown: key must be string, got %s", args[0].Type()), ast.Pos{})
		}
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.keys[strings.ToLower(k.Value)] {
			return TRUE
		}
		return FALSE
	}}
	Builtins["keyPressed"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keyPressed expects 1 argument (key name)", ast.Pos{})
		}
		k, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("keyPressed: key must be string, got %s", args[0].Type()), ast.Pos{})
		}
		gfxState.mu.Lock()
		defer gfxState.mu.Unlock()
		if gfxState.keysPressed[strings.ToLower(k.Value)] {
			return TRUE
		}
		return FALSE
	}}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func getCtx(name string) (js.Value, bool) {
	gfxState.mu.Lock()
	defer gfxState.mu.Unlock()
	if !gfxState.initialised {
		return js.Value{}, false
	}
	return gfxState.ctx, true
}

// toFloat coerces an Object to a Go float64 — Integer or Float both accept.
func toFloat(o Object) (float64, bool) {
	switch v := o.(type) {
	case *Float:
		return v.Value, true
	case *Integer:
		return float64(v.Value), true
	}
	return 0, false
}

// parseColor turns 1/3/4 numeric args into a CSS rgba() string. Values
// in [0.0, 1.0]; clamped to that range. Matches the GLFW backend's
// fill()/stroke() shape.
func parseColor(fn string, args []Object) (string, Object) {
	var r, g, b, a float64
	switch len(args) {
	case 1:
		v, ok := toFloat(args[0])
		if !ok {
			return "", typeError(fmt.Sprintf("%s: value must be number, got %s", fn, args[0].Type()), ast.Pos{})
		}
		r, g, b, a = v, v, v, 1.0
	case 3:
		var ok bool
		if r, ok = toFloat(args[0]); !ok {
			return "", typeError(fmt.Sprintf("%s: r must be number, got %s", fn, args[0].Type()), ast.Pos{})
		}
		if g, ok = toFloat(args[1]); !ok {
			return "", typeError(fmt.Sprintf("%s: g must be number, got %s", fn, args[1].Type()), ast.Pos{})
		}
		if b, ok = toFloat(args[2]); !ok {
			return "", typeError(fmt.Sprintf("%s: b must be number, got %s", fn, args[2].Type()), ast.Pos{})
		}
		a = 1.0
	case 4:
		var ok bool
		if r, ok = toFloat(args[0]); !ok {
			return "", typeError(fmt.Sprintf("%s: r must be number, got %s", fn, args[0].Type()), ast.Pos{})
		}
		if g, ok = toFloat(args[1]); !ok {
			return "", typeError(fmt.Sprintf("%s: g must be number, got %s", fn, args[1].Type()), ast.Pos{})
		}
		if b, ok = toFloat(args[2]); !ok {
			return "", typeError(fmt.Sprintf("%s: b must be number, got %s", fn, args[2].Type()), ast.Pos{})
		}
		if a, ok = toFloat(args[3]); !ok {
			return "", typeError(fmt.Sprintf("%s: a must be number, got %s", fn, args[3].Type()), ast.Pos{})
		}
	default:
		return "", runtimeError(fmt.Sprintf("%s: expected 1, 3, or 4 numeric arguments, got %d", fn, len(args)), ast.Pos{})
	}
	clamp01 := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%g)",
		int(clamp01(r)*255+0.5),
		int(clamp01(g)*255+0.5),
		int(clamp01(b)*255+0.5),
		clamp01(a)), nil
}

func applyFillStyle(ctx js.Value) {
	gfxState.mu.Lock()
	defer gfxState.mu.Unlock()
	ctx.Set("fillStyle", gfxState.fillCSS)
}

func applyStrokeStyle(ctx js.Value) {
	gfxState.mu.Lock()
	defer gfxState.mu.Unlock()
	ctx.Set("strokeStyle", gfxState.strokeCSS)
	ctx.Set("lineWidth", gfxState.strokeWeight)
}

// strokeAndFill applies fill then stroke to the current path,
// respecting noFill()/noStroke() state.
func strokeAndFill(ctx js.Value) {
	gfxState.mu.Lock()
	fillOn := gfxState.fillEnabled
	strokeOn := gfxState.strokeEnabled
	gfxState.mu.Unlock()
	if fillOn {
		applyFillStyle(ctx)
		ctx.Call("fill")
	}
	if strokeOn {
		applyStrokeStyle(ctx)
		ctx.Call("stroke")
	}
}

// drawRect handles rect() and roundedRect() — r=0 means sharp corners.
// Canvas2D has native roundRect (Chromium 99+, Safari 16+, Firefox 113+)
// which covers every browser shipping in 2026.
func drawRect(args []Object, r float64) Object {
	if len(args) != 4 {
		return runtimeError("rect expects 4 arguments (x, y, w, h)", ast.Pos{})
	}
	x, ok1 := toFloat(args[0])
	y, ok2 := toFloat(args[1])
	w, ok3 := toFloat(args[2])
	h, ok4 := toFloat(args[3])
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return typeError("rect: all arguments must be numbers", ast.Pos{})
	}
	ctx, ok := getCtx("rect")
	if !ok {
		return runtimeError("rect: window() must be called first", ast.Pos{})
	}
	if r > 0 {
		ctx.Call("beginPath")
		ctx.Call("roundRect", x, y, w, h, r)
	} else {
		ctx.Call("beginPath")
		ctx.Call("rect", x, y, w, h)
	}
	strokeAndFill(ctx)
	return NULL
}

// nullaryInt is shorthand for a no-arg builtin returning an Integer.
func nullaryInt(fn func() int, name string) *Builtin {
	return &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError(name+" expects no arguments", ast.Pos{})
		}
		return &Integer{Value: fn()}
	}}
}

// ctxNullary registers a no-arg builtin that calls a single ctx method.
func ctxNullary(klexName, ctxMethod string) *Builtin {
	return &Builtin{Fn: func(args []Object) Object {
		if len(args) != 0 {
			return runtimeError(klexName+" expects no arguments", ast.Pos{})
		}
		ctx, ok := getCtx(klexName)
		if !ok {
			return runtimeError(klexName+": window() must be called first", ast.Pos{})
		}
		ctx.Call(ctxMethod)
		return NULL
	}}
}

// ctxTwoFloats registers a (float, float) -> null builtin.
func ctxTwoFloats(klexName, ctxMethod string) *Builtin {
	return &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError(klexName+" expects 2 arguments", ast.Pos{})
		}
		a, ok1 := toFloat(args[0])
		b, ok2 := toFloat(args[1])
		if !ok1 || !ok2 {
			return typeError(klexName+": both arguments must be numbers", ast.Pos{})
		}
		ctx, ok := getCtx(klexName)
		if !ok {
			return runtimeError(klexName+": window() must be called first", ast.Pos{})
		}
		ctx.Call(ctxMethod, a, b)
		return NULL
	}}
}

// installInputHandlers wires mouse and keyboard events on the canvas.
// Idempotent isn't strictly required — window() rejects a second call.
// wasmSuperSample renders the canvas backing store this many times larger
// than devicePixelRatio, then lets the browser downscale it on display —
// brute-force SSAA. Canvas2D fillText only does greyscale AA (no subpixel/LCD
// like DOM text), so on standard-DPI screens its text looks softer than HTML;
// supersampling recovers much of that crispness for text AND shapes. The cost
// is fill-rate: 2.0 means 4× the pixels. Tune toward 1.0 if a heavy sketch
// drops frames; 1.0 = off (DPR only). Affects every WASM graphics app.
var wasmSuperSample = 2.0

// targetIsHostEditable reports whether a key/clipboard event originated in a
// host-page editable field (e.g. the graphics playground's <textarea> code
// editor) rather than the canvas app. The WASM keeps its own hidden
// "__klex_clip" textarea focused to capture clipboard events — that one is
// explicitly NOT treated as host-editable, so canvas keyboard input keeps
// working. For any other <input>/<textarea>/contentEditable target we let the
// browser handle typing/paste natively instead of intercepting it.
func targetIsHostEditable(evt js.Value) bool {
	t := evt.Get("target")
	if t.IsNull() || t.IsUndefined() {
		return false
	}
	if t.Get("id").String() == "__klex_clip" {
		return false
	}
	switch strings.ToUpper(t.Get("tagName").String()) {
	case "INPUT", "TEXTAREA":
		return true
	}
	return t.Get("isContentEditable").Truthy()
}

func installInputHandlers() {
	canvas := gfxState.canvas

	// Persistent hidden <textarea> that owns the browser's clipboard
	// events. A <canvas> is not an editable element, so browsers never
	// fire reliable, gesture-synchronous copy/cut/paste on it. We keep
	// this textarea focused while the canvas app runs; the browser then
	// dispatches copy/cut/paste (which bubble to the document listeners
	// below) inside the user gesture — the only context where Safari
	// permits clipboard access. The async Clipboard API does not work
	// from the requestAnimationFrame callback the widgets run in.
	doc := js.Global().Get("document")
	clip := doc.Call("createElement", "textarea")
	clip.Set("id", "__klex_clip")
	clip.Set("autocomplete", "off")
	clip.Set("autocorrect", "off")
	clip.Set("autocapitalize", "off")
	clip.Set("spellcheck", false)
	cs := clip.Get("style")
	cs.Set("position", "fixed")
	cs.Set("top", "0")
	cs.Set("left", "0")
	cs.Set("width", "1px")
	cs.Set("height", "1px")
	cs.Set("padding", "0")
	cs.Set("border", "0")
	cs.Set("margin", "0")
	cs.Set("opacity", "0")
	cs.Set("outline", "none")
	cs.Set("resize", "none")
	cs.Set("overflow", "hidden")
	cs.Set("pointer-events", "none")
	cs.Set("zIndex", "-1")
	if body := doc.Get("body"); !body.IsNull() && !body.IsUndefined() {
		body.Call("appendChild", clip)
	}
	clipFocus := func() {
		clip.Call("focus", map[string]interface{}{"preventScroll": true})
	}
	clipFocus()

	// Keep the textarea drained. While it holds focus it also receives the
	// printable keydowns the app consumes via the window listener, so it
	// would otherwise accumulate typed text. We never read its value (paste
	// comes from the clipboard event's clipboardData), so clearing on every
	// input keeps it inert.
	clip.Call("addEventListener", "input", js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
		clip.Set("value", "")
		return nil
	}))

	// Mouse position — track relative to canvas bounding rect, not the
	// viewport, so the values match what kLex scripts expect (pixel
	// coords inside the drawing surface).
	canvas.Call("addEventListener", "mousemove", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		evt := args[0]
		rect := canvas.Call("getBoundingClientRect")
		x := evt.Get("clientX").Float() - rect.Get("left").Float()
		y := evt.Get("clientY").Float() - rect.Get("top").Float()
		gfxState.mu.Lock()
		gfxState.mouseX = x
		gfxState.mouseY = y
		gfxState.mu.Unlock()
		return nil
	}))

	canvas.Call("addEventListener", "mousedown", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		btn := args[0].Get("button").Int()
		gfxState.mu.Lock()
		switch btn {
		case 0: // left
			gfxState.mouseDown = true
			gfxState.mouseClicked = true
		case 2: // right
			gfxState.mouseRightDown = true
			gfxState.mouseRightClicked = true
		}
		gfxState.mu.Unlock()
		// Clicking the canvas moves browser focus off the hidden clipboard
		// textarea; pull it back so copy/cut/paste keep firing there.
		clipFocus()
		return nil
	}))
	canvas.Call("addEventListener", "mouseup", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		btn := args[0].Get("button").Int()
		gfxState.mu.Lock()
		switch btn {
		case 0:
			gfxState.mouseDown = false
		case 2:
			gfxState.mouseRightDown = false
		}
		gfxState.mu.Unlock()
		return nil
	}))
	// Suppress the browser context menu so kLex contextMenu() owns the right-click.
	canvas.Call("addEventListener", "contextmenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) >= 1 {
			args[0].Call("preventDefault")
		}
		return nil
	}))

	// Clipboard copy / cut. The focused text widget mirrors its live
	// selection into __klex_copy_buf every frame (uiPublishSelection), so
	// when Cmd/Ctrl+C or Cmd/Ctrl+X fires the text is already waiting here.
	// The event fires on the focused hidden textarea and bubbles to the
	// document; setData + preventDefault hand the selection to the OS
	// clipboard synchronously, inside the user gesture. (Cut deletes the
	// selection in the widget on the next frame via the keyX handler.)
	copyCut := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		buf := js.Global().Get("__klex_copy_buf")
		if buf.IsUndefined() || buf.IsNull() || buf.String() == "" {
			return nil
		}
		cd := args[0].Get("clipboardData")
		if cd.IsUndefined() || cd.IsNull() {
			return nil
		}
		cd.Call("setData", "text/plain", buf.String())
		args[0].Call("preventDefault")
		return nil
	})
	js.Global().Get("document").Call("addEventListener", "copy", copyCut)
	js.Global().Get("document").Call("addEventListener", "cut", copyCut)

	// Clipboard paste — fires on Cmd/Ctrl+V (or browser paste menu) on the
	// focused hidden textarea and bubbles to the document. preventDefault
	// stops the browser from also pasting into the textarea.
	js.Global().Get("document").Call("addEventListener", "paste", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		if targetIsHostEditable(args[0]) {
			return nil // native paste into the host editor
		}
		evt := args[0]
		data := evt.Get("clipboardData")
		if data.IsUndefined() || data.IsNull() {
			return nil
		}
		text := data.Call("getData", "text/plain").String()
		evt.Call("preventDefault")
		gfxState.mu.Lock()
		gfxState.pasteBuf = text
		gfxState.mu.Unlock()
		return nil
	}))

	// Wheel events. preventDefault keeps the page from scrolling
	// when the cursor is over the canvas — common expectation for
	// in-canvas scrolled widgets (lists, scrollAreas, tables).
	canvas.Call("addEventListener", "wheel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		evt := args[0]
		evt.Call("preventDefault")
		dy := evt.Get("deltaY").Float()
		dx := evt.Get("deltaX").Float()
		gfxState.mu.Lock()
		// kLex convention: positive = scroll content DOWN. Browser
		// wheel.deltaY is already positive on wheel-down, so pass
		// through; widgets multiply by their per-step factor.
		gfxState.scrollDeltaY += dy
		gfxState.scrollDeltaX += dx
		gfxState.mu.Unlock()
		return nil
	}), map[string]interface{}{"passive": false})

	// Key events live on window so they fire regardless of focus.
	win := js.Global().Get("window")
	win.Call("addEventListener", "keydown", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		// Let the host page's own editable fields (e.g. the graphics
		// playground editor) receive keystrokes natively — don't capture
		// or preventDefault them for the canvas app.
		if targetIsHostEditable(args[0]) {
			return nil
		}
		rawKey := args[0].Get("key").String()
		key := strings.ToLower(rawKey)
		evt := args[0]
		metaKey := evt.Get("metaKey").Bool()
		ctrlKey := evt.Get("ctrlKey").Bool()
		cmdOrCtrl := metaKey || ctrlKey
		// Suppress browser defaults for keys that drive in-canvas text
		// editing — otherwise Tab leaves the canvas, Backspace navigates
		// back, arrow keys scroll the page, etc.
		switch key {
		case "backspace", "delete", "arrowleft", "arrowright", "arrowup", "arrowdown", "tab":
			args[0].Call("preventDefault")
		}
		// Clipboard paste is handled by the native 'paste' event on the
		// persistent hidden textarea (see installInputHandlers) — no
		// per-keypress element creation or async readText() needed.
		gfxState.mu.Lock()
		if !gfxState.keys[key] {
			gfxState.keysPressed[key] = true // one-shot
		}
		gfxState.keys[key] = true
		// Single-character event.key values are printable text — push
		// them to the typed-char buffer so getTypedChars() returns them.
		// Multi-character values ("Enter", "Tab", "ArrowLeft", …) are
		// non-printable and only contribute to keys / keysPressed.
		// Skip when Cmd/Ctrl is held — shortcuts like Cmd+C must not
		// also insert the letter into the focused text widget.
		if r := []rune(rawKey); len(r) == 1 && !cmdOrCtrl {
			gfxState.charBuf = append(gfxState.charBuf, r[0])
		}
		// Key-repeat counts for special keys the text widgets consume.
		// Browser auto-repeats fire keydown repeatedly so this matches
		// the desktop GLFW callback (which also increments on repeat).
		switch key {
		case "backspace":
			gfxState.backspaceCount++
		case "delete":
			gfxState.deleteCount++
		case "arrowleft":
			gfxState.leftCount++
		case "arrowright":
			gfxState.rightCount++
		case "arrowup":
			gfxState.upCount++
		case "arrowdown":
			gfxState.downCount++
		}
		gfxState.mu.Unlock()
		return nil
	}))
	win.Call("addEventListener", "keyup", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		if targetIsHostEditable(args[0]) {
			return nil
		}
		key := strings.ToLower(args[0].Get("key").String())
		gfxState.mu.Lock()
		delete(gfxState.keys, key)
		gfxState.mu.Unlock()
		return nil
	}))

	// ── Drag and drop ─────────────────────────────────────────────────
	//
	// dragover MUST call preventDefault, otherwise the browser ignores
	// the subsequent drop. The drop handler reads each File via a fresh
	// FileReader; when the async readAsDataURL completes, the resulting
	// data: URL is pushed onto gfxState.droppedFiles for droppedFiles()
	// to harvest.
	canvas.Call("addEventListener", "dragover", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			args[0].Call("preventDefault")
		}
		return nil
	}))
	canvas.Call("addEventListener", "drop", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		evt := args[0]
		evt.Call("preventDefault")
		dt := evt.Get("dataTransfer")
		if dt.IsNull() || dt.IsUndefined() {
			return nil
		}
		files := dt.Get("files")
		if files.IsNull() || files.IsUndefined() {
			return nil
		}
		n := files.Get("length").Int()
		for i := 0; i < n; i++ {
			file := files.Index(i)
			reader := js.Global().Get("FileReader").New()
			// Release the callback once it fires — one FileReader per
			// dropped file, so without this each drop leaks a js.Func.
			var onload js.Func
			onload = js.FuncOf(func(this js.Value, _ []js.Value) interface{} {
				onload.Release()
				url := this.Get("result").String()
				gfxState.mu.Lock()
				gfxState.droppedFiles = append(gfxState.droppedFiles, url)
				gfxState.mu.Unlock()
				return nil
			})
			reader.Set("onload", onload)
			reader.Call("readAsDataURL", file)
		}
		return nil
	}))
}

// runtimeGosched yields to other goroutines / the JS event loop.
// time.Sleep(0) is the cheapest equivalent under Go-WASM.
func runtimeGosched() {
	// 0 nanoseconds = "yield without delay" in Go-WASM; the scheduler
	// returns control to JS, fires any queued callbacks (input events
	// etc.), then resumes us.
	time.Sleep(0)
}

// jsImageRegistry stores HTMLImageElement js.Values keyed by the
// synthetic uint32 we stuff into *Image.TextureID. Under the GLFW
// backend TextureID is a real GL texture name; here we just reuse the
// field as a registry key so the *Image struct stays unchanged across
// the WASM/desktop split.
var (
	imageRegistryMu sync.Mutex
	imageRegistry   = map[uint32]js.Value{}
	imageNextID     uint32
)

// fontRegistry mirrors the image registry pattern: the FontFace API
// added the font to document.fonts under a synthesised family name;
// we hold that family + the loadFont-time ptSize so textFont() can
// rebuild the CSS font shorthand on every draw call.
type fontHandle struct {
	family string
	ptSize float32
}

var (
	fontRegistryMu sync.Mutex
	fontRegistry   = map[uint32]fontHandle{}
	fontNextID     uint32
)

func lookupFont(f *Font) (fontHandle, bool) {
	fontRegistryMu.Lock()
	defer fontRegistryMu.Unlock()
	h, ok := fontRegistry[f.TextureID]
	return h, ok
}

// canvasBlendMode maps kLex blend-mode names onto Canvas2D's
// globalCompositeOperation values. Unknown names pass through
// verbatim so users who know the Canvas2D spec can use its names
// directly (e.g. "color-dodge", "lighter").
func canvasBlendMode(name string) string {
	switch strings.ToLower(name) {
	case "normal", "source-over", "":
		return "source-over"
	case "add", "additive", "lighter":
		return "lighter"
	case "multiply":
		return "multiply"
	case "screen":
		return "screen"
	case "overlay":
		return "overlay"
	case "darken":
		return "darken"
	case "lighten":
		return "lighten"
	case "difference":
		return "difference"
	case "exclusion":
		return "exclusion"
	case "subtract":
		// Canvas2D has no native "subtract"; "difference" is the closest
		// behaviour-equivalent that ships everywhere.
		return "difference"
	default:
		return name
	}
}
