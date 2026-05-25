// Tier 9 — break and continue inside while loops.
//
// Verifies the per-loop break-patch list resolves correctly, and
// that continue jumps land on the loop's condition check (not the
// body's first instruction).

// break — exit early
let i = 0
while true {
    if i == 3 {
        break
    }
    println(i)
    i = i + 1
}
println("after break — i =")
println(i)

// continue — skip to next iteration
let j = 0
while j < 10 {
    j = j + 1
    if j == 3 {
        continue
    }
    if j == 7 {
        continue
    }
    println(j)
}

// Nested: break in inner loop only exits inner
let outer = 0
while outer < 3 {
    let inner = 0
    while inner < 5 {
        if inner == 2 {
            break
        }
        println(outer * 10 + inner)
        inner = inner + 1
    }
    outer = outer + 1
}
println("nested-break done")
