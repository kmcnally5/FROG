// playground.lex — kLex Playground
// FROG hosting itself: an in-browser IDE written entirely in kLex.
//
// Run via serve.sh — open http://localhost:8768/

import "stdlib/json.lex" as json
import "stdlib/http.lex" as http
import "stdlib/fs.lex"   as fs

let DOCS_BASE = "https://github.com/kmcnally5/FROG/blob/main/docs/"
let CFG_FILE  = "opfs://klex-playground/.theme"

// ── Built-in examples ─────────────────────────────────────────────────────────

let EX_NAMES = [
    "Hello World",
    "Fibonacci",
    "FizzBuzz",
    "Closures",
    "Higher-Order Functions",
    "Async Workers",
    "Structs",
    "Error Handling",
    "Channels",
    "JSON",
    "HTTP Fetch",
    "Prime Sieve",
    "Sort & Filter",
]

let EX_CODE = [

// Hello World
`println("Hello, World!")
println("Welcome to kLex — FROG Language Runtime")`,

// Fibonacci
`fn fib(n) {
    if n <= 1 { return n }
    return fib(n - 1) + fib(n - 2)
}

let i = 0
while i <= 10 {
    println("fib(" + str(i) + ") = " + str(fib(i)))
    i = i + 1
}`,

// FizzBuzz
`let i = 1
while i <= 20 {
    if i % 15 == 0 {
        println("FizzBuzz")
    } else if i % 3 == 0 {
        println("Fizz")
    } else if i % 5 == 0 {
        println("Buzz")
    } else {
        println(str(i))
    }
    i = i + 1
}`,

// Closures
`fn makeCounter(start) {
    let count = start
    return fn() {
        count = count + 1
        return count
    }
}

let c = makeCounter(0)
println(str(c()))
println(str(c()))
println(str(c()))`,

// Higher-Order Functions
`fn apply(arr, f) {
    let result = makeArray(len(arr), 0)
    let i = 0
    while i < len(arr) {
        result[i] = f(arr[i])
        i = i + 1
    }
    return result
}

let nums = [1, 2, 3, 4, 5]
let squares = apply(nums, fn(x) { return x * x })
println(str(squares))`,

// Async Workers
`let tasks = makeArray(4, null)
let i = 0
while i < 4 {
    let n = i + 1
    tasks[i] = async(fn() { return "worker " + str(n) + " done" })
    i = i + 1
}

i = 0
while i < 4 {
    println(await(tasks[i]))
    i = i + 1
}`,

// Structs
`struct Point { x, y }
struct Rectangle { topLeft, size }

fn area(r) {
    return r.size.x * r.size.y
}

fn describe(r) {
    return "Rect @ (" + str(r.topLeft.x) + "," + str(r.topLeft.y) +
           ") " + str(r.size.x) + "×" + str(r.size.y) +
           " area=" + str(area(r))
}

let r1 = Rectangle {
    topLeft: Point { x: 10, y: 20 },
    size:    Point { x: 100, y: 50 },
}
let r2 = Rectangle {
    topLeft: Point { x: 0, y: 0 },
    size:    Point { x: 1280, y: 820 },
}

println(describe(r1))
println(describe(r2))
println("r2 is " + str(area(r2) / area(r1)) + "× larger")`,

// Error Handling
`fn divide(a, b) {
    if b == 0 {
        return null, error("DIV_ZERO", "cannot divide by zero")
    }
    return float(a) / float(b), null
}

fn safeSqrt(n) {
    if n < 0 {
        return null, error("NEGATIVE", "sqrt of a negative number")
    }
    return sqrt(float(n)), null
}

let result, err = divide(10, 2)
if err != null {
    println("Error: " + err.message)
} else {
    println("10 ÷ 2 = " + str(result))
}

let bad, err2 = divide(7, 0)
if err2 != null { println("Error: " + err2.message) }

let root, err3 = safeSqrt(144)
if err3 != null {
    println("Error: " + err3.message)
} else {
    println("√144 = " + str(root))
}

let _, err4 = safeSqrt(-9)
if err4 != null { println("Error: " + err4.message) }`,

// Channels
`// Simulate parallel file scanning via channels.
let files = ["auth.go", "server.go", "database.go", "utils.go", "main.go"]
let results = channel(len(files))

let i = 0
while i < len(files) {
    let name = files[i]
    async(fn() {
        let lines = len(name) * 47 + 100
        send(results, name + ": " + str(lines) + " lines scanned")
    })
    i = i + 1
}

let j = 0
while j < len(files) {
    let r, _ = recv(results)
    println(r)
    j = j + 1
}
println("All " + str(len(files)) + " files scanned in parallel")`,

// JSON
`import "stdlib/json.lex" as json

let data = {
    "language": "kLex",
    "version":  "0.3.37",
    "features": ["WASM", "channels", "immediate-mode UI", "bridges"],
    "stats":    {"builtins": 522, "stdlib_modules": 60},
}

// stringify returns a plain string (not a tuple)
let encoded = json.stringify(data)
println(encoded)

// parse returns (value, err)
let parsed, parseErr = json.parse(encoded)
if parseErr != null { println("Parse error: " + str(parseErr))  return }
println("\nRound-trip check:")
println("  language: " + parsed["language"])
println("  version:  " + parsed["version"])
println("  builtins: " + str(parsed["stats"]["builtins"]))`,

// HTTP Fetch
`import "stdlib/http.lex" as http
import "stdlib/json.lex"  as json

println("Fetching live data from the internet...")

// jsonplaceholder.typicode.com is a free public REST API for demos.
// Todo #4 has completed: true — shows both branches working.
let resp, err = http.get("https://jsonplaceholder.typicode.com/todos/4")
if err != null { println("Request failed: " + str(err))  return }

println("HTTP " + str(resp.status))

let todo, parseErr = json.parse(resp.body)
if parseErr != null { println("Parse error: " + str(parseErr))  return }

println("Todo #" + str(todo["id"]) + ": " + todo["title"])

let done = todo["completed"]
if type(done) == "BOOLEAN" && done {
    println("Status: ✓ completed")
} else {
    println("Status: pending")
}`,

// Prime Sieve
`fn isPrime(n) {
    if n < 2 { return false }
    let i = 2
    while i * i <= n {
        if n % i == 0 { return false }
        i = i + 1
    }
    return true
}

let count = 0
let i = 2
while i <= 100 { if isPrime(i) { count = count + 1 }  i = i + 1 }

let primes = makeArray(count, 0)
let idx = 0
i = 2
while i <= 100 {
    if isPrime(i) { primes[idx] = i  idx = idx + 1 }
    i = i + 1
}

println(str(count) + " primes up to 100:")
println(str(primes))`,

// Sort & Filter
`struct Person { name, age, city }

let people = [
    Person { name: "Alice",   age: 30, city: "London" },
    Person { name: "Bob",     age: 25, city: "Paris"  },
    Person { name: "Charlie", age: 30, city: "Berlin" },
    Person { name: "Diana",   age: 22, city: "Madrid" },
    Person { name: "Eve",     age: 28, city: "London" },
]

let londoners = filter(people, fn(p) { return p.city == "London" })
println("Londoners:")
let i = 0
while i < len(londoners) {
    println("  " + londoners[i].name + " (age " + str(londoners[i].age) + ")")
    i = i + 1
}

let byAge = sortBy(people, fn(a, b) { return a.age < b.age })
println("\nAll by age:")
i = 0
while i < len(byAge) {
    println("  " + byAge[i].name + "  " + str(byAge[i].age) + "  " + byAge[i].city)
    i = i + 1
}`,

]

