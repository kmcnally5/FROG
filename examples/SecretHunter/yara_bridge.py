#!/usr/bin/env python3
"""
yara_bridge.py — kLex native bridge for YARA secret scanning + Shannon-entropy
detection. Uses the klex_bridge helper module (PYTHONPATH is auto-injected by
kLex's nativeBridge so the import resolves transparently).

Handlers exposed:
  load(rules_path)       — compile YARA rules from a .yar file
  scan(file_path)        — scan a single file, return list of findings
  scan_batch(file_paths) — scan many files in one call (fast path)
  entropy_scan(files)    — Shannon-entropy heuristic for novel credentials
  ping()                 — health check

If yara-python isn't installed, only entropy_scan and ping work; the YARA
handlers surface a clean ImportError as a structured kLex error (the bridge
itself stays alive so other handlers continue to function).
"""
import math
import os
import re
from collections import Counter

from klex_bridge import handler, stream_handler, serve


# ── YARA-backed handlers ─────────────────────────────────────────────────────

_compiled = None  # module-level YARA ruleset; populated by load()


def _yara_module():
    """Lazy-import yara so a missing yara-python doesn't kill the bridge at
    startup — only YARA handlers fail, with a structured ImportError that
    surfaces cleanly through err.errorType / err.message on the kLex side."""
    import yara  # noqa: F401 — raises ImportError with the standard message
    return yara


@handler(args=[("rules_path", "string")], returns="hash")
def load(rules_path):
    yara = _yara_module()
    if not os.path.exists(rules_path):
        raise FileNotFoundError(f"rules file not found: {rules_path}")
    global _compiled
    _compiled = yara.compile(filepath=rules_path)
    return {"loaded": True, "rules": rules_path}


def _scan_one(file_path):
    """Scan a single file. Returns list of findings (one per matched rule)."""
    if _compiled is None:
        raise RuntimeError("call load() before scan()")
    yara = _yara_module()
    try:
        matches = _compiled.match(file_path)
    except yara.Error:
        return []

    findings   = []
    seen_rules = set()
    for match in matches:
        if match.rule in seen_rules:
            continue
        seen_rules.add(match.rule)

        meta         = match.meta
        severity     = meta.get("severity", "HIGH")
        action       = meta.get("action", "Review and rotate this credential.")
        matched_text = match.rule
        for sm in match.strings:
            for inst in sm.instances:
                try:
                    matched_text = inst.matched_data.decode("utf-8", errors="replace")
                except Exception:
                    matched_text = f"<binary:{len(inst.matched_data)}b>"
                break
            break

        findings.append({
            "rule":     match.rule,
            "severity": severity,
            "action":   action,
            "match":    matched_text,
            "tags":     list(match.tags),
        })
    return findings


@handler(args=[("file_path", "string")], returns="array")
def scan(file_path):
    """Scan a single file — kept for backward compatibility."""
    return _scan_one(file_path)


@handler(args=[("file_paths", "array")], returns="array")
def scan_batch(file_paths):
    """Scan a list of files in one call. Each finding includes a 'file' key.
    This is the fast path used by parallel workers — eliminates per-file
    round-trip overhead and lets each Python process work a full chunk."""
    if _compiled is None:
        raise RuntimeError("call load() before scan_batch()")

    all_findings = []
    for file_path in file_paths:
        for finding in _scan_one(file_path):
            finding["file"] = file_path
            all_findings.append(finding)
    return all_findings


@stream_handler(args=[("file_paths", "array")], yields="hash")
def scan_batch_stream(file_paths):
    """Streaming variant of scan_batch — same detection, but yields one item
    at a time so kLex callers can update progress + tile counts live.

    Two item shapes (mirrors entropy_scan_stream):
      {"kind": "finding",  "file": ..., "rule": ..., "severity": ..., "action": ..., "match": ..., "tags": [...]}
      {"kind": "progress", "file": ..., "done": N, "total": M}

    One "progress" item is emitted after every file (including those with no
    matches) so the UI progress bar can tick reliably.
    """
    if _compiled is None:
        raise RuntimeError("call load() before scan_batch_stream()")

    total = len(file_paths)
    for i, file_path in enumerate(file_paths):
        for finding in _scan_one(file_path):
            finding["file"] = file_path
            finding["kind"] = "finding"
            yield finding
        yield {
            "kind":  "progress",
            "file":  file_path,
            "done":  i + 1,
            "total": total,
        }


