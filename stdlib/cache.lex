// stdlib/cache.lex — Cache struct
//
// Replaces the former global singleton with an instantiable cache.
// Multiple independent caches can coexist in the same program.
//
// Usage:
//   import "cache.lex" as cache
//   c = cache.newCache()
//   c.set("key", 42)
//   println(c.get("key"))   // 42

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
        ks = keys(self.data)
        i = 0
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
        ks = keys(self.data)
        content = ""
        i = 0
        while i < len(ks) {
            k = ks[i]
            v = str(self.data[k])
            content = content + k + "|" + v + "\n"
            i = i + 1
        }
        writeFile(path, content)
        return null
    }

    // load(path) — load entries from a file written by save(). Merges into existing data.
    fn load(path) {
        content = readFile(path)
        lines = split(content, "\n")
        i = 0
        while i < len(lines) {
            line = lines[i]
            if line != "" {
                parts = split(line, "|")
                if len(parts) == 2 {
                    self.data[parts[0]] = parts[1]
                }
            }
            i = i + 1
        }
        return null
    }

    // memoize(fnRef) — return a cached version of fnRef.
    // First call with a given argument list computes and stores the result.
    // Subsequent calls with the same arguments return the cached value.
    // Supports 0–3 arguments.
    fn memoize(fnRef) {
        c = self
        return fn(args...) {
            key = str(args)
            if c.has(key) {
                return c.get(key)
            }
            result = null
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
