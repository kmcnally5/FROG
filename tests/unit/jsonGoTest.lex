// jsonGoTest.lex — locks the Go-backed JSON parser/stringifier (OFI #14).
//
// Run with: ./klex tests/unit/jsonGoTest.lex
// Exit 0 on all-pass.

import "stdlib/json.lex" as json

let failures = 0

// ── Helper to write a JSON document via shell, avoiding any kLex-side
// escape interpretation of quotes/braces. The kLex lexer panics on
// some edge cases of escaped-quote-heavy literals (separate OFI), so
// we keep test inputs OUT of the .lex source as much as possible.
let fix = "/tmp/__klex_json_fix"
_, _ = _fsMkdirAll(fix)

fn writeFix(name, shellQuotedJSON) {
    let path = fix + "/" + name
    _, _, _, _ = _processExec("/bin/sh", ["-c",
        "printf '%s' " + shellQuotedJSON + " > " + path])
    let raw, _ = _fsRead(path)
    return raw
}

fn parseFix(name, shellQuotedJSON) {
    return json.parse(writeFix(name, shellQuotedJSON))
}

// ── Primitives ───────────────────────────────────────────────────────
let v, e = parseFix("v_null", "null")
if e != null || v != null { println("FAIL: null") failures = failures + 1 }
else { println("ok: parse null") }

v, e = parseFix("v_true", "true")
if e != null || v != true { println("FAIL: true") failures = failures + 1 }
else { println("ok: parse true") }

v, e = parseFix("v_false", "false")
if e != null || v != false { println("FAIL: false") failures = failures + 1 }
else { println("ok: parse false") }

// ── Numbers ──────────────────────────────────────────────────────────
v, _ = parseFix("v_int", "42")
if type(v) != "INTEGER" || v != 42 { println("FAIL: 42") failures = failures + 1 }
else { println("ok: parse 42 -> Integer") }

v, _ = parseFix("v_neg", "-17")
if type(v) != "INTEGER" || v != -17 { println("FAIL: -17") failures = failures + 1 }
else { println("ok: parse -17 -> Integer") }

v, _ = parseFix("v_float", "3.14")
if type(v) != "FLOAT" { println("FAIL: 3.14 type=" + type(v)) failures = failures + 1 }
else { println("ok: parse 3.14 -> Float") }

v, _ = parseFix("v_exp", "1.5e3")
if type(v) != "FLOAT" || v != 1500.0 { println("FAIL: 1.5e3") failures = failures + 1 }
else { println("ok: parse 1.5e3 -> 1500.0 Float") }

v, _ = parseFix("v_bigint", "9000000000000")
if type(v) != "INTEGER" || v != 9000000000000 { println("FAIL: bigint") failures = failures + 1 }
else { println("ok: parse 9e12 -> Integer (no float loss)") }

// ── Strings — quoted via single-quote shell wrap ────────────────────
let dq = chr(34)              // "  (literal double-quote char)
let hello = dq + "hello world" + dq
v, _ = parseFix("v_str", "'" + hello + "'")
if v != "hello world" { println("FAIL: hello world") failures = failures + 1 }
else { println("ok: parse simple string") }

// ── Arrays / objects — build the JSON via concatenation ─────────────
let arrJSON = "'[1, 2, 3]'"
v, _ = parseFix("v_arr", arrJSON)
if type(v) != "ARRAY" || len(v) != 3 || v[0] != 1 || v[2] != 3 {
    println("FAIL: [1,2,3]") failures = failures + 1
} else { println("ok: parse [1,2,3]") }

v, _ = parseFix("v_arr_empty", "'[]'")
if type(v) != "ARRAY" || len(v) != 0 { println("FAIL: empty array") failures = failures + 1 }
else { println("ok: parse []") }

// Mixed array.
let arrMixed = "'[1, " + dq + "two" + dq + ", 3.0, true, null]'"
v, _ = parseFix("v_arr_mix", arrMixed)
if len(v) != 5 || v[0] != 1 || v[1] != "two" || type(v[2]) != "FLOAT" ||
   v[3] != true || v[4] != null {
    println("FAIL: mixed array") failures = failures + 1
} else { println("ok: parse mixed array") }

// Object.
let lb = chr(123)
let rb = chr(125)
let objJSON = "'" + lb + dq + "name" + dq + ":" + dq + "karl" + dq + "," +
          dq + "age" + dq + ":42," +
          dq + "items" + dq + ":[1,2,3]" + rb + "'"
v, _ = parseFix("v_obj", objJSON)
if type(v) != "HASH" || v["name"] != "karl" || v["age"] != 42 ||
   len(v["items"]) != 3 {
    println("FAIL: object") failures = failures + 1
} else { println("ok: parse object") }

