// Tier 21 — enum pattern matching in switch.
//
// Short form:  case Circle(r)       — match variant name only
// Full form:   case Shape.Circle(r) — match type + variant name
// Zero-field:  case Origin          — short form for empty variants

enum Shape {
    Circle(radius)
    Rect(w, h)
    Origin
}

fn area(s) {
    switch s {
        case Circle(r) {
            return r * r * 3
        }
        case Rect(w, h) {
            return w * h
        }
        case Origin() {
            return 0
        }
    }
    return 0 - 1
}

println(area(Shape.Circle(5)))
println(area(Shape.Rect(4, 6)))
println(area(Shape.Origin))

// Full form: case qualified by type
fn describe(s) {
    switch s {
        case Shape.Circle(r) { return "circle of " + str(r) }
        case Shape.Rect(w, h) { return "rect " + str(w) + "x" + str(h) }
        case Shape.Origin() { return "origin" }
    }
    return "unknown"
}
println(describe(Shape.Circle(7)))
println(describe(Shape.Rect(3, 4)))
println(describe(Shape.Origin))

// Bindings are scoped to the case body
fn ringArea(s) {
    switch s {
        case Circle(r) {
            // r is bound here
            return r * r * 3
        }
        case Rect(w, h) {
            // w and h are bound here, NOT r
            return w * h
        }
    }
    return 0
}
println(ringArea(Shape.Circle(10)))

// Discard binding with _
enum Maybe {
    Some(value)
    None
}

fn unwrapOrZero(m) {
    switch m {
        case Some(v) { return v }
        case None()  { return 0 }
    }
    return 0 - 1
}
println(unwrapOrZero(Maybe.Some(42)))
println(unwrapOrZero(Maybe.None))
