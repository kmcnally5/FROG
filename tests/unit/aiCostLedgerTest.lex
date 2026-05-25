// aiCostLedgerTest.lex — exercises stdlib/ai/ai_common.lex cost ledger.
//
// Network-free: drives the ledger directly via ai._record() so the test
// runs deterministically without an API key. Provider call sites use
// the same _record() path, so passing here proves the wiring shape.

import "stdlib/ai/ai_common.lex" as ai


fn assertEq(label, got, want) {
    if got != want {
        println("FAIL " + label + ": got " + str(got) + " want " + str(want))
        exit(1)
    }
}


fn approxEq(label, got, want, tol) {
    let diff = got - want
    if diff < 0.0 { diff = 0.0 - diff }
    if diff > tol {
        println("FAIL " + label + ": got " + str(got) + " want " + str(want) +
                " (tol " + str(tol) + ")")
        exit(1)
    }
}


// Clean slate before each section so prior runs in the same process
// (REPL, eval suite) don't leak state.
ai.resetUsage()
ai.budget(null)

// ── empty ledger ─────────────────────────────────────────────────────────
assertEq("spent() starts at 0",       ai.spent(),           0.0)
assertEq("budgetExceeded() default",  ai.budgetExceeded(),  false)
assertEq("usage() starts empty",      len(keys(ai.usage())), 0)


// ── single record at known rates ────────────────────────────────────────
// Sonnet rates: $3/M input, $15/M output. 1000 in + 500 out = $0.003 + $0.0075 = $0.0105
ai._record("claude-sonnet-4-6", 1000, 500, 3.0, 15.0)
approxEq("single record updates spent", ai.spent(), 0.0105, 0.00001)
let u = ai.usage()
assertEq("usage() now has 1 model", len(keys(u)), 1)
let sonnet = u["claude-sonnet-4-6"]
assertEq("input_tokens recorded",  sonnet["in_tokens"],  1000)
assertEq("output_tokens recorded", sonnet["out_tokens"], 500)


// ── accumulation across calls ───────────────────────────────────────────
ai._record("claude-sonnet-4-6", 2000, 1000, 3.0, 15.0)
sonnet = ai.usage()["claude-sonnet-4-6"]
assertEq("tokens accumulate (in)",  sonnet["in_tokens"],  3000)
assertEq("tokens accumulate (out)", sonnet["out_tokens"], 1500)
// Total spend so far: ($0.003 + $0.0075) + ($0.006 + $0.015) = $0.0315
approxEq("spent accumulates", ai.spent(), 0.0315, 0.00001)


// ── multiple models keep separate buckets ───────────────────────────────
ai._record("claude-haiku-4-5", 5000, 2000, 1.0, 5.0)
u = ai.usage()
assertEq("two models tracked", len(keys(u)), 2)
let haiku = u["claude-haiku-4-5"]
assertEq("haiku in_tokens isolated",  haiku["in_tokens"],  5000)
assertEq("haiku out_tokens isolated", haiku["out_tokens"], 2000)


// ── free providers (Ollama) record tokens but not spend ─────────────────
let spentBefore = ai.spent()
ai._record("llama3.2", 1000, 1000, 0.0, 0.0)
approxEq("local-model spend stays flat", ai.spent(), spentBefore, 0.00001)
let local = ai.usage()["llama3.2"]
assertEq("local-model tokens still recorded", local["in_tokens"], 1000)


// ── budget enforcement ──────────────────────────────────────────────────
ai.resetUsage()
ai.budget(0.05)   // 5¢ cap

assertEq("under cap → not exceeded", ai.budgetExceeded(), false)

// Record up to the cap.
ai._record("claude-sonnet-4-6", 10000, 2000, 3.0, 15.0)  // $0.03 + $0.03 = $0.06
assertEq("over cap → exceeded", ai.budgetExceeded(), true)

// Budget error carries the right code so callers can match on it.
let e = ai._budgetError()
assertEq("budget error code", e.code, "BUDGET_EXCEEDED")


// ── resetUsage clears spend, preserves cap ──────────────────────────────
ai.resetUsage()
assertEq("reset zeroes spent", ai.spent(), 0.0)
assertEq("reset preserves cap → still under", ai.budgetExceeded(), false)


// ── clearing the cap ────────────────────────────────────────────────────
ai.budget(null)
ai._record("claude-opus-4-7", 100000, 50000, 15.0, 75.0)  // $1.50 + $3.75 = $5.25
assertEq("no cap → never exceeded", ai.budgetExceeded(), false)
approxEq("spent reflects opus call", ai.spent(), 5.25, 0.001)


// ── negative / non-numeric cap is treated as 'unlimited' ────────────────
ai.budget(-1)
assertEq("negative cap clears", ai.budgetExceeded(), false)
ai.budget("oops")
assertEq("non-numeric cap clears", ai.budgetExceeded(), false)


println("PASS aiCostLedgerTest")
