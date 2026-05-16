# SecretHunter

A local security audit tool, written in kLex, that scans a codebase and its full git history for accidentally committed secrets — API keys, tokens, private keys, database URLs, passwords.

**All operations are read-only.** No files are modified, no network calls are made, no git mutations occur. The only git commands invoked are `git rev-list` and `git show`, both strictly read-only.

---

## Quick Start

From the kLex root directory:

```
./klex tests/examples/SecretHunter/secretHunter.lex [path] [flags]
```

### Examples

```
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

---

## Flags

| Flag             | Default | Meaning                                              |
| ---------------- | ------- | ---------------------------------------------------- |
| `--no-git`       | off     | Skip git history scan, scan working tree only        |
| `--json`         | off     | Emit a single JSON array instead of coloured text    |
| `--max-size=N`   | `5`     | Maximum file size in MB to scan (skip larger files)  |

---

## Reading the Output

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

---

## Patterns Detected (currently 23)

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

---

## How It Works

SecretHunter is **embarrassingly parallel**. The tool fans out across CPU cores in three phases:

1. **File phase** — recursively walks the working tree, then scans every eligible file in parallel via `async`/`await`. Files larger than 500 lines are themselves split into sub-chunks and scanned across multiple cores in parallel.

2. **Repository discovery** — during the same directory walk, any directory containing a `.git` subdirectory is recorded as a repo root. This works regardless of nesting depth — a scan of `~/Documents` will automatically discover `~/Documents/Development/MyProject`, `~/Documents/Work/ApiService`, and any other git repos present.

3. **Git phase** — enumerates every commit from **all discovered repositories** via `git rev-list --all`, then flattens all commits across all repos into a single work queue. The parallel worker pool distributes those commits across CPU cores simultaneously — not one-repo-at-a-time. Each commit's diff is further split by file block and scanned in parallel.

The `--no-git` flag suppresses phases 2 and 3 entirely.

Both phases use a **cheap "needle" pre-filter**: each pattern has a distinctive substring (e.g. `"AKIA"`, `"ghp_"`, `"BEGIN"`) checked with `indexOf` before the regex runs. Lines that can't possibly match any pattern are skipped in microseconds.

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

### What Writes? Nothing.

SecretHunter writes **zero files** and makes **zero network calls**. Output goes to stdout only. You can run it with the network disconnected and it will behave identically — proof that no credential is ever transmitted.

---

## Performance

Measured on a 10-core Mac Mini (Apple M-series) using GNU `gtime`. All runs used the default 5 MB file size limit.

| Workload | Files | Commits | Wall | User | Sys | CPU% | Peak RSS | Throughput |
| -------- | ----- | ------- | ---- | ---- | --- | ---- | -------- | ---------- |
| kLex repo — files only | 328 | — | **0.81 s** | 5.67 s | 0.52 s | 763% | 95 MB | — |
| kLex repo — files + full git history | 328 | 27 | **1.61 s** | 8.95 s | 1.19 s | 626% | 83 MB | — |
| ~/Documents/Development (22 GB, files only) | 5,501 | — | **8.32 s** | 60.28 s | 5.62 s | 791% | 134 MB | 2.6 GB/s |
| ~/Documents/ (71 GB, files only) | 12,811 | — | **17.21 s** | 124.55 s | 11.61 s | 791% | 139 MB | 4.1 GB/s |
| ~/Documents/ (71 GB, **files + git history**, 3 repos auto-discovered) | 12,811 | 29 | **18.42 s** | 130.27 s | 12.65 s | 775% | 146 MB | 3.9 GB/s |

The 71 GB full-documents run with git history enabled found **48 real findings** (2 CRITICAL, 21 HIGH, 25 LOW) across 12,811 scanned files and 29 commits across 3 automatically discovered repositories — in under 19 seconds with no configuration, no network access, and no indexing infrastructure.

> **Why 12,811 files from 71 GB?** Most of the data in a typical documents folder is binary — images, videos, PDFs, archives, compiled binaries. SecretHunter skips those automatically. The 12,811 figure represents the text and source files that could actually contain a secret.

**CPU scaling:** SecretHunter consistently saturates 7–8 cores regardless of workload size. The async worker pool scales to the available core count automatically — it will use whatever you give it.

**Throughput note:** GB/s throughput is measured against total folder size including skipped binaries. If measured against scanned-file bytes only, effective throughput is higher.

**`gtime` field definitions:** `Wall` = elapsed clock time; `User` = total CPU time in user space summed across all cores; `Sys` = kernel time; `CPU%` = average core utilisation ((user + sys) / wall); `Peak RSS` = maximum resident set size (heap high-water mark).

---

## Adding a New Pattern

Edit `secretHunter.lex` and append to the `PATTERNS` array:

```
{ "name": "Heroku API Key",
  "needle": "HRKU-",
  "regex": `HRKU-[A-Za-z0-9]{32}`,
  "severity": "CRITICAL",
  "action": "Rotate the key in the Heroku dashboard." },
```

The `needle` must be a distinctive case-sensitive substring that appears anywhere a real match would. Leave it as `""` for heuristic patterns that have no distinctive prefix — they will always run the regex.

Re-run the fixture test after adding patterns:

```
./klex tests/unit/secretHunterTest.lex
```

---

## Tests

The end-to-end fixture test lives at `tests/unit/secretHunterTest.lex`. It plants known-fake secret strings in a temp directory, runs the scanner, and asserts every expected pattern is detected with the correct severity.

```
./klex tests/unit/secretHunterTest.lex
```

Should print `13/13 assertions passed`.

---

## Limitations

- **Regex-based**: SecretHunter detects credentials with deterministic shapes. Custom or non-standard credential formats won't be caught unless you add a pattern.
- **No entropy detection**: Tools like TruffleHog also flag high-entropy strings near suspicious keywords. SecretHunter does not do this (yet).
- **No verification**: SecretHunter does **not** test whether a found key is currently active. It only finds the *shape*. (This is intentional — verification would require network calls.)