// ── UI state ──────────────────────────────────────────────────────────────────

let code            = EX_CODE[0]
let output          = ""
let docQuery        = ""
let docResults      = []
let docNames        = []
let selectedDocName = ""
let selectedDoc     = null
let exPick          = ""
let docs            = []
let statusMsg       = "Loading docs..."
let rightTab        = 0      // 0=Output  1=Docs  2=Config

// Execution tracking
let running      = false
let execTimeMs   = 0
let lastRunOk    = true

// Share / clipboard feedback (counted in frames, not seconds)
let shareMsg        = ""
let shareMsgFrames  = 0


// Config state
let currentTheme = "dark"
let prevTheme    = "dark"

// ── Load documentation index ──────────────────────────────────────────────────

let docsResp, docsErr = http.get("./docs-index.json")
if docsErr == null {
    let parsed, pErr = json.parse(docsResp.body)
    if pErr == null {
        docs = parsed
        statusMsg = "Ready  ·  " + str(len(docs)) + " doc entries"
    } else {
        statusMsg = "Docs parse error"
    }
} else {
    statusMsg = "Docs unavailable"
}

// ── Load font ─────────────────────────────────────────────────────────────────

let monoFont = null
let monoFontRaw = loadFont("./JetBrainsMono-Regular.woff2", 28)
if type(monoFontRaw) != "ERROR" {
    monoFont = monoFontRaw
}

