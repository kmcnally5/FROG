// storeRepairTest.lex — proves OFI #15 crash-recovery in openStoreLite.
//
// Builds a synthetic broken frogLight store: library.f32 holds 8 vectors
// (each 4 floats = 16 bytes, dim=4) but library.json holds only 5 lines.
// manifest.rows claims 8. openStoreLite should detect the desync,
// truncate library.f32 down to 5 rows, rewrite the manifest to rows=5,
// and stash the orphan tail in library.f32.orphan-<ts>.
//
// Also tests the two new fs builtins (_fsCountLines, _fsTruncate)
// directly.
//
// Run with: ./klex tests/unit/storeRepairTest.lex
// Exit 0 on all-pass.

import "projects/frogLight/store.lex" as store

let failures = 0
let dir = "/tmp/__klex_store_repair_test"

// Clean slate.
safe(fn() { _processExec("/bin/sh", ["-c", "rm -rf " + dir]) return null })
_, _ = _fsMkdirAll(dir)

// ── 1. _fsCountLines direct ─────────────────────────────────────────
let testFile = dir + "/lines.txt"
_, _ = _fsWrite(testFile, "a\nb\nc\nd\n")
let n, e = _fsCountLines(testFile)
if e != null {
    println("FAIL: _fsCountLines on 4-line file returned err: " + e)
    failures = failures + 1
} else if n != 4 {
    println("FAIL: _fsCountLines: 4-line file -> " + str(n))
    failures = failures + 1
} else {
    println("ok: _fsCountLines: 4-line file -> 4")
}

// File ending without trailing newline.
_, _ = _fsWrite(testFile, "a\nb\nc")
n, e = _fsCountLines(testFile)
if e != null || n != 3 {
    println("FAIL: _fsCountLines no-trailing-newline: " + str(n) + " err=" + str(e))
    failures = failures + 1
} else {
    println("ok: _fsCountLines: trailing-newline-less file counts last line")
}

// Empty file.
_, _ = _fsWrite(testFile, "")
n, e = _fsCountLines(testFile)
if n != 0 || e != null {
    println("FAIL: _fsCountLines empty: " + str(n) + " err=" + str(e))
    failures = failures + 1
} else {
    println("ok: _fsCountLines: empty file -> 0")
}

// Missing file -> (0, error) with err non-null.
n, e = _fsCountLines(dir + "/__nope__.txt")
if e == null {
    println("FAIL: _fsCountLines on missing file did not return an error")
    failures = failures + 1
} else if n != 0 {
    println("FAIL: _fsCountLines missing file count " + str(n) + " expected 0")
    failures = failures + 1
} else {
    println("ok: _fsCountLines: missing file -> (0, error)")
}

// ── 2. _fsTruncate direct ────────────────────────────────────────────
_, _ = _fsWrite(testFile, "0123456789abcdef")           // 16 bytes
e = _fsTruncate(testFile, 10)
if e != null {
    println("FAIL: _fsTruncate to 10 returned err: " + str(e))
    failures = failures + 1
} else {
    let info, _ = _fsStat(testFile)
    if info["size"] != 10 {
        println("FAIL: _fsTruncate: file size after truncate = " + str(info["size"]))
        failures = failures + 1
    } else {
        println("ok: _fsTruncate: 16 -> 10 bytes")
    }
}

// _fsTruncate negative -> runtime error.
_, e = safe(fn() { return _fsTruncate(testFile, -1) })
if e == null {
    println("FAIL: _fsTruncate(-1) did not error")
    failures = failures + 1
} else {
    println("ok: _fsTruncate(-1) rejected -> " + e.message)
}

// ── 3. Synthetic broken store ────────────────────────────────────────
// Wipe + recreate.
safe(fn() { _processExec("/bin/sh", ["-c", "rm -rf " + dir + "/*"]) return null })

let dim = 4                       // 4 floats per vector = 16 bytes per row
let rowBytes = dim * 4

// library.f32: 8 rows of zeros = 128 bytes. We use ZERO bytes so the
// orphan-tail backup is easy to identify.
let zerosRow = bytes(rowBytes)
let allRows  = bytesConcat([zerosRow, zerosRow, zerosRow, zerosRow,
                        zerosRow, zerosRow, zerosRow, zerosRow])
_, e = _fsWrite(dir + "/library.f32", "")           // create
// _fsWrite takes a string; use bytes-aware write via the bytes-string
// roundtrip — for zero bytes that's safe.
// Actually we need to write raw bytes, so use _fsAppendBytesSync via
// an empty wipe then append.
_, e = _fsAppendBytesSync(dir + "/library.f32", allRows)
if e != null {
    println("FAIL: writing library.f32: " + e)
    failures = failures + 1
}

