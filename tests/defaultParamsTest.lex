// defaultParamsTest.lex — tests for default parameter values

// Single default
fn greet(name = "world") {
    return "hello, " + name + "!"
}
println(greet())
println(greet("Karl"))

// Multiple defaults
fn connect(host, port = 8080, timeout = 30) {
    return host + ":" + str(port) + " t=" + str(timeout)
}
println(connect("localhost"))
println(connect("localhost", 9000))
println(connect("localhost", 9000, 60))

// Default evaluated at call time in closure env
base = 100
fn add(x, n = base) { return x + n }
println(add(5))     // 105
base = 200
println(add(5))     // 205

// Default with expression
fn padded(s, width = 2 + 3) {
    return s + str(width)
}
println(padded("w"))   // w5

// Arity error: too few required args
result, err = safe(connect)
println(err.message)

// Arity error: too many args
result, err = safe(connect, "a", 1, 2, 3, 4)
println(err.message)

// Functions without defaults still enforce strict arity
fn add2(a, b) { return a + b }
println(add2(3, 4))
result, err = safe(add2, 1)
println(err.message)

// Ordering: required after defaulted is a parse error
// (tested via safe around a string eval — can't be tested at runtime directly)

// map/filter with a callback that has defaults
fn isAbove(x, threshold = 0) { return x > threshold }
println(filter([-1, 2, -3, 4], isAbove))

// Struct method with default
struct Counter {
    n
    fn step(amount = 1) {
        self.n = self.n + amount
    }
}
c = Counter { n: 0 }
c.step()
println(c.n)    // 1
c.step(5)
println(c.n)    // 6

// Variadic still works alongside defaults on earlier params
fn logMsg(level = "INFO", msgs...) {
    return "[" + level + "] " + join(msgs, " ")
}
println(logMsg("WARN", "disk", "full"))
println(logMsg("INFO", "started"))
