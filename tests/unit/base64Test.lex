import "stdlib/base64.lex" as b64

// --- standard encode / decode ---
let encoded = b64.encode("hello")
println(encoded)                    // aGVsbG8=

let decoded, err = b64.decode(encoded)
println(err == null)                // true
println(decoded)                    // hello

// round-trip
let msg = "kLex is a tree-walking interpreter"
let rt, err = b64.decode(b64.encode(msg))
println(rt == msg)                  // true

// empty string
println(b64.encode(""))             // (empty)
let mt, err = b64.decode("")
println(err == null)                // true
println(mt)                         // (empty)

// --- url-safe encode / decode ---
let urlEnc = b64.urlEncode("hello world")
println(type(urlEnc) == "STRING")   // true
println(indexOf(urlEnc, "+") == -1) // true — no + in url-safe

let urlDec, err = b64.urlDecode(urlEnc)
println(err == null)                // true
println(urlDec)                     // hello world

// --- decode error ---
let bad, err = b64.decode("not!valid==base64!")
println(err != null)                // true
println(bad == null)                // true
