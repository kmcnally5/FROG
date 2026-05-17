# SecretHunter

A local security audit tool, written in kLex, that scans a codebase and its full git history for accidentally committed secrets — API keys, tokens, private keys, database URLs, passwords. Ships with two front-ends (a CLI and a native OpenGL GUI), optional YARA-rule matching, optional Shannon-entropy detection, and an optional GitHub-org-wide online scan mode.

**All local operations are read-only.** No files are modified and no git mutations occur. The only git commands invoked locally are `git rev-list` and `git show`, both strictly read-only. The GitHub-org mode does make HTTPS calls to `api.github.com` (using a personal access token you provide) — that's the only mode that touches the network.

---

## Files in this folder

| File | What it is |
|---|---|
| `secretHunter.lex` | CLI front-end. Run on a single directory; prints coloured or JSON output. |
| `secretHunterUI.lex` | Native OpenGL GUI front-end. Interactive scan, live progress, filtering, settings panel. |
| `secretHunterLib.lex` | Scan engine — pattern library, parallel scanner, git enumeration, YARA glue, entropy glue, GitHub-org orchestrator. Imported by both front-ends. |
| `secretHunterTest.lex` | Fixture test that plants known-fake secrets and asserts every expected pattern is detected. |
| `yara_bridge.py` | Native bridge: YARA rule matching + Shannon-entropy scanning (both Python sidecars). |
| `secrets.yar` | YARA rule file consumed by `yara_bridge.py`. Catches binary secrets and patterns regex can't easily express. |
| `yaraTest.lex` | Standalone YARA-bridge smoke test. |
| `github_bridge.py` | Native bridge: enumerates a GitHub org or user, fetches each repo (tarball or blobless clone). |

---

## Quick Start

### CLI

```bash
# Scan the current directory (files + git history)
./klex tests/examples/SecretHunter/secretHunter.lex .

# Scan a different repo
./klex tests/examples/SecretHunter/secretHunter.lex ~/Documents/Development/MyProject

# Files only — skip git history (faster)
./klex tests/examples/SecretHunter/secretHunter.lex . --no-git

# Emit JSON for piping to other tools
./klex tests/examples/SecretHunter/secretHunter.lex . --json > findings.json

# Override max file size (default 5 MB)
./klex tests/examples/SecretHunter/secretHunter.lex . --max-size=20
```

### GUI

```bash
./klex tests/examples/SecretHunter/secretHunterUI.lex
```

Opens an OpenGL window. Type a path in the SCAN PATH field, toggle the checkboxes for the scan layers you want (git history, YARA, entropy), and press SCAN. Findings stream into the tree view in real time; severity counters tick up. Click a severity tile in the sidebar to filter the tree by that level.

To scan an entire GitHub organisation, type the org URL in the path field (e.g. `https://github.com/octocat`). The first time, open the **CFG** dialog and paste a GitHub personal access token.

---

## The five scan layers

A single scan can run any combination of these. The CLI exposes the first two; the GUI exposes all five via checkboxes and path heuristics.

| Layer | What it catches | Front-end controls |
|---|---|---|
| **1. File regex** | Working-tree text files matched against the `PATTERNS` array (currently 25 patterns). Always on. | Always runs |
| **2. Git history regex** | Every commit from every discovered git repository, diffs scanned with the same `PATTERNS` array. | CLI: default on (`--no-git` to skip). GUI: "Include git history" checkbox. |
| **3. YARA rules** | Binary secrets, multi-string correlations, format-specific patterns. Rules live in `secrets.yar`. Uses `yara-python` via a native bridge. | GUI: "Use YARA rules" checkbox (CLI does not expose YARA). |
| **4. Entropy detection** | Shannon-entropy heuristic: long strings (≥20 chars quoted, ≥36 bare) with high randomness (≥4.5 bits/char) get flagged. Catches novel credential shapes regex doesn't know about. | GUI: "Entropy detection" checkbox. |
| **5. GitHub org scan** | Enumerates every repo in a GitHub org or user, fetches each one (tarball for online mode, blobless clone for deep mode), runs layers 1+2 on the local copy, deletes the temp dir after each repo. | GUI: triggered automatically when the SCAN PATH is a `https://github.com/...` URL. |

All five layers run in parallel where possible — YARA and entropy each fan out across CPU cores via independent Python subprocesses.

---

## CLI flags

| Flag             | Default | Meaning                                              |
| ---------------- | ------- | ---------------------------------------------------- |
| `--no-git`       | off     | Skip git history scan, scan working tree only        |
| `--json`         | off     | Emit a single JSON array instead of coloured text    |
| `--max-size=N`   | `5`     | Maximum file size in MB to scan (skip larger files)  |

