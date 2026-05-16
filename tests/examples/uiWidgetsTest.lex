// Demonstrates every builtin UI widget in one window.
// Original widgets: uiBegin, uiEnd, label, button, textInput,
//                   checkbox, slider, progressBar, dropdown, list,
//                   tabs, textArea, toggle, radio, numericStepper
// New widgets:      table, contextMenu, colorPicker,
//                   treeView, scrollArea

name       = ""
email      = ""
notes      = ""
checked    = false
darkMode   = true
notifs     = false
vol        = 50.0
workers    = 4
progress   = 0.0
dropSel    = ""
listSel    = ""
submitted  = false
clicks     = 0
activeTab  = 0
memo       = ""
priority   = "Medium"

categories = ["Bug Report", "Feature Request", "Question", "Documentation", "Other"]
fruits     = ["Apple", "Banana", "Cherry", "Date", "Elderberry",
              "Fig", "Grape", "Honeydew", "Kiwi", "Lemon",
              "Mango", "Nectarine", "Orange", "Papaya"]
tabLabels  = ["Inputs", "Selection", "Progress", "Notes", "Table", "Advanced"]

// Table data
tableHeaders = ["Name", "Role", "Status"]
tableRows    = [
    ["Alice",   "Engineer",   "Active"],
    ["Bob",     "Designer",   "Active"],
    ["Carol",   "Manager",    "Away"],
    ["Dave",    "QA",         "Active"],
    ["Eve",     "DevOps",     "Inactive"],
    ["Frank",   "Engineer",   "Active"],
    ["Grace",   "Designer",   "Away"],
    ["Hank",    "Manager",    "Active"]
]
tableSelected = 0

// Context menu
menuOpen = false
menuX    = 0
menuY    = 0
menuItems = ["Edit", "Duplicate", "Delete", "---", "Properties"]
menuResult = -1

// Color picker
cpR = 0.45
cpG = 0.72
cpB = 1.0
cpA = 1.0
cpResult = [0.45, 0.72, 1.0, 1.0]

// Tree view
treeLabels   = ["Documents", "Projects", "kLex", "src", "tests",
                "stdlib", "Notes", "Archive", "2024", "2025"]
treeLevels   = [0, 0, 1, 2, 2, 2, 0, 0, 1, 1]
treeExpanded = [false, true, true, false, false, false, false, true, false, false]
treeSel      = 0

// Scroll area
scrollOffset = 0.0
scrollItems  = ["Item 01", "Item 02", "Item 03", "Item 04", "Item 05",
                "Item 06", "Item 07", "Item 08", "Item 09", "Item 10",
                "Item 11", "Item 12", "Item 13", "Item 14", "Item 15",
                "Item 16", "Item 17", "Item 18", "Item 19", "Item 20"]

