// bytesTest.lex — coverage for the bytes type and its builtins.
//
// Sections:
//   1. Literals + escape sequences
//   2. Indexing (returns integer 0..255)
//   3. len() / slice() on bytes
//   4. Concatenation via +
//   5. Equality (by content, not identity) + null-compare rules
//   6. bytes() constructor — all four forms
//   7. str <-> bytes round trip
//   8. base64 + hex encode/decode round trips
//   9. Error paths: invalid utf-8, malformed base64/hex
//  10. Type errors: cross-type compare with strings is a TypeError
//
// Run: ./klex tests/examples/bytesTest.lex

println("=== 1. literals ===")
let a = b"\x00\x01\x02"
println(len(a))             // 3
println(a[0])               // 0
println(a[2])               // 2

let b = b"hi\t\n"
println(len(b))             // 4
println(b[0])               // 104
println(b[2])               // 9 (tab)

let empty = b""
println(len(empty))         // 0

println("")
println("=== 2. indexing ===")
let hi = b"Hi!"
println(hi[0])              // 72
println(hi[1])              // 105
println(hi[2])              // 33

println("")
println("=== 3. len / slice ===")
let hex = b"\xde\xad\xbe\xef"
println(len(hex))           // 4
let mid = slice(hex, 1, 3)
println(len(mid))           // 2
println(mid[0])             // 173
println(mid[1])             // 190

println("")
println("=== 4. concatenation ===")
let joined = b"foo" + b"bar"
println(len(joined))        // 6
println(joined[3])          // 98 ('b')

println("")
println("=== 5. equality ===")
println(b"abc" == b"abc")          // true
println(b"abc" == b"xyz")          // false
println(b"" == b"")                // true
println(b"abc" == null)            // false (null-compare rule)
println(null == b"abc")            // false
// Content-based: two separately-built bytes with same content are equal.
let c1 = bytes("Hello")
let c2 = b"Hello"
println(c1 == c2)                  // true

println("")
println("=== 6. bytes() constructor ===")
let fromStr = bytes("Hi")
println(len(fromStr))              // 2
println(fromStr == b"Hi")          // true

let fromArr = bytes([72, 105])
println(fromArr == b"Hi")          // true

let fromInt = bytes(4)
println(len(fromInt))              // 4
println(fromInt[0])                // 0

let fromBytes = bytes(b"copy me")
println(fromBytes == b"copy me")   // true

println("")
println("=== 7. str <-> bytes round trip ===")
let out = strToBytes("Hello, kLex!")
println(len(out))                  // 12 (utf-8 byte count, not rune count)
let back, err = bytesToStr(out)
if err == null { println(back) }   // Hello, kLex!

// utf-8 multibyte: emoji = 4 bytes
let emojiBytes = strToBytes("😀")
println(len(emojiBytes))           // 4
let roundTrip, err = bytesToStr(emojiBytes)
if err == null { println(roundTrip) }  // 😀

println("")
println("=== 8. base64 + hex round trips ===")
let src = b"\x00\x01\x02\xde\xad\xbe\xef"

let enc64 = bytesToBase64(src)
println(enc64)                     // AAEC3q2+7w==
let dec64, err = base64ToBytes(enc64)
if err == null { println(dec64 == src) }   // true

let encHex = bytesToHex(src)
println(encHex)                    // 000102deadbeef
let decHex, err = hexToBytes(encHex)
if err == null { println(decHex == src) }  // true

// Uppercase hex accepted too
let decUpper, err = hexToBytes("DEADBEEF")
if err == null { println(decUpper == b"\xde\xad\xbe\xef") }  // true

println("")
println("=== 9. error paths ===")
let notUtf8 = b"\xff\xfe\xfd"
_, err = bytesToStr(notUtf8)
if err != null {
    println("✓ bytesToStr code: " + err.code)
    println("  message: " + err.message)
}

_, err = base64ToBytes("!!! not base64 !!!")
if err != null {
    println("✓ base64ToBytes code: " + err.code)
}

_, err = hexToBytes("nothex")
if err != null {
    println("✓ hexToBytes code: " + err.code)
}

_, err = hexToBytes("abc")  // odd length
if err != null {
    println("✓ odd-length hex code: " + err.code)
}

println("")
println("=== 10. type() reports BYTES ===")
println(type(b"abc"))              // BYTES
println(type(bytes([1, 2, 3])))    // BYTES

println("")
println("bytes test suite complete")
