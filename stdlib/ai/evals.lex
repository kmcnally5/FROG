// evals.lex — AI eval framework for kLex.
// @module    evals
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   AI eval framework for kLex.
//
// Run a model against a set of test cases, score the outputs, and get a
// pass/fail/score summary. Built on kLex's native async + channel
// primitives — case execution is parallel across N workers without
// blocking the caller.
//
// Usage:
//   import "stdlib/ai/evals.lex"    as evals
//   import "stdlib/ai/ollama.lex"   as ollama
//   import "stdlib/ai/ai_common.lex" as ai
//
//   cases = [
//       {"input": "What's 2+2?",            "expected": "4"},
//       {"input": "Capital of France?",     "expected": "Paris"},
//       {"input": "When did WW2 end?",      "expected": "1945"},
//   ]
//
//   results = evals.run(cases, fn(case) {
//       resp, _ = ollama.chat(c, {"messages": [ai.userMsg(case["input"])]})
//       return ollama.textOf(resp)
//   }, evals.contains, {"workers": 4})
//
//   evals.report(results)
//
// Each result hash contains:
//   {idx, case, actual, score, pass, error, duration_ms}
//
// Scoring functions take (actual_output, case_hash) and return either:
//   - bool                → pass/fail directly
//   - float in [0.0, 1.0] → partial-credit score; >= passThreshold passes
//   - error               → treated as a failure with the error attached

import "stdlib/datetime.lex" as dt


// ── Parallel runner ──────────────────────────────────────────────────────

// run executes each case in parallel via a worker pool. Returns an array
// of result hashes in case order (results[i] corresponds to cases[i]).
//
// opts (all optional):
//   workers        — int, default 4. Concurrency level for case execution.
//   passThreshold  — float, default 1.0. Min score to count as a pass.
//
// runFn signature:    fn(case) -> any
// scoreFn signature:  fn(actual, case) -> bool | float
//
// Errors in either function are caught via safe() and recorded as a
// failed result with the error attached — the whole eval doesn't crash
// because one case errored.
fn run(cases, runFn, scoreFn, opts) {
    let workers = 4
    let passThreshold = 1.0
    if opts != null {
        if hasKey(opts, "workers")        { workers       = opts["workers"] }
        if hasKey(opts, "passThreshold")  { passThreshold = opts["passThreshold"] }
    }
    if workers < 1 { workers = 1 }
    let n = len(cases)
    if n == 0 { return makeArray(0) }

    let workCh   = channel(n)
    let resultCh = channel(n)

    // Enqueue all work and close the work channel so workers naturally
    // exit when the queue drains.
    let i = 0
    while i < n {
        send(workCh, {"idx": i, "case": cases[i]})
        i = i + 1
    }
    close(workCh)

    // Spawn worker pool. Each worker drains workCh until close, scoring
    // and posting one result per case.
    let w = 0
    while w < workers {
        async(fn() {
            for item in workCh {
                let startNs = dt.nowNanos()
                let actual, runErr = safe(runFn, item["case"])
                let durationMs = (dt.nowNanos() - startNs) / 1000000

                let result = {
                    "idx":         item["idx"],
                    "case":        item["case"],
                    "actual":      actual,
                    "duration_ms": durationMs,
                    "error":       runErr,
                    "score":       0.0,
                    "pass":        false,
                }
                if runErr == null {
                    let score, scoreErr = safe(scoreFn, actual, item["case"])
                    if scoreErr != null {
                        result["error"] = scoreErr
                    } else {
                        result["score"] = _normaliseScore(score)
                        result["pass"]  = result["score"] >= passThreshold
                    }
                }
                send(resultCh, result)
            }
        })
        w = w + 1
    }

    // Collect — results may arrive out of order; we place them by idx so
    // the output array mirrors the input order. Recovering input order is
    // essential for cross-eval comparison.
    let results = makeArray(n, null)
    let collected = 0
    while collected < n {
        let r, _ = recv(resultCh)
        results[r["idx"]] = r
        collected = collected + 1
    }
    return results
}


// runSerial is the sequential equivalent of run(). Same signature minus
// opts.workers. Useful when:
//   - You want deterministic ordering of side effects (e.g. logs).
//   - The runFn is rate-limited and parallelism would just queue at the API.
//   - You're debugging a flaky case.
fn runSerial(cases, runFn, scoreFn, opts) {
    let passThreshold = 1.0
    if opts != null && hasKey(opts, "passThreshold") {
        passThreshold = opts["passThreshold"]
    }
    let n = len(cases)
    let results = makeArray(n, null)
    let i = 0
    while i < n {
        let startNs = dt.nowNanos()
        let actual, runErr = safe(runFn, cases[i])
        let durationMs = (dt.nowNanos() - startNs) / 1000000
        let r = {
            "idx":         i,
            "case":        cases[i],
            "actual":      actual,
            "duration_ms": durationMs,
            "error":       runErr,
            "score":       0.0,
            "pass":        false,
        }
        if runErr == null {
            let score, scoreErr = safe(scoreFn, actual, cases[i])
            if scoreErr != null {
                r["error"] = scoreErr
            } else {
                r["score"] = _normaliseScore(score)
                r["pass"]  = r["score"] >= passThreshold
            }
        }
        results[i] = r
        i = i + 1
    }
    return results
}


