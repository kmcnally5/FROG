// stdlib/ui.lex — immediate-mode 2D UI library for kLex
//
// NEBULA EDITION — deep space violet + ice cyan theme
// Rounded-rectangle edition: all interactive widgets use smooth corners.
//
// OVERLAY RULE: dropdown and tooltip must be called AFTER other widgets.


// ----------------------------------------------------------------------------
// Theme  —  Nebula (deep space violet / ice cyan)
// ----------------------------------------------------------------------------

let UI_BG        = [0.00, 0.00, 0.02, 1.0]
let UI_PANEL     = [0.05, 0.03, 0.10, 1.0]
let UI_BORDER    = [0.38, 0.18, 0.68, 1.0]
let UI_BTN       = [0.08, 0.05, 0.16, 1.0]
let UI_BTN_HOVER = [0.18, 0.10, 0.36, 1.0]
let UI_BTN_PRESS = [0.05, 0.03, 0.10, 1.0]
let UI_ACCENT    = [0.45, 0.85, 1.00, 1.0]
let UI_TEXT      = [0.92, 0.88, 1.00, 1.0]
let UI_TEXT_DIM  = [0.48, 0.40, 0.65, 1.0]
let UI_SCALE     = 0.75

let UI_BADGE_GREEN  = [0.08, 0.55, 0.22, 1.0]
let UI_BADGE_RED    = [0.65, 0.10, 0.15, 1.0]
let UI_BADGE_YELLOW = [0.62, 0.50, 0.05, 1.0]
let UI_BADGE_BLUE   = [0.10, 0.35, 0.85, 1.0]
let UI_BADGE_GREY   = [0.25, 0.20, 0.38, 1.0]


// ----------------------------------------------------------------------------
// Internal state
// ----------------------------------------------------------------------------

let _uiFocus         = ""
let _uiDropdownOpen  = ""
let _uiScrollOffsets = {}

let _uiLayoutX    = 0.0
let _uiLayoutY    = 0.0
let _uiLayoutW    = 200.0
let _uiLayoutGap  = 6.0
let _uiLayoutCurY = 0.0


// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

fn _fill(c) {
    fill(c[0], c[1], c[2], c[3])
}

fn _stroke(c) {
    stroke(c[0], c[1], c[2], c[3])
}

fn _over(x, y, w, h) {
    return mouseX() >= x && mouseX() <= x + w &&
           mouseY() >= y && mouseY() <= y + h
}

fn _id(x, y) {
    return str(int(x)) + "," + str(int(y))
}

fn _charW() {
    return float(fontCharWidth()) * UI_SCALE
}

fn _charH() {
    return float(fontCharHeight()) * UI_SCALE
}

fn _widgetH() {
    return _charH() + 10.0
}

fn _textX(x, w, s) {
    return x + (w - float(len(s)) * _charW()) * 0.5
}

fn _textY(y, h) {
    return y + (h - _charH()) * 0.5
}

// _rrect — rounded rectangle using a 24-vertex polygon (6 arc segments per
// corner). Respects current fill and stroke state, just like rect().
fn _rrect(x, y, w, h, r) {
    roundedRect(x, y, w, h, r)
}

// Three-layer soft shadow under a rounded rect
fn _softShadow(x, y, w, h, r) {
    noStroke()
    fill(0.0, 0.0, 0.0, 0.20)
    _rrect(x + 2.0, y + 3.0, w, h, r)
    fill(0.0, 0.0, 0.0, 0.10)
    _rrect(x + 4.0, y + 6.0, w, h, r)
    fill(0.0, 0.0, 0.0, 0.05)
    _rrect(x + 6.0, y + 9.0, w, h, r)
}

// Three-layer phosphor glow
fn _glow(x, y, w, h) {
    noStroke()
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.10)
    _rrect(x - 3.0, y - 3.0, w + 6.0, h + 6.0, 8.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.05)
    _rrect(x - 8.0, y - 8.0, w + 16.0, h + 16.0, 12.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.02)
    _rrect(x - 14.0, y - 14.0, w + 28.0, h + 28.0, 18.0)
}

