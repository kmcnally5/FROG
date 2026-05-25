// Tier 20 — enum declaration + variant access + variant construction.
// Pattern-matching in switch is the NEXT tier (tier 21).

enum Shape {
    Circle(radius)
    Rect(w, h)
    Origin
}

// Zero-field variant — usable directly
let o = Shape.Origin
println(o)

// Data-carrying variant — needs to be called to construct
let c = Shape.Circle(5)
println(c)
println(c.radius)

let r = Shape.Rect(10, 20)
println(r.w)
println(r.h)

// Variants are first-class values — store, pass, etc.
fn area(s) {
    // Without pattern-matching yet, use field access only.
    // (Real-world code would use `switch s { case Circle(r) ... }`.)
    return s.radius * s.radius * 314
}
println(area(c))

// Multiple instances independent
let c2 = Shape.Circle(10)
println(c.radius)
println(c2.radius)
