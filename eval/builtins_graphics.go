//go:build !js

package eval

import (
	"bytes"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"klex/ast"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ── Embedded GLSL shaders ────────────────────────────────────────────────────

// Particle shaders — per-vertex RGBA colour, smooth circular point sprites
// via gl_PointCoord. One draw call renders thousands of particles.
const partVertexShaderSrc = `
#version 410 core
in vec2 position;
in vec4 pColor;
out vec4 vColor;
uniform mat4 projection;
uniform float pointSize;
void main() {
    gl_Position  = projection * vec4(position, 0.0, 1.0);
    gl_PointSize = pointSize;
    vColor       = pColor;
}
` + "\x00"

const partFragmentShaderSrc = `
#version 410 core
in vec4 vColor;
out vec4 fragColor;
void main() {
    vec2  c = gl_PointCoord - vec2(0.5);
    float d = length(c) * 2.0;
    float a = smoothstep(1.0, 0.55, d);
    if (a < 0.004) discard;
    fragColor = vec4(vColor.rgb, vColor.a * a);
}
` + "\x00"

const vertexShaderSrc = `
#version 410 core
in vec2 position;
uniform mat4 projection;
void main() {
    gl_Position = projection * vec4(position, 0.0, 1.0);
}
` + "\x00"

const fragmentShaderSrc = `
#version 410 core
uniform vec4 color;
out vec4 fragColor;
void main() {
    fragColor = color;
}
` + "\x00"

const texVertexShaderSrc = `
#version 410 core
in vec2 position;
in vec2 texcoord;
out vec2 vTexCoord;
uniform mat4 projection;
void main() {
    gl_Position = projection * vec4(position, 0.0, 1.0);
    vTexCoord = texcoord;
}
` + "\x00"

// SDF shaders — mathematically perfect anti-aliased rounded rects and circles.
// A single quad covers the shape + 2px fringe; the fragment shader computes
// the signed distance field of a rounded box and smooth-steps the alpha.
// Works for circles too: pass r = min(w,h)/2 as both radius and the half-size.
const sdfVertexShaderSrc = `
#version 410 core
in vec2 position;
in vec2 localPos;
out vec2 vLocal;
uniform mat4 projection;
void main() {
    gl_Position = projection * vec4(position, 0.0, 1.0);
    vLocal = localPos;
}
` + "\x00"

// SDF AA tightening scale. Apple's deprecated GL 4.1 (frozen since 2018)
// returns slightly larger dFdx/dFdy values through the Metal compatibility
// shim than Mesa does on Linux — that widens the smoothstep AA band and
// softens edges. Multiplying the derivative by a sub-1.0 factor on macOS
// recovers most of the crispness without affecting Linux/Windows where the
// default (1.0) is already correct.
//
// 0.70 was chosen empirically as a starting point; a higher value (e.g.
// 0.85) gives gentler AA, lower (0.55) gives crisper but slightly aliased
// edges on near-axis-aligned strokes. Tweak here if visual results disagree.
var sdfAAScale = float32(1.0)

// sdfTextGamma applies a gamma curve to the SDF text coverage so antialiased
// edges read crisp rather than washed-out under sRGB alpha blending. <1.0
// fills thin edges (slightly heavier, sharper-looking); 1.0 = off; >1.0 thins.
// Affects text alpha only — never layout. Tune to taste and rebuild.
var sdfTextGamma = float32(0.85)

var sdfFragmentShaderSrc string
var texFragmentShaderSrc string

func init() {
	if runtime.GOOS == "darwin" {
		sdfAAScale = 0.70
	}

	sdfFragmentShaderSrc = fmt.Sprintf(`
#version 410 core
in vec2 vLocal;
uniform vec2  uHalfSize;
uniform float uRadius;
uniform vec4  uColor;
uniform int   uMode;    // 0 = fill, 1 = stroke
uniform float uStrokeW;
out vec4 fragColor;

float sdRoundedBox(vec2 p, vec2 b, float r) {
    vec2 d = abs(p) - b + r;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - r;
}

void main() {
    float dist = sdRoundedBox(vLocal, uHalfSize, uRadius);
    // Pixel-perfect AA width derived from screen-space derivative.
    // Scaled by sdfAAScale to tighten the band on macOS (see Go comment).
    float pxW  = length(vec2(dFdx(dist), dFdy(dist))) * %f;
    float alpha;
    if (uMode == 1) {
        alpha = smoothstep(pxW, -pxW, abs(dist) - uStrokeW);
    } else {
        alpha = smoothstep(pxW, -pxW, dist);
    }
    if (alpha < 0.004) discard;
    fragColor = uColor * alpha;
}
`+"\x00", sdfAAScale)

	texFragmentShaderSrc = fmt.Sprintf(`
#version 410 core
in vec2 vTexCoord;
uniform sampler2D tex;
uniform vec4 tint;
uniform int textMode;   // 0 = image, 1 = text (smooth-step alpha for crisp edges)
out vec4 fragColor;
void main() {
    vec4 s = texture(tex, vTexCoord);
    if (textMode == 1) {
        // SDF atlas: r channel stores normalised distance (0.5 = on edge).
        // fwidth gives the pixel size in SDF space — auto-sizes the AA fringe.
        // Scaled by sdfAAScale to tighten the band on macOS (see Go comment).
        float sdf   = s.r;
        float w     = length(vec2(dFdx(sdf), dFdy(sdf))) * %f;
        float alpha = smoothstep(0.5 - w, 0.5 + w, sdf);
        // Gamma-correct the coverage (see sdfTextGamma) so AA fringes read
        // crisp instead of washed-out under sRGB blending.
        alpha = pow(alpha, %f);
        fragColor   = vec4(tint.rgb, alpha * tint.a);
    } else {
        fragColor = s * tint;
    }
}
`+"\x00", sdfAAScale, sdfTextGamma)
}

// Gradient shaders — two-color linear fill interpolated in the fragment shader.
// uDir 0 = horizontal (left→right), 1 = vertical (top→bottom).
const gradVertexShaderSrc = `
#version 410 core
in vec2 position;
in vec2 texcoord;
out vec2 vUV;
uniform mat4 projection;
void main() {
    gl_Position = projection * vec4(position, 0.0, 1.0);
    vUV = texcoord;
}
` + "\x00"

const gradFragmentShaderSrc = `
#version 410 core
in vec2 vUV;
out vec4 fragColor;
uniform vec4 uColor1;
uniform vec4 uColor2;
uniform int  uDir;
void main() {
    float t = (uDir == 0) ? vUV.x : vUV.y;
    fragColor = mix(uColor1, uColor2, t);
}
` + "\x00"

// shadowFragmentShaderSrc — analytical Gaussian drop shadow for rounded rects.
// The vertex shader is sdfVertexShaderSrc (same position+localPos layout).
// vLocal is relative to the shadow quad centre; adding uOffset gives position
// relative to the original shape centre, where the SDF is computed.
const shadowFragmentShaderSrc = `
#version 410 core
in vec2 vLocal;
uniform vec2  uHalfSize;
uniform float uRadius;
uniform float uBlur;
uniform vec2  uOffset;
uniform vec4  uColor;
out vec4 fragColor;

float sdRoundedBox(vec2 p, vec2 b, float r) {
    vec2 d = abs(p) - b + r;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - r;
}

void main() {
    vec2  p     = vLocal + uOffset;
    float dist  = sdRoundedBox(p, uHalfSize, uRadius);
    float sigma = max(uBlur * 0.5, 0.5);
    float alpha = exp(-dist * dist / (2.0 * sigma * sigma));
    fragColor   = vec4(uColor.rgb, uColor.a * clamp(alpha, 0.0, 1.0));
}
` + "\x00"

// clipRect is a scissor rectangle in kLex screen-space (y-down, top-left origin).
type clipRect struct{ x, y, w, h float32 }

// intersectClip returns the intersection of two clip rects.
// If the rects don't overlap the result has w or h of 0 (draws nothing).
func intersectClip(a, b clipRect) clipRect {
	x1 := a.x
	if b.x > x1 {
		x1 = b.x
	}
	y1 := a.y
	if b.y > y1 {
		y1 = b.y
	}
	x2 := a.x + a.w
	if b.x+b.w < x2 {
		x2 = b.x + b.w
	}
	y2 := a.y + a.h
	if b.y+b.h < y2 {
		y2 = b.y + b.h
	}
	w := x2 - x1
	if w < 0 {
		w = 0
	}
	h := y2 - y1
	if h < 0 {
		h = 0
	}
	return clipRect{x1, y1, w, h}
}

// applyScissor enables the OpenGL scissor test for the given kLex-space rect.
// Converts y-down screen coords to OpenGL's y-up window coords.
func applyScissor(r clipRect) {
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(int32(r.x), int32(float32(gfx.winH)-r.y-r.h), int32(r.w), int32(r.h))
}

// uiPalette, toastEntry, pendingTooltip, pendingDropdown moved to
// eval/builtins_ui_state.go so the WASM build (which has no `gfx`
// struct) can still reference them via uiCore.

// defaultUIPalette also moved to eval/builtins_ui_state.go.

// ── Graphics state ───────────────────────────────────────────────────────────

var gfx struct {
	win              *glfw.Window
	shaderProg       uint32
	vao, vbo         uint32
	projLoc          int32
	colorLoc         int32
	fillColor        [4]float32
	strokeColor      [4]float32
	strokeWidth      float32
	doFill           bool
	doStroke         bool
	winW, winH       int
	frameCount       int
	mouseX           float64
	mouseY           float64
	mouseDown        bool
	mouseJustClicked bool
	charBuf          []rune
	// Phase 2
	ortho       mgl32.Mat4
	modelStack  []mgl32.Mat4
	keys        map[glfw.Key]bool
	justPressed map[glfw.Key]bool
	startTime   time.Time
	// Phase 3 — textured rendering
	texProg         uint32
	texProjLoc      int32
	texTintLoc      int32
	texTextModeLoc  int32
	texTexLoc       int32
	texVAO          uint32
	texVBO          uint32
	fontTex         uint32
	fontCellW       int
	fontCellH       int
	fontRenderScale int // atlas was rendered at this multiple of 1× DPI
	// Phase 4
	frameBudget time.Duration
	// Particle batch rendering
	partProg    uint32
	partProjLoc int32
	partSizeLoc int32
	partVAO     uint32
	partVBO     uint32
	// SDF rendering
	sdfProg       uint32
	sdfProjLoc    int32
	sdfColorLoc   int32
	sdfHSizeLoc   int32
	sdfRadiusLoc  int32
	sdfModeLoc    int32
	sdfStrokeWLoc int32
	sdfVAO        uint32
	sdfVBO        uint32
	// UI system
	// Theme, hover/active IDs, nextID counter, element registry, layout
	// cursors and list maps moved to uiCore (eval/builtins_ui_state.go)
	// so they're shared with the WASM Canvas2D backend.
	uiBackspaceCount  int     // number of backspaces this frame
	uiDeleteCount     int     // forward-deletes this frame (Delete key)
	uiLeftCount       int     // ← arrow presses this frame
	uiRightCount      int     // → arrow presses this frame
	uiUpCount         int     // ↑ arrow presses this frame
	uiDownCount       int     // ↓ arrow presses this frame
	uiScrollDelta     float64 // vertical mouse wheel delta this frame
	uiScrollX         float64 // horizontal mouse wheel delta this frame
	mouseRightClicked bool    // right-click fired this frame
	mouseRightDown    bool    // right button currently held
	// uiMenuOpenFrame moved to uiCore.menuOpenFrame.
	// uiPendingDropdown / uiToasts moved to uiCore for shared
	// popup/toast rendering across desktop + WASM.
	cursorArrow    *glfw.Cursor
	cursorIBeam    *glfw.Cursor
	cursorHand     *glfw.Cursor
	cursorResizeEW *glfw.Cursor // horizontal drag — vertical splitter bar
	cursorResizeNS *glfw.Cursor // vertical drag — horizontal splitter bar
	clipStack      []clipRect   // scissor stack; pushClip pushes, popClip pops and restores
	gradProg       uint32
	gradProjLoc    int32
	gradC1Loc      int32
	gradC2Loc      int32
	gradDirLoc     int32
	gradVAO        uint32
	gradVBO        uint32
	// uiUndoStacks / uiRedoStacks / uiTextCursor / uiTextAnchor /
	// uiTextScroll / uiTextBlink moved to uiCore for shared text-edit
	// state across desktop + WASM.
	droppedFiles []string // paths from the last file-drop event
	// uiLastElementID + tooltip timer / pending state moved to uiCore.
	// Layout cursors moved to uiCore (eval/builtins_ui_state.go).
	// Drop shadow state
	shadowActive  bool
	shadowOffX    float32
	shadowOffY    float32
	shadowBlur    float32
	shadowColor   [4]float32
	shadowProg    uint32
	shadowProjLoc int32
	shadowHSLoc   int32 // uHalfSize
	shadowRadLoc  int32 // uRadius
	shadowBlurLoc int32 // uBlur
	shadowOffLoc  int32 // uOffset
	shadowColLoc  int32 // uColor
	shadowVAO     uint32
	shadowVBO     uint32
	// Vector path builder
	pathPts      []vec2
	pathPenX     float32
	pathPenY     float32
	pathStartX   float32
	pathStartY   float32
	pathHasStart bool
}

// keyNames maps kLex string names to GLFW key constants.
var keyNames = map[string]glfw.Key{
	"A": glfw.KeyA, "B": glfw.KeyB, "C": glfw.KeyC, "D": glfw.KeyD,
	"E": glfw.KeyE, "F": glfw.KeyF, "G": glfw.KeyG, "H": glfw.KeyH,
	"I": glfw.KeyI, "J": glfw.KeyJ, "K": glfw.KeyK, "L": glfw.KeyL,
	"M": glfw.KeyM, "N": glfw.KeyN, "O": glfw.KeyO, "P": glfw.KeyP,
	"Q": glfw.KeyQ, "R": glfw.KeyR, "S": glfw.KeyS, "T": glfw.KeyT,
	"U": glfw.KeyU, "V": glfw.KeyV, "W": glfw.KeyW, "X": glfw.KeyX,
	"Y": glfw.KeyY, "Z": glfw.KeyZ,
	"0": glfw.Key0, "1": glfw.Key1, "2": glfw.Key2, "3": glfw.Key3,
	"4": glfw.Key4, "5": glfw.Key5, "6": glfw.Key6, "7": glfw.Key7,
	"8": glfw.Key8, "9": glfw.Key9,
	"SPACE":     glfw.KeySpace,
	"ENTER":     glfw.KeyEnter,
	"ESC":       glfw.KeyEscape,
	"LEFT":      glfw.KeyLeft,
	"RIGHT":     glfw.KeyRight,
	"UP":        glfw.KeyUp,
	"DOWN":      glfw.KeyDown,
	"BACKSPACE": glfw.KeyBackspace,
	"TAB":       glfw.KeyTab,
	"SHIFT":     glfw.KeyLeftShift,
	"CTRL":      glfw.KeyLeftControl,
	"F1":        glfw.KeyF1, "F2": glfw.KeyF2, "F3": glfw.KeyF3,
	"F4": glfw.KeyF4, "F5": glfw.KeyF5, "F6": glfw.KeyF6,
	"F7": glfw.KeyF7, "F8": glfw.KeyF8, "F9": glfw.KeyF9,
	"F10": glfw.KeyF10, "F11": glfw.KeyF11, "F12": glfw.KeyF12,
	"DELETE": glfw.KeyDelete,
	"INSERT": glfw.KeyInsert,
	"HOME":   glfw.KeyHome,
	"END":    glfw.KeyEnd,
	"PGUP":   glfw.KeyPageUp,
	"PGDOWN": glfw.KeyPageDown,
	"CMD":    glfw.KeyLeftSuper,
	"SUPER":  glfw.KeyLeftSuper,
}

func init() {
	// window — open a window and run the draw loop until it closes.
	//
	// The entry point for all graphics: opens a width×height window titled `title`
	// and calls drawFn(frameCount) every frame (at vsync, or the frameRate cap)
	// until the user closes it. Call every drawing builtin from inside drawFn. The
	// coordinate origin is the top-left, x right, y down. Blocks until the window
	// closes; a drawFn that returns false exits the loop early.
	//
	// @sig     window(width: int, height: int, title: string, drawFn: function) -> null
	// @param   width   window width in pixels
	// @param   height  window height in pixels
	// @param   title   the window title-bar text
	// @param   drawFn  callback invoked as drawFn(frameCount) once per frame
	// @returns null
	// @errors  TypeError if width/height aren't integers, title isn't a string, or drawFn isn't callable; RuntimeError unless given 4 arguments
	// @example no-run window(640, 480, "sketch", fn(frame) { background(0.1, 0.1, 0.1)  circle(320.0, 240.0, 40.0) })
	// @since   0.1.0
	// @see     background, circle, frameRate, frameCount
	Builtins["window"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("window expects 4 arguments: width, height, title, drawFn", ast.Pos{})
		}
		wObj, ok1 := args[0].(*Integer)
		hObj, ok2 := args[1].(*Integer)
		titleObj, ok3 := args[2].(*String)
		if !ok1 || !ok2 {
			return typeError("window: width and height must be integers", ast.Pos{})
		}
		if !ok3 {
			return typeError("window: title must be a string", ast.Pos{})
		}
		drawFn := args[3]
		if !IsCallable(drawFn) {
			return typeError(fmt.Sprintf("window: drawFn must be a function, got %s", drawFn.Type()), ast.Pos{})
		}

		if err := glfw.Init(); err != nil {
			return runtimeError(fmt.Sprintf("window: glfw init failed: %v", err), ast.Pos{})
		}
		defer glfw.Terminate()

		glfw.WindowHint(glfw.Resizable, glfw.True)
		glfw.WindowHint(glfw.Samples, 8)     // 8× MSAA — smooth edges on all geometry
		glfw.WindowHint(glfw.StencilBits, 8) // required for fillPath() even-odd stencil technique
		glfw.WindowHint(glfw.ContextVersionMajor, 4)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

		win, err := glfw.CreateWindow(int(wObj.Value), int(hObj.Value), titleObj.Value, nil, nil)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: could not create window: %v", err), ast.Pos{})
		}
		win.MakeContextCurrent()
		if gfx.frameBudget > 0 {
			glfw.SwapInterval(0) // manual frame timing active
		} else {
			glfw.SwapInterval(1) // vsync
		}

		if err := gl.Init(); err != nil {
			return runtimeError(fmt.Sprintf("window: gl init failed: %v", err), ast.Pos{})
		}
		gl.Enable(gl.MULTISAMPLE)

		prog, err := compileShaderProgram(vertexShaderSrc, fragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: shader compile failed: %v", err), ast.Pos{})
		}

		var vao, vbo uint32
		gl.GenVertexArrays(1, &vao)
		gl.GenBuffers(1, &vbo)
		gl.BindVertexArray(vao)
		gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
		posLoc := uint32(gl.GetAttribLocation(prog, gl.Str("position\x00")))
		gl.EnableVertexAttribArray(posLoc)
		gl.VertexAttribPointer(posLoc, 2, gl.FLOAT, false, 8, gl.PtrOffset(0))
		gl.BindVertexArray(0)

		texProg, err := compileShaderProgram(texVertexShaderSrc, texFragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: texture shader compile failed: %v", err), ast.Pos{})
		}
		var texVAO, texVBO uint32
		gl.GenVertexArrays(1, &texVAO)
		gl.GenBuffers(1, &texVBO)
		gl.BindVertexArray(texVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, texVBO)
		// Interleaved: position (2 floats) + texcoord (2 floats) = 16 bytes stride
		texPosLoc := uint32(gl.GetAttribLocation(texProg, gl.Str("position\x00")))
		texTCLoc := uint32(gl.GetAttribLocation(texProg, gl.Str("texcoord\x00")))
		gl.EnableVertexAttribArray(texPosLoc)
		gl.VertexAttribPointer(texPosLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(texTCLoc)
		gl.VertexAttribPointer(texTCLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(8))
		gl.BindVertexArray(0)

		gfx.win = win
		gfx.shaderProg = prog
		gfx.vao = vao
		gfx.vbo = vbo
		gfx.projLoc = gl.GetUniformLocation(prog, gl.Str("projection\x00"))
		gfx.colorLoc = gl.GetUniformLocation(prog, gl.Str("color\x00"))
		gfx.winW = int(wObj.Value)
		gfx.winH = int(hObj.Value)
		gfx.fillColor = [4]float32{1, 1, 1, 1}
		gfx.strokeColor = [4]float32{0, 0, 0, 1}
		gfx.strokeWidth = 1.0
		gfx.doFill = true
		gfx.doStroke = false
		gfx.frameCount = 0
		gfx.ortho = mgl32.Ortho2D(0, float32(gfx.winW), float32(gfx.winH), 0)
		gfx.modelStack = []mgl32.Mat4{mgl32.Ident4()}
		gfx.keys = make(map[glfw.Key]bool)
		gfx.justPressed = make(map[glfw.Key]bool)
		gfx.startTime = time.Now()
		gfx.texProg = texProg
		gfx.texProjLoc = gl.GetUniformLocation(texProg, gl.Str("projection\x00"))
		gfx.texTintLoc = gl.GetUniformLocation(texProg, gl.Str("tint\x00"))
		gfx.texTextModeLoc = gl.GetUniformLocation(texProg, gl.Str("textMode\x00"))
		gfx.texTexLoc = gl.GetUniformLocation(texProg, gl.Str("tex\x00"))
		gfx.texVAO = texVAO
		gfx.texVBO = texVBO
		gfx.fontTex = buildFontAtlas()

		// ── SDF shader setup ─────────────────────────────────────────────
		sdfProg, err := compileShaderProgram(sdfVertexShaderSrc, sdfFragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: SDF shader compile failed: %v", err), ast.Pos{})
		}
		var sdfVAO, sdfVBO uint32
		gl.GenVertexArrays(1, &sdfVAO)
		gl.GenBuffers(1, &sdfVBO)
		gl.BindVertexArray(sdfVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, sdfVBO)
		sdfPosLoc := uint32(gl.GetAttribLocation(sdfProg, gl.Str("position\x00")))
		sdfLocLoc := uint32(gl.GetAttribLocation(sdfProg, gl.Str("localPos\x00")))
		gl.EnableVertexAttribArray(sdfPosLoc)
		gl.VertexAttribPointer(sdfPosLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(sdfLocLoc)
		gl.VertexAttribPointer(sdfLocLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(8))
		gl.BindVertexArray(0)
		gfx.sdfProg = sdfProg
		gfx.sdfProjLoc = gl.GetUniformLocation(sdfProg, gl.Str("projection\x00"))
		gfx.sdfColorLoc = gl.GetUniformLocation(sdfProg, gl.Str("uColor\x00"))
		gfx.sdfHSizeLoc = gl.GetUniformLocation(sdfProg, gl.Str("uHalfSize\x00"))
		gfx.sdfRadiusLoc = gl.GetUniformLocation(sdfProg, gl.Str("uRadius\x00"))
		gfx.sdfModeLoc = gl.GetUniformLocation(sdfProg, gl.Str("uMode\x00"))
		gfx.sdfStrokeWLoc = gl.GetUniformLocation(sdfProg, gl.Str("uStrokeW\x00"))
		gfx.sdfVAO = sdfVAO
		gfx.sdfVBO = sdfVBO

		// ── Particle batch shader ─────────────────────────────────────────
		gl.Enable(gl.PROGRAM_POINT_SIZE) // allow vertex shader to set point size
		partProg, err := compileShaderProgram(partVertexShaderSrc, partFragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: particle shader compile failed: %v", err), ast.Pos{})
		}
		var partVAO, partVBO uint32
		gl.GenVertexArrays(1, &partVAO)
		gl.GenBuffers(1, &partVBO)
		gl.BindVertexArray(partVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, partVBO)
		// Interleaved: x,y (pos) + r,g,b,a (color) = 6 floats = 24 bytes stride
		pPosLoc := uint32(gl.GetAttribLocation(partProg, gl.Str("position\x00")))
		pColLoc := uint32(gl.GetAttribLocation(partProg, gl.Str("pColor\x00")))
		gl.EnableVertexAttribArray(pPosLoc)
		gl.VertexAttribPointer(pPosLoc, 2, gl.FLOAT, false, 24, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(pColLoc)
		gl.VertexAttribPointer(pColLoc, 4, gl.FLOAT, false, 24, gl.PtrOffset(8))
		gl.BindVertexArray(0)
		gfx.partProg = partProg
		gfx.partProjLoc = gl.GetUniformLocation(partProg, gl.Str("projection\x00"))
		gfx.partSizeLoc = gl.GetUniformLocation(partProg, gl.Str("pointSize\x00"))
		gfx.partVAO = partVAO
		gfx.partVBO = partVBO

		// ── Gradient shader ───────────────────────────────────────────────
		gradProg, err := compileShaderProgram(gradVertexShaderSrc, gradFragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: gradient shader compile failed: %v", err), ast.Pos{})
		}
		var gradVAO, gradVBO uint32
		gl.GenVertexArrays(1, &gradVAO)
		gl.GenBuffers(1, &gradVBO)
		gl.BindVertexArray(gradVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, gradVBO)
		gradPosLoc := uint32(gl.GetAttribLocation(gradProg, gl.Str("position\x00")))
		gradTCLoc := uint32(gl.GetAttribLocation(gradProg, gl.Str("texcoord\x00")))
		gl.EnableVertexAttribArray(gradPosLoc)
		gl.VertexAttribPointer(gradPosLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(gradTCLoc)
		gl.VertexAttribPointer(gradTCLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(8))
		gl.BindVertexArray(0)
		gfx.gradProg = gradProg
		gfx.gradProjLoc = gl.GetUniformLocation(gradProg, gl.Str("projection\x00"))
		gfx.gradC1Loc = gl.GetUniformLocation(gradProg, gl.Str("uColor1\x00"))
		gfx.gradC2Loc = gl.GetUniformLocation(gradProg, gl.Str("uColor2\x00"))
		gfx.gradDirLoc = gl.GetUniformLocation(gradProg, gl.Str("uDir\x00"))
		gfx.gradVAO = gradVAO
		gfx.gradVBO = gradVBO

		// ── Shadow shader ─────────────────────────────────────────────────
		// Reuses sdfVertexShaderSrc (same position + localPos layout).
		shadowProg, err := compileShaderProgram(sdfVertexShaderSrc, shadowFragmentShaderSrc)
		if err != nil {
			return runtimeError(fmt.Sprintf("window: shadow shader compile failed: %v", err), ast.Pos{})
		}
		var shadowVAO, shadowVBO uint32
		gl.GenVertexArrays(1, &shadowVAO)
		gl.GenBuffers(1, &shadowVBO)
		gl.BindVertexArray(shadowVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, shadowVBO)
		shPosLoc := uint32(gl.GetAttribLocation(shadowProg, gl.Str("position\x00")))
		shLocLoc := uint32(gl.GetAttribLocation(shadowProg, gl.Str("localPos\x00")))
		gl.EnableVertexAttribArray(shPosLoc)
		gl.VertexAttribPointer(shPosLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(0))
		gl.EnableVertexAttribArray(shLocLoc)
		gl.VertexAttribPointer(shLocLoc, 2, gl.FLOAT, false, 16, gl.PtrOffset(8))
		gl.BindVertexArray(0)
		gfx.shadowProg = shadowProg
		gfx.shadowProjLoc = gl.GetUniformLocation(shadowProg, gl.Str("projection\x00"))
		gfx.shadowHSLoc = gl.GetUniformLocation(shadowProg, gl.Str("uHalfSize\x00"))
		gfx.shadowRadLoc = gl.GetUniformLocation(shadowProg, gl.Str("uRadius\x00"))
		gfx.shadowBlurLoc = gl.GetUniformLocation(shadowProg, gl.Str("uBlur\x00"))
		gfx.shadowOffLoc = gl.GetUniformLocation(shadowProg, gl.Str("uOffset\x00"))
		gfx.shadowColLoc = gl.GetUniformLocation(shadowProg, gl.Str("uColor\x00"))
		gfx.shadowVAO = shadowVAO
		gfx.shadowVBO = shadowVBO
		gfx.shadowColor = [4]float32{0, 0, 0, 0.5}

		win.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
			gfx.mouseX = x
			gfx.mouseY = y
		})
		win.SetMouseButtonCallback(func(_ *glfw.Window, btn glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
			if btn == glfw.MouseButton1 {
				gfx.mouseDown = action != glfw.Release
				if action == glfw.Press {
					gfx.mouseJustClicked = true
				}
			}
			if btn == glfw.MouseButton2 {
				gfx.mouseRightDown = action != glfw.Release
				if action == glfw.Press {
					gfx.mouseRightClicked = true
				}
			}
		})
		win.SetCharCallback(func(_ *glfw.Window, char rune) {
			gfx.charBuf = append(gfx.charBuf, char)
		})
		win.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
			// Edit / navigation keys need a count (not just `justPressed`) so
			// the held-key auto-repeat events GLFW delivers translate into
			// multiple edits-per-frame. Otherwise only the first press
			// registers per frame and the user has to tap once per character.
			bumpEditCount := func() {
				switch key {
				case glfw.KeyBackspace:
					gfx.uiBackspaceCount++
				case glfw.KeyDelete:
					gfx.uiDeleteCount++
				case glfw.KeyLeft:
					gfx.uiLeftCount++
				case glfw.KeyRight:
					gfx.uiRightCount++
				case glfw.KeyUp:
					gfx.uiUpCount++
				case glfw.KeyDown:
					gfx.uiDownCount++
				}
			}
			switch action {
			case glfw.Press:
				gfx.keys[key] = true
				gfx.justPressed[key] = true
				bumpEditCount()
			case glfw.Repeat:
				bumpEditCount()
			case glfw.Release:
				gfx.keys[key] = false
			}
		})
		win.SetFramebufferSizeCallback(func(_ *glfw.Window, w, h int) {
			gfx.winW = w
			gfx.winH = h
			gfx.ortho = mgl32.Ortho2D(0, float32(w), float32(h), 0)
		})
		win.SetScrollCallback(func(_ *glfw.Window, xOffset, yOffset float64) {
			gfx.uiScrollDelta = yOffset
			gfx.uiScrollX = xOffset
		})
		win.SetDropCallback(func(_ *glfw.Window, names []string) {
			gfx.droppedFiles = append(gfx.droppedFiles, names...)
		})

		gl.Enable(gl.BLEND)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

		for !win.ShouldClose() {
			frameStart := time.Now()

			gl.Viewport(0, 0, int32(gfx.winW), int32(gfx.winH))
			gl.UseProgram(prog)

			gfx.modelStack = []mgl32.Mat4{mgl32.Ident4()}

			callCallable(drawFn, []Object{&Integer{Value: gfx.frameCount}})
			gfx.frameCount++

			// Clear one-shot flags after each frame.
			for k := range gfx.justPressed {
				delete(gfx.justPressed, k)
			}
			gfx.mouseJustClicked = false
			gfx.mouseRightClicked = false
			gfx.charBuf = gfx.charBuf[:0]
			gfx.uiBackspaceCount = 0
			gfx.uiDeleteCount = 0
			gfx.uiLeftCount = 0
			gfx.uiRightCount = 0
			gfx.uiUpCount = 0
			gfx.uiDownCount = 0
			gfx.uiScrollDelta = 0
			gfx.uiScrollX = 0
			// Safety: reset clip stack in case user left unmatched pushClip/popClip pairs.
			gfx.clipStack = gfx.clipStack[:0]
			gl.Disable(gl.SCISSOR_TEST)

			win.SwapBuffers()
			glfw.PollEvents()

			if gfx.frameBudget > 0 {
				if remaining := gfx.frameBudget - time.Since(frameStart); remaining > 0 {
					time.Sleep(remaining)
				}
			}
		}

		gl.DeleteBuffers(1, &vbo)
		gl.DeleteVertexArrays(1, &vao)
		gl.DeleteProgram(prog)
		return NULL
	}}

	// ── State builtins ───────────────────────────────────────────────────────

	// background — clear the whole canvas to a solid colour.
	//
	// Call at the top of each draw frame to erase the previous one. Colour
	// components are floats in 0.0–1.0: one arg is a grey level, three are RGB,
	// four are RGBA. (Two args is not accepted.)
	//
	// @sig     background(r: number, [g: number], [b: number], [a: number]) -> null
	// @param   r  red, or the grey level when called with a single argument (0.0–1.0)
	// @param   g  green (0.0–1.0)
	// @param   b  blue (0.0–1.0)
	// @param   a  alpha (0.0–1.0, default 1.0)
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 1, 3, or 4 arguments
	// @example no-run background(0.08, 0.08, 0.12)
	// @since   0.1.0
	// @see     fill, stroke, window
	Builtins["background"] = &Builtin{Fn: func(args []Object) Object {
		r, g, b, a, err := parseColor("background", args)
		if err != nil {
			return err
		}
		gl.ClearColor(r, g, b, a)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		return NULL
	}}

	// fill — set the colour shapes are filled with.
	//
	// Stays in effect until the next fill/noFill. Components are floats in
	// 0.0–1.0: one arg is a grey level, three are RGB, four are RGBA. Affects
	// rect, circle, ellipse, arc, polygon, triangle, roundedRect and text.
	//
	// @sig     fill(r: number, [g: number], [b: number], [a: number]) -> null
	// @param   r  red, or the grey level when called with a single argument (0.0–1.0)
	// @param   g  green (0.0–1.0)
	// @param   b  blue (0.0–1.0)
	// @param   a  alpha (0.0–1.0, default 1.0)
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 1, 3, or 4 arguments
	// @example no-run fill(0.9, 0.3, 0.2)
	// @since   0.1.0
	// @see     noFill, stroke, background
	Builtins["fill"] = &Builtin{Fn: func(args []Object) Object {
		r, g, b, a, err := parseColor("fill", args)
		if err != nil {
			return err
		}
		gfx.fillColor = [4]float32{r, g, b, a}
		gfx.doFill = true
		return NULL
	}}

	// noFill — stop filling shapes, leaving only their stroke.
	//
	// After this, rect/circle/etc. draw only their outline (if stroke is on).
	// Re-enable with any fill() call.
	//
	// @sig     noFill() -> null
	// @returns null
	// @errors  none
	// @example no-run noFill()
	// @since   0.1.0
	// @see     fill, noStroke
	Builtins["noFill"] = &Builtin{Fn: func(args []Object) Object {
		gfx.doFill = false
		return NULL
	}}

	// stroke — set the colour shape outlines are drawn with.
	//
	// Stays in effect until the next stroke/noStroke. Components are floats in
	// 0.0–1.0: one arg is a grey level, three are RGB, four are RGBA. Line width
	// is set separately with strokeWeight.
	//
	// @sig     stroke(r: number, [g: number], [b: number], [a: number]) -> null
	// @param   r  red, or the grey level when called with a single argument (0.0–1.0)
	// @param   g  green (0.0–1.0)
	// @param   b  blue (0.0–1.0)
	// @param   a  alpha (0.0–1.0, default 1.0)
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 1, 3, or 4 arguments
	// @example no-run stroke(1.0, 1.0, 1.0)
	// @since   0.1.0
	// @see     noStroke, strokeWeight, fill
	Builtins["stroke"] = &Builtin{Fn: func(args []Object) Object {
		r, g, b, a, err := parseColor("stroke", args)
		if err != nil {
			return err
		}
		gfx.strokeColor = [4]float32{r, g, b, a}
		gfx.doStroke = true
		return NULL
	}}

	// noStroke — stop drawing shape outlines, leaving only the fill.
	//
	// After this, shapes draw with no border. Re-enable with any stroke() call.
	//
	// @sig     noStroke() -> null
	// @returns null
	// @errors  none
	// @example no-run noStroke()
	// @since   0.1.0
	// @see     stroke, noFill
	Builtins["noStroke"] = &Builtin{Fn: func(args []Object) Object {
		gfx.doStroke = false
		return NULL
	}}

	// strokeWeight — set the line width for strokes, in pixels.
	//
	// Applies to line, point, and the outlines of filled shapes. Persists until
	// changed.
	//
	// @sig     strokeWeight(width: number) -> null
	// @param   width  the stroke width in pixels
	// @returns null
	// @errors  TypeError unless given a single numeric argument
	// @example no-run strokeWeight(2.0)
	// @since   0.1.0
	// @see     stroke, noStroke
	Builtins["strokeWeight"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 || !canArithmetic(args[0].Type()) {
			return typeError("strokeWeight expects 1 numeric argument", ast.Pos{})
		}
		gfx.strokeWidth = float32(toFloat64(args[0]))
		return NULL
	}}

	// shadow — turn on drop shadows for subsequent rect/circle/roundedRect draws.
	//
	// Offsets the shadow by (offsetX, offsetY) and softens it by `blur` pixels.
	// With three args the shadow is black at 50% alpha; pass r, g, b, a (0.0–1.0)
	// for an explicit colour. Stays on until noShadow.
	//
	// @sig     shadow(offsetX: number, offsetY: number, blur: number, [r: number], [g: number], [b: number], [a: number]) -> null
	// @param   offsetX  horizontal shadow offset in pixels
	// @param   offsetY  vertical shadow offset in pixels
	// @param   blur     blur radius in pixels
	// @param   r        shadow red (0.0–1.0)
	// @param   g        shadow green (0.0–1.0)
	// @param   b        shadow blue (0.0–1.0)
	// @param   a        shadow alpha (0.0–1.0)
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 3 or 7 arguments
	// @example no-run shadow(0.0, 4.0, 8.0)
	// @since   0.1.0
	// @see     noShadow, rect, roundedRect
	Builtins["shadow"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 7 {
			return runtimeError("shadow expects 3 or 7 arguments: offsetX, offsetY, blur [, r, g, b, a]", ast.Pos{})
		}
		if !allNumeric(args[:3]) {
			return typeError("shadow: offsetX, offsetY, blur must be numeric", ast.Pos{})
		}
		gfx.shadowOffX = float32(toFloat64(args[0]))
		gfx.shadowOffY = float32(toFloat64(args[1]))
		gfx.shadowBlur = float32(toFloat64(args[2]))
		if len(args) == 7 {
			if !allNumeric(args[3:]) {
				return typeError("shadow: r, g, b, a must be numeric", ast.Pos{})
			}
			gfx.shadowColor = [4]float32{
				float32(toFloat64(args[3])),
				float32(toFloat64(args[4])),
				float32(toFloat64(args[5])),
				float32(toFloat64(args[6])),
			}
		} else {
			gfx.shadowColor = [4]float32{0, 0, 0, 0.5}
		}
		gfx.shadowActive = true
		return NULL
	}}

	// noShadow — turn off drop shadows enabled by shadow().
	//
	// @sig     noShadow() -> null
	// @returns null
	// @errors  none
	// @example no-run noShadow()
	// @since   0.1.0
	// @see     shadow
	Builtins["noShadow"] = &Builtin{Fn: func(args []Object) Object {
		gfx.shadowActive = false
		return NULL
	}}

	// blendMode — set how subsequent draws blend with what's already on the canvas.
	//
	// Modes: "normal" (standard alpha blending, the default), "add" (additive —
	// fire, glow, light, particles), "multiply" (darkening, shadows), "screen"
	// (brightening). Call blendMode("normal") to reset after an effect.
	//
	// @sig     blendMode(mode: string) -> null
	// @param   mode  one of "normal", "add", "multiply", "screen"
	// @returns null
	// @errors  TypeError if mode isn't a string or isn't one of the four known modes
	// @example no-run blendMode("add")
	// @since   0.1.0
	// @see     fill, drawParticles
	Builtins["blendMode"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return typeError("blendMode expects 1 argument: mode string", ast.Pos{})
		}
		mode, ok := args[0].(*String)
		if !ok {
			return typeError("blendMode: argument must be a string", ast.Pos{})
		}
		switch mode.Value {
		case "normal":
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		case "add":
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		case "multiply":
			gl.BlendFunc(gl.DST_COLOR, gl.ZERO)
		case "screen":
			gl.BlendFunc(gl.ONE, gl.ONE_MINUS_SRC_COLOR)
		default:
			return typeError(`blendMode: unknown mode "`+mode.Value+`"; use "normal", "add", "multiply", or "screen"`, ast.Pos{})
		}
		return NULL
	}}

	// frameRate — cap the draw loop to a target frames-per-second.
	//
	// Pass the desired fps; pass 0 to disable the cap and sync to vsync instead.
	// The render loop sleeps out any time left in each frame's budget to hold the
	// rate steady.
	//
	// @sig     frameRate(fps: number) -> null
	// @param   fps  target frames per second, or 0 for vsync
	// @returns null
	// @errors  TypeError unless given a single numeric argument
	// @example no-run frameRate(30.0)
	// @since   0.1.0
	// @see     frameCount, window
	Builtins["frameRate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 || !canArithmetic(args[0].Type()) {
			return typeError("frameRate expects 1 numeric argument: fps (0 = vsync)", ast.Pos{})
		}
		fps := toFloat64(args[0])
		if fps <= 0 {
			gfx.frameBudget = 0
			if gfx.win != nil {
				glfw.SwapInterval(1)
			}
		} else {
			gfx.frameBudget = time.Duration(float64(time.Second) / fps)
			if gfx.win != nil {
				glfw.SwapInterval(0)
			}
		}
		return NULL
	}}

	// ── Shape builtins ───────────────────────────────────────────────────────

	// point — draw a single point at (x, y) using the stroke colour.
	//
	// The point's size is the current strokeWeight.
	//
	// @sig     point(x: number, y: number) -> null
	// @param   x  x coordinate in pixels
	// @param   y  y coordinate in pixels
	// @returns null
	// @errors  TypeError unless given 2 numeric arguments
	// @example no-run point(100.0, 100.0)
	// @since   0.1.0
	// @see     line, stroke, strokeWeight
	Builtins["point"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 || !allNumeric(args) {
			return typeError("point expects 2 numeric arguments: x, y", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		gl.PointSize(gfx.strokeWidth)
		drawPrimitive(gl.POINTS, []float32{x, y}, gfx.strokeColor)
		gl.PointSize(1)
		return NULL
	}}

	// drawParticles — render thousands of coloured points in a single draw call.
	//
	// The fast path for particle systems: positions and per-particle colours are
	// passed as parallel arrays (struct-of-arrays layout) and uploaded in one
	// batch, so cost stays flat as particle counts climb. `count` caps how many
	// to draw; particles with alpha < 0.01 are skipped. Pair with blendMode("add")
	// for glow. Each array index i describes one particle.
	//
	// @sig     drawParticles(xs: array, ys: array, rs: array, gs: array, bs: array, alphas: array, count: int, pointSize: number) -> null
	// @param   xs         x positions
	// @param   ys         y positions
	// @param   rs         red components (0.0–1.0)
	// @param   gs         green components (0.0–1.0)
	// @param   bs         blue components (0.0–1.0)
	// @param   alphas     alpha components (0.0–1.0)
	// @param   count      how many particles to draw (clamped to xs length)
	// @param   pointSize  diameter of each particle in pixels
	// @returns null
	// @errors  TypeError if the first six arguments aren't arrays; RuntimeError unless given 8 arguments
	// @example no-run drawParticles(xs, ys, rs, gs, bs, alphas, n, 4.0)
	// @since   0.1.0
	// @see     blendMode, point
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
		count := int(toFloat64(args[6]))
		pointSz := float32(toFloat64(args[7]))
		if count > len(xs.Elements) {
			count = len(xs.Elements)
		}

		// Build interleaved VBO: [x, y, r, g, b, a] per live particle
		verts := make([]float32, 0, count*6)
		for i := 0; i < count; i++ {
			a := float32(toFloat64(alphas.Elements[i]))
			if a < 0.01 {
				continue
			}
			verts = append(verts,
				float32(toFloat64(xs.Elements[i])),
				float32(toFloat64(ys.Elements[i])),
				float32(toFloat64(rs.Elements[i])),
				float32(toFloat64(gs.Elements[i])),
				float32(toFloat64(bs.Elements[i])),
				a,
			)
		}
		if len(verts) == 0 {
			return NULL
		}

		mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
		gl.UseProgram(gfx.partProg)
		gl.UniformMatrix4fv(gfx.partProjLoc, 1, false, &mvp[0])
		gl.Uniform1f(gfx.partSizeLoc, pointSz)
		gl.BindVertexArray(gfx.partVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, gfx.partVBO)
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
		gl.DrawArrays(gl.POINTS, 0, int32(len(verts)/6))
		gl.BindVertexArray(0)
		return NULL
	}}

	// rect — draw a rectangle with its top-left corner at (x, y).
	//
	// Honours the current fill and stroke state, and any active shadow().
	//
	// @sig     rect(x: number, y: number, w: number, h: number) -> null
	// @param   x  left edge in pixels
	// @param   y  top edge in pixels
	// @param   w  width in pixels
	// @param   h  height in pixels
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 4 arguments
	// @example no-run rect(20.0, 20.0, 120.0, 80.0)
	// @since   0.1.0
	// @see     roundedRect, circle, fill, stroke
	Builtins["rect"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("rect expects 4 arguments: x, y, w, h", ast.Pos{})
		}
		if !allNumeric(args) {
			return typeError("rect: all arguments must be numeric", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		w := float32(toFloat64(args[2]))
		h := float32(toFloat64(args[3]))

		drawShadowShape(x, y, w, h, 0)
		verts := []float32{
			x, y,
			x + w, y,
			x + w, y + h,
			x, y + h,
		}
		if gfx.doFill {
			drawPrimitive(gl.TRIANGLE_FAN, verts, gfx.fillColor)
		}
		if gfx.doStroke {
			drawPrimitive(gl.LINE_LOOP, verts, gfx.strokeColor)
		}
		return NULL
	}}

	// circle — draw a filled and/or stroked circle centred at (x, y).
	//
	// Honours the current fill and stroke state (fill / stroke / noFill /
	// noStroke) and is rendered with a signed-distance field, so the edge stays
	// crisp at any radius. Call inside a window() draw function.
	//
	// @sig     circle(x: number, y: number, radius: number) -> null
	// @param   x       centre x in pixels
	// @param   y       centre y in pixels
	// @param   radius  radius in pixels
	// @returns null
	// @errors  RuntimeError unless given exactly 3 numeric arguments
	// @example no-run circle(200.0, 150.0, 40.0)
	// @since   0.1.0
	// @see     ellipse, rect, fill, stroke
	Builtins["circle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 {
			return runtimeError("circle expects 3 arguments: x, y, radius", ast.Pos{})
		}
		if !allNumeric(args) {
			return typeError("circle: all arguments must be numeric", ast.Pos{})
		}
		cx := float32(toFloat64(args[0]))
		cy := float32(toFloat64(args[1]))
		r := float32(toFloat64(args[2]))
		// A circle is a rounded rect with radius = half-size — SDF handles it perfectly.
		drawShadowShape(cx-r, cy-r, r*2, r*2, r)
		if gfx.doFill {
			drawRoundedRectSDF(cx-r, cy-r, r*2, r*2, r, gfx.fillColor, false, 0)
		}
		if gfx.doStroke {
			drawRoundedRectSDF(cx-r, cy-r, r*2, r*2, r, gfx.strokeColor, true, gfx.strokeWidth*0.5)
		}
		return NULL
	}}

	// roundedRect — draw a rectangle with rounded corners.
	//
	// Like rect but with corners rounded to `radius` pixels, drawn with a
	// signed-distance field for crisp edges. Honours fill, stroke, and shadow.
	//
	// @sig     roundedRect(x: number, y: number, w: number, h: number, radius: number) -> null
	// @param   x       left edge in pixels
	// @param   y       top edge in pixels
	// @param   w       width in pixels
	// @param   h       height in pixels
	// @param   radius  corner radius in pixels
	// @returns null
	// @errors  TypeError unless given 5 numeric arguments
	// @example no-run roundedRect(20.0, 20.0, 120.0, 80.0, 12.0)
	// @since   0.1.0
	// @see     rect, circle, fill
	Builtins["roundedRect"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 || !allNumeric(args) {
			return typeError("roundedRect expects 5 numeric arguments: x, y, w, h, radius", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		w := float32(toFloat64(args[2]))
		h := float32(toFloat64(args[3]))
		r := float32(toFloat64(args[4]))
		drawShadowShape(x, y, w, h, r)
		if gfx.doFill {
			drawRoundedRectSDF(x, y, w, h, r, gfx.fillColor, false, 0)
		}
		if gfx.doStroke {
			drawRoundedRectSDF(x, y, w, h, r, gfx.strokeColor, true, gfx.strokeWidth*0.5)
		}
		return NULL
	}}

	// gradient — fill a rectangle with a two-colour linear gradient.
	//
	// Blends from color1 to color2 across the rectangle, either horizontally
	// (dir "h", left→right) or vertically (dir "v", top→bottom). Each colour is a
	// four-element [r, g, b, a] array of floats in 0.0–1.0.
	//
	// @sig     gradient(x: number, y: number, w: number, h: number, color1: array, color2: array, dir: string) -> null
	// @param   x       left edge in pixels
	// @param   y       top edge in pixels
	// @param   w       width in pixels
	// @param   h       height in pixels
	// @param   color1  start colour as [r, g, b, a] (0.0–1.0)
	// @param   color2  end colour as [r, g, b, a] (0.0–1.0)
	// @param   dir     "h" for horizontal or "v" for vertical
	// @returns null
	// @errors  TypeError if x/y/w/h aren't numeric, the colours aren't 4-element float arrays, or dir isn't "h"/"v"; RuntimeError unless given 7 arguments
	// @example no-run gradient(0.0, 0.0, 200.0, 100.0, [1.0, 0.0, 0.0, 1.0], [0.0, 0.0, 1.0, 1.0], "v")
	// @since   0.1.0
	// @see     fill, rect
	Builtins["gradient"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 7 {
			return typeError("gradient expects 7 arguments: x, y, w, h, color1, color2, dir", ast.Pos{})
		}
		if !allNumeric(args[:4]) {
			return typeError("gradient: x, y, w, h must be numeric", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		w := float32(toFloat64(args[2]))
		h := float32(toFloat64(args[3]))

		toColor4 := func(o Object) ([4]float32, bool) {
			arr, ok := o.(*Array)
			if !ok || len(arr.Elements) != 4 {
				return [4]float32{}, false
			}
			var c [4]float32
			for i, el := range arr.Elements {
				if !canArithmetic(el.Type()) {
					return [4]float32{}, false
				}
				c[i] = float32(toFloat64(el))
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
		dirInt := int32(0)
		if dirObj.Value == "v" {
			dirInt = 1
		}

		mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
		verts := []float32{
			x, y, 0, 0,
			x + w, y, 1, 0,
			x, y + h, 0, 1,
			x + w, y, 1, 0,
			x + w, y + h, 1, 1,
			x, y + h, 0, 1,
		}
		gl.UseProgram(gfx.gradProg)
		gl.UniformMatrix4fv(gfx.gradProjLoc, 1, false, &mvp[0])
		gl.Uniform4f(gfx.gradC1Loc, c1[0], c1[1], c1[2], c1[3])
		gl.Uniform4f(gfx.gradC2Loc, c2[0], c2[1], c2[2], c2[3])
		gl.Uniform1i(gfx.gradDirLoc, dirInt)
		gl.BindVertexArray(gfx.gradVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, gfx.gradVBO)
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
		gl.DrawArrays(gl.TRIANGLES, 0, 6)
		gl.BindVertexArray(0)
		return NULL
	}}

	// line — draw a straight line from (x1, y1) to (x2, y2).
	//
	// Uses the current stroke colour and strokeWeight.
	//
	// @sig     line(x1: number, y1: number, x2: number, y2: number) -> null
	// @param   x1  start x in pixels
	// @param   y1  start y in pixels
	// @param   x2  end x in pixels
	// @param   y2  end y in pixels
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 4 arguments
	// @example no-run line(0.0, 0.0, 200.0, 150.0)
	// @since   0.1.0
	// @see     point, stroke, strokeWeight
	Builtins["line"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("line expects 4 arguments: x1, y1, x2, y2", ast.Pos{})
		}
		if !allNumeric(args) {
			return typeError("line: all arguments must be numeric", ast.Pos{})
		}
		verts := []float32{
			float32(toFloat64(args[0])), float32(toFloat64(args[1])),
			float32(toFloat64(args[2])), float32(toFloat64(args[3])),
		}
		drawPrimitive(gl.LINES, verts, gfx.strokeColor)
		return NULL
	}}

	// ellipse — draw an ellipse centred at (x, y) with radii rx and ry.
	//
	// Approximated with 64 segments. Honours fill and stroke. For a circle, use
	// circle() (it stays perfectly crisp via a signed-distance field).
	//
	// @sig     ellipse(x: number, y: number, rx: number, ry: number) -> null
	// @param   x   centre x in pixels
	// @param   y   centre y in pixels
	// @param   rx  horizontal radius in pixels
	// @param   ry  vertical radius in pixels
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 4 arguments
	// @example no-run ellipse(150.0, 100.0, 80.0, 40.0)
	// @since   0.1.0
	// @see     circle, arc, fill
	Builtins["ellipse"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return runtimeError("ellipse expects 4 arguments: x, y, rx, ry", ast.Pos{})
		}
		if !allNumeric(args) {
			return typeError("ellipse: all arguments must be numeric", ast.Pos{})
		}
		cx := float32(toFloat64(args[0]))
		cy := float32(toFloat64(args[1]))
		rx := float32(toFloat64(args[2]))
		ry := float32(toFloat64(args[3]))

		const segments = 64
		verts := make([]float32, 0, segments*2)
		for i := 0; i < segments; i++ {
			angle := float64(i) / float64(segments) * 2 * math.Pi
			verts = append(verts, cx+rx*float32(math.Cos(angle)), cy+ry*float32(math.Sin(angle)))
		}
		if gfx.doFill {
			fanVerts := make([]float32, 0, (segments+2)*2)
			fanVerts = append(fanVerts, cx, cy)
			fanVerts = append(fanVerts, verts...)
			fanVerts = append(fanVerts, verts[0], verts[1])
			drawPrimitive(gl.TRIANGLE_FAN, fanVerts, gfx.fillColor)
		}
		if gfx.doStroke {
			drawPrimitive(gl.LINE_LOOP, verts, gfx.strokeColor)
		}
		return NULL
	}}

	// arc — draw an arc (or filled pie slice) centred at (x, y).
	//
	// Sweeps from startAngle to endAngle in radians, following screen-space
	// convention (0 = right, π/2 = down, since y points down). With fill on it
	// draws a filled sector (pie slice); with stroke on it draws just the arc line.
	//
	// @sig     arc(x: number, y: number, r: number, startAngle: number, endAngle: number) -> null
	// @param   x           centre x in pixels
	// @param   y           centre y in pixels
	// @param   r           radius in pixels
	// @param   startAngle  start angle in radians
	// @param   endAngle    end angle in radians
	// @returns null
	// @errors  TypeError unless given 5 numeric arguments
	// @example no-run arc(150.0, 100.0, 60.0, 0.0, 3.14159)
	// @since   0.1.0
	// @see     circle, ellipse, fill
	Builtins["arc"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 5 || !allNumeric(args) {
			return typeError("arc expects 5 numeric arguments: x, y, r, startAngle, endAngle", ast.Pos{})
		}
		cx := float32(toFloat64(args[0]))
		cy := float32(toFloat64(args[1]))
		r := float32(toFloat64(args[2]))
		start := toFloat64(args[3])
		end := toFloat64(args[4])
		sweep := end - start
		if sweep == 0 {
			return NULL
		}
		steps := int(math.Abs(sweep) / (2 * math.Pi) * 64)
		if steps < 3 {
			steps = 3
		}
		if steps > 128 {
			steps = 128
		}
		arcPts := make([]float32, 0, (steps+1)*2)
		for i := 0; i <= steps; i++ {
			a := start + float64(i)/float64(steps)*sweep
			arcPts = append(arcPts, cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a)))
		}
		if gfx.doFill {
			fan := make([]float32, 0, (steps+3)*2)
			fan = append(fan, cx, cy)
			fan = append(fan, arcPts...)
			drawPrimitive(gl.TRIANGLE_FAN, fan, gfx.fillColor)
		}
		if gfx.doStroke {
			drawPrimitive(gl.LINE_STRIP, arcPts, gfx.strokeColor)
		}
		return NULL
	}}

	// polygon — draw a closed polygon from a flat array of x,y pairs.
	//
	// The array is [x1, y1, x2, y2, …] — an even length, at least 3 points (6
	// values). Honours fill (triangulated from the centroid) and stroke (closed
	// outline).
	//
	// @sig     polygon(points: array) -> null
	// @param   points  flat [x1, y1, x2, y2, …] array, even length, >= 3 points
	// @returns null
	// @errors  TypeError if points isn't an array or holds a non-numeric element; RuntimeError if the length is odd or under 6
	// @example no-run polygon([100.0, 20.0, 160.0, 120.0, 40.0, 120.0])
	// @since   0.1.0
	// @see     triangle, line, fill
	Builtins["polygon"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("polygon expects 1 argument: flat array of x,y pairs", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError("polygon: argument must be an array", ast.Pos{})
		}
		if len(arr.Elements)%2 != 0 || len(arr.Elements) < 6 {
			return runtimeError("polygon: array must have an even number of elements and at least 3 points (6 values)", ast.Pos{})
		}
		verts := make([]float32, len(arr.Elements))
		for i, el := range arr.Elements {
			if !canArithmetic(el.Type()) {
				return typeError("polygon: all array elements must be numeric", ast.Pos{})
			}
			verts[i] = float32(toFloat64(el))
		}
		if gfx.doFill {
			// Centre point (2) + every vertex (len(verts)) + closing vertex (2) = +4.
			fanVerts := make([]float32, 0, len(verts)+4)
			var cx, cy float32
			for i := 0; i < len(verts); i += 2 {
				cx += verts[i]
				cy += verts[i+1]
			}
			n := float32(len(verts) / 2)
			fanVerts = append(fanVerts, cx/n, cy/n)
			fanVerts = append(fanVerts, verts...)
			fanVerts = append(fanVerts, verts[0], verts[1])
			drawPrimitive(gl.TRIANGLE_FAN, fanVerts, gfx.fillColor)
		}
		if gfx.doStroke {
			drawPrimitive(gl.LINE_LOOP, verts, gfx.strokeColor)
		}
		return NULL
	}}

	// triangle — draw a triangle through three points.
	//
	// Honours fill and stroke.
	//
	// @sig     triangle(x1: number, y1: number, x2: number, y2: number, x3: number, y3: number) -> null
	// @param   x1  first vertex x
	// @param   y1  first vertex y
	// @param   x2  second vertex x
	// @param   y2  second vertex y
	// @param   x3  third vertex x
	// @param   y3  third vertex y
	// @returns null
	// @errors  TypeError if any argument isn't numeric; RuntimeError unless given 6 arguments
	// @example no-run triangle(100.0, 20.0, 160.0, 120.0, 40.0, 120.0)
	// @since   0.1.0
	// @see     polygon, rect, fill
	Builtins["triangle"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 6 {
			return runtimeError("triangle expects 6 arguments: x1,y1, x2,y2, x3,y3", ast.Pos{})
		}
		if !allNumeric(args) {
			return typeError("triangle: all arguments must be numeric", ast.Pos{})
		}
		verts := []float32{
			float32(toFloat64(args[0])), float32(toFloat64(args[1])),
			float32(toFloat64(args[2])), float32(toFloat64(args[3])),
			float32(toFloat64(args[4])), float32(toFloat64(args[5])),
		}
		if gfx.doFill {
			drawPrimitive(gl.TRIANGLES, verts, gfx.fillColor)
		}
		if gfx.doStroke {
			drawPrimitive(gl.LINE_LOOP, verts, gfx.strokeColor)
		}
		return NULL
	}}

	// ── Image builtins ───────────────────────────────────────────────────────

	// loadImage — decode an image from a file path or encoded bytes.
	//
	// Accepts either a filesystem path string or a bytes value holding an encoded
	// image (PNG/JPEG/GIF/WebP) — the bytes form lets you draw images fetched over
	// a bridge or HTTP without touching disk. Returns an image handle for
	// drawImage / imageSize. GPU upload is deferred to the first drawImage.
	//
	// @sig     loadImage(source: string|bytes) -> image
	// @param   source  a path to an image file, or encoded image bytes
	// @returns an image handle
	// @errors  TypeError if source isn't a string or bytes; RuntimeError if the file can't be opened or the data can't be decoded
	// @example no-run logo = loadImage("logo.png")
	// @since   0.1.0
	// @see     drawImage, imageSize, saveImage
	Builtins["loadImage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("loadImage expects 1 argument: path | bytes", ast.Pos{})
		}

		// loadImage accepts either a filesystem path or encoded bytes — bytes
		// fetched over a bridge (AI image generation, HTTP downloads, format
		// decoders) skip the disk round-trip entirely. Decode path is the
		// same past source resolution.
		var src image.Image
		switch a := args[0].(type) {
		case *String:
			f, err := os.Open(a.Value)
			if err != nil {
				return runtimeError(fmt.Sprintf("loadImage: cannot open %q: %v", a.Value, err), ast.Pos{})
			}
			defer f.Close()
			decoded, _, err := image.Decode(f)
			if err != nil {
				return runtimeError(fmt.Sprintf("loadImage: cannot decode %q: %v", a.Value, err), ast.Pos{})
			}
			src = decoded
		case *Bytes:
			decoded, _, err := image.Decode(bytes.NewReader(a.Value))
			if err != nil {
				return runtimeError(fmt.Sprintf("loadImage: cannot decode bytes (%d bytes): %v", len(a.Value), err), ast.Pos{})
			}
			src = decoded
		default:
			return typeError(fmt.Sprintf("loadImage: argument must be a path string or bytes, got %s", args[0].Type()), ast.Pos{})
		}

		rgba := image.NewRGBA(src.Bounds())
		draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)

		// Store pixels; GPU upload deferred to first drawImage() call.
		return &Image{W: rgba.Bounds().Dx(), H: rgba.Bounds().Dy(), pixels: rgba.Pix}
	}}

	// imageFromRgba(bytes, width, height) → image
	//
	// Wraps raw RGBA8 pixel bytes (row-major, exactly width*height*4
	// bytes) into a kLex Image handle without going through PNG/JPEG
	// decode. The counterpart to loadImage(pngBytes) for sources that
	// already have raw pixels — Metal surface readbacks, procedural
	// generators, video frames, screen captures.
	//
	// imageFromRgba — wrap raw RGBA8 pixel bytes into an image handle.
	//
	// The counterpart to loadImage for sources that already have raw pixels —
	// surface readbacks, procedural generators, video frames, screen captures —
	// skipping PNG/JPEG decode. `bytes` must be exactly width*height*4 bytes,
	// row-major. The pixels are copied, so you may mutate the source afterwards.
	//
	// @sig     imageFromRgba(bytes: bytes, width: int, height: int) -> image
	// @param   bytes   raw RGBA8 pixels, row-major, length width*height*4
	// @param   width   image width in pixels (> 0)
	// @param   height  image height in pixels (> 0)
	// @returns an image handle
	// @errors  TypeError on wrong argument types; RuntimeError if width/height aren't positive or bytes length doesn't match width*height*4
	// @example no-run img = imageFromRgba(pixels, 64, 64)
	// @since   0.1.0
	// @see     imageToRgba, loadImage, drawImage
	Builtins["imageFromRgba"] = &Builtin{Fn: func(args []Object) Object {
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
		// Defensive copy — kLex bytes are reference-type from the
		// runtime's perspective; if the caller mutates them later, an
		// already-uploaded GPU texture wouldn't change but the
		// pre-upload CPU pixels would, which is surprising. Copying
		// closes that gap for the small one-time cost of width*height*4
		// bytes.
		pix := make([]byte, len(bs.Value))
		copy(pix, bs.Value)
		return &Image{W: w.Value, H: h.Value, pixels: pix}
	}}

	// imageToRgba — read an image's raw RGBA8 pixels back out as bytes.
	//
	// The counterpart to imageFromRgba: round-tripping
	// imageFromRgba(imageToRgba(img), w, h) reproduces the image. Returns
	// row-major width*height*4 bytes. If the image has already been drawn (its
	// pixels uploaded to the GPU), this reads them back from the texture and so
	// MUST be called from inside a draw frame.
	//
	// @sig     imageToRgba(img: image) -> bytes
	// @param   img  the image to read
	// @returns the image's raw RGBA8 pixels (width*height*4 bytes)
	// @errors  TypeError if img isn't an image; RuntimeError if it has no dimensions or no available pixel data
	// @example no-run pixels = imageToRgba(img)
	// @since   0.1.0
	// @see     imageFromRgba, imageSize, loadImage
	Builtins["imageToRgba"] = &Builtin{Fn: func(args []Object) Object {
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
			// Defensive copy so caller mutations don't leak into the
			// Image's source bytes.
			out := make([]byte, expected)
			copy(out, img.pixels)
			return &Bytes{Value: out}
		}
		if img.TextureID != 0 {
			out := make([]byte, expected)
			gl.BindTexture(gl.TEXTURE_2D, img.TextureID)
			gl.GetTexImage(gl.TEXTURE_2D, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(out))
			gl.BindTexture(gl.TEXTURE_2D, 0)
			return &Bytes{Value: out}
		}
		return runtimeError("imageToRgba: image has no pixel data — has it been drawn yet?", ast.Pos{})
	}}

	// imageSize — get an image's pixel dimensions as a (width, height) tuple.
	//
	// Cheap (struct field reads) and safe to call before the image's first draw.
	//
	// @sig     imageSize(img: image) -> (int, int)
	// @param   img  the image to measure
	// @returns a (width, height) tuple of integers
	// @errors  TypeError if img isn't an image; RuntimeError unless given 1 argument
	// @example no-run w, h = imageSize(img)
	// @since   0.1.0
	// @see     loadImage, drawImage
	Builtins["imageSize"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("imageSize expects 1 argument: img", ast.Pos{})
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

	// saveImage — encode an image to disk, format chosen by file extension.
	//
	// .png (default for unknown extensions, lossless), .jpg/.jpeg (quality 92),
	// or .gif (palette-quantised). If the image has already been drawn its pixels
	// are read back from the GPU, so this works post-render. Desktop only — the
	// browser build raises a clear "no filesystem" error.
	//
	// @sig     saveImage(img: image, path: string) -> null
	// @param   img   the image to encode
	// @param   path  the output file path; its extension selects the format
	// @returns null
	// @errors  TypeError on wrong argument types; RuntimeError if the image has no pixel data or the file can't be written/encoded
	// @example no-run saveImage(img, "out.png")
	// @since   0.1.0
	// @see     loadImage, imageToRgba
	Builtins["saveImage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return runtimeError("saveImage expects 2 arguments: img, path", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError(fmt.Sprintf("saveImage: first argument must be an image, got %s", args[0].Type()), ast.Pos{})
		}
		pathObj, ok := args[1].(*String)
		if !ok {
			return typeError(fmt.Sprintf("saveImage: path must be a string, got %s", args[1].Type()), ast.Pos{})
		}
		// Resolve pixel data. Once an Image has been drawn, drawImageGL
		// uploads its bytes to a GPU texture and sets img.pixels = nil to
		// free the CPU-side memory. That means after the first paint, the
		// only place the pixels live is the texture — read them back with
		// glGetTexImage so saveImage works post-render. We don't repopulate
		// img.pixels because subsequent paints reuse the existing texture.
		var pix []byte
		switch {
		case len(img.pixels) > 0:
			pix = img.pixels
		case img.TextureID != 0 && img.W > 0 && img.H > 0:
			pix = make([]byte, img.W*img.H*4)
			gl.BindTexture(gl.TEXTURE_2D, img.TextureID)
			gl.GetTexImage(gl.TEXTURE_2D, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pix))
			gl.BindTexture(gl.TEXTURE_2D, 0)
		default:
			return runtimeError("saveImage: image has no pixel data — has it been drawn yet?", ast.Pos{})
		}
		rgba := &image.RGBA{
			Pix:    pix,
			Stride: img.W * 4,
			Rect:   image.Rect(0, 0, img.W, img.H),
		}
		ext := strings.ToLower(filepath.Ext(pathObj.Value))
		f, err := os.Create(pathObj.Value)
		if err != nil {
			return runtimeError(fmt.Sprintf("saveImage: cannot create %q: %v", pathObj.Value, err), ast.Pos{})
		}
		defer f.Close()
		switch ext {
		case ".jpg", ".jpeg":
			if err := jpeg.Encode(f, rgba, &jpeg.Options{Quality: 92}); err != nil {
				return runtimeError(fmt.Sprintf("saveImage: JPEG encode failed: %v", err), ast.Pos{})
			}
		case ".gif":
			if err := gif.Encode(f, rgba, nil); err != nil {
				return runtimeError(fmt.Sprintf("saveImage: GIF encode failed: %v", err), ast.Pos{})
			}
		default:
			// PNG is the safe lossless default for unknown / missing extensions.
			if err := png.Encode(f, rgba); err != nil {
				return runtimeError(fmt.Sprintf("saveImage: PNG encode failed: %v", err), ast.Pos{})
			}
		}
		return NULL
	}}

	// drawImage — draw a loaded image at (x, y), optionally scaled to (w, h).
	//
	// With three args the image draws at its native size; with five it's scaled to
	// w×h. The GPU texture is created lazily on the first draw. Call inside a
	// window() draw function.
	//
	// @sig     drawImage(img: image, x: number, y: number, [w: number], [h: number]) -> null
	// @param   img  an image from loadImage or imageFromRgba
	// @param   x    left position in pixels
	// @param   y    top position in pixels
	// @param   w    draw width in pixels (default: image width)
	// @param   h    draw height in pixels (default: image height)
	// @returns null
	// @errors  TypeError if img isn't an image or the coordinates aren't numeric; RuntimeError unless given 3 or 5 arguments
	// @example no-run drawImage(logo, 20.0, 20.0)
	// @since   0.1.0
	// @see     loadImage, imageSize
	Builtins["drawImage"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 5 {
			return runtimeError("drawImage expects 3 or 5 arguments: img, x, y  or  img, x, y, w, h", ast.Pos{})
		}
		img, ok := args[0].(*Image)
		if !ok {
			return typeError("drawImage: first argument must be an image from loadImage()", ast.Pos{})
		}
		if !allNumeric(args[1:]) {
			return typeError("drawImage: x, y (and w, h) must be numeric", ast.Pos{})
		}
		x := float32(toFloat64(args[1]))
		y := float32(toFloat64(args[2]))
		w := float32(img.W)
		h := float32(img.H)
		if len(args) == 5 {
			w = float32(toFloat64(args[3]))
			h = float32(toFloat64(args[4]))
		}
		drawImageGL(img, x, y, w, h)
		return NULL
	}}

	// ── Transform builtins ───────────────────────────────────────────────────

	// translate — shift the coordinate origin by (x, y) for later draws.
	//
	// Multiplies the current transform matrix. Wrap in pushMatrix/popMatrix to
	// confine the effect to a block.
	//
	// @sig     translate(x: number, y: number) -> null
	// @param   x  horizontal shift in pixels
	// @param   y  vertical shift in pixels
	// @returns null
	// @errors  TypeError unless given 2 numeric arguments
	// @example no-run translate(100.0, 50.0)
	// @since   0.1.0
	// @see     rotate, scale, pushMatrix
	Builtins["translate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 || !allNumeric(args) {
			return typeError("translate expects 2 numeric arguments: x, y", ast.Pos{})
		}
		x := float32(toFloat64(args[0]))
		y := float32(toFloat64(args[1]))
		top := len(gfx.modelStack) - 1
		gfx.modelStack[top] = gfx.modelStack[top].Mul4(mgl32.Translate3D(x, y, 0))
		return NULL
	}}

	// rotate — rotate the coordinate system by an angle in radians.
	//
	// Rotation is about the current origin (use translate first to pivot
	// elsewhere). Wrap in pushMatrix/popMatrix to confine the effect.
	//
	// @sig     rotate(angle: number) -> null
	// @param   angle  rotation in radians (positive = clockwise, since y points down)
	// @returns null
	// @errors  TypeError unless given a single numeric argument
	// @example no-run rotate(0.7854)
	// @since   0.1.0
	// @see     translate, scale, pushMatrix
	Builtins["rotate"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 || !canArithmetic(args[0].Type()) {
			return typeError("rotate expects 1 numeric argument: angle (radians)", ast.Pos{})
		}
		angle := float32(toFloat64(args[0]))
		top := len(gfx.modelStack) - 1
		gfx.modelStack[top] = gfx.modelStack[top].Mul4(mgl32.HomogRotate3DZ(angle))
		return NULL
	}}

	// scale — scale the coordinate system by (sx, sy) for later draws.
	//
	// Values above 1 enlarge, below 1 shrink; scaling is about the current
	// origin. Wrap in pushMatrix/popMatrix to confine the effect.
	//
	// @sig     scale(sx: number, sy: number) -> null
	// @param   sx  horizontal scale factor
	// @param   sy  vertical scale factor
	// @returns null
	// @errors  TypeError unless given 2 numeric arguments
	// @example no-run scale(2.0, 2.0)
	// @since   0.1.0
	// @see     translate, rotate, pushMatrix
	Builtins["scale"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 || !allNumeric(args) {
			return typeError("scale expects 2 numeric arguments: sx, sy", ast.Pos{})
		}
		sx := float32(toFloat64(args[0]))
		sy := float32(toFloat64(args[1]))
		top := len(gfx.modelStack) - 1
		gfx.modelStack[top] = gfx.modelStack[top].Mul4(mgl32.Scale3D(sx, sy, 1))
		return NULL
	}}

	// pushMatrix — save the current transform onto the matrix stack.
	//
	// Pushes a copy of the current transform; later translate/rotate/scale calls
	// modify the copy, and popMatrix restores the saved one. Always pair with a
	// popMatrix.
	//
	// @sig     pushMatrix() -> null
	// @returns null
	// @errors  none
	// @example no-run pushMatrix()
	// @since   0.1.0
	// @see     popMatrix, translate, rotate, scale
	Builtins["pushMatrix"] = &Builtin{Fn: func(args []Object) Object {
		top := gfx.modelStack[len(gfx.modelStack)-1]
		gfx.modelStack = append(gfx.modelStack, top)
		return NULL
	}}

	// popMatrix — restore the transform saved by the matching pushMatrix.
	//
	// Discards the current transform and pops the one beneath it. Errors on an
	// empty stack (a popMatrix without a pushMatrix).
	//
	// @sig     popMatrix() -> null
	// @returns null
	// @errors  RuntimeError on stack underflow (no matching pushMatrix)
	// @example no-run popMatrix()
	// @since   0.1.0
	// @see     pushMatrix, translate, rotate, scale
	Builtins["popMatrix"] = &Builtin{Fn: func(args []Object) Object {
		if len(gfx.modelStack) <= 1 {
			return runtimeError("popMatrix: matrix stack underflow", ast.Pos{})
		}
		gfx.modelStack = gfx.modelStack[:len(gfx.modelStack)-1]
		return NULL
	}}

	// ── Text builtin ─────────────────────────────────────────────────────────

	// text — draw a string in the built-in bitmap font at (x, y).
	//
	// Uses the embedded monospace font in the current fill colour; (x, y) is the
	// top-left of the text. An optional scale multiplies the base size. For
	// proportional or custom fonts, load one with loadFont and use textFont.
	//
	// @sig     text(str: string, x: number, y: number, [scale: number]) -> null
	// @param   str    the text to draw
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   scale  size multiplier (default 1.0)
	// @returns null
	// @errors  TypeError if str isn't a string or x/y/scale aren't numeric; RuntimeError unless given 3 or 4 arguments
	// @example no-run text("hello", 20.0, 20.0)
	// @since   0.1.0
	// @see     textFont, loadFont, fill
	Builtins["text"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 3 || len(args) > 4 {
			return runtimeError("text expects 3 or 4 arguments: str, x, y [, scale]", ast.Pos{})
		}
		strObj, ok := args[0].(*String)
		if !ok {
			return typeError("text: first argument must be a string", ast.Pos{})
		}
		if !allNumeric(args[1:3]) {
			return typeError("text: x and y must be numeric", ast.Pos{})
		}
		cx := float32(toFloat64(args[1]))
		cy := float32(toFloat64(args[2]))
		scale := float32(1)
		if len(args) == 4 {
			if !canArithmetic(args[3].Type()) {
				return typeError("text: scale must be numeric", ast.Pos{})
			}
			scale = float32(toFloat64(args[3]))
		}

		mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
		gl.UseProgram(gfx.texProg)
		gl.UniformMatrix4fv(gfx.texProjLoc, 1, false, &mvp[0])
		gl.Uniform4f(gfx.texTintLoc,
			gfx.fillColor[0], gfx.fillColor[1], gfx.fillColor[2], gfx.fillColor[3])
		gl.Uniform1i(gfx.texTextModeLoc, 1) // text mode — smooth-step alpha for crisp edges
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, gfx.fontTex)
		gl.BindVertexArray(gfx.texVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, gfx.texVBO)

		charW := float32(gfx.fontCellW) * scale / float32(gfx.fontRenderScale)
		charH := float32(gfx.fontCellH) * scale / float32(gfx.fontRenderScale)
		const atlasChars = float32(96)
		verts := make([]float32, 0, len([]rune(strObj.Value))*24)
		pos := 0
		for _, ch := range strObj.Value {
			idx := int(ch) - 32
			if idx < 0 || idx >= 96 {
				idx = 0
			}
			u0 := float32(idx) / atlasChars
			u1 := float32(idx+1) / atlasChars
			qx := cx + float32(pos)*charW
			qy := cy
			verts = append(verts,
				qx, qy, u0, 0,
				qx+charW, qy, u1, 0,
				qx+charW, qy+charH, u1, 1,
				qx, qy, u0, 0,
				qx+charW, qy+charH, u1, 1,
				qx, qy+charH, u0, 1,
			)
			pos++
		}
		if len(verts) > 0 {
			gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
			gl.DrawArrays(gl.TRIANGLES, 0, int32(len(verts)/4))
		}

		gl.BindVertexArray(0)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		return NULL
	}}

	// ── Keyboard builtins ────────────────────────────────────────────────────

	// keyDown — whether a named key is currently held down.
	//
	// Level-triggered: true for every frame the key is held. Use keyPressed for a
	// one-shot edge. Key names are things like "a", "space", "left", "escape".
	//
	// @sig     keyDown(key: string) -> bool
	// @param   key  the key name to test
	// @returns true while the key is held, false otherwise
	// @errors  TypeError if key isn't a string; RuntimeError if the key name is unknown
	// @example no-run if keyDown("space") { jump() }
	// @since   0.1.0
	// @see     keyPressed
	Builtins["keyDown"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keyDown expects 1 argument: key name string", ast.Pos{})
		}
		nameObj, ok := args[0].(*String)
		if !ok {
			return typeError("keyDown: argument must be a string", ast.Pos{})
		}
		key, known := keyNames[nameObj.Value]
		if !known {
			return runtimeError(fmt.Sprintf("keyDown: unknown key %q", nameObj.Value), ast.Pos{})
		}
		if gfx.keys[key] {
			return TRUE
		}
		return FALSE
	}}

	// keyPressed — whether a named key was pressed this frame (edge-triggered).
	//
	// True only on the single frame the key transitions from up to down — ideal
	// for menu navigation or toggles, where keyDown would fire every frame.
	//
	// @sig     keyPressed(key: string) -> bool
	// @param   key  the key name to test
	// @returns true on the frame the key went down, false otherwise
	// @errors  TypeError if key isn't a string; RuntimeError if the key name is unknown
	// @example no-run if keyPressed("escape") { pause() }
	// @since   0.1.0
	// @see     keyDown
	Builtins["keyPressed"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("keyPressed expects 1 argument: key name string", ast.Pos{})
		}
		nameObj, ok := args[0].(*String)
		if !ok {
			return typeError("keyPressed: argument must be a string", ast.Pos{})
		}
		key, known := keyNames[nameObj.Value]
		if !known {
			return runtimeError(fmt.Sprintf("keyPressed: unknown key %q", nameObj.Value), ast.Pos{})
		}
		if gfx.justPressed[key] {
			return TRUE
		}
		return FALSE
	}}

	// ── Query builtins ───────────────────────────────────────────────────────

	// frameCount — the number of frames drawn so far.
	//
	// Increments once per draw loop iteration; useful for driving animation and
	// timing. The same value is passed to the window() draw callback.
	//
	// @sig     frameCount() -> int
	// @returns the count of frames rendered since the window opened
	// @errors  none
	// @example no-run if frameCount() % 60 == 0 { tick() }
	// @since   0.1.0
	// @see     frameRate, window, elapsedTime
	Builtins["frameCount"] = &Builtin{Fn: func(args []Object) Object {
		return &Integer{Value: gfx.frameCount}
	}}

	// mouseX — the mouse cursor's current x position in pixels.
	//
	// Measured from the left edge of the window. Pair with mouseY.
	//
	// @sig     mouseX() -> float
	// @returns the cursor x position in pixels
	// @errors  none
	// @example no-run circle(mouseX(), mouseY(), 10.0)
	// @since   0.1.0
	// @see     mouseY, mouseDown
	Builtins["mouseX"] = &Builtin{Fn: func(args []Object) Object {
		return &Float{Value: gfx.mouseX}
	}}

	// mouseY — the mouse cursor's current y position in pixels.
	//
	// Measured from the top edge of the window (y points down). Pair with mouseX.
	//
	// @sig     mouseY() -> float
	// @returns the cursor y position in pixels
	// @errors  none
	// @example no-run circle(mouseX(), mouseY(), 10.0)
	// @since   0.1.0
	// @see     mouseX, mouseDown
	Builtins["mouseY"] = &Builtin{Fn: func(args []Object) Object {
		return &Float{Value: gfx.mouseY}
	}}

	// mouseDown — whether the left mouse button is currently held.
	//
	// Level-triggered (true every frame the button is down). Use mouseClicked for
	// a one-shot click edge.
	//
	// @sig     mouseDown() -> bool
	// @returns true while the left button is held, false otherwise
	// @errors  none
	// @example no-run if mouseDown() { paint(mouseX(), mouseY()) }
	// @since   0.1.0
	// @see     mouseClicked, mouseX, mouseY
	Builtins["mouseDown"] = &Builtin{Fn: func(args []Object) Object {
		if gfx.mouseDown {
			return TRUE
		}
		return FALSE
	}}

	// mouseClicked — whether the left button was clicked this frame (edge-triggered).
	//
	// True only on the frame the left button goes down — use mouseDown for the
	// held state.
	//
	// @sig     mouseClicked() -> bool
	// @returns true on the frame the left button went down, false otherwise
	// @errors  none
	// @example no-run if mouseClicked() { select() }
	// @since   0.1.0
	// @see     mouseDown, mouseRightClicked
	Builtins["mouseClicked"] = &Builtin{Fn: func(args []Object) Object {
		if gfx.mouseJustClicked {
			return TRUE
		}
		return FALSE
	}}

	// mouseRightClicked — whether the right button was clicked this frame.
	//
	// Edge-triggered: true only on the frame the right button goes down. The
	// right-button counterpart to mouseClicked.
	//
	// @sig     mouseRightClicked() -> bool
	// @returns true on the frame the right button went down, false otherwise
	// @errors  none
	// @example no-run if mouseRightClicked() { showMenu() }
	// @since   0.1.0
	// @see     mouseClicked, mouseRightDown
	Builtins["mouseRightClicked"] = &Builtin{Fn: func(args []Object) Object {
		if gfx.mouseRightClicked {
			return TRUE
		}
		return FALSE
	}}

	// mouseRightDown — whether the right mouse button is currently held.
	//
	// Level-triggered (true every frame it's held). The right-button counterpart
	// to mouseDown.
	//
	// @sig     mouseRightDown() -> bool
	// @returns true while the right button is held, false otherwise
	// @errors  none
	// @example no-run if mouseRightDown() { pan() }
	// @since   0.1.0
	// @see     mouseDown, mouseRightClicked
	Builtins["mouseRightDown"] = &Builtin{Fn: func(args []Object) Object {
		if gfx.mouseRightDown {
			return TRUE
		}
		return FALSE
	}}

	// mouseScrollY — the vertical scroll-wheel delta for this frame.
	//
	// Accumulated wheel/trackpad movement since the last frame; positive is one
	// direction, negative the other, 0 when idle.
	//
	// @sig     mouseScrollY() -> float
	// @returns this frame's vertical scroll delta (0.0 when not scrolling)
	// @errors  none
	// @example no-run zoom = zoom + mouseScrollY() * 0.1
	// @since   0.1.0
	// @see     mouseScrollX, mouseX, mouseY
	Builtins["mouseScrollY"] = &Builtin{Fn: func(args []Object) Object {
		return &Float{Value: gfx.uiScrollDelta}
	}}

	// mouseScrollX — the horizontal scroll-wheel delta for this frame.
	//
	// The horizontal counterpart to mouseScrollY (trackpad sideways scroll, tilt
	// wheels). 0 when idle.
	//
	// @sig     mouseScrollX() -> float
	// @returns this frame's horizontal scroll delta (0.0 when not scrolling)
	// @errors  none
	// @example no-run offset = offset + mouseScrollX()
	// @since   0.1.0
	// @see     mouseScrollY
	Builtins["mouseScrollX"] = &Builtin{Fn: func(args []Object) Object {
		return &Float{Value: gfx.uiScrollX}
	}}

	// pushClip — restrict drawing to a rectangle, pushing it onto the clip stack.
	//
	// Subsequent draws are clipped to (x, y, w, h). If a clip is already active the
	// new rect is intersected with it, so nested clips stay within their parent.
	// Always pair with a popClip.
	//
	// @sig     pushClip(x: number, y: number, w: number, h: number) -> null
	// @param   x  left edge in pixels
	// @param   y  top edge in pixels
	// @param   w  width in pixels
	// @param   h  height in pixels
	// @returns null
	// @errors  TypeError unless given 4 numeric arguments
	// @example no-run pushClip(0.0, 0.0, 200.0, 200.0)
	// @since   0.1.0
	// @see     popClip
	Builtins["pushClip"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 4 {
			return typeError("pushClip expects 4 arguments: x, y, w, h", ast.Pos{})
		}
		getFloat := func(o Object) (float32, bool) {
			if v, ok := o.(*Integer); ok {
				return float32(v.Value), true
			}
			if v, ok := o.(*Float); ok {
				return float32(v.Value), true
			}
			return 0, false
		}
		x, ok1 := getFloat(args[0])
		y, ok2 := getFloat(args[1])
		w, ok3 := getFloat(args[2])
		h, ok4 := getFloat(args[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return typeError("pushClip: x, y, w, h must be numbers", ast.Pos{})
		}
		r := clipRect{x, y, w, h}
		if len(gfx.clipStack) > 0 {
			r = intersectClip(gfx.clipStack[len(gfx.clipStack)-1], r)
		}
		gfx.clipStack = append(gfx.clipStack, r)
		applyScissor(r)
		return NULL
	}}

	// popClip — remove the top clip rectangle, restoring the one beneath it.
	//
	// Undoes the most recent pushClip; when the stack empties, clipping is turned
	// off entirely. Safe to call on an empty stack (no-op).
	//
	// @sig     popClip() -> null
	// @returns null
	// @errors  none
	// @example no-run popClip()
	// @since   0.1.0
	// @see     pushClip
	Builtins["popClip"] = &Builtin{Fn: func(args []Object) Object {
		if len(gfx.clipStack) > 0 {
			gfx.clipStack = gfx.clipStack[:len(gfx.clipStack)-1]
		}
		if len(gfx.clipStack) > 0 {
			applyScissor(gfx.clipStack[len(gfx.clipStack)-1])
		} else {
			gl.Disable(gl.SCISSOR_TEST)
		}
		return NULL
	}}

	// winWidth — the current window width in pixels.
	//
	// @sig     winWidth() -> int
	// @returns the window width in pixels
	// @errors  none
	// @example no-run cx = winWidth() / 2
	// @since   0.1.0
	// @see     winHeight, window
	Builtins["winWidth"] = &Builtin{Fn: func(args []Object) Object {
		return &Integer{Value: gfx.winW}
	}}

	// winHeight — the current window height in pixels.
	//
	// @sig     winHeight() -> int
	// @returns the window height in pixels
	// @errors  none
	// @example no-run cy = winHeight() / 2
	// @since   0.1.0
	// @see     winWidth, window
	Builtins["winHeight"] = &Builtin{Fn: func(args []Object) Object {
		return &Integer{Value: gfx.winH}
	}}

	// fontCharWidth — the width of one character in the built-in bitmap font.
	//
	// In pixels at scale 1. Pair with fontCharHeight to lay out text drawn with
	// text() on a fixed grid.
	//
	// @sig     fontCharWidth() -> int
	// @returns the built-in font's character cell width in pixels
	// @errors  none
	// @example no-run w = fontCharWidth()
	// @since   0.1.0
	// @see     fontCharHeight, text
	Builtins["fontCharWidth"] = &Builtin{Fn: func(args []Object) Object {
		return &Integer{Value: gfx.fontCellW / gfx.fontRenderScale}
	}}

	// fontCharHeight — the height of one character in the built-in bitmap font.
	//
	// In pixels at scale 1. Pair with fontCharWidth for fixed-grid text layout.
	//
	// @sig     fontCharHeight() -> int
	// @returns the built-in font's character cell height in pixels
	// @errors  none
	// @example no-run h = fontCharHeight()
	// @since   0.1.0
	// @see     fontCharWidth, text
	Builtins["fontCharHeight"] = &Builtin{Fn: func(args []Object) Object {
		return &Integer{Value: gfx.fontCellH / gfx.fontRenderScale}
	}}

	// loadFont — load a TrueType/OpenType font and build a proportional atlas.
	//
	// Returns a font handle for textFont / textWidth. ptSize sets the rasterised
	// size (default 16); GPU upload is deferred to the first textFont call, so this
	// is safe to call before or inside window().
	//
	// @sig     loadFont(path: string, [ptSize: number]) -> Font
	// @param   path    path to a .ttf or .otf font file
	// @param   ptSize  point size to rasterise at (default 16)
	// @returns a font handle
	// @errors  TypeError if path isn't a string or ptSize isn't numeric; RuntimeError if the file can't be read or the atlas can't be built
	// @example no-run font = loadFont("Outfit.ttf", 32)
	// @since   0.1.0
	// @see     textFont, textWidth, text
	Builtins["loadFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 1 || len(args) > 2 {
			return runtimeError("loadFont expects 1-2 arguments: path [, ptSize]", ast.Pos{})
		}
		pathObj, ok := args[0].(*String)
		if !ok {
			return typeError("loadFont: path must be a string", ast.Pos{})
		}
		ptSize := 16.0
		if len(args) == 2 {
			if !canArithmetic(args[1].Type()) {
				return typeError("loadFont: ptSize must be numeric", ast.Pos{})
			}
			ptSize = toFloat64(args[1])
		}
		data, err := os.ReadFile(pathObj.Value)
		if err != nil {
			return runtimeError(fmt.Sprintf("loadFont: cannot read %q: %v", pathObj.Value, err), ast.Pos{})
		}
		fnt, err := buildProportionalFontAtlas(data, ptSize)
		if err != nil {
			return runtimeError(fmt.Sprintf("loadFont: failed to build atlas for %q: %v", pathObj.Value, err), ast.Pos{})
		}
		return fnt
	}}

	// textFont — draw a string using a font loaded with loadFont, at (x, y).
	//
	// Uses the proportional glyph atlas in the current fill colour; (x, y) is the
	// top-left. An optional scale multiplies the loaded point size. Use textWidth
	// to measure for centring or right-alignment.
	//
	// @sig     textFont(font: Font, str: string, x: number, y: number, [scale: number]) -> null
	// @param   font   a font from loadFont
	// @param   str    the text to draw
	// @param   x      left position in pixels
	// @param   y      top position in pixels
	// @param   scale  size multiplier (default 1.0)
	// @returns null
	// @errors  TypeError if font isn't a Font, str isn't a string, or x/y/scale aren't numeric; RuntimeError unless given 4 or 5 arguments
	// @example no-run textFont(font, "hello", 20.0, 20.0)
	// @since   0.1.0
	// @see     loadFont, textWidth, text
	Builtins["textFont"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 4 || len(args) > 5 {
			return runtimeError("textFont expects 4-5 arguments: font, str, x, y [, scale]", ast.Pos{})
		}
		fnt, ok1 := args[0].(*Font)
		strObj, ok2 := args[1].(*String)
		if !ok1 {
			return typeError("textFont: first argument must be a font from loadFont()", ast.Pos{})
		}
		if !ok2 {
			return typeError("textFont: second argument must be a string", ast.Pos{})
		}
		if !allNumeric(args[2:4]) {
			return typeError("textFont: x and y must be numeric", ast.Pos{})
		}
		x := float32(toFloat64(args[2]))
		y := float32(toFloat64(args[3]))
		scale := float32(1.0)
		if len(args) == 5 {
			if !canArithmetic(args[4].Type()) {
				return typeError("textFont: scale must be numeric", ast.Pos{})
			}
			scale = float32(toFloat64(args[4]))
		}

		// Deferred GPU upload on first use.
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

		lineH := fnt.LineH * scale

		mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
		gl.UseProgram(gfx.texProg)
		gl.UniformMatrix4fv(gfx.texProjLoc, 1, false, &mvp[0])
		gl.Uniform4f(gfx.texTintLoc,
			gfx.fillColor[0], gfx.fillColor[1], gfx.fillColor[2], gfx.fillColor[3])
		gl.Uniform1i(gfx.texTextModeLoc, 1)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, fnt.TextureID)
		gl.BindVertexArray(gfx.texVAO)
		gl.BindBuffer(gl.ARRAY_BUFFER, gfx.texVBO)

		verts := make([]float32, 0, len([]rune(strObj.Value))*24)
		penX := x
		for _, ch := range strObj.Value {
			g, ok := fnt.glyphs[ch]
			if !ok {
				g = fnt.fallback
			}
			qw := g.advance * scale
			verts = append(verts,
				penX, y, g.u0, 0,
				penX+qw, y, g.u1, 0,
				penX+qw, y+lineH, g.u1, 1,
				penX, y, g.u0, 0,
				penX+qw, y+lineH, g.u1, 1,
				penX, y+lineH, g.u0, 1,
			)
			penX += qw
		}
		if len(verts) > 0 {
			gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
			gl.DrawArrays(gl.TRIANGLES, 0, int32(len(verts)/4))
		}

		gl.BindVertexArray(0)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		return NULL
	}}

	// textWidth — measure how wide a string is in a given font.
	//
	// Returns the pixel width str would occupy when drawn with textFont at the
	// given scale (default 1) — the basis for centring or right-aligning text
	// before drawing it.
	//
	// @sig     textWidth(font: Font, str: string, [scale: number]) -> float
	// @param   font   a font from loadFont
	// @param   str    the text to measure
	// @param   scale  size multiplier (default 1.0)
	// @returns the rendered width in pixels
	// @errors  TypeError if font isn't a Font, str isn't a string, or scale isn't numeric; RuntimeError unless given 2 or 3 arguments
	// @example no-run x = (winWidth() - textWidth(font, label)) / 2.0
	// @since   0.1.0
	// @see     textFont, loadFont
	Builtins["textWidth"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) < 2 || len(args) > 3 {
			return runtimeError("textWidth expects 2-3 arguments: font, str [, scale]", ast.Pos{})
		}
		fnt, ok1 := args[0].(*Font)
		strObj, ok2 := args[1].(*String)
		if !ok1 {
			return typeError("textWidth: first argument must be a font from loadFont()", ast.Pos{})
		}
		if !ok2 {
			return typeError("textWidth: second argument must be a string", ast.Pos{})
		}
		scale := 1.0
		if len(args) == 3 {
			if !canArithmetic(args[2].Type()) {
				return typeError("textWidth: scale must be numeric", ast.Pos{})
			}
			scale = toFloat64(args[2])
		}
		var w float64
		for _, ch := range strObj.Value {
			if g, ok := fnt.glyphs[ch]; ok {
				w += float64(g.advance)
			} else {
				w += float64(fnt.fallback.advance)
			}
		}
		return &Float{Value: w * scale}
	}}

	// droppedFiles — the file paths dropped onto the window since the last call.
	//
	// Drag-and-drop support: returns the paths dropped onto the window and clears
	// the buffer, so each drop is reported once. Empty array when nothing was
	// dropped. Poll it once per frame.
	//
	// @sig     droppedFiles() -> array
	// @returns an array of dropped file-path strings (empty if none)
	// @errors  none
	// @example no-run for path in droppedFiles() { open(path) }
	// @since   0.1.0
	// @see     loadImage, window
	Builtins["droppedFiles"] = &Builtin{Fn: func(args []Object) Object {
		paths := gfx.droppedFiles
		gfx.droppedFiles = nil
		elems := make([]Object, len(paths))
		for i, p := range paths {
			elems[i] = &String{Value: p}
		}
		return &Array{Elements: elems}
	}}

}

