# kLex FROG Graphics Developer Guide

> **Platform note:** The full graphics and UI system runs on macOS and Linux (OpenGL 4.1 core + GLFW). Windows support is in progress.

---

## Chapter 1 — Architecture Overview

The kLex graphics system is built in layers, each sitting cleanly on the one below it.

```
┌─────────────────────────────────────────────────────┐
│  Your .lex program                                   │
├──────────────────────────┬──────────────────────────┤
│  UI Widgets              │  Charts                  │
│  (builtins_ui.go)        │  (builtins_charts.go)    │
├──────────────────────────┴──────────────────────────┤
│  Graphics Primitives                                 │
│  (builtins_graphics.go)                              │
├─────────────────────────────────────────────────────┤
│  OpenGL 4.1 core  +  GLFW 3.3                        │
└─────────────────────────────────────────────────────┘
```

**Three shader programs** live inside the graphics layer:

| Program | Used for |
|---|---|
| Flat colour | `rect`, `line`, `triangle`, `polygon`, `point`, particles |
| Texture / SDF text | `drawImage`, `text`, `textFont` |
| SDF shapes | `circle`, `roundedRect`, `arc`, all UI widgets |

The SDF (Signed Distance Field) shaders give mathematically perfect anti-aliased edges on circles and rounded rectangles at any size — no jagged edges regardless of scale.

**8× MSAA** is enabled globally via `glfw.WindowHint(glfw.Samples, 8)`.

All drawing happens on the main OS thread. kLex enforces this with `runtime.LockOSThread()` at startup.

---

## Chapter 2 — The Window and Frame Loop

Everything starts with `window()`. This call is **blocking** — it opens the window, runs your frame callback once per display refresh, and only returns when the window is closed.

```frog
window(width, height, title, fn(frame) {
    // This runs every frame (typically 60 fps)
})
```

### The Coordinate System

Origin is **top-left**. X increases rightward, Y increases **downward**. Units are pixels.

```
(0,0) ─────────────────── (winWidth, 0)
  │                              │
  │                              │
  │                              │
(0, winHeight) ─────── (winWidth, winHeight)
```

### Frame Callback

The callback receives `frame` (an integer frame counter starting at 0). Inside it:

1. Call `background()` to clear the screen
2. Draw shapes, text, images
3. Call `uiBegin()` before any UI widgets, `uiEnd()` after the last one
4. The frame is swapped to screen automatically when the callback returns

```frog
window(1200, 800, "My App", fn(frame) {
    background(0.08, 0.08, 0.12)     // dark background

    fill(1.0, 0.5, 0.2, 1.0)
    circle(400.0, 300.0, 60.0)

    uiBegin()
    if button("Click me", 50, 50, 120, 36) {
        println("clicked!")
    }
    uiEnd()
})
```

### Window Dimensions

```frog
w = winWidth()    // → int — current window width in pixels
h = winHeight()   // → int — current window height in pixels
```

The window is resizable by default. Query these every frame to build responsive layouts.

### Frame Rate

```frog
frameRate(60)   // cap at 60 fps
frameRate(0)    // revert to vsync (default)
```

Safe to call before `window()`.

---

## Chapter 3 — Colour and Drawing State

### Setting Colours

All colour values are floats in `[0.0, 1.0]`. Four forms are accepted:

```frog
fill(gray)             // e.g. fill(0.5) — 50% grey, fully opaque
fill(r, g, b)          // e.g. fill(1.0, 0.3, 0.1)
fill(r, g, b, a)       // with alpha
stroke(r, g, b)        // outline colour
stroke(r, g, b, a)
```

### Fill and Stroke State

`fill` and `stroke` are **global state** that persists across draw calls. Enable or disable independently:

```frog
fill(0.2, 0.6, 1.0, 1.0)
noStroke()
rect(10.0, 10.0, 200.0, 100.0)   // filled, no outline

noFill()
stroke(1.0, 1.0, 1.0, 0.5)
strokeWeight(2.0)
rect(10.0, 10.0, 200.0, 100.0)   // outline only

fill(0.3, 0.8, 0.3, 1.0)
stroke(1.0, 1.0, 1.0, 1.0)
strokeWeight(1.5)
circle(100.0, 100.0, 40.0)       // both fill AND stroke
```

`fill`, `stroke`, and `strokeWeight` carry over to the **next frame** — they do not reset automatically. Always set the state you need before drawing.

### HSL Colour Helper

When you want to work in hue/saturation/lightness space:

```frog
c = hsl(0.6, 0.7, 0.5)         // → [r, g, b, 1.0] — blue at full saturation
c = hsl(0.0, 0.8, 0.5, 0.7)   // red, 70% alpha

fill(c[0], c[1], c[2], c[3])

// Cycle through rainbow
t = fmod(elapsedTime() * 0.1, 1.0)
c = hsl(t, 0.9, 0.55)
fill(c[0], c[1], c[2], 1.0)
```

`hsl()` returns a `[r, g, b, a]` array compatible with `fill()`, `gradient()`, and theme colour slots.

### Blend Modes

By default, drawing uses standard alpha blending. Change it with `blendMode()`:

