// Tier 2 — assignment + identifier load.
//
// Exercises AssignStmt (bare =), LetStmt (let), the local-slot
// allocator, and LoadLocal / StoreLocal handlers in vm.go.

let x = 5
println(x)

let y = "kLex"
println(y)

let z = 999
println(z)

// Reassignment to an existing slot — mutates in place via StoreLocal,
// no new slot allocated.
x = 100
println(x)
