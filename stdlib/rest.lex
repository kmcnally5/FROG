// rest.lex
// @module    rest
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   JSON REST client for kLex — combines http.lex and json.lex.
// JSON REST client for kLex — combines http.lex and json.lex.
//
// All functions return (RestResponse, err). On success err is null.
// On failure resp is null and err is a descriptive string.
//
// The data field of RestResponse holds the parsed JSON body when the
// server responds with Content-Type: application/json, otherwise it
// holds the raw body string.
//
// Authentication:
//   - Bearer token: rest.getBearer(url, token)
//   - Basic auth: rest.getBasic(url, username, password)
//   - Custom headers: resp, err = rest.getWith(url, {"X-API-Key": "secret"})
//
// Usage:
//   import "rest.lex" as rest
//
//   // Simple GET
//   resp, err = rest.get("https://api.example.com/users/1")
//   println(resp.data["name"])
//
//   // GET with Bearer token
//   resp, err = rest.getBearer("https://api.example.com/me", "token123")
//   println(resp.data["id"])

import "stdlib/http.lex" as _http
import "stdlib/json.lex" as _json
import "stdlib/base64.lex" as _b64

struct RestResponse {
    status, data, headers
}

// request is the base function all others delegate to.
// method       — HTTP verb string
// url          — full URL including scheme
// extraHeaders — hash of additional headers, or null
// body         — kLex value to send as JSON, or null for no body
// timeoutSec   — optional per-call timeout (number of seconds, or null).
//                Overrides the shared 30s default for LLM cold-starts
//                and other long-running endpoints.
fn request(method, url, extraHeaders, body, timeoutSec = null) {
    // Build a fresh headers hash so we never mutate the caller's hash.
    let headers = {}
    if extraHeaders != null {
        for k, v in extraHeaders {
            headers[k] = v
        }
    }

    // Serialise body to JSON and set Content-Type.
    let jsonBody = null
    if body != null {
        jsonBody = _json.stringify(body)
        headers["Content-Type"] = "application/json"
    }

    let resp, err = _http.request(method, url, headers, jsonBody, timeoutSec)
    if err != null { return null, err }

    // Auto-parse JSON responses.
    let data = resp.body
    let ct = _http.header(resp, "content-type")
    if ct != null && indexOf(ct, "application/json") != -1 {
        let parsed, parseErr = _json.parse(resp.body)
        if parseErr == null {
            data = parsed
        }
    }

    return RestResponse { status: resp.status, data: data, headers: resp.headers }, null
}

// postWithTimeout / postWithHeadersAndTimeout — convenience wrappers that
// surface the per-call timeout without having to thread it through the
// generic request() call.
fn postWithTimeout(url, body, timeoutSec) {
    return request("POST", url, null, body, timeoutSec)
}

// postWithHeadersAndTimeout(url, body, headers, timeoutSec) — POST with custom headers and a per-call timeout in seconds.
fn postWithHeadersAndTimeout(url, body, headers, timeoutSec) {
    return request("POST", url, headers, body, timeoutSec)
}

// getWithTimeout(url, timeoutSec) — GET with a per-call timeout in seconds.
fn getWithTimeout(url, timeoutSec) {
    return request("GET", url, null, null, timeoutSec)
}

// get performs a GET request with optional extra headers.
fn get(url) {
    return request("GET", url, null, null)
}

// getWith(url, headers) — GET with custom request headers hash.
fn getWith(url, headers) {
    return request("GET", url, headers, null)
}

// post serialises body as JSON and performs a POST.
fn post(url, body) {
    return request("POST", url, null, body)
}

// postWith(url, body, headers) — POST with body serialised as JSON and custom request headers hash.
fn postWith(url, body, headers) {
    return request("POST", url, headers, body)
}

// put serialises body as JSON and performs a PUT.
fn put(url, body) {
    return request("PUT", url, null, body)
}

// putWith(url, body, headers) — PUT with body serialised as JSON and custom request headers hash.
fn putWith(url, body, headers) {
    return request("PUT", url, headers, body)
}

// patch serialises body as JSON and performs a PATCH.
fn patch(url, body) {
    return request("PATCH", url, null, body)
}

// patchWith(url, body, headers) — PATCH with body serialised as JSON and custom request headers hash.
fn patchWith(url, body, headers) {
    return request("PATCH", url, headers, body)
}

// del performs a DELETE request.
fn del(url) {
    return request("DELETE", url, null, null)
}

// delWith(url, headers) — DELETE with custom request headers hash.
fn delWith(url, headers) {
    return request("DELETE", url, headers, null)
}

// isOk returns true if the response status is in the 2xx range.
fn isOk(resp) {
    return resp.status >= 200 && resp.status < 300
}


// ============================================================================
// AUTHENTICATION HELPERS
// ============================================================================

// bearerToken returns the Authorization header value for Bearer token auth.
// Usage:
//   headers = {}
//   headers["Authorization"] = rest.bearerToken("mytoken123")
//   resp, err = rest.getWith(url, headers)
fn bearerToken(token) {
    return "Bearer " + token
}

// basicAuth returns the Authorization header value for HTTP Basic auth.
// Encodes username:password in base64 as per RFC 7617.
// Usage:
//   headers = {}
//   headers["Authorization"] = rest.basicAuth("user", "pass")
//   resp, err = rest.getWith(url, headers)
fn basicAuth(username, password) {
    let credentials = username + ":" + password
    let encoded = _b64.encode(credentials)
    return "Basic " + encoded
}

// apiKeyHeader returns the value for an API key header.
// User specifies the header name when passing to request.
// Usage:
//   headers = {}
//   headers["X-API-Key"] = rest.apiKeyHeader("abc123")
//   resp, err = rest.getWith(url, headers)
fn apiKeyHeader(key) {
    return key
}


// ============================================================================
// CONVENIENCE WRAPPERS WITH AUTH
// ============================================================================

// getBearer performs a GET request with Bearer token authentication.
fn getBearer(url, token) {
    let headers = {}
    headers["Authorization"] = bearerToken(token)
    return getWith(url, headers)
}

// postBearer performs a POST request with Bearer token authentication.
fn postBearer(url, body, token) {
    let headers = {}
    headers["Authorization"] = bearerToken(token)
    return postWith(url, body, headers)
}

// putBearer performs a PUT request with Bearer token authentication.
fn putBearer(url, body, token) {
    let headers = {}
    headers["Authorization"] = bearerToken(token)
    return putWith(url, body, headers)
}

// patchBearer performs a PATCH request with Bearer token authentication.
fn patchBearer(url, body, token) {
    let headers = {}
    headers["Authorization"] = bearerToken(token)
    return patchWith(url, body, headers)
}

// delBearer performs a DELETE request with Bearer token authentication.
fn delBearer(url, token) {
    let headers = {}
    headers["Authorization"] = bearerToken(token)
    return delWith(url, headers)
}


// getBasic performs a GET request with HTTP Basic authentication.
fn getBasic(url, username, password) {
    let headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return getWith(url, headers)
}

// postBasic performs a POST request with HTTP Basic authentication.
fn postBasic(url, body, username, password) {
    let headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return postWith(url, body, headers)
}

// putBasic performs a PUT request with HTTP Basic authentication.
fn putBasic(url, body, username, password) {
    let headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return putWith(url, body, headers)
}

// patchBasic performs a PATCH request with HTTP Basic authentication.
fn patchBasic(url, body, username, password) {
    let headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return patchWith(url, body, headers)
}

// delBasic performs a DELETE request with HTTP Basic authentication.
fn delBasic(url, username, password) {
    let headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return delWith(url, headers)
}
