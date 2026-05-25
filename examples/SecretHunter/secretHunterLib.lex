// secretHunterLib.lex — scan engine (import this; do not run directly)
//
// Provides all pattern matching, file enumeration, parallel scanning,
// and result sorting. Entry point: runScan(root, doGit, maxSizeMB).

import "stdlib/path.lex" as p

// ─────────────────────────────────────────────────────────────────
// PATTERN LIBRARY
// ─────────────────────────────────────────────────────────────────

let PATTERNS = [
    { "name": "AWS Access Key ID",
      "needle": "AKIA",
      "regex": `AKIA[0-9A-Z]{16}`,
      "severity": "CRITICAL",
      "action": "Rotate this key immediately in the AWS console." },

    { "name": "AWS Session Token",
      "needle": "aws_session_token",
      "regex": `(?i)aws_session_token["'\s:=]+["']?[A-Za-z0-9/+=]{100,}`,
      "severity": "CRITICAL",
      "action": "Rotate the originating credentials and revoke any active sessions." },

    { "name": "Google API Key",
      "needle": "AIza",
      "regex": `AIza[0-9A-Za-z\-_]{35}`,
      "severity": "CRITICAL",
      "action": "Rotate this key in the Google Cloud Console." },

    { "name": "Google OAuth Client ID",
      "needle": ".apps.googleusercontent.com",
      "regex": `[0-9]+-[0-9A-Za-z_]{32}\.apps\.googleusercontent\.com`,
      "severity": "CRITICAL",
      "action": "Verify exposure; rotate client secret if paired secret is also exposed." },

    { "name": "GitHub Classic PAT",
      "needle": "ghp_",
      "regex": `ghp_[A-Za-z0-9]{36}`,
      "severity": "CRITICAL",
      "action": "Revoke immediately on GitHub. Purge from history with git filter-repo." },

    { "name": "GitHub Fine-grained PAT",
      "needle": "github_pat_",
      "regex": `github_pat_[A-Za-z0-9_]{82}`,
      "severity": "CRITICAL",
      "action": "Revoke immediately on GitHub. Purge from history with git filter-repo." },

    { "name": "GitHub OAuth Token",
      "needle": "gho_",
      "regex": `gho_[A-Za-z0-9]{36}`,
      "severity": "CRITICAL",
      "action": "Revoke immediately on GitHub. Re-authorise the OAuth app." },

    { "name": "GitHub App Token",
      "needle": "_",
      "regex": `(ghu|ghs)_[A-Za-z0-9]{36}`,
      "severity": "CRITICAL",
      "action": "Regenerate the GitHub App installation token." },

    { "name": "GitLab Personal Access Token",
      "needle": "glpat-",
      "regex": `glpat-[A-Za-z0-9\-_]{20}`,
      "severity": "CRITICAL",
      "action": "Revoke immediately in GitLab Profile → Access Tokens." },

    { "name": "Private Key Block",
      "needle": "BEGIN",
      "regex": `-----BEGIN ((RSA|OPENSSH|EC|DSA|PGP|ENCRYPTED) )?PRIVATE KEY( BLOCK)?-----`,
      "severity": "CRITICAL",
      "action": "Rotate the key pair. Remove the private key from the repo and history." },

    { "name": "Stripe Live Secret Key",
      "needle": "sk_live_",
      "regex": `sk_live_[0-9a-zA-Z]{24,}`,
      "severity": "CRITICAL",
      "action": "Roll the key in the Stripe Dashboard immediately." },

    { "name": "Stripe Restricted Key",
      "needle": "rk_live_",
      "regex": `rk_live_[0-9a-zA-Z]{24,}`,
      "severity": "CRITICAL",
      "action": "Roll the restricted key in the Stripe Dashboard." },

    { "name": "Square Access Token",
      "needle": "sq0",
      "regex": `sq0[a-z]{3}-[0-9A-Za-z\-_]{22,43}`,
      "severity": "CRITICAL",
      "action": "Revoke the token in the Square Developer Dashboard." },

    { "name": "Slack Token",
      "needle": "xox",
      "regex": `xox[baprs]-[0-9a-zA-Z\-]{10,}`,
      "severity": "HIGH",
      "action": "Revoke the token in your Slack app settings." },

    { "name": "Slack Webhook URL",
      "needle": "hooks.slack.com/services/",
      "regex": `https://hooks\.slack\.com/services/[A-Z0-9/]+`,
      "severity": "HIGH",
      "action": "Rotate the webhook in the originating Slack app integration." },

    { "name": "Discord Webhook URL",
      "needle": "/api/webhooks/",
      "regex": `https://discord(app)?\.com/api/webhooks/[0-9]+/[A-Za-z0-9_\-]+`,
      "severity": "HIGH",
      "action": "Delete the webhook in Discord server integration settings." },

    { "name": "SendGrid API Key",
      "needle": "SG.",
      "regex": `SG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}`,
      "severity": "HIGH",
      "action": "Revoke the key in SendGrid → Settings → API Keys." },

    { "name": "Mailgun API Key",
      "needle": "key-",
      "regex": `key-[0-9a-zA-Z]{32}`,
      "severity": "HIGH",
      "action": "Regenerate the key in the Mailgun control panel." },

    { "name": "Twilio Account SID",
      "needle": "AC",
      "regex": `AC[a-f0-9]{32}`,
      "severity": "HIGH",
      "action": "Verify exposure; rotate the paired auth token if compromised." },

    { "name": "Database URL with credentials",
      "needle": "://",
      "regex": `(postgres(ql)?|mysql|mongodb(\+srv)?|redis)://[^\s/]*:[^\s/@]+@[^\s]+`,
      "severity": "HIGH",
      "action": "Rotate the database user's password and move credentials to a secret store." },

    { "name": "Connection String Password (ADO.NET / SQL Server)",
      "needle": "Password=",
      "regex": `(?i)Password=[^;'"\s]{8,}`,
      "severity": "HIGH",
      "action": "Rotate the database password and store it in an environment variable or secret manager." },

    { "name": "JWT / Signing Secret",
      "needle": "Secret",
      "regex": `(?i)"[a-zA-Z_]*[Ss]ecret[a-zA-Z_]*"\s*:\s*"([A-Za-z0-9+/=_\-\.]{20,})"`,
      "severity": "HIGH",
      "action": "Rotate the signing secret immediately and invalidate all tokens issued with it." },

    { "name": "JSON Web Token (JWT)",
      "needle": "eyJ",
      "regex": `eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`,
      "severity": "MEDIUM",
      "action": "Review claims; rotate the signing key if the token grants real access." },

    { "name": "Generic API Key (heuristic)",
      "needle": "",
      "regex": `(?i)(api[_\-]?key|apikey|api[_\-]?secret|access[_\-]?token)["\s:=]+["']?([A-Za-z0-9_\-]{20,})["']?`,
      "severity": "LOW",
      "action": "Heuristic match — verify if this is a real secret, rotate if so." },

    { "name": "Hard-coded Password (heuristic)",
      "needle": "",
      "regex": `(?i)(password|passwd|pwd)["\s:=]+["']([^"']{8,})["']`,
      "severity": "LOW",
      "action": "Heuristic match — move to env var or secret store if this is a real password." },
]

// ─────────────────────────────────────────────────────────────────
// SECRETIGNORE — baseline / allowlist management
//
// Reads .secretignore from the scan root. Each non-comment line is
// a suppression rule in one of three forms:
//
//   path/glob/**           suppress ALL findings in matching files
//   PatternName            suppress this pattern everywhere
//   path/glob:PatternName  suppress this pattern only in matching files
//
// Glob support: **/suffix, prefix/**, *.ext, exact path, or substring.
// Pattern names are matched case-insensitively by prefix.
//
// Usage from the UI (after scan):
//   rules = sh.loadIgnoreFile(scanPath)
//   filtered, suppressed = sh.filterFindings(allFindings, rules)
// ─────────────────────────────────────────────────────────────────

