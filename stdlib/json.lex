// json.lex
// @module    json
// @version   2.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   JSON parser + stringifier for kLex (Go-backed)
//
// HISTORY: This module used to be a hand-rolled parser written in kLex
// itself (~350 LOC of interpreted character-by-character scanning).
// That was catastrophically slow at scale — replaying a 1M-line JSONL
// could block for 30+ seconds because every peek/next/parseValue ran
// through the kLex evaluator. OFI #14 replaced the body with thin
// wrappers around the Go-side `_jsonParse` / `_jsonStringify` builtins
// (eval/builtins_json.go) which use Go's encoding/json under the hood.
// Same public API, ~100× speedup.
//
// PUBLIC API (unchanged from v1):
//
//   parse(input: string) -> (value, err: string?)
//     Decodes a JSON document. Returns (value, null) on success or
//     (null, "<message>") on parse failure. Numbers are decoded as
//     integer if the literal has no '.' or 'e'/'E' and fits in int64,
//     otherwise as float.
//
//   stringify(v: any) -> string
//     Encodes a kLex value. Hash keys are sorted for stable output.
//     Bytes / NaN / Inf are not representable and surface as a kLex
//     runtime error (matches the historical behaviour of crashing on
//     bad input). Call _jsonStringify directly if you want the
//     (string, err) tuple form to handle bad input non-fatally.

// parse(input) — decode a JSON string. Returns (value, null) on success or (null, errMsg) on failure.
// Numbers without '.' or 'e' that fit in int64 are decoded as integer; otherwise float.
fn parse(input) {
    return _jsonParse(input)
}

// stringify(v) — encode a kLex value as a JSON string. Hash keys are sorted for stable output.
// Raises a runtime error on unencodable values (NaN, Inf). Use _jsonStringify for the safe (string, err) form.
fn stringify(v) {
    let s, err = _jsonStringify(v)
    if err != null {
        // Match the historical kLex implementation's behaviour: bad
        // input surfaces as a runtime error rather than a tuple. The
        // raw builtin is available for callers who want the safe form.
        return error("JSON_STRINGIFY", err)
    }
    return s
}