@handler(args=[], returns="string")
def ping():
    return "pong"


# ── Entropy detection ─────────────────────────────────────────────────────────
# Shannon entropy measures randomness in a string (bits per character).
# Real secrets (API keys, tokens, passwords) have high entropy (≥4.5)
# because they're designed to be unpredictable. Normal English words and
# code identifiers score much lower.

_ENTROPY_RE = re.compile(
    r'["\']([A-Za-z0-9+/=_\-\.]{20,})["\']'   # quoted strings ≥20 chars
    r'|([A-Za-z0-9+/=_\-]{36,})'               # bare long tokens ≥36 chars
)
_SECRET_CHARS = frozenset(
    'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=_-.'
)

# File extensions where entropy scanning produces mostly noise.
# Regex + YARA handle these formats better.
_ENTROPY_SKIP_EXTS = frozenset({
    '.json', '.lock', '.sum', '.mod', '.toml', '.xml', '.yaml', '.yml',
    '.svg', '.map', '.min', '.css', '.html', '.htm', '.wasm',
})

# Patterns that look high-entropy but are never secrets.
_HEX_ONLY_RE = re.compile(r'^[0-9a-fA-F]+$')
_UUID_RE     = re.compile(
    r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}'
    r'-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
)


def _entropy(s):
    if not s:
        return 0.0
    counts = Counter(s)
    n = len(s)
    return -sum((c / n) * math.log2(c / n) for c in counts.values())


def _entropy_scan_one(file_path, threshold=4.5):
    """Scan a single file for high-entropy strings. Returns list of finding
    dicts WITHOUT the 'file' key — callers add it. Returns [] for files that
    are skipped (binary, wrong extension, unreadable)."""
    ext = os.path.splitext(file_path)[1].lower()
    if ext in _ENTROPY_SKIP_EXTS:
        return []

    try:
        with open(file_path, 'r', errors='replace') as f:
            content = f.read(1024 * 1024)
    except Exception:
        return []

    if '\x00' in content[:4096]:
        return []

    findings = []
    seen = set()
    for m in _ENTROPY_RE.finditer(content):
        token = m.group(1) or m.group(2)
        if not token or token in seen:
            continue
        seen.add(token)

        if _HEX_ONLY_RE.match(token):
            continue
        if _UUID_RE.match(token):
            continue

        has_upper = any(c.isupper() for c in token)
        has_lower = any(c.islower() for c in token)
        has_digit = any(c.isdigit() for c in token)
        if not (has_upper and has_lower and has_digit):
            continue

        if sum(1 for c in token if c in _SECRET_CHARS) / len(token) < 0.85:
            continue

        ent = _entropy(token)
        if ent < threshold:
            continue

        findings.append({
            "match":    token[:80],
            "entropy":  round(ent, 2),
            "severity": "HIGH" if ent >= 5.0 else "MEDIUM",
        })
    return findings


@handler(args=[("file_paths", "array")], returns="array")
def entropy_scan(file_paths):
    """Scan files for high-entropy strings that may be credentials.
    Returns list of {file, match, entropy, severity} dicts.

    Threshold is fixed at 4.5 bits/char — a value below most real secrets
    but above typical English text and code identifiers.
    """
    findings = []
    for file_path in file_paths:
        for f in _entropy_scan_one(file_path):
            f["file"] = file_path
            findings.append(f)
    return findings


@stream_handler(args=[("file_paths", "array")], yields="hash")
def entropy_scan_stream(file_paths):
    """Streaming variant of entropy_scan — same detection logic, but yields
    items one at a time so kLex callers can update progress live.

    Two item shapes:
      {"kind": "finding",  "file": ..., "match": ..., "entropy": ..., "severity": ...}
      {"kind": "progress", "file": ..., "done": N, "total": M}

    One "progress" item is emitted after every file (including skipped ones)
    so the UI can drive a progress bar without waiting for the whole scan.
    """
    total = len(file_paths)
    for i, file_path in enumerate(file_paths):
        for f in _entropy_scan_one(file_path):
            f["file"] = file_path
            f["kind"] = "finding"
            yield f
        yield {
            "kind":  "progress",
            "file":  file_path,
            "done":  i + 1,
            "total": total,
        }


serve()