// ── Load persisted theme ──────────────────────────────────────────────────────

let savedTheme, themeReadErr = fs.read(CFG_FILE)
if themeReadErr == null && len(savedTheme) > 0 {
    currentTheme = savedTheme
    prevTheme    = savedTheme
}

// ── URL hash — restore shared code on startup ─────────────────────────────────

let initialHash = _wasmGetHash()
if len(initialHash) > 8 {
    let decoded, decErr = safe(_wasmBase64Decode, initialHash)
    if decErr == null && type(decoded) == "STRING" && len(decoded) > 0 {
        code   = decoded
        exPick = "— shared code —"
        statusMsg = "Loaded from shared link"
    }
}

// ── Docs helpers ──────────────────────────────────────────────────────────────

fn buildNames(results) {
    let n = len(results)
    if n == 0 { return [] }
    let names = makeArray(n, "")
    let i = 0
    while i < n {
        let e = results[i]
        let cat = e["category"]
        if cat != "" {
            names[i] = "[" + cat + "]  " + e["name"]
        } else {
            names[i] = e["name"]
        }
        i = i + 1
    }
    return names
}

fn searchDocs(q) {
    if len(q) < 2 { return [] }
    let total = len(docs)
    if total == 0 { return [] }
    let lq = lower(q)

    // Three relevance tiers so the most on-point matches lead, instead of
    // being buried under commands that merely mention the query in their
    // description (searching "print" otherwise surfaces anything whose
    // summary says "println" / "printf" / "Prints").
    let prefixHits  = makeArray(30, null)   // name starts with the query
    let nameHits    = makeArray(30, null)   // name contains the query (not prefix)
    let summaryHits = makeArray(30, null)   // only the summary contains it
    let pc = 0
    let nc = 0
    let sc = 0

    let i = 0
    while i < total {
        let e = docs[i]
        let name = e["name"]
        if type(name) != "STRING" { i = i + 1  continue }
        let summary = e["summary"]
        if type(summary) != "STRING" { summary = "" }
        let pos = indexOf(lower(name), lq)
        if pos == 0 {
            if pc < 30 { prefixHits[pc] = e  pc = pc + 1 }
        } else if pos > 0 {
            if nc < 30 { nameHits[nc] = e  nc = nc + 1 }
        } else if indexOf(lower(summary), lq) >= 0 {
            if sc < 30 { summaryHits[sc] = e  sc = sc + 1 }
        }
        i = i + 1
    }

    // Concatenate the tiers in priority order, capped at 30 results.
    let out = makeArray(30, null)
    let oc = 0
    let k = 0
    while k < pc && oc < 30 { out[oc] = prefixHits[k]   oc = oc + 1  k = k + 1 }
    k = 0
    while k < nc && oc < 30 { out[oc] = nameHits[k]     oc = oc + 1  k = k + 1 }
    k = 0
    while k < sc && oc < 30 { out[oc] = summaryHits[k]  oc = oc + 1  k = k + 1 }
    return slice(out, 0, oc)
}

