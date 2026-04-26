import "http.lex" as http

// --- GET ---
resp, err = http.get("https://httpbin.org/get")
println(err == null)           // true
println(http.isOk(resp))       // true
println(resp.status)           // 200
println(type(resp.body) == "STRING")   // true
println(type(resp.headers) == "HASH")  // true

// content-type header present
ct = http.header(resp, "content-type")
println(ct != null)            // true

// --- POST JSON ---
resp, err = http.post("https://httpbin.org/post", "\{\"name\":\"kLex\"}", "application/json")
println(err == null)           // true
println(http.isOk(resp))       // true
println(resp.status)           // 200

// --- 404 is not an error — it is a valid response ---
resp, err = http.get("https://httpbin.org/status/404")
println(err == null)           // true
println(resp.status)           // 404
println(http.isOk(resp))       // false

// --- network error ---
resp, err = http.get("https://does-not-exist.invalid")
println(err != null)           // true
println(resp == null)          // true

// --- custom request with headers ---
resp, err = http.request("GET", "https://httpbin.org/headers",
    {"X-Custom-Header": "kLex"}, null)
println(err == null)           // true
println(http.isOk(resp))       // true