```frog
blendMode("normal")    // default — src*alpha + dst*(1-alpha)
blendMode("add")       // additive — fire, glow, light, energy
blendMode("multiply")  // multiply — shadows, darkening
blendMode("screen")    // screen — brightening, soft light
```

`blendMode` persists until changed. Always reset after drawing effects:

```frog
blendMode("add")
// draw glowing particles
blendMode("normal")
// continue drawing normally
```

---

## Chapter 4 — Shape Primitives

### Basic Shapes

All shape functions accept integers or floats.

```frog
rect(x, y, w, h)                    // rectangle; origin top-left
circle(x, y, radius)                // SDF-rendered — perfect AA
ellipse(x, y, rx, ry)               // ellipse with separate radii
line(x1, y1, x2, y2)               // uses stroke colour + strokeWeight
triangle(x1, y1, x2, y2, x3, y3)   // three vertices
point(x, y)                         // single pixel; uses stroke colour + strokeWeight
polygon(points)                     // flat [x1,y1, x2,y2, ...]; min 3 points
```

### SDF Shapes

Rendered with a GPU signed distance field shader — mathematically perfect edges at any size:

```frog
roundedRect(x, y, w, h, r)         // r = corner radius
```

For a circle with a specific stroke weight, `circle()` is preferred over `roundedRect` with `r = half-size`.

### Arc

Draw a portion of a circle:

```frog
arc(x, y, r, startAngle, endAngle)
```

Angles are in **radians**. The convention is screen-space: `0` points right, `π/2` points down (y-down coordinate system).

With `fill` active: draws a **sector** (pie slice from centre to arc edge).  
With `stroke` active: draws the **arc line** only.  
Both can be active simultaneously.

```frog
// Circular progress bar — 75% filled
fill(0.3, 0.7, 1.0, 1.0)
noStroke()
arc(200.0, 200.0, 60.0, -1.5708, 3.1416)  // -π/2 to π

// Gauge ring outline
noFill()
stroke(0.4, 0.4, 0.55, 0.5)
strokeWeight(8.0)
arc(200.0, 200.0, 50.0, 2.356, 0.785)   // 135° to 45°

// Clock hand using sector
fill(1.0, 1.0, 1.0, 0.9)
noStroke()
arc(400.0, 300.0, 80.0, -1.5708, -1.5708 + 3.1416 * 0.5)
```

### Gradient Fill

Fill any rectangle with a smooth two-colour GPU gradient:

```frog
gradient(x, y, w, h, color1, color2, dir)
```

`color1` and `color2` are `[r, g, b, a]` float arrays. `dir` is `"h"` (left→right) or `"v"` (top→bottom).

```frog
// Dark panel header
gradient(0, 0, winWidth(), 48,
    [0.14, 0.13, 0.19, 1.0],
    [0.09, 0.09, 0.13, 1.0], "v")

// Horizontal accent strip
gradient(0, 48, winWidth(), 2,
    [0.54, 0.07, 0.12, 0.8],
    [0.54, 0.07, 0.12, 0.0], "h")

// Using hsl() to generate gradient endpoints
top = hsl(0.65, 0.5, 0.22)
bot = hsl(0.65, 0.5, 0.14)
gradient(panelX, 0, panelW, winHeight(), top, bot, "v")
```

`gradient()` can be called inside or outside `uiBegin()`/`uiEnd()`.

---

## Chapter 5 — Transforms

The transform system uses a **matrix stack**. Push the current state, apply transforms, draw, then pop to restore.

```frog
pushMatrix()
    translate(400.0, 300.0)   // move origin
    rotate(elapsedTime())     // rotate (radians, clockwise)
    scale(2.0, 1.5)           // scale axes
    fill(1.0, 0.5, 0.0, 1.0)
    rect(-30.0, -20.0, 60.0, 40.0)   // drawn at transformed origin
popMatrix()
// origin is restored here
```

### Transform Functions

```frog
translate(x, y)   // shift the origin
rotate(angle)     // rotate clockwise by angle radians; y-down convention
scale(sx, sy)     // scale x and y axes independently
pushMatrix()      // save current transform to stack
popMatrix()       // restore last saved transform
```

Transforms **accumulate**. A `translate` followed by a `rotate` rotates around the new origin.

### Spinning Shape Example

```frog
t = elapsedTime()
cx = float(winWidth()) * 0.5
cy = float(winHeight()) * 0.5

pushMatrix()
    translate(cx, cy)
    rotate(t * 1.5)
    fill(0.4, 0.8, 1.0, 1.0)
    noStroke()
    rect(-40.0, -40.0, 80.0, 80.0)
popMatrix()
```

---

## Chapter 6 — Clipping

Clipping restricts drawing to a rectangular region. The clip system uses a **stack** — nested clips automatically intersect, so content can never escape a parent clip region.

```frog
pushClip(x, y, w, h)   // push a clip rect onto the stack
// ... draw here; all output clipped to this rect ...
popClip()               // pop the stack; parent clip (or no clip) restored
```

Every `pushClip` must have a matching `popClip`. The frame-loop resets the stack as a safety measure in case they become unbalanced.

### Nested Clipping

```frog
pushClip(0, 0, sidebarW, winHeight())
    // draw sidebar content — clipped to sidebar bounds
    pushClip(8, scrollOffset, sidebarW - 16, listH)
        // inner clip intersects with outer — can't overflow sidebar
        // draw scrollable list items
    popClip()
popClip()
```