// Bright L-shaped bracket corners — FROG terminal identity
fn _corners(x, y, w, h) {
    const cs = 10.0
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.92)
    noStroke()
    rect(x,               y,               cs,  1.0)
    rect(x,               y,               1.0, cs)
    rect(x + w - cs,      y,               cs,  1.0)
    rect(x + w - 1.0,     y,               1.0, cs)
    rect(x,               y + h - 1.0,     cs,  1.0)
    rect(x,               y + h - cs,      1.0, cs)
    rect(x + w - cs,      y + h - 1.0,     cs,  1.0)
    rect(x + w - 1.0,     y + h - cs,      1.0, cs)
}


// ----------------------------------------------------------------------------
// Background
// ----------------------------------------------------------------------------

fn background() {
    _fill(UI_BG)
    noStroke()
    rect(0.0, 0.0, float(winWidth()), float(winHeight()))
    // Subtle scanlines
    fill(0.0, 0.0, 0.0, 0.10)
    ww = float(winWidth())
    i  = 0
    while i < int(float(winHeight()) / 4.0) {
        rect(0.0, float(i) * 4.0 + 2.0, ww, 1.0)
        i = i + 1
    }
}


// ----------------------------------------------------------------------------
// Typography
// ----------------------------------------------------------------------------

fn label(x, y, s) {
    _fill(UI_TEXT)
    noStroke()
    text(s, x, y, UI_SCALE)
}

fn labelDim(x, y, s) {
    _fill(UI_TEXT_DIM)
    noStroke()
    text(s, x, y, UI_SCALE)
}

fn labelColored(x, y, s, color) {
    _fill(color)
    noStroke()
    text(s, x, y, UI_SCALE)
}

fn labelRight(x, y, w, s) {
    _fill(UI_TEXT)
    noStroke()
    text(s, x + w - float(len(s)) * _charW(), y, UI_SCALE)
}

fn labelCenter(x, y, w, s) {
    _fill(UI_TEXT)
    noStroke()
    text(s, _textX(x, w, s), y, UI_SCALE)
}

fn heading(x, y, s) {
    _fill(UI_ACCENT)
    noStroke()
    text(s, x, y, UI_SCALE * 2.0)
}

fn subheading(x, y, s) {
    _fill(UI_TEXT)
    noStroke()
    text(s, x, y, UI_SCALE * 1.5)
}


// ----------------------------------------------------------------------------
// Layout
// ----------------------------------------------------------------------------

fn separator(x, y, w) {
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.22)
    noFill()
    line(x, y, x + w, y)
}

fn dividerLabel(x, y, w, lbl) {
    labelW = float(len(lbl)) * _charW() + 16.0
    midX   = x + (w - labelW) * 0.5
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.22)
    line(x, y + _charH() * 0.5, midX - 4.0, y + _charH() * 0.5)
    line(midX + labelW + 4.0, y + _charH() * 0.5, x + w, y + _charH() * 0.5)
    _fill(UI_TEXT_DIM)
    noStroke()
    text(lbl, midX + 8.0, y, UI_SCALE)
}


// ----------------------------------------------------------------------------
// Panel / Container
// ----------------------------------------------------------------------------

fn panel(x, y, w, h) {
    const r = 6.0
    _softShadow(x, y, w, h, r)
    _fill(UI_PANEL)
    _stroke(UI_BORDER)
    _rrect(x, y, w, h, r)
    // Top accent sheen
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.20)
    noStroke()
    rect(x + r, y + 1.0, w - r * 2.0, 1.0)
    _corners(x, y, w, h)
}

fn panelTitle(x, y, w, h, title) {
    const r = 6.0
    _softShadow(x, y, w, h, r)
    _fill(UI_PANEL)
    _stroke(UI_BORDER)
    _rrect(x, y, w, h, r)
    // Title bar — flat rect inset so corners are hidden by the rounded panel
    fill(0.08, 0.04, 0.16, 1.0)
    noStroke()
    rect(x + 1.0, y + 1.0, w - 2.0, 26.0)
    // Full-brightness accent line at very top
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 1.00)
    rect(x + r, y + 1.0, w - r * 2.0, 1.0)
    // Subtle sheen below it
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.16)
    rect(x + 1.0, y + 2.0, w - 2.0, 4.0)
    // Terminal prompt
    _fill(UI_ACCENT)
    noStroke()
    text(">_ " + title, x + 10.0, y + (28.0 - _charH()) * 0.5, UI_SCALE)
    _corners(x, y, w, h)
}