v, _ = parseFix("v_obj_empty", "'" + lb + rb + "'")
if type(v) != "HASH" || len(v) != 0 { println("FAIL: empty object") failures = failures + 1 }
else { println("ok: parse empty object") }

// ── Parse errors ─────────────────────────────────────────────────────
v, e = parseFix("v_bad", "'" + lb + " bad json'")
if e == null { println("FAIL: bad json no error") failures = failures + 1 }
else { println("ok: malformed json errored") }

v, e = parseFix("v_trail", "'42 trailing'")
if e == null { println("FAIL: trailing content no error") failures = failures + 1 }
else { println("ok: trailing content errored") }

// ── Stringify ────────────────────────────────────────────────────────
if json.stringify(null)  != "null"  { println("FAIL: stringify null") failures = failures + 1 }
else { println("ok: stringify null") }

if json.stringify(true)  != "true"  { println("FAIL: stringify true") failures = failures + 1 }
else { println("ok: stringify true") }

if json.stringify(42)    != "42"    { println("FAIL: stringify 42") failures = failures + 1 }
else { println("ok: stringify 42") }

if json.stringify(3.14)  != "3.14"  { println("FAIL: stringify 3.14") failures = failures + 1 }
else { println("ok: stringify 3.14") }

if json.stringify([1, 2, 3]) != "[1,2,3]" {
    println("FAIL: stringify [1,2,3]") failures = failures + 1
} else { println("ok: stringify [1,2,3]") }

let s = json.stringify({"b": 2, "a": 1})
let expectedHash = lb + dq + "a" + dq + ":1," + dq + "b" + dq + ":2" + rb
if s != expectedHash {
    println("FAIL: stringify hash sorted-keys got '" + s + "', expected '" + expectedHash + "'")
    failures = failures + 1
} else { println("ok: stringify hash sorted") }

// ── Round-trip ───────────────────────────────────────────────────────
let orig = {"name": "karl",
        "items": [1, 2.5, "x", null, true, false],
        "nested": {"k": "v"}}
s = json.stringify(orig)
v, e = json.parse(s)
if e != null {
    println("FAIL: round-trip parse — " + e) failures = failures + 1
} else if v["name"] != "karl" || v["items"][0] != 1 || v["items"][1] != 2.5 ||
          v["items"][3] != null || v["items"][5] != false || v["nested"]["k"] != "v" {
    println("FAIL: round-trip mismatch — " + s) failures = failures + 1
} else { println("ok: round-trip orig -> string -> parse") }

// ── Bytes rejected on stringify ──────────────────────────────────────
// stdlib/json.lex's stringify wrapper returns an Error VALUE (via the
// error() builtin) on bad input — the Go path returns a (string, err)
// tuple and the wrapper turns the err into an error value. The right
// check is therefore `type(result) == "ERROR"`, not safe().
let b = strToBytes("xx")
let result = json.stringify(b)
if type(result) != "ERROR" {
    println("FAIL: stringify(bytes) returned " + type(result) + " not ERROR")
    failures = failures + 1
} else { println("ok: stringify(bytes) returned ERROR with code " + result.code) }

// ── Performance smoke — parse 5000 JSONL lines ──────────────────────
let perfFix = fix + "/perf.jsonl"
// Build the JSON literal once via concatenation, then have the shell
// repeat it 5000 times. Using single quotes around the JSON in the
// shell layer means we never have to escape the inner double quotes —
// the shell hands them through to printf intact.
let jsonLine = lb + dq + "path" + dq + ":" + dq + "/x.lex" + dq +
           "," + dq + "chunk" + dq + ":0," +
           dq + "mtime" + dq + ":1779000000" + rb
_, _, _, _ = _processExec("/bin/sh", ["-c",
    "rm -f " + perfFix + "; " +
    "for i in $(seq 1 5000); do " +
    "  printf '%s\n' '" + jsonLine + "' >> " + perfFix + "; " +
    "done"])

let raw, _ = _fsRead(perfFix)
let lines = split(raw, "\n")

let t0 = _timeNanos()
let ok = 0
let li = 0
while li < len(lines) {
    if len(lines[li]) > 0 {
        v, e = json.parse(lines[li])
        if e == null { ok = ok + 1 }
    }
    li = li + 1
}
let elapsedMs = (_timeNanos() - t0) / 1000000
println("ok: parsed " + str(ok) + " JSONL lines in " + str(elapsedMs) + "ms")
if ok != 5000 {
    println("FAIL: only " + str(ok) + "/5000 parsed")
    failures = failures + 1
}
if elapsedMs > 3000 {
    println("FAIL: 5000-line parse took " + str(elapsedMs) + "ms (>3s) — Go path may not be active")
    failures = failures + 1
}

// Cleanup.
safe(fn() { _processExec("/bin/sh", ["-c", "rm -rf " + fix]) return null })

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — json.parse / json.stringify backed by Go (OFI #14)")
