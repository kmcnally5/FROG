// enumDestructureTest.lex — enum switch destructuring (short form + full form)

passed = 0
failed = 0

fn check(name, got, expected) {
    if got == expected {
        passed = passed + 1
    } else {
        failed = failed + 1
        println("FAIL: " + name + " — expected " + str(expected) + " got " + str(got))
    }
}

// ── Enum definitions ──────────────────────────────────────────────────────────

enum Shape {
    Circle(radius)
    Rect(w, h)
    Triangle(base, height)
    Point
}

enum Result {
    Ok(value)
    Err(code, message)
}

// ── Short form: case Variant(bindings) ───────────────────────────────────────

fn describeShape(s) {
    switch s {
        case Circle(r)       { return "circle r=" + str(r) }
        case Rect(w, h)      { return str(w) + "x" + str(h) }
        case Triangle(b, ht) { return "tri b=" + str(b) + " h=" + str(ht) }
        case Point()         { return "point" }    // zero-field: use empty parens for short form
    }
    return "unknown"
}

check("Circle short form",   describeShape(Shape.Circle(5.0)),       "circle r=5")
check("Rect short form",     describeShape(Shape.Rect(10, 20)),      "10x20")
check("Triangle short form", describeShape(Shape.Triangle(6, 4)),    "tri b=6 h=4")
check("Point (no fields)",   describeShape(Shape.Point),             "point")

// ── Bindings are scoped to the case body ─────────────────────────────────────

r = "outer"
switch Shape.Circle(99.0) {
    case Circle(r) { r = "inner " + str(r) }
}
check("binding scoped to case body", r, "outer")

// ── Full form still works: case Type.Variant(bindings) ───────────────────────

fn describeShapeFull(s) {
    switch s {
        case Shape.Circle(r)       { return "circle r=" + str(r) }
        case Shape.Rect(w, h)      { return str(w) + "x" + str(h) }
        case Shape.Point           { return "point" }
    }
    return "unknown"
}

check("Circle full form",  describeShapeFull(Shape.Circle(3.0)),   "circle r=3")
check("Rect full form",    describeShapeFull(Shape.Rect(4, 5)),    "4x5")
check("Point full form",   describeShapeFull(Shape.Point),         "point")

// ── Multi-field variant ───────────────────────────────────────────────────────

fn area(s) {
    switch s {
        case Rect(w, h)      { return w * h }
        case Triangle(b, ht) { return b * ht / 2 }
        case Circle(r)       { return int(3.14159 * r * r) }
    }
    return 0
}

check("area rect",     area(Shape.Rect(6, 4)),       24)
check("area triangle", area(Shape.Triangle(10, 6)),  30)
check("area circle",   area(Shape.Circle(5.0)),      78)

// ── Result type pattern ───────────────────────────────────────────────────────

fn handleResult(res) {
    switch res {
        case Ok(v)        { return "ok: " + str(v) }
        case Err(c, msg)  { return "err " + c + ": " + msg }
    }
    return "unknown"
}

check("Ok short form",  handleResult(Result.Ok(42)),                    "ok: 42")
check("Err short form", handleResult(Result.Err("NOT_FOUND", "gone")),  "err NOT_FOUND: gone")

// ── Bindings usable in expressions ───────────────────────────────────────────

fn doubled(s) {
    switch s {
        case Circle(r) { return r * 2.0 }
    }
    return 0.0
}

check("binding in expr", doubled(Shape.Circle(7.5)), 15.0)

// ── Wrong binding count gives clear error ────────────────────────────────────

fn wrongCount() {
    val, err = safe(fn() {
        switch Shape.Rect(3, 4) {
            case Rect(w) { return w }   // Rect has 2 fields, only 1 bound
        }
        return 0
    })
    if err != null { return err.message }
    return "no error"
}

result = wrongCount()
check("wrong binding count errors", indexOf(result, "has 2 field") >= 0, true)

// ── Summary ───────────────────────────────────────────────────────────────────

println("enum destructure: " + str(passed) + " passed, " + str(failed) + " failed")
