println("--- modules ---")
import "math.lex" as math
// println(math.add(3, 4))
println(math.abs(-9))
println(math.max(10, 20))
println(math.min(10, 20))
// println(math.PI)

// this is a comment — it should not affect output
println("--- unary minus ---")
println(-5)
x = 10
println(-x)
println(-x + 3)
println(-(2 * 4))

println("--- comments ---")
x = 42 // inline comment
println(x) // should print 42

println("--- equality ---")
println(1 == 1)
println("a" == "a")

x = null
println(x == null)

println("--- out of bounds ---")
arr = [1, 2, 3]
println(arr[2])

println("--- hash/map ---")
person = {"name": "Karl", "age": 42}
println(person["name"])
println(person["age"])
person["age"] = 43
println(person["age"])
println(person["missing"] == null)
delete(person, "age")
println(person["age"] == null)
k = keys(person)
println(len(k))
h = {"x": 10, "y": 20, "z": 30}
println(len(values(h)))    // 3
println(hasKey(h, "x"))    // true
println(hasKey(h, "w"))    // false
delete(h, "y")
println(hasKey(h, "y"))    // false
println(len(h))            // 2

println("--- string escapes ---")
println("hello\nworld")
println("tab\there")
println("she said \"hi\"")
println("back\\slash")

println("--- type ---")
println(type(42))
println(type("hello"))
println(type(true))
println(type(null))
println(type([1, 2]))

println("--- str and int ---")
println(str(42))
println(str(true))
n = int("99")
println(n + 1)

println("--- print ---")
print("hello ")
print("world")
println("")

println("--- split and join ---")
parts = split("one,two,three", ",")
println(len(parts))
println(parts[1])
println(join(parts, " - "))

println("--- pop ---")
arr = [1, 2, 3, 4]
arr2 = pop(arr)
println(len(arr))
println(len(arr2))
println(arr2[2])

println("--- filter ---")
nums = [1, 2, 3, 4, 5, 6]
evens = filter(nums, fn(x) { x % 2 == 0 })
println(len(evens))
println(evens[0])
println(evens[2])

println("--- reduce ---")
total = reduce([1, 2, 3, 4, 5], fn(acc, x) { acc + x }, 0)
println(total)
product = reduce([1, 2, 3, 4], fn(acc, x) { acc * x }, 1)
println(product)

println("--- map ---")
nums = [1, 2, 3, 4, 5]
doubled = map(nums, fn(x) { x * 2 })
println(doubled[0])
println(doubled[4])
squares = map(nums, fn(x) { x * x })
println(squares[2])

println("--- multiple return values ---")
fn divide(a, b) {
    if b == 0 {
        return null, "division by zero"
    }
    return a / b, null
}
val, err = divide(10, 2)
println(val)
println(err == null)
val, err = divide(10, 0)
println(val == null)
println(err)

fn minmax(arr) {
    lo = arr[0]
    hi = arr[0]
    for x in arr {
        if x < lo { lo = x }
        if x > hi { hi = x }
    }
    return lo, hi
}
lo, hi = minmax([3, 1, 4, 1, 5, 9, 2, 6])
println(lo)
println(hi)

println("--- string builtins ---")
println(replace("hello world", "world", "kLex"))   // hello kLex
println(indexOf("hello", "ll"))                     // 2
println(indexOf("hello", "x"))                      // -1
println(startsWith("hello", "he"))                  // true
println(startsWith("hello", "lo"))                  // false
println(endsWith("hello", "lo"))                    // true
println(endsWith("hello", "he"))                    // false
println(indexOf("café", "fé"))                      // 2  (unicode-aware)

println("--- string concatenation ---")
println("hello" + " " + "world")
name = "Karl"
println("Hello, " + name + "!")
println("count: " + str(42))

println("--- modulo ---")
println(10 % 3)
println(9 % 3)
println(7 % 2)
evens = filter([1, 2, 3, 4, 5, 6], fn(x) { x % 2 == 0 })
println(len(evens))

