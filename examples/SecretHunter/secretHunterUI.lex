// secretHunterUI.lex — graphical front-end for the Secret Hunter scan engine.
//
// Run: KLEX_PATH=. ./klex examples/SecretHunter/secretHunterUI.lex

import "examples/SecretHunter/secretHunterLib.lex" as sh
import "stdlib/ui_themes.lex" as themes

const SIDE_W = 290
const HDR_H  = 48

// ── Fonts ─────────────────────────────────────────────────────────────────────
// Walk a cross-platform candidate list and use the first font that loads.
// loadFont accepts .ttf / .otf only — .ttc files silently fail.

fn tryFont(paths, size) {
    let i = 0
    while i < len(paths) {
        let f, err = safe(loadFont, paths[i], size)
        if err == null { return f }
        i = i + 1
    }
    return null
}

let uiFont = tryFont([
    // macOS
    "/System/Library/Fonts/SFNS.ttf",
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/System/Library/Fonts/Supplemental/Arial.ttf",
    "/System/Library/Fonts/Supplemental/Tahoma.ttf",
    "/System/Library/Fonts/Supplemental/Verdana.ttf",
    // Linux — Fedora paths first, then Debian/Ubuntu
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    "/usr/share/fonts/liberation-sans/LiberationSans-Bold.ttf",
    "/usr/share/fonts/liberation-sans/LiberationSans-Regular.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
    // Windows
    "C:/Windows/Fonts/segoeuib.ttf",
    "C:/Windows/Fonts/segoeui.ttf",
    "C:/Windows/Fonts/arialbd.ttf",
    "C:/Windows/Fonts/arial.ttf",
], 18)

if uiFont == null {
    println("ERROR: no usable font found. Install dejavu-sans-fonts (Fedora) or fonts-dejavu (Debian/Ubuntu), or run on a system with default OS fonts available.")
    _osExit(1)
}

// ── Theme ─────────────────────────────────────────────────────────────────────
// Swap themes.crimson() for themes.dark() / themes.midnight() / themes.forest()
// / themes.light() to instantly retheme the entire UI.

let theme = themes.crimson()

// ── Remediation helpers ───────────────────────────────────────────────────────

fn copyToClipboard(text) {
    let tmp = "/tmp/.secrethunter_clip"
    let _, werr = safe(writeFile, tmp, text)
    if werr != null { return }
    // Run async — xclip can hang waiting for clipboard requests (especially
    // on Wayland with XWayland). Running the chain in a goroutine means the
    // UI thread is never blocked regardless of which tool ends up handling
    // the paste. Order: wl-copy (Wayland-native, exits clean) → pbcopy
    // (macOS) → xsel (X11, exits clean) → xclip (X11, last resort).
    async(fn() {
        _, _, _, _ = _processExec("sh", ["-c",
            "wl-copy < " + tmp + " 2>/dev/null || " +
            "pbcopy < " + tmp + " 2>/dev/null || " +
            "xsel --clipboard --input < " + tmp + " 2>/dev/null || " +
            "xclip -selection clipboard -in < " + tmp + " 2>/dev/null"])
    })
}

fn buildGitCmd(f, root) {
    let file   = f["file"]
    let src    = f["source"]
    let commit = f["commit"]
    let out = "cd '" + root + "'"
    if src == "git" && len(commit) >= 7 {
        let sha = substr(commit, 0, 7)
        let author = f["author"]
        out = out + "\n# Secret found in commit " + sha + " by " + author
    } else {
        out = out + "\n# WARNING: coordinate with your team before force-pushing"
    }
    if len(file) > 0 {
        out = out + "\ngit filter-repo --path '" + file + "' --invert-paths --force"
    }
    out = out + "\ngit push origin --force --all"
    out = out + "\ngit push origin --force --tags"
    return out
}

fn buildEnvHint(f) {
    let pat = lower(f["patternName"])
    if indexOf(pat, "aws_access") >= 0 {
        return "export AWS_ACCESS_KEY_ID=\"<new-key>\"\nexport AWS_SECRET_ACCESS_KEY=\"<new-secret>\""
    }
    if indexOf(pat, "github") >= 0 { return "export GITHUB_TOKEN=\"<new-token>\"" }
    if indexOf(pat, "gitlab") >= 0 { return "export GITLAB_TOKEN=\"<new-token>\"" }
    if indexOf(pat, "stripe") >= 0 { return "export STRIPE_SECRET_KEY=\"<new-key>\"" }
    if indexOf(pat, "slack") >= 0  { return "export SLACK_TOKEN=\"<new-token>\"" }
    if indexOf(pat, "database") >= 0 {
        return "export DATABASE_URL=\"<driver>://user:pass@host/db\""
    }
    if indexOf(pat, "sendgrid") >= 0 { return "export SENDGRID_API_KEY=\"<new-key>\"" }
    return ""
}

// ── App state ─────────────────────────────────────────────────────────────────

let scanPath    = "."
let includeGit  = true
let useYara     = false
let useEntropy  = false
let useJWT      = true
let maxSizeMB   = 5.0
let scanning    = false
let scanStarted = false
let progressCh  = null
let resultCh    = null
let findings    = makeArray(0)
let fileCount   = 0
let commitCount = 0
let repoCount   = 0
let totalSec    = 0.0
let filesSec    = 0.0
let gitSec      = 0.0
let filesPerSec   = 0.0
let commitsPerSec = 0.0
let selectedFinding  = -1
let lastClickRow     = -1
let lastClickTime    = -1.0
let allFindings      = makeArray(0)
let ignoreRules      = makeArray(0)
let suppressedCount  = 0
let showCtxMenu      = false
let ctxMenuX         = 0
let ctxMenuY         = 0
let ctxFindingIdx    = -1
let scanPhase        = ""
let scanTotal        = 0
let scanDone         = 0
let scanRepoCount    = 0

// Live per-severity counts populated from streaming bridge events during a
// scan. Currently driven only by the entropy phase (entropy_scan_stream).
// During scanning these feed the sidebar tiles so they tick up live; once
// the scan ends, sevCounts(findings) takes over and reflects everything.
let liveSevCounts    = makeArray(4, 0)

// ── GitHub org scan state ─────────────────────────────────────────────────────

let orgCfg         = sh.loadSecretHunterConfig()
let settingsOpen       = false
let settingsJustOpened = false
let settingsToken  = orgCfg["github_token"]
let settingsBridge = orgCfg["github_bridge_path"]
let settingsPython = orgCfg["python_executable"]
let settingsDeep   = orgCfg["default_scan_mode"] == "deep"
let settingsTmp    = orgCfg["temp_dir"]
let orgScanRepo    = ""
let orgScanIdx     = 0
let orgScanTotal   = 0
let orgScanDone    = 0
let orgScanPhase   = ""

// ── Filter + tree-view state ──────────────────────────────────────────────────

let SEV_OPTIONS    = ["ALL", "CRITICAL", "HIGH", "MEDIUM", "LOW"]
let SEV_NAMES      = ["CRITICAL", "HIGH", "MEDIUM", "LOW"]
let SRC_OPTIONS    = ["ALL", "FILES", "GIT", "YARA", "ENTROPY", "JWT"]
let severityFilter = "ALL"
let sourceFilter   = "ALL"

// Sidebar count-up animation — set to elapsedTime() at scan complete; -1 = idle
let SEV_ANIM_DURATION = 0.55
let sevAnimStart      = -1.0
let treeLabels     = makeArray(0)
let treeLevels     = makeArray(0)
let treeExpanded   = makeArray(0)
let treeSelected   = 0
let treeNodeFind   = makeArray(0)
let treeKey        = ""

