import "base64.lex" as b64

// --- standard encode / decode ---
encoded = b64.encode("hello")
println(encoded)                    // aGVsbG8=

decoded, err = b64.decode(encoded)
println(err == null)                // true
println(decoded)                    // hello

// round-trip
msg = "kLex is a tree-walking interpreter"
rt, err = b64.decode(b64.encode(msg))
println(rt == msg)                  // true

// empty string
println(b64.encode(""))             // (empty)
mt, err = b64.decode("")
println(err == null)                // true
println(mt)                         // (empty)

// --- url-safe encode / decode ---
urlEnc = b64.urlEncode("hello world")
println(type(urlEnc) == "STRING")   // true
println(indexOf(urlEnc, "+") == -1) // true — no + in url-safe

urlDec, err = b64.urlDecode(urlEnc)
println(err == null)                // true
println(urlDec)                     // hello world

// --- decode error ---
bad, err = b64.decode("not!valid==base64!")
println(err != null)                // true
println(bad == null)                // true