The CLI does not currently expose YARA, entropy, or GitHub-org scanning — those are GUI-only. Use the GUI for those layers, or call `sh.runScanWithProgress()` / `sh.runOrgScan()` from your own kLex script.

---

## GUI features

- **Live scan progress** — separate progress bars per phase (file enumeration, file scan, git enumeration, git scan, YARA, entropy). Animated scan-line clipped to the main area while a phase is active.
- **Severity tiles** — CRIT / HIGH / MED / LOW counters in the sidebar are clickable filters. Click a tile to filter the findings tree to that severity; click again to clear. An ease-out tween animates the counters up from zero on scan complete.
- **Threat-level badge** — header strip pulses red when CRITICAL findings exist.
- **Filter dropdowns** — filter by severity (`ALL / CRITICAL / HIGH / MEDIUM / LOW`) and source (`ALL / FILES / GIT / YARA / ENTROPY`).
- **Findings tree** — grouped by file, expandable per file, with a [source] tag on each row. Double-click a row to open the file in your default editor.
- **Remediation panel** — selecting a finding reveals an inline panel below the tree with copy-ready snippets: env-var template for rotating the credential and a `git filter-repo` command for purging from history.
- **CFG settings dialog** — bridge script paths (Python interpreter, github_bridge.py, temp dir), GitHub token, and a "Deep scan" toggle for full-history clones vs. tarball downloads.
- **Themes** — five built-in colour themes; crimson is the default (configurable via `stdlib/ui_themes.lex`).

---

## Reading the output

```
[CRITICAL] AWS Access Key ID
  Source: src/config.go:23
  Match:  AKIAIOSFODNN7EXAMPLE
  Action: Rotate this key immediately in the AWS console.

[CRITICAL] GitHub Classic PAT  (in git history)
  Commit: a1b2c3d (The FrogMan, 2024-08-12)
  File:   scripts/deploy.sh
  Match:  ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
  Action: Revoke immediately on GitHub. Purge from history with git filter-repo.

─────────────────────────────────────────────
Scanned: 1,247 files, 312 commits
Findings: 3 CRITICAL  |  5 HIGH  |  2 MEDIUM  |  11 LOW
─────────────────────────────────────────────
```

Matches longer than 40 characters are truncated in the middle (`prefix…suffix`) so secrets are not fully echoed into terminal scrollback or log files.

---

## Severity Guide

| Severity     | Meaning                                                                  | Action                          |
| ------------ | ------------------------------------------------------------------------ | ------------------------------- |
| **CRITICAL** | Real credentials with confirmed shape (AWS keys, GitHub tokens, private keys, payment keys). | Rotate **immediately**.         |
| **HIGH**     | Exposed but less catastrophic (Slack tokens, DB URLs, webhooks).         | Rotate at the next opportunity. |
| **MEDIUM**   | JSON Web Tokens — may be sample/test tokens. Review claims before acting.| Inspect, then decide.           |
| **LOW**      | Heuristic password / API key patterns. Often false positives in docs.    | Glance through, ignore obvious noise. |

YARA rules carry their own severity in the rule metadata (`meta: severity = "..."`) — entropy findings score `HIGH` for ≥5.0 bits/char and `MEDIUM` otherwise.

---

## What To Do When You Find a Real Secret

