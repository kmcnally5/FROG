// Canvas2D demo — exercises the in-browser graphics builtins.
//
// MVP coverage: window/rAF loop, background, fill/stroke state,
// shape primitives, transforms, text, mouse + keyboard input.
// Expansion: arc, polygon, point, shadow/noShadow, pushClip/popClip,
// blendMode, textWidth.

let x = 80.0
let dx = 3.5

let cy = 50.0
let dcy = 2.0

// Pre-compute star polygon points (5-pointed) around centre (560, 260)
// at radius 40 (outer) / 18 (inner) — flat [x1, y1, x2, y2, ...].
let starPts = makeArray(20, 0.0)
let starCx = 560.0
let starCy = 260.0

// Optional image — drops cleanly if there is no demo.png in this
// directory. Place any PNG / JPEG / GIF / WebP as ./demo.png and the
// demo will draw it in the top-right corner.
let demoImg = loadImage("demo.png")
let haveImg = type(demoImg) != "ERROR"
let imgW = 0
let imgH = 0
if haveImg {
    let w, h = imageSize(demoImg)
    imgW = w
    imgH = h
}

// Optional custom font — serve.sh copies Outfit.ttf from the repo.
// If absent, falls back to the default monospace text() builtin.
let titleFont = loadFont("Outfit.ttf", 28)
let haveFont = type(titleFont) != "ERROR"

window(700, 500, "kLex Canvas2D Demo", fn(frame) {
    background(0.07, 0.09, 0.13)

    // ── handle dropped files (drag-and-drop swaps the rotating image) ─
    let drops = droppedFiles()
    for url in drops {
        let img = loadImage(url)
        if type(img) != "ERROR" {
            demoImg = img
            haveImg = true
            let w, h = imageSize(demoImg)
            imgW = w
            imgH = h
        }
    }

    // ── bouncing circle ────────────────────────────────────────────────
    x = x + dx
    if x < 40.0  { dx = 3.5 }
    if x > 660.0 { dx = -3.5 }

    cy = cy + dcy
    if cy < 80.0  { dcy = 2.0 }
    if cy > 420.0 { dcy = -2.0 }

    fill(1.0, 0.55, 0.2)
    noStroke()
    circle(x, cy, 28.0)

    // ── shadow + rounded rect ─────────────────────────────────────────
    shadow(4.0, 4.0, 8.0, 0.0, 0.0, 0.0, 0.6)
    fill(0.2, 0.7, 1.0)
    stroke(1.0, 1.0, 1.0)
    strokeWeight(2.0)
    roundedRect(50.0, 50.0, 160.0, 80.0, 14.0)
    noShadow()

    // ── triangle ──────────────────────────────────────────────────────
    fill(0.95, 0.85, 0.2)
    noStroke()
    triangle(280.0, 60.0, 400.0, 60.0, 340.0, 160.0)

    // ── arc (donut wedge) ──────────────────────────────────────────────
    fill(0.6, 0.4, 0.95)
    noStroke()
    arc(150.0, 350.0, 50.0, 0.0, 1.8)

    // ── star polygon ──────────────────────────────────────────────────
    let i = 0
    let TWO_PI = 6.283185307
    while i < 10 {
        let theta = i * (TWO_PI / 10.0) - 1.5707963
        let r = 18.0
        if i % 2 == 0 { r = 40.0 }
        starPts[i * 2]     = starCx + r * cos(theta)
        starPts[i * 2 + 1] = starCy + r * sin(theta)
        i = i + 1
    }
    fill(0.95, 0.85, 0.2)
    stroke(1.0, 1.0, 1.0)
    strokeWeight(1.5)
    polygon(starPts)

    // ── additive blend (overlapping circles) ──────────────────────────
    blendMode("add")
    noStroke()
    fill(1.0, 0.0, 0.0, 0.7)
    circle(420.0, 360.0, 30.0)
    fill(0.0, 1.0, 0.0, 0.7)
    circle(450.0, 360.0, 30.0)
    fill(0.0, 0.0, 1.0, 0.7)
    circle(435.0, 380.0, 30.0)
    blendMode("normal")

    // ── clipped animated content ──────────────────────────────────────
    pushClip(240.0, 200.0, 80.0, 80.0)
    fill(0.3, 0.9, 0.6)
    noStroke()
    circle(280.0 + (frame % 60) - 30.0, 240.0, 40.0)
    fill(0.95, 0.4, 0.6)
    circle(280.0 - (frame % 60) + 30.0, 240.0, 30.0)
    popClip()

    // Frame around the clipped region.
    noFill()
    stroke(1.0, 1.0, 1.0, 0.5)
    strokeWeight(1.0)
    rect(240.0, 200.0, 80.0, 80.0)

    // ── scattered points ──────────────────────────────────────────────
    stroke(1.0, 1.0, 1.0)
    strokeWeight(3.0)
    let j = 0
    while j < 12 {
        point(40.0 + j * 50.0, 480.0)
        j = j + 1
    }

    // ── ground line ───────────────────────────────────────────────────
    stroke(0.5, 1.0, 0.5)
    strokeWeight(2.0)
    line(50.0, 460.0, 650.0, 460.0)

    // ── optional loaded image ─────────────────────────────────────────
    // Slowly rotating, drawn at 96×96 in the top-right corner.
    if haveImg {
        pushMatrix()
        translate(620.0, 80.0)
        rotate(frame * 0.01)
        drawImage(demoImg, -48.0, -48.0, 96.0, 96.0)
        popMatrix()

        fill(0.7, 0.7, 0.7)
        text("demo.png " + str(imgW) + "x" + str(imgH), 540.0, 140.0, 1.0)
    } else {
        // No demo image — draw a placeholder so the spot isn't empty.
        noFill()
        stroke(0.4, 0.4, 0.4)
        strokeWeight(1.0)
        rect(572.0, 32.0, 96.0, 96.0)
        fill(0.5, 0.5, 0.5)
        text("drag-drop", 588.0, 70.0, 1.0)
        text("any image", 588.0, 82.0, 1.0)
    }

    // ── title text (custom font if loaded, default monospace otherwise) ─
    fill(1.0, 1.0, 1.0)
    if haveFont {
        textFont(titleFont, "kLex Canvas2D — frame " + str(frame), 50.0, 14.0)
    } else {
        text("kLex Canvas2D — frame " + str(frame), 50.0, 20.0, 2.0)
    }

    // mouse-coords label — right-aligned via textWidth().
    let label = "mouse: (" + str(mouseX()) + ", " + str(mouseY()) + ")"
    let w = 0.0
    if haveFont {
        w = textWidth(titleFont, label, 0.6)
    } else {
        w = textWidth(label, 1.5)
    }
    if haveFont {
        textFont(titleFont, label, 690.0 - w, 438.0, 0.6)
    } else {
        text(label, 690.0 - w, 440.0, 1.5)
    }

    // ── cursor highlight while button is held ─────────────────────────
    if mouseDown() {
        fill(1.0, 1.0, 0.3, 0.4)
        noStroke()
        circle(mouseX(), mouseY(), 40.0)
    }

    // Click feedback — one-shot per frame.
    if mouseClicked() {
        fill(1.0, 0.3, 0.3)
        noStroke()
        circle(mouseX(), mouseY(), 8.0)
    }
})