fn shadow(x, y, w, h) {
    _softShadow(x, y, w, h, 6.0)
}

fn card(x, y, w, h) {
    shadow(x, y, w, h)
    panel(x, y, w, h)
}


// ----------------------------------------------------------------------------
// Button
// ----------------------------------------------------------------------------

fn button(x, y, w, h, lbl) {
    const r = 4.0
    over    = _over(x, y, w, h)
    pressed = over && mouseDown()
    if over { _glow(x, y, w, h) }
    if pressed {
        _fill(UI_BTN_PRESS)
        _stroke(UI_BORDER)
    } else if over {
        _fill(UI_BTN_HOVER)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.75)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    _rrect(x, y, w, h, r)
    // Glass highlight on top half
    if !pressed {
        fill(1.0, 1.0, 1.0, 0.06)
        noStroke()
        _rrect(x + 1.0, y + 1.0, w - 2.0, (h - 2.0) * 0.5, r - 1.0)
    }
    if over && !pressed {
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.18)
        noStroke()
        rect(x + r, y + 1.0, w - r * 2.0, 2.0)
    }
    if over {
        _fill(UI_ACCENT)
    } else {
        _fill(UI_TEXT)
    }
    noStroke()
    text(lbl, _textX(x, w, lbl), _textY(y, h), UI_SCALE)
    return over && mouseClicked()
}


// ----------------------------------------------------------------------------
// Toggle switch  (pill-shaped track)
// ----------------------------------------------------------------------------

fn toggle(x, y, lbl, on) {
    const tw = 40.0
    th  = _widgetH()
    r   = th * 0.5         // full pill shape
    hr  = th * 0.34
    over = _over(x, y, tw, th)
    if over && on { _glow(x, y, tw, th) }
    if on {
        fill(0.12, 0.06, 0.26, 1.0)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.80)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    _rrect(x, y, tw, th, r)
    // Handle glow + dot
    if on {
        hx = x + tw - hr - 4.0
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.18)
        noStroke()
        circle(hx, y + th * 0.5, hr + 3.0)
        _fill(UI_ACCENT)
    } else {
        hx = x + hr + 4.0
        _fill(UI_TEXT_DIM)
    }
    noStroke()
    circle(hx, y + th * 0.5, hr)
    _fill(UI_TEXT)
    text(lbl, x + tw + 10.0, _textY(y, th), UI_SCALE)
    if over && mouseClicked() {
        return !on
    }
    return on
}


// ----------------------------------------------------------------------------
// Checkbox
// ----------------------------------------------------------------------------

fn checkbox(x, y, lbl, checked) {
    const size = 16.0
    const r    = 3.0
    over = _over(x, y, size, size)
    if checked { _glow(x, y, size, size) }
    if checked {
        fill(0.12, 0.06, 0.26, 1.0)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.85)
    } else if over {
        _fill(UI_BTN_HOVER)
        _stroke(UI_BORDER)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    _rrect(x, y, size, size, r)
    if checked {
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 1.0)
        strokeWeight(2.0)
        line(x + 3.0, y + 8.0,  x + 7.0,  y + 12.0)
        line(x + 7.0, y + 12.0, x + 13.0, y + 4.0)
        strokeWeight(1.0)
    }
    if checked {
        _fill(UI_ACCENT)
    } else {
        _fill(UI_TEXT)
    }
    noStroke()
    text(lbl, x + size + 8.0, y + (size - _charH()) * 0.5, UI_SCALE)
    if over && mouseClicked() {
        return !checked
    }
    return checked
}


// ----------------------------------------------------------------------------
// Radio button
// ----------------------------------------------------------------------------