fn loadIgnoreFile(root) {
    let igPath = root + "/.secretignore"
    if _fsExists(igPath) == false { return makeArray(0) }
    let content, err = _fsRead(igPath)
    if err != null { return makeArray(0) }
    let lines = split(content, "\n")
    let n = len(lines)
    let count = 0
    let i = 0
    while i < n {
        let line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" { count = count + 1 }
        i = i + 1
    }
    let rules = makeArray(count, "")
    let idx = 0
    i = 0
    while i < n {
        let line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" {
            rules[idx] = line
            idx = idx + 1
        }
        i = i + 1
    }
    return rules
}

fn appendIgnoreRule(root, rule) {
    let igPath = root + "/.secretignore"
    if _fsExists(igPath) == false {
        let header = "# .secretignore — SecretHunter allowlist\n"
        header = header + "# Lines: path, PatternName, or path:PatternName\n\n"
        let _, err = _fsWrite(igPath, header + rule + "\n")
        return err
    }
    let _, err = _fsAppend(igPath, rule + "\n")
    return err
}

fn _pathMatches(pattern, filePath) {
    if pattern == filePath { return true }
    if startsWith(pattern, "**/") {
        let suffix = substr(pattern, 3)
        if endsWith(filePath, "/" + suffix) { return true }
        if indexOf(filePath, "/" + suffix + "/") >= 0 { return true }
        return filePath == suffix
    }
    if endsWith(pattern, "/**") {
        let prefix = substr(pattern, 0, len(pattern) - 3)
        return startsWith(filePath, prefix + "/") || filePath == prefix
    }
    if startsWith(pattern, "*.") {
        return endsWith(filePath, substr(pattern, 1))
    }
    return indexOf(filePath, pattern) >= 0
}

fn _patternMatches(rule, patternName) {
    return startsWith(lower(patternName), lower(rule))
}

fn ruleMatchesFinding(rule, filePath, patternName) {
    let colonIdx = indexOf(rule, ":")
    if colonIdx >= 0 {
        let filePart = trim(substr(rule, 0, colonIdx))
        let patPart  = trim(substr(rule, colonIdx + 1))
        return _pathMatches(filePart, filePath) && _patternMatches(patPart, patternName)
    }
    if indexOf(rule, "/") < 0 && indexOf(rule, "*") < 0 {
        return _patternMatches(rule, patternName)
    }
    return _pathMatches(rule, filePath)
}

fn filterFindings(findings, rules) {
    let n      = len(findings)
    let nRules = len(rules)
    if nRules == 0 { return findings, 0 }

    let kept = 0
    let i = 0
    while i < n {
        let f = findings[i]
        let suppressed = false
        let r = 0
        while r < nRules && suppressed == false {
            if ruleMatchesFinding(rules[r], f["file"], f["patternName"]) {
                suppressed = true
            }
            r = r + 1
        }
        if suppressed == false { kept = kept + 1 }
        i = i + 1
    }

    let suppressedCount = n - kept
    if suppressedCount == 0 { return findings, 0 }

    let out = makeArray(kept)
    let idx = 0
    i = 0
    while i < n {
        let f = findings[i]
        let suppressed = false
        let r = 0
        while r < nRules && suppressed == false {
            if ruleMatchesFinding(rules[r], f["file"], f["patternName"]) {
                suppressed = true
            }
            r = r + 1
        }
        if suppressed == false {
            out[idx] = f
            idx = idx + 1
        }
        i = i + 1
    }
    return out, suppressedCount
}

// makeIgnoreRule builds the correct rule string for a finding.
// ruleType: "pattern" | "file" | "file_pattern"
fn makeIgnoreRule(finding, ruleType) {
    let pat = finding["patternName"]
    let parenIdx = indexOf(pat, " (")
    if parenIdx >= 0 { pat = substr(pat, 0, parenIdx) }
    pat = trim(pat)
    if ruleType == "pattern"      { return pat }
    if ruleType == "file"         { return finding["file"] }
    return finding["file"] + ":" + pat
}

// ─────────────────────────────────────────────────────────────────
// SKIP LISTS
// ─────────────────────────────────────────────────────────────────

let SKIP_DIRS = {
    ".git": true, "node_modules": true, "vendor": true, ".venv": true,
    "__pycache__": true, "target": true, "dist": true, "build": true,
    ".next": true, ".cache": true, ".idea": true, ".vscode": true,
}

let SKIP_EXTS = {
    ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".webp": true,
    ".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".xz": true, ".bz2": true,
    ".7z": true, ".rar": true, ".dmg": true, ".iso": true,
    ".exe": true, ".dll": true, ".so": true, ".dylib": true, ".o": true, ".a": true,
    ".class": true, ".jar": true, ".wasm": true, ".bin": true,
    ".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".wav": true,
    ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
    ".sqlite": true, ".db": true, ".db3": true,
}

