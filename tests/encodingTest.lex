import "encoding.lex" as enc

println("== ord ==")
println(enc.ord(" "))    // 32
println(enc.ord("A"))    // 65
println(enc.ord("a"))    // 97
println(enc.ord("0"))    // 48

println("== chr ==")
println(enc.chr(32))     //   (space)
println(enc.chr(65))     // A
println(enc.chr(97))     // a
println(enc.chr(48))     // 0

println("== round-trip ==")
println(enc.chr(enc.ord("Z")))    // Z
println(enc.chr(enc.ord("z")))    // z

println("== bytes ==")
bs = enc.bytes("AB")
println(len(bs))     // 2
println(bs[0])       // 65
println(bs[1])       // 66

println("== stringFromBytes ==")
println(enc.stringFromBytes([65, 66, 67]))    // ABC
println(enc.stringFromBytes([104, 105]))      // hi

println("== bytes round-trip ==")
original = "Hello"
reconstructed = enc.stringFromBytes(enc.bytes(original))
println(reconstructed)    // Hello