fn radio(x, y, lbl, myValue, currentValue) {
    const r  = 8.0
    cx       = x + r
    cy       = y + r
    selected = currentValue == myValue
    hitW     = r * 2.0 + float(len(lbl)) * _charW() + 12.0
    over     = _over(x, y - r, hitW, r * 2.0)
    if selected { _glow(x - r, y - r, r * 2.0, r * 2.0) }
    if selected {
        fill(0.12, 0.06, 0.26, 1.0)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.85)
    } else if over {
        _fill(UI_BTN_HOVER)
        _stroke(UI_BORDER)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    circle(cx, cy, r)
    if selected {
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 1.0)
        noStroke()
        circle(cx, cy, r * 0.40)
    }
    if selected {
        _fill(UI_ACCENT)
    } else {
        _fill(UI_TEXT)
    }
    noStroke()
    text(lbl, x + r * 2.0 + 8.0, y - 1.0, UI_SCALE)
    if over && mouseClicked() {
        return myValue
    }
    return currentValue
}


// ----------------------------------------------------------------------------
// Slider  (capsule track, glowing handle)
// ----------------------------------------------------------------------------

fn slider(x, y, w, value, minVal, maxVal) {
    const trackH  = 5.0
    const handleR = 8.0
    const r       = trackH * 0.5
    trackY = y + handleR - trackH * 0.5
    if _over(x - handleR, y, w + handleR * 2.0, handleR * 2.0) && mouseDown() {
        t = (mouseX() - x) / w
        if t < 0.0 { t = 0.0 }
        if t > 1.0 { t = 1.0 }
        value = float(minVal) + t * (float(maxVal) - float(minVal))
    }
    t = (float(value) - float(minVal)) / (float(maxVal) - float(minVal))
    // Track background — capsule
    _fill(UI_BTN)
    noStroke()
    _rrect(x, trackY, w, trackH, r)
    // Filled portion
    if t > 0.01 {
        fw = w * t
        fill(0.14, 0.08, 0.32, 1.0)
        if fw >= trackH {
            _rrect(x, trackY, fw, trackH, r)
        } else {
            rect(x, trackY, fw, trackH)
        }
        // Bright leading edge
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.90)
        rect(x + fw - 1.5, trackY - 1.0, 2.0, trackH + 2.0)
    }
    // Handle
    hx     = x + w * t
    active = _over(hx - handleR, y, handleR * 2.0, handleR * 2.0) || mouseDown()
    if active {
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.18)
        noStroke()
        circle(hx, y + handleR, handleR * 2.2)
        _fill(UI_ACCENT)
    } else {
        fill(0.30, 0.72, 0.95, 1.0)
    }
    noStroke()
    circle(hx, y + handleR, handleR)
    return value
}


// ----------------------------------------------------------------------------
// Numeric field
// ----------------------------------------------------------------------------

fn numericField(x, y, w, value, minVal, maxVal) {
    const btnW = 28.0
    h = _widgetH()
    if button(x, y, btnW, h, "-") {
        value = value - 1.0
        if value < float(minVal) { value = float(minVal) }
    }
    _fill(UI_PANEL)
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.30)
    _rrect(x + btnW, y, w - btnW * 2.0, h, 3.0)
    s = str(int(value))
    _fill(UI_ACCENT)
    noStroke()
    text(s, _textX(x + btnW, w - btnW * 2.0, s), _textY(y, h), UI_SCALE)
    if button(x + w - btnW, y, btnW, h, "+") {
        value = value + 1.0
        if value > float(maxVal) { value = float(maxVal) }
    }
    return value
}


// ----------------------------------------------------------------------------
// Progress bar  (capsule shape)
// ----------------------------------------------------------------------------

fn progressBar(x, y, w, h, value, maxVal) {
    r = h * 0.5
    // Background track
    _fill(UI_BTN)
    noStroke()
    _rrect(x, y, w, h, r)
    t = float(value) / float(maxVal)
    if t > 1.0 { t = 1.0 }
    if t < 0.0 { t = 0.0 }
    // Fill
    if t > 0.01 {
        fw = w * t
        fill(0.12, 0.06, 0.28, 1.0)
        if fw >= h {
            _rrect(x, y, fw, h, r)
        } else {
            rect(x, y, fw, h)
        }
        // Bright leading edge
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.95)
        rect(x + fw - 2.0, y, 2.0, h)
        // Top sheen
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.10)
        rect(x + r, y, fw - r, 2.0)
    }
    pct = int(t * 100.0)
    _fill(UI_TEXT_DIM)
    text(str(pct) + "%", x + w + 10.0, _textY(y, h), UI_SCALE)
}


