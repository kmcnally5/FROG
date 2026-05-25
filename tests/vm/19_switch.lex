// Tier 19 — switch (value form + expression form).
// Enum-pattern cases are deferred until the enum milestone.

// Value switch — single value per case
let x = 2
switch x {
    case 1 { println("one") }
    case 2 { println("two") }
    case 3 { println("three") }
}

// Multiple values per case
fn classify(n) {
    switch n {
        case 1, 2, 3 { return "small" }
        case 4, 5, 6 { return "mid"   }
        case 7, 8, 9 { return "big"   }
    }
    return "unknown"
}
println(classify(2))
println(classify(5))
println(classify(8))
println(classify(0))

// Default clause
fn day(n) {
    switch n {
        case 1 { return "mon" }
        case 2 { return "tue" }
        case 3 { return "wed" }
        default { return "weekend?" }
    }
}
println(day(1))
println(day(3))
println(day(99))

// Expression switch (no subject — each case is a bool expr)
let y = 42
switch {
    case y < 0  { println("negative") }
    case y == 0 { println("zero") }
    case y < 10 { println("single-digit") }
    case y < 100 { println("two-digit") }
    default { println("large") }
}

// First match wins — no fallthrough
fn first(n) {
    switch n {
        case 0, 1, 2 { return "low" }
        case 2, 3, 4 { return "mid" }
    }
    return "miss"
}
println(first(2))
println(first(3))

// String subject
let animal = "dog"
switch animal {
    case "cat" { println("meow") }
    case "dog" { println("woof") }
    case "cow" { println("moo") }
}
