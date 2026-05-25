// stdlib/cache.lex — Cache struct + AI response-cache helpers.
// @module    cache
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Cache struct + AI response-cache helpers.
//
// Replaces the former global singleton with an instantiable cache.
// Multiple independent caches can coexist in the same program.
//
// Usage — generic KV:
//   import "stdlib/cache.lex" as cache
//   c = cache.newCache()
//   c.set("key", 42)
//   println(c.get("key"))   // 42
//
// Usage — AI response caching (skip the API call when inputs unchanged):
//   import "stdlib/cache.lex"        as cache
//   import "stdlib/ai/anthropic.lex" as claude
//
//   c    = cache.newCache()
//   req  = {"messages": [ai.userMsg("Hi")], "max_tokens": 64}
//   key  = cache.keyOf(req)
//   resp, err = c.wrap(key, fn() { return claude.messages(client, req) })
//
// keyOf() produces a deterministic SHA256 over canonical-JSON (hash keys
// are sorted), so the same logical request always yields the same key.
// wrap() returns the cached value on hit, otherwise calls f and caches the
// (non-error) result. Errors are never cached — callers retry naturally.

struct Cache {
    data

    // set(key, value) — store a value under key.
    fn set(key, value) {
        self.data[key] = value
        return null
    }

    // get(key) — retrieve the value for key, or null if not present.
    fn get(key) {
        if hasKey(self.data, key) {
            return self.data[key]
        }
        return null
    }

    // has(key) — return true if key exists in the cache.
    fn has(key) {
        return hasKey(self.data, key)
    }

    // del(key) — remove key from the cache. No-op if key absent.
    fn del(key) {
        if hasKey(self.data, key) {
            delete(self.data, key)
        }
        return null
    }

    // clear() — remove all entries.
    fn clear() {
        let ks = keys(self.data)
        let i = 0
        while i < len(ks) {
            delete(self.data, ks[i])
            i = i + 1
        }
        return null
    }

    // size() — return the number of entries.
    fn size() {
        return len(self.data)
    }

    // save(path) — persist cache to a file (pipe-delimited, one entry per line).
    fn save(path) {
        let ks = keys(self.data)
        let content = ""
        let i = 0
        while i < len(ks) {
            let k = ks[i]
            let v = str(self.data[k])
            content = content + k + "|" + v + "\n"
            i = i + 1
        }
        writeFile(path, content)
        return null
    }

    // load(path) — load entries from a file written by save(). Merges into existing data.
    fn load(path) {
        let content = readFile(path)
        let lines = split(content, "\n")
        let i = 0
        while i < len(lines) {
            let line = lines[i]
            if line != "" {
                let parts = split(line, "|")
                if len(parts) == 2 {
                    self.data[parts[0]] = parts[1]
                }
            }
            i = i + 1
        }
        return null
    }

    // wrap(key, f) — cached call wrapper for (val, err) producers.
    //
    // Returns (cached, null) on hit; otherwise invokes f(), caches the
    // value on success, and returns (val, err) unchanged. Errors are
    // never cached so transient failures get a fresh attempt next time.
    //
    // Use cache.keyOf(req) to derive a deterministic key from a request
    // hash — see file header for the AI-response-cache pattern.
    fn wrap(key, f) {
        if hasKey(self.data, key) {
            return self.data[key], null
        }
        let result, err = f()
        if err == null {
            self.data[key] = result
        }
        return result, err
    }

    // memoize(fnRef) — return a cached version of fnRef.
    // First call with a given argument list computes and stores the result.
    // Subsequent calls with the same arguments return the cached value.
    // Supports 0–3 arguments.
    fn memoize(fnRef) {
        let c = self
        return fn(args...) {
            let key = str(args)
            if c.has(key) {
                return c.get(key)
            }
            let result = null
            if len(args) == 0 {
                result = fnRef()
            } else if len(args) == 1 {
                result = fnRef(args[0])
            } else if len(args) == 2 {
                result = fnRef(args[0], args[1])
            } else if len(args) == 3 {
                result = fnRef(args[0], args[1], args[2])
            } else {
                return null, "memoize: too many arguments (max 3)"
            }
            c.set(key, result)
            return result
        }
    }
}

// newCache() returns a fresh empty Cache instance.
fn newCache() {
    return Cache { data: {} }
}


// keyOf(req) → SHA256 hex string.
//
// Produces a deterministic key from any kLex value — typically an AI
// request hash (model + messages + params). Hash keys are sorted before
// stringification so {a:1, b:2} and {b:2, a:1} always produce the same
// key. Arrays preserve their order (positional semantics).
//
// Use as the key for cache.wrap() when caching AI responses by request
// shape — see file header for the full pattern.
fn keyOf(req) {
    return _sha256(_canonStr(req))
}


// _canonStr(v) → string.
//
// Internal helper. Produces a JSON-shaped string with deterministic
// ordering: hash keys are sorted, arrays preserve insertion order,
// strings are escaped. The output is intended for hashing, not parsing
// — it's similar to JSON but not strictly compliant (no whitespace,
// minimal escapes). Stable across runs is the only guarantee.
fn _canonStr(v) {
    let t = type(v)
    if t == "NULL"    { return "null" }
    if t == "BOOLEAN" {
        if v { return "true" }
        return "false"
    }
    if t == "INTEGER" || t == "FLOAT" {
        return str(v)
    }
    if t == "STRING" {
        return "\"" + _canonEsc(v) + "\""
    }
    if t == "ARRAY" {
        let n     = len(v)
        let parts = makeArray(n, "")
        let i     = 0
        while i < n {
            parts[i] = _canonStr(v[i])
            i = i + 1
        }
        return "[" + join(parts, ",") + "]"
    }
    if t == "HASH" {
        let ks    = sort(keys(v))
        let n     = len(ks)
        let parts = makeArray(n, "")
        let i     = 0
        while i < n {
            let k = ks[i]
            parts[i] = "\"" + _canonEsc(k) + "\":" + _canonStr(v[k])
            i = i + 1
        }
        return "\{" + join(parts, ",") + "\}"
    }
    // Fallback for any other type (function, struct instance, etc.) —
    // stringify and quote. Not great for stability across renames; AI
    // request hashes should stick to JSON-compatible primitives.
    return "\"" + _canonEsc(str(v)) + "\""
}

fn _canonEsc(s) {
    s = replace(s, "\\", "\\\\")
    s = replace(s, "\"", "\\\"")
    s = replace(s, "\n", "\\n")
    s = replace(s, "\r", "\\r")
    s = replace(s, "\t", "\\t")
    return s
}
