// replaceAllTest.lex — proves replaceAll and replace are equivalent
// (both call strings.ReplaceAll under the hood). Locks the contract
// that `replace` IS replace-all-occurrences, not single-replace.
//
// Run with: ./klex tests/unit/replaceAllTest.lex
// Exit 0 on all-pass.

let failures = 0

// 1. replaceAll: multiple occurrences all replaced.
let got = replaceAll("a-b-c-d", "-", "_")
if got != "a_b_c_d" {
    println("FAIL: replaceAll multi-occurrence: got '" + got + "' expected 'a_b_c_d'")
    failures = failures + 1
} else {
    println("ok: replaceAll multi-occurrence")
}

// 2. replaceAll: zero occurrences -> string unchanged.
got = replaceAll("nothing to find", "xyz", "?")
if got != "nothing to find" {
    println("FAIL: replaceAll zero matches: got '" + got + "'")
    failures = failures + 1
} else {
    println("ok: replaceAll zero matches")
}

// 3. replaceAll: empty needle -> Go semantics (insert between every char).
got = replaceAll("abc", "", "-")
if got != "-a-b-c-" {
    println("FAIL: replaceAll empty needle: got '" + got + "'")
    failures = failures + 1
} else {
    println("ok: replaceAll empty needle (Go semantics)")
}

// 4. `replace` must be IDENTICAL to `replaceAll` — they're aliases.
let ra = replaceAll("x.y.z", ".", "/")
let r  = replace("x.y.z", ".", "/")
if ra != r {
    println("FAIL: replace vs replaceAll mismatch: '" + r + "' vs '" + ra + "'")
    failures = failures + 1
} else if r != "x/y/z" {
    println("FAIL: replace not doing replace-all: got '" + r + "'")
    failures = failures + 1
} else {
    println("ok: replace and replaceAll are equivalent")
}

// 5. Wrong arity -> runtime error.
let _, e = safe(fn() { return replaceAll("a") })
if e == null {
    println("FAIL: replaceAll(1 arg) did not error")
    failures = failures + 1
} else {
    println("ok: replaceAll(1 arg) errored cleanly: " + e.message)
}

// 6. Wrong type -> runtime error.
_, e = safe(fn() { return replaceAll(42, "x", "y") })
if e == null {
    println("FAIL: replaceAll(int, ...) did not error")
    failures = failures + 1
} else {
    println("ok: replaceAll(int, ...) errored cleanly: " + e.message)
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — replaceAll builtin behaves as documented")