// _normaliseScore coerces a scoreFn return value into a float in [0.0, 1.0].
// Accepts bool (true→1.0, false→0.0), Integer, Float. Anything else → 0.0.
fn _normaliseScore(v) {
    if type(v) == "BOOLEAN" {
        if v { return 1.0 }
        return 0.0
    }
    if type(v) == "INTEGER" { return float(v) }
    if type(v) == "FLOAT"   { return v }
    return 0.0
}


// ── Built-in scorers ─────────────────────────────────────────────────────

// IMPORTANT: parameter is `tc` (test case) — `case` is a reserved
// keyword in kLex's switch syntax and silently zero-arities the function
// if used as a parameter name.

// exactMatch — actual == tc["expected"]. Strict equality.
fn exactMatch(actual, tc) {
    return actual == tc["expected"]
}


// contains — case-insensitive substring match. tc["expected"] must be
// a string; actual is stringified if it isn't already.
fn contains(actual, tc) {
    let expected = tc["expected"]
    if type(expected) != "STRING" { return false }
    let actualStr = actual
    if type(actualStr) != "STRING" { actualStr = str(actualStr) }
    return indexOf(lower(actualStr), lower(expected)) >= 0
}


// containsAll — every string in the tc["expected"] array must appear
// in actual (case-insensitive). Useful when the answer needs to cover
// multiple keywords without a strict order.
fn containsAll(actual, tc) {
    let expected = tc["expected"]
    if type(expected) != "ARRAY" { return false }
    let actualStr = actual
    if type(actualStr) != "STRING" { actualStr = str(actualStr) }
    let al = lower(actualStr)
    for e in expected {
        if indexOf(al, lower(e)) < 0 { return false }
    }
    return true
}


// containsAny — at least one string in tc["expected"] (array) appears
// in actual. Useful when several phrasings are acceptable.
fn containsAny(actual, tc) {
    let expected = tc["expected"]
    if type(expected) != "ARRAY" { return false }
    let actualStr = actual
    if type(actualStr) != "STRING" { actualStr = str(actualStr) }
    let al = lower(actualStr)
    for e in expected {
        if indexOf(al, lower(e)) >= 0 { return true }
    }
    return false
}


// ── Reporting ────────────────────────────────────────────────────────────

// report prints a pass/fail summary to stdout. Compact for small evals
// (under 100 cases); shows the first 5 failing cases with their actual
// outputs for quick debugging.
fn report(results) {
    let n = len(results)
    println("")
    println("Eval Results")
    println("============")
    if n == 0 {
        println("No cases.")
        return
    }
    let passed = 0
    let failed = 0
    let errored = 0
    let totalMs = 0
    let failedCases = makeArray(0)
    for r in results {
        totalMs = totalMs + r["duration_ms"]
        if r["error"] != null {
            errored = errored + 1
            failedCases = concat(failedCases, [r])
        } else if r["pass"] {
            passed = passed + 1
        } else {
            failed = failed + 1
            failedCases = concat(failedCases, [r])
        }
    }
    let passRate = 100 * passed / n
    let meanMs   = totalMs / n
    println("Total:       " + str(n))
    println("Passed:      " + str(passed))
    println("Failed:      " + str(failed))
    if errored > 0 {
        println("Errored:     " + str(errored))
    }
    println("Pass rate:   " + str(passRate) + "%")
    println("Mean ms:     " + str(meanMs))

    if len(failedCases) > 0 {
        println("")
        println("Failed cases:")
        let i = 0
        let cap = 5
        if len(failedCases) < cap { cap = len(failedCases) }
        while i < cap {
            let r = failedCases[i]
            let input = ""
            if hasKey(r["case"], "input") { input = str(r["case"]["input"]) }
            if len(input) > 70 { input = substr(input, 0, 67) + "..." }
            println("  [" + str(r["idx"]) + "] " + input)
            if r["error"] != null {
                println("        error: " + r["error"].message)
            } else {
                let actual = str(r["actual"])
                if len(actual) > 70 { actual = substr(actual, 0, 67) + "..." }
                println("        actual: " + actual)
                println("        score:  " + str(r["score"]))
            }
            i = i + 1
        }
        if len(failedCases) > cap {
            println("  ... and " + str(len(failedCases) - cap) + " more")
        }
    }
}


// ── Cross-provider compare ───────────────────────────────────────────────

// compare runs the same cases through multiple named runners and returns
// a hash of {runnerName: results_array}. Use to compare providers or
// prompt variants side by side.
//
//   runners = {
//       "claude": fn(case) { ... claude.messages ... },
//       "ollama": fn(case) { ... ollama.chat ... },
//   }
//   results = evals.compare(cases, runners, evals.contains, {"workers": 4})
//   evals.compareReport(results)
fn compare(cases, runners, scoreFn, opts) {
    let out = {}
    for name, runFn in runners {
        out[name] = run(cases, runFn, scoreFn, opts)
    }
    return out
}


// compareReport prints a side-by-side scoreboard for multiple runners.
fn compareReport(compareResults) {
    println("")
    println("Eval Comparison")
    println("===============")
    for name, results in compareResults {
        let n = len(results)
        if n == 0 {
            println(name + ":  no cases")
            continue
        }
        let passed = 0
        let totalMs = 0
        for r in results {
            if r["pass"] { passed = passed + 1 }
            totalMs = totalMs + r["duration_ms"]
        }
        let rate = 100 * passed / n
        let meanMs = totalMs / n
        let nameStr = name
        while len(nameStr) < 12 { nameStr = nameStr + " " }
        println(nameStr + "  " + str(passed) + "/" + str(n) +
                "  (" + str(rate) + "%)  mean " + str(meanMs) + "ms")
    }
}
