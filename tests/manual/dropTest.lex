// dropTest.lex — tests the droppedFiles() builtin.
// Run: KLEX_PATH=. ./klex tests/unit/dropTest.lex
// Drag any files or folders onto the window. Paths accumulate in the list.

let fontResult, fontErr = safe(loadFont, ["/System/Library/Fonts/SFNS.ttf", 16])
if fontErr != null {
    fontResult = loadFont("/System/Library/Fonts/Supplemental/Arial.ttf", 16)
}
let fnt = fontResult

let allDropped = makeArray(0)

window(700, 480, "droppedFiles() test", fn(frame) {
    background(0.07, 0.07, 0.09)

    let ww  = winWidth()
    let wh  = winHeight()
    let cx  = float(ww) * 0.5
    let t   = elapsedTime()

    // ── Consume this frame's drops and append to allDropped ──────────────────
    let dropped = droppedFiles()
    if len(dropped) > 0 {
        let n = len(allDropped)
        let m = len(dropped)
        let combined = makeArray(n + m, "")
        let i = 0
        while i < n {
            combined[i] = allDropped[i]
            i = i + 1
        }
        let j = 0
        while j < m {
            combined[n + j] = dropped[j]
            j = j + 1
        }
        allDropped = combined
    }

    // ── Empty state ──────────────────────────────────────────────────────────
    if len(allDropped) == 0 {
        let pulse = 0.25 + 0.12 * sin(t * 2.1)

        fill(0.11, 0.11, 0.16, 1.0)
        noStroke()
        roundedRect(36.0, 36.0, float(ww) - 72.0, float(wh) - 72.0, 18.0)

        stroke(0.38, 0.50, 0.92, pulse + 0.28)
        strokeWeight(2.0)
        roundedRect(36.0, 36.0, float(ww) - 72.0, float(wh) - 72.0, 18.0)
        noStroke()

        // Down-arrow
        let ay  = float(wh) * 0.5 - 46.0
        let col = pulse + 0.42
        fill(0.38, 0.50, 0.92, col)
        noStroke()
        polygon([cx - 20.0, ay,
                 cx + 20.0, ay,
                 cx + 20.0, ay + 26.0,
                 cx + 38.0, ay + 26.0,
                 cx,        ay + 58.0,
                 cx - 38.0, ay + 26.0,
                 cx - 20.0, ay + 26.0])

        fill(0.82, 0.84, 0.96, 0.90)
        noStroke()
        let msg = "Drop files or folders here"
        let tw  = textWidth(fnt, msg, 0.88)
        textFont(fnt, msg, cx - tw * 0.5, float(wh) * 0.5 + 32.0, 0.88)

        fill(0.38, 0.40, 0.56, 0.60)
        noStroke()
        let sub = "droppedFiles() returns their paths each frame"
        let tw2 = textWidth(fnt, sub, 0.56)
        textFont(fnt, sub, cx - tw2 * 0.5, float(wh) * 0.5 + 58.0, 0.56)

    // ── Files received ───────────────────────────────────────────────────────
    } else {
        // Header bar
        fill(0.10, 0.22, 0.12, 1.0)
        noStroke()
        rect(0.0, 0.0, float(ww), 46.0)

        fill(0.35, 0.80, 0.42, 1.0)
        noStroke()
        let n = len(allDropped)
        let suffix = " file dropped"
        if n != 1 { suffix = " files dropped" }
        let hdr = str(n) + suffix
        let tw  = textWidth(fnt, hdr, 0.82)
        textFont(fnt, hdr, cx - tw * 0.5, 12.0, 0.82)

        // File rows
        let lineH  = 34.0
        let isEven = true
        let i      = 0
        while i < n {
            let rowY = 46.0 + float(i) * lineH

            if isEven {
                fill(0.11, 0.11, 0.14, 1.0)
            } else {
                fill(0.09, 0.09, 0.12, 1.0)
            }
            isEven = !isEven
            noStroke()
            rect(0.0, rowY, float(ww), lineH)

            // Bottom separator
            fill(1.0, 1.0, 1.0, 0.05)
            noStroke()
            rect(0.0, rowY + lineH - 1.0, float(ww), 1.0)

            // Index badge
            fill(0.22, 0.32, 0.58, 1.0)
            noStroke()
            roundedRect(10.0, rowY + 7.0, 26.0, 20.0, 4.0)
            fill(0.75, 0.85, 1.0, 1.0)
            noStroke()
            let num = str(i + 1)
            let nw  = textWidth(fnt, num, 0.54)
            textFont(fnt, num, 23.0 - nw * 0.5, rowY + 9.0, 0.54)

            // Clip path text to window width
            pushClip(44, int(rowY), ww - 48, int(lineH))
            fill(0.82, 0.88, 0.96, 0.95)
            noStroke()
            textFont(fnt, allDropped[i], 48.0, rowY + 9.0, 0.64)
            popClip()

            i = i + 1
        }

        // Footer hint
        fill(0.28, 0.28, 0.38, 0.55)
        noStroke()
        let hint = "drop more files to add  •  restart to clear"
        let hw   = textWidth(fnt, hint, 0.50)
        textFont(fnt, hint, cx - hw * 0.5, float(wh) - 18.0, 0.50)
    }
})
