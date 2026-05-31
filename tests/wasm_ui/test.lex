// tests/wasm_ui/test.lex — Phase 1a + 1b widget smoke test.
// Tabbed layout so we can browse each batch.

let activeTab = 0
let themeName = "nebula"
let chkOn     = true
let toggleOn  = false
let volume    = 0.4
let radioVal  = "medium"
let clicks    = 0
let stepVal   = 10
let openIdx   = 0
let splitX    = 380
let modalOpen = false
let modalRes  = ""

// Phase 2 — state declared OUTSIDE the closure so listMulti / treeView
// state survives across frames. Both widgets follow the immediate-mode
// "caller owns state" convention: pass current state in, reassign the
// return value each frame.
let selFlags = [false, true, false, false, false,
                true, false, false, false, false,
                false, false, false, false, false]
let tvExpanded = [true, true, false, false, true, false, false, true, false, false]
let tvSelected = -1

// Phase 3 — dropdown selection persists across frames.
let ddPicked = "apple"
let ddItems  = ["apple", "banana", "cherry", "date", "elderberry"]

// Phase 3.5 — contextMenu state + typed-char echo buffer
let ctxVisible = false
let ctxX = 0
let ctxY = 0
let ctxLastPick = ""
let typedEcho = ""

// Phase 5 — global disabled toggle for the demo.
let formEnabled = true

// Phase 4 — text input state (threaded across frames).
let nameField = "Karl"
let emailField = ""
let notesField = "Try typing in either field. Arrow keys, Home/End, Shift+Arrows for selection, Backspace, Delete all work."

// Phase 4 — textArea (multi-line)
let bigNotes = "This is a textArea.\nPress Enter to insert a new line.\nUp/Down arrows navigate between lines.\nWheel scrolls when content overflows.\nShift+Click or Shift+Arrow to select across lines.\n\nAdd more lines below to test vertical scroll.\nLine 8.\nLine 9.\nLine 10.\nLine 11.\nLine 12.\nLine 13.\nLine 14."

// Install the initial theme once.
setTheme(themeName)

