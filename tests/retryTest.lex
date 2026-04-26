import "retry.lex" as retry

// --- do: succeeds on first try ---
result, err = retry.do(fn() { return 42, null }, 3)
println(err == null)    // true
println(result)         // 42

// --- do: succeeds after failures ---
attempts = 0
result, err = retry.do(fn() {
    attempts = attempts + 1
    if attempts < 3 { return null, "not yet" }
    return "done", null
}, 5)
println(err == null)    // true
println(result)         // done
println(attempts)       // 3

// --- do: exhausts all attempts ---
result, err = retry.do(fn() { return null, "always fails" }, 3)
println(err != null)    // true
println(err)            // always fails
println(result == null) // true

// --- doWithBackoff: succeeds after failures with delay=0 ---
n = 0
result, err = retry.doWithBackoff(fn() {
    n = n + 1
    if n < 2 { return null, "fail" }
    return "ok", null
}, 4, 0)
println(err == null)    // true
println(result)         // ok
println(n)              // 2

// --- doWithBackoff: exhausts attempts ---
result, err = retry.doWithBackoff(fn() { return null, "nope" }, 2, 0)
println(err)            // nope
println(result == null) // true
