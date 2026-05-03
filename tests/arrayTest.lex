import "array.lex" as arr

println("== first / last ==")
a = [10, 20, 30]
println(arr.first(a))     // 10
println(arr.last(a))      // 30

println("== contains ==")
println(arr.contains([1, 2, 3], 2))   // true
println(arr.contains([1, 2, 3], 5))   // false

println("== reverse ==")
rev = arr.reverse([1, 2, 3])
println(rev[0])    // 3
println(rev[1])    // 2
println(rev[2])    // 1

println("== unique ==")
u = arr.unique([1, 2, 2, 3, 3, 3])
println(len(u))    // 3
println(u[0])      // 1
println(u[1])      // 2
println(u[2])      // 3

println("== flatten ==")
flat = arr.flatten([[1, 2], [3, 4], 5])
println(len(flat))    // 5
println(flat[0])      // 1
println(flat[4])      // 5

println("== zip ==")
z = arr.zip([1, 2, 3], ["a", "b", "c"])
println(len(z))          // 3
println(z[0][0])         // 1
println(z[0][1])         // a
println(z[2][1])         // c

// zip stops at shorter length
z2 = arr.zip([1, 2], ["a", "b", "c"])
println(len(z2))          // 2

println("== sort ==")
s = sort([3, 1, 4, 1, 5, 9, 2])
println(s[0])     // 1
println(s[1])     // 1
println(s[6])     // 9

println("== split ==")
sp = arr.split([1, 2, ":", 3, 4], ":")
println(len(sp))        // 2
println(sp[0][0])       // 1
println(sp[0][1])       // 2
println(sp[1][0])       // 3
println(sp[1][1])       // 4
