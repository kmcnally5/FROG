// stdlib/kv_store.lex — in-memory key/value store with optional persistence
//
// KVStore is an instantiable store backed by a Cache for repeated reads.
// Multiple independent stores can coexist in the same program.
//
// Usage:
//   import "kv_store.lex" as kv
//   store = kv.newKVStore()
//   store.set("x", 42)
//   println(store.get("x"))   // 42

import "cache.lex" as cache_mod

struct KVStore {
    store
    cache

    // set(key, value) — write key to both the store and the cache.
    fn set(key, value) {
        self.store[key] = value
        self.cache.set(key, value)
        return null
    }

    // get(key) — read key, checking cache first, then the store.
    fn get(key) {
        val = self.cache.get(key)
        if val != null {
            return val
        }
        if hasKey(self.store, key) {
            return self.store[key]
        }
        return null
    }

    // del(key) — remove key from both the store and the cache.
    fn del(key) {
        delete(self.store, key)
        self.cache.del(key)
        return null
    }

    // allKeys() — return all keys in the store as an array.
    fn allKeys() {
        return keys(self.store)
    }

    // allValues() — return all values in the store as an array.
    fn allValues() {
        ks = keys(self.store)
        out = []
        i = 0
        while i < len(ks) {
            out = push(out, self.store[ks[i]])
            i = i + 1
        }
        return out
    }

    // save(path) — persist the store to a file (pipe-delimited, one entry per line).
    fn save(path) {
        ks = keys(self.store)
        content = ""
        i = 0
        while i < len(ks) {
            k = ks[i]
            v = str(self.store[k])
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
                    self.store[parts[0]] = parts[1]
                    self.cache.set(parts[0], parts[1])
                }
            }
            i = i + 1
        }
        return null
    }

    // mapValues(fnRef) — apply fnRef to every value, returning an array of results.
    fn mapValues(fnRef) {
        store = self.store
        ks = keys(store)
        return map(ks, fn(k) {
            return fnRef(store[k])
        })
    }

    // filterKeys(fnRef) — return keys where fnRef(key, value) is true.
    fn filterKeys(fnRef) {
        store = self.store
        ks = keys(store)
        return filter(ks, fn(k) {
            return fnRef(k, store[k])
        })
    }

    // reduceStore(fnRef, initial) — fold over all (key, value) pairs.
    fn reduceStore(fnRef, initial) {
        store = self.store
        ks = keys(store)
        return reduce(ks, fn(acc, k) {
            return fnRef(acc, k, store[k])
        }, initial)
    }
}

// newKVStore() — returns a fresh empty KVStore with its own Cache.
fn newKVStore() {
    return KVStore { store: {}, cache: cache_mod.newCache() }
}
