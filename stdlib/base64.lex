// base64.lex
// Base64 encoding and decoding for kLex.
//
// Standard base64 uses A-Z, a-z, 0-9, +, / with = padding.
// URL-safe base64 uses A-Z, a-z, 0-9, -, _ with no padding.
// Use URL-safe when embedding in URLs, HTTP headers, or JWT tokens.
//
// Usage:
//   import "base64.lex" as b64
//   encoded = b64.encode("hello")          // "aGVsbG8="
//   decoded, err = b64.decode(encoded)     // "hello", null

// encode encodes a string to standard base64.
fn encode(s) {
    return _base64Encode(s)
}

// decode decodes a standard base64 string. Returns (string, err).
fn decode(s) {
    return _base64Decode(s)
}

// urlEncode encodes a string to URL-safe base64 (no padding).
fn urlEncode(s) {
    return _base64UrlEncode(s)
}

// urlDecode decodes a URL-safe base64 string. Returns (string, err).
fn urlDecode(s) {
    return _base64UrlDecode(s)
}
