import "stdlib/ui.lex" as ui

// ── State ─────────────────────────────────────────────────────────────────────

activeTab  = 0
tabLabels  = ["Inputs", "Selection", "Display", "Typography", "Panels"]

// Inputs tab
checked   = false
toggled   = false
radioVal  = "B"
buf       = "Hello kLex"
numVal    = 7.0
sliderA   = 60.0
sliderB   = 30.0
sliderC   = 80.0
clicks    = 0

// Selection tab
dropIdx   = 1
listIdx   = 3

dropItems = ["Alpha",  "Beta", "Gamma", "Delta", "Epsilon",
             "Zeta",   "Eta",  "Theta", "Iota",  "Kappa"]

listItems = ["Amphibian", "Axolotl",   "Bullfrog",  "Caecilian",
             "Dart Frog", "Fire Belly", "Giant Toad","Glass Frog",
             "Hellbender","Mudpuppy",   "Newt",      "Olm",
             "Salamander","Siren",      "Tree Frog", "Waterdог"]

// Display tab
prog1  = 0.0
prog2  = 0.0
prog3  = 0.0

// Panels tab
focusedPanel = 0


window(1100, 760, "kLex UI Showcase — All Widgets", fn(frame) {

    ui.background()

    // Animate progress bars
    prog1 = prog1 + 0.12
    if prog1 > 100.0 { prog1 = 0.0 }
    prog2 = prog2 + 0.07
    if prog2 > 100.0 { prog2 = 0.0 }
    prog3 = prog3 + 0.19
    if prog3 > 100.0 { prog3 = 0.0 }

    ww  = float(winWidth())
    wh  = float(winHeight())
    pad = 10.0

    // ── Tab bar ───────────────────────────────────────────────────────────────
    activeTab = ui.tabs(pad, pad, ww - pad * 2.0, tabLabels, activeTab)

    tabY = pad + 40.0
    ch   = wh - tabY - pad
    colW = (ww - pad * 4.0) / 3.0

    // ── Tab 0: Inputs ─────────────────────────────────────────────────────────
    if activeTab == 0 {
        col1X = pad
        col2X = pad * 2.0 + colW
        col3X = pad * 3.0 + colW * 2.0

        // Left — toggles & radio
        ui.panelTitle(col1X, tabY, colW, ch, "Toggles & Radio")
        ui.beginLayout(col1X + 12.0, tabY + 38.0, colW - 24.0, 10.0)
        ui.layoutSubheading("Toggle Controls")
        checked = ui.layoutCheckbox("Enable shadows", checked)
        toggled = ui.layoutToggle("Dark mode", toggled)
        ui.layoutSeparator()
        ui.layoutSubheading("Radio Group")
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option A", "A", radioVal)
        ui.advanceCursor(26.0)
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option B", "B", radioVal)
        ui.advanceCursor(26.0)
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option C", "C", radioVal)
        ui.advanceCursor(26.0)
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option D", "D", radioVal)
        ui.advanceCursor(26.0)
        ui.layoutSeparator()
        ui.layoutLabelDim("radio: " + radioVal)
        ui.layoutLabelDim("checked: " + str(checked))
        ui.layoutLabelDim("toggled: " + str(toggled))

        // Middle — text & numeric input
        ui.panelTitle(col2X, tabY, colW, ch, "Text & Numeric Input")
        ui.beginLayout(col2X + 12.0, tabY + 38.0, colW - 24.0, 10.0)
        ui.layoutSubheading("Text Field")
        buf = ui.layoutTextField("Label", buf)
        ui.layoutLabelDim("value: " + buf)
        ui.layoutSeparator()
        ui.layoutSubheading("Numeric Field")
        numVal = ui.layoutNumericField("Count", numVal, 0.0, 20.0)
        ui.layoutLabelDim("value: " + str(int(numVal)))
        ui.layoutSeparator()
        ui.layoutSubheading("Button")
        if ui.layoutButton(150.0, 34.0, "Click Me") {
            clicks = clicks + 1
        }
        ui.layoutLabelDim("clicked " + str(clicks) + " times")
        ui.layoutSeparator()
        if ui.layoutButton(150.0, 34.0, "Reset All") {
            checked  = false
            toggled  = false
            radioVal = "B"
            buf      = ""
            numVal   = 7.0
            sliderA  = 60.0
            sliderB  = 30.0
            sliderC  = 80.0
            clicks   = 0
        }

        // Right — sliders
        ui.panelTitle(col3X, tabY, colW, ch, "Sliders")
        ui.beginLayout(col3X + 12.0, tabY + 38.0, colW - 24.0, 10.0)
        ui.layoutSubheading("Three Sliders")
        ui.layoutSeparator()
        sliderA = ui.layoutSlider("Volume",   sliderA, 0.0, 100.0)
        sliderB = ui.layoutSlider("Pitch",    sliderB, 0.0, 100.0)
        sliderC = ui.layoutSlider("Reverb",   sliderC, 0.0, 100.0)
        ui.layoutSeparator()
        ui.layoutSubheading("Live Values")
        ui.layoutLabel("Volume: " + str(int(sliderA)) + " %")
        ui.layoutLabel("Pitch:  " + str(int(sliderB)) + " %")
        ui.layoutLabel("Reverb: " + str(int(sliderC)) + " %")
        ui.layoutSeparator()
        ui.layoutLabelDim("Drag handle or click track")
    }

    // ── Tab 1: Selection ──────────────────────────────────────────────────────
    if activeTab == 1 {
        leftW   = ww * 0.40
        rightX  = pad * 2.0 + leftW
        rightW  = ww - rightX - pad
        halfH   = ch * 0.5 - 4.0

        // List box fills the left column
        ui.panelTitle(pad, tabY, leftW, ch, "List Box")
        ui.beginLayout(pad + 12.0, tabY + 38.0, leftW - 24.0, 8.0)
        ui.layoutLabelDim("UP / DOWN or drag scrollbar")
        ui.advanceCursor(4.0)
        listIdx = ui.listBox(pad + 12.0, ui.layoutY(),
                             leftW - 24.0, ch - (ui.layoutY() - tabY) - 12.0,
                             listItems, listIdx)

        // Top-right: dropdown
        ui.panelTitle(rightX, tabY, rightW, halfH, "Dropdown")
        ui.beginLayout(rightX + 12.0, tabY + 38.0, rightW - 24.0, 10.0)
        ui.layoutSubheading("Select an item")
        dropIdx = ui.layoutDropdown("", dropItems, dropIdx)
        ui.layoutSeparator()
        ui.layoutLabel("Selected index: " + str(dropIdx))
        if dropIdx >= 0 && dropIdx < len(dropItems) {
            ui.layoutLabel("Selected value: " + dropItems[dropIdx])
        }

        // Bottom-right: selection detail
        detailY = tabY + halfH + 8.0
        detailH = ch - halfH - 8.0
        ui.panelTitle(rightX, detailY, rightW, detailH, "List Selection")
        ui.beginLayout(rightX + 12.0, detailY + 38.0, rightW - 24.0, 10.0)
        ui.layoutLabel("List index: " + str(listIdx))
        if listIdx >= 0 && listIdx < len(listItems) {
            ui.layoutSubheading(listItems[listIdx])
        }
        ui.layoutSeparator()
        ui.layoutLabelDim("Click any row to select.")
        ui.layoutLabelDim("Scroll with UP/DOWN or drag.")
    }

    // ── Tab 2: Display ────────────────────────────────────────────────────────
    if activeTab == 2 {
        col1X = pad
        col2X = pad * 2.0 + colW
        col3X = pad * 3.0 + colW * 2.0

        // Left — progress bars
        ui.panelTitle(col1X, tabY, colW, ch * 0.55, "Progress Bars")
        ui.beginLayout(col1X + 12.0, tabY + 38.0, colW - 24.0, 14.0)
        ui.layoutProgressBar("Upload",   prog1, 100.0)
        ui.layoutProgressBar("Download", prog2, 100.0)
        ui.layoutProgressBar("Sync",     prog3, 100.0)
        ui.layoutSeparator()
        ui.layoutLabelDim("All bars animate automatically.")

        // Left-bottom — spinner
        spinY = tabY + ch * 0.55 + 8.0
        spinH = ch - ch * 0.55 - 8.0
        ui.panelTitle(col1X, spinY, colW, spinH, "Spinner")
        ui.beginLayout(col1X + 12.0, spinY + 38.0, colW - 24.0, 12.0)
        ui.layoutSubheading("Loading animation")
        ui.advanceCursor(6.0)
        sy = ui.layoutY()
        ui.spinner(col1X + 30.0, sy + 14.0, 16.0)
        ui.label(col1X + 58.0, sy + 6.0, "Processing...")
        ui.advanceCursor(40.0)
        ui.layoutSeparator()
        ui.layoutLabelDim("12-segment rotating dots.")

        // Middle — badges
        ui.panelTitle(col2X, tabY, colW, ch * 0.55, "Badges")
        ui.beginLayout(col2X + 12.0, tabY + 38.0, colW - 24.0, 14.0)
        ui.layoutSubheading("Status Pills")
        ui.advanceCursor(4.0)
        by = ui.layoutY()
        ui.badge(col2X + 12.0,  by, "OK",    ui.UI_BADGE_GREEN)
        ui.badge(col2X + 56.0,  by, "ERROR", ui.UI_BADGE_RED)
        ui.badge(col2X + 120.0, by, "WARN",  ui.UI_BADGE_YELLOW)
        ui.badge(col2X + 176.0, by, "INFO",  ui.UI_BADGE_BLUE)
        ui.advanceCursor(34.0)
        by2 = ui.layoutY()
        ui.badge(col2X + 12.0, by2, "IDLE",    ui.UI_BADGE_GREY)
        ui.badge(col2X + 64.0, by2, "RUNNING", ui.UI_BADGE_BLUE)
        ui.badge(col2X + 140.0, by2, "DONE",   ui.UI_BADGE_GREEN)
        ui.advanceCursor(34.0)
        ui.layoutSeparator()
        ui.layoutSubheading("Notification Counts")
        ui.advanceCursor(4.0)
        by3 = ui.layoutY()
        ui.badge(col2X + 12.0,  by3, "5 unread",   ui.UI_BADGE_BLUE)
        ui.badge(col2X + 100.0, by3, "2 errors",   ui.UI_BADGE_RED)
        ui.badge(col2X + 170.0, by3, "12 pending", ui.UI_BADGE_YELLOW)
        ui.advanceCursor(34.0)

        // Middle-bottom — color swatches
        swatchY = tabY + ch * 0.55 + 8.0
        swatchH = ch - ch * 0.55 - 8.0
        ui.panelTitle(col2X, swatchY, colW, swatchH, "Color Swatches")
        ui.beginLayout(col2X + 12.0, swatchY + 38.0, colW - 24.0, 10.0)
        ui.layoutSubheading("Palette")
        ui.advanceCursor(6.0)
        palette = [[1.0,0.3,0.3],[1.0,0.6,0.2],[0.9,0.9,0.2],
                   [0.3,0.9,0.4],[0.3,0.6,1.0],[0.7,0.3,1.0],
                   [1.0,0.4,0.8],[0.3,0.9,0.9],[0.9,0.6,0.4]]
        sw    = (colW - 28.0) / 9.0
        sy2   = ui.layoutY()
        for ci in range(0, 9) {
            c = palette[ci]
            ui.colorSwatch(col2X + 12.0 + float(ci) * sw, sy2, sw - 3.0, c[0], c[1], c[2])
        }
        ui.advanceCursor(sw + 6.0)
        ui.layoutLabelDim("9 swatches via colorSwatch()")

        // Right — raw builtin progress bar (no auto-layout) to show standalone use
        ui.panelTitle(col3X, tabY, colW, ch, "Standalone Builtins")
        ui.beginLayout(col3X + 12.0, tabY + 38.0, colW - 24.0, 14.0)
        ui.layoutSubheading("progressBar() raw call")
        ui.advanceCursor(8.0)
        ry = ui.layoutY()
        ui.progressBar(col3X + 12.0, ry, colW - 24.0, 16.0, prog1, 100.0)
        ui.advanceCursor(28.0)
        ui.progressBar(col3X + 12.0, ui.layoutY(), colW - 24.0, 10.0, prog2, 100.0)
        ui.advanceCursor(22.0)
        ui.layoutSeparator()
        ui.layoutSubheading("separator() raw call")
        ui.advanceCursor(4.0)
        ui.separator(col3X + 12.0, ui.layoutY(), colW - 24.0)
        ui.advanceCursor(10.0)
        ui.layoutLabelDim("A standalone horizontal rule.")
        ui.separator(col3X + 12.0, ui.layoutY(), colW - 24.0)
        ui.advanceCursor(10.0)
        ui.separator(col3X + 12.0, ui.layoutY(), colW - 24.0)
        ui.advanceCursor(10.0)
        ui.layoutSeparator()
        ui.layoutSubheading("shadow() raw call")
        ui.advanceCursor(6.0)
        shY = ui.layoutY()
        ui.shadow(col3X + 12.0, shY, colW - 24.0, 50.0)
        ui.panel(col3X + 12.0, shY, colW - 24.0, 50.0)
        ui.label(col3X + 20.0, shY + 16.0, "Panel with shadow()")
    }

    // ── Tab 3: Typography ─────────────────────────────────────────────────────
    if activeTab == 3 {
        halfW  = (ww - pad * 3.0) * 0.5
        rightX = pad * 2.0 + halfW

        // Left — all text variants
        ui.panelTitle(pad, tabY, halfW, ch, "All Text Variants")
        ui.beginLayout(pad + 12.0, tabY + 38.0, halfW - 24.0, 8.0)

        ui.layoutHeading("Heading  ( × 2 scale )")
        ui.layoutSubheading("Subheading  ( × 1.5 scale )")
        ui.layoutSeparator()

        ui.layoutLabel("label()  — standard body text")
        ui.layoutLabelDim("labelDim()  — hint / secondary text")
        ly = ui.layoutY()
        ui.labelColored(pad + 12.0, ly, "labelColored()  — custom colour", ui.UI_ACCENT)
        ui.advanceCursor(18.0)
        ui.layoutSeparator()

        ui.layoutLabel("Alignment variants:")
        ui.advanceCursor(4.0)
        ay = ui.layoutY()
        ui.label(pad + 12.0, ay, "label()  left")
        ui.advanceCursor(18.0)
        ay2 = ui.layoutY()
        ui.labelCenter(pad + 12.0, ay2, halfW - 24.0, "labelCenter()")
        ui.advanceCursor(18.0)
        ay3 = ui.layoutY()
        ui.labelRight(pad + 12.0, ay3, halfW - 24.0, "labelRight()")
        ui.advanceCursor(18.0)
        ui.layoutSeparator()

        ui.layoutLabel("dividerLabel():")
        ui.advanceCursor(6.0)
        dy = ui.layoutY()
        ui.dividerLabel(pad + 12.0, dy, halfW - 24.0, "SECTION")
        ui.advanceCursor(20.0)
        ui.dividerLabel(pad + 12.0, ui.layoutY(), halfW - 24.0, "MORE")
        ui.advanceCursor(20.0)

        // Right — tooltip demo
        ui.panelTitle(rightX, tabY, halfW, ch, "Tooltip Demo")
        ui.beginLayout(rightX + 12.0, tabY + 38.0, halfW - 24.0, 14.0)
        ui.layoutSubheading("Hover buttons for tooltips")
        ui.layoutSeparator()
        ui.layoutLabelDim("tooltip() always renders last.")
        ui.layoutSeparator()

        if ui.layoutButton(160.0, 34.0, "Save") { }
        if ui.layoutButton(160.0, 34.0, "Delete") { }
        if ui.layoutButton(160.0, 34.0, "Export") { }
        if ui.layoutButton(160.0, 34.0, "Share") { }
        ui.layoutSeparator()

        ui.layoutLabelDim("Also: badge() renders pill text.")
        ui.advanceCursor(6.0)
        bb = ui.layoutY()
        ui.badge(rightX + 12.0, bb, "NEW",    ui.UI_BADGE_GREEN)
        ui.badge(rightX + 60.0, bb, "v2.0",   ui.UI_BADGE_BLUE)
        ui.badge(rightX + 112.0, bb, "BETA",  ui.UI_BADGE_YELLOW)

        // Tooltips — must be last in draw order
        ui.tooltip(rightX + 12.0, tabY + 116.0, 160.0, 34.0, "Save current document to disk")
        ui.tooltip(rightX + 12.0, tabY + 164.0, 160.0, 34.0, "Permanently delete — cannot undo!")
        ui.tooltip(rightX + 12.0, tabY + 212.0, 160.0, 34.0, "Export to JSON or CSV format")
        ui.tooltip(rightX + 12.0, tabY + 260.0, 160.0, 34.0, "Share via link or email")
    }

    // ── Tab 4: Panels ─────────────────────────────────────────────────────────
    if activeTab == 4 {
        col1X = pad
        col2X = pad * 2.0 + colW
        col3X = pad * 3.0 + colW * 2.0
        halfH = ch * 0.5 - 4.0

        // panel() — no title bar
        ui.panel(col1X, tabY, colW, halfH)
        ui.beginLayout(col1X + 12.0, tabY + 16.0, colW - 24.0, 8.0)
        ui.layoutLabel("panel()")
        ui.layoutLabelDim("Plain panel — no title bar.")
        ui.layoutLabelDim("Border + corner brackets.")
        ui.layoutSeparator()
        ui.layoutLabelDim("Use when no header needed.")

        // panelTitle() below
        pBY = tabY + halfH + 8.0
        pBH = ch - halfH - 8.0
        ui.panelTitle(col1X, pBY, colW, pBH, "panelTitle()")
        ui.beginLayout(col1X + 12.0, pBY + 38.0, colW - 24.0, 8.0)
        ui.layoutLabelDim("Has a dark title bar with")
        ui.layoutLabelDim(">_ prompt and accent line.")

        // card() top
        ui.card(col2X, tabY, colW, halfH)
        ui.beginLayout(col2X + 12.0, tabY + 16.0, colW - 24.0, 8.0)
        ui.layoutLabel("card()")
        ui.layoutLabelDim("= shadow() + panel().")
        ui.layoutLabelDim("Floating elevated surface.")
        ui.layoutSeparator()
        if ui.layoutButton(120.0, 30.0, "Card Button") {
            clicks = clicks + 1
        }
        ui.layoutLabelDim("clicks: " + str(clicks))

        // shadow() standalone below card()
        shBY = tabY + halfH + 8.0
        shBH = ch - halfH - 8.0
        ui.shadow(col2X, shBY, colW, shBH)
        ui.panel(col2X, shBY, colW, shBH)
        ui.beginLayout(col2X + 12.0, shBY + 16.0, colW - 24.0, 8.0)
        ui.layoutLabel("shadow() + panel()")
        ui.layoutLabelDim("Three-layer soft drop shadow.")
        ui.layoutLabelDim("Manually composited.")

        // filePanel() — unfocused
        ui.filePanel(col3X, tabY, colW, halfH, "Documents")
        ui.beginLayout(col3X + 12.0, tabY + 38.0, colW - 24.0, 6.0)
        ui.layoutLabelDim("filePanel()")
        ui.layoutSeparator()
        fileItems = ["readme.md", "main.lex", "stdlib/", "tests/", "go.mod", "go.sum"]
        for fi in range(0, len(fileItems)) {
            ffy = ui.layoutY()
            if focusedPanel == 0 && fi == 0 {
                ui.labelColored(col3X + 14.0, ffy, "> " + fileItems[fi], ui.UI_ACCENT)
            } else {
                ui.label(col3X + 14.0, ffy, "  " + fileItems[fi])
            }
            ui.advanceCursor(16.0)
        }

        // filePanelFocused() — active/focused state
        fpFY = tabY + halfH + 8.0
        fpFH = ch - halfH - 8.0
        ui.filePanelFocused(col3X, fpFY, colW, fpFH, "src/eval/ [focused]")
        ui.beginLayout(col3X + 12.0, fpFY + 38.0, colW - 24.0, 6.0)
        ui.layoutLabelDim("filePanelFocused()")
        ui.layoutLabelDim("Bright border — active pane.")
        ui.layoutSeparator()
        srcItems = ["eval.go", "object.go", "env.go", "typecheck.go", "builtins_ui.go"]
        for si in range(0, len(srcItems)) {
            sfy = ui.layoutY()
            if si == 0 {
                ui.labelColored(col3X + 14.0, sfy, "> " + srcItems[si], ui.UI_ACCENT)
            } else {
                ui.label(col3X + 14.0, sfy, "  " + srcItems[si])
            }
            ui.advanceCursor(16.0)
        }
    }
})
