import "rest.lex" as rest

// --- GET with auto JSON parse ---
resp, err = rest.get("https://jsonplaceholder.typicode.com/posts/1")
println(err == null)           // true
println(rest.isOk(resp))       // true
println(resp.status)           // 200
println(type(resp.data) == "HASH")   // true
println(resp.data["id"])       // 1
println(resp.data["userId"])   // 1

// --- POST with auto JSON serialise + parse ---
resp, err = rest.post("https://jsonplaceholder.typicode.com/posts",
    {"title": "kLex", "body": "hello", "userId": 1})
println(err == null)           // true
println(rest.isOk(resp))       // true
println(resp.status)           // 201
println(type(resp.data) == "HASH")   // true
println(resp.data["title"])    // kLex

// --- custom headers via getWith ---
resp, err = rest.getWith("https://httpbin.org/headers", {"X-App": "kLex"})
println(err == null)           // true
println(rest.isOk(resp))       // true

// --- 404 is a valid response, not an error ---
resp, err = rest.get("https://jsonplaceholder.typicode.com/posts/99999")
println(err == null)           // true
println(resp.status)           // 404
println(rest.isOk(resp))       // false

// --- network error ---
resp, err = rest.get("https://does-not-exist.invalid")
println(err != null)           // true
println(resp == null)          // true
