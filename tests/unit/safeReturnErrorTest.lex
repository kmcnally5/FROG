// safeReturnErrorTest.lex — locks OFI #17.
//
// Before the fix, `safe(fn() { return error("CODE", "msg") })` returned
// (Error, null) — the user-constructed Error sat in the value slot and
// the err slot was null, so the natural `_, err = safe(...)` idiom missed
// the error entirely.
//
// After the fix, safe() detects a bare *Error return and lifts it into
// the err slot, matching the propagation path. Both forms ("raise via
// dividing by zero" and "return error(...)") now produce (null, Error).
//
// Pre-existing behaviour preserved:
//   - functions that return a tuple pass through unchanged
//   - functions that return a non-error value still produce (val, null)
//   - functions that propagate a runtime error still produce (null, Error)

let failures = 0

fn check(name, cond) {
    if cond {
        println("ok: " + name)
    } else {
        println("FAIL: " + name)
        failures = failures + 1
    }
}

// 1. Returned error() value is lifted to err slot.
let v, e = safe(fn() { return error("BAD", "things went wrong") })
check("returned error: value slot is null",  v == null)
check("returned error: err slot is non-null", e != null)
check("returned error: err.code preserved",   e.code == "BAD")
check("returned error: err.message preserved", e.message == "things went wrong")

// 2. Plain value still goes to value slot.
v, e = safe(fn() { return 42 })
check("plain value: 42 in value slot", v == 42)
check("plain value: err is null",      e == null)

// 3. Null return still produces (null, null).
v, e = safe(fn() { return null })
check("null return: value slot null",  v == null)
check("null return: err slot null",    e == null)

// 4. Tuple passthrough — fn that returns (val, err) directly.
v, e = safe(fn() { return 7, null })
check("tuple passthrough: success value", v == 7)
check("tuple passthrough: success err",   e == null)

v, e = safe(fn() { return null, error("X", "explicit tuple err") })
check("tuple passthrough: failure value", v == null)
check("tuple passthrough: failure err.code", e.code == "X")

// 5. Propagated runtime error still caught as before.
v, e = safe(fn() { return 1 / 0 })
check("propagated runtime: value null", v == null)
check("propagated runtime: err non-null", e != null)
check("propagated runtime: code is RUNTIME_ERROR", e.code == "RUNTIME_ERROR")

// 6. Returning error() from a builtin call wrapped in fn() works too.
fn wrapErr() { return error("WRAPPED", "from fn wrapper") }
v, e = safe(wrapErr)
check("named fn returning error: value null", v == null)
check("named fn returning error: code WRAPPED", e.code == "WRAPPED")

// 7. Nested safe() — the inner returns error(), outer also lifts.
fn inner() { return error("INNER", "deep") }
fn outer() {
    let val, err = safe(inner)
    // Pass the error back up by returning it as a value — outer's safe()
    // should lift it again rather than wrapping it.
    if err != null { return err }
    return val
}
v, e = safe(outer)
check("nested safe: value null",    v == null)
check("nested safe: code preserved", e.code == "INNER")

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — safe() lifts returned error() values into the err slot")