### Common Pattern — Scroll Content

```frog
contentH = 1200.0
scroll = scrollArea(panelX, panelY, panelW, panelH, contentH)

pushClip(panelX, panelY, panelW, panelH)
    i = 0
    while i < numItems {
        itemY = panelY + float(i) * 28.0 - scroll
        if itemY > panelY - 28.0 && itemY < panelY + panelH {
            fill(1.0, 1.0, 1.0, 0.8)
            text(items[i], panelX + 8.0, itemY, 0.9)
        }
        i = i + 1
    }
popClip()
```

---

## Chapter 7 — Text

kLex has two text systems: an embedded monospace bitmap font for quick use, and a proportional SDF font system for polished UI.

### Embedded Monospace Font

Always available, no loading required:

```frog
fill(1.0, 1.0, 1.0, 1.0)
text("Hello kLex!", 100.0, 50.0)         // scale 1 = 8px per char
text("Bigger", 100.0, 80.0, 2.5)         // scale 2.5 = 20px per char
```

Character size at scale 1 is 8×8 pixels. Use `fontCharWidth()` and `fontCharHeight()` to get exact pixel dimensions for alignment.

Non-ASCII characters render as `?` in the monospace font.

### Proportional SDF Fonts

Load any TrueType or OpenType font from disk:

```frog
// Load once, outside the frame loop
myFont = loadFont("/System/Library/Fonts/SFNS.ttf", 18)

// Draw in the frame loop
fill(0.9, 0.9, 0.95, 1.0)
textFont(myFont, "Hello world", 100, 50)        // scale 1.0 (default)
textFont(myFont, "Bigger text", 100, 80, 1.5)   // scale 1.5×
```

`loadFont()` is safe to call before `window()` — GPU upload is deferred to the first draw call.

### Measuring Text Width

Use `textWidth()` to centre or right-align text:

```frog
fn drawCentred(fnt, str, cx, y, scale) {
    w = textWidth(fnt, str, scale)
    textFont(fnt, str, cx - w / 2.0, y, scale)
}

fn drawRight(fnt, str, rightX, y, scale) {
    w = textWidth(fnt, str, scale)
    textFont(fnt, str, rightX - w, y, scale)
}
```

### Unicode

The proportional font atlas covers approximately 210 codepoints automatically:

- ASCII printable (32–126)
- Latin-1 Supplement (160–255): accented characters, ©, ®, £, ¥, §, etc.
- Key Unicode: en dash –, em dash —, curly quotes '', "", ellipsis …, € euro, ™ trademark, arrows →←↑↓, bullet •

Characters outside the atlas render as · (middle dot). The monospace font covers ASCII only.

### Using Fonts for UI Widgets

To render all UI widget text (buttons, labels, sliders, etc.) with a loaded font:

```frog
uiSetFont(myFont)    // route all widget text through this font
uiBegin()
// ... widgets now use myFont ...
uiEnd()
uiResetFont()        // revert to embedded monospace
```

---

## Chapter 8 — Images

### Loading

```frog
// Safe to call before window()
img = loadImage("assets/logo.png")   // PNG or JPEG
```

GPU texture upload happens on the first draw — the image handle is always immediately valid.

### Drawing

```frog
drawImage(img, x, y)            // natural size
drawImage(img, x, y, w, h)      // scaled to w×h
```

### Image Widget (inside UI)

For images that need to respect push/popClip and the UI layout:

```frog
uiBegin()
image(img, x, y, w, h)           // "fit" mode — letterbox + centre (default)
image(img, x, y, w, h, "fill")   // crop to fill; clips automatically
image(img, x, y, w, h, "stretch") // stretch to exact w×h
uiEnd()
```

---

## Chapter 9 — Particles

`drawParticles()` renders thousands of particles in a **single GPU draw call** using Structure-of-Arrays (SoA) layout:

```frog
drawParticles(xs, ys, rs, gs, bs, alphas, count, pointSize)
```

Each particle is a circular point sprite. Particles with `alpha < 0.01` are skipped automatically.

### Pattern — Particle System

```frog
const N = 2000

// Allocate arrays once
xs     = makeArray(N, 0.0)
ys     = makeArray(N, 0.0)
vxs    = makeArray(N, 0.0)
vys    = makeArray(N, 0.0)
rs     = makeArray(N, 1.0)
gs     = makeArray(N, 0.5)
bs     = makeArray(N, 0.2)
alphas = makeArray(N, 0.0)

// Initialise
i = 0
while i < N {
    xs[i]  = float(winWidth()) * rand()
    ys[i]  = float(winHeight()) * rand()
    vxs[i] = (rand() - 0.5) * 2.0
    vys[i] = -rand() * 3.0
    alphas[i] = rand()
    i = i + 1
}

window(1200, 800, "Particles", fn(frame) {
    background(0.05, 0.04, 0.08)

    // Update
    i = 0
    while i < N {
        xs[i] = xs[i] + vxs[i]
        ys[i] = ys[i] + vys[i]
        alphas[i] = alphas[i] - 0.005
        if alphas[i] < 0.0 {
            xs[i]     = float(winWidth()) * rand()
            ys[i]     = float(winHeight())
            vxs[i]    = (rand() - 0.5) * 2.0
            vys[i]    = -rand() * 3.0
            alphas[i] = rand() * 0.8 + 0.2
        }
        i = i + 1
    }

    // Draw with additive blend for glow effect
    blendMode("add")
    drawParticles(xs, ys, rs, gs, bs, alphas, N, 3.0)
    blendMode("normal")
})
```