println("--- comparison operators ---")
println(3 <= 3)
println(3 <= 4)
println(4 <= 3)
println(5 >= 5)
println(5 >= 4)
println(4 >= 5)

println("--- for in ---")
for x in [10, 20, 30] {
    println(x)
}
names = ["Alice", "Bob", "Carol"]
for name in names {
    println(name)
}
for k in keys({"a": 1, "b": 2}) {
    println(k)
}

println("--- for k v in hash ---")
totals = {"apples": 3, "bananas": 5}
sum = 0
for fruit, count in totals {
    sum = sum + count
}
println(sum)     // 8

println("--- for i v in array ---")
letters = ["x", "y", "z"]
for idx, val in letters {
    println(str(idx) + ":" + val)
}

println("--- floats ---")
println(3.14)
println(type(3.14))
println(type(3))
println(1.5 + 1.5)
println(10.0 / 4.0)
println(3.0 - 1.5)
println(2.0 * 2.5)
// int + float → float
println(1 + 0.5)
println(10 / 4)
println(10.0 / 4)
// unary minus
println(-3.14)
// float() conversion
println(float(3))
println(float("2.5"))
// comparison with mixed types
println(1.5 < 2.0)
println(3.0 > 2)
println(2.0 == 2.0)
println(2.0 == 2.5)
// int() conversion from float (truncates toward zero)
println(int(3.9))
println(int(3.1))
println(int(-3.9))

println("--- string interpolation ---")
name = "Karl"
age = 42
println("Hello, {name}!")
println("Age: {age}")
println("Sum: {1 + 2}")
x = 10
println("Double: {x * 2}")
fn double(n) { n * 2 }
println("Result: {double(5)}")
println("literal brace: \{not interpolated}")
pi = 3.14
println("Pi is {pi}")
arr = [1, 2, 3]
println("Length: {len(arr)}")
println("Multipart: {name} is {age} years old")

println("--- short-circuit ---")
// && must not evaluate the right side when left is false
println(false && false)
println(false && true)
println(true && false)
println(true && true)
// || must not evaluate the right side when left is true
println(false || false)
println(false || true)
println(true || false)
println(true || true)
// real short-circuit test: right side is an undefined variable
// if evaluation reaches it, this produces a runtime error
println(false && undeclaredShortCircuit)
println(true || undeclaredShortCircuit)

println("--- utf8 string indexing ---")
s = "café"
println(len(s))
println(s[0])
println(s[3])
s2 = "hello"
println(len(s2))
println(s2[0])
println(s2[4])

println("--- loops ---")
i = 0
while i < 10 {
    println(i)
    i = i + 1
}

neg = -44
println(neg)

println("--- while continue ---")
i = 0
evens = []
while i < 10 {
    i = i + 1
    if (i % 2 != 0) { continue }
    evens = push(evens, i)
}
println(len(evens))   // 5
println(evens[0])     // 2
println(evens[4])     // 10

println("--- self ---")
struct SelfCounter {
    value
    fn inc() { self.value = self.value + 1 }
    fn get() { return self.value }
}
sc = SelfCounter { value: 0 }
sc.inc()
sc.inc()
sc.inc()
println(sc.get())   // 3

struct SelfRect {
    w, h
    fn area()   { return self.w * self.h }
    fn scale(f) { self.w = self.w * f   self.h = self.h * f }
}
sr = SelfRect { w: 4, h: 5 }
println(sr.area())   // 20
sr.scale(2)
println(sr.area())   // 80

struct SelfObj {
    x
    fn double()  { return self.x * 2 }
    fn quad()    { return self.double() * 2 }
}
so = SelfObj { x: 10 }
println(so.quad())   // 40


println(math.min(3, 4))

fn WriteProgress(arr) {
    for a in arr {
        print(".")
    }
    
    println("Complete!")
}

names2 = ["Alice", "Bob", "Carol"]