1. **Stay calm.** Most matches are fakes, test fixtures, or sample values.
2. **Verify** by opening the file at the reported line.
3. **If real:**
   a. **Rotate the credential FIRST** (so the leaked value is invalid).
   b. **Purge from git history** using `git filter-repo` (https://github.com/newren/git-filter-repo).
   c. **Force-push** to all remotes that had the leak.
   d. **Notify** anyone who may have cloned the repo before the purge.
4. **Re-scan** to confirm the secret no longer appears in files or history.

The GUI's remediation panel pre-fills steps 3a and 3b for the selected finding — copy the snippets, paste them into your shell.

---

## Patterns detected (currently 25)

### Cloud Provider Keys — CRITICAL
- AWS Access Key ID
- AWS Session Token
- Google API Key
- Google OAuth Client ID

### Source Control Tokens — CRITICAL
- GitHub Classic PAT
- GitHub Fine-grained PAT
- GitHub OAuth Token
- GitHub App Token (`ghu_` / `ghs_`)
- GitLab Personal Access Token

### Private Keys — CRITICAL
- RSA / OpenSSH / EC / DSA / PGP / encrypted / generic

### Payment — CRITICAL
- Stripe Live Secret Key
- Stripe Restricted Key
- Square Access Token

### Comms & CI — HIGH
- Slack Token
- Slack Webhook URL
- Discord Webhook URL
- SendGrid API Key
- Mailgun API Key
- Twilio Account SID

### Databases — HIGH
- Database URL with embedded credentials (postgres / mysql / mongodb / redis)

### Tokens — MEDIUM
- JSON Web Token (JWT)

### Heuristics — LOW
- Generic API Key (`api_key = "..."`, `access_token: ...`)
- Hard-coded Password (`password = "..."`)

**Additional patterns via YARA:** when the YARA layer is enabled, `secrets.yar` adds further rules — including binary patterns and multi-string correlations that regex can't easily express. Each rule carries its own severity in metadata.

---

## `.secretignore`

To suppress known false positives (sample fixtures, public dummy keys in test files, etc.), drop a `.secretignore` file at the root of the scanned directory. Each line is a rule that suppresses matching findings:

```
# Match all findings from a specific file:
docs/examples/sample_keys.md

# Match all findings of a given pattern name (case-insensitive substring on patternName):
JWT

# Match a specific pattern in a specific file:
docs/examples/sample_keys.md:JWT

# Glob-style file matches:
**/test/fixtures/*
src/**/*.test.go
```

Lines beginning with `#` are comments. Suppressed findings are counted in a separate `suppressed: N` line in the GUI sidebar so you can see at a glance how much noise is being filtered.

---

## How it works

SecretHunter is **embarrassingly parallel**. A full scan runs up to five phases, each of which fans out across CPU cores:

1. **File enumeration** — recursively walks the working tree once, recording (a) eligible scan files and (b) any directory containing a `.git` subdirectory. Repo discovery is depth-unaware: a scan of `~/Documents` automatically discovers every git repo nested anywhere under it.

2. **File regex phase** — every eligible file is scanned in parallel via `async`/`await`. Files larger than 500 lines are themselves split into sub-chunks and scanned across multiple cores concurrently. Each line is first checked against a cheap "needle" pre-filter (e.g. `AKIA`, `ghp_`, `BEGIN`) before the regex runs — lines that can't possibly match any pattern are skipped in microseconds.

3. **Git regex phase** *(optional)* — every commit from every discovered repository is collected via `git rev-list --all`, then flattened into a single work queue. The parallel worker pool distributes commits across CPU cores simultaneously — not one-repo-at-a-time. Each commit's diff is further split by file block and scanned in parallel.

4. **YARA phase** *(optional)* — file list is sharded across N parallel YARA bridges. Each bridge is a separate Python subprocess loading `secrets.yar` independently, so YARA itself runs lock-free across cores. One `scan_batch` call per worker eliminates per-file round-trip overhead.

5. **Entropy phase** *(optional)* — same parallel sharding as YARA but using the `entropy_scan` handler in `yara_bridge.py`. Skips known-noise extensions (`.json`, `.lock`, `.toml`, `.yml`, etc.) and obvious non-secret shapes (pure hex, UUIDs).

For **GitHub org scans**, an outer loop drives this five-phase pipeline once per repository:

- The org bridge calls `list_repos` to enumerate every repo in the org/user.
- For each repo: `fetch_tarball` (online mode, single HTTPS call, current files only) or `clone_blobless` (deep mode, git fetch with `--filter=blob:none`, full history scannable).
- The local copy is fed through the five phases above.
- The temp directory is deleted immediately after the repo is scanned — nothing is kept on disk between repos.

### Skipped Paths

The following directory names are skipped automatically (both in file scans and in git-diff scans):

```
.git  node_modules  vendor  .venv  __pycache__  target
dist  build  .next  .cache  .idea  .vscode
```

### Skipped Extensions

```
.png .jpg .jpeg .gif .ico .webp .pdf .zip .tar .gz .xz .bz2 .7z .rar
.dmg .iso .exe .dll .so .dylib .o .a .class .jar .wasm .bin
.mp3 .mp4 .mov .avi .wav .ttf .otf .woff .woff2 .sqlite .db .db3
```

### What writes? Almost nothing.

The CLI and the local-folder GUI scan write **zero files** and make **zero network calls** — you can run them with the network disconnected and they behave identically.

The GitHub-org scan is the one exception: it makes HTTPS requests to `api.github.com` for repo enumeration and to `codeload.github.com` for tarball downloads. Authentication is via a token you provide in the CFG dialog or the `GITHUB_TOKEN` env var. The token is stored locally in `~/.secrethunter/config` (mode 0600 — readable only by your user) and is never transmitted anywhere except GitHub itself. Temp directories live under `/tmp/secrethunter_*` and are removed after each repo.

---

## Performance

Measured on a 10-core Mac Mini (Apple M-series) using GNU `gtime`. All runs used the default 5 MB file size limit. **Layers 1 and 2 only** (file regex + git history regex).

| Workload | Files | Commits | Wall | User | Sys | CPU% | Peak RSS | Throughput |
| -------- | ----- | ------- | ---- | ---- | --- | ---- | -------- | ---------- |
| kLex repo — files only | 328 | — | **0.81 s** | 5.67 s | 0.52 s | 763% | 95 MB | — |
| kLex repo — files + full git history | 328 | 27 | **1.61 s** | 8.95 s | 1.19 s | 626% | 83 MB | — |
| ~/Documents/Development (22 GB, files only) | 5,501 | — | **8.32 s** | 60.28 s | 5.62 s | 791% | 134 MB | 2.6 GB/s |
| ~/Documents/ (71 GB, files only) | 12,811 | — | **17.21 s** | 124.55 s | 11.61 s | 791% | 139 MB | 4.1 GB/s |
| ~/Documents/ (71 GB, **files + git history**, 3 repos auto-discovered) | 12,811 | 29 | **18.42 s** | 130.27 s | 12.65 s | 775% | 146 MB | 3.9 GB/s |

The 71 GB full-documents run with git history found **48 real findings** (2 CRITICAL, 21 HIGH, 25 LOW) across 12,811 scanned files and 29 commits — in under 19 seconds with no configuration, no network access, and no indexing infrastructure.

**With YARA enabled,** expect roughly 1.5×–2× the wall-clock time on the same workload — YARA rule evaluation is heavier per file than regex pre-filtering. With **entropy detection enabled** the cost is similar to YARA.

**With GitHub org scan,** wall-clock is dominated by HTTPS download time — typically 50–500ms per repo for tarball mode (depending on repo size) and 2–10 seconds for deep blobless clones. The local scan after each fetch follows the same numbers as above.

**CPU scaling:** SecretHunter consistently saturates 7–8 cores regardless of workload size. The async worker pool scales to the available core count automatically — it will use whatever you give it.

**`gtime` field definitions:** `Wall` = elapsed clock time; `User` = total CPU time in user space summed across all cores; `Sys` = kernel time; `CPU%` = average core utilisation ((user + sys) / wall); `Peak RSS` = maximum resident set size (heap high-water mark).

---

## Adding patterns

### Regex pattern

Edit `secretHunterLib.lex` and append to the `PATTERNS` array:

```lex
{ "name": "Heroku API Key",
  "needle": "HRKU-",
  "regex": `HRKU-[A-Za-z0-9]{32}`,
  "severity": "CRITICAL",
  "action": "Rotate the key in the Heroku dashboard." },
```

The `needle` must be a distinctive case-sensitive substring that appears anywhere a real match would. Leave it as `""` for heuristic patterns that have no distinctive prefix — they will always run the regex (slower).

### YARA rule

Edit `secrets.yar` and add a rule using standard YARA syntax. Severity, action, and tags are read from the rule's `meta:` block:

```yara
rule MyCustomKey {
    meta:
        severity = "CRITICAL"
        action = "Rotate this key in <vendor> dashboard."
    strings:
        $a = /MYK-[A-Za-z0-9]{32}/
    condition:
        $a
}
```

Re-run the fixture tests after adding patterns (regex or YARA):

```bash
./klex tests/examples/SecretHunter/secretHunterTest.lex   # regex fixtures
./klex tests/examples/SecretHunter/yaraTest.lex           # YARA smoke test
```

---

## Tests

The end-to-end fixture test lives at `tests/unit/secretHunterTest.lex`. It plants known-fake secret strings in a temp directory, runs the scanner, and asserts every expected pattern is detected with the correct severity:

```bash
./klex tests/unit/secretHunterTest.lex
```

Should print `13/13 assertions passed`.

The YARA bridge has its own standalone smoke test:

```bash
./klex tests/examples/SecretHunter/yaraTest.lex
```

This requires `yara-python` to be installed on the host (`pip install yara-python`).

---

## Limitations

- **Regex-based by default.** SecretHunter's primary detection is deterministic-shape regex. Enable YARA + entropy in the GUI for broader coverage. Custom or non-standard credential formats still won't be caught unless you add a pattern or YARA rule for them.
- **No verification.** SecretHunter does **not** test whether a found key is currently active. It only finds the *shape*. (This is intentional — verification would require network calls to dozens of vendor APIs, which would be a security and reliability liability of its own.)
- **GitHub org mode requires `PyGitHub`.** The `github_bridge.py` sidecar uses the `PyGitHub` library. Install with `pip install PyGitHub` before using the org-scan mode.
- **YARA mode requires `yara-python`.** Install with `pip install yara-python` before enabling the YARA checkbox.