---

## Chapter 10 — Data Charts

All chart builtins use the current `fill()` colour for their primary colour. They each draw their own background, axes, and border.

```frog
fill(0.3, 0.7, 1.0, 1.0)
lineChart(data, x, y, w, h)            // line + filled area (25% alpha) + dots
lineChart(data, x, y, w, h, min, max)  // explicit axis range

fill(0.9, 0.4, 0.2, 1.0)
barChart(data, x, y, w, h)             // vertical bars with ghost track
barChart(data, x, y, w, h, min, max)

fill(0.4, 0.8, 0.4, 1.0)
sparkline(data, x, y, w, h)            // minimal — just the line, no background

// Pie and donut
counts  = [crit, high, med, low]
colours = [
    [0.90, 0.18, 0.14, 1.0],
    [0.94, 0.48, 0.08, 1.0],
    [0.90, 0.76, 0.08, 1.0],
    [0.26, 0.78, 0.42, 1.0]
]
pieChart(counts, colours, cx, cy, radius)              // full pie
pieChart(counts, colours, cx, cy, radius, innerRadius) // donut; innerRadius > 0
```

Charts with zero total data are silently ignored (no crash, no draw).

---

## Chapter 11 — Utility Math

### Range Remapping

```frog
remap(val, inLow, inHigh, outLow, outHigh)  // → float, not clamped
```

Translates a value from one range to another. Does **not** clamp — use `constrain()` if you need to stay in bounds.

```frog
// Map mouse X to a colour intensity
intensity = remap(mouseX(), 0, winWidth(), 0.0, 1.0)

// Map a data value to a bar height
barH = remap(dataVal, dataMin, dataMax, 0.0, maxBarH)

// With clamping
barH = constrain(remap(dataVal, 0, 100, 0, 200), 0, 200)
```

### Constrain

```frog
constrain(val, lo, hi)  // → same type as val (int or float)
```

```frog
x = constrain(x + dx, 0, winWidth())
alpha = constrain(alpha - 0.02, 0.0, 1.0)
```

### Linear Interpolation

```frog
lerp(a, b, t)  // → float; a at t=0, b at t=1; not clamped
```

```frog
// Smooth camera follow — moves 10% toward target each frame
camX = lerp(camX, targetX, 0.1)
camY = lerp(camY, targetY, 0.1)

// Fade between two values
alpha = lerp(0.0, 1.0, progress)

// Animate colour
r = lerp(0.2, 0.9, sin(elapsedTime()) * 0.5 + 0.5)
```

### Standard Math

```frog
sin(x)   cos(x)   tan(x)   atan2(y, x)   asin(x)   acos(x)   atan(x)
sqrt(x)  pow(x, y)  abs(x)  log(x)  exp(x)
floor(x) ceil(x)  round(x)
min(a, b)  max(a, b)
mod(a, b)       // integer modulo
fmod(a, b)      // floating-point modulo (both args must be float)
pi()            // 3.14159...
rand()          // random float in [0, 1)
```

---

## Chapter 12 — The Immediate-Mode UI System

### What Immediate-Mode Means

In kLex's UI system there are no widget objects, no event listeners, and no retained state. Every frame, you describe the UI by calling functions. Each function both **draws** the widget and **returns its updated state**.

```frog
// Classic retained-mode approach (NOT how kLex works):
// btn = Button.new("Click me")
// btn.onClick(fn() { ... })

// kLex immediate-mode (how it actually works):
if button("Click me", x, y, w, h) {
    // this block runs on the frame the button is clicked
}
```

The key insight: **call the widget each frame, use the return value**. The UI system keeps minimal internal state (hover, focus, scroll positions) in Go — your kLex code owns everything else.

### The Frame Contract

```frog
window(w, h, title, fn(frame) {
    background(...)

    // Optional: draw custom graphics before UI
    gradient(0, 0, w, 48, darkTop, darkBot, "v")

    uiBegin()        // ← REQUIRED before first widget
    uiSetFont(font)  // optional: use proportional font for all widgets

    // ... widget calls ...

    uiEnd()          // ← REQUIRED after last widget
})
```

`uiBegin()` resets the widget ID counter and hover registry. `uiEnd()` renders deferred overlays (open dropdowns, tooltips, toasts) and updates cursor shapes. **Forgetting either causes visual bugs.**

### Widget IDs

Every widget gets an ID generated from a per-frame counter (`txt_0`, `btn_1`, `sld_2`, etc.). This means widgets must be called in the **same order every frame**. Conditional widgets are fine — just ensure the conditions are stable:

```frog
// Fine — the order is consistent every frame
if showPanel {
    label("Title", x, y)          // lbl_N
    name = textInput("", name, x, y+24, w, 30)  // txt_N+1
}
button("OK", x, y+60, 80, 30)    // btn_N+2 (or btn_0 when showPanel=false)
```