WriteProgress(names2)

println("--- safe ---")

// safe: success path — wraps plain return value in (result, null)
fn double(x) { return x * 2 }
val, err = safe(double, 21)
println(val)       // 42
println(err)       // null

// safe: function that already returns (val, err) tuple — passed through unchanged
fn divide2(a, b) {
    if b == 0 { return null, "division by zero" }
    return a / b, null
}
val, err = safe(divide2, 10, 2)
println(val)       // 5
println(err)       // null
val, err = safe(divide2, 10, 0)
println(val)       // null
println(err)       // division by zero

// safe: unanticipated runtime error (array index out of bounds) caught as (null, message)
fn risky(i) {
    arr = [1, 2, 3]
    return arr[i]
}
val, err = safe(risky, 1)
println(val)       // 2
println(err)       // null
val, err = safe(risky, 99)
println(val == null)           // null
println(err.code)              // RUNTIME_ERROR
println(err.message)           // index 99 out of bounds (length 3)

println("--- io ---")

// write a file then read it back
writeFile("/tmp/klex_test.txt", "hello from kLex\n")
content = readFile("/tmp/klex_test.txt")
print(content)     // hello from kLex

// append a second line and read the whole thing back
appendFile("/tmp/klex_test.txt", "second line\n")
content = readFile("/tmp/klex_test.txt")
print(content)     // hello from kLex\nsecond line

// overwrite resets the file — no append from previous write
writeFile("/tmp/klex_test.txt", "fresh start\n")
content = readFile("/tmp/klex_test.txt")
print(content)     // fresh start

// reading a file that does not exist is a runtime error — catch it with safe
val, err = safe(readFile, "/tmp/klex_no_such_file_xyz.txt")
println(val)       // null
println(err == null)  // false

println("--- exec ---")

// run echo and capture its output
out = exec("echo", ["hello from exec"])
print(out)         // hello from exec

// run a command with no arguments
out = exec("pwd", [])
println(len(out) > 0)  // true

// safe wraps a command that does not exist
val, err = safe(exec, "no_such_binary_xyz", [])
println(val)       // null
println(err == null)  // false

println("--- hash bracket access ---")

person = {"name": "Karl", "age": 42}
println(person["name"])           // Karl
println(person["age"])            // 42

// missing key returns null
println(person["missing"] == null) // true

// bracket assignment
person["age"] = 43
println(person["age"])            // 43

// new key via bracket
person["city"] = "Dublin"
println(person["city"])           // Dublin

// nested hashes
address = {"street": "Main St", "zip": "D01"}
person["address"] = address
println(person["address"]["street"]) // Main St
println(person["address"]["zip"])    // D01

// bracket assignment and read
person["name"] = "Alice"
println(person["name"])           // Alice
person["name"] = "Bob"
println(person["name"])           // Bob

println("--- variadics ---")

// pure variadic — all args collected into an array
fn sum(nums...) {
    return reduce(nums, fn(acc, x) { acc + x }, 0)
}
println(sum(1, 2, 3))          // 6
println(sum(10, 20))           // 30
println(sum())                 // 0 — zero args gives empty array

// mixed: fixed params then variadic
fn joinWith(sep, parts...) {
    return join(parts, sep)
}
println(joinWith(", ", "a", "b", "c"))   // a, b, c
println(joinWith("-", "x", "y"))         // x-y

// variadic with a single extra arg
fn first(x, rest...) {
    return x
}
println(first(42, 99, 100))    // 42

// variadic passed to map/filter via anonymous wrapper
fn biggest(nums...) {
    sorted = []
    for n in nums {
        sorted = push(sorted, n)
    }
    hi = sorted[0]
    for n in sorted {
        if n > hi { hi = n }
    }
    return hi
}
println(biggest(3, 1, 4, 1, 5, 9, 2, 6))  // 9

println("--- switch ---")

// value switch
status = "error"
switch status {
    case "ok"    { println("all good") }
    case "error" { println("something failed") }
    case "retry" { println("try again") }
    default      { println("unknown") }
}

