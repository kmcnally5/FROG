// secretHunterUI.lex — graphical front-end for the Secret Hunter scan engine.
//
// Run: KLEX_PATH=. ./klex tests/examples/SecretHunter/secretHunterUI.lex

import "tests/examples/SecretHunter/secretHunterLib.lex" as sh
import "stdlib/ui_themes.lex" as themes

const SIDE_W = 290
const HDR_H  = 48

// ── Fonts ─────────────────────────────────────────────────────────────────────

fontResult, fontErr = safe(loadFont, ["/System/Library/Fonts/SFNS.ttf", 18])
if fontErr != null {
    fontResult = loadFont("/System/Library/Fonts/Supplemental/Arial Bold.ttf", 18)
}
uiFont = fontResult

// ── Theme ─────────────────────────────────────────────────────────────────────
// Swap themes.crimson() for themes.dark() / themes.midnight() / themes.forest()
// / themes.light() to instantly retheme the entire UI.

theme = themes.crimson()

// ── App state ─────────────────────────────────────────────────────────────────

scanPath    = "."
includeGit  = true
maxSizeMB   = 5.0
scanning    = false
scanStarted = false
progressCh  = null
resultCh    = null
findings    = makeArray(0)
fileCount   = 0
commitCount = 0
repoCount   = 0
totalSec    = 0.0
filesSec    = 0.0
gitSec      = 0.0
filesPerSec   = 0.0
commitsPerSec = 0.0
selectedFinding  = -1
lastClickRow     = -1
lastClickTime    = -1.0
scanPhase        = ""
scanTotal        = 0
scanDone         = 0
scanRepoCount    = 0

// ── Filter + tree-view state ──────────────────────────────────────────────────

SEV_OPTIONS    = ["ALL", "CRITICAL", "HIGH", "MEDIUM", "LOW"]
SRC_OPTIONS    = ["ALL", "FILES", "GIT"]
severityFilter = "ALL"
sourceFilter   = "ALL"
treeLabels     = makeArray(0)
treeLevels     = makeArray(0)
treeExpanded   = makeArray(0)
treeSelected   = 0
treeNodeFind   = makeArray(0)
treeKey        = ""

// ── Severity helpers ──────────────────────────────────────────────────────────

fn sevFill(sev) {
    if sev == "CRITICAL"    { fillC(theme["crit"]) }
    else if sev == "HIGH"   { fillC(theme["high"]) }
    else if sev == "MEDIUM" { fillC(theme["med"])  }
    else                    { fillC(theme["low"])  }
}

fn sevCounts(flist) {
    crit = 0   high = 0   med = 0   low = 0
    i = 0
    n = len(flist)
    while i < n {
        s = flist[i]["severity"]
        if s == "CRITICAL"    { crit = crit + 1 }
        else if s == "HIGH"   { high = high + 1 }
        else if s == "MEDIUM" { med  = med  + 1 }
        else                  { low  = low  + 1 }
        i = i + 1
    }
    return [crit, high, med, low]
}

fn threatLevel(counts) {
    if counts[0] > 0 { return "CRITICAL" }
    if counts[1] > 0 { return "HIGH" }
    if counts[2] > 0 { return "ELEVATED" }
    if counts[3] > 0 { return "LOW" }
    return "CLEAN"
}

