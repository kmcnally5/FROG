// proof.lex — focused proof of the four graphics builtins ported to the
// Canvas2D (WASM) backend: gradient, imageFromRgba/imageToRgba,
// drawParticles, and saveImage (browser clean-error).
//
// Served by proof.html in this directory. Each section is labelled on
// the canvas so a glance confirms it rendered.

// ── image round-trip + image-fx (imageToRgba → filter → imageFromRgba) ──
// Loaded once; the inverted / sepia variants are built once, not per
// frame (imageToRgba's getImageData readback is not free).
let demoImg, imgErr = safe(loadImage, "demo.png")
let haveImg = imgErr == null
let imgW = 0
let imgH = 0
let invImg = null
let sepImg = null
if haveImg {
    let w, h = imageSize(demoImg)
    imgW = w
    imgH = h
    let rgba = imageToRgba(demoImg)
    let inv, e1 = _imgInvert(rgba, imgW, imgH)
    if e1 == null { invImg = imageFromRgba(inv, imgW, imgH) }
    let sep, e2 = _imgSepia(rgba, imgW, imgH, 1.0)
    if e2 == null { sepImg = imageFromRgba(sep, imgW, imgH) }
}

// ── saveImage: prove the clean browser error (no filesystem) ────────────
let _, saveErr = safe(saveImage, demoImg, "out.png")
let saveMsg = "(unexpectedly succeeded)"
if saveErr != null { saveMsg = saveErr.message }

// ── particle field (SoA arrays). Colours/alpha are static; positions
// animate each frame. Every 10th particle is fully transparent to prove
// the alpha < 0.01 skip. ─────────────────────────────────────────────────
let N = 80
let pxs = makeArray(N, 0.0)
let pys = makeArray(N, 0.0)
let prs = makeArray(N, 0.0)
let pgs = makeArray(N, 0.0)
let pbs = makeArray(N, 0.0)
let pas = makeArray(N, 0.0)
let k = 0
while k < N {
    prs[k] = 0.3 + 0.7 * (float(k % 7) / 7.0)
    pgs[k] = 0.4 + 0.5 * (float(k % 4) / 4.0)
    pbs[k] = 1.0 - 0.7 * (float(k % 5) / 5.0)
    pas[k] = 0.7
    if k % 10 == 0 { pas[k] = 0.0 }
    k = k + 1
}

window(700, 520, "kLex Canvas2D — new builtins proof", fn(frame) {
    background(0.06, 0.08, 0.12)

    fill(1.0, 1.0, 1.0)
    text("new Canvas2D builtins — frame " + str(frame), 24.0, 16.0, 1.5)

    // ── gradient (horizontal + vertical) ───────────────────────────────
    gradient(24.0, 50.0, 300.0, 50.0, [0.2, 0.4, 1.0, 1.0], [1.0, 0.3, 0.5, 1.0], "h")
    gradient(24.0, 116.0, 300.0, 50.0, [0.1, 0.6, 0.3, 1.0], [0.95, 0.9, 0.2, 1.0], "v")
    fill(0.8, 0.8, 0.85)
    text("gradient \"h\"", 28.0, 44.0, 1.0)
    text("gradient \"v\"", 28.0, 110.0, 1.0)

    // ── image round-trip: original / inverted / sepia ──────────────────
    if haveImg {
        drawImage(demoImg, 360.0, 50.0, 90.0, 90.0)
        if invImg != null { drawImage(invImg, 460.0, 50.0, 90.0, 90.0) }
        if sepImg != null { drawImage(sepImg, 560.0, 50.0, 90.0, 90.0) }
        fill(0.8, 0.8, 0.85)
        text("original", 372.0, 150.0, 1.0)
        text("_imgInvert", 466.0, 150.0, 1.0)
        text("_imgSepia", 568.0, 150.0, 1.0)
        text("imageToRgba -> filter -> imageFromRgba", 360.0, 164.0, 1.0)
    } else {
        fill(0.6, 0.4, 0.4)
        text("demo.png not found — image round-trip skipped", 360.0, 90.0, 1.0)
    }

    // ── particles ───────────────────────────────────────────────────────
    let pcx = 350.0
    let pcy = 320.0
    let a = 0
    while a < N {
        let ang = float(a) * 0.16 + frame * 0.02
        let rad = 50.0 + float(a % 24) * 4.5
        pxs[a] = pcx + rad * cos(ang)
        pys[a] = pcy + rad * sin(ang)
        a = a + 1
    }
    drawParticles(pxs, pys, prs, pgs, pbs, pas, N, 7.0)
    fill(0.8, 0.8, 0.85)
    text("drawParticles (every 10th is alpha 0 — skipped)", 24.0, 200.0, 1.0)

    // ── saveImage clean-error proof ─────────────────────────────────────
    fill(1.0, 0.65, 0.65)
    text("saveImage in browser ->", 24.0, 480.0, 1.0)
    text(saveMsg, 24.0, 496.0, 1.0)
})