// ── Config helpers ────────────────────────────────────────────────────────────

fn bgForTheme(name) {
    if name == "nebula"       { return [0.08, 0.09, 0.13] }
    if name == "light"        { return [0.93, 0.94, 0.96] }
    if name == "dark"         { return [0.13, 0.14, 0.18] }
    if name == "highContrast" { return [0.00, 0.00, 0.00] }
    return [0.08, 0.09, 0.13]
}

// ── Layout constants ──────────────────────────────────────────────────────────

let EDGE     = 8
let HDR_H    = 48
let LEFT_W   = 624
let GAP      = 8
let RX       = EDGE + LEFT_W + GAP   // 640
let RW       = 1280 - RX - EDGE      // 632
let CY       = HDR_H + 4             //  52
let STATUS_H = 28
let CH       = 820 - CY - STATUS_H - EDGE  // 732
let STATUS_Y = CY + CH               // 784

// Right pane
let TAB_H    = 36
let CONT_Y   = CY + TAB_H            //  88
let CONT_H   = CH - TAB_H            // 696

// Docs sub-layout
let D_LBL_Y  = CONT_Y
let D_SRCH_Y = D_LBL_Y + 22
let D_LIST_Y = D_SRCH_Y + 36
let D_LIST_H = 340
let D_DET_Y  = D_LIST_Y + D_LIST_H + GAP

// Config sub-layout (Tab 2)
let CFG_LBL_Y   = CONT_Y
let CFG_THEME_Y = CFG_LBL_Y + 28

let TAB_LABELS = ["Output", "Docs", "Config"]

// drawWrappedLine draws `text` word-wrapped to maxChars per visual row, from
// (x, y), advancing by lh per row. Output is monospace so char count == pixel
// width. A single token longer than the line is hard-broken so nothing
// overflows. Returns the y after the last row so the caller continues below.
fn drawWrappedLine(text, x, y, lh, maxChars) {
    if text == "" { return y + lh }
    let words = split(text, " ")
    let cur = ""
    let ty  = y
    let wi  = 0
    while wi < len(words) {
        let w = words[wi]
        // Hard-break a single word that is wider than the whole line.
        while len(w) > maxChars {
            if cur != "" {
                label(cur, x, ty, 0.42)
                ty  = ty + lh
                cur = ""
            }
            label(substr(w, 0, maxChars), x, ty, 0.42)
            ty = ty + lh
            w  = substr(w, maxChars, len(w))
        }
        let candidate = w
        if cur != "" { candidate = cur + " " + w }
        if len(candidate) > maxChars && cur != "" {
            label(cur, x, ty, 0.42)
            ty  = ty + lh
            cur = w
        } else {
            cur = candidate
        }
        wi = wi + 1
    }
    if cur != "" {
        label(cur, x, ty, 0.42)
        ty = ty + lh
    }
    return ty
}

