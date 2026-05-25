// Tier 8 — while loop with body emitting + backward Jump.

// Simple counter
let i = 0
while i < 5 {
    println(i)
    i = i + 1
}

// Sum the first ten naturals
let n = 1
let total = 0
while n <= 10 {
    total = total + n
    n = n + 1
}
println(total)

// Nested loops
let r = 0
while r < 3 {
    let c = 0
    while c < 3 {
        println(r * 10 + c)
        c = c + 1
    }
    r = r + 1
}

// Zero iterations (condition false from the start)
while false {
    println("should NOT print")
}
println("after while-false")
