// ord() and chr() are kLex builtins (eval/builtins_strings.go).
// stdlib/encoding.lex provides the array-shaped helpers bytes/stringFromBytes
// on top of them.
import "stdlib/encoding.lex" as enc

println("== ord ==")
println(ord(" "))    // 32
println(ord("A"))    // 65
println(ord("a"))    // 97
println(ord("0"))    // 48

println("== chr ==")
println(chr(32))     //   (space)
println(chr(65))     // A
println(chr(97))     // a
println(chr(48))     // 0

println("== round-trip ==")
println(chr(ord("Z")))    // Z
println(chr(ord("z")))    // z

println("== bytes ==")
let bs = enc.bytes("AB")
println(len(bs))     // 2
println(bs[0])       // 65
println(bs[1])       // 66

println("== stringFromBytes ==")
println(enc.stringFromBytes([65, 66, 67]))    // ABC
println(enc.stringFromBytes([104, 105]))      // hi

println("== bytes round-trip ==")
let original = "Hello"
let reconstructed = enc.stringFromBytes(enc.bytes(original))
println(reconstructed)    // Hello

println("== unicode ==")
println(ord("☃"))                 // 9731
println(chr(9731))                // ☃
println(chr(ord("ñ")))            // ñ
