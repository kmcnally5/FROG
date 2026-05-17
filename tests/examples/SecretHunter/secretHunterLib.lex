// secretHunterLib.lex — scan engine (import this; do not run directly)
//
// Provides all pattern matching, file enumeration, parallel scanning,
// and result sorting. Entry point: runScan(root, doGit, maxSizeMB).

import "stdlib/path.lex" as p

// ─────────────────────────────────────────────────────────────────
// PATTERN LIBRARY
// ─────────────────────────────────────────────────────────────────

PATTERNS = [
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
    igPath = root + "/.secretignore"
    if _fsExists(igPath) == false { return makeArray(0) }
    content, err = _fsRead(igPath)
    if err != null { return makeArray(0) }
    lines = split(content, "\n")
    n = len(lines)
    count = 0
    i = 0
    while i < n {
        line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" { count = count + 1 }
        i = i + 1
    }
    rules = makeArray(count, "")
    idx = 0
    i = 0
    while i < n {
        line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" {
            rules[idx] = line
            idx = idx + 1
        }
        i = i + 1
    }
    return rules
}

fn appendIgnoreRule(root, rule) {
    igPath = root + "/.secretignore"
    if _fsExists(igPath) == false {
        header = "# .secretignore — SecretHunter allowlist\n"
        header = header + "# Lines: path, PatternName, or path:PatternName\n\n"
        _, err = _fsWrite(igPath, header + rule + "\n")
        return err
    }
    _, err = _fsAppend(igPath, rule + "\n")
    return err
}

