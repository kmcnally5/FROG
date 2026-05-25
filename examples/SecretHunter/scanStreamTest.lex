// scanStreamTest.lex — verifies the streaming bridge paths for both
// YARA and entropy phases via secretHunterLib.
//
// Pass 1 (CLI shape): runScan with both YARA + entropy enabled and
//   progressCh=null still completes — proves the streaming replacement is
//   non-regressive for the CLI path.
//
// Pass 2 (UI shape, entropy): scanEntropyFilesParallel with a live progressCh
//   emits one "entropy_progress" event per file and one "entropy_finding"
//   event per high-entropy hit.
//
// Pass 3 (UI shape, YARA): scanYaraFilesParallel with a live progressCh emits
//   one "yara_progress" event per file and one "yara_finding" event per
//   matched YARA rule.

import "examples/SecretHunter/secretHunterLib.lex" as sh

let RULES = "examples/SecretHunter/secrets.yar"

println("=== Pass 1: CLI path (YARA + entropy, progressCh = null) ===")
let res = sh.runScan("examples/SecretHunter", true, 5, RULES, true)
let allFindings = res["findings"]

let yaraCount    = 0
let entropyCount = 0
for f in allFindings {
    if f["source"] == "yara"    { yaraCount    = yaraCount    + 1 }
    if f["source"] == "entropy" { entropyCount = entropyCount + 1 }
}
println("total findings:   " + str(len(allFindings)))
println("yara findings:    " + str(yaraCount))
println("entropy findings: " + str(entropyCount))

// Common file set for the streaming passes.
let files = makeArray(8, "")
files[0] = "examples/SecretHunter/secretHunterLib.lex"
files[1] = "examples/SecretHunter/secretHunterUI.lex"
files[2] = "examples/SecretHunter/yara_bridge.py"
files[3] = "examples/SecretHunter/github_bridge.py"
files[4] = "examples/SecretHunter/secrets.yar"
files[5] = "examples/SecretHunter/README.md"
files[6] = "examples/SecretHunter/secretHunter.lex"
files[7] = "examples/SecretHunter/yaraTest.lex"

// ── Pass 2: entropy streaming ────────────────────────────────────────────────
println("")
println("=== Pass 2: entropy streaming path (live progressCh) ===")

let pch = channel(200)
let _files1 = files
let _pch1   = pch
let task1 = async(fn() { return sh.scanEntropyFilesParallel(_files1, _pch1) })
let entropyFindings = await(task1)

let ePrg = 0
let eFnd = 0
let draining = true
while draining {
    let msg = recvNonBlock(pch)
    if msg == null {
        draining = false
    } else {
        let ph = msg["phase"]
        if ph == "entropy_progress" { ePrg = ePrg + msg["done"] }
        else if ph == "entropy_finding" { eFnd = eFnd + 1 }
    }
}
println("progress events:  " + str(ePrg) + " (expected " + str(len(files)) + ")")
println("finding events:   " + str(eFnd))
println("findings array:   " + str(len(entropyFindings)))

// ── Pass 3: YARA streaming ───────────────────────────────────────────────────
println("")
println("=== Pass 3: YARA streaming path (live progressCh) ===")

let pch2 = channel(200)
let _rules  = RULES
let _files2 = files
let _pch2   = pch2
let task2 = async(fn() { return sh.scanYaraFilesParallel(_rules, _files2, _pch2) })
let yaraFindings = await(task2)

let yPrg = 0
let yFnd = 0
draining = true
while draining {
    let msg = recvNonBlock(pch2)
    if msg == null {
        draining = false
    } else {
        let ph = msg["phase"]
        if ph == "yara_progress" { yPrg = yPrg + msg["done"] }
        else if ph == "yara_finding" { yFnd = yFnd + 1 }
    }
}
println("progress events:  " + str(yPrg) + " (expected " + str(len(files)) + ")")
println("finding events:   " + str(yFnd))
println("findings array:   " + str(len(yaraFindings)))

// ── Assertions ───────────────────────────────────────────────────────────────
println("")
let ok = true
if ePrg != len(files)         { println("FAIL: entropy progress event count mismatch")   ok = false }
if eFnd != len(entropyFindings) { println("FAIL: entropy finding-event count != findings") ok = false }
if yPrg != len(files)         { println("FAIL: yara progress event count mismatch")      ok = false }
if yFnd != len(yaraFindings)  { println("FAIL: yara finding-event count != findings")    ok = false }
if ok { println("OK") }