// ── Internal helpers ─────────────────────────────────────────────────────────

// drawImageGL uploads img to the GPU on first use, then draws it as a
// textured quad at pixel rect (x, y, w, h). Called by drawImage() and image().
func drawImageGL(img *Image, x, y, w, h float32) {
	if img.pixels != nil {
		var texID uint32
		gl.GenTextures(1, &texID)
		gl.BindTexture(gl.TEXTURE_2D, texID)
		// Trilinear filtering with a generated mipmap chain so the image
		// stays crisp at any display size — drawing a large image into a
		// small slot (e.g. 1024×512 logo into a 234×117 header) would
		// otherwise alias badly with a plain LINEAR min filter. Mag filter
		// stays LINEAR because mipmaps only apply when down-sampling.
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
			int32(img.W), int32(img.H), 0,
			gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.pixels))
		gl.GenerateMipmap(gl.TEXTURE_2D)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		img.TextureID = texID
		img.pixels = nil
	}
	verts := []float32{
		x, y, 0, 0,
		x + w, y, 1, 0,
		x, y + h, 0, 1,
		x + w, y + h, 1, 1,
	}
	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
	gl.UseProgram(gfx.texProg)
	gl.UniformMatrix4fv(gfx.texProjLoc, 1, false, &mvp[0])
	gl.Uniform4f(gfx.texTintLoc, 1, 1, 1, 1)
	gl.Uniform1i(gfx.texTextModeLoc, 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, img.TextureID)
	gl.BindVertexArray(gfx.texVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.texVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
}

