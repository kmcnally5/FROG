import "url.lex" as url

// --- encode / decode ---
println(url.encode("hello world"))         // hello+world
println(url.encode("a=1&b=2"))             // a%3D1%26b%3D2

decoded, err = url.decode("hello+world")
println(err == null)                        // true
println(decoded)                            // hello world

decoded, err = url.decode("a%3D1%26b%3D2")
println(err == null)                        // true
println(decoded)                            // a=1&b=2

// --- build ---
result = url.build("https://api.example.com/search", {"q": "hello world", "page": "1"})
println(startsWith(result, "https://api.example.com/search?"))  // true
println(indexOf(result, "hello+world") != -1)                   // true
println(indexOf(result, "page=1") != -1)                        // true

// empty params returns base unchanged
println(url.build("https://example.com", {}))  // https://example.com

// --- joinPath ---
println(url.joinPath("https://api.example.com", "users/42"))
// https://api.example.com/users/42

println(url.joinPath("https://api.example.com/", "/users/42"))
// https://api.example.com/users/42

println(url.joinPath("https://api.example.com", "/users/42"))
// https://api.example.com/users/42

// --- decode error ---
bad, err = url.decode("%zz")
println(err != null)    // true
println(bad == null)    // true
