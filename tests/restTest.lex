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


// ============================================================================
// AUTHENTICATION TESTS
// ============================================================================

// --- bearerToken() generates correct header format ---
header = rest.bearerToken("abc123")
println(header == "Bearer abc123")      // true
println(startsWith(header, "Bearer "))  // true

// --- basicAuth() generates correct header format ---
header = rest.basicAuth("user", "pass")
println(header == "Basic dXNlcjpwYXNz")  // true (base64 of "user:pass")
println(startsWith(header, "Basic "))    // true

// --- apiKeyHeader() returns key as-is ---
header = rest.apiKeyHeader("secret-key-123")
println(header == "secret-key-123")     // true

// --- getBearer() with httpbin /bearer endpoint ---
// (httpbin returns 401 for missing bearer, 200 with valid token)
resp, err = rest.getBearer("https://httpbin.org/bearer", "test-token")
println(err == null)                    // true (network succeeded)
println(resp.status == 200)             // true (Bearer token accepted)
println(type(resp.data) == "HASH")      // true (JSON response)

// --- getBasic() with httpbin /basic-auth endpoint ---
// (httpbin /basic-auth/user/pass returns 200 for correct creds, 401 for wrong)
resp, err = rest.getBasic("https://httpbin.org/basic-auth/testuser/testpass", "testuser", "testpass")
println(err == null)                    // true (network succeeded)
println(resp.status == 200)             // true (Basic auth accepted)
println(type(resp.data) == "HASH")      // true (JSON response)

// --- getBasic() with wrong credentials returns 401 ---
resp, err = rest.getBasic("https://httpbin.org/basic-auth/testuser/testpass", "testuser", "wrongpass")
println(err == null)                    // true (network succeeded, 401 is not an error)
println(resp.status == 401)             // true (wrong password)

// --- postBearer() convenience wrapper ---
resp, err = rest.postBearer("https://httpbin.org/post", {"test": "data"}, "token123")
println(err == null)                    // true
println(rest.isOk(resp))                // true
println(type(resp.data) == "HASH")      // true

// --- postBasic() convenience wrapper ---
resp, err = rest.postBasic("https://httpbin.org/post", {"test": "data"}, "user", "pass")
println(err == null)                    // true
println(rest.isOk(resp))                // true

// --- putBearer() convenience wrapper ---
resp, err = rest.putBearer("https://httpbin.org/put", {"test": "data"}, "token123")
println(err == null)                    // true
println(rest.isOk(resp))                // true

// --- patchBearer() convenience wrapper ---
resp, err = rest.patchBearer("https://httpbin.org/patch", {"test": "data"}, "token123")
println(err == null)                    // true
println(rest.isOk(resp))                // true

// --- delBearer() convenience wrapper ---
resp, err = rest.delBearer("https://httpbin.org/delete", "token123")
println(err == null)                    // true
println(rest.isOk(resp))                // true

println("")
println("restTest: all authentication tests passed!")