// multiple values per case
day = "Saturday"
switch day {
    case "Saturday", "Sunday" { println("weekend") }
    default                   { println("weekday") }
}

// no match, no default — silent
switch "nope" {
    case "a" { println("a") }
    case "b" { println("b") }
}
println("after silent switch")

// expression switch (no subject)
x = 42
switch {
    case x > 100 { println("big") }
    case x > 10  { println("medium") }
    default      { println("small") }
}

// switch on integer
code = 404
switch code {
    case 200      { println("ok") }
    case 404      { println("not found") }
    case 500, 503 { println("server error") }
    default       { println("other") }
}

// switch inside a function
fn grade(score) {
    switch {
        case score >= 90 { return "A" }
        case score >= 80 { return "B" }
        case score >= 70 { return "C" }
        default          { return "F" }
    }
}
println(grade(95))   // A
println(grade(83))   // B
println(grade(71))   // C
println(grade(55))   // F

println("--- range ---")

// range(stop) — 0 to stop-1
for i in range(5) { print(str(i) + " ") }
println("")   // 0 1 2 3 4

// range(start, stop)
for i in range(3, 8) { print(str(i) + " ") }
println("")   // 3 4 5 6 7

// range(start, stop, step)
for i in range(0, 10, 2) { print(str(i) + " ") }
println("")   // 0 2 4 6 8

// negative step counts down
for i in range(5, 0, -1) { print(str(i) + " ") }
println("")   // 5 4 3 2 1

// empty range produces no iterations
for i in range(5, 5) { println("never") }
println("empty range ok")

// range used outside a loop — it is just an array
r = range(4)
println(len(r))   // 4
println(r[2])     // 2

println("--- env ---")

// HOME is always set on macOS/Unix
home = env("HOME")
println(home == null)        // false
println(type(home))          // STRING

// unset variable returns null
println(env("KLEX_NO_SUCH_VAR_XYZ") == null)  // true

println("--- async ---")

// basic async/await — function runs in background, result retrieved via await
task = async(fn() { return 42 })
println(await(task))   // 42

// async with arguments passed through
task2 = async(fn(x, y) { return x + y }, 19, 23)
println(await(task2))  // 42

// sleep inside an async task
task3 = async(fn() {
    sleep(10)
    return "done"
})
println(await(task3))  // done

// type of a task value
t = async(fn() { return 1 })
println(type(t))   // TASK
await(t)

// multiple concurrent tasks — total time is the slowest, not the sum
t1 = async(fn() { sleep(30)  return "a" })
t2 = async(fn() { sleep(10)  return "b" })
println(await(t1))  // a
println(await(t2))  // b

// async errors propagate via await — use safe() to catch them
errTask = async(fn() { return 1 / 0 })
val, err = safe(await, errTask)
println(val == null)  // true
println(err != null)  // true

println("--- structs ---")

// basic struct declaration and instantiation
struct Point {
    x, y
}
p = Point { x: 10, y: 20 }
println(p.x)   // 10
println(p.y)   // 20

// field mutation
p.x = 99
println(p.x)   // 99

// type check
println(type(p))   // STRUCT

// struct with methods
struct Rect {
    w, h
    fn area() { return self.w * self.h }
    fn scale(f) { self.w = self.w * f   self.h = self.h * f }
    fn desc() { return str(self.w) + "x" + str(self.h) }
}
r = Rect { w: 4, h: 5 }
println(r.area())   // 20
r.scale(2)
println(r.desc())   // 8x10
println(r.area())   // 80

// counter with encapsulated state
struct Counter {
    n
    fn inc()   { self.n = self.n + 1 }
    fn reset() { self.n = 0 }
    fn get()   { return self.n }
}
c = Counter { n: 0 }
c.inc()
c.inc()
c.inc()
println(c.get())   // 3
c.reset()
println(c.get())   // 0