window(1280, 820, "kLex Playground", fn(frame) {
    setTheme(currentTheme)
    let bg = bgForTheme(currentTheme)
    background(bg[0], bg[1], bg[2])
    uiBegin()

    // ── Cmd+Enter ─────────────────────────────────────────────────────────────
    let runNow = _wasmCheckRunFlag()

    // ── Header ────────────────────────────────────────────────────────────────
    label("kLex Playground", 16, 15, 0.65)
    label(statusMsg, 226, 19, 0.40)

    if button("Run", 754, 10, 90, 32) || runNow {
        let t0 = _timeNanos()
        statusMsg = "Running..."
        let res = runScript(code)
        execTimeMs = int((_timeNanos() - t0) / 1000000)
        if res["isError"] {
            output    = "── Error ──\n" + res["error"]
            lastRunOk = false
            statusMsg = "Error  ·  " + str(execTimeMs) + "ms"
        } else {
            output    = res["output"]
            if output == "" { output = "(no output)" }
            lastRunOk = true
            statusMsg = "OK  ·  " + str(execTimeMs) + "ms"
        }
        rightTab = 0
    }

    if button("Clear", 852, 10, 70, 32) {
        output    = ""
        statusMsg = "Ready"
    }

    if button("Share", 930, 10, 80, 32) {
        let encoded = _wasmBase64Encode(code)
        _wasmSetHash(encoded)
        _wasmCopyToClipboard(_wasmGetHref())
        shareMsg       = "Link copied!"
        shareMsgFrames = 0
    }

    // Copy/share feedback lifecycle — disappears after ~180 frames (~3s at
    // 60fps). The message itself is drawn in the footer status bar (below),
    // clear of the toolbar and the example dropdown.
    if len(shareMsg) > 0 {
        shareMsgFrames = shareMsgFrames + 1
        if shareMsgFrames > 180 {
            shareMsg       = ""
            shareMsgFrames = 0
        }
    }

    // ── Left pane: code editor ────────────────────────────────────────────────
    code = textArea("", code, EDGE, CY, LEFT_W, CH, "klex")

    // ── Right pane: tabs ──────────────────────────────────────────────────────
    rightTab = tabs(RX, CY, RW, TAB_LABELS, rightTab)

    // ── Examples dropdown — registered HERE for stable widget ID ──────────────
    // The dropdown is registered before any conditional tab content so its
    // widget ID never shifts (which would lose its state and spuriously fire
    // the handler, clearing output). The open menu still renders on top because
    // kLex defers popup drawing to uiEnd() regardless of registration order.
    let newPick = dropdown("", EX_NAMES, 1022, 10, 250)
    if newPick != "" && newPick != exPick {
        exPick    = newPick
        let i     = 0
        while i < len(EX_NAMES) {
            if EX_NAMES[i] == exPick {
                code      = EX_CODE[i]
                statusMsg = "Loaded: " + exPick
            }
            i = i + 1
        }
    }

    // ── Tab 0: Output ─────────────────────────────────────────────────────────
    if rightTab == 0 {
        // Copy button
        if button("Copy", RX + RW - 72, CONT_Y + 4, 62, 26, 0.42) {
            _wasmCopyToClipboard(output)
            shareMsg       = "Copied!"
            shareMsgFrames = 0
        }

        if monoFont != null { uiSetFont(monoFont) }
        if output == "" {
            // Empty state — centred hint text
            label("Press Run or Cmd+Enter to execute your code",
                  RX + 60, CONT_Y + CONT_H / 2, 0.42)
        } else {
            // Plain text drawn straight onto the background — no textbox
            // container. Long lines are word-wrapped to the pane width so
            // output never runs off the right edge. The output font is
            // monospace, so character count equals pixel width — we wrap by
            // char count (no per-character measuring).
            let lh = lineHeight(0.42)
            // Measure the char width with the ACTUAL font label() renders in.
            // The string-only textWidth() uses a fixed 8px base font and badly
            // overestimates how many chars fit (→ effectively no wrapping); the
            // font-bound form measures monoFont at its real size, matching the
            // text we draw.
            let charW = textWidth("0", 0.42)
            if monoFont != null { charW = textWidth(monoFont, "0", 0.42) }
            let maxChars = int(float(RW - 14) / charW)
            if maxChars < 1 { maxChars = 1 }
            let yMax = CONT_Y + CONT_H
            // Start below the Copy button (top-right) so the first lines of
            // output don't run underneath it.
            let ty   = CONT_Y + 36
            pushClip(RX, CONT_Y, RW, CONT_H)
            let lines = split(output, "\n")
            let li = 0
            while li < len(lines) && ty < yMax {
                ty = drawWrappedLine(lines[li], RX, ty, lh, maxChars)
                li = li + 1
            }
            popClip()
        }
        if monoFont != null { uiResetFont() }
    }

    // ── Tab 1: Docs ───────────────────────────────────────────────────────────
    if rightTab == 1 {
        label("Documentation", RX, D_LBL_Y, 0.45)

        let newQuery = textInput("", docQuery, RX, D_SRCH_Y, RW, 30)
        if newQuery != docQuery {
            docQuery        = newQuery
            docResults      = searchDocs(docQuery)
            docNames        = buildNames(docResults)
            selectedDocName = ""
            selectedDoc     = null
        }

        let picked = list("", docNames, RX, D_LIST_Y, RW, D_LIST_H)
        if picked != "" && picked != selectedDocName {
            selectedDocName = picked
            selectedDoc     = null
            let j = 0
            while j < len(docNames) {
                if docNames[j] == picked { selectedDoc = docResults[j] }
                j = j + 1
            }
        }

        if selectedDoc != null {
            label(selectedDoc["sig"],     RX, D_DET_Y,      0.47)
            label(selectedDoc["summary"], RX, D_DET_Y + 22, 0.43)
            label("Category: " + selectedDoc["category"], RX, D_DET_Y + 42, 0.38)
            if button("Open in GitHub", RX, D_DET_Y + 62, 160, 28) {
                openURL(DOCS_BASE + selectedDoc["doc"] + "#" + selectedDoc["anchor"])
            }
        } else if docQuery != "" && len(docResults) == 0 {
            label("No results for \"" + docQuery + "\"", RX, D_DET_Y + 10, 0.43)
        } else {
            label("Type a name or keyword to search docs", RX, D_DET_Y + 10, 0.40)
        }
    }

    // ── Tab 2: Config ─────────────────────────────────────────────────────────
    if rightTab == 2 {
        label("Theme", RX, CFG_LBL_Y, 0.45)

        currentTheme = radio("Nebula",        RX, CFG_THEME_Y,       "nebula",       currentTheme)
        currentTheme = radio("Light",         RX, CFG_THEME_Y + 34,  "light",        currentTheme)
        currentTheme = radio("Dark",          RX, CFG_THEME_Y + 68,  "dark",         currentTheme)
        currentTheme = radio("High Contrast", RX, CFG_THEME_Y + 102, "highContrast", currentTheme)

        if currentTheme != prevTheme {
            prevTheme = currentTheme
            let wOk, wErr = fs.write(CFG_FILE, currentTheme)
        }
    }

    // ── Status bar ────────────────────────────────────────────────────────────
    let lines      = split(code, "\n")
    let lineCount  = len(lines)
    let charCount  = len(code)
    let statusL    = "Lines: " + str(lineCount) + "   Chars: " + str(charCount)
    if len(shareMsg) > 0 {
        statusL = shareMsg + "      " + statusL
    }
    let statusR    = ""
    if execTimeMs > 0 {
        statusR = "Last run: " + str(execTimeMs) + "ms"
    }

    // Status bar background
    let sbBg = [0.06, 0.06, 0.09, 1.0]
    if currentTheme == "light"        { sbBg = [0.85, 0.86, 0.88, 1.0] }
    if currentTheme == "dark"         { sbBg = [0.08, 0.08, 0.12, 1.0] }
    if currentTheme == "highContrast" { sbBg = [0.00, 0.00, 0.00, 1.0] }
    fill(sbBg[0], sbBg[1], sbBg[2], 1.0)
    rect(0, STATUS_Y, 1280, STATUS_H)

    label(statusL, EDGE + 4, STATUS_Y + 7, 0.38)
    if len(statusR) > 0 {
        label(statusR, 1130, STATUS_Y + 7, 0.38)
    }

    // Centred tagline — the gold nugget: this whole IDE is FROG itself,
    // compiled to WebAssembly and running in the browser.
    let tagline = "This entire IDE is a FROG program — compiled to WebAssembly, running in your browser"
    let tagW    = textWidth(tagline, 0.38)
    label(tagline, int((1280.0 - tagW) / 2.0), STATUS_Y + 7, 0.38)

    uiEnd()
})
