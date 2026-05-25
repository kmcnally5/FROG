// vector_store.lex — in-memory vector store for kLex.
// @module    vector_store
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   in-memory vector store for kLex.
//
// A small, dependency-free K-nearest-neighbour store designed to feed
// retrieval-augmented generation (RAG), semantic search, and clustering
// workflows. Vectors come from any embedding source — Ollama's
// `ollama.embed()` is the default in stdlib/ai, but any provider that
// returns a float array works.
//
// Storage: linear-scan. Suitable for up to ~10k entries with modest
// embedding dimensions (e.g. 768). For larger corpora you'd want HNSW
// or a hosted vector DB (Chroma, Qdrant) — those can wrap this same
// API via a bridge in a future revision.
//
// Usage:
//   import "stdlib/ai/vector_store.lex" as vs
//   import "stdlib/ai/ollama.lex"       as ollama
//
//   c     = ollama.newClient(null, "nomic-embed-text")
//   store = vs.newStore()
//
//   // Add documents (any embedding source works; here we use Ollama)
//   docs = ["the quick brown fox", "a swift fox leaps", "the lazy dog sleeps"]
//   i = 0
//   while i < len(docs) {
//       resp, _ = ollama.embed(c, docs[i])
//       store.add("doc" + str(i), resp["embeddings"][0], {"text": docs[i]})
//       i = i + 1
//   }
//
//   // Query
//   q, _   = ollama.embed(c, "fast canine")
//   hits   = store.search(q["embeddings"][0], 2)
//   for h in hits {
//       println(str(h["score"]) + "  " + h["metadata"]["text"])
//   }


// Entry is one stored vector + its metadata. The user-facing API hides
// this type — callers interact with the VectorStore struct.
struct _Entry {
    id, vector, metadata
}


// VectorStore — in-memory K-NN store.
//
// Public API:
//   store.add(id, vector, metadata)  — add or replace an entry
//   store.delete(id)                 — remove by id; returns bool
//   store.get(id)                    — fetch entry hash by id; null if absent
//   store.search(query, k)           — top-k by cosine similarity
//   store.searchScored(query, minScore) — all entries with score ≥ minScore
//   store.count()                    — entry count
//   store.clear()                    — wipe all entries
//   store.ids()                      — array of all stored ids
struct VectorStore {
    entries

    // _findIdx returns the index of an entry by id, or -1.
    fn _findIdx(id) {
        let i = 0
        let n = len(self.entries)
        while i < n {
            if self.entries[i].id == id { return i }
            i = i + 1
        }
        return -1
    }

    // add inserts or replaces an entry. metadata may be any kLex value
    // (typically a hash with text + source info).
    fn add(id, vector, metadata) {
        let e = _Entry { id: id, vector: vector, metadata: metadata }
        let idx = self._findIdx(id)
        if idx >= 0 {
            self.entries[idx] = e
        } else {
            self.entries = concat(self.entries, [e])
        }
        return null
    }

    // delete removes the entry with the given id. Returns true if an
    // entry was removed, false if no such id was stored.
    fn delete(id) {
        let idx = self._findIdx(id)
        if idx < 0 { return false }
        let out = makeArray(0)
        let i = 0
        let n = len(self.entries)
        while i < n {
            if i != idx { out = concat(out, [self.entries[i]]) }
            i = i + 1
        }
        self.entries = out
        return true
    }

    // get returns {id, vector, metadata} hash for the given id, or null
    // if absent.
    fn get(id) {
        let idx = self._findIdx(id)
        if idx < 0 { return null }
        let e = self.entries[idx]
        return {"id": e.id, "vector": e.vector, "metadata": e.metadata}
    }

    // count returns the number of stored entries.
    fn count() { return len(self.entries) }

    // ids returns an array of all stored ids, in insertion order.
    fn ids() {
        let out = makeArray(len(self.entries), "")
        let i = 0
        let n = len(self.entries)
        while i < n {
            out[i] = self.entries[i].id
            i = i + 1
        }
        return out
    }

    // clear removes all entries.
    fn clear() {
        self.entries = makeArray(0)
        return null
    }

    // search returns the top k entries ranked by cosine similarity to
    // the query vector. Each result is a hash {id, score, metadata}.
    // Returns an empty array if the store is empty or k <= 0.
    fn search(queryVector, k) {
        if k <= 0 { return makeArray(0) }
        let scored = self._scoreAll(queryVector)
        scored = _sortByScoreDesc(scored)
        let topK = k
        if topK > len(scored) { topK = len(scored) }
        let out = makeArray(topK, null)
        let i = 0
        while i < topK {
            out[i] = scored[i]
            i = i + 1
        }
        return out
    }

    // searchScored returns ALL entries whose cosine similarity to the
    // query is at least `minScore`. Results are sorted descending.
    // Use when you want a relevance cutoff rather than a fixed k.
    fn searchScored(queryVector, minScore) {
        let scored = self._scoreAll(queryVector)
        let kept = makeArray(0)
        for s in scored {
            if s["score"] >= minScore { kept = concat(kept, [s]) }
        }
        return _sortByScoreDesc(kept)
    }

    // _scoreAll computes cosine similarity from queryVector to every
    // entry. Returns array of {id, score, metadata} hashes.
    fn _scoreAll(queryVector) {
        let out = makeArray(len(self.entries), null)
        let i = 0
        let n = len(self.entries)
        while i < n {
            let e = self.entries[i]
            out[i] = {
                "id":       e.id,
                "score":    cosineSim(queryVector, e.vector),
                "metadata": e.metadata,
            }
            i = i + 1
        }
        return out
    }
}


// newStore returns an empty VectorStore.
fn newStore() {
    return VectorStore { entries: makeArray(0) }
}


// _sortByScoreDesc sorts an array of {score, …} hashes by score
// descending. Uses stdlib insertion sort — fine for the result sizes
// vector search produces (typically tens to hundreds).
fn _sortByScoreDesc(arr) {
    let n = len(arr)
    if n <= 1 { return arr }
    let out = makeArray(n, null)
    let i = 0
    while i < n {
        out[i] = arr[i]
        i = i + 1
    }
    // Insertion sort, descending by score.
    i = 1
    while i < n {
        let cur = out[i]
        let j = i - 1
        while j >= 0 && out[j]["score"] < cur["score"] {
            out[j + 1] = out[j]
            j = j - 1
        }
        out[j + 1] = cur
        i = i + 1
    }
    return out
}
