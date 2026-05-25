// Tier 18 — struct declarations + field access + mutation.
// Methods are deferred to a follow-up (the tree-walker's method
// dispatch needs a different compile pass; field-only structs
// already cover a lot of real code).

struct Point {
    x, y
}

let p = Point { x: 3, y: 4 }
println(p.x)
println(p.y)

// Mutation
p.x = 100
println(p.x)
println(p.y)

// Multi-instance with different values
let a = Point { x: 1, y: 2 }
let b = Point { x: 9, y: 8 }
println(a.x)
println(b.x)
println(a.y)
println(b.y)

// Nested struct field — store + read
struct Box {
    origin, size
}
let box = Box { origin: Point { x: 0, y: 0 }, size: 10 }
println(box.size)
println(box.origin.x)

// Mutation through dot chain
box.origin.x = 42
println(box.origin.x)

// Struct used as function arg + return
fn shift(pt, dx) {
    pt.x = pt.x + dx
    return pt
}
let q = Point { x: 5, y: 5 }
shift(q, 10)
println(q.x)
