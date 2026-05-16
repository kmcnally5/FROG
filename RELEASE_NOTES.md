# kLex v0.3.34 Release Notes

## Overview

v0.3.34 is a major graphics and UI milestone. The kLex OpenGL graphics system has been comprehensively upgraded — closing the gap with Dear ImGui in widget breadth, adding production-quality text editing to `textInput`, extending the drawing primitive set, and shipping a complete developer guide. The system now stands as the most capable GPU-accelerated immediate-mode UI available in any scripting language on macOS/Linux.

---

## New Features

### `splitter()` — Resizable Panel Divider

A new immediate-mode widget for drag-to-resize panel layouts:

```frog
splitX = splitter(splitX, 0, 0, winHeight(), "v", 120, winWidth() - 200)

pushClip(0, 0, splitX, winHeight())
// sidebar content
popClip()
pushClip(splitX + 1, 0, winWidth() - splitX - 1, winHeight())
// main content
popClip()
```

- `"v"` = vertical bar (drags left/right); `"h"` = horizontal bar (drags up/down)
- Pass `pos` in, reassign return value each frame — same immediate-mode pattern as `slider()`
- Cursor auto-changes to resize arrow (`HResizeCursor` / `VResizeCursor`) on hover
- Bar draws in track colour normally, accent colour on hover/drag
- Optional `thickness` parameter controls hit-area width (default 6px)

---

### `textInput` — Full Text Editing Engine

`textInput` has been completely rewritten from an append-only field to a proper text editor:

**Cursor and selection:**
- Click to position cursor; Shift+click extends selection
- Arrow keys (Left/Right), Ctrl/Cmd+Arrow (word jump), Home/End
- Shift+any navigation extends selection
- Ctrl/Cmd+A to select all
- Visual blinking caret (1Hz after 0.5s idle) in accent colour
- Selection highlighted with `accentBg` colour

**Editing:**
- Backspace deletes character before cursor (or selection)
- Delete key deletes character after cursor (or selection)
- Typing inserts at cursor position (replaces selection if active)
- Text scrolls horizontally to keep cursor visible

**Undo/redo:**
- Ctrl/Cmd+Z — undo at word-boundary granularity (up to 50 steps per field)
- Ctrl/Cmd+Shift+Z / Ctrl/Cmd+Y — redo
- Clipboard: Ctrl/Cmd+V paste, Ctrl/Cmd+C copy, Ctrl/Cmd+X cut — all selection-aware

**Unicode input:**
- Accepts all printable Unicode characters (filter relaxed from ASCII-only)
- Cursor uses rune-based indexing throughout — multi-byte UTF-8 is handled correctly
- `uiMeasurePrefix()` and `uiCharAtX()` helpers count runes, not bytes

---

### Unicode Text Rendering

The proportional font system now covers ~210 Unicode codepoints:

| Block | Range | Characters |
|---|---|---|
| ASCII printable | 32–126 | All standard characters |
| Latin-1 Supplement | 160–255 | Accented Latin, ©®£¥§²³ etc. |
| Key Unicode | ~20 hand-picked | `–` `—` `'` `'` `"` `"` `…` `€` `™` `•` `→` `←` `↑` `↓` `✔` |

Previously, any character outside ASCII was **silently dropped** — invisible in output. Now:
- Proportional fonts (`loadFont`/`textFont`) render all codepoints in the atlas
- Characters outside the atlas render as `·` (middle dot) — visible, never invisible
- Monospace `text()` renders non-ASCII as `?` — visible, never invisible
- `textInput` accepts and correctly handles non-ASCII input

Implementation: `Font.glyphs` changed from `[96]glyphMetric` (array, ASCII only) to `map[rune]glyphMetric` (keyed by codepoint, any range) + `fallback glyphMetric` field. All glyph access sites updated.

---

### `gradient()` — GPU Linear Fill

New drawing primitive for two-colour linear gradient fills:

```frog
// Dark panel header
gradient(0, 0, winWidth(), 48,
    [0.14, 0.13, 0.19, 1.0],
    [0.09, 0.09, 0.13, 1.0], "v")

// Accent button fill
gradient(btnX, btnY, btnW, btnH,
    [0.35, 0.55, 1.0, 1.0],
    [0.20, 0.40, 0.90, 1.0], "v")
```

- `color1` and `color2` are `[r, g, b, a]` float arrays — same format as `fill()`, `pieChart()` colours, and theme slots
- `"h"` = horizontal (color1 left → color2 right); `"v"` = vertical (color1 top → color2 bottom)
- Rendered by a dedicated GPU shader — single draw call, no CPU interpolation
- Can be called inside or outside `uiBegin()`/`uiEnd()`

