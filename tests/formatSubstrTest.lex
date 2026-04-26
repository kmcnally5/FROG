// format() tests
println("=== format ===")
println(format("hello, %s!", "world"))
println(format("integer: %d", 42))
println(format("padded:  %05d", 42))
println(format("float:   %.2f", 3.14159))
println(format("bool:    %t", true))
println(format("%s is %d years old", "Karl", 99))

// substr() tests
println("")
println("=== substr ===")
s = "hello world"
println(substr(s, 6))        // world
println(substr(s, 0, 5))     // hello
println(substr(s, 6, 11))    // world
println(substr(s, 0, 0))     // empty

// substr out-of-bounds error
result, err = safe(fn() { return substr(s, 20) })
if err != null { println("out-of-bounds caught: " + err.message) }

// slice() tests
println("")
println("=== slice ===")
arr = [10, 20, 30, 40, 50]
println(str(slice(arr, 2)))        // [30, 40, 50]
println(str(slice(arr, 1, 4)))     // [20, 30, 40]
println(str(slice(arr, 0, 1)))     // [10]

// slice out-of-bounds error
result2, err2 = safe(fn() { return slice(arr, 10) })
if err2 != null { println("out-of-bounds caught: " + err2.message) }