// drawRoundedRectSDF renders a rounded rectangle (or circle when r = half-size)
// using a signed distance field computed in the fragment shader.
// drawShadowShape draws an analytical Gaussian drop shadow for a rounded rect.
// Call before drawing the shape itself so the shadow renders underneath.
// pad = blur*3+2 ensures the Gaussian has fully decayed at the quad edge.
func drawShadowShape(x, y, w, h, r float32) {
	if !gfx.shadowActive || gfx.shadowProg == 0 {
		return
	}
	blur := gfx.shadowBlur
	if blur < 0.5 {
		blur = 0.5
	}
	pad := blur*3 + 2

	origHW := w * 0.5
	origHH := h * 0.5
	origCX := x + origHW
	origCY := y + origHH
	shHW := origHW + pad
	shHH := origHH + pad
	shCX := origCX + gfx.shadowOffX
	shCY := origCY + gfx.shadowOffY

	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
	verts := []float32{
		shCX - shHW, shCY - shHH, -shHW, -shHH,
		shCX + shHW, shCY - shHH, shHW, -shHH,
		shCX - shHW, shCY + shHH, -shHW, shHH,
		shCX + shHW, shCY - shHH, shHW, -shHH,
		shCX + shHW, shCY + shHH, shHW, shHH,
		shCX - shHW, shCY + shHH, -shHW, shHH,
	}
	col := gfx.shadowColor
	gl.UseProgram(gfx.shadowProg)
	gl.UniformMatrix4fv(gfx.shadowProjLoc, 1, false, &mvp[0])
	gl.Uniform2f(gfx.shadowHSLoc, origHW, origHH)
	gl.Uniform1f(gfx.shadowRadLoc, r)
	gl.Uniform1f(gfx.shadowBlurLoc, blur)
	gl.Uniform2f(gfx.shadowOffLoc, gfx.shadowOffX, gfx.shadowOffY)
	gl.Uniform4f(gfx.shadowColLoc, col[0], col[1], col[2], col[3])
	gl.BindVertexArray(gfx.shadowVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.shadowVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
}

// The quad covers the shape plus a 2px fringe for the anti-alias transition.
// mode: false = fill, true = stroke (strokeHalfW = strokeWeight / 2).
func drawRoundedRectSDF(x, y, w, h, r float32, color [4]float32, isStroke bool, strokeHalfW float32) {
	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])

	pad := r + 2.0 // cover shape + AA fringe
	cx, cy := x+w*0.5, y+h*0.5
	hw, hh := w*0.5+pad, h*0.5+pad

	// 6 vertices × 4 floats: world-x, world-y, local-x, local-y
	verts := []float32{
		cx - hw, cy - hh, -hw, -hh,
		cx + hw, cy - hh, hw, -hh,
		cx - hw, cy + hh, -hw, hh,
		cx + hw, cy - hh, hw, -hh,
		cx + hw, cy + hh, hw, hh,
		cx - hw, cy + hh, -hw, hh,
	}

	gl.UseProgram(gfx.sdfProg)
	gl.UniformMatrix4fv(gfx.sdfProjLoc, 1, false, &mvp[0])
	gl.Uniform4f(gfx.sdfColorLoc, color[0], color[1], color[2], color[3])
	gl.Uniform2f(gfx.sdfHSizeLoc, w*0.5, h*0.5)
	gl.Uniform1f(gfx.sdfRadiusLoc, r)
	if isStroke {
		gl.Uniform1i(gfx.sdfModeLoc, 1)
		gl.Uniform1f(gfx.sdfStrokeWLoc, strokeHalfW)
	} else {
		gl.Uniform1i(gfx.sdfModeLoc, 0)
		gl.Uniform1f(gfx.sdfStrokeWLoc, 0)
	}
	gl.BindVertexArray(gfx.sdfVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.sdfVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
}

