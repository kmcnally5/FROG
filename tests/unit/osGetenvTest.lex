// osGetenvTest.lex — locks the _osGetenv contract:
//   - returns *String when the variable IS set
//   - returns NULL when the variable is NOT set
//   - is a SINGLE-VALUE return (not a tuple) — unpacking into two
//     variables must fail (matches OFI #6 resolution: signature is
//     `_osGetenv(name) -> string | null`, not (string, err))
//
// Run with: ./klex tests/unit/osGetenvTest.lex
// Exit 0 on all-pass.

import "stdlib/assert.lex" as a

let failures = 0

// 1. Variable that's almost always set: PATH.
let v = _osGetenv("PATH")
if v == null {
    println("FAIL: _osGetenv(PATH) returned null — but PATH should be set")
    failures = failures + 1
} else if type(v) != "STRING" {
    println("FAIL: _osGetenv(PATH) returned " + type(v) + ", expected STRING")
    failures = failures + 1
} else if len(v) == 0 {
    println("FAIL: _osGetenv(PATH) returned an empty string")
    failures = failures + 1
} else {
    println("ok: _osGetenv(PATH) returned non-empty string (" + str(len(v)) + " chars)")
}

// 2. Variable that doesn't exist.
v = _osGetenv("__KLEX_DOES_NOT_EXIST__")
if v != null {
    println("FAIL: _osGetenv(unset var) returned " + str(v) + ", expected null")
    failures = failures + 1
} else {
    println("ok: _osGetenv(unset var) returned null")
}

// 3. Single-value contract: tuple unpacking MUST fail because the
//    return shape is a single value, not (val, err).
let _, e = safe(fn() {
    let val, err = _osGetenv("PATH")
    return val
})
if e == null {
    println("FAIL: _osGetenv tuple-unpack succeeded — return shape is NOT (val, err) per the locked contract")
    failures = failures + 1
} else {
    println("ok: tuple-unpack of _osGetenv correctly errored (" + e.message + ")")
}

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — _osGetenv contract is single-value string|null")