// structs passed to functions
fn double_rect(r) {
    r.scale(2)
    return r
}
r2 = Rect { w: 3, h: 4 }
double_rect(r2)
println(r2.area())  // 48 (passed by reference)

// str() of a struct instance
p2 = Point { x: 1, y: 2 }
s = str(p2)
println(type(s))   // STRING

println("--- await_all ---")
import "async.lex" as a

// await_all returns results in input order, not completion order
t1 = async(fn() { sleep(30)  return "slow" })
t2 = async(fn() { sleep(10)  return "fast" })
results = a.await_all([t1, t2])
println(results[0])   // slow
println(results[1])   // fast

// await_all with a single task
t3 = async(fn() { return 99 })
r = a.await_all([t3])
println(r[0])   // 99

// task can be awaited more than once — second await returns immediately
t4 = async(fn() { return 42 })
println(await(t4))   // 42
println(await(t4))   // 42 (no block)

println("--- channels ---")

// basic send/recv — (value, bool) tuple
ch = channel()
task = async(fn() { send(ch, 99) })
val, ok = recv(ch)
println(ok)           // true
println(val)          // 99
await(task)

// recv on closed channel returns (null, false)
ch2 = channel()
close(ch2)
val, ok = recv(ch2)
println(ok)           // false
println(val == null)  // true

// buffered channel — send does not block until full
ch3 = channel(3)
send(ch3, 10)
send(ch3, 20)
send(ch3, 30)
val, ok = recv(ch3)
println(val)          // 10
println(ok)           // true

// for-in drains channel until closed
ch4 = channel()
t = async(fn() {
    for i in range(4) {
        send(ch4, i * i)
    }
    close(ch4)
})
for v in ch4 {
    print(str(v) + " ")
}
println("")   // 0 1 4 9
await(t)

// type check
println(type(channel()))   // CHANNEL

// send to closed channel is a RuntimeError
ch5 = channel()
close(ch5)
val, err = safe(send, ch5, 1)
println(val == null)  // true
println(err != null)  // true

// close already-closed channel is a RuntimeError
ch6 = channel()
close(ch6)
val, err = safe(close, ch6)
println(val == null)  // true
println(err != null)  // true

println("--- enums ---")

enum Shape {
    Circle(r)
    Rect(w, h)
    Point
}

// construction
s1 = Shape.Circle(5.0)
s2 = Shape.Rect(4.0, 3.0)
s3 = Shape.Point

// field access
println(s1.r)      // 5
println(s2.w)      // 4
println(s2.h)      // 3

// type
println(type(s1))  // ENUM
println(type(s3))  // ENUM

// str
println(str(s3))   // Shape.Point

// switch on variant
fn describe(s) {
    switch s {
        case Shape.Circle { return "circle r=" + str(s.r) }
        case Shape.Rect   { return str(s.w) + "x" + str(s.h) }
        case Shape.Point  { return "point" }
    }
}
println(describe(s1))   // circle r=5
println(describe(s2))   // 4x3
println(describe(s3))   // point

// equality — same variant and fields
println(Shape.Circle(5.0) == Shape.Circle(5.0))   // true
println(Shape.Circle(5.0) == Shape.Circle(6.0))   // false
println(Shape.Circle(5.0) == Shape.Rect(5.0, 1))  // false

// instance == variant descriptor (the switch mechanism)
println(s1 == Shape.Circle)   // true
println(s1 == Shape.Rect)     // false
println(s3 == Shape.Point)    // true

// real-world: Result type
enum Result {
    Ok(value)
    Err(message)
}

fn divide(a, b) {
    if b == 0 { return Result.Err("division by zero") }
    return Result.Ok(a / b)
}

r = divide(10, 2)
switch r {
    case Result.Ok  { println(r.value) }    // 5
    case Result.Err { println(r.message) }
}

r2 = divide(10, 0)
switch r2 {
    case Result.Ok  { println(r2.value) }
    case Result.Err { println(r2.message) }  // division by zero
}


