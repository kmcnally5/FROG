// discardTest.lex — tests for the _ formal discard identifier

// Multi-assign discard — ignore the error
val, _ = safe(fn() { return 42 })
println(val)

// Reversed — only care about the error
fn alwaysFails() { return 1/0 }
_, err = safe(alwaysFails)
println(err == null)   // false — error was captured

// Multiple discards in the same scope do not conflict
a, _ = safe(fn() { return 1 })
b, _ = safe(fn() { return 2 })
println(a)
println(b)

// Reading _ is always a RuntimeError
result, err = safe(fn() { return _ })
println(err.message)

// Single-assign discard — side effects run, value not stored
count = 0
fn sideEffect() { count = count + 1  return 99 }
_ = sideEffect()
println(count)   // 1 — side effect ran
result, err = safe(fn() { return _ })
println(err.message)

// for-in with element discard — iterate without binding
total = 0
for _ in [1, 2, 3] { total = total + 1 }
println(total)   // 3

// for k, _ in hash — only care about keys
h = {"a": 1, "b": 2, "c": 3}
keys = []
for k, _ in h { keys = push(keys, k) }
println(len(keys))   // 3

// _ is never stored — cannot be read back even after assignment
_ = 100
result, err = safe(fn() { return _ })
println(err.message)