// ── Severity helpers ──────────────────────────────────────────────────────────

fn sevFill(sev) {
    if sev == "CRITICAL"    { fillC(theme["crit"]) }
    else if sev == "HIGH"   { fillC(theme["high"]) }
    else if sev == "MEDIUM" { fillC(theme["med"])  }
    else                    { fillC(theme["low"])  }
}

fn sevCounts(flist) {
    let crit = 0   let high = 0   let med = 0   let low = 0
    let i = 0
    let n = len(flist)
    while i < n {
        let s = flist[i]["severity"]
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
    let n = len(flist)
    if n == 0 { return [makeArray(0), makeArray(0), makeArray(0)] }

    // Group by (file, source) so git and filesystem findings get separate parent nodes
    let seenKeys  = makeArray(n * 2, "")
    let seenFiles = makeArray(n * 2, "")
    let seenSrcs  = makeArray(n * 2, "")
    let seenCount = 0
    let filtIdxs  = makeArray(n, 0)
    let filtCount = 0

    let i = 0
    while i < n {
        let f     = flist[i]
        let okSev = sevF == "ALL" || f["severity"] == sevF
        let okSrc = srcF == "ALL" ||
                (srcF == "FILES"   && f["source"] == "file")    ||
                (srcF == "GIT"    && f["source"] == "git")     ||
                (srcF == "YARA"   && f["source"] == "yara")    ||
                (srcF == "ENTROPY" && f["source"] == "entropy") ||
                (srcF == "JWT"    && hasKey(f, "jwt"))
        if okSev && okSrc {
            filtIdxs[filtCount] = i
            filtCount = filtCount + 1
            let fname    = f["file"]
            let fsrc     = f["source"]
            let groupKey = fname + "|" + fsrc
            let isNew = true
            let j = 0
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

    let totalNodes = seenCount + filtCount
    let labels   = makeArray(totalNodes, "")
    let levels   = makeArray(totalNodes, 0)
    let nodeFnd  = makeArray(totalNodes, -1)

    let nodeIdx = 0
    let fi = 0
    while fi < seenCount {
        let fname    = seenFiles[fi]
        let fsrc     = seenSrcs[fi]
        let groupKey = seenKeys[fi]
        if fsrc == "git" {
            labels[nodeIdx] = "[GIT]     " + fname
        } else if fsrc == "yara" {
            labels[nodeIdx] = "[YARA]    " + fname
        } else if fsrc == "entropy" {
            labels[nodeIdx] = "[ENTROPY] " + fname
        } else {
            labels[nodeIdx] = "[FILE]    " + fname
        }
        levels[nodeIdx]  = 0
        nodeFnd[nodeIdx] = -1
        nodeIdx = nodeIdx + 1
        let ci = 0
        while ci < filtCount {
            let origIdx = filtIdxs[ci]
            let f = flist[origIdx]
            if f["file"] == fname && f["source"] == fsrc {
                if f["source"] == "file" {
                    let locPart = "line " + str(f["line"])
                } else if f["source"] == "yara" {
                    let locPart = "yara"
                } else if f["source"] == "entropy" {
                    let locPart = "entropy"
                } else {
                    let locPart = f["commit"]
                    if len(locPart) > 7 { locPart = substr(locPart, 0, 7) }
                }
                let matchSnip = sh.fitText(f["match"], 44)
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
    let tw = textWidth(uiFont, txt, 0.62)
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
    let w = textWidth(fnt, txt, sc)
    textFont(fnt, txt, cx - w / 2, y, sc)
}

fn drawRight(fnt, txt, rightX, y, sc) {
    let w = textWidth(fnt, txt, sc)
    textFont(fnt, txt, rightX - w, y, sc)
}

// Severity pill sized to actual text
fn drawSevPill(sev, bx, by) {
    let sc = 0.60
    let tw = textWidth(uiFont, sev, sc)
    let bw = tw + 16.0
    let bh = 18.0
    let br = bh * 0.5
    sevFill(sev)
    noStroke()
    roundedRect(bx, by, bw, bh, br)
    fillC(theme["pillText"])
    textFont(uiFont, sev, bx + 8.0, by + 1.0, sc)
    return bw
}

// Source tag pill
fn drawSourceTag(tag, tx, ty) {
    let sc = 0.52
    let tw = textWidth(uiFont, tag, sc)
    let bw = tw + 10.0
    let bh = 14.0
    let br = bh * 0.5
    fillC(theme["sourceTagBg"])
    noStroke()
    roundedRect(tx, ty, bw, bh, br)
    fillC(theme["sourceTagText"])
    textFont(uiFont, tag, tx + 5.0, ty + 1.0, sc)
    return bw
}

// Card-style severity row for sidebar results
fn sevCard(sev, count, y) {
    shadow(2.0, 3.0, 8.0, 0.0, 0.0, 0.0, 0.30)
    fillC(theme["cardBg"])
    noStroke()
    roundedRect(10.0, float(y), 270.0, 27.0, 4.0)
    noShadow()
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
    let total = float(counts[0] + counts[1] + counts[2] + counts[3])
    if total <= 0.0 { return }
    const barH = 10.0
    const barR = 4.0
    fillC(theme["threatBarBg"])
    noStroke()
    roundedRect(float(x), float(y), float(w), barH, barR)
    let cx = float(x)
    if counts[0] > 0 {
        let sw = float(w) * float(counts[0]) / total
        fillC(theme["crit"])
        noStroke()
        roundedRect(cx, float(y), sw, barH, barR)
        cx = cx + sw
    }
    if counts[1] > 0 {
        let sw = float(w) * float(counts[1]) / total
        fillC(theme["high"])
        noStroke()
        rect(cx, float(y), sw, barH)
        cx = cx + sw
    }
    if counts[2] > 0 {
        let sw = float(w) * float(counts[2]) / total
        fillC(theme["med"])
        noStroke()
        rect(cx, float(y), sw, barH)
        cx = cx + sw
    }
    if counts[3] > 0 {
        let sw = float(w) * float(counts[3]) / total
        fillC(theme["low"])
        noStroke()
        roundedRect(cx, float(y), sw, barH, barR)
    }
    // Subtle inner highlight along the top edge for depth
    fillCA(theme["wText"], 0.10)
    noStroke()
    rect(float(x) + 1.0, float(y), float(w) - 2.0, 1.0)
}

// Animated scan-sweep line clipped to main area
fn drawScanLine(mx, mw, ay, ah) {
    let t  = elapsedTime()
    let lx = float(mx) + fmod(t * 220.0, float(mw + 30))
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
    let hw = sz * 0.52
    shadow(0.0, 6.0, 18.0)
    fillC(theme["shieldOuter"])
    noStroke()
    roundedRect(cx - hw, cy - sz * 0.60, hw * 2.0, sz * 1.10, hw * 0.35)
    noShadow()
    polygon([cx - hw, cy + sz * 0.20,
             cx + hw, cy + sz * 0.20,
             cx,      cy + sz * 0.72])
    fillC(theme["shieldInner"])
    noStroke()
    let hw2 = hw * 0.68
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
    let t = elapsedTime() * 4.5
    for i in range(0, segs) {
        let angle = float(i) / float(segs) * 6.2832 - t
        let alpha = float(i) / float(segs) * 0.90 + 0.10
        fillCA(theme["spinner"], alpha)
        noStroke()
        let sx = cx + cos(angle) * r
        let sy = cy + sin(angle) * r
        circle(sx, sy, r * 0.22)
    }
}

// ── Main window ───────────────────────────────────────────────────────────────

let themeApplied = false

window(1100, 800, "Secret Hunter", fn(frame) {
    // Apply widget theme once on the first frame (must be after window opens)
    if !themeApplied {
        themes.applyTheme(theme)
        themeApplied = true
    }

    background(theme["bg"][0], theme["bg"][1], theme["bg"][2])

    let ww       = winWidth()
    let wh       = winHeight()
    let mainX    = SIDE_W + 2
    let mainW    = ww - mainX
    let footerH  = 0
    if scanStarted && !scanning { footerH = 40 }
    let areaY    = HDR_H
    let areaH    = wh - HDR_H - footerH
    let midX     = float(mainX) + float(mainW) * 0.5
    let midY     = float(areaY) + float(areaH) * 0.5
    let t        = elapsedTime()

    uiBegin()
    uiSetFont(uiFont)

    // ── Pre-compute severity counts used in header + sidebar ─────────────────
    // During a scan the tiles tick up from liveSevCounts (currently fed by the
    // entropy streaming bridge). Once the scan ends, the final filtered
    // findings array takes over.
    let frameCounts = makeArray(4, 0)
    if scanning {
        frameCounts = liveSevCounts
    } else if scanStarted && len(findings) > 0 {
        frameCounts = sevCounts(findings)
    }
    let hasCritical = frameCounts[0] > 0

    // ── Drain progress channel ────────────────────────────────────────────────
    if scanning {
        let draining = true
        while draining {
            let msg = recvNonBlock(progressCh)
            if msg == null {
                draining = false
            } else {
                let ph = msg["phase"]
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
                } else if ph == "yara" {
                    scanPhase = "yara"
                    scanTotal = msg["total"]
                    scanDone  = 0
                } else if ph == "yara_progress" {
                    scanDone = scanDone + msg["done"]
                } else if ph == "yara_finding" {
                    // Per-finding event from the streaming YARA bridge —
                    // feeds the sidebar tiles live, identical bookkeeping
                    // to entropy_finding.
                    let sev = msg["severity"]
                    if sev == "CRITICAL"    { liveSevCounts[0] = liveSevCounts[0] + 1 }
                    else if sev == "HIGH"   { liveSevCounts[1] = liveSevCounts[1] + 1 }
                    else if sev == "MEDIUM" { liveSevCounts[2] = liveSevCounts[2] + 1 }
                    else                    { liveSevCounts[3] = liveSevCounts[3] + 1 }
                } else if ph == "yara_done" {
                    scanDone = scanTotal
                } else if ph == "entropy" {
                    scanPhase = "entropy"
                    scanTotal = msg["total"]
                    scanDone  = 0
                } else if ph == "entropy_progress" {
                    scanDone = scanDone + msg["done"]
                } else if ph == "entropy_finding" {
                    // Per-finding event from the streaming entropy bridge —
                    // bump the matching severity slot so the sidebar tiles
                    // tick up live instead of waiting for scan end.
                    let sev = msg["severity"]
                    if sev == "CRITICAL"    { liveSevCounts[0] = liveSevCounts[0] + 1 }
                    else if sev == "HIGH"   { liveSevCounts[1] = liveSevCounts[1] + 1 }
                    else if sev == "MEDIUM" { liveSevCounts[2] = liveSevCounts[2] + 1 }
                    else                    { liveSevCounts[3] = liveSevCounts[3] + 1 }
                } else if ph == "entropy_done" {
                    scanDone = scanTotal
                } else if ph == "org_list" {
                    scanPhase    = "org_list"
                    orgScanPhase = "org_list"
                    orgScanTotal = 0
                    orgScanDone  = 0
                } else if ph == "org_repos" {
                    scanPhase    = "org_repos"
                    orgScanPhase = "org_repos"
                    orgScanTotal = msg["total"]
                    orgScanDone  = msg["done"]
                    orgScanRepo  = msg["repo"]
                } else if ph == "org_repo" {
                    scanPhase   = "org_repo"
                    orgScanRepo = msg["repo"]
                    orgScanIdx  = msg["repoIdx"]
                    orgScanTotal = msg["total"]
                }
            }
        }
        let res = recvNonBlock(resultCh)
        if res != null {
            scanning  = false
            scanPhase = ""

            // Check for a top-level error before touching any other fields
            let resErr = res["error"]
            if resErr != null {
                toast("Scan failed: " + str(resErr), "error", 10.0)
            } else {
                // Null-safe field reads — a missing field becomes a safe default
                let rf = res["findings"]
                if rf == null { rf = makeArray(0) }
                allFindings = rf

                let fc = res["fileCount"]
                if fc != null { fileCount = fc }

                let cc = res["commitCount"]
                if cc != null { commitCount = cc }

                let rc = res["repoCount"]
                if rc != null { repoCount = rc }

                let ts = res["totalSec"]
                if ts != null { totalSec = ts }

                let fs = res["filesSec"]
                if fs != null { filesSec = fs }

                let gs = res["gitSec"]
                if gs != null { gitSec = gs }

                let fp = res["filesPerSec"]
                if fp != null { filesPerSec = fp }

                let cp = res["commitsPerSec"]
                if cp != null { commitsPerSec = cp }

                ignoreRules = sh.loadIgnoreFile(scanPath)
                findings, suppressedCount = sh.filterFindings(allFindings, ignoreRules)
                sevAnimStart = elapsedTime()
                let nf = len(findings)
                if nf == 0 {
                    toast("Scan complete — no secrets found", "success", 5.0)
                } else {
                    let sc = sevCounts(findings)
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

    glowWhite(uiFont, "SECRET HUNTER", 18, 14, 0.95)

    fillC(theme["subtitleText"])
    textFont(uiFont, "security audit tool  //  v1.0", 20, 46, 0.60)

    let flicker = 0.04 + 0.02 * sin(t * 3.7)
    fillCA(theme["crit"], flicker)
    rect(18.0, 8.0, float(SIDE_W) - 28.0, 1.0)

    sideSep(68)

    // ── Scan controls ─────────────────────────────────────────────────────────
    uiBeginCol(10, 78, 270, 0)
    sectionLabel("SCAN PATH", uiColY())                                          uiColAdvance(18)
    fillC(theme["inputHint"])
    scanPath   = textInput(".", scanPath, uiColX(), uiColY(), uiColW(), 30, 0.62)  uiColAdvance(40)
    tooltip("Path to scan — local directory, or https://github.com/org for org-wide audit")

    if sh.isGitHubUrl(scanPath) {
        // Token field
        fillCA(theme["accentBar"], 0.60)
        noStroke()
        roundedRect(float(uiColX()), float(uiColY()) + 1.0, 3.0, 12.0, 1.5)
        fillCA(theme["dimLabel"], 0.85)
        textFont(uiFont, "GITHUB TOKEN", uiColX() + 8, uiColY(), 0.52)
        let tw = textWidth(uiFont, "GITHUB TOKEN", 0.52)
        if len(orgCfg["github_token"]) > 0 {
            fillCA(theme["low"], 0.70)
            textFont(uiFont, "(env/config)", uiColX() + 12 + tw, uiColY(), 0.48)
        }
        uiColAdvance(16)
        let tokenDisplay = settingsToken
        if len(tokenDisplay) > 8 {
            tokenDisplay = substr(tokenDisplay, 0, 4) + "••••••••" + substr(tokenDisplay, len(tokenDisplay) - 4)
        }
        settingsToken = textInput("ghp_...", settingsToken, uiColX(), uiColY(), uiColW(), 28, 0.58)
        tooltip("GitHub personal access token — or set GITHUB_TOKEN env var")
        uiColAdvance(36)
        settingsDeep = checkbox("Deep scan  (git history)", uiColX() + 2, uiColY(), settingsDeep, 0.58)
        tooltip("Deep: blobless git clone — scans full commit history. Online: tarball download — current files only.")
        uiColAdvance(26)
    } else {
        includeGit = checkbox("Include git history", uiColX() + 2, uiColY(), includeGit, 0.58)  uiColAdvance(26)
        tooltip("Also scans every git commit for leaked credentials, tokens, and API keys")
    }
    useYara    = checkbox("Use YARA rules", uiColX() + 2, uiColY(), useYara, 0.58)           uiColAdvance(26)
    tooltip("Run YARA rules after the main scan — catches binary secrets and complex patterns regex misses. Requires yara-python.")
    useEntropy = checkbox("Entropy detection", uiColX() + 2, uiColY(), useEntropy, 0.58)    uiColAdvance(26)
    tooltip("Flag high-entropy strings (≥4.5 bits/char) that match no known pattern — catches novel or custom credentials regex and YARA will miss.")
    useJWT     = checkbox("Decode JWTs", uiColX() + 2, uiColY(), useJWT, 0.58)              uiColAdvance(26)
    tooltip("For every JWT-shaped finding, decode the header + payload via the Node bridge and surface claims (sub, iss, scopes), algorithm, and expiry status. Severity is rewritten from the verdict: alg:none → CRITICAL, expired → LOW. Requires Node in PATH.")
    sectionLabel("MAX FILE SIZE (MB)", uiColY())
    fillC(theme["sliderHint"])
    drawRight(uiFont, str(int(maxSizeMB)) + " MB", 278.0, uiColY(), 0.62)       uiColAdvance(18)
    maxSizeMB  = slider("", uiColX(), uiColY(), uiColW(), float(maxSizeMB), 1.0, 100.0, 0.62)  uiColAdvance(42)
    tooltip("Files larger than this are skipped — lower values keep scans fast on large repos")
    sideSep(uiColY())                                                             uiColAdvance(8)

    // ── SCAN button ───────────────────────────────────────────────────────────
    if scanning {
        shadow(0.0, 4.0, 12.0, 0.0, 0.0, 0.0, 0.40)
        fillC(theme["scanningBg"])
        noStroke()
        roundedRect(uiColX(), uiColY(), uiColW(), 46.0, 8.0)
        noShadow()
        fillC(theme["scanningText"])
        drawCentred(uiFont, "Scanning...", float(uiColX()) + float(uiColW()) * 0.5, float(uiColY()) + 14.0, 0.70)
        if scanPhase == "enumerate" || scanPhase == "git_enumerate" {
            drawSpinner(float(uiColX()) + 62.0, float(uiColY()) + 25.0, 10.0)
        }
        if scanTotal > 0 {
            progressBar(uiColX(), uiColY() + 56, uiColW(), 8, float(scanDone), float(scanTotal))
            let pct = int(float(scanDone) / float(scanTotal) * 100.0)
            fillCA(theme["scanningText"], 0.80)
            noStroke()
            if scanPhase == "git" || scanPhase == "git_progress" {
                textFont(uiFont, str(scanDone) + " / " + str(scanTotal) + " commits  (" + str(pct) + "%)", uiColX(), uiColY() + 72, 0.55)
            } else {
                textFont(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  (" + str(pct) + "%)", uiColX(), uiColY() + 72, 0.55)
            }
        }
    } else {
        let pulse = 0.5 + 0.5 * sin(t * 2.2)
        fillCA(theme["crit"], pulse * 0.12)
        noStroke()
        roundedRect(float(uiColX()) - 4.0, float(uiColY()) - 4.0, float(uiColW()) + 8.0, 54.0, 10.0)
        if button("SCAN", uiColX(), uiColY(), uiColW(), 46, 0.80) {
            let path = scanPath
            if len(path) == 0 { path = "." }

            let pch = channel(200)
            let rch = channel(1)

            if sh.isGitHubUrl(path) {
                toast("Connecting to GitHub — listing repositories...", "info", 8.0)
                // Org scan — build a config snapshot with current UI token/mode
                let scanCfg = {
                    "github_bridge_path": orgCfg["github_bridge_path"],
                    "python_executable":  orgCfg["python_executable"],
                    "github_token":       settingsToken,
                    "default_scan_mode":  "online",
                    "temp_dir":           orgCfg["temp_dir"],
                }
                if settingsDeep == true { scanCfg["default_scan_mode"] = "deep" }
                let _orgUrl = path
                let _cfg    = scanCfg
                let _maxMB  = maxSizeMB
                let _pch    = pch
                let _rch    = rch
                async(fn() {
                    let result, err = safe(fn() { return sh.runOrgScan(_orgUrl, _cfg, _maxMB, _pch) })
                    if err != null {
                        send(_rch, {"error": err.message, "findings": makeArray(0),
                            "fileCount": 0, "commitCount": 0, "repoCount": 0,
                            "totalSec": 0.0, "filesSec": 0.0, "gitSec": 0.0,
                            "yaraSec": 0.0, "filesPerSec": 0.0, "commitsPerSec": 0.0})
                    } else {
                        send(_rch, result)
                    }
                })
            } else {
                let _scan  = sh.runScanWithProgress
                let _path  = path
                let _doGit = includeGit
                let _maxMB = maxSizeMB
                let _pch   = pch
                let _rch   = rch
                let _yara    = null
                let _entropy = useEntropy
                let _jwt     = useJWT
                if useYara { _yara = "tests/examples/SecretHunter/secrets.yar" }
                async(fn() {
                    send(_rch, _scan(_path, _doGit, _maxMB, _pch, _yara, _entropy, _jwt))
                })
            }
            progressCh  = pch
            resultCh    = rch
            scanning    = true
            scanStarted = true
            scanPhase   = "enumerate"
            scanTotal   = 0
            scanDone    = 0
            liveSevCounts   = makeArray(4, 0)
            allFindings     = makeArray(0)
            findings        = makeArray(0)
            suppressedCount = 0
            ignoreRules     = makeArray(0)
            sevAnimStart    = -1.0
            showCtxMenu     = false
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

    // ctrlEnd tracks the bottom of the controls/button area so every section
    // below it positions itself dynamically — adding or removing controls
    // never causes overlaps.
    let ctrlEnd = uiColY() + 54

    // ── Post-scan results ─────────────────────────────────────────────────────
    if scanStarted && !scanning {
        let r0 = ctrlEnd
        sideSep(r0)
        sectionLabel("RESULTS", r0 + 10)

        let lvl = threatLevel(frameCounts)
        fillC(theme["dimLabel"])
        textFont(uiFont, "THREAT LEVEL", 12, r0 + 32, 0.56)
        if lvl == "CRITICAL"      { fillC(theme["crit"])     }
        else if lvl == "HIGH"     { fillC(theme["high"])     }
        else if lvl == "ELEVATED" { fillC(theme["med"])      }
        else if lvl == "LOW"      { fillC(theme["threatLow"])}
        else                      { fillC(theme["low"])      }
        drawRight(uiFont, lvl, 278.0, r0 + 32, 0.65)

        drawThreatBar(10, r0 + 54, 270, frameCounts)

        let sevColors = [theme["crit"], theme["high"], theme["med"], theme["low"]]
        pieChart(frameCounts, sevColors, 145.0, float(r0) + 114.0, 44.0, 18.0)

        // Ease-out cubic count-up. While the tween runs we render scaled
        // integers; once finished we display the real frameCounts.
        let displayCounts = frameCounts
        if sevAnimStart >= 0.0 {
            let p = (elapsedTime() - sevAnimStart) / SEV_ANIM_DURATION
            if p < 1.0 {
                if p < 0.0 { p = 0.0 }
                let inv  = 1.0 - p
                let ease = 1.0 - inv * inv * inv
                displayCounts = [
                    int(float(frameCounts[0]) * ease),
                    int(float(frameCounts[1]) * ease),
                    int(float(frameCounts[2]) * ease),
                    int(float(frameCounts[3]) * ease),
                ]
            }
        }

        let abbrevs = ["CRIT", "HIGH", "MED", "LOW"]
        let tileW   = 60.0
        let tileH   = 32.0
        let tileY   = float(r0) + 156.0
        let i = 0
        while i < 4 {
            let tileX    = 10.0 + float(i) * 66.0
            let isActive = severityFilter == SEV_NAMES[i]
            let mx       = mouseX()
            let my       = mouseY()
            let isHover  = mx >= tileX && mx < tileX + tileW && my >= tileY && my < tileY + tileH

            if isActive {
                fillCA(sevColors[i], 0.16)
                noStroke()
                roundedRect(tileX, tileY, tileW, tileH, 5.0)
                fillCA(sevColors[i], 0.95)
                noStroke()
                roundedRect(tileX, tileY, 3.0, tileH, 1.5)
            } else if isHover {
                fillCA(sevColors[i], 0.08)
                noStroke()
                roundedRect(tileX, tileY, tileW, tileH, 5.0)
            }

            // Centre number + label horizontally inside the tile
            let numText  = str(displayCounts[i])
            let numW     = textWidth(uiFont, numText, 0.72)
            let numX     = tileX + (tileW - numW) * 0.5
            let labelW   = textWidth(uiFont, abbrevs[i], 0.48)
            let labelX   = tileX + (tileW - labelW) * 0.5

            fillCA(sevColors[i], 1.0)
            textFont(uiFont, numText, numX, float(r0) + 162.0, 0.72)
            fillCA(sevColors[i], 0.60)
            textFont(uiFont, abbrevs[i], labelX, float(r0) + 178.0, 0.48)

            if isHover && mouseClicked() && !settingsOpen {
                if isActive {
                    severityFilter = "ALL"
                } else {
                    severityFilter = SEV_NAMES[i]
                }
            }
            i = i + 1
        }

        sideSep(r0 + 200)

        uiBeginCol(12, r0 + 210, 266, 0)
        fillC(theme["statLabel"])
        textFont(uiFont, "files", uiColX(), uiColY(), 0.58)
        fillC(theme["statValue"])
        drawRight(uiFont, str(fileCount), 278.0, uiColY(), 0.62)       uiColAdvance(16)
        fillC(theme["statLabel"])
        textFont(uiFont, "commits", uiColX(), uiColY(), 0.58)
        fillC(theme["statValue"])
        drawRight(uiFont, str(commitCount), 278.0, uiColY(), 0.62)     uiColAdvance(16)
        if repoCount > 1 {
            fillC(theme["statLabel"])
            textFont(uiFont, "git repositories", uiColX(), uiColY(), 0.58)
            fillCA(theme["wAccent"], 0.90)
            drawRight(uiFont, str(repoCount), 278.0, uiColY(), 0.62)   uiColAdvance(16)
        }
        fillC(theme["statLabel"])
        textFont(uiFont, "findings", uiColX(), uiColY(), 0.58)
        fillC(theme["statValue"])
        drawRight(uiFont, str(len(findings)), 278.0, uiColY(), 0.62)         uiColAdvance(16)
        if suppressedCount > 0 {
            fillCA(theme["dimLabel"], 0.70)
            textFont(uiFont, "suppressed", uiColX(), uiColY(), 0.56)
            fillCA(theme["dimLabel"], 0.85)
            drawRight(uiFont, str(suppressedCount), 278.0, uiColY(), 0.60)
        }
    }

    // ── Filters — anchored to bottom so they never crowd the results ──────────
    let filterBase = wh - 178
    sideSep(filterBase)
    sectionLabel("FILTERS", filterBase + 10)
    uiBeginCol(10, filterBase + 34, 270, 0)
    fillC(theme["filterLabel"])
    textFont(uiFont, "Severity", uiColX() + 3, uiColY(), 0.60)              uiColAdvance(16)
    severityFilter = dropdown("", SEV_OPTIONS, uiColX(), uiColY(), uiColW(), 0.60)  uiColAdvance(38)
    tooltip("Show only findings at or above this severity — ALL shows everything")
    fillC(theme["filterLabel"])
    textFont(uiFont, "Source", uiColX() + 3, uiColY(), 0.60)                uiColAdvance(16)
    sourceFilter = dropdown("", SRC_OPTIONS, uiColX(), uiColY(), uiColW(), 0.60)
    tooltip("Show only findings from files, git history, YARA rules, or all sources combined")

    // ── Main header ───────────────────────────────────────────────────────────
    gradient(float(mainX), 0, float(mainW), float(HDR_H), [0.14, 0.13, 0.19, 1.0], theme["headerBg"], "v")

    if hasCritical {
        let alpha = 0.04 + 0.02 * sin(t * 2.8)
        fillCA(theme["crit"], alpha)
        noStroke()
        rect(float(mainX), 0.0, float(mainW), float(HDR_H))
    }

    fillC(theme["headerBorder"])
    noStroke()
    rect(float(mainX), float(HDR_H) - 1.0, float(mainW), 1.0)

    fillC(theme["mainLabel"])
    textFont(uiFont, "FINDINGS", float(mainX) + 20.0, 14.0, 0.80)

    // Settings button — top-right of the left panel header, nudged a touch
    // further left so the SECRET HUNTER title isn't right up against it.
    if button("CFG", SIDE_W - 68, 11, 40, 26, 0.58) {
        settingsOpen       = true
        settingsJustOpened = true
        settingsToken  = orgCfg["github_token"]
        settingsBridge = orgCfg["github_bridge_path"]
        settingsPython = orgCfg["python_executable"]
        settingsDeep   = orgCfg["default_scan_mode"] == "deep"
        settingsTmp    = orgCfg["temp_dir"]
    }
    tooltip("Configure bridge path, GitHub token, and scan defaults")

    // Threat level badge in header (post-scan)
    if scanStarted && !scanning {
        let nf  = len(findings)
        let lvl = threatLevel(frameCounts)
        if nf == 0 {
            fillC(theme["low"])
            let badgeTxt = "CLEAN"
        } else if frameCounts[0] > 0 {
            fillC(theme["crit"])
            let badgeTxt = lvl
        } else if frameCounts[1] > 0 {
            fillC(theme["high"])
            let badgeTxt = lvl
        } else if frameCounts[2] > 0 {
            fillC(theme["med"])
            let badgeTxt = lvl
        } else {
            fillC(theme["low"])
            let badgeTxt = lvl
        }
        noStroke()
        let bsc  = 0.64
        let btw  = textWidth(uiFont, badgeTxt, bsc) + 22.0
        let bth  = 26.0
        let btr  = bth * 0.5
        let btx  = float(mainX) + float(mainW) - btw - 20.0
        let bty  = (float(HDR_H) - bth) * 0.5

        if nf > 0 && frameCounts[0] > 0 {
            let ba = 0.08 + 0.06 * sin(t * 3.0)
            fillCA(theme["crit"], ba)
            roundedRect(btx - 4.0, bty - 4.0, btw + 8.0, bth + 8.0, btr + 4.0)
        }

        if nf == 0 { fillC(theme["low"]) }
        else if frameCounts[0] > 0 { fillC(theme["crit"]) }
        else if frameCounts[1] > 0 { fillC(theme["high"]) }
        else if frameCounts[2] > 0 { fillC(theme["med"])  }
        else { fillC(theme["low"]) }
        noStroke()
        shadow(2.0, 3.0, 10.0, 0.0, 0.0, 0.0, 0.45)
        roundedRect(btx, bty, btw, bth, btr)
        noShadow()
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

        } else if scanPhase == "yara" || scanPhase == "yara_done" {
            fillC(theme["scanProgress"])
            drawCentred(uiFont, "YARA rules scanning", midX, midY - 50.0, 0.74)
            fillC(theme["scanSub"])
            drawCentred(uiFont, "deep pattern analysis across all files", midX, midY - 28.0, 0.62)

        } else if scanPhase == "entropy" || scanPhase == "entropy_done" {
            fillC(theme["scanProgress"])
            drawCentred(uiFont, "Entropy detection", midX, midY - 50.0, 0.74)
            fillC(theme["scanSub"])
            drawCentred(uiFont, "scanning for high-entropy strings", midX, midY - 28.0, 0.62)
            if scanTotal > 0 {
                let barW = int(float(mainW) * 0.58)
                let barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(scanDone), float(scanTotal))
                let pct = int(float(scanDone) / float(scanTotal) * 100.0)
                fillC(theme["progressText"])
                drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
            } else {
                drawSpinner(midX, midY, 20.0)
            }

        } else if scanPhase == "files" || scanPhase == "files_progress" ||
                  scanPhase == "git"   || scanPhase == "git_progress" {
            let isGit = (scanPhase == "git" || scanPhase == "git_progress")
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
                let barW = int(float(mainW) * 0.58)
                let barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(scanDone), float(scanTotal))
                let pct = int(float(scanDone) / float(scanTotal) * 100.0)
                fillC(theme["progressText"])
                if isGit {
                    drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " commits  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
                } else {
                    drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
                }
            } else {
                drawSpinner(midX, midY, 20.0)
            }
        } else if scanPhase == "org_list" {
            drawSpinner(midX, midY - 44.0, 26.0)
            fillC(theme["scanStatus"])
            drawCentred(uiFont, "Listing repositories...", midX, midY + 0.0, 0.72)
            fillC(theme["scanSub"])
            drawCentred(uiFont, "contacting GitHub API", midX, midY + 22.0, 0.60)

        } else if scanPhase == "org_repos" || scanPhase == "org_repo" {
            fillC(theme["scanProgress"])
            drawCentred(uiFont, "Scanning org repo " + str(orgScanIdx + 1) + " of " + str(orgScanTotal), midX, midY - 50.0, 0.74)
            fillC(theme["scanSub"])
            drawCentred(uiFont, orgScanRepo, midX, midY - 28.0, 0.62)
            if orgScanTotal > 0 {
                let barW = int(float(mainW) * 0.58)
                let barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(orgScanDone), float(orgScanTotal))
                fillC(theme["progressText"])
                let pct = int(float(orgScanDone) / float(orgScanTotal) * 100.0)
                drawCentred(uiFont, str(orgScanDone) + " / " + str(orgScanTotal) + " repos done  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
            }

        } else {
            drawSpinner(midX, midY - 24.0, 22.0)
        }

    } else if len(findings) == 0 {
        let ckx = midX - 10.0
        let cky = midY - 62.0
        let gA  = 0.65 + 0.12 * sin(t * 1.8)
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

        let lx = float(mainX) + float(mainW) - 6.0
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

        let treeY  = areaY + 22
        let remH   = 0
        if selectedFinding >= 0 {
            remH = 228
            // Grow the panel when the selected finding carries decoded JWT
            // claims so the JWT card has room without crowding remediation.
            if selectedFinding < len(findings) {
                let _selF = findings[selectedFinding]
                if _selF["jwt"] != null { remH = remH + 86 }
            }
        }
        let treeH  = areaH - 22 - remH

        // ── Findings tree ─────────────────────────────────────────────────────
        let newKey = str(len(findings)) + ":" + severityFilter + ":" + sourceFilter
        if newKey != treeKey {
            let built        = buildFindingTree(findings, severityFilter, sourceFilter)
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
        } else if settingsOpen {
            // Modal is up — don't run the interactive tree widget. It owns its
            // own mouse handling internally, so calling it here would change the
            // selection through the dialog. The dim overlay below the modal makes
            // this absence read as "background is inactive" rather than missing.
        } else {
            let res          = treeView(mainX, treeY, mainW, treeH, treeLabels, treeLevels, treeExpanded, 0.60)
            let newSel       = res[0]
            treeExpanded = res[1]

            let mx     = mouseX()
            let my     = mouseY()
            let inTree = mx >= float(mainX) && mx <= float(mainX + mainW) &&
                     my >= float(treeY)  && my <= float(treeY + treeH)
            // Freeze tree-row input while the settings modal is open so
            // clicks on SAVE/CANCEL don't leak through to the rows behind.
            if mouseClicked() && inTree && !settingsOpen {
                let tt = elapsedTime()
                if newSel == lastClickRow && tt - lastClickTime < 0.35 {
                    if newSel >= 0 && newSel < len(treeNodeFind) {
                        let fi = treeNodeFind[newSel]
                        if fi >= 0 {
                            let fpath = findings[fi]["file"]
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

            // Right-click on a finding → suppress context menu
            if mouseRightClicked() && inTree && selectedFinding >= 0 && !settingsOpen {
                showCtxMenu  = true
                ctxMenuX     = int(mx)
                ctxMenuY     = int(my)
                ctxFindingIdx = selectedFinding
            }

            treeSelected = newSel
            if treeSelected >= 0 && treeSelected < len(treeNodeFind) {
                let fi = treeNodeFind[treeSelected]
                if fi >= 0 { selectedFinding = fi }
            }
        }

        // ── Remediation panel (shown when a finding is selected) ──────────────
        // Suppressed while the settings modal is open so its COPY buttons
        // don't catch clicks meant for the dialog.
        if selectedFinding >= 0 && selectedFinding < len(findings) && !settingsOpen {
            let f    = findings[selectedFinding]
            let py   = wh - 228 - footerH   // constant — avoids first-frame remH=0 issue
            let pw   = mainW
            let px   = mainX
            let pPad = 14

            // Background + top border
            gradient(float(px), float(py), float(pw), float(remH), theme["headerBg"], theme["bg"], "v")
            fillC(theme["sep"])
            noStroke()
            rect(float(px), float(py), float(pw), 1.0)
            fillCA(theme["accentBar"], 0.15)
            rect(float(px), float(py), float(pw), 1.0)

            // ── Finding header ────────────────────────────────────────────────
            sevFill(f["severity"])
            noStroke()
            roundedRect(float(px + pPad), float(py + 8), 62.0, 18.0, 4.0)
            fillC(theme["bg"])
            textFont(uiFont, f["severity"], px + pPad + 6, py + 11, 0.52)

            fillC(theme["mainLabel"])
            let pat = f["patternName"]
            let parenIdx = indexOf(pat, " (")
            if parenIdx >= 0 { pat = substr(pat, 0, parenIdx) }
            textFont(uiFont, pat, px + pPad + 72, py + 11, 0.60)

            // Location
            fillCA(theme["dimLabel"], 0.75)
            let loc = f["file"]
            if f["source"] == "git" {
                let commit = f["commit"]
                if len(commit) > 7 { commit = substr(commit, 0, 7) }
                loc = commit + "  " + f["author"]
            }
            if len(loc) > 0 {
                let locTxt = sh.fitText(loc, 55)
                textFont(uiFont, locTxt, px + pPad + 72, py + 25, 0.52)
            }

            // ── JWT claims (Node bridge enrichment) ──────────────────────────
            // Shown only when enrichWithJWT() attached a decoded payload to
            // this finding. Surfaces alg + warnings, status (expired / no
            // exp / valid), and the high-signal claims (sub / iss / scopes)
            // so the user can decide rotation priority at a glance.
            let jwtOffset = 0
            let jwtInfo = f["jwt"]
            if jwtInfo != null {
                let jwtY = py + 46
                fillCA(theme["accentBar"], 0.90)
                rect(float(px + pPad), float(jwtY), 2.0, 12.0)
                fillC(theme["sectionText"])
                textFont(uiFont, "JWT CLAIMS", px + pPad + 8, jwtY, 0.52)

                let jwtDecodeErr = jwtInfo["error"]
                if jwtDecodeErr != null {
                    fillCA(theme["dimLabel"], 0.80)
                    textFont(uiFont, "decode error: " + jwtDecodeErr, px + pPad + 8, jwtY + 14, 0.52)
                } else {
                    // Line 1 — alg + warning badge if any
                    fillCA(theme["mainLabel"], 0.90)
                    textFont(uiFont, "alg: " + jwtInfo["alg"], px + pPad + 8, jwtY + 14, 0.56)

                    let algW = jwtInfo["alg_warning"]
                    if algW != null {
                        // Critical for alg:none (forgeable), warning for HS-family
                        let algIsNone = jwtInfo["alg"] == "none"
                        if algIsNone { fillCA(theme["crit"], 0.85) }
                        else         { fillCA(theme["high"], 0.85) }
                        textFont(uiFont, "⚠ " + sh.fitText(algW, 60), px + pPad + 70, jwtY + 14, 0.50)
                    }

                    // Status pill — EXPIRED / NO EXPIRY / EXPIRES SOON / VALID
                    let statusLabel = "VALID"
                    let statusFill  = theme["low"]
                    if jwtInfo["expired"] {
                        statusLabel = "EXPIRED"
                        statusFill  = theme["dimLabel"]
                    } else if jwtInfo["missing_exp"] {
                        statusLabel = "NO EXPIRY"
                        statusFill  = theme["high"]
                    } else if jwtInfo["expires_soon"] {
                        statusLabel = "EXPIRES SOON"
                        statusFill  = theme["med"]
                    }
                    let statusW = textWidth(uiFont, statusLabel, 0.48) + 14.0
                    fillCA(statusFill, 0.75)
                    noStroke()
                    roundedRect(float(pw + px - pPad) - statusW, float(jwtY) + 2.0, statusW, 14.0, 4.0)
                    fillC(theme["bg"])
                    textFont(uiFont, statusLabel, int(float(pw + px - pPad) - statusW + 7.0), jwtY + 4, 0.48)

                    // Line 2 — sub / iss
                    fillCA(theme["dimLabel"], 0.85)
                    let subLine = ""
                    if len(jwtInfo["sub"]) > 0 { subLine = "sub: " + jwtInfo["sub"] }
                    if len(jwtInfo["iss"]) > 0 {
                        if len(subLine) > 0 { subLine = subLine + "  ·  " }
                        subLine = subLine + "iss: " + jwtInfo["iss"]
                    }
                    if len(subLine) > 0 {
                        textFont(uiFont, sh.fitText(subLine, 70), px + pPad + 8, jwtY + 30, 0.52)
                    }

                    // Line 3 — scopes (or exp date when no scopes)
                    let scopes = jwtInfo["scopes"]
                    if len(scopes) > 0 {
                        let scopeStr = "scopes: " + scopes[0]
                        let si = 1
                        while si < len(scopes) && si < 5 {
                            scopeStr = scopeStr + " " + scopes[si]
                            si = si + 1
                        }
                        fillCA(theme["mainLabel"], 0.75)
                        textFont(uiFont, sh.fitText(scopeStr, 70), px + pPad + 8, jwtY + 46, 0.52)
                    } else if len(jwtInfo["exp_iso"]) > 0 {
                        fillCA(theme["dimLabel"], 0.70)
                        textFont(uiFont, "exp: " + jwtInfo["exp_iso"], px + pPad + 8, jwtY + 46, 0.52)
                    }
                }
                jwtOffset = 86
            }

            // ── Immediate action ──────────────────────────────────────────────
            let secY = py + 46 + jwtOffset
            fillCA(theme["accentBar"], 0.90)
            rect(float(px + pPad), float(secY), 2.0, 12.0)
            fillC(theme["sectionText"])
            textFont(uiFont, "IMMEDIATE ACTION", px + pPad + 8, secY, 0.52)

            fillCA(theme["mainLabel"], 0.85)
            let action = sh.fitText(f["action"], int(float(pw - pPad * 2 - 10) / 6.0))
            textFont(uiFont, action, px + pPad + 8, secY + 14, 0.56)

            // ── Git purge command ─────────────────────────────────────────────
            let gitY = secY + 36
            fillCA(theme["accentBar"], 0.90)
            rect(float(px + pPad), float(gitY), 2.0, 12.0)
            fillC(theme["sectionText"])
            textFont(uiFont, "PURGE FROM GIT HISTORY", px + pPad + 8, gitY, 0.52)
            tooltip("Rewrites git history — coordinate with your team before force-pushing")

            let gitCmd = buildGitCmd(f, scanPath)
            let cmdLines = split(gitCmd, "\n")
            fillCA(theme["wInputBg"], 0.80)
            noStroke()
            roundedRect(float(px + pPad), float(gitY + 16), float(pw - pPad * 2 - 70), float(len(cmdLines) * 13 + 8), 3.0)
            fillCA(theme["scanLine"], 0.85)
            let li = 0
            while li < len(cmdLines) {
                let lc = cmdLines[li]
                if startsWith(lc, "#") { fillCA(theme["dimLabel"], 0.65) }
                else { fillCA(theme["scanLine"], 0.90) }
                textFont(uiFont, lc, px + pPad + 6, gitY + 20 + li * 13, 0.48)
                li = li + 1
            }
            if button("COPY", pw + px - pPad - 58, gitY + 16, 54, 22, 0.52) {
                copyToClipboard(gitCmd)
                toast("Git commands copied to clipboard", "success", 2.5)
            }

            // ── Env var hint (where applicable) ──────────────────────────────
            let envHint = buildEnvHint(f)
            if len(envHint) > 0 {
                let envY = gitY + len(cmdLines) * 13 + 32
                if envY + 50 < py + remH - 8 {
                    fillCA(theme["accentBar"], 0.90)
                    rect(float(px + pPad), float(envY), 2.0, 12.0)
                    fillC(theme["sectionText"])
                    textFont(uiFont, "REPLACE WITH ENV VAR", px + pPad + 8, envY, 0.52)
                    let envLines = split(envHint, "\n")
                    fillCA(theme["wInputBg"], 0.80)
                    noStroke()
                    roundedRect(float(px + pPad), float(envY + 16), float(pw - pPad * 2 - 70), float(len(envLines) * 13 + 8), 3.0)
                    fillCA(theme["scanLine"], 0.80)
                    let el = 0
                    while el < len(envLines) {
                        textFont(uiFont, envLines[el], px + pPad + 6, envY + 20 + el * 13, 0.48)
                        el = el + 1
                    }
                    if button("COPY", pw + px - pPad - 58, envY + 16, 54, 22, 0.52) {
                        copyToClipboard(envHint)
                        toast("Env var snippet copied to clipboard", "success", 2.5)
                    }
                }
            }
        }
    }

    // ── Performance footer (main area bottom, post-scan only) ────────────────
    if scanStarted && !scanning {
        let fy = wh - 40

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
        let totalStr = format("%.2f's  total", totalSec)
        textFont(uiFont, totalStr, uiRowX(), uiRowY(), 0.65)
        uiRowAdvance(textWidth(uiFont, totalStr, 0.65) + 16.0)
        fDiv(uiRowX())
        uiRowAdvance(14)

        if filesPerSec >= 1000.0 {
            let ftpStr = format("%.1f k files/s", filesPerSec / 1000.0)
        } else {
            let ftpStr = format("%.0f files/s", filesPerSec)
        }
        fillC(theme["footerStat"])
        textFont(uiFont, ftpStr, uiRowX(), uiRowY(), 0.65)
        uiRowAdvance(textWidth(uiFont, ftpStr, 0.65) + 16.0)

        if includeGit && commitCount > 0 {
            fDiv(uiRowX())
            uiRowAdvance(14)
            fillC(theme["footerStat"])
            let gitTpStr = format("%.0f commits/s", commitsPerSec)
            textFont(uiFont, gitTpStr, uiRowX(), uiRowY(), 0.65)
            uiRowAdvance(textWidth(uiFont, gitTpStr, 0.65) + 16.0)
            if repoCount > 1 {
                fDiv(uiRowX())
                uiRowAdvance(14)
                fillCA(theme["wAccent"], 0.85)
                let repoStr = str(repoCount) + " repos"
                textFont(uiFont, repoStr, uiRowX(), uiRowY(), 0.65)
            }
        }
    }

    // ── Suppress context menu (right-click on a finding) ─────────────────────
    if showCtxMenu && ctxFindingIdx >= 0 && ctxFindingIdx < len(findings) {
        let f = findings[ctxFindingIdx]
        let hasFile = len(f["file"]) > 0 && f["source"] != "git"
        let menuItems = makeArray(3, "")
        menuItems[0] = "Suppress pattern everywhere"
        if hasFile {
            menuItems[1] = "Suppress pattern in this file"
            menuItems[2] = "Suppress all findings in this file"
        } else {
            menuItems[1] = ""
            menuItems[2] = ""
        }
        // Trim to non-empty items
        let itemCount = 1
        if hasFile { itemCount = 3 }
        let ctxItems = makeArray(itemCount, "")
        ctxItems[0] = menuItems[0]
        if hasFile {
            ctxItems[1] = menuItems[1]
            ctxItems[2] = menuItems[2]
        }

        let sel = contextMenu(ctxMenuX, ctxMenuY, ctxItems, showCtxMenu, 0.60)
        if sel >= 0 {
            let ruleType = "pattern"
            if sel == 1 { ruleType = "file_pattern" }
            if sel == 2 { ruleType = "file" }
            let rule = sh.makeIgnoreRule(f, ruleType)
            sh.appendIgnoreRule(scanPath, rule)
            // Add to in-memory rules and re-filter immediately
            let newRules = makeArray(len(ignoreRules) + 1, "")
            let ri = 0
            while ri < len(ignoreRules) { newRules[ri] = ignoreRules[ri]  ri = ri + 1 }
            newRules[len(ignoreRules)] = rule
            ignoreRules = newRules
            findings, suppressedCount = sh.filterFindings(allFindings, ignoreRules)
            selectedFinding = -1
            treeKey = ""
            showCtxMenu = false
            toast("Suppressed: " + rule, "success", 4.0)
        } else if sel == -2 {
            showCtxMenu = false
        }
    }

    // ── Settings modal ────────────────────────────────────────────────────────
    if settingsOpen {
        // Full-window dim shade so the inactive background reads as
        // "frozen / not interactive" and the dialog has visual focus.
        fillCA([0.0, 0.0, 0.0, 1.0], 0.55)
        noStroke()
        rect(0.0, 0.0, float(ww), float(wh))

        let mw = 600
        let mh = 295
        let mx = mainX + (mainW - mw) / 2
        let my = (wh - mh) / 2

        // Panel background
        shadow(2.0, 6.0, 24.0, 0.0, 0.0, 0.0, 0.55)
        fillC(theme["panelBg"])
        stroke(theme["wAccent"][0], theme["wAccent"][1], theme["wAccent"][2], 0.55)
        roundedRect(float(mx), float(my), float(mw), float(mh), 8.0)
        noShadow()

        // Title bar
        fillC(theme["headerBg"])
        noStroke()
        rect(float(mx) + 1.0, float(my) + 1.0, float(mw) - 2.0, 28.0)
        fillC(theme["wAccent"])
        rect(float(mx) + 8.0, float(my) + 1.0, float(mw) - 16.0, 2.0)
        fillC(theme["mainLabel"])
        textFont(uiFont, "SETTINGS", mx + 16, my + 7, 0.72)

        // Use explicit mx/my-relative positions — never column cursor state,
        // which reflects wherever the sidebar left off, not the modal origin.
        let sPad = mx + 18
        let sW   = mw - 36
        let cy   = my + 36

        fillCA(theme["dimLabel"], 0.80)
        textFont(uiFont, "Bridge script path", sPad, cy, 0.55)
        cy = cy + 16
        settingsBridge = textInput("", settingsBridge, sPad, cy, sW, 28, 0.56)
        tooltip("Absolute or relative path to github_bridge.py — override with SECRETHUNTER_BRIDGE env var")
        cy = cy + 36

        fillCA(theme["dimLabel"], 0.80)
        textFont(uiFont, "Python executable", sPad, cy, 0.55)
        cy = cy + 16
        settingsPython = textInput("", settingsPython, sPad, cy, int(float(sW) * 0.44), 28, 0.56)
        tooltip("python3, python, or full path — override with SECRETHUNTER_PYTHON env var")
        cy = cy + 36

        fillCA(theme["dimLabel"], 0.80)
        textFont(uiFont, "GitHub token  (stored locally, never transmitted)", sPad, cy, 0.55)
        cy = cy + 16
        settingsToken = textInput("", settingsToken, sPad, cy, sW, 28, 0.56)
        tooltip("Personal access token with repo scope — or set GITHUB_TOKEN env var")
        cy = cy + 36

        settingsDeep = checkbox("Deep scan  (blobless git clone, includes full history)", sPad + 2, cy, settingsDeep, 0.56)

        // Save / Cancel buttons — widths generous so auto-sizing never expands past the modal edge
        let btnY = my + mh - 46
        if button("SAVE", mx + mw - 196, btnY, 86, 30, 0.64) {
            orgCfg["github_bridge_path"] = settingsBridge
            orgCfg["python_executable"]  = settingsPython
            orgCfg["github_token"]       = settingsToken
            if settingsDeep == true {
                orgCfg["default_scan_mode"] = "deep"
            } else {
                orgCfg["default_scan_mode"] = "online"
            }
            sh.saveSecretHunterConfig(orgCfg)
            settingsOpen = false
            toast("Settings saved to ~/.secrethunter/config", "success", 3.0)
        }
        if button("CANCEL", mx + mw - 102, btnY, 90, 30, 0.64) {
            settingsOpen = false
        }

        // Block all background input while modal is open.
        // settingsJustOpened suppresses the close check on the opening frame
        // so the same click that opens the modal doesn't immediately close it.
        if settingsJustOpened {
            settingsJustOpened = false
        } else if mouseClicked() || mouseRightClicked() {
            let mx2 = mouseX()
            let my2 = mouseY()
            let inModal = mx2 >= float(mx) && mx2 <= float(mx + mw) &&
                      my2 >= float(my) && my2 <= float(my + mh)
            if inModal == false { settingsOpen = false }
        }
    }

    uiEnd()
})