println("--- inline progress bar ---")

fn progressBar(total) {
    i = 0

    while i <= total {

        percent = int((i * 100) / total)

        bar = "["
        filled = int((i * 20) / total)

        j = 0
        while j < 20 {
            if j < filled {
                bar = bar + "#"
            } else {
                bar = bar + "-"
            }
            j = j + 1
        }

        bar = bar + "] " + str(percent) + "%"

        // overwrite same line
        print("\r" + bar)

        sleep(10)
        i = i + 1
    }

    println("")
    println("Complete!")
}

progressBar(100)

println("--- format ---")

// %d — integer decimal
println(format("%d", 42) == "42")
println(format("%10d", 42) == "        42")
println(format("%-10d|", 42) == "42        |")
println(format("%010d", 42) == "0000000042")
println(format("%+d", 42) == "+42")
println(format("%+d", -42) == "-42")

// %f — float
println(format("%f", 3.14159) == "3.141590")
println(format("%.2f", 3.14159) == "3.14")
println(format("%10.2f", 3.14159) == "      3.14")
println(format("%-10.2f|", 3.14159) == "3.14      |")
println(format("%.2f", 3) == "3.00")        // int auto-promotes to float

// %s — string (strict)
println(format("%s", "hello") == "hello")
println(format("%10s", "hi") == "        hi")
println(format("%-10s|", "hi") == "hi        |")

// %t — boolean
println(format("%t", true) == "true")
println(format("%t", false) == "false")

// %v — any type via Inspect()
println(format("%v", 42) == "42")
println(format("%v", true) == "true")
println(format("%v", [1, 2, 3]) == "[1, 2, 3]")
println(format("%10v", 99) == "        99")

// %x %X — hex
println(format("%x", 255) == "ff")
println(format("%X", 255) == "FF")
println(format("%08x", 255) == "000000ff")

// %o — octal, %b — binary
println(format("%o", 8) == "10")
println(format("%b", 10) == "1010")

// %% — literal percent
println(format("%.1f%%", 99.9) == "99.9%")

// multiple arguments
println(format("%s is %d years old", "Karl", 42) == "Karl is 42 years old")
println(format("%-10s %5d %8.2f", "item", 3, 9.99) == "item           3     9.99")
println(format("%d + %d = %d", 1, 2, 3) == "1 + 2 = 3")

// type mismatch is a RuntimeError caught by safe()
val, err = safe(format, "%d", "oops")
println(err.code == "RUNTIME_ERROR")
println(err.is("RUNTIME_ERROR"))

// too few arguments
val, err = safe(format, "%d %d", 1)
println(err.code == "RUNTIME_ERROR")

// too many arguments
val, err = safe(format, "%d", 1, 2)
println(err.code == "RUNTIME_ERROR")

println("--- typed errors ---")

// --- error() constructor ---
e = error("NOT_FOUND", "key was missing")
println(type(e) == "ERROR")               // true
println(e.code == "NOT_FOUND")            // true
println(e.message == "key was missing")   // true
println(e.is("NOT_FOUND"))               // true
println(e.is("RUNTIME_ERROR"))           // false

// str() on an error shows its full form
println(str(e) == "error(NOT_FOUND: key was missing)")  // true

// --- safe() now returns typed ErrorObject for system errors ---

// runtime error: integer division by zero
val, err = safe(fn() { return 1 / 0 })
println(val == null)              // true
println(type(err) == "ERROR")    // true
println(err.code == "RUNTIME_ERROR")  // true
println(err.is("RUNTIME_ERROR"))      // true
println(err.is("TYPE_ERROR"))         // false

// type error: incompatible operands
val, err = safe(fn() { return 1 + true })
println(val == null)             // true
println(err.code == "TYPE_ERROR")     // true
println(err.is("TYPE_ERROR"))         // true