window(1180, 820, "kLex UI widgets — theme + polish demo", fn(frame) {
    background(0.10, 0.12, 0.16)

    uiBegin()

    label("kLex UI smoke test — tabs select the widget batch", 24, 14, 0.45)

    // Theme picker — top-right row of buttons. Clicking re-applies the
    // preset so palette + radii + font base all update in lockstep.
    label("theme:", 540, 18, 0.45)
    uiBeginRow(590, 12, 28, 6)
    if button("nebula", uiRowX(), uiRowY(), 60, uiRowH()) {
        themeName = "nebula"
        setTheme(themeName)
    }
    uiRowAdvance(60)
    if button("light", uiRowX(), uiRowY(), 56, uiRowH()) {
        themeName = "light"
        setTheme(themeName)
    }
    uiRowAdvance(56)
    if button("dark", uiRowX(), uiRowY(), 50, uiRowH()) {
        themeName = "dark"
        setTheme(themeName)
    }
    uiRowAdvance(50)
    if button("highContrast", uiRowX(), uiRowY(), 110, uiRowH()) {
        themeName = "highContrast"
        setTheme(themeName)
    }
    uiRowAdvance(110)

    let tabs_items = ["Phase 1a — foundations", "Phase 1b — composites", "Phase 1b — charts", "Phase 2 — scroll family", "Phase 3 — popups", "Phase 3.5 — input", "Phase 4 — text edit"]
    activeTab = tabs(24, 56, 1130, tabs_items, activeTab)

    if activeTab == 0 {
        uiBeginCol(24, 100, 280, 12)
        if button("Click me", uiColX(), uiColY(), 140, 32) {
            clicks = clicks + 1
        }
        uiColAdvance(32)
        chkOn = checkbox("Enable feature", uiColX(), uiColY(), chkOn)
        uiColAdvance(28)
        toggleOn = toggle("Pretty mode", uiColX(), uiColY(), toggleOn)
        uiColAdvance(28)
        label("Volume", uiColX(), uiColY(), 0.45)
        uiColAdvance(20)
        volume = slider("", uiColX(), uiColY(), 260, volume, 0.0, 1.0)
        uiColAdvance(40)
        label("Difficulty", uiColX(), uiColY(), 0.45)
        uiColAdvance(20)
        radioVal = radio("Easy",   uiColX(),       uiColY(), "easy",   radioVal)
        radioVal = radio("Medium", uiColX() + 100, uiColY(), "medium", radioVal)
        radioVal = radio("Hard",   uiColX() + 220, uiColY(), "hard",   radioVal)
        uiColAdvance(32)
        label("Progress", uiColX(), uiColY(), 0.45)
        uiColAdvance(20)
        progressBar(uiColX(), uiColY(), 260, 14, volume, 0.0, 1.0)

        // Live state readout.
        uiBeginCol(420, 100, 320, 8)
        label("clicks:   " + str(clicks),    uiColX(), uiColY(), 0.45) uiColAdvance(22)
        label("checkbox: " + str(chkOn),     uiColX(), uiColY(), 0.45) uiColAdvance(22)
        label("toggle:   " + str(toggleOn),  uiColX(), uiColY(), 0.45) uiColAdvance(22)
        label("slider:   " + str(volume),    uiColX(), uiColY(), 0.45) uiColAdvance(22)
        label("radio:    " + radioVal,       uiColX(), uiColY(), 0.45) uiColAdvance(22)
        label("frame:    " + str(frame),     uiColX(), uiColY(), 0.45)
    }

    if activeTab == 1 {
        // Composites column.
        uiBeginCol(24, 100, 320, 14)

        label("numericStepper", uiColX(), uiColY(), 0.45)
        uiColAdvance(20)
        stepVal = numericStepper("", uiColX(), uiColY(), 200, stepVal, 0, 100)
        uiColAdvance(48)

        label("accordion (pass openIdx through)", uiColX(), uiColY(), 0.45)
        uiColAdvance(20)
        let sections = ["General", "Display", "Audio", "Network"]
        openIdx = accordion(uiColX(), uiColY(), 280, sections, openIdx)
        uiColAdvance(28 * 4 + 6)

        if button("Show modal", uiColX(), uiColY(), 140, 32) {
            modalOpen = true
        }

        // Splitter column.
        uiBeginCol(400, 100, 340, 10)
        label("splitter — drag the vertical bar", uiColX(), uiColY(), 0.45)
        uiColAdvance(22)
        // Splitter ranges within this column.
        splitX = splitter(splitX, 400, uiColY(), 360, "v", 410, 730)
        // Visual hint of the two panes.
        progressBar(400, uiColY() + 380, splitX - 400, 14, 1.0, 0.0, 1.0)
        progressBar(splitX + 4, uiColY() + 380, 730 - splitX, 14, 1.0, 0.0, 1.0)

        if modalOpen {
            modalRes = modal("Confirm action", "This will reset your settings. Continue?", ["Cancel", "OK"])
            if modalRes != "" {
                modalOpen = false
            }
        }
        label("last modal: " + modalRes, 24, 520, 0.45)
    }

    if activeTab == 3 {
        // ── Phase 2 — list / listMulti / treeView / table ────────
        let cities = ["London", "Paris", "Tokyo", "New York", "Sydney",
                      "Berlin", "Madrid", "Rome", "Cairo", "Lagos",
                      "Mumbai", "Beijing", "Seoul", "Mexico City", "Dubai"]
        label("list (single-select, scrollable)", 24, 100, 0.45)
        let picked = list("", cities, 24, 115, 200, 180)
        label("picked: " + str(picked), 24, 305, 0.45)

        label("listMulti (toggle each row)", 250, 100, 0.45)
        selFlags = listMulti("", cities, selFlags, 250, 115, 200, 180)

        label("treeView (click row, click again to expand)", 480, 100, 0.45)
        let labels = ["Animals", "Mammals", "Cats", "Dogs", "Birds",
                      "Eagles", "Sparrows", "Plants", "Trees", "Flowers"]
        let levels = [0, 1, 2, 2, 1, 2, 2, 0, 1, 1]
        let tvResult = treeView(480, 115, 250, 180, labels, levels, tvExpanded)
        tvSelected = tvResult[0]
        tvExpanded = tvResult[1]

        // ── table (the datagrid) — scrollable bottom area ────────
        label("table — datagrid with scroll", 24, 335, 0.45)
        let headers = ["Name", "Age", "City"]
        let rows = [
            ["Alice",   "32", "London"],
            ["Bob",     "45", "Paris"],
            ["Cara",    "28", "Tokyo"],
            ["Dan",     "37", "New York"],
            ["Eve",     "41", "Sydney"],
            ["Finn",    "29", "Berlin"],
            ["Grace",   "34", "Madrid"],
            ["Helga",   "52", "Rome"],
            ["Ivan",    "26", "Cairo"],
            ["Juno",    "44", "Lagos"],
            ["Karim",   "31", "Mumbai"],
            ["Lina",    "39", "Beijing"]
        ]
        let selRow = table(headers, rows, 24, 360, 480, 160)
        label("selected row: " + str(selRow), 520, 360, 0.45)
    }

    if activeTab == 4 {
        // ── dropdown ─────────────────────────────────────────────
        label("dropdown — click header, click item; hover triggers tooltip", 24, 100, 0.45)
        ddPicked = dropdown("Fruit", ddItems, 24, 130, 200)
        tooltip("Pick your favourite fruit (hover holds 500ms to show this)")
        label("picked: " + ddPicked, 240, 145, 0.45)

        // ── toast triggers ───────────────────────────────────────
        label("toast — click to push a notification (bottom-right)", 24, 200, 0.45)
        uiBeginRow(24, 230, 32, 8)
        if button("info", uiRowX(), uiRowY(), 80, uiRowH()) {
            toast("Saved your changes.")
        }
        uiRowAdvance(80)
        if button("success", uiRowX(), uiRowY(), 90, uiRowH()) {
            toast("Backup complete.", "success")
        }
        uiRowAdvance(90)
        if button("warn", uiRowX(), uiRowY(), 80, uiRowH()) {
            toast("Disk almost full.", "warn", 4.0)
        }
        uiRowAdvance(80)
        if button("error", uiRowX(), uiRowY(), 80, uiRowH()) {
            toast("Connection lost.", "error", 4.0)
        }
        uiRowAdvance(80)

        label("Hover the buttons for their tooltips:", 24, 290, 0.45)
        if button("Hover me", 24, 320, 140, 32) { }
        tooltip("This button doesn't do anything — it just demonstrates tooltips.")
        if button("Another one", 180, 320, 140, 32) { }
        tooltip("Different button, different tooltip!")
    }

    if activeTab == 5 {
        // ── contextMenu — right-click anywhere in the panel ──────
        label("contextMenu — right-click in the dark panel below to open", 24, 100, 0.45)
        // Panel.
        progressBar(24, 110, 480, 200, 0.0, 0.0, 1.0)
        // Right-click detection inside the panel.
        if mouseRightClicked() {
            if mouseX() >= 24.0 && mouseX() <= 504.0 && mouseY() >= 110.0 && mouseY() <= 310.0 {
                ctxVisible = true
                ctxX = int(mouseX())
                ctxY = int(mouseY())
            }
        }
        if ctxVisible {
            let pick = contextMenu(ctxX, ctxY, ["Open", "Save", "Rename", "Delete"], ctxVisible)
            if pick >= 0 {
                ctxLastPick = ["Open", "Save", "Rename", "Delete"][pick]
                ctxVisible = false
            }
            if pick == -2 {
                ctxVisible = false
            }
        }
        label("last context-menu pick: " + ctxLastPick, 24, 320, 0.45)

        // ── getTypedChars — type with the canvas focused ─────────
        label("getTypedChars — type any keys; printable chars accumulate", 24, 360, 0.45)
        let chars = getTypedChars()
        if chars != "" {
            typedEcho = typedEcho + chars
        }
        progressBar(24, 390, 700, 36, 0.0, 0.0, 1.0)
        // Vertically centre the echo text inside the 36-px-tall progress
        // bar via the new lineHeight() builtin.
        label(typedEcho, 32, 390 + (36 - lineHeight(0.45)) / 2, 0.45)
        if button("Clear", 24, 440, 100, 32) {
            typedEcho = ""
        }
    }

    if activeTab == 6 {
        // ── textInput (Phase 4) + disabled-state demo ────────────
        label("textInput — click to focus, type, arrow keys to navigate", 24, 100, 0.45)
        label("Shift+Arrow / Shift+Click extends selection. Home/End jump to ends.", 24, 110, 0.4)

        // Top-right: toggle the disabled state across the whole form.
        formEnabled = toggle("Form enabled", 580, 150, formEnabled)

        // pushDisabled greys out and freezes everything between push
        // and pop — buttons no longer hover, inputs no longer focus.
        pushDisabled(!formEnabled)
        nameField  = textInput("Name",  nameField,  24, 150, 320, 36)
        emailField = textInput("Email", emailField, 24, 220, 320, 36)
        notesField = textInput("Notes", notesField, 24, 290, 700, 36)
        popDisabled()

        // Echo back so we can verify state threading works.
        label("name:  " + nameField,  380, 150, 0.45)
        label("email: " + emailField, 380, 220, 0.45)

        label("textArea — Enter inserts newline, Up/Down navigate lines, wheel scrolls", 24, 360, 0.45)
        bigNotes = textArea("Multi-line notes", bigNotes, 24, 390, 700, 130)
        label("lines: " + str(len(split(bigNotes, "\n"))) + "  total chars: " + str(len(bigNotes)), 24, 530, 0.4)
    }

    if activeTab == 2 {
        let series = [12.0, 18.0, 9.0, 22.0, 15.0, 27.0, 33.0, 19.0, 25.0, 30.0]

        label("sparkline", 24, 100, 0.45)
        sparkline(series, 24, 110, 320, 40)

        label("lineChart", 24, 170, 0.45)
        lineChart(series, 24, 190, 320, 110)

        label("barChart", 380, 100, 0.45)
        barChart(series, 380, 110, 320, 190)

        label("pieChart", 24, 330, 0.45)
        let slices  = [30.0, 25.0, 20.0, 15.0, 10.0]
        let palette = [
            [0.45, 0.85, 1.00, 1.0],
            [0.95, 0.62, 0.30, 1.0],
            [0.55, 0.85, 0.45, 1.0],
            [0.95, 0.40, 0.55, 1.0],
            [0.65, 0.55, 0.95, 1.0]
        ]
        pieChart(slices, palette, 120, 440, 60)
        label("Five-slice distribution", 220, 440, 0.45)
    }

    uiEnd()
})