---

## Chapter 13 — Theming

The theme system controls the visual appearance of every widget. kLex ships with five built-in themes.

### Applying a Theme

```frog
import "stdlib/ui_themes.lex" as themes

theme = themes.crimson()   // dark navy + red accent (recommended)
// theme = themes.dark()
// theme = themes.midnight()
// theme = themes.forest()
// theme = themes.light()

themeApplied = false

window(1200, 800, "My App", fn(frame) {
    if !themeApplied {
        themes.applyTheme(theme)   // must be called inside window(), after first frame
        themeApplied = true
    }
    background(theme["bg"][0], theme["bg"][1], theme["bg"][2])
    // ...
})
```

### Custom Themes

```frog
t = makeTheme()         // returns default 14-slot palette
t[6]  = [0.2, 0.7, 1.0, 1.0]   // slot 6 = accent colour
t[7]  = [0.1, 0.3, 0.5, 1.0]   // slot 7 = accent background
uiTheme(t)
```

**Slot reference:**

| Slot | Name | Controls |
|---|---|---|
| 0 | widgetBg | button / widget normal background |
| 1 | widgetBgHover | hover state |
| 2 | widgetBgActive | pressed / checked state |
| 3 | widgetText | text on interactive widgets |
| 4 | labelText | label text |
| 5 | dimText | secondary / unselected text |
| 6 | accent | accent line / stripe |
| 7 | accentBg | selected item / active tab background |
| 8 | track | slider / progress track |
| 9 | trackFill | slider / progress fill |
| 10 | handle | slider handle, scrollbar thumb, checkmark |
| 11 | inputBg | text input / list container background |
| 12 | inputFocusBg | focused text input background |
| 13 | shadow | drop shadow for floating elements |

---

## Chapter 14 — Core Widgets

All widget coordinate arguments (`x`, `y`, `w`, `h`) must be **integers**, not floats. Use `float()` only when passing to drawing functions that require floats.

### Button

```frog
if button(label, x, y, w, h) { ... }
if button("Save", x, y, w, h, 0.7) { ... }  // optional size scale (default 0.5)
```

Returns `true` on the single frame it is clicked.

### Label

```frog
label(text, x, y)
label(text, x, y, 0.65)   // optional size scale
```

No return value. Draws text in the theme's `labelText` colour.

### Text Input

Full cursor and selection support. Pass your buffer in, reassign the return value each frame:

```frog
buf = textInput("", buf, x, y, w, h)          // no label
buf = textInput("Name:", buf, x, y, w, h)     // label drawn 80px left of field
buf = textInput("", buf, x, y, w, h, 0.65)   // with size scale
```

**Keyboard shortcuts while focused:**

| Shortcut | Action |
|---|---|
| Arrow keys | Move cursor |
| Ctrl/Cmd + Arrow | Jump by word |
| Home / End | Jump to start / end |
| Shift + any navigation | Extend selection |
| Ctrl/Cmd + A | Select all |
| Backspace / Delete | Delete before / after cursor (or selection) |
| Ctrl/Cmd + Z | Undo (word-boundary granularity, up to 50 steps) |
| Ctrl/Cmd + Shift+Z or Y | Redo |
| Ctrl/Cmd + V / C / X | Paste / Copy / Cut |

Text longer than the field scrolls horizontally to track the cursor.

### Checkbox and Toggle

```frog
checked = checkbox("Enable shadows", x, y, checked)         // tick box
toggled = toggle("Dark mode", x, y, toggled)                // pill switch
```

Both return the new state.

### Radio Group

Chain radio calls, passing the group value through each one:

```frog
group = radio("Option A", x, y, "A", group)
group = radio("Option B", x, y+28, "B", group)
group = radio("Option C", x, y+56, "C", group)
```

Returns the value of the clicked option, or the unchanged group value.

### Slider

```frog
val = slider("Volume", x, y, w, val, 0.0, 100.0)
val = slider("", x, y, w, val, 0.0, 1.0, 0.65)   // no label, custom size
```

Returns the updated float value clamped to `[min, max]`.

### Numeric Stepper

```frog
n = numericStepper("Threads", x, y, w, n, 1, 32)
```

`[−] N [+]` widget for discrete integers. Better than a slider for small integer ranges.

### Progress Bar

Display-only:

```frog
progressBar(x, y, w, h, value, max)
```

### Tooltip

Attach a tooltip to the widget drawn immediately before this call. Appears after 0.5 seconds of hover:

```frog
slider("", x, y, w, val, 0.0, 1.0)
tooltip("Adjust master volume")

checkbox("Include git history", x, y, on)
tooltip("Also scans every commit for leaked credentials")
```

---

## Chapter 15 — Selection Widgets

### Dropdown

```frog
items = ["Alpha", "Beta", "Gamma"]
selected = dropdown("Filter:", items, x, y, w)       // returns selected string
selected = dropdown("", items, x, y, w, 0.6)         // no label, custom size
```

**Call dropdowns last** in your widget block (before `uiEnd()`) so the open popup renders on top of everything else.

### List

Scrollable single-selection list:

```frog
selected = list("", items, x, y, w, h)            // returns selected item string
selected = list("Items", items, x, y, w, h, 0.6)
```