Applied to `secretHunterUI.lex`: panel backgrounds, header, threat distribution strip, and footer now use gradients instead of flat fills.

---

### Scissor Stack — Nested Clipping

`pushClip()` / `popClip()` now maintain a proper stack with rect intersection:

```frog
pushClip(0, 0, sidebarW, winHeight())
    // sidebar content

    pushClip(8, scrollY, sidebarW - 16, listH)
        // nested clip — automatically intersected with parent
        // content can never escape the sidebar bounds
    popClip()

popClip()
```

Previously, each `pushClip` overwrote the previous rect and `popClip` disabled clipping entirely. Now:
- Each `pushClip` intersects the new rect with the active clip (if any)
- `popClip` restores the previous clip rect, or disables clipping if the stack is empty
- Frame-loop safety reset: if pushClip/popClip pairs are unbalanced, the stack is cleared at frame end
- Internal uses updated: `textInput` field clipping and `image("fill")` mode are now stack-aware

---

### `arc()` — Circular Arc Primitive

```frog
arc(x, y, r, startAngle, endAngle)
```

- With `fill` active: draws a filled **sector** (pie slice from centre to arc)
- With `stroke` active: draws the **arc line** only
- Both can be active simultaneously
- Angles in radians; convention: 0 = right, π/2 = down (screen-space y-down)
- Step count proportional to sweep angle — smooth curves at any radius

```frog
// Circular progress ring — 75% filled
fill(0.3, 0.7, 1.0, 1.0)
noStroke()
arc(cx, cy, 60.0, -1.5708, 3.1416)   // -π/2 to π

// Gauge outline ring
noFill()
stroke(0.5, 0.5, 0.6, 0.4)
strokeWeight(8.0)
arc(cx, cy, 50.0, 2.356, 0.785)
```

---

### `blendMode()` — GPU Blend Modes

```frog
blendMode("normal")    // default — standard alpha blending
blendMode("add")       // additive — fire, glow, particles, light
blendMode("multiply")  // multiply — shadows, darkening
blendMode("screen")    // screen — brightening, soft light
```

Persists until changed. Reset with `blendMode("normal")` after drawing effects:

```frog
blendMode("add")
drawParticles(xs, ys, rs, gs, bs, alphas, N, 4.0)   // glowing fire particles
blendMode("normal")
```

---

### New Math / Utility Builtins

**`remap(val, inLow, inHigh, outLow, outHigh)` → float**
Re-map a value from one range to another. Not clamped.
```frog
intensity = remap(mouseX(), 0, winWidth(), 0.0, 1.0)
barH = remap(value, 0, maxVal, 0, 200)
```
*Named `remap` (not `map`) to avoid collision with the higher-order `map(arr, fn)` builtin.*

**`constrain(val, lo, hi)` → int or float**
Clamp a value to a range. Preserves the input type.
```frog
x = constrain(x + dx, 0, winWidth())
```

**`lerp(a, b, t)` → float**
Linear interpolation. Not clamped — extrapolates outside [0,1].
```frog
camX = lerp(camX, targetX, 0.1)   // smooth camera follow
```

**`hsl(h, s, l [, a])` → `[r, g, b, a]`**
Convert HSL colour to a float array compatible with `fill()`, `gradient()`, and theme slots.
```frog
c = hsl(fmod(elapsedTime() * 0.1, 1.0), 0.8, 0.5)  // cycling rainbow
fill(c[0], c[1], c[2], c[3])
```

---

## Technical Improvements

### UI System

- `gfx.uiTextCursor`, `gfx.uiTextAnchor`, `gfx.uiTextScroll`, `gfx.uiTextBlink` maps added for per-widget textInput cursor state
- `gfx.uiUndoStacks` / `gfx.uiRedoStacks` maps added for per-widget undo history
- `gfx.cursorResizeEW` / `gfx.cursorResizeNS` cursor handles added for splitter widget
- `uiMeasurePrefix(text, n, scale)` helper — pixel width of first n **runes** of text (monospace + proportional)
- `uiCharAtX(text, px, scale)` helper — rune index closest to a pixel offset (for click-to-position)
- Splitter widget uses `spl_v_` / `spl_h_` ID prefixes for cursor shape dispatch in `uiEnd()`

### Rendering

