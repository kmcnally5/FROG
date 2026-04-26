// url.lex
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
    ks = keys(params)
    if len(ks) == 0 { return base }

    parts = []
    i = 0
    while i < len(ks) {
        k = ks[i]
        v = str(params[k])
        parts = push(parts, _urlEncode(k) + "=" + _urlEncode(v))
        i = i + 1
    }
    return base + "?" + join(parts, "&")
}

// joinPath joins a base URL and a path segment, handling slashes correctly.
//   joinPath("https://api.example.com", "users/42")  → "https://api.example.com/users/42"
//   joinPath("https://api.example.com/", "/users/42") → "https://api.example.com/users/42"
fn joinPath(base, path) {
    if endsWith(base, "/") {
        bl = len(base) - 1
        trimmed = ""
        i = 0
        while i < bl {
            trimmed = trimmed + base[i]
            i = i + 1
        }
        base = trimmed
    }
    if !startsWith(path, "/") {
        path = "/" + path
    }
    return base + path
}
