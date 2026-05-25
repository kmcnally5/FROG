// Tier 12 — for-in over arrays + strings (the indexable shapes).
// Hashes deferred to a separate iteration tier (needs keys() lowering).

// Single-var: bind element
let arr = [10, 20, 30]
for x in arr {
    println(x)
}

// Two-var: bind index + element
for i, v in arr {
    println(i)
    println(v)
}

// Empty array — body never runs
for x in [] {
    println("should NOT print")
}
println("after empty-for")

// Iterating with break
for x in [1, 2, 3, 4, 5] {
    if x == 3 {
        break
    }
    println(x)
}

// Iterating with continue
for x in [1, 2, 3, 4, 5] {
    if x % 2 == 0 {
        continue
    }
    println(x)
}

// Nested for-in
for r in [0, 1, 2] {
    for c in [0, 1, 2] {
        println(r * 10 + c)
    }
}