### Multi-Select List

Pass a bool array in, get it back with toggles applied:

```frog
selected = makeArray(len(items), false)   // initialise once

// In draw loop:
selected = listMulti("", items, selected, x, y, w, h)

// Read selections:
i = 0
while i < len(selected) {
    if selected[i] { doSomething(items[i]) }
    i = i + 1
}
```

### Tabs

```frog
tabLabels = ["Overview", "Settings", "Logs"]
activeTab = tabs(x, y, w, tabLabels, activeTab)

if activeTab == 0 { /* overview content */ }
if activeTab == 1 { /* settings content */ }
if activeTab == 2 { /* logs content */ }
```

`tabs()` is a selector — it returns the active index. You draw the content yourself with `if` blocks.

### Scroll Area

Manages vertical scroll state. Use with `pushClip`/`popClip` to clip scrollable content:

```frog
contentH = float(numItems) * 28.0
scrollOffset = scrollArea(x, y, w, h, contentH)

pushClip(x, y, w, h)
i = 0
while i < numItems {
    itemY = float(y) + float(i) * 28.0 - scrollOffset
    label(items[i], x + 8, int(itemY))
    i = i + 1
}
popClip()
```

### Splitter

Resizable divider between two panels. Pass `pos` in, reassign each frame:

```frog
splitX = 240  // initial position

// In draw loop:
splitX = splitter(splitX, 0, 0, winHeight(), "v", 120, winWidth() - 200)

// Left panel
pushClip(0, 0, splitX, winHeight())
// ... sidebar content ...
popClip()

// Right panel
mainX = splitX + 1
pushClip(mainX, 0, winWidth() - mainX, winHeight())
// ... main content ...
popClip()
```

`orient`: `"v"` = vertical bar (drags horizontally), `"h"` = horizontal bar (drags vertically).

---

## Chapter 16 — Layout Cursors

Layout cursors eliminate manual x/y arithmetic. Two are provided: a horizontal row cursor and a vertical column cursor.

### Row Cursor (horizontal layout)

```frog
uiBeginRow(x, y, h, gap)    // initialise; h = item height, gap = spacing
// Place each widget at:
widget(..., uiRowX(), uiRowY(), widgetW, uiRowH())
uiRowAdvance(widgetW)        // move cursor right by widgetW + gap
```

### Column Cursor (vertical layout)

```frog
uiBeginCol(x, y, w, gap)    // initialise; w = column width, gap = spacing
// Place each widget at:
widget(..., uiColX(), uiColY(), uiColW(), widgetH)
uiColAdvance(widgetH)        // move cursor down by widgetH + gap
```

**Critical:** The accessor functions (`uiColX()`, `uiRowY()`, etc.) return **integers**. This is correct for widget coordinates — do not convert to float before passing to widgets like `button`, `textInput`, `slider`.

### Column Layout Example

```frog
uiBeginCol(16, 80, 260, 8)

label("SCAN PATH", uiColX(), uiColY())                       uiColAdvance(18)
scanPath = textInput("", scanPath, uiColX(), uiColY(), uiColW(), 30)  uiColAdvance(38)
includeGit = checkbox("Include git history", uiColX(), uiColY(), includeGit) uiColAdvance(30)
```

### Composing Row Inside Column

```frog
uiBeginCol(x, y, w, gap)
// ... column widgets ...
uiBeginRow(uiColX(), uiColY(), 30, 8)   // start a row at the cursor position
btn1 = button("Cancel", uiRowX(), uiRowY(), 80, uiRowH())  uiRowAdvance(80)
btn2 = button("OK",     uiRowX(), uiRowY(), 80, uiRowH())  uiRowAdvance(80)
uiColAdvance(38)   // advance column past the row
```

---

## Chapter 17 — Advanced Widgets

### Table

```frog
headers = ["Name", "Size", "Date"]
rows = [
    ["file_a.txt", "12 KB", "2026-05-01"],
    ["file_b.go",  "4 KB",  "2026-05-10"]
]
selectedRow = table(headers, rows, x, y, w, h)
selectedRow = table(headers, rows, x, y, w, h, 0.55)  // with size
```

Returns the index of the selected row (-1 = none). Scrollable.

### Tree View

```frog
labels   = ["src/", "main.go", "eval.go", "parser/", "parser.go"]
levels   = [0, 1, 1, 0, 1]          // 0 = root, 1 = child
expanded = makeArray(len(labels), false)

result   = treeView(x, y, w, h, labels, levels, expanded)
selectedIdx = result[0]
expanded    = result[1]   // reassign — treeView owns expansion state
```

### Accordion

```frog
sections = ["Configuration", "Advanced", "Diagnostics"]
openIdx  = accordion(x, y, w, sections, openIdx)

if openIdx == 0 { /* configuration content */ }
if openIdx == 1 { /* advanced content */ }
```

Returns the open section index (-1 = all closed).

### Context Menu

```frog
menuVisible = false
selectedItem = -1

// In draw loop:
if mouseRightClicked() {
    menuVisible = true
}
result = contextMenu(int(mouseX()), int(mouseY()), ["Open", "Rename", "Delete"], menuVisible)
if result >= 0 {
    // result is the index of the clicked item
    menuVisible = false
}
if result == -2 {
    // clicked outside — dismiss
    menuVisible = false
}
```

