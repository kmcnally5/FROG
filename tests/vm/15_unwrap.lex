// Tier 15 — postfix `?` error-propagation operator.
//
// kLex tuples are only formed via `return a, b` (or multi-assign
// RHS), so every test constructs its 2-tuple via a helper function
// and applies `?` at top level. Top-level cross-function calls
// work without closures because the CallExpr's callee is a
// top-level local; the closure limitation only blocks references
// from INSIDE a function body to an outer-scope name.

fn ok2() {
    return 42, null
}
fn fail2() {
    return null, "boom"
}

// Success path: ? unwraps the value
let v = ok2()?
println(v)

// `?` at top level with an error halts the program, mirroring how
// `return err` from a function exits the function. Demonstrating
// the success path is enough; the failure-mid-program case would
// halt before any later assertion runs, which is exactly what the
// tree-walker does too.

// Use in a more realistic expression — ? lifts the value cleanly.
let sum = ok2()? + ok2()?
println(sum)

// Mixed: explicit destructure vs `?` should agree
let a, e = ok2()
println(a)
println(e)
