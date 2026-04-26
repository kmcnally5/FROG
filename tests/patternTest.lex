enum Shape {
    Circle(radius)
    Rect(width, height)
    Point
}

fn describe(s) {
    switch s {
        case Shape.Circle(r) {
            return "circle with radius " + str(r)
        }
        case Shape.Rect(w, h) {
            return "rect " + str(w) + "x" + str(h)
        }
        case Shape.Point {
            return "point"
        }
        default {
            return "unknown"
        }
    }
}

println(describe(Shape.Circle(5)))
println(describe(Shape.Rect(10, 3)))
println(describe(Shape.Point))

// bindings are local to the arm
r = "outer"
switch Shape.Circle(99) {
    case Shape.Circle(r) {
        println("inner r = " + str(r))
    }
}
println("outer r = " + r)

// wrong binding count is a runtime error — caught with safe() so the test keeps running
fn badBindingCount() {
    switch Shape.Circle(7) {
        case Shape.Circle(a, b) {
            return "matched"
        }
        default {
            return "default"
        }
    }
}
val, err = safe(badBindingCount)
if err == null {
    println("ERROR: expected runtime error for wrong binding count, got none")
} else {
    println("correct: wrong binding count produced error: " + err.message)
}
