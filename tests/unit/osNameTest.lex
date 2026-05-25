// osNameTest.lex — locks the _osName contract.
//
// _osName returns Go's runtime.GOOS verbatim. We can't assert a specific
// value (the test must pass on any platform), so we assert structural
// properties: non-empty string, all-lowercase ASCII, no whitespace, and
// it matches one of the known platforms our build targets.
//
// Run with: ./klex tests/unit/osNameTest.lex
// Exit 0 on all-pass.

let failures = 0

let name = _osName()

// 1. Type and non-empty.
if type(name) != "STRING" {
    println("FAIL: _osName() returned " + type(name) + ", expected STRING")
    failures = failures + 1
} else if len(name) == 0 {
    println("FAIL: _osName() returned empty string")
    failures = failures + 1
} else {
    println("ok: _osName() = '" + name + "'")
}

// 2. Lowercase ASCII identifier, no whitespace.
let i = 0
let n = len(name)
let bad = -1
while i < n {
    let c = ord(substr(name, i, i + 1))
    let isLower = c >= 97 && c <= 122
    if !isLower {
        bad = i
        i = n
    }
    i = i + 1
}
if bad >= 0 {
    println("FAIL: non-lowercase byte at index " + str(bad) + " in '" + name + "'")
    failures = failures + 1
} else {
    println("ok: all-lowercase ASCII identifier")
}

// 3. Match a known platform we expect to build on.
let known = {
    "darwin":  true,
    "linux":   true,
    "windows": true,
    "freebsd": true,
    "openbsd": true,
    "netbsd":  true,
}
if !hasKey(known, name) {
    println("FAIL: _osName() = '" + name + "' is not in the known set. Add it to the test if this is a legitimate new target.")
    failures = failures + 1
} else {
    println("ok: '" + name + "' is a known build target")
}

// 4. Idempotency — two calls return the same value (no side-effects).
let n1 = _osName()
let n2 = _osName()
if n1 != n2 {
    println("FAIL: two calls returned different values: '" + n1 + "' vs '" + n2 + "'")
    failures = failures + 1
} else {
    println("ok: idempotent")
}

// 5. Arity check.
let _, e = safe(fn() { return _osName("extra") })
if e == null {
    println("FAIL: _osName('extra') did not error")
    failures = failures + 1
} else {
    println("ok: extra-arg rejected — " + e.message)
}

// 6. Real cross-platform dispatch demo (proves it's usable for the
//    intended use case).
let cmd = ""
if name == "darwin"  { cmd = "open"     }
if name == "linux"   { cmd = "xdg-open" }
if name == "windows" { cmd = "cmd"      }
if len(cmd) == 0 {
    println("FAIL: dispatch demo produced empty cmd for '" + name + "'")
    failures = failures + 1
} else {
    println("ok: open-file dispatch for this platform = '" + cmd + "'")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — _osName behaves as documented on " + name)
