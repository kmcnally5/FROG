// Tier 14 — pipe operator |>
//
// Lowers to the existing CallExpr/CallBuiltin path with the piped
// value prepended as arg 0.

// Bare reference — pipe to a single-arg builtin
println("  hi  " |> trim)

// CallExpr on the right — extra args
println("hello" |> substr(1, 4))

// Chained pipes — left-associative per parser docs
println("HELLO" |> lower |> substr(1, 3))

// User-defined function on the right
fn double(x) {
    return x * 2
}
println(7 |> double)

// User function with extra args
fn add(a, b) {
    return a + b
}
println(3 |> add(4))

// Multi-step pipeline through user fns
fn inc(n) { return n + 1 }
fn triple(n) { return n * 3 }
println(5 |> inc |> triple)