func drawPrimitive(mode uint32, verts []float32, color [4]float32) {
	mvp := gfx.ortho.Mul4(gfx.modelStack[len(gfx.modelStack)-1])
	gl.UseProgram(gfx.shaderProg)
	gl.UniformMatrix4fv(gfx.projLoc, 1, false, &mvp[0])
	gl.BindVertexArray(gfx.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, gfx.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STREAM_DRAW)
	gl.Uniform4f(gfx.colorLoc, color[0], color[1], color[2], color[3])
	gl.DrawArrays(mode, 0, int32(len(verts)/2))
	gl.BindVertexArray(0)
}

func parseColor(name string, args []Object) (r, g, b, a float32, err Object) {
	switch len(args) {
	case 1:
		if !canArithmetic(args[0].Type()) {
			return 0, 0, 0, 0, typeError(fmt.Sprintf("%s: argument must be numeric", name), ast.Pos{})
		}
		v := float32(toFloat64(args[0]))
		return v, v, v, 1, nil
	case 3:
		if !allNumeric(args) {
			return 0, 0, 0, 0, typeError(fmt.Sprintf("%s: arguments must be numeric", name), ast.Pos{})
		}
		return float32(toFloat64(args[0])), float32(toFloat64(args[1])), float32(toFloat64(args[2])), 1, nil
	case 4:
		if !allNumeric(args) {
			return 0, 0, 0, 0, typeError(fmt.Sprintf("%s: arguments must be numeric", name), ast.Pos{})
		}
		return float32(toFloat64(args[0])), float32(toFloat64(args[1])), float32(toFloat64(args[2])), float32(toFloat64(args[3])), nil
	default:
		return 0, 0, 0, 0, runtimeError(fmt.Sprintf("%s expects 1, 3, or 4 arguments", name), ast.Pos{})
	}
}

