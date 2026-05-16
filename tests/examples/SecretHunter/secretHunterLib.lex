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

fn runScanWithProgress(root, doGit, maxSizeMB, progressCh) {
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

    totalEnd = _timeNanos()

    totalSec = float(totalEnd - totalStart) / 1000000000.0
    filesSec = float(filesEnd - filesStart) / 1000000000.0
    gitSec   = float(gitEnd   - gitStart)   / 1000000000.0

    filesPerSec   = 0.0
    commitsPerSec = 0.0
    if filesSec > 0.001 { filesPerSec   = float(fileCount)   / filesSec }
    if gitSec   > 0.001 { commitsPerSec = float(commitCount) / gitSec   }

    nf  = len(fileFindings)
    ng  = len(gitFindings)
    all = makeArray(nf + ng)
    i = 0
    while i < nf { all[i] = fileFindings[i]        i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]    i = i + 1 }

    return {
        "findings":      sortFindings(all),
        "fileCount":     fileCount,
        "commitCount":   commitCount,
        "repoCount":     repoCount,
        "totalSec":      totalSec,
        "filesSec":      filesSec,
        "gitSec":        gitSec,
        "filesPerSec":   filesPerSec,
        "commitsPerSec": commitsPerSec,
    }
}

// ─────────────────────────────────────────────────────────────────
// MAIN SCAN ENTRY POINT (CLI — no progress reporting)
// ─────────────────────────────────────────────────────────────────

fn runScan(root, doGit, maxSizeMB) {
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

    nf  = len(fileFindings)
    ng  = len(gitFindings)
    all = makeArray(nf + ng)
    i = 0
    while i < nf { all[i] = fileFindings[i]        i = i + 1 }
    i = 0
    while i < ng { all[nf + i] = gitFindings[i]    i = i + 1 }

    return {
        "findings":    sortFindings(all),
        "fileCount":   fileCount,
        "commitCount": commitCount,
        "repoCount":   repoCount,
    }
}