// user-returned tuples pass through safe() unchanged (strings stay strings)
fn safeDivide(a, b) {
    if b == 0 { return null, "division by zero" }
    return a / b, null
}
val, err = safe(safeDivide, 10, 0)
println(val == null)             // true
println(type(err) == "STRING")  // true — user string, not an ErrorObject

// --- switch on error code ---
val, err = safe(fn() { return 1 / 0 })
switch err.code {
    case "RUNTIME_ERROR" { println(true) }   // true
    case "TYPE_ERROR"    { println(false) }
}

// --- error propagation pattern ---
fn lookup(store, key) {
    if store[key] == null {
        return null, error("NOT_FOUND", "key '" + key + "' does not exist")
    }
    return store[key], null
}

myStore = {"x": 10, "y": 20}

result, err = lookup(myStore, "x")
println(err == null)    // true
println(result == 10)   // true

result, err = lookup(myStore, "z")
println(result == null)             // true
println(err.code == "NOT_FOUND")   // true
println(err.message == "key 'z' does not exist")  // true
println("--- let ---")
// basic: let creates a local binding, does not touch outer scope
i = 0
fn letTest() {
    let i = 99
    i = i + 1
    return i
}
println(letTest() == 100)   // true — local i goes 99 → 100
println(i == 0)             // true — outer i untouched

// let inside while: local counter does not escape
outer = 0
fn countLocal() {
    let j = 0
    while j < 3 {
        j = j + 1
    }
    return j
}
println(countLocal() == 3)  // true — local j reached 3
println(outer == 0)         // true — outer is unchanged

// let shadows, then closure mutation still works on a let-declared var
fn makeAdder(start) {
    let n = start
    fn add(x) { n = n + x }
    add(10)
    add(5)
    return n
}
println(makeAdder(0) == 15)  // true — closure mutates let-declared n correctly

// let with expression value
let computed = 2 + 3 * 4
println(computed == 14)  // true


println("--- select ---")

// basic recv: read the only ready channel
ch1 = channel()
async(fn() { send(ch1, 42) })
sleep(10)
select {
    case val, ok = recv(ch1) {
        println(val == 42)       // true
        println(ok == true)      // true
    }
}

// default: non-blocking — channel not ready, default fires
empty = channel()
hit = "none"
select {
    case _, _ = recv(empty) {
        hit = "recv"
    }
    default {
        hit = "default"
    }
}
println(hit == "default")        // true

// fan-in: two channels, whichever is ready fires
fa = channel()
fb = channel()
async(fn() { send(fb, 99) })
sleep(10)
fanResult = 0
select {
    case v, _ = recv(fa) {
        fanResult = v
    }
    case v, _ = recv(fb) {
        fanResult = v
    }
}
println(fanResult == 99)         // true

// send case: non-blocking send to a buffered channel
bch = channel(1)
sent = false
select {
    case send(bch, 7) {
        sent = true
    }
    default {
        sent = false
    }
}
println(sent == true)            // true
val2, _ = recv(bch)
println(val2 == 7)               // true

// timeout pattern: result arrives before timer
resultCh = channel()
timerCh  = channel()
async(fn() { sleep(10); send(resultCh, "done") })
async(fn() { sleep(200); send(timerCh, "timeout") })
got = ""
select {
    case v, _ = recv(resultCh) {
        got = v
    }
    case _, _ = recv(timerCh) {
        got = "timeout"
    }
}
println(got == "done")           // true

// recv with single binding
sch = channel()
async(fn() { send(sch, "hello") })
sleep(10)
select {
    case msg = recv(sch) {
        println(msg == "hello")  // true
    }
}

// recv with no binding
nch = channel()
async(fn() { send(nch, true) })
sleep(10)
reached = false
select {
    case recv(nch) {
        reached = true
    }
}
println(reached == true)         // true

// closed channel: recv on closed returns (null, false)
dch = channel()
close(dch)
closedOk = true
select {
    case _, closedOk = recv(dch) {
    }
}
println(closedOk == false)       // true