func allNumeric(args []Object) bool {
	for _, a := range args {
		if !canArithmetic(a.Type()) {
			return false
		}
	}
	return true
}

// buildFontAtlas rasterises the embedded Go monospace TrueType font into a
// 96-character RGBA texture atlas (ASCII 32–127) and stores the cell
// dimensions in gfx.fontCellW / gfx.fontCellH.
// computeGlyphSDF converts a high-resolution RGBA glyph atlas into a
// display-resolution SDF atlas.  Each output pixel stores the signed distance
// to the nearest glyph edge, normalised to [0,1] (0.5 = exactly on the edge,
// >0.5 = inside, <0.5 = outside).  searchR is the search radius in high-res
// pixels; 8 at renderScale=4 gives 2 display-pixel margin on each side.
func computeGlyphSDF(src *image.RGBA, displayW, displayH, scale int) []byte {
	const searchR = 8
	hrW := src.Bounds().Dx()
	hrH := src.Bounds().Dy()
	pix := src.Pix
	sdf := make([]byte, displayW*displayH*4)

	for dy := 0; dy < displayH; dy++ {
		for dx := 0; dx < displayW; dx++ {
			hcx := dx*scale + scale/2
			hcy := dy*scale + scale/2
			if hcx >= hrW {
				hcx = hrW - 1
			}
			if hcy >= hrH {
				hcy = hrH - 1
			}

			inside := pix[(hcy*hrW+hcx)*4+3] > 127
			minDistSq := searchR*searchR + 1

			for sy := -searchR; sy <= searchR; sy++ {
				ny := hcy + sy
				if ny < 0 || ny >= hrH {
					continue
				}
				for sx := -searchR; sx <= searchR; sx++ {
					dSq := sx*sx + sy*sy
					if dSq >= minDistSq {
						continue
					}
					nx := hcx + sx
					if nx < 0 || nx >= hrW {
						continue
					}
					if (pix[(ny*hrW+nx)*4+3] > 127) != inside {
						minDistSq = dSq
					}
				}
			}

			d := float32(math.Sqrt(float64(minDistSq)))
			var v float32
			if inside {
				v = 0.5 + (d/float32(searchR))*0.5
			} else {
				v = 0.5 - (d/float32(searchR))*0.5
			}
			if v > 1.0 {
				v = 1.0
			}
			if v < 0.0 {
				v = 0.0
			}

			b := byte(v * 255.0)
			idx := (dy*displayW + dx) * 4
			sdf[idx], sdf[idx+1], sdf[idx+2], sdf[idx+3] = b, b, b, 255
		}
	}
	return sdf
}

