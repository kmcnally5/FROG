#!/usr/bin/env python3
"""
yara_bridge.py — kLex native bridge for YARA secret scanning.

Protocol: line-delimited JSON over stdin/stdout (standard kLex bridge protocol).

Functions exposed:
  load(rules_path)       — compile YARA rules from a .yar file
  scan(file_path)        — scan a file, return list of findings
  ping()                 — health check
"""
import json
import sys
import os


try:
    import yara
except ImportError:
    sys.stderr.write("yara-python not installed. Run: pip3 install yara-python\n")
    sys.exit(1)

_compiled = None


def load(rules_path):
    global _compiled
    if not os.path.exists(rules_path):
        raise ValueError(f"rules file not found: {rules_path}")
    _compiled = yara.compile(filepath=rules_path)
    return {"loaded": True, "rules": rules_path}


def _scan_one(file_path):
    """Scan a single file. Returns list of findings (one per matched rule)."""
    if _compiled is None:
        raise RuntimeError("call load() before scan()")
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


def scan(file_path):
    """Scan a single file — kept for backward compatibility."""
    return _scan_one(file_path)


def scan_batch(file_paths):
    """Scan a list of files in one call. Each finding includes 'file' key.
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


def ping():
    return "pong"


import math
import re
from collections import Counter

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
_HEX_ONLY_RE  = re.compile(r'^[0-9a-fA-F]+$')
_UUID_RE       = re.compile(
    r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}'
    r'-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
)

def _entropy(s):
    if not s:
        return 0.0
    counts = Counter(s)
    n = len(s)
    return -sum((c / n) * math.log2(c / n) for c in counts.values())

def entropy_scan(file_paths, threshold=4.5):
    """Scan files for high-entropy strings that may be credentials.
    Returns list of {file, match, entropy, severity} dicts."""
    findings = []
    for file_path in file_paths:
        # Skip file types known to produce high-entropy noise.
        ext = os.path.splitext(file_path)[1].lower()
        if ext in _ENTROPY_SKIP_EXTS:
            continue

        try:
            with open(file_path, 'r', errors='replace') as f:
                content = f.read(1024 * 1024)   # cap at 1 MB per file
        except Exception:
            continue

        # Skip binary files — null bytes in the first 4 KB = compiled binary.
        if '\x00' in content[:4096]:
            continue

        seen = set()
        for m in _ENTROPY_RE.finditer(content):
            token = m.group(1) or m.group(2)
            if not token or token in seen:
                continue
            seen.add(token)

            # Skip pure-hex strings (SHA256, MD5, commit hashes, etc.)
            if _HEX_ONLY_RE.match(token):
                continue

            # Skip UUIDs — common in JSON and config, never secrets.
            if _UUID_RE.match(token):
                continue

            # Real secrets almost always mix upper, lower, AND digits.
            has_upper = any(c.isupper() for c in token)
            has_lower = any(c.islower() for c in token)
            has_digit = any(c.isdigit() for c in token)
            if not (has_upper and has_lower and has_digit):
                continue

            # Must be overwhelmingly secret-charset characters.
            if sum(1 for c in token if c in _SECRET_CHARS) / len(token) < 0.85:
                continue

            ent = _entropy(token)
            if ent < threshold:
                continue

            findings.append({
                "file":     file_path,
                "match":    token[:80],
                "entropy":  round(ent, 2),
                "severity": "HIGH" if ent >= 5.0 else "MEDIUM",
            })

    return findings

HANDLERS = {
    "load":         load,
    "scan":         scan,
    "scan_batch":   scan_batch,
    "entropy_scan": entropy_scan,
    "ping":         ping,
}

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req     = json.loads(line)
        req_id  = req["id"]
        fn_name = req["fn"]
        args    = req.get("args", [])

        if fn_name not in HANDLERS:
            resp = {"id": req_id, "error": f"unknown function: {fn_name}"}
        else:
            result = HANDLERS[fn_name](*args)
            resp   = {"id": req_id, "result": result}

    except Exception as e:
        resp = {"id": req.get("id", 0), "error": str(e)}

    print(json.dumps(resp), flush=True)
