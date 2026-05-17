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

// ── Remediation helpers ───────────────────────────────────────────────────────

fn copyToClipboard(text) {
    writeFile("/tmp/.secrethunter_clip", text)
    _processExec("bash", ["-c", "cat /tmp/.secrethunter_clip | pbcopy 2>/dev/null || xclip -selection clipboard < /tmp/.secrethunter_clip 2>/dev/null || xsel --clipboard < /tmp/.secrethunter_clip 2>/dev/null"])
}

fn buildGitCmd(f, root) {
    file   = f["file"]
    src    = f["source"]
    commit = f["commit"]
    out = "cd '" + root + "'"
    if src == "git" && len(commit) >= 7 {
        sha = substr(commit, 0, 7)
        author = f["author"]
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
    pat = lower(f["patternName"])
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

scanPath    = "."
includeGit  = true
useYara     = false
useEntropy  = false
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
allFindings      = makeArray(0)
ignoreRules      = makeArray(0)
suppressedCount  = 0
showCtxMenu      = false
ctxMenuX         = 0
ctxMenuY         = 0
ctxFindingIdx    = -1
scanPhase        = ""
scanTotal        = 0
scanDone         = 0
scanRepoCount    = 0

// ── GitHub org scan state ─────────────────────────────────────────────────────

orgCfg         = sh.loadSecretHunterConfig()
settingsOpen       = false
settingsJustOpened = false
settingsToken  = orgCfg["github_token"]
settingsBridge = orgCfg["github_bridge_path"]
settingsPython = orgCfg["python_executable"]
settingsDeep   = orgCfg["default_scan_mode"] == "deep"
settingsTmp    = orgCfg["temp_dir"]
orgScanRepo    = ""
orgScanIdx     = 0
orgScanTotal   = 0
orgScanDone    = 0
orgScanPhase   = ""

// ── Filter + tree-view state ──────────────────────────────────────────────────

SEV_OPTIONS    = ["ALL", "CRITICAL", "HIGH", "MEDIUM", "LOW"]
SEV_NAMES      = ["CRITICAL", "HIGH", "MEDIUM", "LOW"]
SRC_OPTIONS    = ["ALL", "FILES", "GIT", "YARA", "ENTROPY"]
severityFilter = "ALL"
sourceFilter   = "ALL"

// Sidebar count-up animation — set to elapsedTime() at scan complete; -1 = idle
SEV_ANIM_DURATION = 0.55
sevAnimStart      = -1.0
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
                (srcF == "FILES"   && f["source"] == "file")    ||
                (srcF == "GIT"    && f["source"] == "git")     ||
                (srcF == "YARA"   && f["source"] == "yara")    ||
                (srcF == "ENTROPY" && f["source"] == "entropy")
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
        ci = 0
        while ci < filtCount {
            origIdx = filtIdxs[ci]
            f = flist[origIdx]
            if f["file"] == fname && f["source"] == fsrc {
                if f["source"] == "file" {
                    locPart = "line " + str(f["line"])
                } else if f["source"] == "yara" {
                    locPart = "yara"
                } else if f["source"] == "entropy" {
                    locPart = "entropy"
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
    roundedRect(bx, by, bw, bh, br)
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
    total = float(counts[0] + counts[1] + counts[2] + counts[3])
    if total <= 0.0 { return }
    const barH = 10.0
    const barR = 4.0
    fillC(theme["threatBarBg"])
    noStroke()
    roundedRect(float(x), float(y), float(w), barH, barR)
    cx = float(x)
    if counts[0] > 0 {
        sw = float(w) * float(counts[0]) / total
        fillC(theme["crit"])
        noStroke()
        roundedRect(cx, float(y), sw, barH, barR)
        cx = cx + sw
    }
    if counts[1] > 0 {
        sw = float(w) * float(counts[1]) / total
        fillC(theme["high"])
        noStroke()
        rect(cx, float(y), sw, barH)
        cx = cx + sw
    }
    if counts[2] > 0 {
        sw = float(w) * float(counts[2]) / total
        fillC(theme["med"])
        noStroke()
        rect(cx, float(y), sw, barH)
        cx = cx + sw
    }
    if counts[3] > 0 {
        sw = float(w) * float(counts[3]) / total
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
                } else if ph == "yara" {
                    scanPhase = "yara"
                    scanTotal = msg["total"]
                    scanDone  = 0
                } else if ph == "yara_done" {
                    scanDone = scanTotal
                } else if ph == "entropy" {
                    scanPhase = "entropy"
                    scanTotal = msg["total"]
                    scanDone  = 0
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
        res = recvNonBlock(resultCh)
        if res != null {
            scanning  = false
            scanPhase = ""

            // Check for a top-level error before touching any other fields
            resErr = res["error"]
            if resErr != null {
                toast("Scan failed: " + str(resErr), "error", 10.0)
            } else {
                // Null-safe field reads — a missing field becomes a safe default
                rf = res["findings"]
                if rf == null { rf = makeArray(0) }
                allFindings = rf

                fc = res["fileCount"]
                if fc != null { fileCount = fc }

                cc = res["commitCount"]
                if cc != null { commitCount = cc }

                rc = res["repoCount"]
                if rc != null { repoCount = rc }

                ts = res["totalSec"]
                if ts != null { totalSec = ts }

                fs = res["filesSec"]
                if fs != null { filesSec = fs }

                gs = res["gitSec"]
                if gs != null { gitSec = gs }

                fp = res["filesPerSec"]
                if fp != null { filesPerSec = fp }

                cp = res["commitsPerSec"]
                if cp != null { commitsPerSec = cp }

                ignoreRules = sh.loadIgnoreFile(scanPath)
                findings, suppressedCount = sh.filterFindings(allFindings, ignoreRules)
                sevAnimStart = elapsedTime()
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

    flicker = 0.04 + 0.02 * sin(t * 3.7)
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
        tw = textWidth(uiFont, "GITHUB TOKEN", 0.52)
        if len(orgCfg["github_token"]) > 0 {
            fillCA(theme["low"], 0.70)
            textFont(uiFont, "(env/config)", uiColX() + 12 + tw, uiColY(), 0.48)
        }
        uiColAdvance(16)
        tokenDisplay = settingsToken
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

            if sh.isGitHubUrl(path) {
                toast("Connecting to GitHub — listing repositories...", "info", 8.0)
                // Org scan — build a config snapshot with current UI token/mode
                scanCfg = {
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
                    result, err = safe(fn() { return sh.runOrgScan(_orgUrl, _cfg, _maxMB, _pch) })
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
                if useYara { _yara = "tests/examples/SecretHunter/secrets.yar" }
                async(fn() {
                    send(_rch, _scan(_path, _doGit, _maxMB, _pch, _yara, _entropy))
                })
            }
            progressCh  = pch
            resultCh    = rch
            scanning    = true
            scanStarted = true
            scanPhase   = "enumerate"
            scanTotal   = 0
            scanDone    = 0
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
    ctrlEnd = uiColY() + 54

    // ── Post-scan results ─────────────────────────────────────────────────────
    if scanStarted && !scanning {
        r0 = ctrlEnd
        sideSep(r0)
        sectionLabel("RESULTS", r0 + 10)

        lvl = threatLevel(frameCounts)
        fillC(theme["dimLabel"])
        textFont(uiFont, "THREAT LEVEL", 12, r0 + 32, 0.56)
        if lvl == "CRITICAL"      { fillC(theme["crit"])     }
        else if lvl == "HIGH"     { fillC(theme["high"])     }
        else if lvl == "ELEVATED" { fillC(theme["med"])      }
        else if lvl == "LOW"      { fillC(theme["threatLow"])}
        else                      { fillC(theme["low"])      }
        drawRight(uiFont, lvl, 278.0, r0 + 32, 0.65)

        drawThreatBar(10, r0 + 54, 270, frameCounts)

        sevColors = [theme["crit"], theme["high"], theme["med"], theme["low"]]
        pieChart(frameCounts, sevColors, 145.0, float(r0) + 114.0, 44.0, 18.0)

        // Ease-out cubic count-up. While the tween runs we render scaled
        // integers; once finished we display the real frameCounts.
        displayCounts = frameCounts
        if sevAnimStart >= 0.0 {
            p = (elapsedTime() - sevAnimStart) / SEV_ANIM_DURATION
            if p < 1.0 {
                if p < 0.0 { p = 0.0 }
                inv  = 1.0 - p
                ease = 1.0 - inv * inv * inv
                displayCounts = [
                    int(float(frameCounts[0]) * ease),
                    int(float(frameCounts[1]) * ease),
                    int(float(frameCounts[2]) * ease),
                    int(float(frameCounts[3]) * ease),
                ]
            }
        }

        abbrevs = ["CRIT", "HIGH", "MED", "LOW"]
        tileW   = 60.0
        tileH   = 32.0
        tileY   = float(r0) + 156.0
        i = 0
        while i < 4 {
            tileX    = 10.0 + float(i) * 66.0
            isActive = severityFilter == SEV_NAMES[i]
            mx       = mouseX()
            my       = mouseY()
            isHover  = mx >= tileX && mx < tileX + tileW && my >= tileY && my < tileY + tileH

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
            numText  = str(displayCounts[i])
            numW     = textWidth(uiFont, numText, 0.72)
            numX     = tileX + (tileW - numW) * 0.5
            labelW   = textWidth(uiFont, abbrevs[i], 0.48)
            labelX   = tileX + (tileW - labelW) * 0.5

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
    filterBase = wh - 178
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

    // Settings button — top-right of the left panel header
    if button("CFG", SIDE_W - 50, 11, 40, 26, 0.58) {
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
                barW = int(float(mainW) * 0.58)
                barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(scanDone), float(scanTotal))
                pct = int(float(scanDone) / float(scanTotal) * 100.0)
                fillC(theme["progressText"])
                drawCentred(uiFont, str(scanDone) + " / " + str(scanTotal) + " files  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
            } else {
                drawSpinner(midX, midY, 20.0)
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
                barW = int(float(mainW) * 0.58)
                barX = int(midX) - barW / 2
                progressBar(barX, int(midY) - 16, barW, 16, float(orgScanDone), float(orgScanTotal))
                fillC(theme["progressText"])
                pct = int(float(orgScanDone) / float(orgScanTotal) * 100.0)
                drawCentred(uiFont, str(orgScanDone) + " / " + str(orgScanTotal) + " repos done  —  " + str(pct) + "%", midX, midY + 14.0, 0.64)
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

        treeY  = areaY + 22
        remH   = 0
        if selectedFinding >= 0 { remH = 228 }
        treeH  = areaH - 22 - remH

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
        } else if settingsOpen {
            // Modal is up — don't run the interactive tree widget. It owns its
            // own mouse handling internally, so calling it here would change the
            // selection through the dialog. The dim overlay below the modal makes
            // this absence read as "background is inactive" rather than missing.
        } else {
            res          = treeView(mainX, treeY, mainW, treeH, treeLabels, treeLevels, treeExpanded, 0.60)
            newSel       = res[0]
            treeExpanded = res[1]

            mx     = mouseX()
            my     = mouseY()
            inTree = mx >= float(mainX) && mx <= float(mainX + mainW) &&
                     my >= float(treeY)  && my <= float(treeY + treeH)
            // Freeze tree-row input while the settings modal is open so
            // clicks on SAVE/CANCEL don't leak through to the rows behind.
            if mouseClicked() && inTree && !settingsOpen {
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

            // Right-click on a finding → suppress context menu
            if mouseRightClicked() && inTree && selectedFinding >= 0 && !settingsOpen {
                showCtxMenu  = true
                ctxMenuX     = int(mx)
                ctxMenuY     = int(my)
                ctxFindingIdx = selectedFinding
            }

            treeSelected = newSel
            if treeSelected >= 0 && treeSelected < len(treeNodeFind) {
                fi = treeNodeFind[treeSelected]
                if fi >= 0 { selectedFinding = fi }
            }
        }

        // ── Remediation panel (shown when a finding is selected) ──────────────
        // Suppressed while the settings modal is open so its COPY buttons
        // don't catch clicks meant for the dialog.
        if selectedFinding >= 0 && selectedFinding < len(findings) && !settingsOpen {
            f    = findings[selectedFinding]
            py   = wh - 228 - footerH   // constant — avoids first-frame remH=0 issue
            pw   = mainW
            px   = mainX
            pPad = 14

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
            pat = f["patternName"]
            parenIdx = indexOf(pat, " (")
            if parenIdx >= 0 { pat = substr(pat, 0, parenIdx) }
            textFont(uiFont, pat, px + pPad + 72, py + 11, 0.60)

            // Location
            fillCA(theme["dimLabel"], 0.75)
            loc = f["file"]
            if f["source"] == "git" {
                commit = f["commit"]
                if len(commit) > 7 { commit = substr(commit, 0, 7) }
                loc = commit + "  " + f["author"]
            }
            if len(loc) > 0 {
                locTxt = sh.fitText(loc, 55)
                textFont(uiFont, locTxt, px + pPad + 72, py + 25, 0.52)
            }

            // ── Immediate action ──────────────────────────────────────────────
            secY = py + 46
            fillCA(theme["accentBar"], 0.90)
            rect(float(px + pPad), float(secY), 2.0, 12.0)
            fillC(theme["sectionText"])
            textFont(uiFont, "IMMEDIATE ACTION", px + pPad + 8, secY, 0.52)

            fillCA(theme["mainLabel"], 0.85)
            action = sh.fitText(f["action"], int(float(pw - pPad * 2 - 10) / 6.0))
            textFont(uiFont, action, px + pPad + 8, secY + 14, 0.56)

            // ── Git purge command ─────────────────────────────────────────────
            gitY = secY + 36
            fillCA(theme["accentBar"], 0.90)
            rect(float(px + pPad), float(gitY), 2.0, 12.0)
            fillC(theme["sectionText"])
            textFont(uiFont, "PURGE FROM GIT HISTORY", px + pPad + 8, gitY, 0.52)
            tooltip("Rewrites git history — coordinate with your team before force-pushing")

            gitCmd = buildGitCmd(f, scanPath)
            cmdLines = split(gitCmd, "\n")
            fillCA(theme["wInputBg"], 0.80)
            noStroke()
            roundedRect(float(px + pPad), float(gitY + 16), float(pw - pPad * 2 - 70), float(len(cmdLines) * 13 + 8), 3.0)
            fillCA(theme["scanLine"], 0.85)
            li = 0
            while li < len(cmdLines) {
                lc = cmdLines[li]
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
            envHint = buildEnvHint(f)
            if len(envHint) > 0 {
                envY = gitY + len(cmdLines) * 13 + 32
                if envY + 50 < py + remH - 8 {
                    fillCA(theme["accentBar"], 0.90)
                    rect(float(px + pPad), float(envY), 2.0, 12.0)
                    fillC(theme["sectionText"])
                    textFont(uiFont, "REPLACE WITH ENV VAR", px + pPad + 8, envY, 0.52)
                    envLines = split(envHint, "\n")
                    fillCA(theme["wInputBg"], 0.80)
                    noStroke()
                    roundedRect(float(px + pPad), float(envY + 16), float(pw - pPad * 2 - 70), float(len(envLines) * 13 + 8), 3.0)
                    fillCA(theme["scanLine"], 0.80)
                    el = 0
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

    // ── Suppress context menu (right-click on a finding) ─────────────────────
    if showCtxMenu && ctxFindingIdx >= 0 && ctxFindingIdx < len(findings) {
        f = findings[ctxFindingIdx]
        hasFile = len(f["file"]) > 0 && f["source"] != "git"
        menuItems = makeArray(3, "")
        menuItems[0] = "Suppress pattern everywhere"
        if hasFile {
            menuItems[1] = "Suppress pattern in this file"
            menuItems[2] = "Suppress all findings in this file"
        } else {
            menuItems[1] = ""
            menuItems[2] = ""
        }
        // Trim to non-empty items
        itemCount = 1
        if hasFile { itemCount = 3 }
        ctxItems = makeArray(itemCount, "")
        ctxItems[0] = menuItems[0]
        if hasFile {
            ctxItems[1] = menuItems[1]
            ctxItems[2] = menuItems[2]
        }

        sel = contextMenu(ctxMenuX, ctxMenuY, ctxItems, showCtxMenu, 0.60)
        if sel >= 0 {
            ruleType = "pattern"
            if sel == 1 { ruleType = "file_pattern" }
            if sel == 2 { ruleType = "file" }
            rule = sh.makeIgnoreRule(f, ruleType)
            sh.appendIgnoreRule(scanPath, rule)
            // Add to in-memory rules and re-filter immediately
            newRules = makeArray(len(ignoreRules) + 1, "")
            ri = 0
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

        mw = 600
        mh = 295
        mx = mainX + (mainW - mw) / 2
        my = (wh - mh) / 2

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
        sPad = mx + 18
        sW   = mw - 36
        cy   = my + 36

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
        btnY = my + mh - 46
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
            mx2 = mouseX()
            my2 = mouseY()
            inModal = mx2 >= float(mx) && mx2 <= float(mx + mw) &&
                      my2 >= float(my) && my2 <= float(my + mh)
            if inModal == false { settingsOpen = false }
        }
    }

    uiEnd()
})