// ----------------------------------------------------------------------------
// Text field
// ----------------------------------------------------------------------------

fn textField(x, y, w, buf) {
    const r = 4.0
    h = _widgetH()
    id      = _id(x, y)
    focused = _uiFocus == id
    if _over(x, y, w, h) && mouseClicked() {
        _uiFocus = id
        focused  = true
    } else if mouseClicked() {
        _uiFocus = ""
        focused  = false
    }
    if focused {
        typed = getTypedChars()
        if len(typed) > 0 {
            buf = buf + typed
        }
        if keyPressed("BACKSPACE") && len(buf) > 0 {
            buf = substr(buf, 0, len(buf) - 1)
        }
    }
    if focused {
        _glow(x, y, w, h)
        _fill(UI_PANEL)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.90)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    _rrect(x, y, w, h, r)
    // Inner shadow (recessed feel)
    fill(0.0, 0.0, 0.0, 0.15)
    noStroke()
    rect(x + r, y + 1.0, w - r * 2.0, 2.0)
    if focused {
        // Active bottom edge
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.50)
        rect(x + r, y + h - 2.0, w - r * 2.0, 1.0)
    }
    _fill(UI_TEXT)
    noStroke()
    text(buf, x + 8.0, _textY(y, h), UI_SCALE)
    if focused {
        cx = x + 8.0 + float(len(buf)) * _charW()
        if mod(int(elapsedTime() * 2.0), 2) == 0 {
            fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.95)
            rect(cx, y + 5.0, 2.0, h - 10.0)
        }
    }
    return buf
}


// ----------------------------------------------------------------------------
// Dropdown
// ----------------------------------------------------------------------------

fn dropdown(x, y, w, items, selectedIdx) {
    const r    = 4.0
    itemH = _widgetH()
    id     = _id(x, y)
    isOpen = _uiDropdownOpen == id
    if mouseClicked() {
        if _over(x, y, w, itemH) {
            if isOpen {
                _uiDropdownOpen = ""
            } else {
                _uiDropdownOpen = id
            }
            isOpen = !isOpen
        } else if isOpen {
            for i in range(0, len(items)) {
                iy = y + itemH + float(i) * itemH
                if _over(x, iy, w, itemH) {
                    selectedIdx = i
                }
            }
            _uiDropdownOpen = ""
            isOpen = false
        }
    }
    if isOpen {
        fill(0.10, 0.05, 0.22, 1.0)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.75)
    } else if _over(x, y, w, itemH) {
        _fill(UI_BTN_HOVER)
        stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.50)
    } else {
        _fill(UI_BTN)
        _stroke(UI_BORDER)
    }
    _rrect(x, y, w, itemH, r)
    if selectedIdx >= 0 && selectedIdx < len(items) {
        _fill(UI_TEXT)
        noStroke()
        text(items[selectedIdx], x + 10.0, _textY(y, itemH), UI_SCALE)
    }
    _fill(UI_ACCENT)
    text("v", x + w - _charW() - 10.0, _textY(y, itemH), UI_SCALE)
    if isOpen {
        // Drop shadow behind open list
        fill(0.0, 0.0, 0.0, 0.30)
        noStroke()
        _rrect(x + 2.0, y + itemH + 3.0, w, float(len(items)) * itemH, r)
        for i in range(0, len(items)) {
            iy = y + itemH + float(i) * itemH
            if i == selectedIdx {
                fill(0.12, 0.06, 0.26, 1.0)
                stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.40)
            } else if _over(x, iy, w, itemH) {
                _fill(UI_BTN_HOVER)
                stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.30)
            } else {
                fill(0.06, 0.04, 0.13, 1.0)
                _stroke(UI_BORDER)
            }
            if i == 0 {
                _rrect(x, iy, w, itemH, r)
            } else if i == len(items) - 1 {
                _rrect(x, iy, w, itemH, r)
            } else {
                rect(x, iy, w, itemH)
            }
            if i == selectedIdx {
                _fill(UI_ACCENT)
            } else {
                _fill(UI_TEXT)
            }
            noStroke()
            text(items[i], x + 10.0, _textY(iy, itemH), UI_SCALE)
        }
    }
    return selectedIdx
}


// ----------------------------------------------------------------------------
// Tab bar
// ----------------------------------------------------------------------------

