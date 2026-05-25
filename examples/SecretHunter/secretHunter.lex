// secretHunter.lex — CLI front-end for the Secret Hunter scan engine.
//
// Usage:
//   ./klex examples/SecretHunter/secretHunter.lex [path] [flags]
//
//   path             Directory to scan (default: ".")
//   --no-git         Skip git history scan
//   --json           Emit JSON instead of coloured text
//   --max-size=N     Max file size in MB to scan (default: 5)

import "examples/SecretHunter/secretHunterLib.lex" as sh
import "stdlib/json.lex" as js

fn parseArgs() {
    let args = _osArgs()
    let n = len(args)
    let opts = { "path": ".", "doGit": true, "json": false, "maxSizeMB": 5, "doJWT": true }
    let seenPath = false
    let i = 2
    while i < n {
        let a = args[i]
        if a == "--no-git" {
            opts["doGit"] = false
        } else if a == "--no-jwt" {
            opts["doJWT"] = false
        } else if a == "--json" {
            opts["json"] = true
        } else if startsWith(a, "--max-size=") {
            opts["maxSizeMB"] = int(substr(a, 11))
        } else if startsWith(a, "--") {
            println("Unknown flag: " + a)
            _osExit(2)
        } else {
            if seenPath == false {
                opts["path"] = a
                seenPath = true
            } else {
                println("Unexpected extra argument: " + a)
                _osExit(2)
            }
        }
        i = i + 1
    }
    return opts
}

fn severityColor(sev) {
    if sev == "CRITICAL" { return color_red() + color_bold() }
    if sev == "HIGH"     { return color_red() }
    if sev == "MEDIUM"   { return color_yellow() }
    return color_cyan()
}

// printJWT — render decoded JWT claims for a finding the Node bridge enriched.
// Compact: one line for alg + status, one line for sub/iss, one line for
// scopes (if any). Keeps the report scannable in a terminal.
fn printJWT(jwt) {
    if jwt == null { return }
    let decodeErr = jwt["error"]
    if decodeErr != null {
        println("  JWT:    " + color_dim() + "decode error: " + decodeErr + color_reset())
        return
    }
    let algLine = "  JWT:    alg=" + jwt["alg"]
    if jwt["expired"]      { algLine = algLine + color_dim()   + "  [EXPIRED]"      + color_reset() }
    else if jwt["missing_exp"]   { algLine = algLine + color_yellow() + "  [NO EXPIRY]"    + color_reset() }
    else if jwt["expires_soon"]  { algLine = algLine + color_yellow() + "  [EXPIRES SOON]" + color_reset() }
    else                         { algLine = algLine + color_green()  + "  [VALID]"        + color_reset() }
    let algW = jwt["alg_warning"]
    if algW != null {
        if jwt["alg"] == "none" { algLine = algLine + "  " + color_red() + color_bold() + "⚠ " + algW + color_reset() }
        else                    { algLine = algLine + "  " + color_yellow() + "⚠ " + algW + color_reset() }
    }
    println(algLine)
    let claimsLine = ""
    if len(jwt["sub"]) > 0 { claimsLine = "sub=" + jwt["sub"] }
    if len(jwt["iss"]) > 0 {
        if len(claimsLine) > 0 { claimsLine = claimsLine + "  " }
        claimsLine = claimsLine + "iss=" + jwt["iss"]
    }
    if len(jwt["aud"]) > 0 {
        if len(claimsLine) > 0 { claimsLine = claimsLine + "  " }
        claimsLine = claimsLine + "aud=" + jwt["aud"]
    }
    if len(claimsLine) > 0 { println("          " + color_cyan() + claimsLine + color_reset()) }
    let scopes = jwt["scopes"]
    if len(scopes) > 0 {
        let scopeStr = scopes[0]
        let si = 1
        while si < len(scopes) { scopeStr = scopeStr + " " + scopes[si]   si = si + 1 }
        println("          " + color_dim() + "scopes: " + scopeStr + color_reset())
    }
}

fn printFinding(f) {
    let sev   = f["severity"]
    let col   = severityColor(sev)
    let reset = color_reset()
    if f["source"] == "file" {
        println(col + "[" + sev + "]" + reset + " " + f["patternName"])
        println("  Source: " + color_cyan() + f["file"] + reset + ":" + color_green() + str(f["line"]) + reset)
        println("  Match:  " + sh.truncateMatch(f["match"]))
        printJWT(f["jwt"])
        println("  Action: " + f["action"])
        println("")
    } else {
        let short = f["commit"]
        if len(short) > 7 { short = substr(short, 0, 7) }
        println(col + "[" + sev + "]" + reset + " " + f["patternName"] + color_dim() + "  (in git history)" + reset)
        println("  Commit: " + color_cyan() + short + reset + "  (" + f["author"] + ", " + f["date"] + ")")
        println("  File:   " + f["file"])
        println("  Match:  " + sh.truncateMatch(f["match"]))
        printJWT(f["jwt"])
        println("  Action: " + f["action"])
        println("")
    }
}

fn printTextReport(findings, fileCount, commitCount, repoCount) {
    let n = len(findings)
    let i = 0
    while i < n { printFinding(findings[i])   i = i + 1 }

    let crit = 0   let high = 0   let med = 0   let low = 0
    i = 0
    while i < n {
        let s = findings[i]["severity"]
        if s == "CRITICAL" { crit = crit + 1 }
        if s == "HIGH"     { high = high + 1 }
        if s == "MEDIUM"   { med  = med  + 1 }
        if s == "LOW"      { low  = low  + 1 }
        i = i + 1
    }
    let repoStr = ""
    if repoCount > 1 { repoStr = " across " + str(repoCount) + " git repositories" }
    println("─────────────────────────────────────────────")
    println("Scanned: " + str(fileCount) + " files, " + str(commitCount) + " commits" + repoStr)
    println("Findings: " +
        color_red()    + color_bold() + str(crit) + " CRITICAL" + color_reset() + "  |  " +
        color_red()    + str(high) + " HIGH"     + color_reset() + "  |  " +
        color_yellow() + str(med)  + " MEDIUM"   + color_reset() + "  |  " +
        color_cyan()   + str(low)  + " LOW"      + color_reset())
    println("─────────────────────────────────────────────")
}

fn main() {
    let opts = parseArgs()
    let root = opts["path"]

    let info, sErr = _fsStat(root)
    if sErr != null {
        println("error: cannot stat path: " + root + ": " + sErr)
        _osExit(2)
    }
    if info["isDir"] != true {
        println("error: path is not a directory: " + root)
        _osExit(2)
    }

    if opts["json"] == false { println("Scanning " + root + " ...") }

    let result      = sh.runScan(root, opts["doGit"], opts["maxSizeMB"], null, false, opts["doJWT"])
    let findings    = result["findings"]
    let fileCount   = result["fileCount"]
    let commitCount = result["commitCount"]
    let repoCount   = result["repoCount"]

    if opts["json"] == true {
        println(js.stringify(findings))
    } else {
        println("")
        printTextReport(findings, fileCount, commitCount, repoCount)
    }
}

main()