let SEV_RANK = { "CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3 }

let SUB_LINE_CHUNK  = 500
let WORKER_MULTIPLIER = 4

// ─────────────────────────────────────────────────────────────────
// DISPLAY HELPERS
// ─────────────────────────────────────────────────────────────────

fn truncateMatch(s) {
    let n = len(s)
    if n <= 40 { return s }
    return substr(s, 0, 18) + "…" + substr(s, n - 18, n)
}

fn fitText(s, maxLen) {
    if len(s) <= maxLen { return s }
    return substr(s, 0, maxLen - 1) + "…"
}

// ─────────────────────────────────────────────────────────────────
// FILE ENUMERATION
// ─────────────────────────────────────────────────────────────────

fn lowerExt(name) {
    let n = len(name)
    let i = n - 1
    while i >= 0 {
        let c = name[i]
        if c == "." { return lower(substr(name, i)) }
        if c == "/" { return "" }
        i = i - 1
    }
    return ""
}

fn shouldKeep(entry, maxBytes) {
    if entry["isSymlink"] == true { return false }
    if entry["isDir"] == true { return false }
    let ext = lowerExt(entry["name"])
    if SKIP_EXTS[ext] == true { return false }
    if entry["size"] > maxBytes { return false }
    return true
}

fn countFiles(root, maxBytes) {
    let entries, err = _fsReadDir(root)
    if err != null { return 0 }
    let total = 0
    let i = 0
    let n = len(entries)
    while i < n {
        let e = entries[i]
        if e["isSymlink"] != true {
            if e["isDir"] == true {
                if SKIP_DIRS[e["name"]] != true {
                    total = total + countFiles(p.join(root, e["name"]), maxBytes)
                }
            } else {
                if shouldKeep(e, maxBytes) == true { total = total + 1 }
            }
        }
        i = i + 1
    }
    return total
}

fn fillFiles(root, files, idx, maxBytes) {
    let entries, err = _fsReadDir(root)
    if err != null { return idx }
    let i = 0
    let n = len(entries)
    while i < n {
        let e = entries[i]
        if e["isSymlink"] != true {
            if e["isDir"] == true {
                if SKIP_DIRS[e["name"]] != true {
                    idx = fillFiles(p.join(root, e["name"]), files, idx, maxBytes)
                }
            } else {
                if shouldKeep(e, maxBytes) == true {
                    files[idx] = p.join(root, e["name"])
                    idx = idx + 1
                }
            }
        }
        i = i + 1
    }
    return idx
}

fn enumerateFiles(root, maxBytes) {
    let total = countFiles(root, maxBytes)
    let files = makeArray(total)
    fillFiles(root, files, 0, maxBytes)
    return files
}

// ─────────────────────────────────────────────────────────────────
// PER-FILE SCAN
// ─────────────────────────────────────────────────────────────────

fn shouldSkipPath(path) {
    let parts = split(path, "/")
    let i = 0
    let n = len(parts)
    while i < n {
        if SKIP_DIRS[parts[i]] == true { return true }
        i = i + 1
    }
    return false
}

fn scanFileLineRange(lines_arr, filePath, startLine, endLine) {
    let npats = len(PATTERNS)
    let total = 0
    let li = startLine
    while li < endLine {
        let line = lines_arr[li]
        if len(line) > 0 {
            let pi = 0
            while pi < npats {
                let pat = PATTERNS[pi]
                let needle = pat["needle"]
                let mightMatch = false
                if needle == "" {
                    mightMatch = true
                } else {
                    if indexOf(line, needle) >= 0 { mightMatch = true }
                }
                if mightMatch == true {
                    let matches, rerr = _regexFindAll(pat["regex"], line)
                    if rerr == null { total = total + len(matches) }
                }
                pi = pi + 1
            }
        }
        li = li + 1
    }

    let out = makeArray(total)
    if total == 0 { return out }

    let idx = 0
    li = startLine
    while li < endLine {
        let line = lines_arr[li]
        if len(line) > 0 {
            let pi = 0
            while pi < npats {
                let pat = PATTERNS[pi]
                let needle = pat["needle"]
                let mightMatch = false
                if needle == "" {
                    mightMatch = true
                } else {
                    if indexOf(line, needle) >= 0 { mightMatch = true }
                }
                if mightMatch == true {
                    let matches, rerr = _regexFindAll(pat["regex"], line)
                    if rerr == null {
                        let mi = 0
                        let nm = len(matches)
                        while mi < nm {
                            out[idx] = {
                                "source":      "file",
                                "patternName": pat["name"],
                                "severity":    pat["severity"],
                                "action":      pat["action"],
                                "file":        filePath,
                                "line":        li + 1,
                                "match":       matches[mi],
                                "commit":      "",
                                "author":      "",
                                "date":        "",
                            }
                            idx = idx + 1
                            mi = mi + 1
                        }
                    }
                }
                pi = pi + 1
            }
        }
        li = li + 1
    }
    return out
}

fn scanOneFile(filePath) {
    let content, ferr = _fsRead(filePath)
    if ferr != null { return makeArray(0) }

    let lines_arr = split(content, "\n")
    let nlines = len(lines_arr)

    if nlines <= SUB_LINE_CHUNK {
        return scanFileLineRange(lines_arr, filePath, 0, nlines)
    }

    let numChunks = (nlines + SUB_LINE_CHUNK - 1) / SUB_LINE_CHUNK
    let tasks = makeArray(numChunks, null)
    let i = 0
    while i < numChunks {
        let s = i * SUB_LINE_CHUNK
        let e = s + SUB_LINE_CHUNK
        if e > nlines { e = nlines }
        let path = filePath
        let lns  = lines_arr
        tasks[i] = async(fn() { return scanFileLineRange(lns, path, s, e) })
        i = i + 1
    }

    let chunkResults = makeArray(numChunks, null)
    i = 0
    while i < numChunks {
        chunkResults[i] = await(tasks[i])
        i = i + 1
    }

    let total = 0
    i = 0
    while i < numChunks { total = total + len(chunkResults[i])   i = i + 1 }

    let out = makeArray(total)
    let idx = 0
    i = 0
    while i < numChunks {
        let sub = chunkResults[i]
        let m = len(sub)
        let j = 0
        while j < m { out[idx] = sub[j]   idx = idx + 1   j = j + 1 }
        i = i + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// PER-COMMIT GIT SCAN
// ─────────────────────────────────────────────────────────────────

fn scanCommitFileBlock(lines_arr, startLine, endLine, currentFile, hash, author, date) {
    let npats = len(PATTERNS)
    let total = 0
    let li = startLine
    while li < endLine {
        let line = lines_arr[li]
        let n = len(line)
        if n > 0 {
            if line[0] == "+" {
                let isHeader = false
                if n >= 3 { if substr(line, 0, 3) == "+++" { isHeader = true } }
                if isHeader == false {
                    let body = substr(line, 1)
                    let pi = 0
                    while pi < npats {
                        let pat = PATTERNS[pi]
                        let needle = pat["needle"]
                        let mightMatch = false
                        if needle == "" {
                            mightMatch = true
                        } else {
                            if indexOf(body, needle) >= 0 { mightMatch = true }
                        }
                        if mightMatch == true {
                            let matches, rerr = _regexFindAll(pat["regex"], body)
                            if rerr == null { total = total + len(matches) }
                        }
                        pi = pi + 1
                    }
                }
            }
        }
        li = li + 1
    }

    let out = makeArray(total)
    if total == 0 { return out }

    let idx = 0
    li = startLine
    while li < endLine {
        let line = lines_arr[li]
        let n = len(line)
        if n > 0 {
            if line[0] == "+" {
                let isHeader = false
                if n >= 3 { if substr(line, 0, 3) == "+++" { isHeader = true } }
                if isHeader == false {
                    let body = substr(line, 1)
                    let pi = 0
                    while pi < npats {
                        let pat = PATTERNS[pi]
                        let needle = pat["needle"]
                        let mightMatch = false
                        if needle == "" {
                            mightMatch = true
                        } else {
                            if indexOf(body, needle) >= 0 { mightMatch = true }
                        }
                        if mightMatch == true {
                            let matches, rerr = _regexFindAll(pat["regex"], body)
                            if rerr == null {
                                let mi = 0
                                let nm = len(matches)
                                while mi < nm {
                                    out[idx] = {
                                        "source":      "git",
                                        "patternName": pat["name"],
                                        "severity":    pat["severity"],
                                        "action":      pat["action"],
                                        "file":        currentFile,
                                        "line":        0,
                                        "match":       matches[mi],
                                        "commit":      hash,
                                        "author":      author,
                                        "date":        date,
                                    }
                                    idx = idx + 1
                                    mi = mi + 1
                                }
                            }
                        }
                        pi = pi + 1
                    }
                }
            }
        }
        li = li + 1
    }
    return out
}

fn scanOneCommit(hash, repoPath) {
    let stdout, stderr, code, gerr = _processExec("git", [
        "-C", repoPath, "show", "--no-color", "--unified=0",
        "--pretty=format:%H%n%an%n%ae%n%ad", hash,
    ])
    if gerr != null { return makeArray(0) }
    if code != 0   { return makeArray(0) }

    let lines_arr = split(stdout, "\n")
    let nlines = len(lines_arr)
    if nlines < 4 { return makeArray(0) }

    let author = lines_arr[1]
    let date   = lines_arr[3]

    let headerCount = 0
    let li = 4
    while li < nlines {
        let line = lines_arr[li]
        if len(line) >= 12 {
            if startsWith(line, "diff --git ") == true { headerCount = headerCount + 1 }
        }
        li = li + 1
    }
    if headerCount == 0 { return makeArray(0) }

    let blocks = makeArray(headerCount)
    let bi = 0
    li = 4
    while li < nlines {
        let line = lines_arr[li]
        if len(line) >= 12 {
            if startsWith(line, "diff --git ") == true {
                if bi > 0 { blocks[bi - 1]["endLine"] = li }
                let bIdx = indexOf(line, " b/")
                let filePath = ""
                if bIdx >= 0 { filePath = substr(line, bIdx + 3) }
                blocks[bi] = {
                    "startLine": li, "endLine": nlines,
                    "filePath": filePath, "skip": shouldSkipPath(filePath),
                }
                bi = bi + 1
            }
        }
        li = li + 1
    }

    let activeCount = 0
    let i = 0
    while i < headerCount {
        if blocks[i]["skip"] == false { activeCount = activeCount + 1 }
        i = i + 1
    }
    if activeCount == 0 { return makeArray(0) }

    let tasks = makeArray(activeCount, null)
    let ti = 0
    i = 0
    while i < headerCount {
        let b = blocks[i]
        if b["skip"] == false {
            let s = b["startLine"]
            let e = b["endLine"]
            let path = b["filePath"]
            let lns = lines_arr
            let h = hash
            let a = author
            let d = date
            tasks[ti] = async(fn() { return scanCommitFileBlock(lns, s, e, path, h, a, d) })
            ti = ti + 1
        }
        i = i + 1
    }

    let perBlock = makeArray(activeCount, null)
    i = 0
    while i < activeCount { perBlock[i] = await(tasks[i])   i = i + 1 }

    let total = 0
    i = 0
    while i < activeCount { total = total + len(perBlock[i])   i = i + 1 }

    let out = makeArray(total)
    let idx = 0
    i = 0
    while i < activeCount {
        let sub = perBlock[i]
        let m = len(sub)
        let j = 0
        while j < m { out[idx] = sub[j]   idx = idx + 1   j = j + 1 }
        i = i + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// GIT REPO DISCOVERY
// Walk the directory tree and return every directory that contains
// a .git subdirectory (i.e. every git repo root found under root).
// Respects SKIP_DIRS so node_modules, vendor, build etc. are not
// recursed into — no accidental scanning of npm-installed packages.
// ─────────────────────────────────────────────────────────────────

fn countGitRepos(root) {
    let entries, err = _fsReadDir(root)
    if err != null { return 0 }
    let total  = 0
    let hasGit = false
    let n = len(entries)
    let i = 0
    while i < n {
        let e = entries[i]
        if e["isSymlink"] != true && e["isDir"] == true {
            if e["name"] == ".git" {
                hasGit = true
            } else if SKIP_DIRS[e["name"]] != true {
                total = total + countGitRepos(p.join(root, e["name"]))
            }
        }
        i = i + 1
    }
    if hasGit { total = total + 1 }
    return total
}

fn fillGitRepos(root, repos, idx) {
    let entries, err = _fsReadDir(root)
    if err != null { return idx }
    let hasGit = false
    let n = len(entries)
    let i = 0
    while i < n {
        let e = entries[i]
        if e["isSymlink"] != true && e["isDir"] == true {
            if e["name"] == ".git" {
                hasGit = true
            }
        }
        i = i + 1
    }
    if hasGit {
        repos[idx] = root
        idx = idx + 1
    }
    i = 0
    while i < n {
        let e = entries[i]
        if e["isSymlink"] != true && e["isDir"] == true {
            if SKIP_DIRS[e["name"]] != true {
                idx = fillGitRepos(p.join(root, e["name"]), repos, idx)
            }
        }
        i = i + 1
    }
    return idx
}

fn findGitRepos(root) {
    let total = countGitRepos(root)
    if total == 0 { return makeArray(0) }
    let repos = makeArray(total)
    fillGitRepos(root, repos, 0)
    return repos
}

// ─────────────────────────────────────────────────────────────────
// MULTI-REPO COMMIT GATHERING
// Enumerate commits from every discovered repo and flatten into a
// single array of {commit, repo} pairs so the parallel worker pool
// can distribute work across all repos simultaneously.
// ─────────────────────────────────────────────────────────────────

fn gatherAllCommits(repos) {
    let nRepos = len(repos)
    if nRepos == 0 { return makeArray(0) }

    let repoCommits = makeArray(nRepos, null)
    let repoCounts  = makeArray(nRepos, 0)
    let total = 0
    let ri = 0
    while ri < nRepos {
        let commits, ok = enumerateCommits(repos[ri])
        if ok == true {
            repoCommits[ri] = commits
            repoCounts[ri]  = len(commits)
            total = total + len(commits)
        } else {
            repoCommits[ri] = makeArray(0)
            repoCounts[ri]  = 0
        }
        ri = ri + 1
    }

    let pairs = makeArray(total)
    let idx = 0
    ri = 0
    while ri < nRepos {
        let cmts = repoCommits[ri]
        let cnt  = repoCounts[ri]
        let ci = 0
        while ci < cnt {
            pairs[idx] = { "commit": cmts[ci], "repo": repos[ri] }
            idx = idx + 1
            ci = ci + 1
        }
        ri = ri + 1
    }
    return pairs
}

// ─────────────────────────────────────────────────────────────────
// GIT COMMIT ENUMERATION
// ─────────────────────────────────────────────────────────────────

fn enumerateCommits(repoPath) {
    let stdout, stderr, code, err = _processExec("git", [
        "-C", repoPath, "rev-list", "--all", "--no-merges",
    ])
    if err != null  { return makeArray(0), false }
    if code != 0    { return makeArray(0), false }

    let n = len(stdout)
    if n == 0 { return makeArray(0), true }

    let total = 0
    let i = 0
    while i < n {
        if stdout[i] == "\n" { total = total + 1 }
        i = i + 1
    }
    if stdout[n - 1] != "\n" { total = total + 1 }

    let commits = makeArray(total)
    let idx = 0
    let start = 0
    i = 0
    while i < n {
        if stdout[i] == "\n" {
            commits[idx] = substr(stdout, start, i)
            idx = idx + 1
            start = i + 1
        }
        i = i + 1
    }
    if start < n { commits[idx] = substr(stdout, start, n) }
    return commits, true
}

// ─────────────────────────────────────────────────────────────────
// PARALLEL SCAN
// ─────────────────────────────────────────────────────────────────

fn parScanFiles(files) {
    let n = len(files)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myFiles = files
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneFile(myFiles[li])
                li = li + 1
                oi = oi + 1
            }
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanCommits(commits, repoPath) {
    let n = len(commits)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myCommits = commits
        let myRepo = repoPath
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneCommit(myCommits[li], myRepo)
                li = li + 1
                oi = oi + 1
            }
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn flattenWorkerResults(perWorker) {
    let nw = len(perWorker)
    let total = 0
    let i = 0
    while i < nw {
        let chunkArr = perWorker[i]
        let m = len(chunkArr)
        let j = 0
        while j < m { total = total + len(chunkArr[j])   j = j + 1 }
        i = i + 1
    }
    let out = makeArray(total)
    let idx = 0
    i = 0
    while i < nw {
        let chunkArr = perWorker[i]
        let m = len(chunkArr)
        let j = 0
        while j < m {
            let sub = chunkArr[j]
            let k = 0
            let ks = len(sub)
            while k < ks { out[idx] = sub[k]   idx = idx + 1   k = k + 1 }
            j = j + 1
        }
        i = i + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// SORT
// ─────────────────────────────────────────────────────────────────

fn findingRank(f) {
    let sevR = SEV_RANK[f["severity"]]
    let srcR = 0
    if f["source"] == "git" { srcR = 1 }
    return sevR * 2 + srcR
}

fn sortFindings(findings) {
    return sortBy(findings, fn(a, b) { return findingRank(a) < findingRank(b) })
}

// ─────────────────────────────────────────────────────────────────
// PROGRESS-AWARE PARALLEL SCAN
// Each worker sends one message on progressCh when its chunk is done.
// The UI drains progressCh each frame with recvNonBlock and accumulates
// the "done" field to drive a progress bar.
// ─────────────────────────────────────────────────────────────────

fn parScanFilesWithProgress(files, progressCh) {
    let n = len(files)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myFiles = files
        let pch = progressCh
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneFile(myFiles[li])
                li = li + 1
                oi = oi + 1
            }
            send(pch, {"phase": "files_progress", "done": e - s, "total": 0})
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanCommitsWithProgress(commits, repoPath, progressCh) {
    let n = len(commits)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myCommits = commits
        let myRepo = repoPath
        let pch = progressCh
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneCommit(myCommits[li], myRepo)
                li = li + 1
                oi = oi + 1
            }
            send(pch, {"phase": "git_progress", "done": e - s, "total": 0})
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

// parScanAllCommits and parScanAllCommitsWithProgress work over a flat
// array of {commit, repo} pairs so a single worker pool covers every
// discovered repository — maximising parallelism across all repos.

fn parScanAllCommits(pairs) {
    let n = len(pairs)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myPairs = pairs
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneCommit(myPairs[li]["commit"], myPairs[li]["repo"])
                li = li + 1
                oi = oi + 1
            }
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanAllCommitsWithProgress(pairs, progressCh) {
    let n = len(pairs)
    if n == 0 { return makeArray(0) }

    let numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    let base = n / numWorkers
    let rem  = n % numWorkers
    let tasks = makeArray(numWorkers, null)
    let start = 0
    let w = 0
    while w < numWorkers {
        let size = base
        if w < rem { size = size + 1 }
        let s = start
        let e = start + size
        let myPairs = pairs
        let pch = progressCh
        tasks[w] = async(fn() {
            let local = makeArray(e - s, null)
            let li = s
            let oi = 0
            while li < e {
                local[oi] = scanOneCommit(myPairs[li]["commit"], myPairs[li]["repo"])
                li = li + 1
                oi = oi + 1
            }
            send(pch, {"phase": "git_progress", "done": e - s, "total": 0})
            return local
        })
        start = e
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn runScanWithProgress(root, doGit, maxSizeMB, progressCh, yaraRulesFile = null, doEntropy = false, doJWT = true) {
    let maxBytes   = maxSizeMB * 1024 * 1024
    let totalStart = _timeNanos()

    send(progressCh, {"phase": "enumerate", "total": 0, "done": 0})
    let files     = enumerateFiles(root, maxBytes)
    let fileCount = len(files)

    let filesStart = _timeNanos()
    send(progressCh, {"phase": "files", "total": fileCount, "done": 0})
    let perWorkerFiles = parScanFilesWithProgress(files, progressCh)
    let fileFindings   = flattenWorkerResults(perWorkerFiles)
    let filesEnd = _timeNanos()

    let repoCount   = 0
    let commitCount = 0
    let gitFindings = makeArray(0)
    let gitStart = filesEnd
    let gitEnd   = filesEnd
    if doGit == true {
        gitStart = _timeNanos()
        send(progressCh, {"phase": "git_enumerate", "total": 0, "done": 0})
        let repos     = findGitRepos(root)
        repoCount = len(repos)
        if repoCount > 0 {
            let pairs       = gatherAllCommits(repos)
            commitCount = len(pairs)
            if commitCount > 0 {
                send(progressCh, {"phase": "git", "total": commitCount, "done": 0, "repoCount": repoCount})
                let perWorkerCommits = parScanAllCommitsWithProgress(pairs, progressCh)
                gitFindings      = flattenWorkerResults(perWorkerCommits)
            }
        }
        gitEnd = _timeNanos()
    }

    let yaraFindings = makeArray(0)
    let yaraStart = _timeNanos()
    if yaraRulesFile != null {
        send(progressCh, {"phase": "yara", "total": fileCount, "done": 0})
        yaraFindings = scanYaraFilesParallel(yaraRulesFile, files, progressCh)
        send(progressCh, {"phase": "yara_done", "total": fileCount, "done": fileCount})
    }
    let yaraEnd = _timeNanos()

    let entropyFindings = makeArray(0)
    let entropyStart = _timeNanos()
    if doEntropy == true {
        send(progressCh, {"phase": "entropy", "total": fileCount, "done": 0})
        entropyFindings = scanEntropyFilesParallel(files, progressCh)
        send(progressCh, {"phase": "entropy_done", "total": fileCount, "done": fileCount})
    }
    let entropyEnd = _timeNanos()

    let totalEnd = _timeNanos()

    let totalSec = float(totalEnd - totalStart) / 1000000000.0
    let filesSec = float(filesEnd - filesStart) / 1000000000.0
    let gitSec   = float(gitEnd   - gitStart)   / 1000000000.0
    let yaraSec  = float(yaraEnd  - yaraStart)  / 1000000000.0

    let filesPerSec   = 0.0
    let commitsPerSec = 0.0
    if filesSec > 0.001 { filesPerSec   = float(fileCount)   / filesSec }
    if gitSec   > 0.001 { commitsPerSec = float(commitCount) / gitSec   }

    let nf  = len(fileFindings)
    let ng  = len(gitFindings)
    let ny  = len(yaraFindings)
    let ne  = len(entropyFindings)
    let all = makeArray(nf + ng + ny + ne)
    let i = 0
    while i < nf { all[i] = fileFindings[i]                    i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]                i = i + 1 }
    i = 0
    while i < ny { all[nf + ng + i] = yaraFindings[i]          i = i + 1 }
    i = 0
    while i < ne { all[nf + ng + ny + i] = entropyFindings[i]  i = i + 1 }

    let enriched = enrichWithJWT(all, doJWT)

    return {
        "findings":      sortFindings(enriched),
        "fileCount":     fileCount,
        "commitCount":   commitCount,
        "repoCount":     repoCount,
        "totalSec":      totalSec,
        "filesSec":      filesSec,
        "gitSec":        gitSec,
        "yaraSec":       yaraSec,
        "filesPerSec":   filesPerSec,
        "commitsPerSec": commitsPerSec,
    }
}

// ─────────────────────────────────────────────────────────────────
// YARA INTEGRATION
// Optional third scan phase using YARA rules via nativeBridge.
// Complements the regex patterns — YARA catches what regex misses:
// binary secrets, multi-string correlations, entropy heuristics.
//
// Uses N parallel bridge processes (one per async worker) so YARA
// scales across all CPU cores instead of running single-threaded.
// Each worker sends its entire file chunk in ONE scan_batch call,
// eliminating per-file round-trip overhead.
//
// Usage:
//   // Pass the rules file path to runScan / runScanWithProgress
//   result = runScan(root, true, 5, "tests/examples/SecretHunter/secrets.yar")
// ─────────────────────────────────────────────────────────────────

let YARA_WORKERS = 16

// startYaraBridge — kept for standalone / CLI use (yaraTest.lex etc.)
fn startYaraBridge(rulesFile) {
    let bridge, err = nativeBridge("python3", [_scriptDir() + "/yara_bridge.py"])
    if err != null { return null, err }
    _, err = bridgeCall(bridge, "load", [rulesFile])
    if err != null {
        bridgeClose(bridge)
        return null, err
    }
    return bridge, null
}

// _yaraFlushBatch — scan one batch of files and append findings to accum hash.
// Returns the updated accumCount.
fn _yaraFlushBatch(bridge, batch, batchCount, accum, accumCount) {
    let batchArr = makeArray(batchCount, "")
    let k = 0
    while k < batchCount { batchArr[k] = batch[k]  k = k + 1 }
    let matches, err = bridgeCall(bridge, "scan_batch", [batchArr])
    if err != null { return accumCount }
    let i = 0
    while i < len(matches) {
        let hit = matches[i]
        accum[accumCount] = {
            "source":      "yara",
            "patternName": hit["rule"],
            "severity":    hit["severity"],
            "action":      hit["action"],
            "file":        hit["file"],
            "line":        0,
            "match":       hit["match"],
            "commit":      "",
            "author":      "",
            "date":        "",
        }
        accumCount = accumCount + 1
        i = i + 1
    }
    return accumCount
}

// scanYaraChunkBatch — one worker: starts its own Python YARA bridge, loads
// rules, streams findings + progress for its files. When progressCh is
// non-null (UI path), per-finding events ({"phase":"yara_finding",
// "severity":...}) feed the sidebar tiles and per-file events
// ({"phase":"yara_progress", "done":1}) tick the progress bar.
fn scanYaraChunkBatch(rulesFile, files, startIdx, endIdx, progressCh) {
    let bridge, err = nativeBridge("python3", [_scriptDir() + "/yara_bridge.py"])
    if err != null { return makeArray(0) }

    _, err = bridgeCall(bridge, "load", [rulesFile])
    if err != null { bridgeClose(bridge)  return makeArray(0) }

    let chunkSize = endIdx - startIdx
    let chunk = makeArray(chunkSize, "")
    let i = startIdx
    let j = 0
    while i < endIdx { chunk[j] = files[i]  i = i + 1  j = j + 1 }

    let ch, err = bridgeStream(bridge, "scan_batch_stream", [chunk])
    if err != null { bridgeClose(bridge)  return makeArray(0) }

    // Doubling buffer — findings arrive interleaved with progress markers
    // and we don't know the final count up front.
    let cap = 16
    let buf = makeArray(cap, null)
    let count = 0
    for item in ch {
        if type(item) == "ERROR" {
            bridgeClose(bridge)
            let out = makeArray(count)
            let k = 0
            while k < count { out[k] = buf[k]  k = k + 1 }
            return out
        }
        let kind = item["kind"]
        if kind == "progress" {
            if progressCh != null {
                send(progressCh, {"phase": "yara_progress", "done": 1})
            }
        } else if kind == "finding" {
            let sev = item["severity"]
            let finding = {
                "source":      "yara",
                "patternName": item["rule"],
                "severity":    sev,
                "action":      item["action"],
                "file":        item["file"],
                "line":        0,
                "match":       item["match"],
                "commit":      "",
                "author":      "",
                "date":        "",
            }
            if count == cap {
                cap = cap * 2
                let grown = makeArray(cap, null)
                let k = 0
                while k < count { grown[k] = buf[k]  k = k + 1 }
                buf = grown
            }
            buf[count] = finding
            count = count + 1
            if progressCh != null {
                send(progressCh, {"phase": "yara_finding", "severity": sev})
            }
        }
    }
    bridgeClose(bridge)

    let out = makeArray(count)
    let k = 0
    while k < count { out[k] = buf[k]  k = k + 1 }
    return out
}

// scanYaraFilesParallel — round-robin distribution, one scan_batch per worker.
//
// Files are assigned round-robin: worker 0 gets files [0,16,32,...],
// worker 1 gets [1,17,33,...], etc. This interleaves files from every
// directory so large files are spread evenly across all workers rather
// than clustering in one chunk. Each worker starts its own Python YARA
// process and calls scan_batch ONCE with its entire file list — no
// polling loop, no channel overhead, minimal kLex interpreter work.
fn scanYaraFilesParallel(rulesFile, files, progressCh) {
    let n = len(files)
    if n == 0 { return makeArray(0) }

    let numWorkers = YARA_WORKERS
    if numWorkers > n { numWorkers = n }

    let tasks = makeArray(numWorkers, null)
    let w = 0
    while w < numWorkers {
        // Count and pre-allocate this worker's share.
        let wCount = 0
        let j = w
        while j < n { wCount = wCount + 1  j = j + numWorkers }

        let chunk = makeArray(wCount, "")
        j = w
        let k = 0
        while j < n { chunk[k] = files[j]  k = k + 1  j = j + numWorkers }

        let myRules = rulesFile
        let myChunk = chunk
        let myCh    = progressCh
        tasks[w] = async(fn() { return scanYaraChunkBatch(myRules, myChunk, 0, len(myChunk), myCh) })
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        let result, err = safe(await, tasks[w])
        if err != null { perWorker[w] = makeArray(0) }
        else           { perWorker[w] = result }
        w = w + 1
    }

    let total = 0
    w = 0
    while w < numWorkers { total = total + len(perWorker[w])  w = w + 1 }

    let out = makeArray(total)
    let idx = 0
    w = 0
    while w < numWorkers {
        let sub = perWorker[w]
        let m = len(sub)
        let j = 0
        while j < m { out[idx] = sub[j]  idx = idx + 1  j = j + 1 }
        w = w + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// ENTROPY DETECTION
// Optional scan phase using Shannon entropy analysis to find high-
// entropy strings that are statistically likely to be credentials,
// even when they match no known regex or YARA pattern.
// Strings with entropy ≥ 4.5 bits/char are flagged. Real secrets
// (API keys, tokens, passwords) are designed to be unpredictable;
// normal text and code identifiers score much lower.
// ─────────────────────────────────────────────────────────────────

let ENTROPY_WORKERS = 16

// scanEntropyChunk — scans one worker's chunk via the streaming bridge.
//
// Uses entropy_scan_stream so per-file progress and per-finding events arrive
// live on the worker's stdout pipe instead of as one batch at the end. When
// progressCh is non-null (UI path), each finding emits {"phase":"entropy_finding",
// "severity": ...} for the sidebar tiles and each file emits {"phase":
// "entropy_progress", "done": 1} for the progress bar. The CLI path passes
// progressCh = null and just collects results.
fn scanEntropyChunk(files, startIdx, endIdx, progressCh) {
    let bridge, err = nativeBridge("python3", [_scriptDir() + "/yara_bridge.py"])
    if err != null { return makeArray(0) }

    let chunkSize = endIdx - startIdx
    let chunk = makeArray(chunkSize, "")
    let i = startIdx
    let j = 0
    while i < endIdx { chunk[j] = files[i]  i = i + 1  j = j + 1 }

    let ch, err = bridgeStream(bridge, "entropy_scan_stream", [chunk])
    if err != null { bridgeClose(bridge)  return makeArray(0) }

    // Two-pass collect: count first so the result array is pre-allocated.
    // Findings arrive interleaved with progress items, so we buffer findings
    // in a doubling array (no push() per kLex rules — manual capacity).
    let cap = 16
    let buf = makeArray(cap, null)
    let count = 0
    for item in ch {
        if type(item) == "ERROR" {
            // mid-stream error — bail out with whatever we collected so far
            bridgeClose(bridge)
            let out = makeArray(count)
            let k = 0
            while k < count { out[k] = buf[k]  k = k + 1 }
            return out
        }
        let kind = item["kind"]
        if kind == "progress" {
            if progressCh != null {
                send(progressCh, {"phase": "entropy_progress", "done": 1})
            }
        } else if kind == "finding" {
            let ent = item["entropy"]
            let sev = item["severity"]
            let finding = {
                "source":      "entropy",
                "patternName": "High-Entropy String (" + str(ent) + " bits)",
                "severity":    sev,
                "action":      "Shannon entropy suggests a possible credential or key. Review and rotate if this is sensitive data.",
                "file":        item["file"],
                "line":        0,
                "match":       item["match"],
                "commit":      "",
                "author":      "",
                "date":        "",
            }
            if count == cap {
                cap = cap * 2
                let grown = makeArray(cap, null)
                let k = 0
                while k < count { grown[k] = buf[k]  k = k + 1 }
                buf = grown
            }
            buf[count] = finding
            count = count + 1
            if progressCh != null {
                send(progressCh, {"phase": "entropy_finding", "severity": sev})
            }
        }
    }
    bridgeClose(bridge)

    let out = makeArray(count)
    let k = 0
    while k < count { out[k] = buf[k]  k = k + 1 }
    return out
}

fn scanEntropyFilesParallel(files, progressCh) {
    let n = len(files)
    if n == 0 { return makeArray(0) }

    let numWorkers = ENTROPY_WORKERS
    if numWorkers > n { numWorkers = n }

    let tasks = makeArray(numWorkers, null)
    let w = 0
    while w < numWorkers {
        let wCount = 0
        let j = w
        while j < n { wCount = wCount + 1  j = j + numWorkers }

        let chunk = makeArray(wCount, "")
        j = w
        let k = 0
        while j < n { chunk[k] = files[j]  k = k + 1  j = j + numWorkers }

        let myChunk = chunk
        let myCh    = progressCh
        tasks[w] = async(fn() { return scanEntropyChunk(myChunk, 0, len(myChunk), myCh) })
        w = w + 1
    }

    let perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        let result, err = safe(await, tasks[w])
        if err != null { perWorker[w] = makeArray(0) }
        else           { perWorker[w] = result }
        w = w + 1
    }

    let total = 0
    w = 0
    while w < numWorkers { total = total + len(perWorker[w])  w = w + 1 }

    let out = makeArray(total)
    let idx = 0
    w = 0
    while w < numWorkers {
        let sub = perWorker[w]
        let m = len(sub)
        let j = 0
        while j < m { out[idx] = sub[j]  idx = idx + 1  j = j + 1 }
        w = w + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// JWT ENRICHMENT (Node bridge)
//
// Runs after all detection phases. For every finding whose `match` looks
// like a JWT (three base64url segments, header + payload both starting
// with "eyJ" which decodes to the leading `{` of their JSON), call the
// Node JWT bridge to decode and attach a `jwt` field. Critically, we
// do NOT call the bridge at all when no findings look like JWTs — the
// subprocess only spawns for scans that need it.
//
// The attached `jwt` hash carries everything jwt_bridge.js returns:
//   alg, typ, alg_warning, iss, sub, aud, exp_iso/iat_iso/nbf_iso,
//   expired, expires_soon, missing_exp, scopes, error
// ─────────────────────────────────────────────────────────────────

// isLikelyJWT — cheap pre-filter so we don't ship every entropy-flagged
// random string to the Node bridge. Both header and payload segments are
// base64-encoded JSON objects, so each starts with `{` whose base64 prefix
// is `eyJ`. False positives are still possible but the bridge handles them
// safely (returns a per-token error row).
fn isLikelyJWT(s) {
    if type(s) != "STRING"   { return false }
    if !startsWith(s, "eyJ") { return false }
    if len(s) < 30           { return false }
    let parts = split(s, ".")
    if len(parts) != 3       { return false }
    if !startsWith(parts[1], "eyJ") { return false }
    return true
}

// enrichWithJWT — augment findings whose match looks like a JWT. Returns the
// same findings array with a `jwt` field attached on each enriched entry.
// Findings that don't look like JWTs are returned unchanged.
fn enrichWithJWT(findings, doJWT) {
    if doJWT == false { return findings }
    let n = len(findings)
    if n == 0 { return findings }

    // Two-pass: count + collect tokens to send in one batch, then map results
    // back by index. Avoids spawning the bridge if there are zero JWT shapes.
    let jwtIdx = makeArray(n, -1)
    let tokenCount = 0
    let i = 0
    while i < n {
        if isLikelyJWT(findings[i]["match"]) {
            jwtIdx[i]  = tokenCount
            tokenCount = tokenCount + 1
        }
        i = i + 1
    }
    if tokenCount == 0 { return findings }

    let tokens = makeArray(tokenCount, "")
    i = 0
    while i < n {
        if jwtIdx[i] >= 0 { tokens[jwtIdx[i]] = findings[i]["match"] }
        i = i + 1
    }

    let bridge, err = nativeBridge("node", [_scriptDir() + "/jwt_bridge.js"])
    if err != null {
        // Decode is optional — if Node isn't installed or the bridge fails to
        // start, leave findings untouched rather than failing the whole scan.
        return findings
    }
    let decoded, err = bridgeCall(bridge, "decode_batch", [tokens])
    bridgeClose(bridge)
    if err != null { return findings }

    // Merge decoded info back into each enriched finding. For findings that
    // were caught by the JWT regex specifically we also rewrite severity to
    // reflect what the decoded token actually says — alg:none earns a
    // CRITICAL, anything expired drops to LOW, etc. We DON'T rewrite for
    // findings caught by unrelated patterns that happened to also contain a
    // JWT-shaped substring; those keep the original pattern's verdict.
    i = 0
    while i < n {
        let idx = jwtIdx[i]
        if idx >= 0 && idx < len(decoded) {
            let d = decoded[idx]
            findings[i]["jwt"] = d
            if findings[i]["patternName"] == "JSON Web Token (JWT)" && d["error"] == null {
                findings[i]["severity"] = jwtSeverity(d)
            }
        }
        i = i + 1
    }
    return findings
}

// jwtSeverity — derive a finding severity from the decoded JWT. Ordering
// reflects rotation urgency, not abstract risk: a forgeable alg:none token
// is worse than a leaked HS256 token that's already expired (the latter
// still warrants rotation since the SIGNING SECRET may be live, but it's
// not the same fire as alg:none in production code).
fn jwtSeverity(d) {
    if d["alg"] == "none"                   { return "CRITICAL" }
    let weakAlg = d["alg_warning"] != null
    if weakAlg && d["expired"]              { return "LOW" }
    if weakAlg && d["missing_exp"]          { return "HIGH" }
    if weakAlg                              { return "HIGH" }
    if d["expired"]                         { return "LOW" }
    if d["missing_exp"]                     { return "MEDIUM" }
    return "MEDIUM"
}

// ─────────────────────────────────────────────────────────────────
// MAIN SCAN ENTRY POINT (CLI — no progress reporting)
// ─────────────────────────────────────────────────────────────────

fn runScan(root, doGit, maxSizeMB, yaraRulesFile = null, doEntropy = false, doJWT = true) {
    let maxBytes = maxSizeMB * 1024 * 1024

    let files     = enumerateFiles(root, maxBytes)
    let fileCount = len(files)

    let perWorkerFiles = parScanFiles(files)
    let fileFindings   = flattenWorkerResults(perWorkerFiles)

    let repoCount   = 0
    let commitCount = 0
    let gitFindings = makeArray(0)
    if doGit == true {
        let repos     = findGitRepos(root)
        repoCount = len(repos)
        if repoCount > 0 {
            let pairs       = gatherAllCommits(repos)
            commitCount = len(pairs)
            if commitCount > 0 {
                let perWorkerCommits = parScanAllCommits(pairs)
                gitFindings      = flattenWorkerResults(perWorkerCommits)
            }
        }
    }

    let yaraFindings = makeArray(0)
    if yaraRulesFile != null {
        yaraFindings = scanYaraFilesParallel(yaraRulesFile, files, null)
    }

    let entropyFindings = makeArray(0)
    if doEntropy == true {
        entropyFindings = scanEntropyFilesParallel(files, null)
    }

    let nf  = len(fileFindings)
    let ng  = len(gitFindings)
    let ny  = len(yaraFindings)
    let ne  = len(entropyFindings)
    let all = makeArray(nf + ng + ny + ne)
    let i = 0
    while i < nf { all[i] = fileFindings[i]                    i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]                i = i + 1 }
    i = 0
    while i < ny { all[nf + ng + i] = yaraFindings[i]          i = i + 1 }
    i = 0
    while i < ne { all[nf + ng + ny + i] = entropyFindings[i]  i = i + 1 }

    let enriched = enrichWithJWT(all, doJWT)

    return {
        "findings":    sortFindings(enriched),
        "fileCount":   fileCount,
        "commitCount": commitCount,
        "repoCount":   repoCount,
    }
}

// ─────────────────────────────────────────────────────────────────
// CONFIGURATION
// Reads / writes ~/.secrethunter/config (key=value format).
// Priority: env var > config file > built-in default.
// ─────────────────────────────────────────────────────────────────

fn _shHomeDir() {
    let stdout, err = _processShell("echo $HOME 2>/dev/null")
    if err != null { return "/tmp" }
    let h = trim(stdout)
    if len(h) == 0 { return "/tmp" }
    return h
}

fn _shEnvGet(varName) {
    let stdout, err = _processShell("echo $" + varName)
    if err != null { return "" }
    return trim(stdout)
}

fn secretHunterConfigPath() {
    return _shHomeDir() + "/.secrethunter/config"
}

fn _parseKVConfig(content) {
    let cfg = {}
    let lines = split(content, "\n")
    let i = 0
    let n = len(lines)
    while i < n {
        let line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" {
            let eqIdx = indexOf(line, "=")
            if eqIdx > 0 {
                cfg[trim(substr(line, 0, eqIdx))] = trim(substr(line, eqIdx + 1))
            }
        }
        i = i + 1
    }
    return cfg
}

fn loadSecretHunterConfig() {
    let defaults = {
        "github_bridge_path": "examples/SecretHunter/github_bridge.py",
        "python_executable":  "python3",
        "github_token":       "",
        "default_scan_mode":  "online",
        "temp_dir":           "/tmp",
    }
    let cfgPath = secretHunterConfigPath()
    let parsed  = {}
    if _fsExists(cfgPath) == true {
        let content, err = _fsRead(cfgPath)
        if err == null { parsed = _parseKVConfig(content) }
    }
    // Env vars override config file
    let envBridge = _shEnvGet("SECRETHUNTER_BRIDGE")
    let envPython = _shEnvGet("SECRETHUNTER_PYTHON")
    let envTmpDir = _shEnvGet("SECRETHUNTER_TMPDIR")
    let envToken  = _shEnvGet("GITHUB_TOKEN")
    if len(envBridge) > 0 { parsed["github_bridge_path"] = envBridge }
    if len(envPython) > 0 { parsed["python_executable"]  = envPython }
    if len(envTmpDir) > 0 { parsed["temp_dir"]           = envTmpDir }
    if len(envToken)  > 0 { parsed["github_token"]        = envToken  }
    // Fill any still-missing keys from defaults
    let dkeys = ["github_bridge_path", "python_executable", "github_token", "default_scan_mode", "temp_dir"]
    let ki = 0
    while ki < len(dkeys) {
        let k = dkeys[ki]
        if parsed[k] == null { parsed[k] = defaults[k] }
        ki = ki + 1
    }
    return parsed
}

fn saveSecretHunterConfig(cfg) {
    let cfgPath = secretHunterConfigPath()
    let cfgDir  = p.dirname(cfgPath)
    let _, merr = _fsMkdirAll(cfgDir)
    if merr != null { return merr }
    let content = "# SecretHunter configuration\n"
    content = content + "# Edit here or use the settings panel in the app.\n\n"
    content = content + "github_bridge_path=" + cfg["github_bridge_path"] + "\n"
    content = content + "python_executable="  + cfg["python_executable"]  + "\n"
    content = content + "github_token="       + cfg["github_token"]       + "\n"
    content = content + "default_scan_mode="  + cfg["default_scan_mode"]  + "\n"
    content = content + "temp_dir="           + cfg["temp_dir"]           + "\n"
    let _, err = _fsWrite(cfgPath, content)
    return err
}

// ─────────────────────────────────────────────────────────────────
// GITHUB URL HELPERS
// ─────────────────────────────────────────────────────────────────

fn isGitHubUrl(path) {
    return startsWith(lower(trim(path)), "https://github.com/")
}

fn extractOrgFromUrl(url) {
    // "https://github.com/myorg"       → "myorg"
    // "https://github.com/myorg/"      → "myorg"
    // "https://github.com/myorg/repo"  → "myorg"  (org only)
    let prefix = "https://github.com/"
    let rest   = substr(url, len(prefix))
    let slashIdx = indexOf(rest, "/")
    if slashIdx >= 0 { rest = substr(rest, 0, slashIdx) }
    return trim(rest)
}

// ─────────────────────────────────────────────────────────────────
// ARRAY MERGE HELPER
// ─────────────────────────────────────────────────────────────────

fn _mergeArrays(a, b) {
    let na = len(a)
    let nb = len(b)
    if nb == 0 { return a }
    if na == 0 { return b }
    let out = makeArray(na + nb)
    let i = 0
    while i < na { out[i]      = a[i]   i = i + 1 }
    i = 0
    while i < nb { out[na + i] = b[i]   i = i + 1 }
    return out
}

// ─────────────────────────────────────────────────────────────────
// GITHUB ORG SCAN
// Enumerates every repo in an org/user via github_bridge.py,
// fetches each one (tarball = online, blobless clone = deep),
// runs the existing scanner on the local copy, tags findings with
// the repo name, then cleans up.  Sends progress on progressCh in
// the same message format as runScanWithProgress so the UI needs
// only minimal additions.
// ─────────────────────────────────────────────────────────────────

fn runOrgScan(orgUrl, cfg, maxSizeMB, progressCh) {
    let org        = extractOrgFromUrl(orgUrl)
    let python     = cfg["python_executable"]
    if len(python) == 0 { python = "python3" }
    let bridgePath = cfg["github_bridge_path"]
    if len(bridgePath) == 0 { bridgePath = "tests/examples/SecretHunter/github_bridge.py" }
    let token      = cfg["github_token"]
    let deepMode   = cfg["default_scan_mode"] == "deep"
    let tmpBase    = cfg["temp_dir"]
    if len(tmpBase) == 0 { tmpBase = "/tmp" }

    let totalStart = _timeNanos()

    // Start the bridge process
    let bridge, berr = nativeBridge(python, [bridgePath])
    if berr != null {
        return {
            "error": "Could not start github_bridge.py: " + berr.message,
            "findings": makeArray(0), "fileCount": 0, "commitCount": 0,
            "repoCount": 0, "totalSec": 0.0, "filesSec": 0.0,
            "gitSec": 0.0, "yaraSec": 0.0, "filesPerSec": 0.0, "commitsPerSec": 0.0,
        }
    }

    // Remove any leftover temp dirs from a previous crashed run
    bridgeCall(bridge, "cleanup_stale", ["secrethunter_"])

    // List all repos in the org
    send(progressCh, {"phase": "org_list", "org": org})
    let listResp, lerr = bridgeCall(bridge, "list_repos", [org, token, true])
    if lerr != null {
        bridgeClose(bridge)
        return {
            "error": "Failed to list repos: " + lerr.message,
            "findings": makeArray(0), "fileCount": 0, "commitCount": 0,
            "repoCount": 0, "totalSec": 0.0, "filesSec": 0.0,
            "gitSec": 0.0, "yaraSec": 0.0, "filesPerSec": 0.0, "commitsPerSec": 0.0,
        }
    }
    if listResp["error"] != null {
        bridgeClose(bridge)
        return {
            "error": listResp["error"],
            "findings": makeArray(0), "fileCount": 0, "commitCount": 0,
            "repoCount": 0, "totalSec": 0.0, "filesSec": 0.0,
            "gitSec": 0.0, "yaraSec": 0.0, "filesPerSec": 0.0, "commitsPerSec": 0.0,
        }
    }

    let repos   = listResp["repos"]
    let nRepos  = len(repos)
    send(progressCh, {"phase": "org_repos", "total": nRepos, "done": 0, "repo": ""})

    let allFindings  = makeArray(0)
    let totalFiles   = 0
    let totalCommits = 0
    let scanned      = 0

    let i = 0
    while i < nRepos {
        let repo      = repos[i]
        let repoName  = repo["full_name"]
        let repoSlug  = replace(repoName, "/", "_")
        let repoTmp   = tmpBase + "/secrethunter_" + repoSlug

        send(progressCh, {"phase": "org_repo", "repo": repoName, "repoIdx": i, "total": nRepos})

        let localPath = ""
        let doGit     = false

        let fetchErr = ""
        if deepMode == true {
            let cloneResp, cerr = bridgeCall(bridge, "clone_blobless", [repo["clone_url"], token, repoTmp])
            if cerr != null {
                fetchErr = "clone_blobless bridge error for " + repoName + ": " + cerr.message
            } else if cloneResp["error"] != null {
                fetchErr = "clone failed for " + repoName + ": " + str(cloneResp["error"])
            } else {
                localPath = cloneResp["path"]
                doGit     = true
            }
        } else {
            let tarResp, terr = bridgeCall(bridge, "fetch_tarball", [repoName, token, repoTmp])
            if terr != null {
                fetchErr = "fetch_tarball bridge error for " + repoName + ": " + terr.message
            } else if tarResp["error"] != null {
                fetchErr = "tarball failed for " + repoName + ": " + str(tarResp["error"])
            } else {
                localPath = tarResp["path"]
            }
        }

        if len(fetchErr) > 0 {
            send(progressCh, {"phase": "org_repo_error", "repo": repoName, "error": fetchErr})
        }

        if len(localPath) > 0 {
            let repoResult = runScanWithProgress(localPath, doGit, maxSizeMB, progressCh, null, false)

            // Tag every finding with its source repo and prefix the file path
            let rFindings = repoResult["findings"]
            let nrf = len(rFindings)
            let fi  = 0
            while fi < nrf {
                rFindings[fi]["repo"] = repoName
                rFindings[fi]["file"] = repoName + "/" + rFindings[fi]["file"]
                fi = fi + 1
            }

            allFindings  = _mergeArrays(allFindings, rFindings)
            totalFiles   = totalFiles   + repoResult["fileCount"]
            totalCommits = totalCommits + repoResult["commitCount"]
            scanned      = scanned + 1
        }

        // Remove temp dir immediately after each repo is done
        bridgeCall(bridge, "cleanup", [repoTmp])
        send(progressCh, {"phase": "org_repos", "total": nRepos, "done": scanned, "repo": repoName})

        i = i + 1
    }

    bridgeClose(bridge)

    let totalSec = float(_timeNanos() - totalStart) / 1000000000.0

    return {
        "findings":      sortFindings(allFindings),
        "fileCount":     totalFiles,
        "commitCount":   totalCommits,
        "repoCount":     nRepos,
        "totalSec":      totalSec,
        "filesSec":      0.0,
        "gitSec":        0.0,
        "yaraSec":       0.0,
        "filesPerSec":   0.0,
        "commitsPerSec": 0.0,
        "error":         null,
    }
}
