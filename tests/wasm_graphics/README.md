# kLex Canvas2D graphics test

Phase 4 MVP — in-browser kLex drawing through a `<canvas>` element.
The render loop is driven by `requestAnimationFrame`; input events
(mouse + keyboard) feed Go-side state through `js.FuncOf` callbacks.

## Run

```
./serve.sh
# kitchen-sink demo : http://localhost:8766/         → click "Run"
# new-builtins proof: http://localhost:8766/proof.html → click "Run"
```

## What's covered

- Lifecycle: `window(w, h, title, drawFn)`, `winWidth`, `winHeight`, `frameCount`
- State: `background`, `fill`, `stroke`, `strokeWeight`, `noFill`, `noStroke`
- Transform stack: `pushMatrix`, `popMatrix`, `translate`, `rotate`, `scale`
- Shapes: `rect`, `roundedRect`, `circle`, `ellipse`, `line`, `triangle`, `arc`, `polygon`, `point`
- Effects: `shadow`, `noShadow`, `blendMode`, `gradient`
- Clipping: `pushClip`, `popClip`
- Text: `text(str, x, y, scale?)`, `textWidth(str, scale?)`, `loadFont`, `textFont`
- Images: `loadImage(path)`, `drawImage(img, x, y [, w, h])`, `imageSize(img)`,
  `imageFromRgba(bytes, w, h)`, `imageToRgba(img)`
- Particles: `drawParticles(xs, ys, rs, gs, bs, alphas, count, pointSize)`
- Input: `mouseX`, `mouseY`, `mouseDown`, `mouseClicked`, `keyDown`, `keyPressed`
- Drag-and-drop: `droppedFiles()`

### New-builtins proof (`proof.html` / `proof.lex`)

A focused page that proves the four builtins most recently ported from
the desktop (GL) backend to Canvas2D:

- **`gradient`** — horizontal + vertical two-colour linear fills
- **`imageToRgba` / `imageFromRgba`** — the image-fx round-trip:
  `imageToRgba(img)` → `_imgInvert` / `_imgSepia` → `imageFromRgba(...)` →
  `drawImage`. This is what makes the pure-Go filters in
  `eval/builtins_image_fx.go` usable in the browser.
- **`drawParticles`** — an animated SoA particle field (every 10th
  particle has alpha 0, proving the `alpha < 0.01` skip). Canvas2D draws
  one fill per particle — fine for hundreds/low-thousands, slower than
  the desktop GPU batch at very high counts.
- **`saveImage`** — renders the clean "no filesystem in the browser"
  `RuntimeError` on-canvas (export is desktop-only by design).

## Testing image loading

Drop any image at `tests/wasm_graphics/demo.png` (PNG/JPEG/GIF/WebP all
work — the browser decodes via `HTMLImageElement`). After `serve.sh`
copies it next to `klex.wasm`, refresh the page and the demo will
draw it rotating in the top-right corner. With no `demo.png` present,
the demo shows a placeholder outline — no crash, the error tuple is
checked via `type(img) != "ERROR"`.

`loadImage` is synchronous from the kLex script's perspective but
blocks the eval goroutine on a Go channel while the browser fetches
and decodes the image asynchronously. Same cooperative-async pattern
as worker bridges and the render loop.

## Coverage status

The Canvas2D backend now implements **every** desktop graphics builtin.
The only intentional difference is `saveImage`, which raises a clear
"no filesystem in the browser" error rather than writing a file —
in-browser image export is out of scope by design (run on desktop to
save files). See the `gradient` / `drawParticles` /
`imageFromRgba`+`imageToRgba` notes under the proof page above.

## How the render loop works

`window()` enters a Go loop that, each iteration:

1. Calls `requestAnimationFrame(rafCallback)` via `syscall/js`
2. Blocks on a Go channel (`gfxState.frameAck`)
3. Go-WASM scheduler yields → JS event loop runs → browser paints + fires input events → eventually fires the rAF callback
4. The `rafCallback` is a `js.FuncOf` that writes to `frameAck`
5. Go resumes, increments frame counter, runs `drawFn(frame)`
6. Loops

Same cooperative-async pattern as the Phase 3 worker bridge — yielding
on a channel that's fed by a JS callback lets the JS event loop run
during the wait.

`drawFn` returning `false` cleanly exits the loop. Otherwise the loop
runs until the tab is closed.

## Canvas convention

The host HTML must contain `<canvas id="klex-canvas"></canvas>` before
`klex_eval` runs. `window()` resizes that canvas and grabs its 2D
context. There is exactly one window per kLex program (matches the
GLFW backend's contract).