fn tabs(x, y, w, tabLabels, activeTab) {
    const r = 4.0
    h    = _widgetH() + 6.0
    tabW = w / float(len(tabLabels))
    for i in range(0, len(tabLabels)) {
        tx = x + float(i) * tabW
        if i == activeTab {
            fill(0.10, 0.05, 0.22, 1.0)
            stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.50)
        } else if _over(tx, y, tabW, h) {
            _fill(UI_BTN_HOVER)
            _stroke(UI_BORDER)
        } else {
            _fill(UI_BTN)
            _stroke(UI_BORDER)
        }
        _rrect(tx, y, tabW, h, r)
        // Active underline
        if i == activeTab {
            fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.90)
            noStroke()
            rect(tx + r, y + h - 2.0, tabW - r * 2.0, 2.0)
            // Glass highlight
            fill(1.0, 1.0, 1.0, 0.06)
            _rrect(tx + 1.0, y + 1.0, tabW - 2.0, (h - 2.0) * 0.5, r - 1.0)
            _fill(UI_ACCENT)
        } else {
            _fill(UI_TEXT_DIM)
        }
        noStroke()
        lbl = tabLabels[i]
        text(lbl, _textX(tx, tabW, lbl), _textY(y, h), UI_SCALE)
        if _over(tx, y, tabW, h) && mouseClicked() {
            activeTab = i
        }
    }
    return activeTab
}


// ----------------------------------------------------------------------------
// List box
// ----------------------------------------------------------------------------

fn listBox(x, y, w, h, items, selectedIdx) {
    const r   = 6.0
    id        = _id(x, y)
    itemH     = _charH() + 8.0
    visible   = int(h / itemH)
    maxScroll = len(items) - visible
    if maxScroll < 0 { maxScroll = 0 }
    if !hasKey(_uiScrollOffsets, id) {
        _uiScrollOffsets[id] = 0
    }
    scrollOff = _uiScrollOffsets[id]
    if _over(x, y, w, h) {
        if keyPressed("UP") && scrollOff > 0 {
            scrollOff = scrollOff - 1
            _uiScrollOffsets[id] = scrollOff
        }
        if keyPressed("DOWN") && scrollOff < maxScroll {
            scrollOff = scrollOff + 1
            _uiScrollOffsets[id] = scrollOff
        }
    }
    // Background
    _softShadow(x, y, w, h, r)
    _fill(UI_PANEL)
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.25)
    _rrect(x, y, w, h, r)
    _corners(x, y, w, h)
    const sbW = 10.0
    itemW = w - sbW - 2.0
    for i in range(0, visible) {
        itemIdx = i + scrollOff
        if itemIdx >= len(items) { break }
        iy = y + float(i) * itemH
        if itemIdx == selectedIdx {
            fill(0.12, 0.06, 0.26, 1.0)
            noStroke()
            rect(x + 2.0, iy + 1.0, itemW - 4.0, itemH - 2.0)
            fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.90)
            rect(x + 2.0, iy + 1.0, 2.0, itemH - 2.0)
        } else if _over(x, iy, itemW, itemH) {
            _fill(UI_BTN_HOVER)
            noStroke()
            rect(x + 2.0, iy + 1.0, itemW - 4.0, itemH - 2.0)
            if mouseClicked() {
                selectedIdx = itemIdx
            }
        }
        if itemIdx == selectedIdx {
            _fill(UI_ACCENT)
        } else {
            _fill(UI_TEXT)
        }
        noStroke()
        text(items[itemIdx], x + 12.0, iy + (itemH - _charH()) * 0.5, UI_SCALE)
    }
    sbX = x + w - sbW
    fill(0.04, 0.02, 0.08, 1.0)
    noStroke()
    rect(sbX, y + 1.0, sbW - 1.0, h - 2.0)
    if maxScroll > 0 {
        thumbH = h * float(visible) / float(len(items))
        if thumbH < 14.0 { thumbH = 14.0 }
        thumbY = y + (h - thumbH) * float(scrollOff) / float(maxScroll)
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.40)
        _rrect(sbX + 2.0, thumbY, sbW - 4.0, thumbH, 2.0)
        if _over(sbX, y, sbW, h) && mouseDown() {
            newOff = int(float(maxScroll) * (mouseY() - y) / h)
            if newOff < 0         { newOff = 0 }
            if newOff > maxScroll { newOff = maxScroll }
            _uiScrollOffsets[id] = newOff
        }
    }
    return selectedIdx
}