// library.json: 5 metadata lines (any valid JSON shape works, since
// openStoreLite only counts lines — it doesn't parse them). Write via
// shell to avoid kLex's `{…}` string-interpolation eating the JSON
// literal braces.
_, _, _, _ = _processExec("/bin/sh", ["-c",
    "printf '%s\\n' " +
    "'\"a@1\"' '\"b@2\"' '\"c@3\"' '\"d@4\"' '\"e@5\"' > " +
    dir + "/library.json"])

// manifest.json: claims 8 rows (matching .f32 but NOT .json — exactly
// the desync the repair logic must catch).
_, _, _, _ = _processExec("/bin/sh", ["-c",
    "printf '%s' " +
    "'" + chr(123) + "\"version\":1,\"dim\":4,\"model\":\"test\",\"rows\":8" + chr(125) + "' > " +
    dir + "/manifest.json"])

// Sanity check the synthetic state before opening.
let infoF, _ = _fsStat(dir + "/library.f32")
let infoJ, _ = _fsStat(dir + "/library.json")
println("pre-open: library.f32 = " + str(infoF["size"]) + " bytes (8 rows × 16)")
println("pre-open: library.json = " + str(infoJ["size"]) + " bytes")
let preLines, _ = _fsCountLines(dir + "/library.json")
println("pre-open: library.json line count = " + str(preLines))

// ── 4. openStoreLite — should repair ─────────────────────────────────
let s, sErr = store.openStoreLite(dir)
if sErr != null {
    println("FAIL: openStoreLite returned error: " + sErr.message)
    failures = failures + 1
} else {
    // Manifest.rows must have been rewritten from 8 -> 5.
    if s.rows != 5 {
        println("FAIL: s.rows after repair = " + str(s.rows) + " expected 5")
        failures = failures + 1
    } else {
        println("ok: s.rows after repair = 5 (was 8)")
    }
    // library.f32 must be trimmed to 5 × 16 = 80 bytes.
    let infoF2, _ = _fsStat(dir + "/library.f32")
    if infoF2["size"] != 80 {
        println("FAIL: library.f32 size after repair = " + str(infoF2["size"]) + " expected 80")
        failures = failures + 1
    } else {
        println("ok: library.f32 trimmed to 80 bytes (5 rows × 16)")
    }
    // Orphan backup file must exist with the trimmed 48 bytes (3 rows × 16).
    let entries, _ = _fsReadDir(dir)
    let orphanFound = false
    let i = 0
    while i < len(entries) {
        let name = entries[i]["name"]
        if indexOf(name, "library.f32.orphan-") == 0 {
            orphanFound = true
            let sz = entries[i]["size"]
            if sz != 48 {
                println("FAIL: orphan backup size = " + str(sz) + " expected 48")
                failures = failures + 1
            } else {
                println("ok: orphan backup " + name + " = 48 bytes (3 rows × 16)")
            }
        }
        i = i + 1
    }
    if !orphanFound {
        println("FAIL: no library.f32.orphan-<ts> backup file was created")
        failures = failures + 1
    }
    // Manifest on disk should also reflect rows=5 now.
    let raw, _ = _fsRead(dir + "/manifest.json")
    if indexOf(raw, "\"rows\":5") < 0 {
        println("FAIL: manifest.json on disk doesn't show rows:5 — " + raw)
        failures = failures + 1
    } else {
        println("ok: manifest.json on disk rewritten with rows:5")
    }
}

// ── 5. Idempotent — second openStoreLite is a no-op ─────────────────
let s2, sErr = store.openStoreLite(dir)
if sErr != null {
    println("FAIL: second openStoreLite errored: " + sErr.message)
    failures = failures + 1
} else if s2.rows != 5 {
    println("FAIL: second openStoreLite changed s.rows to " + str(s2.rows))
    failures = failures + 1
} else {
    let info3, _ = _fsStat(dir + "/library.f32")
    if info3["size"] != 80 {
        println("FAIL: second openStoreLite changed library.f32 size to " + str(info3["size"]))
        failures = failures + 1
    } else {
        println("ok: second openStoreLite is a no-op (idempotent)")
    }
}

// Cleanup.
safe(fn() { _processExec("/bin/sh", ["-c", "rm -rf " + dir]) return null })

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — openStoreLite detects and repairs desynced library.f32 (OFI #15)")