fn buildFindingTree(flist, sevF, srcF) {
    n = len(flist)
    if n == 0 { return [makeArray(0), makeArray(0), makeArray(0)] }

    // Group by (file, source) so git and filesystem findings get separate parent nodes
    seenKeys  = makeArray(n * 2, "")
    seenFiles = makeArray(n * 2, "")
    seenSrcs  = makeArray(n * 2, "")
    seenCount = 0
    filtIdxs  = makeArray(n, 0)
    filtCount = 0

    i = 0
    while i < n {
        f     = flist[i]
        okSev = sevF == "ALL" || f["severity"] == sevF
        okSrc = srcF == "ALL" ||
                (srcF == "FILES" && f["source"] == "file") ||
                (srcF == "GIT"   && f["source"] == "git")
        if okSev && okSrc {
            filtIdxs[filtCount] = i
            filtCount = filtCount + 1
            fname    = f["file"]
            fsrc     = f["source"]
            groupKey = fname + "|" + fsrc
            isNew = true
            j = 0
            while j < seenCount {
                if seenKeys[j] == groupKey { isNew = false }
                j = j + 1
            }
            if isNew {
                seenKeys[seenCount]  = groupKey
                seenFiles[seenCount] = fname
                seenSrcs[seenCount]  = fsrc
                seenCount = seenCount + 1
            }
        }
        i = i + 1
    }

    if filtCount == 0 { return [makeArray(0), makeArray(0), makeArray(0)] }

    totalNodes = seenCount + filtCount
    labels   = makeArray(totalNodes, "")
    levels   = makeArray(totalNodes, 0)
    nodeFnd  = makeArray(totalNodes, -1)

    nodeIdx = 0
    fi = 0
    while fi < seenCount {
        fname    = seenFiles[fi]
        fsrc     = seenSrcs[fi]
        groupKey = seenKeys[fi]
        if fsrc == "git" {
            labels[nodeIdx] = "[GIT]   " + fname
        } else {
            labels[nodeIdx] = "[FILE]  " + fname
        }
        levels[nodeIdx]  = 0
        nodeFnd[nodeIdx] = -1
        nodeIdx = nodeIdx + 1
        ci = 0
        while ci < filtCount {
            origIdx = filtIdxs[ci]
            f = flist[origIdx]
            if f["file"] == fname && f["source"] == fsrc {
                if f["source"] == "file" {
                    locPart = "line " + str(f["line"])
                } else {
                    locPart = f["commit"]
                    if len(locPart) > 7 { locPart = substr(locPart, 0, 7) }
                }
                matchSnip = sh.fitText(f["match"], 44)
                labels[nodeIdx]  = "[" + f["severity"] + "]  " + f["patternName"] + "  —  " + locPart + "  " + matchSnip
                levels[nodeIdx]  = 1
                nodeFnd[nodeIdx] = origIdx
                nodeIdx = nodeIdx + 1
            }
            ci = ci + 1
        }
        fi = fi + 1
    }

    return [labels, levels, nodeFnd]
}

// ── Color helpers ─────────────────────────────────────────────────────────────

fn fillC(c)      { fill(c[0], c[1], c[2], c[3]) }
fn fillCA(c, a)  { fill(c[0], c[1], c[2], a)    }

// ── Drawing helpers ───────────────────────────────────────────────────────────

fn sideSep(y) {
    fillC(theme["sep"])
    noStroke()
    rect(12.0, float(y), 266.0, 1.0)
}

fn sectionLabel(txt, y) {
    fillC(theme["sectionAccent"])
    noStroke()
    roundedRect(10.0, float(y) + 1.0, 3.0, 14.0, 1.5)
    fillC(theme["sectionText"])
    textFont(uiFont, txt, 18, y, 0.62)
    tw = textWidth(uiFont, txt, 0.62)
    fillC(theme["sectionLine"])
    rect(18.0, float(y) + 16.0, tw, 1.0)
}

fn glowWhite(fnt, txt, x, y, sc) {
    fillC(theme["titleGlow"])
    textFont(fnt, txt, x - 2, y - 1, sc)
    textFont(fnt, txt, x + 2, y + 1, sc)
    textFont(fnt, txt, x - 1, y + 2, sc)
    textFont(fnt, txt, x + 1, y - 2, sc)
    fillC(theme["titleText"])
    textFont(fnt, txt, x, y, sc)
}

fn glowRed(fnt, txt, x, y, sc) {
    fillCA(theme["crit"], 0.18)
    textFont(fnt, txt, x - 2, y - 1, sc)
    textFont(fnt, txt, x + 2, y + 1, sc)
    textFont(fnt, txt, x - 1, y + 2, sc)
    textFont(fnt, txt, x + 1, y - 2, sc)
    fillC(theme["crit"])
    textFont(fnt, txt, x, y, sc)
}

fn drawCentred(fnt, txt, cx, y, sc) {
    w = textWidth(fnt, txt, sc)
    textFont(fnt, txt, cx - w / 2, y, sc)
}

fn drawRight(fnt, txt, rightX, y, sc) {
    w = textWidth(fnt, txt, sc)
    textFont(fnt, txt, rightX - w, y, sc)
}

// Severity pill sized to actual text
fn drawSevPill(sev, bx, by) {
    sc = 0.60
    tw = textWidth(uiFont, sev, sc)
    bw = tw + 16.0
    bh = 18.0
    br = bh * 0.5
    sevFill(sev)
    noStroke()
    rect(bx + br, by, bw - br * 2.0, bh)
    circle(bx + br,       by + br, br)
    circle(bx + bw - br,  by + br, br)
    fillC(theme["pillText"])
    textFont(uiFont, sev, bx + 8.0, by + 1.0, sc)
    return bw
}

// Source tag pill
fn drawSourceTag(tag, tx, ty) {
    sc = 0.52
    tw = textWidth(uiFont, tag, sc)
    bw = tw + 10.0
    bh = 14.0
    br = bh * 0.5
    fillC(theme["sourceTagBg"])
    noStroke()
    rect(tx + br, ty, bw - br * 2.0, bh)
    circle(tx + br,       ty + br, br)
    circle(tx + bw - br,  ty + br, br)
    fillC(theme["sourceTagText"])
    textFont(uiFont, tag, tx + 5.0, ty + 1.0, sc)
    return bw
}

