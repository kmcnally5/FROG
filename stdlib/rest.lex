// rest.lex
// JSON REST client for kLex — combines http.lex and json.lex.
//
// All functions return (RestResponse, err). On success err is null.
// On failure resp is null and err is a descriptive string.
//
// The data field of RestResponse holds the parsed JSON body when the
// server responds with Content-Type: application/json, otherwise it
// holds the raw body string.
//
// Usage:
//   import "rest.lex" as rest
//   resp, err = rest.get("https://api.example.com/users/1")
//   if err != null { println(err)  return null }
//   println(resp.status)
//   println(resp.data["name"])

import "http.lex" as _http
import "json.lex" as _json

struct RestResponse {
    status, data, headers
}

// request is the base function all others delegate to.
// method      — HTTP verb string
// url         — full URL including scheme
// extraHeaders — hash of additional headers, or null
// body        — kLex value to send as JSON, or null for no body
fn request(method, url, extraHeaders, body) {
    // Build a fresh headers hash so we never mutate the caller's hash.
    headers = {}
    if extraHeaders != null {
        for k, v in extraHeaders {
            headers[k] = v
        }
    }

    // Serialise body to JSON and set Content-Type.
    jsonBody = null
    if body != null {
        jsonBody = _json.stringify(body)
        headers["Content-Type"] = "application/json"
    }

    resp, err = _http.request(method, url, headers, jsonBody)
    if err != null { return null, err }

    // Auto-parse JSON responses.
    data = resp.body
    ct = _http.header(resp, "content-type")
    if ct != null && indexOf(ct, "application/json") != -1 {
        parsed, parseErr = _json.parse(resp.body)
        if parseErr == null {
            data = parsed
        }
    }

    return RestResponse { status: resp.status, data: data, headers: resp.headers }, null
}

// get performs a GET request with optional extra headers.
fn get(url) {
    return request("GET", url, null, null)
}

fn getWith(url, headers) {
    return request("GET", url, headers, null)
}

// post serialises body as JSON and performs a POST.
fn post(url, body) {
    return request("POST", url, null, body)
}

fn postWith(url, body, headers) {
    return request("POST", url, headers, body)
}

// put serialises body as JSON and performs a PUT.
fn put(url, body) {
    return request("PUT", url, null, body)
}

fn putWith(url, body, headers) {
    return request("PUT", url, headers, body)
}

// patch serialises body as JSON and performs a PATCH.
fn patch(url, body) {
    return request("PATCH", url, null, body)
}

fn patchWith(url, body, headers) {
    return request("PATCH", url, headers, body)
}

// del performs a DELETE request.
fn del(url) {
    return request("DELETE", url, null, null)
}

fn delWith(url, headers) {
    return request("DELETE", url, headers, null)
}

// isOk returns true if the response status is in the 2xx range.
fn isOk(resp) {
    return resp.status >= 200 && resp.status < 300
}
