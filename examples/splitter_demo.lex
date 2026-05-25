// Demonstrates the splitter() builtin for resizable panel layouts.
// Drag the vertical divider left and right to resize the sidebar and main content area.

let splitX   = 220
let section  = "Overview"

let sections = ["Overview", "Settings", "Analytics", "Users", "Logs", "Help"]

window(1000, 680, "Splitter Demo", fn(frame) {

    background(18, 18, 22)

    let ww = winWidth()
    let wh = winHeight()

    uiBegin()

    // Draggable vertical divider.
    // Pass splitX in and reassign the return each frame — same pattern as slider().
    splitX = splitter(splitX, 0, 0, wh, "v", 120, ww - 200)

    // ── Sidebar ───────────────────────────────────────────────────────────────
    pushClip(0, 0, splitX, wh)
    fill(28, 30, 38)
    noStroke()
    rect(0, 0, splitX, wh)
    section = list("", sections, 0, 0, splitX, wh)
    popClip()

    // ── Main content ──────────────────────────────────────────────────────────
    let mainX = splitX + 1
    let mainW = ww - mainX
    pushClip(mainX, 0, mainW, wh)
    label("Section: " + section, mainX + 24, 28)
    label("Drag the divider left or right to resize.", mainX + 24, 60)
    popClip()

    uiEnd()

})
