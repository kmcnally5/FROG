// unwrapTest.lex — tests for the postfix ? error-propagation operator

passed = 0
failed = 0

fn check(name, got, expected) {
    if got == expected {
        passed = passed + 1
    } else {
        failed = failed + 1
        println("FAIL: " + name + " — expected " + str(expected) + " got " + str(got))
    }
}

// ── Helpers ──────────────────────────────────────────────────────────────────

fn okTuple(val) {
    return val, null          // kLex bare-comma tuple syntax
}

fn errTuple(msg) {
    return null, error("TEST_ERR", msg)
}

// ── Unwrap success path ───────────────────────────────────────────────────────

fn testUnwrapInt() {
    result = okTuple(42)?
    return result
}
check("? unwraps integer",  testUnwrapInt(),  42)

fn testUnwrapString() {
    s = okTuple("hello")?
    return s
}
check("? unwraps string",  testUnwrapString(),  "hello")

fn testUnwrapBool() {
    b = okTuple(true)?
    return b
}
check("? unwraps bool",  testUnwrapBool(),  true)

// ── Propagation on error ──────────────────────────────────────────────────────
// When ? fires, it returns the error from the enclosing function via ReturnValue.
// applyFunction unwraps ReturnValue → safe() receives the error as the return value.
// safe() wraps it as (val=error, err=null) since it came back as a clean return.
// Check: type(val) == "ERROR" to detect the propagated user error.

fn innerFails() {
    _ = errTuple("boom")?
    return "should not reach"
}

fn outerCatches() {
    val, err = safe(innerFails)    // no extra args — innerFails takes 0 params
    if type(val) == "ERROR" { return val.message }
    if err != null { return err.message }
    return val
}
check("? propagates error out of function",  outerCatches(),  "boom")

// ── Propagation preserves error code ─────────────────────────────────────────

fn innerWithCode() {
    _ = errTuple("code check")?
    return "unreachable"
}

fn outerCheckCode() {
    val, err = safe(innerWithCode)
    if type(val) == "ERROR" { return val.code }
    if err != null { return err.code }
    return "no error"
}
check("? preserves error code",  outerCheckCode(),  "TEST_ERR")

// ── Short-circuit: code after ? does not run ──────────────────────────────────

sideEffect = 0

fn testNoSideEffect() {
    _ = errTuple("stop")?
    sideEffect = 999
    return "unreachable"
}

safe(testNoSideEffect)
check("? short-circuits — code after ? does not run",  sideEffect,  0)

// ── Chained ? calls ───────────────────────────────────────────────────────────

fn step1()  { return "s1", null }
fn step2(v) { return v + "_s2", null }
fn step3(v) { return v + "_s3", null }

fn chainedSuccess() {
    a = step1()?
    b = step2(a)?
    return step3(b)?
}
check("chained ? all succeed",  chainedSuccess(),  "s1_s2_s3")

fn step2Fails(v) { return null, error("STEP2_FAIL", "step 2 blew up") }

fn chainedStopsEarly() {
    val, err = safe(fn() {
        a = step1()?
        b = step2Fails(a)?
        return step3(b)?
    })
    if type(val) == "ERROR" { return val.code }
    if err != null { return err.code }
    return "no error"
}
check("chained ? stops at failing step",  chainedStopsEarly(),  "STEP2_FAIL")

// ── TypeError on non-tuple — safe() catches it in err slot ───────────────────

fn unwrapNonTuple() {
    val, err = safe(fn() {
        _ = 42?
        return "unreachable"
    })
    if err != null { return err.code }
    return "no error"
}
check("? on non-tuple is TYPE_ERROR",  unwrapNonTuple(),  "TYPE_ERROR")

// ── Real-world pattern: fake filesystem ──────────────────────────────────────

fn fakeRead(path) {
    if path == "good" { return "file contents", null }
    return null, error("READ_FAIL", "file not found: " + path)
}

fn fakeProcess(contents) {
    if len(contents) > 0 { return len(contents), null }
    return null, error("PROCESS_FAIL", "empty contents")
}

fn pipeline(path) {
    contents = fakeRead(path)?
    size     = fakeProcess(contents)?
    return size
}

check("? pipeline succeeds",  pipeline("good"),  13)   // "file contents" = 13 chars

fn pipelineFails() {
    val, err = safe(fn() { return pipeline("bad") })
    if type(val) == "ERROR" { return val.code }
    if err != null { return err.code }
    return "no error"
}
check("? pipeline propagates read error",  pipelineFails(),  "READ_FAIL")

// ── Summary ───────────────────────────────────────────────────────────────────

println("unwrap (?): " + str(passed) + " passed, " + str(failed) + " failed")
