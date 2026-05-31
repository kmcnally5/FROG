# kLex Canvas2D graphics test

Phase 4 MVP — in-browser kLex drawing through a `<canvas>` element.
The render loop is driven by `requestAnimationFrame`; input events
(mouse + keyboard) feed Go-side state through `js.FuncOf` callbacks.

## Run

```
./serve.sh
# open http://localhost:8766/  → click "Run"
```

## What's covered

- Lifecycle: `window(w, h, title, drawFn)`, `winWidth`, `winHeight`, `frameCount`
- State: `background`, `fill`, `stroke`, `strokeWeight`, `noFill`, `noStroke`
- Transform stack: `pushMatrix`, `popMatrix`, `translate`, `rotate`, `scale`
- Shapes: `rect`, `roundedRect`, `circle`, `ellipse`, `line`, `triangle`, `arc`, `polygon`, `point`
- Effects: `shadow`, `noShadow`, `blendMode`
- Clipping: `pushClip`, `popClip`
- Text: `text(str, x, y, scale?)`, `textWidth(str, scale?)`
- Images: `loadImage(path)`, `drawImage(img, x, y [, w, h])`, `imageSize(img)`
- Input: `mouseX`, `mouseY`, `mouseDown`, `mouseClicked`, `keyDown`, `keyPressed`

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

## What's deferred (follow-up sessions)

- `loadFont`, `textFont`, `textWidth` (SDF font path — needs Canvas2D
  metric calls or a separate text-rendering strategy)
- `loadImage`, `drawImage`, `saveImage`, `imageFromRgba`, `imageToRgba`
  (no FS in browser; needs OPFS or fetch shim)
- `drawParticles`, `gradient` (GPU-specific; would need WebGL backend)
- `shadow`, `noShadow` (Canvas2D has shadow* properties — doable, just not in MVP)
- `pushClip`, `popClip` (`ctx.save/clip/restore` — doable, not in MVP)
- `arc`, `polygon`, `point` (mechanical; not in MVP)
- `droppedFiles` (drag-and-drop events on canvas)
- `blendMode` (subset of Canvas2D's `globalCompositeOperation`)

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
