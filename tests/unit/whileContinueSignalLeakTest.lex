// whileContinueSignalLeakTest.lex — locks OFI #18.
//
// The OFI was originally logged as a perf bug ("bare assignment via
// Assign() becomes pathologically slow after deep recursion") with the
// workaround being to use `let` instead of bare assignment. The actual
// cause turned out to be a signal-leak in WhileStmt: when the inner
// while's final iteration consumed a `continue`, the body-stmt loop
// `break`'d with result still set to *ContinueSignal{}. On the next
// condition check the loop exited via `cond == false`, and the
// WhileStmt's tail `return result` leaked that stale continue out to
// the enclosing while — which read it as its own continue and skipped
// its counter increment. Infinite loop.
//
// Why `let` "fixed" it: it changed which statement was last in the inner
// body and so where the stale signal would resurface — but the bug was
// always there, not in Assign().
//
// Fix: consume the continue by clearing `result` to NULL when the body
// loop breaks on it. Signal belongs to THIS loop only.

let failures = 0

fn check(name, cond) {
    if cond {
        println("ok: " + name)
    } else {
        println("FAIL: " + name)
        failures = failures + 1
    }
}

// 1. The minimal pathological pattern. Pre-fix this never terminated:
//    inner while's last iter hit continue, signal leaked to outer while,
//    outer skipped `i = i + 1`, repeat forever.
fn nestedContinue(trials, n) {
    let i = 0
    let iters = 0
    while i < trials {
        let j = 0
        while j < n {
            j = j + 1
            if j % 2 == 0 { continue }
            iters = iters + 1
        }
        i = i + 1
    }
    return iters
}

// trials=5, n=100 — for n=100, half (50) of inner iterations bump iters;
// over 5 outer iterations we expect 250.
let got = nestedContinue(5, 100)
check("nestedContinue(5,100) terminates and returns 250", got == 250)

// Also confirm a larger scale terminates in reasonable time.
got = nestedContinue(50, 200)   // 50 * 100 = 5000
check("nestedContinue(50,200) returns 5000", got == 5000)

// 2. continue is the LAST statement of the inner loop's last iter
//    (continue on even j; last j=n is even when n is even). This is
//    the exact shape that triggered the signal leak.
fn endOnContinue(n) {
    let j = 0
    let count = 0
    while j < n {
        j = j + 1
        if j % 2 == 0 {
            continue   // final iter hits this when n is even
        }
        count = count + 1
    }
    return count
}
check("endOnContinue(10) returns 5", endOnContinue(10) == 5)
check("endOnContinue(100) returns 50", endOnContinue(100) == 50)

// 3. The leaked-continue affecting an OUTER while: outer body increments
//    a counter after the inner loop. If the leak revives, the counter
//    is skipped on iterations where the inner loop ended on continue.
fn outerCounterIntegrity(outerN, innerN) {
    let o = 0
    let bumps = 0
    while o < outerN {
        let j = 0
        while j < innerN {
            j = j + 1
            if j % 2 == 0 { continue }
        }
        // This bump MUST run every outer iteration. Pre-fix it was
        // skipped because the inner while leaked a *ContinueSignal{}
        // and the outer body-stmt loop treated it as a continue.
        bumps = bumps + 1
        o = o + 1
    }
    return bumps
}
check("outer counter runs on every iter (5x100)",   outerCounterIntegrity(5, 100)   == 5)
check("outer counter runs on every iter (50x200)",  outerCounterIntegrity(50, 200)  == 50)

// 4. continue inside an `if` (the original repro) still works as a
//    plain control-flow primitive within a single loop.
fn singleLoopContinue(n) {
    let j = 0
    let odds = 0
    let evens = 0
    while j < n {
        j = j + 1
        if j % 2 == 0 {
            evens = evens + 1
            continue
        }
        odds = odds + 1
    }
    return odds == n / 2 && evens == n / 2
}
check("single-loop continue counts correctly", singleLoopContinue(100))

// 5. break still exits cleanly (regression guard — break also returns
//    NULL via the existing isBreak handler).
fn breakWorks(n) {
    let j = 0
    let hits = 0
    while j < n {
        j = j + 1
        if j == 5 { break }
        hits = hits + 1
    }
    return hits == 4
}
check("break exits while at j==5 after 4 hits", breakWorks(100))

// 6. continue inside a deeply nested triple while — the signal must be
//    consumed by the innermost loop and not leak through two levels.
fn tripleNested() {
    let i = 0
    let total = 0
    while i < 3 {
        let j = 0
        while j < 3 {
            let k = 0
            while k < 4 {
                k = k + 1
                if k % 2 == 0 { continue }
                total = total + 1
            }
            j = j + 1
        }
        i = i + 1
    }
    return total
}
// k loop: 4 iters, half odd → 2 bumps per j iter
// 3 j iters per i iter → 6 bumps
// 3 i iters → 18 total
check("triple-nested continue terminates with correct total", tripleNested() == 18)

if failures > 0 {
    println("FAILURES: " + str(failures))
    _osExit(1)
}
println("PASS — WhileStmt consumes its own continue; no signal leak")