**Call context menus after all other widgets** so the popup renders on top.

### Color Picker

```frog
result = colorPicker(x, y, w, r, g, b, a)
r = result[0]
g = result[1]
b = result[2]
a = result[3]
```

Four RGBA drag sliders with a live preview swatch. Values in `[0.0, 1.0]`.

### Modal Dialog

```frog
if showConfirm {
    result = modal("Confirm Delete", "This cannot be undone.", ["Cancel", "Delete"])
    if result != "" {
        showConfirm = false
        if result == "Delete" { performDelete() }
    }
}
```

Renders a full-screen dimmed overlay. **Call after all other widgets** (just before `uiEnd()`) so it renders on top.

### Toast Notifications

```frog
toast("Scan complete!")                         // info, 3 seconds
toast("Saved successfully", "success")          // green
toast("5 warnings found", "warn", 6.0)          // orange, 6 seconds
toast("Critical error: access denied", "error") // red
```

Toasts appear in the bottom-right corner and fade out automatically.

---

## Chapter 18 — Input Handling

All input values are available anywhere inside the frame callback.

### Mouse

```frog
mouseX()             // → float — cursor X in pixels
mouseY()             // → float — cursor Y in pixels
mouseDown()          // → bool — left button currently held
mouseClicked()       // → bool — left button just clicked this frame (one-shot)
mouseRightDown()     // → bool — right button held
mouseRightClicked()  // → bool — right button just clicked (one-shot)
mouseScrollY()       // → float — vertical scroll delta this frame (resets to 0 each frame)
mouseScrollX()       // → float — horizontal scroll delta (trackpad)
```

### Keyboard

```frog
keyDown("SHIFT")     // → bool — true while key is physically held
keyPressed("SPACE")  // → bool — true only on the one frame it was first pressed
```

**Key name strings:** `"A"`–`"Z"`, `"0"`–`"9"`, `"SPACE"`, `"ENTER"`, `"ESC"`, `"LEFT"`, `"RIGHT"`, `"UP"`, `"DOWN"`, `"BACKSPACE"`, `"SHIFT"`, `"CTRL"`, `"TAB"`, `"CMD"` / `"SUPER"`, `"F1"`–`"F12"`, `"DELETE"`, `"INSERT"`, `"HOME"`, `"END"`, `"PGUP"`, `"PGDOWN"`.

For modifier combos, check `keyDown` for the modifier and `keyPressed` for the key:

```frog
if keyDown("CTRL") && keyPressed("S") { save() }
if keyDown("CMD")  && keyPressed("Z") { undo() }
```

### File Drop

```frog
dropped = droppedFiles()   // → array of path strings (cleared after each call)
if len(dropped) > 0 {
    scanPath = dropped[0]
    toast("Path loaded: " + scanPath, "success")
}
```

`droppedFiles()` fires when the user releases dragged files over the window. There is no drag-hover event.

### Timing

```frog
elapsedTime()   // → float — seconds since window opened; use for animation
frameCount()    // → int — frame counter starting at 0
```

Prefer `elapsedTime()` for physics and animation — it is frame-rate independent.

---

## Chapter 19 — Patterns and Best Practices

### Responsive Layouts

Compute all positions from `winWidth()` and `winHeight()` inside the frame callback. Never hardcode positions against a fixed window size.

```frog
window(1100, 800, "Dashboard", fn(frame) {
    ww = winWidth()
    wh = winHeight()

    sideW   = 280
    mainX   = sideW + 2
    mainW   = ww - mainX
    hdrH    = 48
    footerH = 40
    contentH = wh - hdrH - footerH

    // Header
    gradient(0, 0, ww, hdrH, hdrTop, hdrBot, "v")

    // Sidebar
    gradient(0, hdrH, sideW, wh - hdrH, sideTop, sideBot, "v")

    // Main area
    pushClip(mainX, hdrH, mainW, contentH)
    // ... content ...
    popClip()
})
```

### Smooth Animation

Use `elapsedTime()` not `frameCount()`. Multiply by a speed constant:

```frog
t = elapsedTime()
x = 600.0 + sin(t * 1.5) * 200.0
y = 400.0 + cos(t * 1.0) * 100.0
```

For spring/smooth-follow effects, use `lerp()` in the frame loop:

```frog
// Converges 10% per frame toward target — feels springy
displayX = lerp(displayX, targetX, 0.12)
displayY = lerp(displayY, targetY, 0.12)
```

### Handling UI State

Keep all mutable state as module-level variables, updated by widget return values:

```frog
// State declared once at module level
filterText = ""
sortDesc   = true
selectedIdx = -1

window(w, h, title, fn(frame) {
    uiBegin()
    filterText = textInput("", filterText, 10, 10, 300, 30)
    sortDesc   = checkbox("Sort descending", 320, 15, sortDesc)
    // ...
    uiEnd()
})
```

### Loading Resources

Load fonts, images, and other heavy resources **before** `window()`. GPU upload is deferred automatically:

```frog
font  = loadFont("/System/Library/Fonts/SFNS.ttf", 18)
logo  = loadImage("assets/logo.png")
icons = loadImage("assets/icons.png")

window(1200, 800, "App", fn(frame) {
    // font and images are ready on frame 0
    drawImage(logo, 10.0, 10.0, 200.0, 60.0)
    uiSetFont(font)
    uiBegin()
    // ...
    uiEnd()
})
```

### Drawing Custom Backgrounds and Panels

Gradients + pushClip make it easy to build polished panel structures:

```frog
fn drawPanel(x, y, w, h, topCol, botCol) {
    gradient(float(x), float(y), float(w), float(h), topCol, botCol, "v")
}

fn drawSeparator(x, y, w) {
    fill(1.0, 1.0, 1.0, 0.06)
    noStroke()
    rect(float(x), float(y), float(w), 1.0)
}
```

### textArea

Multiline editable text. Scroll is built in:

```frog
content = textArea("", content, x, y, w, h)
content = textArea("Notes:", content, x, y, w, h, 0.6)  // with label + size
```

Supports Enter for newlines, Ctrl/Cmd+V paste (preserving newlines), mouse wheel scroll, and Ctrl/Cmd+C copy.

---

## Appendix A — Quick Reference

### Drawing State
```
fill(r,g,b[,a])   noFill()
stroke(r,g,b[,a]) noStroke()   strokeWeight(w)
blendMode("normal"|"add"|"multiply"|"screen")
```

### Primitives
```
background(r,g,b[,a])
rect(x,y,w,h)             roundedRect(x,y,w,h,r)
circle(x,y,r)             ellipse(x,y,rx,ry)
line(x1,y1,x2,y2)         triangle(x1,y1,x2,y2,x3,y3)
polygon(points)            point(x,y)
arc(x,y,r,start,end)
gradient(x,y,w,h,c1,c2,dir)
drawParticles(xs,ys,rs,gs,bs,as,n,size)
```

### Transforms
```
translate(x,y)   rotate(angle)   scale(sx,sy)
pushMatrix()     popMatrix()
pushClip(x,y,w,h)   popClip()
```

### Text
```
text(str,x,y[,scale])
loadFont(path[,ptSize]) → Font
textFont(font,str,x,y[,scale])
textWidth(font,str[,scale]) → float
```

### Images
```
loadImage(path) → image
drawImage(img,x,y[,w,h])
```

### Input
```
mouseX() mouseY() mouseDown() mouseClicked()
mouseRightDown() mouseRightClicked()
mouseScrollX() mouseScrollY()
keyDown(name)   keyPressed(name)
droppedFiles() → array
elapsedTime() → float   frameCount() → int
winWidth() → int   winHeight() → int
```

### Math Utilities
```
remap(v,il,ih,ol,oh)   constrain(v,lo,hi)   lerp(a,b,t)
hsl(h,s,l[,a]) → [r,g,b,a]
sin cos tan sqrt abs floor ceil round pow log exp min max
fmod(a,b)   mod(a,b)   rand()   pi()
```

### UI Frame Contract
```
uiBegin()          // reset hover registry — REQUIRED
uiSetFont(font)    // optional
// ... widget calls ...
uiEnd()            // render deferred overlays — REQUIRED
```

### Core Widgets
```
button(label,x,y,w,h[,size]) → bool
label(text,x,y[,size])
textInput(label,text,x,y,w,h[,size]) → string
textArea(label,text,x,y,w,h[,size]) → string
checkbox(label,x,y,checked[,size]) → bool
toggle(label,x,y,on[,size]) → bool
radio(label,x,y,value,groupValue[,size]) → string
slider(label,x,y,w,value,min,max[,size]) → float
numericStepper(label,x,y,w,value,min,max[,size]) → int
progressBar(x,y,w,h,value,max)
tooltip(text)
toast(message[,style][,duration])
```

### Selection Widgets
```
dropdown(label,items,x,y,w[,size]) → string
list(label,items,x,y,w,h[,size]) → string
listMulti(label,items,selected,x,y,w,h[,size]) → array
tabs(x,y,w,items,activeIdx[,size]) → int
scrollArea(x,y,w,h,contentH) → float
splitter(pos,x,y,length,orient,min,max[,thickness]) → int
```

### Advanced Widgets
```
table(headers,rows,x,y,w,h[,size]) → int
treeView(x,y,w,h,labels,levels,expanded[,size]) → array
accordion(x,y,w,sections,openIdx[,size]) → int
contextMenu(x,y,items,visible[,size]) → int
colorPicker(x,y,w,r,g,b,a) → array
modal(title,message,buttons) → string
image(img,x,y,w,h[,mode])
```

### Layout Cursors
```
uiBeginRow(x,y,h,gap)
uiRowX() uiRowY() uiRowH()   uiRowAdvance(w)

uiBeginCol(x,y,w,gap)
uiColX() uiColY() uiColW()   uiColAdvance(h)
```

### Charts
```
lineChart(data,x,y,w,h[,min,max])
barChart(data,x,y,w,h[,min,max])
sparkline(data,x,y,w,h)
pieChart(data,colors,cx,cy,radius[,innerRadius])
```

### Theme
```
makeTheme() → array        uiTheme(palette)
uiSetFont(font)            uiResetFont()
```

### Focus Management
```
uiNextFieldID() → string
uiGetFocus() → string
uiSetFocus(id)
```
