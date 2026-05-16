// secretHunter.lex — CLI front-end for the Secret Hunter scan engine.
//
// Usage:
//   ./klex tests/examples/SecretHunter/secretHunter.lex [path] [flags]
//
//   path             Directory to scan (default: ".")
//   --no-git         Skip git history scan
//   --json           Emit JSON instead of coloured text
//   --max-size=N     Max file size in MB to scan (default: 5)

import "tests/examples/SecretHunter/secretHunterLib.lex" as sh
import "stdlib/json.lex" as js

fn parseArgs() {
    args = _osArgs()
    n = len(args)
    opts = { "path": ".", "doGit": true, "json": false, "maxSizeMB": 5 }
    seenPath = false
    i = 2
    while i < n {
        a = args[i]
        if a == "--no-git" {
            opts["doGit"] = false
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

fn printFinding(f) {
    sev   = f["severity"]
    col   = severityColor(sev)
    reset = color_reset()
    if f["source"] == "file" {
        println(col + "[" + sev + "]" + reset + " " + f["patternName"])
        println("  Source: " + color_cyan() + f["file"] + reset + ":" + color_green() + str(f["line"]) + reset)
        println("  Match:  " + sh.truncateMatch(f["match"]))
        println("  Action: " + f["action"])
        println("")
    } else {
        short = f["commit"]
        if len(short) > 7 { short = substr(short, 0, 7) }
        println(col + "[" + sev + "]" + reset + " " + f["patternName"] + color_dim() + "  (in git history)" + reset)
        println("  Commit: " + color_cyan() + short + reset + "  (" + f["author"] + ", " + f["date"] + ")")
        println("  File:   " + f["file"])
        println("  Match:  " + sh.truncateMatch(f["match"]))
        println("  Action: " + f["action"])
        println("")
    }
}

fn printTextReport(findings, fileCount, commitCount, repoCount) {
    n = len(findings)
    i = 0
    while i < n { printFinding(findings[i])   i = i + 1 }

    crit = 0   high = 0   med = 0   low = 0
    i = 0
    while i < n {
        s = findings[i]["severity"]
        if s == "CRITICAL" { crit = crit + 1 }
        if s == "HIGH"     { high = high + 1 }
        if s == "MEDIUM"   { med  = med  + 1 }
        if s == "LOW"      { low  = low  + 1 }
        i = i + 1
    }
    repoStr = ""
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
    opts = parseArgs()
    root = opts["path"]

    info, sErr = _fsStat(root)
    if sErr != null {
        println("error: cannot stat path: " + root + ": " + sErr)
        _osExit(2)
    }
    if info["isDir"] != true {
        println("error: path is not a directory: " + root)
        _osExit(2)
    }

    if opts["json"] == false { println("Scanning " + root + " ...") }

    result      = sh.runScan(root, opts["doGit"], opts["maxSizeMB"])
    findings    = result["findings"]
    fileCount   = result["fileCount"]
    commitCount = result["commitCount"]
    repoCount   = result["repoCount"]

    if opts["json"] == true {
        println(js.stringify(findings))
    } else {
        println("")
        printTextReport(findings, fileCount, commitCount, repoCount)
    }
}

main()
