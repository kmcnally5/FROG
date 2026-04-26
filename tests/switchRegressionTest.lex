// 1. Plain value switch — existing behaviour must be unchanged
status = 404
switch status {
    case 200 { println("ok") }
    case 404 { println("not found") }
    default  { println("other") }
}

// 2. Expression switch (no subject)
x = -5
switch {
    case x < 0  { println("negative") }
    case x == 0 { println("zero") }
    default     { println("positive") }
}

// 3. Multi-value plain arm — first match wins, no pattern
y = 3
switch y {
    case 1, 2   { println("one or two") }
    case 3, 4   { println("three or four") }
    default     { println("other") }
}

// 4. Dot-call in case with a non-identifier arg — must NOT be rewritten as EnumPattern
//    Shape.Circle(5) evaluates to an EnumInstance; the subject is also Shape.Circle(5)
//    so they compare equal via == (same TypeName, VariantName, same field values).
enum Shape { Circle(radius) Rect(width, height) Point }
s = Shape.Circle(5)
switch s {
    case Shape.Circle(5) {
        println("dot-call with literal: matched")
    }
    default {
        println("dot-call with literal: NOT matched")
    }
}

// 5. Pattern arm alongside plain arm in different cases (not mixed in same arm)
fn classifyShape(sh) {
    switch sh {
        case Shape.Circle(r) { return "circle r=" + str(r) }
        case Shape.Rect(w, h) { return "rect " + str(w) + "x" + str(h) }
        case Shape.Point { return "point" }
        default { return "?" }
    }
}
println(classifyShape(Shape.Circle(7)))
println(classifyShape(Shape.Rect(4, 9)))
println(classifyShape(Shape.Point))

// 6. Binding does not leak — verify with explicit check
radius = "before"
switch Shape.Circle(42) {
    case Shape.Circle(radius) {
        println("inside: radius=" + str(radius))
    }
}
println("outside: radius=" + radius)
