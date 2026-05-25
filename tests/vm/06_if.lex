// Tier 6 — if / else with back-patched jump offsets.

// Single-arm if
if true {
    println("then-branch fired")
}
if false {
    println("should NOT print")
}

// if / else
if 1 < 2 {
    println("1 < 2 → then")
} else {
    println("should NOT print")
}

if 1 > 2 {
    println("should NOT print")
} else {
    println("1 > 2 → else")
}

// Nested if
let x = 7
if x > 5 {
    if x < 10 {
        println("5 < x < 10")
    } else {
        println("x >= 10")
    }
} else {
    println("x <= 5")
}

// if-else chains via else { if ... }
let n = 2
if n == 1 {
    println("one")
} else {
    if n == 2 {
        println("two")
    } else {
        println("other")
    }
}