window(980, 720, "kLex — Builtin UI Widgets", fn(frame) {
    background(0.12, 0.12, 0.12)

    uiBegin()

    // ── Tab bar ───────────────────────────────────────────────────────────────
    activeTab = tabs(20, 14, 940, tabLabels, activeTab, 0.65)

    // ── Tab 0: Inputs ─────────────────────────────────────────────────────────
    if activeTab == 0 {
        label("TEXT FIELDS", 30, 72, 0.75)
        name  = textInput("Name",  name,  190, 96,  320, 32, 0.65)
        email = textInput("Email", email, 190, 140, 320, 32, 0.65)
        notes = textInput("Notes", notes, 190, 184, 320, 32, 0.65)

        checked = checkbox("Subscribe to newsletter", 30, 238, checked, 0.65)

        label("TOGGLES", 30, 280, 0.75)
        darkMode = toggle("Dark mode",     30, 304, darkMode, 0.65)
        notifs   = toggle("Notifications", 30, 340, notifs,   0.65)

        label("PRIORITY", 30, 384, 0.75)
        priority = radio("Low",    30,  408, "Low",    priority, 0.65)
        priority = radio("Medium", 180, 408, "Medium", priority, 0.65)
        priority = radio("High",   360, 408, "High",   priority, 0.65)

        label("VOLUME", 30, 452, 0.75)
        vol = slider("", 30, 478, 400, vol, 0, 100, 0.65)

        label("WORKERS", 30, 522, 0.75)
        workers = numericStepper("", 30, 546, 200, workers, 1, 16, 0.65)

        label("name: "     + name,           570, 96,  0.60)
        label("email: "    + email,          570, 114, 0.60)
        label("checked: "  + str(checked),   570, 132, 0.60)
        label("darkMode: " + str(darkMode),  570, 150, 0.60)
        label("notifs: "   + str(notifs),    570, 168, 0.60)
        label("priority: " + priority,       570, 186, 0.60)
        label("volume: "   + str(int(vol)),  570, 204, 0.60)
        label("workers: "  + str(workers),   570, 222, 0.60)

        if button("Clear", 570, 270, 140, 36, 0.65) {
            name     = ""
            email    = ""
            notes    = ""
            checked  = false
            darkMode = true
            notifs   = false
            vol      = 50.0
            workers  = 4
            priority = "Medium"
        }
    }

    // ── Tab 1: Selection ──────────────────────────────────────────────────────
    if activeTab == 1 {
        label("DROPDOWN", 30, 72, 0.75)
        label("Category", 30, 100, 0.60)
        dropSel = dropdown("", categories, 30, 118, 380, 0.65)

        label("LIST", 30, 192, 0.75)
        listSel = list("Pick a fruit", fruits, 30, 210, 500, 340, 0.65)

        label("selected dropdown: " + dropSel, 570, 134, 0.60)
        label("selected list:     " + listSel, 570, 152, 0.60)
    }

    // ── Tab 2: Progress ───────────────────────────────────────────────────────
    if activeTab == 2 {
        progress = progress + 0.25
        if progress > 100.0 { progress = 0.0 }

        label("AUTO-ADVANCING PROGRESS BAR", 30, 72, 0.75)
        progressBar(30, 102, 880, 24, progress, 100)

        if button("Submit", 30, 162, 160, 40, 0.70) {
            submitted = true
            clicks    = clicks + 1
        }
        if button("Reset", 210, 162, 160, 40, 0.70) {
            submitted = false
        }

        label("submitted: " + str(submitted), 30, 232, 0.60)
        label("clicks:    " + str(clicks),    30, 250, 0.60)
    }

    // ── Tab 3: Notes ──────────────────────────────────────────────────────────
    if activeTab == 3 {
        label("MULTILINE TEXT AREA — click to focus, Enter for new line", 30, 72, 0.60)
        memo = textArea("", memo, 30, 100, 880, 480, 0.65)
    }

    // ── Tab 4: Table ──────────────────────────────────────────────────────────
    if activeTab == 4 {
        label("DATA TABLE — click a row to select", 30, 72, 0.75)
        tableSelected = table(tableHeaders, tableRows, 30, 98, 600, 380, 0.65)

        label("Selected row: " + str(tableSelected), 660, 98, 0.60)
        if tableSelected >= 0 && tableSelected < len(tableRows) {
            row = tableRows[tableSelected]
            label("Name:   " + row[0], 660, 120, 0.60)
            label("Role:   " + row[1], 660, 138, 0.60)
            label("Status: " + row[2], 660, 156, 0.60)
        }

    }

    // ── Tab 5: Advanced ───────────────────────────────────────────────────────
    if activeTab == 5 {

        // COLOR PICKER (left column)
        label("COLOR PICKER", 30, 72, 0.75)
        cpResult = colorPicker(30, 98, 340, cpR, cpG, cpB, cpA)
        cpR = cpResult[0]
        cpG = cpResult[1]
        cpB = cpResult[2]
        cpA = cpResult[3]

        // TREE VIEW (middle column)
        label("TREE VIEW — click +/- to expand", 420, 72, 0.75)
        tvResult     = treeView(420, 98, 260, 360, treeLabels, treeLevels, treeExpanded, 0.65)
        treeSel      = tvResult[0]
        treeExpanded = tvResult[1]
        label("selected: " + str(treeSel), 420, 470, 0.60)
        if treeSel >= 0 && treeSel < len(treeLabels) {
            label(treeLabels[treeSel], 420, 488, 0.60)
        }

        // SCROLL AREA (right column)
        label("SCROLL AREA — mouse wheel to scroll", 700, 72, 0.75)
        itemH       = 24.0
        contentH    = float(len(scrollItems)) * itemH
        scrollOffset = scrollArea(700, 98, 230, 360, contentH)
        pushClip(700, 98, 230, 360)
        i = 0
        while i < len(scrollItems) {
            iy = 98.0 + float(i) * itemH - scrollOffset
            if iy >= 98.0 && iy < 458.0 {
                label(scrollItems[i], 710, int(iy) + 4, 0.60)
            }
            i = i + 1
        }
        popClip()

        // CONTEXT MENU — right-click anywhere in the bottom area
        label("Right-click below for context menu", 30, 500, 0.60)
        if button("Open Context Menu", 30, 524, 200, 36, 0.65) {
            menuOpen = true
            menuX    = 30
            menuY    = 570
        }

        if menuOpen {
            menuResult = contextMenu(menuX, menuY, menuItems, menuOpen, 0.65)
            if menuResult >= 0 {
                label("Selected: " + menuItems[menuResult], 250, 534, 0.60)
                menuOpen = false
            }
            if menuResult == -2 {
                menuOpen = false
            }
        }
    }

    uiEnd()
})