// Card-style severity row for sidebar results
fn sevCard(sev, count, y) {
    fillC(theme["cardBg"])
    noStroke()
    roundedRect(10.0, float(y), 270.0, 27.0, 4.0)
    sevFill(sev)
    noStroke()
    roundedRect(10.0, float(y), 5.0, 27.0, 2.5)
    fillC(theme["cardText"])
    textFont(uiFont, sev, 24, y + 5, 0.65)
    fillC(theme["cardCount"])
    drawRight(uiFont, str(count), 276.0, float(y) + 5.0, 0.68)
}

// Horizontal severity distribution bar
fn drawThreatBar(x, y, w, counts) {
    total = float(counts[0] + counts[1] + counts[2] + counts[3])
    if total <= 0.0 { return }
    fillC(theme["threatBarBg"])
    noStroke()
    roundedRect(float(x), float(y), float(w), 6.0, 3.0)
    cx = float(x)
    if counts[0] > 0 {
        sw = float(w) * float(counts[0]) / total
        fillC(theme["crit"])
        noStroke()
        roundedRect(cx, float(y), sw, 6.0, 3.0)
        cx = cx + sw
    }
    if counts[1] > 0 {
        sw = float(w) * float(counts[1]) / total
        fillC(theme["high"])
        noStroke()
        rect(cx, float(y), sw, 6.0)
        cx = cx + sw
    }
    if counts[2] > 0 {
        sw = float(w) * float(counts[2]) / total
        fillC(theme["med"])
        noStroke()
        rect(cx, float(y), sw, 6.0)
        cx = cx + sw
    }
    if counts[3] > 0 {
        sw = float(w) * float(counts[3]) / total
        fillC(theme["low"])
        noStroke()
        roundedRect(cx, float(y), sw, 6.0, 3.0)
    }
}

// Animated scan-sweep line clipped to main area
fn drawScanLine(mx, mw, ay, ah) {
    t  = elapsedTime()
    lx = float(mx) + fmod(t * 220.0, float(mw + 30))
    pushClip(mx, ay, mw, ah)
    fillCA(theme["scanLine"], 0.22)
    rect(lx, float(ay), 2.0, float(ah))
    fillCA(theme["scanLine"], 0.08)
    rect(lx - 14.0, float(ay), 14.0, float(ah))
    fillCA(theme["scanLine"], 0.08)
    rect(lx + 2.0,  float(ay), 14.0, float(ah))
    popClip()
}

// Shield icon for idle state
fn drawShield(cx, cy, sz) {
    hw = sz * 0.52
    fillC(theme["shieldOuter"])
    noStroke()
    roundedRect(cx - hw, cy - sz * 0.60, hw * 2.0, sz * 1.10, hw * 0.35)
    polygon([cx - hw, cy + sz * 0.20,
             cx + hw, cy + sz * 0.20,
             cx,      cy + sz * 0.72])
    fillC(theme["shieldInner"])
    noStroke()
    hw2 = hw * 0.68
    roundedRect(cx - hw2, cy - sz * 0.50, hw2 * 2.0, sz * 0.85, hw2 * 0.30)
    polygon([cx - hw2, cy + sz * 0.14,
             cx + hw2, cy + sz * 0.14,
             cx,       cy + sz * 0.55])
    fillC(theme["shieldDetail"])
    noStroke()
    circle(cx, cy - sz * 0.06, sz * 0.11)
    fillC(theme["shieldDetail"])
    noStroke()
    rect(cx - sz * 0.045, cy - sz * 0.06, sz * 0.09, sz * 0.22)
}

// Orbiting-dot spinner
fn drawSpinner(cx, cy, r) {
    const segs = 12
    t = elapsedTime() * 4.5
    for i in range(0, segs) {
        angle = float(i) / float(segs) * 6.2832 - t
        alpha = float(i) / float(segs) * 0.90 + 0.10
        fillCA(theme["spinner"], alpha)
        noStroke()
        sx = cx + cos(angle) * r
        sy = cy + sin(angle) * r
        circle(sx, sy, r * 0.22)
    }
}

// ── Main window ───────────────────────────────────────────────────────────────

themeApplied = false