func buildFontAtlas() uint32 {
	const chars = 96
	const ptSize = 16
	// renderScale = SDF supersample; oversample stores the atlas at 3× the
	// logical cell resolution for crisper text (storeScale=2). Logical cell
	// size (gfx.fontCellW/H) stays in renderScale units, so on-screen size is
	// unchanged; the per-cell UVs in text() are index fractions (idx/chars)
	// and need no change.
	const renderScale = 6
	const oversample = 3
	const storeScale = renderScale / oversample
	const dpi = 96 * renderScale

	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		panic("buildFontAtlas: parse: " + err.Error())
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    ptSize,
		DPI:     dpi,
		Hinting: xfont.HintingFull,
	})
	if err != nil {
		panic("buildFontAtlas: new face: " + err.Error())
	}
	defer face.Close()

	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	cellH := metrics.Height.Ceil()

	adv, ok := face.GlyphAdvance('M')
	if !ok {
		panic("buildFontAtlas: could not measure glyph advance")
	}
	cellW := adv.Ceil()
	atlasW := chars * cellW

	// Rasterise all glyphs into the high-resolution image.
	dst := image.NewRGBA(image.Rect(0, 0, atlasW, cellH))
	d := &xfont.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(color.White),
		Face: face,
	}
	for i := 0; i < chars; i++ {
		d.Dot = fixed.P(i*cellW, ascent)
		d.DrawString(string(rune(32 + i)))
	}

	// Compute display-resolution SDF from the high-res rasterisation.
	displayW := atlasW / storeScale
	displayH := cellH / storeScale
	sdfData := computeGlyphSDF(dst, displayW, displayH, storeScale)

	// Store logical cell dimensions (renderScale units); the atlas itself is
	// denser (storeScale) but text() sizes glyphs from these logical values.
	gfx.fontCellW = cellW / renderScale
	gfx.fontCellH = cellH / renderScale
	gfx.fontRenderScale = 1

	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		int32(displayW), int32(displayH), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(sdfData))
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex
}