- New gradient shader program: `gradVertexShaderSrc` + `gradFragmentShaderSrc`; `fragment mix(c1,c2,t)` with direction uniform
- New gfx fields: `gradProg`, `gradProjLoc`, `gradC1Loc`, `gradC2Loc`, `gradDirLoc`, `gradVAO`, `gradVBO`
- New clip infrastructure: `clipRect` type, `gfx.clipStack []clipRect`, `intersectClip()`, `applyScissor()` helpers
- `Font.glyphs` changed from `[96]glyphMetric` to `map[rune]glyphMetric` + `fallback glyphMetric`
- `buildProportionalFontAtlas()` now driven by `unicodeAtlasSet()` — 210 codepoints; skips glyphs the face doesn't contain; sets fallback to `·`
- `drawText()` (monospace): fixed X positioning to use rune count not byte offset; non-ASCII now renders as `?`

### Documentation and Tooling

- `syncdocs --fix-vscode` applied: VS Code syntax regex now covers **301 builtins** (up from 293)
- LSP audit: 0 missing from LSP, 0 stale in LSP
- `docs/FROG_GRAPHICS_GUIDE.MD` — new comprehensive 19-chapter developer guide covering the full graphics and UI system

---

## Files Changed

### Core Graphics (`eval/`)
| File | Change |
|---|---|
| `builtins_graphics.go` | `gradient()` + shader; `arc()`; `blendMode()`; scissor stack + helpers; `gradVertexShaderSrc` + `gradFragmentShaderSrc` consts; 7 new gfx fields (grad*); 1 new field (clipStack); frame-loop stack reset |
| `builtins_graphics_windows.go` | Added `arc`, `blendMode`, `gradient` to Windows stubs |
| `builtins_ui.go` | `splitter()` widget; full textInput rewrite (cursor/selection/undo/Unicode/scroll); `uiMeasurePrefix` rune-fix; `uiCharAtX` rune-fix; `drawTextProp` map lookup; `drawText` rune-count + `?` fallback; tooltip measurement map lookup; image fill mode stack-aware; `math` import added |
| `builtins_ui_windows.go` | Added `splitter` to Windows stubs |
| `builtins_math.go` | Added `remap()`, `constrain()`, `lerp()`, `hsl()` |
| `object.go` | `Font.glyphs [96]glyphMetric` → `map[rune]glyphMetric`; added `fallback glyphMetric` field |

### LSP and IDE (`snowball/froglsp/`, `editors/`)
| File | Change |
|---|---|
| `snowball/froglsp/builtins.go` | LSP entries for `splitter`, `gradient`, `arc`, `blendMode`, `remap`, `constrain`, `lerp`, `hsl`; updated `pushClip`/`popClip` docs; updated `textInput` docs |
| `editors/vscode_froglsp/klex-language/syntaxes/klex.tmLanguage.json` | 8 new builtins added to syntax highlight regex (301 total) |

### Documentation (`docs/`)
| File | Change |
|---|---|
| `FROG_GRAPHICS_GUIDE.MD` | New — 19-chapter comprehensive graphics and UI developer guide |
| `KLEX_GRAMMAR.MD` | Added `splitter`, `gradient`, `arc`, `blendMode`, `remap`, `constrain`, `lerp`, `hsl`; updated `pushClip`/`popClip`; updated `textInput` |
| `KLEX_LANGUAGE.TXT` | Updated `textInput`, `pushClip`/`popClip`, `splitter`, `gradient` sections |

### Examples (`tests/examples/`)
| File | Change |
|---|---|
| `splitter_demo.lex` | New — demonstrates vertical splitter with sidebar/main layout |
| `SecretHunter/secretHunterUI.lex` | Applied gradients: panel background, header, threat strip, footer |

---

## Competitive Position

The kLex graphics system now matches or exceeds every scripting language environment in its class:

| System | Widget breadth | GPU drawing | Immediate-mode | Notes |
|---|---|---|---|---|
| **kLex v0.3.34** | **27 widgets** | **Yes** | **Yes** | macOS/Linux; Windows in progress |
| Processing / p5.js | Poor | Partial | No | Great for art; near-zero UI |
| Lua / LÖVE2D | Poor | Yes | No | No built-in widgets |
| Dear ImGui (C++) | Excellent | Via host | Yes | ~75% parity with this release |
| PyQt / Qt | Excellent | Yes | No | Declarative; much heavier |

---

## What This Release Proves

kLex is now a **production-capable application platform** for data tools, security utilities, scientific visualisations, and simulation viewers — not just a scripting language. The immediate-mode UI system, SDF rendering, Unicode text, proper text editing in input fields, and GPU effects (gradients, blend modes, particles) provide everything needed to ship polished tools without reaching for a separate UI framework.

---

*Previous release: [v0.3.32](https://github.com/yourusername/klex/releases/tag/v0.3.32)*
