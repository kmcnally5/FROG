// url.lex
// @module    url
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   URL building and encoding utilities for kLex.
// URL building and encoding utilities for kLex.
//
// Usage:
//   import "url.lex" as url
//   full = url.build("https://api.example.com/search", {"q": "hello world", "page": "1"})
//   // → "https://api.example.com/search?q=hello+world&page=1"

// encode percent-encodes a single query string component.
// Spaces become +, special characters become %XX.
fn encode(s) {
    return _urlEncode(s)
}

// decode decodes a percent-encoded query string component.
// Returns (string, err).
fn decode(s) {
    return _urlDecode(s)
}

// build appends a hash of query parameters to a base URL.
// Values are automatically converted to strings and percent-encoded.
// Returns the base URL unchanged if params is empty.
fn build(base, params) {
    let ks = keys(params)
    let n  = len(ks)
    if n == 0 { return base }

    let parts = makeArray(n, "")
    let i = 0
    while i < n {
        let k = ks[i]
        let v = str(params[k])
        parts[i] = _urlEncode(k) + "=" + _urlEncode(v)
        i = i + 1
    }
    return base + "?" + join(parts, "&")
}

// joinPath joins a base URL and a path segment, handling slashes correctly.
//   joinPath("https://api.example.com", "users/42")  → "https://api.example.com/users/42"
//   joinPath("https://api.example.com/", "/users/42") → "https://api.example.com/users/42"
fn joinPath(base, path) {
    if endsWith(base, "/") {
        base = substr(base, 0, len(base) - 1)
    }
    if !startsWith(path, "/") {
        path = "/" + path
    }
    return base + path
}