func compileShaderProgram(vertSrc, fragSrc string) (uint32, error) {
	vert, err := compileShader(vertSrc, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}
	frag, err := compileShader(fragSrc, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}
	prog := gl.CreateProgram()
	gl.AttachShader(prog, vert)
	gl.AttachShader(prog, frag)
	gl.LinkProgram(prog)

	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(prog, logLen, nil, gl.Str(log))
		return 0, fmt.Errorf("link: %s", log)
	}
	gl.DeleteShader(vert)
	gl.DeleteShader(frag)
	return prog, nil
}

// unicodeAtlasSet is the codepoint set included in every proportional font atlas.
// Covers ASCII printable, Latin-1 Supplement (accented chars, common symbols),
// and key Unicode: dashes, smart quotes, ellipsis, euro, trademark, arrows, bullets.
func unicodeAtlasSet() []rune {
	var r []rune
	for ch := rune(32); ch <= 126; ch++ {
		r = append(r, ch)
	} // ASCII printable
	for ch := rune(160); ch <= 255; ch++ {
		r = append(r, ch)
	} // Latin-1 Supplement
	r = append(r,
		'–', // – en dash
		'—', // — em dash
		'‘', // ' left single quotation mark
		'’', // ' right single quotation mark
		'“', // " left double quotation mark
		'”', // " right double quotation mark
		'…', // … horizontal ellipsis
		'€', // € euro sign
		'™', // ™ trade mark sign
		'•', // • bullet
		'→', // → rightwards arrow
		'←', // ← leftwards arrow
		'↑', // ↑ upwards arrow
		'↓', // ↓ downwards arrow
		'✔', // ✔ heavy check mark
		'×', // × multiplication sign (also Latin-1, kept for explicitness)
	)
	return r
}