window(1100, 800, "Secret Hunter", fn(frame) {
    // Apply widget theme once on the first frame (must be after window opens)
    if !themeApplied {
        themes.applyTheme(theme)
        themeApplied = true
    }

    background(theme["bg"][0], theme["bg"][1], theme["bg"][2])

    ww       = winWidth()
    wh       = winHeight()
    mainX    = SIDE_W + 2
    mainW    = ww - mainX
    footerH  = 0
    if scanStarted && !scanning { footerH = 40 }
    areaY    = HDR_H
    areaH    = wh - HDR_H - footerH
    midX     = float(mainX) + float(mainW) * 0.5
    midY     = float(areaY) + float(areaH) * 0.5
    t        = elapsedTime()

    uiBegin()
    uiSetFont(uiFont)

    // ── Pre-compute severity counts used in header + sidebar ─────────────────
    frameCounts = makeArray(4, 0)
    if scanStarted && !scanning && len(findings) > 0 {
        frameCounts = sevCounts(findings)
    }
    hasCritical = frameCounts[0] > 0

    // ── Drain progress channel ────────────────────────────────────────────────
    if scanning {
        draining = true
        while draining {
            msg = recvNonBlock(progressCh)
            if msg == null {
                draining = false
            } else {
                ph = msg["phase"]
                if ph == "enumerate" || ph == "git_enumerate" {
                    scanPhase = ph
                    scanTotal = 0
                    scanDone  = 0
                } else if ph == "files" || ph == "git" {
                    scanPhase = ph
                    scanTotal = msg["total"]
                    scanDone  = 0
                    if ph == "git" { scanRepoCount = msg["repoCount"] }
                } else if ph == "files_progress" || ph == "git_progress" {
                    scanDone = scanDone + msg["done"]
                }
            }
        }
        res = recvNonBlock(resultCh)
        if res != null {
            findings      = res["findings"]
            fileCount     = res["fileCount"]
            commitCount   = res["commitCount"]
            repoCount     = res["repoCount"]
            totalSec      = res["totalSec"]
            filesSec      = res["filesSec"]
            gitSec        = res["gitSec"]
            filesPerSec   = res["filesPerSec"]
            commitsPerSec = res["commitsPerSec"]
            scanning  = false
            scanPhase = ""
            nf = len(findings)
            if nf == 0 {
                toast("Scan complete — no secrets found", "success", 5.0)
            } else {
                sc = sevCounts(findings)
                if sc[0] > 0 {
                    toast("Scan complete — " + str(sc[0]) + " CRITICAL secrets found!", "error", 8.0)
                } else if sc[1] > 0 {
                    toast("Scan complete — " + str(nf) + " findings  (" + str(sc[1]) + " HIGH)", "warn", 6.0)
                } else {
                    toast("Scan complete — " + str(nf) + " findings", "warn", 5.0)
                }
            }
        }
    }

    // ── Left panel ────────────────────────────────────────────────────────────
    gradient(0, 0, float(SIDE_W), float(wh), theme["panelBg"], theme["bg"], "v")

    fillC(theme["panelBorder"])
    noStroke()
    rect(float(SIDE_W), 0.0, 2.0, float(wh))
    fillC(theme["panelBorderFade"])
    noStroke()
    rect(float(SIDE_W) + 2.0, 0.0, 1.0, float(wh))

    // ── Title ─────────────────────────────────────────────────────────────────
    fillC(theme["accentBar"])
    noStroke()
    roundedRect(10.0, 10.0, 3.0, 32.0, 1.5)

    glowWhite(uiFont, "SECRET HUNTER", 18, 10, 1.14)

    fillC(theme["subtitleText"])
    textFont(uiFont, "security audit tool  //  v1.0", 20, 46, 0.60)

    flicker = 0.04 + 0.02 * sin(t * 3.7)
    fillCA(theme["crit"], flicker)
    rect(18.0, 8.0, float(SIDE_W) - 28.0, 1.0)

    sideSep(68)

    // ── Scan controls ─────────────────────────────────────────────────────────
    uiBeginCol(10, 78, 270, 0)
    sectionLabel("SCAN PATH", uiColY())                                          uiColAdvance(18)
    fillC(theme["inputHint"])
    scanPath   = textInput(".", scanPath, uiColX(), uiColY(), uiColW(), 30, 0.62)  uiColAdvance(40)
    tooltip("Path to scan — use . for the current directory, or paste an absolute path")
    includeGit = checkbox("Include git history", uiColX() + 2, uiColY(), includeGit, 0.62)  uiColAdvance(32)
    tooltip("Also scans every git commit for leaked credentials, tokens, and API keys")
    sectionLabel("MAX FILE SIZE (MB)", uiColY())
    fillC(theme["sliderHint"])
    drawRight(uiFont, str(int(maxSizeMB)) + " MB", 278.0, uiColY(), 0.62)       uiColAdvance(18)
    maxSizeMB  = slider("", uiColX(), uiColY(), uiColW(), float(maxSizeMB), 1.0, 100.0, 0.62)  uiColAdvance(42)
    tooltip("Files larger than this are skipped — lower values keep scans fast on large repos")
    sideSep(uiColY())                                                             uiColAdvance(8)

    // ── SCAN button ───────────────────────────────────────────────────────────
    if scanning {
        fillC(theme["scanningBg"])
        noStroke()
        roundedRect(uiColX(), uiColY(), uiColW(), 46.0, 8.0)
        fillC(theme["scanningText"])
        drawCentred(uiFont, "Scanning...", float(uiColX()) + float(uiColW()) * 0.5, float(uiColY()) + 14.0, 0.70)
        if scanPhase == "enumerate" || scanPhase == "git_enumerate" {
            drawSpinner(float(uiColX()) + 62.0, float(uiColY()) + 25.0, 10.0)
        }
        if scanTotal > 0 {
            progressBar(uiColX(), uiColY() + 56, uiColW(), 8, float(scanDone), float(scanTotal))
            pct = int(float(scanDone) / float(scanTotal) * 100.0)
            fillCA(theme["scanningText"], 0.80)
            noStroke()
            if scanPhase == "git" || scanPhase == "git_progress" {
                textFont(uiFont, str(scanDone) + " / " + str(scanTotal) + " commits  (" + str(pct) + "%)", uiColX(), uiColY() + 72, 0.55)
            } else {
                textFont(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  (" + str(pct) + "%)", uiColX(), uiColY() + 72, 0.55)
            }
        }
    } else {
        pulse = 0.5 + 0.5 * sin(t * 2.2)
        fillCA(theme["crit"], pulse * 0.12)
        noStroke()
        roundedRect(float(uiColX()) - 4.0, float(uiColY()) - 4.0, float(uiColW()) + 8.0, 54.0, 10.0)
        if button("SCAN", uiColX(), uiColY(), uiColW(), 46, 0.80) {
            path = scanPath
            if len(path) == 0 { path = "." }
            pch = channel(200)
            rch = channel(1)
            let _scan  = sh.runScanWithProgress
            let _path  = path
            let _doGit = includeGit
            let _maxMB = maxSizeMB
            let _pch   = pch
            let _rch   = rch
            async(fn() {
                send(_rch, _scan(_path, _doGit, _maxMB, _pch))
            })
            progressCh  = pch
            resultCh    = rch
            scanning    = true
            scanStarted = true
            scanPhase   = "enumerate"
            scanTotal   = 0
            scanDone    = 0
            findings      = makeArray(0)
            fileCount     = 0
            commitCount   = 0
            totalSec      = 0.0
            filesSec      = 0.0
            gitSec        = 0.0
            filesPerSec   = 0.0
            commitsPerSec = 0.0
            selectedFinding  = -1
            lastClickRow     = -1
            lastClickTime    = -1.0
        }
        tooltip("Start scanning the path for secrets, tokens, and credentials")
    }

    // ── Post-scan results ─────────────────────────────────────────────────────
    if scanStarted && !scanning {
        sideSep(296)
        sectionLabel("RESULTS", 306)

        lvl = threatLevel(frameCounts)
        fillC(theme["dimLabel"])
        textFont(uiFont, "THREAT LEVEL", 12, 328, 0.56)
        if lvl == "CRITICAL"      { fillC(theme["crit"])     }
        else if lvl == "HIGH"     { fillC(theme["high"])     }
        else if lvl == "ELEVATED" { fillC(theme["med"])      }
        else if lvl == "LOW"      { fillC(theme["threatLow"])}
        else                      { fillC(theme["low"])      }
        drawRight(uiFont, lvl, 278.0, 328, 0.65)

        drawThreatBar(10, 350, 270, frameCounts)

        sevColors = [theme["crit"], theme["high"], theme["med"], theme["low"]]
        pieChart(frameCounts, sevColors, 145.0, 410.0, 44.0, 18.0)

        abbrevs = ["CRIT", "HIGH", "MED", "LOW"]
        i = 0
        while i < 4 {
            lx = 14.0 + float(i) * 66.0
            fillCA(sevColors[i], 1.0)
            textFont(uiFont, str(frameCounts[i]), lx, 460.0, 0.72)
            fillCA(sevColors[i], 0.60)
            textFont(uiFont, abbrevs[i], lx, 474.0, 0.48)
            i = i + 1
        }

        sideSep(498)

        uiBeginCol(12, 508, 266, 0)
        fillC(theme["statLabel"])
        textFont(uiFont, "files", uiColX(), uiColY(), 0.62)
        fillC(theme["statValue"])
        drawRight(uiFont, str(fileCount), 278.0, uiColY(), 0.66)       uiColAdvance(18)
        fillC(theme["statLabel"])
        textFont(uiFont, "commits", uiColX(), uiColY(), 0.62)
        fillC(theme["statValue"])
        drawRight(uiFont, str(commitCount), 278.0, uiColY(), 0.66)     uiColAdvance(18)
        if repoCount > 1 {
            fillC(theme["statLabel"])
            textFont(uiFont, "git repositories", uiColX(), uiColY(), 0.62)
            fillCA(theme["wAccent"], 0.90)
            drawRight(uiFont, str(repoCount), 278.0, uiColY(), 0.66)   uiColAdvance(18)
        }
        fillC(theme["statLabel"])
        textFont(uiFont, "findings", uiColX(), uiColY(), 0.62)
        fillC(theme["statValue"])
        drawRight(uiFont, str(len(findings)), 278.0, uiColY(), 0.66)
    }

    // ── Filters (always visible) ──────────────────────────────────────────────
    sideSep(606)
    sectionLabel("FILTERS", 616)
    uiBeginCol(10, 640, 270, 0)
    fillC(theme["filterLabel"])
    textFont(uiFont, "Severity", uiColX() + 3, uiColY(), 0.60)              uiColAdvance(16)
    severityFilter = dropdown("", SEV_OPTIONS, uiColX(), uiColY(), uiColW(), 0.60)  uiColAdvance(38)
    tooltip("Show only findings at or above this severity — ALL shows everything")
    fillC(theme["filterLabel"])
    textFont(uiFont, "Source", uiColX() + 3, uiColY(), 0.60)                uiColAdvance(16)
    sourceFilter = dropdown("", SRC_OPTIONS, uiColX(), uiColY(), uiColW(), 0.60)
    tooltip("Show only findings from files, only from git history, or both")

    // ── Main header ───────────────────────────────────────────────────────────
    gradient(float(mainX), 0, float(mainW), float(HDR_H), [0.14, 0.13, 0.19, 1.0], theme["headerBg"], "v")

    if hasCritical {
        alpha = 0.04 + 0.02 * sin(t * 2.8)
        fillCA(theme["crit"], alpha)
        noStroke()
        rect(float(mainX), 0.0, float(mainW), float(HDR_H))
    }

    fillC(theme["headerBorder"])
    noStroke()
    rect(float(mainX), float(HDR_H) - 1.0, float(mainW), 1.0)

    fillC(theme["mainLabel"])
    textFont(uiFont, "FINDINGS", float(mainX) + 20.0, 14.0, 0.80)

    // Threat level badge in header (post-scan)
    if scanStarted && !scanning {
        nf  = len(findings)
        lvl = threatLevel(frameCounts)
        if nf == 0 {
            fillC(theme["low"])
            badgeTxt = "CLEAN"
        } else if frameCounts[0] > 0 {
            fillC(theme["crit"])
            badgeTxt = lvl
        } else if frameCounts[1] > 0 {
            fillC(theme["high"])
            badgeTxt = lvl
        } else if frameCounts[2] > 0 {
            fillC(theme["med"])
            badgeTxt = lvl
        } else {
            fillC(theme["low"])
            badgeTxt = lvl
        }
        noStroke()
        bsc  = 0.64
        btw  = textWidth(uiFont, badgeTxt, bsc) + 22.0
        bth  = 26.0
        btr  = bth * 0.5
        btx  = float(mainX) + float(mainW) - btw - 20.0
        bty  = (float(HDR_H) - bth) * 0.5

        if nf > 0 && frameCounts[0] > 0 {
            ba = 0.08 + 0.06 * sin(t * 3.0)
            fillCA(theme["crit"], ba)
            roundedRect(btx - 4.0, bty - 4.0, btw + 8.0, bth + 8.0, btr + 4.0)
        }

        if nf == 0 { fillC(theme["low"]) }
        else if frameCounts[0] > 0 { fillC(theme["crit"]) }
        else if frameCounts[1] > 0 { fillC(theme["high"]) }
        else if frameCounts[2] > 0 { fillC(theme["med"])  }
        else { fillC(theme["low"]) }
        noStroke()
        rect(btx + btr, bty, btw - btr * 2.0, bth)
        circle(btx + btr,       bty + btr, btr)
        circle(btx + btw - btr, bty + btr, btr)
        fillC(theme["pillText"])
        noStroke()
        textFont(uiFont, badgeTxt, btx + 11.0, bty + 4.0, bsc)

        fillC(theme["findingCount"])
        if nf > 0 {
            drawRight(uiFont, str(nf) + " findings", btx - 14.0, bty + 4.0, 0.62)
        }
    }

    // ── Main area body ────────────────────────────────────────────────────────
    if !scanStarted {
        drawShield(midX, midY - 52.0, 52.0)

        fillC(theme["hintText"])
        drawCentred(uiFont, "Enter a path and press SCAN", midX, midY + 16.0, 0.72)
        fillC(theme["hintSub"])
        drawCentred(uiFont, "Scans files and git history in parallel across all CPU cores", midX, midY + 38.0, 0.60)

        fillC(theme["bottomHint"])
        drawCentred(uiFont, "Double-click any finding to open the file", midX, float(wh) - 28.0, 0.55)

    } else if scanning {
        drawScanLine(mainX, mainW, areaY, areaH)

        if scanPhase == "enumerate" || scanPhase == "git_enumerate" {
            drawSpinner(midX, midY - 44.0, 26.0)
            fillC(theme["scanStatus"])
            if scanPhase == "enumerate" {
                drawCentred(uiFont, "Enumerating files...", midX, midY + 0.0, 0.72)
                fillC(theme["scanSub"])
                drawCentred(uiFont, "walking the directory tree", midX, midY + 22.0, 0.60)
            } else {
                drawCentred(uiFont, "Discovering git repositories...", midX, midY + 0.0, 0.72)
                fillC(theme["scanSub"])
                drawCentred(uiFont, "walking directory tree for .git folders", midX, midY + 22.0, 0.60)
            }

        } else if scanPhase == "files" || scanPhase == "files_progress" ||
                  scanPhase == "git"   || scanPhase == "git_progress" {
            isGit = (scanPhase == "git" || scanPhase == "git_progress")
            fillC(theme["scanProgress"])
            if isGit {
                drawCentred(uiFont, "Scanning git history", midX, midY - 50.0, 0.74)
                if scanRepoCount > 1 {
                    fillC(theme["scanSub"])
                    drawCentred(uiFont, "across " + str(scanRepoCount) + " repositories", midX, midY - 28.0, 0.62)
                }
            } else {
                drawCentred(uiFont, "Scanning files", midX, midY - 50.0, 0.74)
            }
            if scanTotal > 0 {
                barW = int(float(mainW) * 0.58)
                barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(scanDone), float(scanTotal))
                pct = int(float(scanDone) / float(scanTotal) * 100.0)
                fillC(theme["progressText"])
                if isGit {
                    drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " commits  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
                } else {
                    drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
                }
            } else {
                drawSpinner(midX, midY, 20.0)
            }
        } else {
            drawSpinner(midX, midY - 24.0, 22.0)
        }

    } else if len(findings) == 0 {
        ckx = midX - 10.0
        cky = midY - 62.0
        gA  = 0.65 + 0.12 * sin(t * 1.8)
        fillCA(theme["low"], gA)
        noStroke()
        rect(ckx - 14.0, cky + 16.0, 14.0, 6.0)
        rect(ckx - 2.0,  cky + 20.0, 6.0,  25.0)
        rect(ckx + 4.0,  cky,        6.0,  22.0)

        fillC(theme["noFindText"])
        drawCentred(uiFont, "No secrets found", midX, midY + 4.0, 0.82)
        fillC(theme["noFindSub"])
        if commitCount > 0 && repoCount > 1 {
            drawCentred(uiFont, "Scanned " + str(fileCount) + " files  +  " + str(commitCount) + " commits across " + str(repoCount) + " repositories", midX, midY + 28.0, 0.60)
        } else {
            drawCentred(uiFont, "Scanned " + str(fileCount) + " files and " + str(commitCount) + " commits", midX, midY + 28.0, 0.63)
        }

    } else {
        // ── Threat distribution bar strip above tree ──────────────────────────
        gradient(float(mainX), float(areaY), float(mainW), 22.0, theme["headerBg"], theme["bg"], "v")
        drawThreatBar(mainX + 12, areaY + 8, mainW - 24, frameCounts)

        lx = float(mainX) + float(mainW) - 6.0
        if frameCounts[3] > 0 {
            fillCA(theme["low"], 0.60)
            drawRight(uiFont, "L:" + str(frameCounts[3]), lx, areaY + 4, 0.52)
            lx = lx - textWidth(uiFont, "L:" + str(frameCounts[3]), 0.52) - 10.0
        }
        if frameCounts[2] > 0 {
            fillCA(theme["med"], 0.75)
            drawRight(uiFont, "M:" + str(frameCounts[2]), lx, areaY + 4, 0.52)
            lx = lx - textWidth(uiFont, "M:" + str(frameCounts[2]), 0.52) - 10.0
        }
        if frameCounts[1] > 0 {
            fillCA(theme["high"], 0.80)
            drawRight(uiFont, "H:" + str(frameCounts[1]), lx, areaY + 4, 0.52)
            lx = lx - textWidth(uiFont, "H:" + str(frameCounts[1]), 0.52) - 10.0
        }
        if frameCounts[0] > 0 {
            fillC(theme["crit"])
            drawRight(uiFont, "C:" + str(frameCounts[0]), lx, areaY + 4, 0.52)
        }

        treeY = areaY + 22
        treeH = areaH - 22

        // ── Findings tree ─────────────────────────────────────────────────────
        newKey = str(len(findings)) + ":" + severityFilter + ":" + sourceFilter
        if newKey != treeKey {
            built        = buildFindingTree(findings, severityFilter, sourceFilter)
            treeLabels   = built[0]
            treeLevels   = built[1]
            treeNodeFind = built[2]
            treeExpanded = makeArray(len(treeLabels), false)
            treeSelected = 0
            treeKey      = newKey
        }

        if len(treeLabels) == 0 {
            fillC(theme["filterNoMatch"])
            drawCentred(uiFont, "No findings match the current filters", midX, midY, 0.65)
        } else {
            res          = treeView(mainX, treeY, mainW, treeH, treeLabels, treeLevels, treeExpanded, 0.60)
            newSel       = res[0]
            treeExpanded = res[1]

            mx     = mouseX()
            my     = mouseY()
            inTree = mx >= float(mainX) && mx <= float(mainX + mainW) &&
                     my >= float(treeY)  && my <= float(treeY + treeH)
            if mouseClicked() && inTree {
                tt = elapsedTime()
                if newSel == lastClickRow && tt - lastClickTime < 0.35 {
                    if newSel >= 0 && newSel < len(treeNodeFind) {
                        fi = treeNodeFind[newSel]
                        if fi >= 0 {
                            fpath = findings[fi]["file"]
                            if len(fpath) > 0 { _processExec("open", [fpath]) }
                        }
                    }
                    lastClickRow  = -1
                    lastClickTime = -1.0
                } else {
                    lastClickRow  = newSel
                    lastClickTime = tt
                }
            }

            treeSelected = newSel
            if treeSelected >= 0 && treeSelected < len(treeNodeFind) {
                fi = treeNodeFind[treeSelected]
                if fi >= 0 { selectedFinding = fi }
            }
        }
    }

    // ── Performance footer (main area bottom, post-scan only) ────────────────
    if scanStarted && !scanning {
        fy = wh - 40

        gradient(float(mainX), float(fy), float(mainW), 40.0, theme["bg"], theme["footerBg"], "v")
        fillC(theme["footerBorder"])
        rect(float(mainX), float(fy), float(mainW), 1.0)

        fn fDiv(x) {
            fillC(theme["footerDiv"])
            rect(float(x), float(fy) + 8.0, 1.0, 24.0)
        }

        uiBeginRow(float(mainX) + 18.0, float(fy + 11), 18.0, 0)

        if frameCounts[0] > 0 { fillC(theme["crit"]) }
        else { fillC(theme["low"]) }
        circle(uiRowX() + 4.0, uiRowY() + 7.0, 5.0)
        uiRowAdvance(14)

        fillC(theme["footerStatus"])
        textFont(uiFont, "SCAN COMPLETE", uiRowX(), uiRowY(), 0.62)
        uiRowAdvance(textWidth(uiFont, "SCAN COMPLETE", 0.62) + 16.0)
        fDiv(uiRowX())
        uiRowAdvance(14)

        fillC(theme["footerTime"])
        totalStr = format("%.2f's  total", totalSec)
        textFont(uiFont, totalStr, uiRowX(), uiRowY(), 0.65)
        uiRowAdvance(textWidth(uiFont, totalStr, 0.65) + 16.0)
        fDiv(uiRowX())
        uiRowAdvance(14)

        if filesPerSec >= 1000.0 {
            ftpStr = format("%.1f k files/s", filesPerSec / 1000.0)
        } else {
            ftpStr = format("%.0f files/s", filesPerSec)
        }
        fillC(theme["footerStat"])
        textFont(uiFont, ftpStr, uiRowX(), uiRowY(), 0.65)
        uiRowAdvance(textWidth(uiFont, ftpStr, 0.65) + 16.0)

        if includeGit && commitCount > 0 {
            fDiv(uiRowX())
            uiRowAdvance(14)
            fillC(theme["footerStat"])
            gitTpStr = format("%.0f commits/s", commitsPerSec)
            textFont(uiFont, gitTpStr, uiRowX(), uiRowY(), 0.65)
            uiRowAdvance(textWidth(uiFont, gitTpStr, 0.65) + 16.0)
            if repoCount > 1 {
                fDiv(uiRowX())
                uiRowAdvance(14)
                fillCA(theme["wAccent"], 0.85)
                repoStr = str(repoCount) + " repos"
                textFont(uiFont, repoStr, uiRowX(), uiRowY(), 0.65)
            }
        }
    }

    uiEnd()
})
