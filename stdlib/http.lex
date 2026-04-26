// http.lex
// HTTP client stdlib for kLex
//
// All functions return (Response, err) tuples.
// On success err is null. On failure resp is null and err is a string.
//
// Usage:
//   import "http.lex" as http
//   resp, err = http.get("https://example.com")
//   if err != null { println(err)  return null }
//   println(resp.status)
//   println(resp.body)

struct Response {
    status, body, headers
}

// request is the base function — all others delegate to it.
// method  — HTTP verb: "GET", "POST", "PUT", "PATCH", "DELETE"
// url     — full URL including scheme
// headers — hash of string → string, or null
// body    — string body, or null
fn request(method, url, headers, body) {
    status, rbody, rheaders, err = _httpDo(method, url, headers, body)
    if err != null { return null, err }
    return Response { status: status, body: rbody, headers: rheaders }, null
}

// get performs an HTTP GET.
fn get(url) {
    return request("GET", url, null, null)
}

// post performs an HTTP POST with the given body and Content-Type.
fn post(url, body, contentType) {
    return request("POST", url, {"Content-Type": contentType}, body)
}

// put performs an HTTP PUT with the given body and Content-Type.
fn put(url, body, contentType) {
    return request("PUT", url, {"Content-Type": contentType}, body)
}

// patch performs an HTTP PATCH with the given body and Content-Type.
fn patch(url, body, contentType) {
    return request("PATCH", url, {"Content-Type": contentType}, body)
}

// del performs an HTTP DELETE.
fn del(url) {
    return request("DELETE", url, null, null)
}

// isOk returns true if the response status is in the 2xx range.
fn isOk(resp) {
    return resp.status >= 200 && resp.status < 300
}

// header returns the value of a response header by name (lowercase).
// Returns null if the header is not present.
fn header(resp, name) {
    return resp.headers[name]
}