// buildProportionalFontAtlas rasterises a TrueType/OpenType font into a
// single-row SDF atlas for the Unicode codepoints returned by unicodeAtlasSet.
// Codepoints the face doesn't contain are silently skipped.
// Returns a Font with deferred GPU upload and per-glyph UV/advance data.
func buildProportionalFontAtlas(ttfData []byte, ptSize float64) (*Font, error) {
	// renderScale = SDF supersample (raster at this multiple, then distance-
	// field it down). 6 (with oversample 3 → storeScale 2) stores the atlas at
	// 3× the logical glyph resolution for crisp small text, while keeping the
	// same storeScale=2 the SDF code is tuned for. Logical metrics use
	// renderScale, so on-screen size/layout is unchanged.
	const renderScale = 6
	const dpi = 96 * renderScale
	const pad = 2 // high-res px gap between glyphs to prevent SDF bleeding

	f, err := opentype.Parse(ttfData)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    ptSize,
		DPI:     dpi,
		Hinting: xfont.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new face: %w", err)
	}
	defer face.Close()

	metrics := face.Metrics()
	hrAscent := metrics.Ascent.Ceil()
	hrLineH := metrics.Height.Ceil()

	// Measure each requested codepoint; skip those the face doesn't carry.
	type glyphSlot struct {
		r      rune
		hrAdv  int // advance + pad in high-res pixels
		hrXOff int // x offset in high-res atlas
	}
	var slots []glyphSlot
	totalHrW := 0
	for _, r := range unicodeAtlasSet() {
		adv, ok := face.GlyphAdvance(r)
		if !ok || adv == 0 {
			continue
		}
		hrAdv := adv.Ceil() + pad
		slots = append(slots, glyphSlot{r: r, hrAdv: hrAdv, hrXOff: totalHrW})
		totalHrW += hrAdv
	}

	// Rasterise all valid glyphs into the high-res atlas image.
	dst := image.NewRGBA(image.Rect(0, 0, totalHrW, hrLineH))
	d := &xfont.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(color.White),
		Face: face,
	}
	for _, s := range slots {
		d.Dot = fixed.P(s.hrXOff, hrAscent)
		d.DrawString(string(s.r))
	}

	// Store the SDF atlas at oversample× the logical glyph resolution so
	// small text carries far more distance-field detail (crisper edges)
	// WITHOUT changing on-screen size: the logical metrics below stay in
	// renderScale units; only the stored atlas + UVs use storeScale.
	// Degrade gracefully toward 1× for very large fonts so the single-row
	// atlas never exceeds a safe GL texture width.
	oversample := 3
	storeScale := renderScale / oversample
	for oversample > 1 && totalHrW/storeScale > 16384 {
		oversample--
		storeScale = renderScale / oversample
	}
	displayW := totalHrW / storeScale
	displayH := hrLineH / storeScale
	sdfData := computeGlyphSDF(dst, displayW, displayH, storeScale)

	fnt := &Font{
		LineH:    float32(hrLineH) / float32(renderScale), // logical line height
		atlasW:   int32(displayW),
		atlasHpx: int32(displayH),
		pixels:   sdfData,
		glyphs:   make(map[rune]glyphMetric, len(slots)),
	}

	atlasWf := float32(displayW)
	for _, s := range slots {
		advDisplay := float32(s.hrAdv-pad) / float32(renderScale) // logical advance (unchanged)
		// UVs live in atlas-texel space (storeScale), decoupled from the
		// logical advance so the denser atlas doesn't shift layout.
		x0 := float32(s.hrXOff) / float32(storeScale)
		x1 := x0 + float32(s.hrAdv-pad)/float32(storeScale)
		fnt.glyphs[s.r] = glyphMetric{
			u0:      x0 / atlasWf,
			u1:      x1 / atlasWf,
			advance: advDisplay,
		}
	}

	// Fallback glyph for codepoints not in atlas: use · (middle dot) else space.
	if g, ok := fnt.glyphs['·']; ok {
		fnt.fallback = g
	} else if g, ok := fnt.glyphs[' ']; ok {
		fnt.fallback = g
	}

	return fnt, nil
}

func compileShader(src string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	cSrc, free := gl.Strs(src)
	gl.ShaderSource(shader, 1, cSrc, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(log))
		return 0, fmt.Errorf("compile: %s", log)
	}
	return shader, nil
}
