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

import "http.lex" as _http
import "json.lex" as _json
import "base64.lex" as _b64

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
    credentials = username + ":" + password
    encoded = _b64.encode(credentials)
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
    headers = {}
    headers["Authorization"] = bearerToken(token)
    return getWith(url, headers)
}

// postBearer performs a POST request with Bearer token authentication.
fn postBearer(url, body, token) {
    headers = {}
    headers["Authorization"] = bearerToken(token)
    return postWith(url, body, headers)
}

// putBearer performs a PUT request with Bearer token authentication.
fn putBearer(url, body, token) {
    headers = {}
    headers["Authorization"] = bearerToken(token)
    return putWith(url, body, headers)
}

// patchBearer performs a PATCH request with Bearer token authentication.
fn patchBearer(url, body, token) {
    headers = {}
    headers["Authorization"] = bearerToken(token)
    return patchWith(url, body, headers)
}

// delBearer performs a DELETE request with Bearer token authentication.
fn delBearer(url, token) {
    headers = {}
    headers["Authorization"] = bearerToken(token)
    return delWith(url, headers)
}


// getBasic performs a GET request with HTTP Basic authentication.
fn getBasic(url, username, password) {
    headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return getWith(url, headers)
}

// postBasic performs a POST request with HTTP Basic authentication.
fn postBasic(url, body, username, password) {
    headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return postWith(url, body, headers)
}

// putBasic performs a PUT request with HTTP Basic authentication.
fn putBasic(url, body, username, password) {
    headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return putWith(url, body, headers)
}

// patchBasic performs a PATCH request with HTTP Basic authentication.
fn patchBasic(url, body, username, password) {
    headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return patchWith(url, body, headers)
}

// delBasic performs a DELETE request with HTTP Basic authentication.
fn delBasic(url, username, password) {
    headers = {}
    headers["Authorization"] = basicAuth(username, password)
    return delWith(url, headers)
}
