import "stdlib/ui.lex" as ui

// State
checked   = false
toggled   = false
radioVal  = "A"
sliderVal = 40.0
numVal    = 5.0
buf       = ""
activeTab = 0
dropIdx   = 0
listIdx   = 0
progress  = 0.0
clicks    = 0

dropItems = ["Option Alpha", "Option Beta", "Option Gamma", "Option Delta", "Option Epsilon"]
listItems = ["Apple", "Banana", "Cherry", "Date", "Elderberry",
             "Fig", "Grape", "Honeydew", "Kiwi", "Lemon",
             "Mango", "Nectarine", "Orange", "Papaya", "Quince"]
tabLabels = ["Controls", "Layout", "Data", "Visual"]

window(980, 720, "kLex UI — Enhanced", fn(frame) {
    ui.background()
    progress = progress + 0.05
    if progress > 100.0 { progress = 0.0 }

    // Layout constants derived from current window size — adapts on resize
    ww   = float(winWidth())
    wh   = float(winHeight())
    pad  = 10.0
    tabH = 34.0
    tabY = pad + tabH + 4.0       // content starts below tab bar
    ch   = wh - tabY - pad        // content height
    colW = (ww - pad * 4.0) / 3.0 // three equal columns

    // ── Tab bar ──────────────────────────────────────────────────────────────
    activeTab = ui.tabs(pad, pad, ww - pad * 2.0, tabLabels, activeTab)

    // ── Tab 0: Controls ──────────────────────────────────────────────────────
    if activeTab == 0 {
        col1X = pad
        col2X = pad * 2.0 + colW
        col3X = pad * 3.0 + colW * 2.0

        // Left panel — input widgets
        ui.panelTitle(col1X, tabY, colW, ch, "Input Widgets")
        ui.beginLayout(col1X + 12.0, tabY + 38.0, colW - 24.0, 8.0)

        checked = ui.layoutCheckbox("Enable feature", checked)
        toggled = ui.layoutToggle("Dark mode", toggled)
        ui.layoutSeparator()

        ui.layoutLabel("Radio group:")
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option A", "A", radioVal)
        ui.advanceCursor(26.0)
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option B", "B", radioVal)
        ui.advanceCursor(26.0)
        radioVal = ui.radio(col1X + 12.0, ui.layoutY(), "Option C", "C", radioVal)
        ui.advanceCursor(26.0)
        ui.layoutSeparator()

        sliderVal = ui.layoutSlider("Volume", sliderVal, 0.0, 100.0)
        numVal    = ui.layoutNumericField("Count", numVal, 0.0, 20.0)
        ui.layoutSeparator()

        buf = ui.layoutTextField("Notes", buf)
        ui.layoutSeparator()

        if ui.layoutButton(130.0, 32.0, "Submit") {
            clicks = clicks + 1
        }
        ui.layoutLabelDim("Clicked " + str(clicks) + " times")

        // Middle panel — split into two vertically
        halfH = ch * 0.5 - 4.0

        ui.panelTitle(col2X, tabY, colW, halfH, "Live State")
        ui.beginLayout(col2X + 12.0, tabY + 38.0, colW - 24.0, 8.0)
        ui.layoutLabel("checked:  " + str(checked))
        ui.layoutLabel("toggled:  " + str(toggled))
        ui.layoutLabel("radio:    " + radioVal)
        ui.layoutLabel("slider:   " + str(int(sliderVal)))
        ui.layoutLabel("numeric:  " + str(int(numVal)))
        ui.layoutLabel("input:    " + buf)
        ui.layoutLabel("clicks:   " + str(clicks))
        ui.layoutSeparator()
        ui.layoutLabelDim("mouse " + str(int(mouseX())) + ", " + str(int(mouseY())))
        ui.layoutLabelDim("frame " + str(frame))

        // Middle panel — badges
        badgeY = tabY + halfH + 8.0
        ui.panelTitle(col2X, badgeY, colW, ch - halfH - 8.0, "Badges")
        ui.beginLayout(col2X + 12.0, badgeY + 38.0, colW - 24.0, 12.0)
        ui.layoutLabel("Status pills:")
        by = ui.layoutY()
        ui.badge(col2X + 12.0, by, "OK",    ui.UI_BADGE_GREEN)
        ui.badge(col2X + 56.0, by, "ERROR", ui.UI_BADGE_RED)
        ui.badge(col2X + 116.0, by, "WARN", ui.UI_BADGE_YELLOW)
        ui.badge(col2X + 168.0, by, "INFO", ui.UI_BADGE_BLUE)
        ui.advanceCursor(32.0)
        ui.layoutSeparator()
        ui.layoutLabel("Notification counts:")
        b2y = ui.layoutY()
        ui.badge(col2X + 12.0, b2y, "3 unread",  ui.UI_BADGE_BLUE)
        ui.badge(col2X + 96.0, b2y, "12 errors", ui.UI_BADGE_RED)
        ui.advanceCursor(32.0)

        // Right panel — selectors & visual
        ui.panelTitle(col3X, tabY, colW, ch, "Selectors & Visual")
        ui.beginLayout(col3X + 12.0, tabY + 38.0, colW - 24.0, 10.0)

        ui.layoutSubheading("Dropdown")
        dropIdx = ui.layoutDropdown("", dropItems, dropIdx)
        ui.layoutSeparator()

        ui.layoutSubheading("Color Swatches")
        ui.advanceCursor(6.0)
        sy = ui.layoutY()
        sw = (colW - 24.0) / 6.0
        for ci in range(0, 6) {
            colors = [[1.0, 0.3, 0.3], [0.3, 1.0, 0.4], [0.3, 0.5, 1.0],
                      [1.0, 0.8, 0.2], [0.8, 0.3, 1.0], [0.2, 0.9, 0.8]]
            c = colors[ci]
            ui.colorSwatch(col3X + 12.0 + float(ci) * sw, sy, sw - 3.0, c[0], c[1], c[2])
        }
        ui.advanceCursor(sw + 4.0)
        ui.layoutSeparator()

        ui.layoutSubheading("Spinner")
        ui.advanceCursor(6.0)
        spinY = ui.layoutY()
        ui.spinner(col3X + 30.0, spinY + 14.0, 14.0)
        ui.label(col3X + 54.0, spinY + 6.0, "Processing...")
        ui.advanceCursor(38.0)
        ui.layoutSeparator()

        ui.layoutSubheading("Progress")
        ui.layoutProgressBar("Upload", progress, 100.0)
    }

    // ── Tab 1: Auto Layout ───────────────────────────────────────────────────
    if activeTab == 1 {
        panW = ww * 0.45
        ui.panelTitle(pad, tabY, panW, ch, "Auto Layout Demo")
        ui.beginLayout(pad + 12.0, tabY + 38.0, panW - 24.0, 10.0)

        ui.layoutHeading("Settings")
        ui.layoutSeparator()
        ui.layoutSubheading("Appearance")
        toggled  = ui.layoutToggle("Dark mode", toggled)
        checked  = ui.layoutCheckbox("Show tooltips", checked)
        ui.layoutSeparator()

        ui.layoutSubheading("Audio")
        sliderVal = ui.layoutSlider("Volume", sliderVal, 0.0, 100.0)
        numVal    = ui.layoutNumericField("Channels", numVal, 1.0, 8.0)
        ui.layoutSeparator()

        ui.layoutSubheading("Account")
        buf     = ui.layoutTextField("Username", buf)
        dropIdx = ui.layoutDropdown("Region", dropItems, dropIdx)
        ui.layoutSeparator()

        ui.layoutProgressBar("Sync progress", progress, 100.0)
        ui.layoutSeparator()

        if ui.layoutButton(160.0, 32.0, "Save Settings") {
            clicks = clicks + 1
        }
        ui.layoutLabelDim("No manual y positions — all auto-layout.")
    }

    // ── Tab 2: Data ──────────────────────────────────────────────────────────
    if activeTab == 2 {
        listW   = ww * 0.38
        detailX = pad * 2.0 + listW
        detailW = ww - detailX - pad

        ui.panelTitle(pad, tabY, listW, ch, "List Box")
        ui.beginLayout(pad + 12.0, tabY + 38.0, listW - 24.0, 8.0)
        ui.layoutLabel("Select a fruit (UP/DOWN to scroll):")
        listIdx = ui.listBox(pad + 12.0, ui.layoutY(), listW - 24.0, ch - (ui.layoutY() - tabY) - 12.0, listItems, listIdx)

        ui.panelTitle(detailX, tabY, detailW, ch * 0.42, "Selection")
        ui.beginLayout(detailX + 12.0, tabY + 38.0, detailW - 24.0, 8.0)
        ui.layoutLabel("Index:  " + str(listIdx))
        if listIdx >= 0 && listIdx < len(listItems) {
            ui.layoutLabel("Value:  " + listItems[listIdx])
        }
        ui.layoutSeparator()
        ui.layoutLabelDim("UP/DOWN or drag scrollbar")

        detailY2 = tabY + ch * 0.42 + 8.0
        detailH2 = ch - ch * 0.42 - 8.0
        ui.panelTitle(detailX, detailY2, detailW, detailH2, "Progress Bars")
        ui.beginLayout(detailX + 12.0, detailY2 + 38.0, detailW - 24.0, 12.0)
        ui.layoutProgressBar("Upload",   progress,                    100.0)
        ui.layoutProgressBar("Download", mod(int(progress) + 30, 100), 100)
        ui.layoutProgressBar("Sync",     mod(int(progress) + 60, 100), 100)
        ui.layoutSeparator()
        spinY2 = ui.layoutY()
        ui.spinner(detailX + 24.0, spinY2 + 14.0, 12.0)
        ui.label(detailX + 46.0, spinY2 + 6.0, "Loading...")
    }

    // ── Tab 3: Visual ────────────────────────────────────────────────────────
    if activeTab == 3 {
        halfW = (ww - pad * 3.0) * 0.5

        ui.panelTitle(pad, tabY, halfW, ch * 0.55, "Cards & Shadow")
        cardW = (halfW - 40.0) * 0.5
        ui.card(pad + 14.0, tabY + 42.0, cardW, ch * 0.38)
        ui.beginLayout(pad + 24.0, tabY + 56.0, cardW - 20.0, 8.0)
        ui.layoutLabel("This is a card.")
        ui.layoutLabelDim("Drop shadow beneath.")
        ui.layoutSeparator()
        if ui.layoutButton(100.0, 28.0, "Action") { clicks = clicks + 1 }

        ui.card(pad + 20.0 + cardW, tabY + 42.0, cardW, ch * 0.38)
        ui.beginLayout(pad + 30.0 + cardW, tabY + 56.0, cardW - 20.0, 8.0)
        ui.layoutLabel("Another card.")
        ui.layoutLabelDim("Side by side.")
        ui.layoutSeparator()
        ui.layoutLabel("clicks: " + str(clicks))

        typY = tabY + ch * 0.55 + 8.0
        ui.panelTitle(pad, typY, halfW, ch - ch * 0.55 - 8.0, "Typography")
        ui.beginLayout(pad + 12.0, typY + 38.0, halfW - 24.0, 6.0)
        ui.layoutHeading("Heading  (2×)")
        ui.layoutSubheading("Subheading  (1.5×)")
        ui.layoutLabel("Regular label")
        ui.layoutLabelDim("Dimmed / hint text")
        ui.layoutSeparator()
        ui.dividerLabel(pad + 12.0, ui.layoutY(), halfW - 24.0, "divider")
        ui.advanceCursor(18.0)
        ui.labelCenter(pad + 12.0, ui.layoutY(), halfW - 24.0, "Centred text")
        ui.advanceCursor(22.0)
        ui.labelRight(pad + 12.0, ui.layoutY(), halfW - 24.0, "Right aligned")

        // Right half — tooltip demo
        rx = pad * 2.0 + halfW
        ui.panelTitle(rx, tabY, halfW, ch, "Tooltip Demo")
        ui.beginLayout(rx + 12.0, tabY + 38.0, halfW - 24.0, 12.0)
        ui.layoutSubheading("Hover for tooltips")
        ui.layoutSeparator()
        if ui.layoutButton(130.0, 32.0, "Save") { }
        if ui.layoutButton(130.0, 32.0, "Delete") { }
        if ui.layoutButton(130.0, 32.0, "Export") { }
        ui.layoutSeparator()
        ui.layoutLabelDim("Tooltips always render last.")

        // Tooltips — always last in draw order
        ui.tooltip(rx + 12.0, tabY + 90.0, halfW - 24.0, 36.0, "Subheadings scale at 1.5×")
        ui.tooltip(rx + 12.0, tabY + 154.0, 130.0, 32.0, "Saves current document")
        ui.tooltip(rx + 12.0, tabY + 200.0, 130.0, 32.0, "Cannot be undone!")
        ui.tooltip(rx + 12.0, tabY + 246.0, 130.0, 32.0, "Export to JSON or CSV")
    }
})