// ----------------------------------------------------------------------------
// Spinner
// ----------------------------------------------------------------------------

fn spinner(x, y, r) {
    const segments = 12
    t = elapsedTime() * 5.0
    for i in range(0, segments) {
        angle = float(i) / float(segments) * 6.2832 - t
        alpha = float(i) / float(segments) * 0.90 + 0.10
        fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], alpha)
        noStroke()
        sx = x + cos(angle) * r
        sy = y + sin(angle) * r
        circle(sx, sy, r * 0.20)
    }
}


// ----------------------------------------------------------------------------
// Color swatch
// ----------------------------------------------------------------------------

fn colorSwatch(x, y, size, r, g, b) {
    fill(r, g, b, 1.0)
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.40)
    _rrect(x, y, size, size, 3.0)
}


// ----------------------------------------------------------------------------
// Badge  (pill shape)
// ----------------------------------------------------------------------------

fn badge(x, y, lbl, color) {
    bw = float(len(lbl)) * _charW() + 16.0
    bh = _charH() + 8.0
    _fill(color)
    noStroke()
    _rrect(x, y, bw, bh, bh * 0.5)
    // Subtle top sheen
    fill(1.0, 1.0, 1.0, 0.12)
    _rrect(x + 1.0, y + 1.0, bw - 2.0, bh * 0.5, bh * 0.5 - 1.0)
    _fill(UI_TEXT)
    noStroke()
    text(lbl, x + 8.0, y + 4.0, UI_SCALE)
}


// ----------------------------------------------------------------------------
// Tooltip
// ----------------------------------------------------------------------------

fn tooltip(x, y, w, h, msg) {
    if !_over(x, y, w, h) {
        return
    }
    const r  = 4.0
    tipW = float(len(msg)) * _charW() + 18.0
    tipH = _charH() + 14.0
    tx   = mouseX() + 16.0
    ty   = mouseY() - tipH - 8.0
    if tx + tipW > float(winWidth())  { tx = mouseX() - tipW - 8.0 }
    if ty < 0.0                       { ty = mouseY() + 20.0 }
    // Shadow
    fill(0.0, 0.0, 0.0, 0.45)
    noStroke()
    _rrect(tx + 3.0, ty + 3.0, tipW, tipH, r)
    // Body
    fill(0.06, 0.04, 0.14, 0.97)
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.80)
    _rrect(tx, ty, tipW, tipH, r)
    // Top accent line
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.60)
    noStroke()
    rect(tx + r, ty + 1.0, tipW - r * 2.0, 1.0)
    _fill(UI_TEXT)
    text(msg, tx + 9.0, ty + 7.0, UI_SCALE)
}


// ----------------------------------------------------------------------------
// Auto-layout system
// ----------------------------------------------------------------------------

fn beginLayout(x, y, w, gap) {
    _uiLayoutX    = x
    _uiLayoutY    = y
    _uiLayoutW    = w
    _uiLayoutGap  = gap
    _uiLayoutCurY = y
}

fn layoutY() {
    return _uiLayoutCurY
}

fn advanceCursor(h) {
    _uiLayoutCurY = _uiLayoutCurY + h + _uiLayoutGap
}

fn layoutLabel(s) {
    label(_uiLayoutX, _uiLayoutCurY, s)
    advanceCursor(_charH() + 2.0)
}

fn layoutLabelDim(s) {
    labelDim(_uiLayoutX, _uiLayoutCurY, s)
    advanceCursor(_charH() + 2.0)
}

fn layoutHeading(s) {
    heading(_uiLayoutX, _uiLayoutCurY, s)
    advanceCursor(_charH() * 2.0 + 6.0)
}

fn layoutSubheading(s) {
    subheading(_uiLayoutX, _uiLayoutCurY, s)
    advanceCursor(_charH() * 1.5 + 4.0)
}

fn layoutSeparator() {
    separator(_uiLayoutX, _uiLayoutCurY, _uiLayoutW)
    advanceCursor(10.0)
}