fn _pathMatches(pattern, filePath) {
    if pattern == filePath { return true }
    if startsWith(pattern, "**/") {
        suffix = substr(pattern, 3)
        if endsWith(filePath, "/" + suffix) { return true }
        if indexOf(filePath, "/" + suffix + "/") >= 0 { return true }
        return filePath == suffix
    }
    if endsWith(pattern, "/**") {
        prefix = substr(pattern, 0, len(pattern) - 3)
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
    colonIdx = indexOf(rule, ":")
    if colonIdx >= 0 {
        filePart = trim(substr(rule, 0, colonIdx))
        patPart  = trim(substr(rule, colonIdx + 1))
        return _pathMatches(filePart, filePath) && _patternMatches(patPart, patternName)
    }
    if indexOf(rule, "/") < 0 && indexOf(rule, "*") < 0 {
        return _patternMatches(rule, patternName)
    }
    return _pathMatches(rule, filePath)
}

fn filterFindings(findings, rules) {
    n      = len(findings)
    nRules = len(rules)
    if nRules == 0 { return findings, 0 }

    kept = 0
    i = 0
    while i < n {
        f = findings[i]
        suppressed = false
        r = 0
        while r < nRules && suppressed == false {
            if ruleMatchesFinding(rules[r], f["file"], f["patternName"]) {
                suppressed = true
            }
            r = r + 1
        }
        if suppressed == false { kept = kept + 1 }
        i = i + 1
    }

    suppressedCount = n - kept
    if suppressedCount == 0 { return findings, 0 }

    out = makeArray(kept)
    idx = 0
    i = 0
    while i < n {
        f = findings[i]
        suppressed = false
        r = 0
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
    pat = finding["patternName"]
    parenIdx = indexOf(pat, " (")
    if parenIdx >= 0 { pat = substr(pat, 0, parenIdx) }
    pat = trim(pat)
    if ruleType == "pattern"      { return pat }
    if ruleType == "file"         { return finding["file"] }
    return finding["file"] + ":" + pat
}

// ─────────────────────────────────────────────────────────────────
// SKIP LISTS
// ─────────────────────────────────────────────────────────────────

SKIP_DIRS = {
    ".git": true, "node_modules": true, "vendor": true, ".venv": true,
    "__pycache__": true, "target": true, "dist": true, "build": true,
    ".next": true, ".cache": true, ".idea": true, ".vscode": true,
}

SKIP_EXTS = {
    ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".webp": true,
    ".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".xz": true, ".bz2": true,
    ".7z": true, ".rar": true, ".dmg": true, ".iso": true,
    ".exe": true, ".dll": true, ".so": true, ".dylib": true, ".o": true, ".a": true,
    ".class": true, ".jar": true, ".wasm": true, ".bin": true,
    ".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".wav": true,
    ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
    ".sqlite": true, ".db": true, ".db3": true,
}

SEV_RANK = { "CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3 }

SUB_LINE_CHUNK  = 500
WORKER_MULTIPLIER = 4

// ─────────────────────────────────────────────────────────────────
// DISPLAY HELPERS
// ─────────────────────────────────────────────────────────────────

fn truncateMatch(s) {
    n = len(s)
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
    n = len(name)
    i = n - 1
    while i >= 0 {
        c = name[i]
        if c == "." { return lower(substr(name, i)) }
        if c == "/" { return "" }
        i = i - 1
    }
    return ""
}

fn shouldKeep(entry, maxBytes) {
    if entry["isSymlink"] == true { return false }
    if entry["isDir"] == true { return false }
    ext = lowerExt(entry["name"])
    if SKIP_EXTS[ext] == true { return false }
    if entry["size"] > maxBytes { return false }
    return true
}

fn countFiles(root, maxBytes) {
    entries, err = _fsReadDir(root)
    if err != null { return 0 }
    total = 0
    i = 0
    n = len(entries)
    while i < n {
        e = entries[i]
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
    entries, err = _fsReadDir(root)
    if err != null { return idx }
    i = 0
    n = len(entries)
    while i < n {
        e = entries[i]
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
    total = countFiles(root, maxBytes)
    files = makeArray(total)
    fillFiles(root, files, 0, maxBytes)
    return files
}

// ─────────────────────────────────────────────────────────────────
// PER-FILE SCAN
// ─────────────────────────────────────────────────────────────────

fn shouldSkipPath(path) {
    parts = split(path, "/")
    i = 0
    n = len(parts)
    while i < n {
        if SKIP_DIRS[parts[i]] == true { return true }
        i = i + 1
    }
    return false
}

fn scanFileLineRange(lines_arr, filePath, startLine, endLine) {
    npats = len(PATTERNS)
    total = 0
    li = startLine
    while li < endLine {
        line = lines_arr[li]
        if len(line) > 0 {
            pi = 0
            while pi < npats {
                pat = PATTERNS[pi]
                needle = pat["needle"]
                mightMatch = false
                if needle == "" {
                    mightMatch = true
                } else {
                    if indexOf(line, needle) >= 0 { mightMatch = true }
                }
                if mightMatch == true {
                    matches, rerr = _regexFindAll(pat["regex"], line)
                    if rerr == null { total = total + len(matches) }
                }
                pi = pi + 1
            }
        }
        li = li + 1
    }

    out = makeArray(total)
    if total == 0 { return out }

    idx = 0
    li = startLine
    while li < endLine {
        line = lines_arr[li]
        if len(line) > 0 {
            pi = 0
            while pi < npats {
                pat = PATTERNS[pi]
                needle = pat["needle"]
                mightMatch = false
                if needle == "" {
                    mightMatch = true
                } else {
                    if indexOf(line, needle) >= 0 { mightMatch = true }
                }
                if mightMatch == true {
                    matches, rerr = _regexFindAll(pat["regex"], line)
                    if rerr == null {
                        mi = 0
                        nm = len(matches)
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
    content, ferr = _fsRead(filePath)
    if ferr != null { return makeArray(0) }

    lines_arr = split(content, "\n")
    nlines = len(lines_arr)

    if nlines <= SUB_LINE_CHUNK {
        return scanFileLineRange(lines_arr, filePath, 0, nlines)
    }

    numChunks = (nlines + SUB_LINE_CHUNK - 1) / SUB_LINE_CHUNK
    tasks = makeArray(numChunks, null)
    i = 0
    while i < numChunks {
        let s = i * SUB_LINE_CHUNK
        let e = s + SUB_LINE_CHUNK
        if e > nlines { e = nlines }
        let path = filePath
        let lns  = lines_arr
        tasks[i] = async(fn() { return scanFileLineRange(lns, path, s, e) })
        i = i + 1
    }

    chunkResults = makeArray(numChunks, null)
    i = 0
    while i < numChunks {
        chunkResults[i] = await(tasks[i])
        i = i + 1
    }

    total = 0
    i = 0
    while i < numChunks { total = total + len(chunkResults[i])   i = i + 1 }

    out = makeArray(total)
    idx = 0
    i = 0
    while i < numChunks {
        sub = chunkResults[i]
        m = len(sub)
        j = 0
        while j < m { out[idx] = sub[j]   idx = idx + 1   j = j + 1 }
        i = i + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// PER-COMMIT GIT SCAN
// ─────────────────────────────────────────────────────────────────

fn scanCommitFileBlock(lines_arr, startLine, endLine, currentFile, hash, author, date) {
    npats = len(PATTERNS)
    total = 0
    li = startLine
    while li < endLine {
        line = lines_arr[li]
        n = len(line)
        if n > 0 {
            if line[0] == "+" {
                isHeader = false
                if n >= 3 { if substr(line, 0, 3) == "+++" { isHeader = true } }
                if isHeader == false {
                    body = substr(line, 1)
                    pi = 0
                    while pi < npats {
                        pat = PATTERNS[pi]
                        needle = pat["needle"]
                        mightMatch = false
                        if needle == "" {
                            mightMatch = true
                        } else {
                            if indexOf(body, needle) >= 0 { mightMatch = true }
                        }
                        if mightMatch == true {
                            matches, rerr = _regexFindAll(pat["regex"], body)
                            if rerr == null { total = total + len(matches) }
                        }
                        pi = pi + 1
                    }
                }
            }
        }
        li = li + 1
    }

    out = makeArray(total)
    if total == 0 { return out }

    idx = 0
    li = startLine
    while li < endLine {
        line = lines_arr[li]
        n = len(line)
        if n > 0 {
            if line[0] == "+" {
                isHeader = false
                if n >= 3 { if substr(line, 0, 3) == "+++" { isHeader = true } }
                if isHeader == false {
                    body = substr(line, 1)
                    pi = 0
                    while pi < npats {
                        pat = PATTERNS[pi]
                        needle = pat["needle"]
                        mightMatch = false
                        if needle == "" {
                            mightMatch = true
                        } else {
                            if indexOf(body, needle) >= 0 { mightMatch = true }
                        }
                        if mightMatch == true {
                            matches, rerr = _regexFindAll(pat["regex"], body)
                            if rerr == null {
                                mi = 0
                                nm = len(matches)
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
    stdout, stderr, code, gerr = _processExec("git", [
        "-C", repoPath, "show", "--no-color", "--unified=0",
        "--pretty=format:%H%n%an%n%ae%n%ad", hash,
    ])
    if gerr != null { return makeArray(0) }
    if code != 0   { return makeArray(0) }

    lines_arr = split(stdout, "\n")
    nlines = len(lines_arr)
    if nlines < 4 { return makeArray(0) }

    author = lines_arr[1]
    date   = lines_arr[3]

    headerCount = 0
    li = 4
    while li < nlines {
        line = lines_arr[li]
        if len(line) >= 12 {
            if startsWith(line, "diff --git ") == true { headerCount = headerCount + 1 }
        }
        li = li + 1
    }
    if headerCount == 0 { return makeArray(0) }

    blocks = makeArray(headerCount)
    bi = 0
    li = 4
    while li < nlines {
        line = lines_arr[li]
        if len(line) >= 12 {
            if startsWith(line, "diff --git ") == true {
                if bi > 0 { blocks[bi - 1]["endLine"] = li }
                bIdx = indexOf(line, " b/")
                filePath = ""
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

    activeCount = 0
    i = 0
    while i < headerCount {
        if blocks[i]["skip"] == false { activeCount = activeCount + 1 }
        i = i + 1
    }
    if activeCount == 0 { return makeArray(0) }

    tasks = makeArray(activeCount, null)
    ti = 0
    i = 0
    while i < headerCount {
        b = blocks[i]
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

    perBlock = makeArray(activeCount, null)
    i = 0
    while i < activeCount { perBlock[i] = await(tasks[i])   i = i + 1 }

    total = 0
    i = 0
    while i < activeCount { total = total + len(perBlock[i])   i = i + 1 }

    out = makeArray(total)
    idx = 0
    i = 0
    while i < activeCount {
        sub = perBlock[i]
        m = len(sub)
        j = 0
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
    entries, err = _fsReadDir(root)
    if err != null { return 0 }
    total  = 0
    hasGit = false
    n = len(entries)
    i = 0
    while i < n {
        e = entries[i]
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
    entries, err = _fsReadDir(root)
    if err != null { return idx }
    hasGit = false
    n = len(entries)
    i = 0
    while i < n {
        e = entries[i]
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
        e = entries[i]
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
    total = countGitRepos(root)
    if total == 0 { return makeArray(0) }
    repos = makeArray(total)
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
    nRepos = len(repos)
    if nRepos == 0 { return makeArray(0) }

    repoCommits = makeArray(nRepos, null)
    repoCounts  = makeArray(nRepos, 0)
    total = 0
    ri = 0
    while ri < nRepos {
        commits, ok = enumerateCommits(repos[ri])
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

    pairs = makeArray(total)
    idx = 0
    ri = 0
    while ri < nRepos {
        cmts = repoCommits[ri]
        cnt  = repoCounts[ri]
        ci = 0
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
    stdout, stderr, code, err = _processExec("git", [
        "-C", repoPath, "rev-list", "--all", "--no-merges",
    ])
    if err != null  { return makeArray(0), false }
    if code != 0    { return makeArray(0), false }

    n = len(stdout)
    if n == 0 { return makeArray(0), true }

    total = 0
    i = 0
    while i < n {
        if stdout[i] == "\n" { total = total + 1 }
        i = i + 1
    }
    if stdout[n - 1] != "\n" { total = total + 1 }

    commits = makeArray(total)
    idx = 0
    start = 0
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
    n = len(files)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanCommits(commits, repoPath) {
    n = len(commits)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn flattenWorkerResults(perWorker) {
    nw = len(perWorker)
    total = 0
    i = 0
    while i < nw {
        chunkArr = perWorker[i]
        m = len(chunkArr)
        j = 0
        while j < m { total = total + len(chunkArr[j])   j = j + 1 }
        i = i + 1
    }
    out = makeArray(total)
    idx = 0
    i = 0
    while i < nw {
        chunkArr = perWorker[i]
        m = len(chunkArr)
        j = 0
        while j < m {
            sub = chunkArr[j]
            k = 0
            ks = len(sub)
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
    sevR = SEV_RANK[f["severity"]]
    srcR = 0
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
    n = len(files)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanCommitsWithProgress(commits, repoPath, progressCh) {
    n = len(commits)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

// parScanAllCommits and parScanAllCommitsWithProgress work over a flat
// array of {commit, repo} pairs so a single worker pool covers every
// discovered repository — maximising parallelism across all repos.

fn parScanAllCommits(pairs) {
    n = len(pairs)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn parScanAllCommitsWithProgress(pairs, progressCh) {
    n = len(pairs)
    if n == 0 { return makeArray(0) }

    numWorkers = WORKER_MULTIPLIER * 10
    if numWorkers > n { numWorkers = n }

    base = n / numWorkers
    rem  = n % numWorkers
    tasks = makeArray(numWorkers, null)
    start = 0
    w = 0
    while w < numWorkers {
        size = base
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

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers { perWorker[w] = await(tasks[w])   w = w + 1 }
    return perWorker
}

fn runScanWithProgress(root, doGit, maxSizeMB, progressCh, yaraRulesFile = null, doEntropy = false) {
    maxBytes   = maxSizeMB * 1024 * 1024
    totalStart = _timeNanos()

    send(progressCh, {"phase": "enumerate", "total": 0, "done": 0})
    files     = enumerateFiles(root, maxBytes)
    fileCount = len(files)

    filesStart = _timeNanos()
    send(progressCh, {"phase": "files", "total": fileCount, "done": 0})
    perWorkerFiles = parScanFilesWithProgress(files, progressCh)
    fileFindings   = flattenWorkerResults(perWorkerFiles)
    filesEnd = _timeNanos()

    repoCount   = 0
    commitCount = 0
    gitFindings = makeArray(0)
    gitStart = filesEnd
    gitEnd   = filesEnd
    if doGit == true {
        gitStart = _timeNanos()
        send(progressCh, {"phase": "git_enumerate", "total": 0, "done": 0})
        repos     = findGitRepos(root)
        repoCount = len(repos)
        if repoCount > 0 {
            pairs       = gatherAllCommits(repos)
            commitCount = len(pairs)
            if commitCount > 0 {
                send(progressCh, {"phase": "git", "total": commitCount, "done": 0, "repoCount": repoCount})
                perWorkerCommits = parScanAllCommitsWithProgress(pairs, progressCh)
                gitFindings      = flattenWorkerResults(perWorkerCommits)
            }
        }
        gitEnd = _timeNanos()
    }

    yaraFindings = makeArray(0)
    yaraStart = _timeNanos()
    if yaraRulesFile != null {
        send(progressCh, {"phase": "yara", "total": fileCount, "done": 0})
        yaraFindings = scanYaraFilesParallel(yaraRulesFile, files)
        send(progressCh, {"phase": "yara_done", "total": fileCount, "done": fileCount})
    }
    yaraEnd = _timeNanos()

    entropyFindings = makeArray(0)
    entropyStart = _timeNanos()
    if doEntropy == true {
        send(progressCh, {"phase": "entropy", "total": fileCount, "done": 0})
        entropyFindings = scanEntropyFilesParallel(files)
        send(progressCh, {"phase": "entropy_done", "total": fileCount, "done": fileCount})
    }
    entropyEnd = _timeNanos()

    totalEnd = _timeNanos()

    totalSec = float(totalEnd - totalStart) / 1000000000.0
    filesSec = float(filesEnd - filesStart) / 1000000000.0
    gitSec   = float(gitEnd   - gitStart)   / 1000000000.0
    yaraSec  = float(yaraEnd  - yaraStart)  / 1000000000.0

    filesPerSec   = 0.0
    commitsPerSec = 0.0
    if filesSec > 0.001 { filesPerSec   = float(fileCount)   / filesSec }
    if gitSec   > 0.001 { commitsPerSec = float(commitCount) / gitSec   }

    nf  = len(fileFindings)
    ng  = len(gitFindings)
    ny  = len(yaraFindings)
    ne  = len(entropyFindings)
    all = makeArray(nf + ng + ny + ne)
    i = 0
    while i < nf { all[i] = fileFindings[i]                    i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]                i = i + 1 }
    i = 0
    while i < ny { all[nf + ng + i] = yaraFindings[i]          i = i + 1 }
    i = 0
    while i < ne { all[nf + ng + ny + i] = entropyFindings[i]  i = i + 1 }

    return {
        "findings":      sortFindings(all),
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

YARA_WORKERS = 16

// startYaraBridge — kept for standalone / CLI use (yaraTest.lex etc.)
fn startYaraBridge(rulesFile) {
    bridge, err = nativeBridge("python3", ["tests/examples/SecretHunter/yara_bridge.py"])
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
    batchArr = makeArray(batchCount, "")
    k = 0
    while k < batchCount { batchArr[k] = batch[k]  k = k + 1 }
    matches, err = bridgeCall(bridge, "scan_batch", [batchArr])
    if err != null { return accumCount }
    i = 0
    while i < len(matches) {
        hit = matches[i]
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
// rules, sends all its files in ONE scan_batch call, returns findings array.
fn scanYaraChunkBatch(rulesFile, files, startIdx, endIdx) {
    bridge, err = nativeBridge("python3", ["tests/examples/SecretHunter/yara_bridge.py"])
    if err != null { return makeArray(0) }

    _, err = bridgeCall(bridge, "load", [rulesFile])
    if err != null { bridgeClose(bridge)  return makeArray(0) }

    chunkSize = endIdx - startIdx
    chunk = makeArray(chunkSize, "")
    i = startIdx
    j = 0
    while i < endIdx { chunk[j] = files[i]  i = i + 1  j = j + 1 }

    matches, err = bridgeCall(bridge, "scan_batch", [chunk])
    bridgeClose(bridge)
    if err != null { return makeArray(0) }

    nm  = len(matches)
    out = makeArray(nm)
    i = 0
    while i < nm {
        hit = matches[i]
        out[i] = {
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
        i = i + 1
    }
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
fn scanYaraFilesParallel(rulesFile, files) {
    n = len(files)
    if n == 0 { return makeArray(0) }

    numWorkers = YARA_WORKERS
    if numWorkers > n { numWorkers = n }

    tasks = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        // Count and pre-allocate this worker's share.
        wCount = 0
        j = w
        while j < n { wCount = wCount + 1  j = j + numWorkers }

        chunk = makeArray(wCount, "")
        j = w
        k = 0
        while j < n { chunk[k] = files[j]  k = k + 1  j = j + numWorkers }

        let myRules = rulesFile
        let myChunk = chunk
        tasks[w] = async(fn() { return scanYaraChunkBatch(myRules, myChunk, 0, len(myChunk)) })
        w = w + 1
    }

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        result, err = safe(await, tasks[w])
        if err != null { perWorker[w] = makeArray(0) }
        else           { perWorker[w] = result }
        w = w + 1
    }

    total = 0
    w = 0
    while w < numWorkers { total = total + len(perWorker[w])  w = w + 1 }

    out = makeArray(total)
    idx = 0
    w = 0
    while w < numWorkers {
        sub = perWorker[w]
        m = len(sub)
        j = 0
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

ENTROPY_WORKERS = 16

fn scanEntropyChunk(files, startIdx, endIdx) {
    bridge, err = nativeBridge("python3", ["tests/examples/SecretHunter/yara_bridge.py"])
    if err != null { return makeArray(0) }

    chunkSize = endIdx - startIdx
    chunk = makeArray(chunkSize, "")
    i = startIdx
    j = 0
    while i < endIdx { chunk[j] = files[i]  i = i + 1  j = j + 1 }

    matches, err = bridgeCall(bridge, "entropy_scan", [chunk])
    bridgeClose(bridge)
    if err != null { return makeArray(0) }

    nm  = len(matches)
    out = makeArray(nm)
    i = 0
    while i < nm {
        hit = matches[i]
        ent = hit["entropy"]
        out[i] = {
            "source":      "entropy",
            "patternName": "High-Entropy String (" + str(ent) + " bits)",
            "severity":    hit["severity"],
            "action":      "Shannon entropy suggests a possible credential or key. Review and rotate if this is sensitive data.",
            "file":        hit["file"],
            "line":        0,
            "match":       hit["match"],
            "commit":      "",
            "author":      "",
            "date":        "",
        }
        i = i + 1
    }
    return out
}

fn scanEntropyFilesParallel(files) {
    n = len(files)
    if n == 0 { return makeArray(0) }

    numWorkers = ENTROPY_WORKERS
    if numWorkers > n { numWorkers = n }

    tasks = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        wCount = 0
        j = w
        while j < n { wCount = wCount + 1  j = j + numWorkers }

        chunk = makeArray(wCount, "")
        j = w
        k = 0
        while j < n { chunk[k] = files[j]  k = k + 1  j = j + numWorkers }

        let myChunk = chunk
        tasks[w] = async(fn() { return scanEntropyChunk(myChunk, 0, len(myChunk)) })
        w = w + 1
    }

    perWorker = makeArray(numWorkers, null)
    w = 0
    while w < numWorkers {
        result, err = safe(await, tasks[w])
        if err != null { perWorker[w] = makeArray(0) }
        else           { perWorker[w] = result }
        w = w + 1
    }

    total = 0
    w = 0
    while w < numWorkers { total = total + len(perWorker[w])  w = w + 1 }

    out = makeArray(total)
    idx = 0
    w = 0
    while w < numWorkers {
        sub = perWorker[w]
        m = len(sub)
        j = 0
        while j < m { out[idx] = sub[j]  idx = idx + 1  j = j + 1 }
        w = w + 1
    }
    return out
}

// ─────────────────────────────────────────────────────────────────
// MAIN SCAN ENTRY POINT (CLI — no progress reporting)
// ─────────────────────────────────────────────────────────────────

fn runScan(root, doGit, maxSizeMB, yaraRulesFile = null, doEntropy = false) {
    maxBytes = maxSizeMB * 1024 * 1024

    files     = enumerateFiles(root, maxBytes)
    fileCount = len(files)

    perWorkerFiles = parScanFiles(files)
    fileFindings   = flattenWorkerResults(perWorkerFiles)

    repoCount   = 0
    commitCount = 0
    gitFindings = makeArray(0)
    if doGit == true {
        repos     = findGitRepos(root)
        repoCount = len(repos)
        if repoCount > 0 {
            pairs       = gatherAllCommits(repos)
            commitCount = len(pairs)
            if commitCount > 0 {
                perWorkerCommits = parScanAllCommits(pairs)
                gitFindings      = flattenWorkerResults(perWorkerCommits)
            }
        }
    }

    yaraFindings = makeArray(0)
    if yaraRulesFile != null {
        yaraFindings = scanYaraFilesParallel(yaraRulesFile, files)
    }

    entropyFindings = makeArray(0)
    if doEntropy == true {
        entropyFindings = scanEntropyFilesParallel(files)
    }

    nf  = len(fileFindings)
    ng  = len(gitFindings)
    ny  = len(yaraFindings)
    ne  = len(entropyFindings)
    all = makeArray(nf + ng + ny + ne)
    i = 0
    while i < nf { all[i] = fileFindings[i]                    i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]                i = i + 1 }
    i = 0
    while i < ny { all[nf + ng + i] = yaraFindings[i]          i = i + 1 }
    i = 0
    while i < ne { all[nf + ng + ny + i] = entropyFindings[i]  i = i + 1 }

    return {
        "findings":    sortFindings(all),
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
    stdout, err = _processShell("echo $HOME 2>/dev/null")
    if err != null { return "/tmp" }
    h = trim(stdout)
    if len(h) == 0 { return "/tmp" }
    return h
}

fn _shEnvGet(varName) {
    stdout, err = _processShell("echo $" + varName)
    if err != null { return "" }
    return trim(stdout)
}

fn secretHunterConfigPath() {
    return _shHomeDir() + "/.secrethunter/config"
}

fn _parseKVConfig(content) {
    cfg = {}
    lines = split(content, "\n")
    i = 0
    n = len(lines)
    while i < n {
        line = trim(lines[i])
        if len(line) > 0 && line[0] != "#" {
            eqIdx = indexOf(line, "=")
            if eqIdx > 0 {
                cfg[trim(substr(line, 0, eqIdx))] = trim(substr(line, eqIdx + 1))
            }
        }
        i = i + 1
    }
    return cfg
}

fn loadSecretHunterConfig() {
    defaults = {
        "github_bridge_path": "tests/examples/SecretHunter/github_bridge.py",
        "python_executable":  "python3",
        "github_token":       "",
        "default_scan_mode":  "online",
        "temp_dir":           "/tmp",
    }
    cfgPath = secretHunterConfigPath()
    parsed  = {}
    if _fsExists(cfgPath) == true {
        content, err = _fsRead(cfgPath)
        if err == null { parsed = _parseKVConfig(content) }
    }
    // Env vars override config file
    envBridge = _shEnvGet("SECRETHUNTER_BRIDGE")
    envPython = _shEnvGet("SECRETHUNTER_PYTHON")
    envTmpDir = _shEnvGet("SECRETHUNTER_TMPDIR")
    envToken  = _shEnvGet("GITHUB_TOKEN")
    if len(envBridge) > 0 { parsed["github_bridge_path"] = envBridge }
    if len(envPython) > 0 { parsed["python_executable"]  = envPython }
    if len(envTmpDir) > 0 { parsed["temp_dir"]           = envTmpDir }
    if len(envToken)  > 0 { parsed["github_token"]        = envToken  }
    // Fill any still-missing keys from defaults
    dkeys = ["github_bridge_path", "python_executable", "github_token", "default_scan_mode", "temp_dir"]
    ki = 0
    while ki < len(dkeys) {
        k = dkeys[ki]
        if parsed[k] == null { parsed[k] = defaults[k] }
        ki = ki + 1
    }
    return parsed
}

fn saveSecretHunterConfig(cfg) {
    cfgPath = secretHunterConfigPath()
    cfgDir  = p.dirname(cfgPath)
    _, merr = _fsMkdirAll(cfgDir)
    if merr != null { return merr }
    content = "# SecretHunter configuration\n"
    content = content + "# Edit here or use the settings panel in the app.\n\n"
    content = content + "github_bridge_path=" + cfg["github_bridge_path"] + "\n"
    content = content + "python_executable="  + cfg["python_executable"]  + "\n"
    content = content + "github_token="       + cfg["github_token"]       + "\n"
    content = content + "default_scan_mode="  + cfg["default_scan_mode"]  + "\n"
    content = content + "temp_dir="           + cfg["temp_dir"]           + "\n"
    _, err = _fsWrite(cfgPath, content)
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
    prefix = "https://github.com/"
    rest   = substr(url, len(prefix))
    slashIdx = indexOf(rest, "/")
    if slashIdx >= 0 { rest = substr(rest, 0, slashIdx) }
    return trim(rest)
}

// ─────────────────────────────────────────────────────────────────
// ARRAY MERGE HELPER
// ─────────────────────────────────────────────────────────────────

fn _mergeArrays(a, b) {
    na = len(a)
    nb = len(b)
    if nb == 0 { return a }
    if na == 0 { return b }
    out = makeArray(na + nb)
    i = 0
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
    org        = extractOrgFromUrl(orgUrl)
    python     = cfg["python_executable"]
    if len(python) == 0 { python = "python3" }
    bridgePath = cfg["github_bridge_path"]
    if len(bridgePath) == 0 { bridgePath = "tests/examples/SecretHunter/github_bridge.py" }
    token      = cfg["github_token"]
    deepMode   = cfg["default_scan_mode"] == "deep"
    tmpBase    = cfg["temp_dir"]
    if len(tmpBase) == 0 { tmpBase = "/tmp" }

    totalStart = _timeNanos()

    // Start the bridge process
    bridge, berr = nativeBridge(python, [bridgePath])
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
    listResp, lerr = bridgeCall(bridge, "list_repos", [org, token, true])
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

    repos   = listResp["repos"]
    nRepos  = len(repos)
    send(progressCh, {"phase": "org_repos", "total": nRepos, "done": 0, "repo": ""})

    allFindings  = makeArray(0)
    totalFiles   = 0
    totalCommits = 0
    scanned      = 0

    i = 0
    while i < nRepos {
        repo      = repos[i]
        repoName  = repo["full_name"]
        repoSlug  = replace(repoName, "/", "_")
        repoTmp   = tmpBase + "/secrethunter_" + repoSlug

        send(progressCh, {"phase": "org_repo", "repo": repoName, "repoIdx": i, "total": nRepos})

        localPath = ""
        doGit     = false

        fetchErr = ""
        if deepMode == true {
            cloneResp, cerr = bridgeCall(bridge, "clone_blobless", [repo["clone_url"], token, repoTmp])
            if cerr != null {
                fetchErr = "clone_blobless bridge error for " + repoName + ": " + cerr.message
            } else if cloneResp["error"] != null {
                fetchErr = "clone failed for " + repoName + ": " + str(cloneResp["error"])
            } else {
                localPath = cloneResp["path"]
                doGit     = true
            }
        } else {
            tarResp, terr = bridgeCall(bridge, "fetch_tarball", [repoName, token, repoTmp])
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
            repoResult = runScanWithProgress(localPath, doGit, maxSizeMB, progressCh, null, false)

            // Tag every finding with its source repo and prefix the file path
            rFindings = repoResult["findings"]
            nrf = len(rFindings)
            fi  = 0
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

    totalSec = float(_timeNanos() - totalStart) / 1000000000.0

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