fn layoutButton(w, h, lbl) {
    result = button(_uiLayoutX, _uiLayoutCurY, w, h, lbl)
    advanceCursor(h)
    return result
}

fn layoutCheckbox(lbl, checked) {
    checked = checkbox(_uiLayoutX, _uiLayoutCurY, lbl, checked)
    advanceCursor(18.0)
    return checked
}

fn layoutToggle(lbl, on) {
    on = toggle(_uiLayoutX, _uiLayoutCurY, lbl, on)
    advanceCursor(_widgetH() + 4.0)
    return on
}

fn layoutSlider(lbl, value, minVal, maxVal) {
    label(_uiLayoutX, _uiLayoutCurY, lbl + ":  " + str(int(value)))
    advanceCursor(_charH() + 4.0)
    value = slider(_uiLayoutX, _uiLayoutCurY, _uiLayoutW, value, minVal, maxVal)
    advanceCursor(22.0)
    return value
}

fn layoutTextField(lbl, buf) {
    label(_uiLayoutX, _uiLayoutCurY, lbl)
    advanceCursor(_charH() + 4.0)
    buf = textField(_uiLayoutX, _uiLayoutCurY, _uiLayoutW, buf)
    advanceCursor(_widgetH() + 4.0)
    return buf
}

fn layoutProgressBar(lbl, value, maxVal) {
    label(_uiLayoutX, _uiLayoutCurY, lbl)
    advanceCursor(_charH() + 4.0)
    progressBar(_uiLayoutX, _uiLayoutCurY, _uiLayoutW - 44.0, 14.0, value, maxVal)
    advanceCursor(20.0)
}

fn layoutDropdown(lbl, items, selectedIdx) {
    if len(lbl) > 0 {
        label(_uiLayoutX, _uiLayoutCurY, lbl)
        advanceCursor(_charH() + 4.0)
    }
    selectedIdx = dropdown(_uiLayoutX, _uiLayoutCurY, _uiLayoutW, items, selectedIdx)
    advanceCursor(_widgetH() + 4.0)
    return selectedIdx
}

fn layoutNumericField(lbl, value, minVal, maxVal) {
    label(_uiLayoutX, _uiLayoutCurY, lbl)
    advanceCursor(_charH() + 4.0)
    value = numericField(_uiLayoutX, _uiLayoutCurY, _uiLayoutW, value, minVal, maxVal)
    advanceCursor(_widgetH() + 4.0)
    return value
}


// ----------------------------------------------------------------------------
// File manager panel  (optimized for file lists)
// ----------------------------------------------------------------------------

fn filePanel(x, y, w, h, title) {
    const r = 6.0
    _softShadow(x, y, w, h, r)
    _fill(UI_PANEL)
    _stroke(UI_BORDER)
    _rrect(x, y, w, h, r)
    // Title bar
    fill(0.08, 0.04, 0.16, 1.0)
    noStroke()
    rect(x + 1.0, y + 1.0, w - 2.0, 26.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 1.00)
    rect(x + r, y + 1.0, w - r * 2.0, 1.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.16)
    rect(x + 1.0, y + 2.0, w - 2.0, 4.0)
    _fill(UI_ACCENT)
    noStroke()
    text(">_ " + title, x + 10.0, y + (28.0 - _charH()) * 0.5, UI_SCALE)
    _corners(x, y, w, h)
}

fn filePanelFocused(x, y, w, h, title) {
    const r = 6.0
    _softShadow(x, y, w, h, r)
    _fill(UI_PANEL)
    stroke(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.90)
    _rrect(x, y, w, h, r)
    fill(0.08, 0.04, 0.16, 1.0)
    noStroke()
    rect(x + 1.0, y + 1.0, w - 2.0, 26.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 1.00)
    rect(x + r, y + 1.0, w - r * 2.0, 1.0)
    fill(UI_ACCENT[0], UI_ACCENT[1], UI_ACCENT[2], 0.20)
    rect(x + 1.0, y + 2.0, w - 2.0, 4.0)
    _fill(UI_ACCENT)
    noStroke()
    text(">_ " + title, x + 10.0, y + (28.0 - _charH()) * 0.5, UI_SCALE)
    _corners(x, y, w, h)
}
