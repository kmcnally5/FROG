// @module    merkle
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   Merkle-tree chunk hashing for proof-of-custody

import "stdlib/hash.lex" as h

let CHUNK_SIZE = 16

// chunk_count(content, chunk_size) — return the number of fixed-size chunks in content string.
// The last chunk may be shorter than chunk_size.
fn chunk_count(content, chunk_size) {
    let n = len(content) / chunk_size
    if len(content) % chunk_size != 0 {
        n = n + 1
    }
    return n
}

// next_power_of_2(n) — return the smallest power of 2 that is >= n. Used to pad the leaf layer.
fn next_power_of_2(n) {
    let p = 1
    while p < n {
        p = p * 2
    }
    return p
}

// build_leaves(content, chunk_size) — hash each chunk of content and return the padded leaf array.
// Leaf count is rounded up to the next power of 2; empty slots are hashed as 0.
fn build_leaves(content, chunk_size) {
    let count  = chunk_count(content, chunk_size)
    let padded = next_power_of_2(count)
    let leaves = makeArray(padded, 0)
    let i = 0
    while i < padded {
        if i < count {
            let start = i * chunk_size
            let end   = start + chunk_size
            if end > len(content) { end = len(content) }
            leaves[i] = h.hash(substr(content, start, end))
        }
        // i >= count slots already initialised to 0 by makeArray
        i = i + 1
    }
    return leaves
}

// tree_root(content, chunk_size) — compute the Merkle root hash of content split into chunk_size chunks.
fn tree_root(content, chunk_size) {
    let nodes = build_leaves(content, chunk_size)
    while len(nodes) > 1 {
        let nextLen = len(nodes) / 2
        let next    = makeArray(nextLen, null)
        let i = 0
        let j = 0
        while i < len(nodes) {
            next[j] = h.combineHash(nodes[i], nodes[i + 1])
            j = j + 1
            i = i + 2
        }
        nodes = next
    }
    return nodes[0]
}

// get_proof(content, chunk_idx, chunk_size) — generate a Merkle inclusion proof for chunk_idx.
// Returns [chunk_hash, path] where path is the sibling hashes from leaf to root.
fn get_proof(content, chunk_idx, chunk_size) {
    let nodes = build_leaves(content, chunk_size)

    let start = chunk_idx * chunk_size
    let end   = start + chunk_size
    if end > len(content) { end = len(content) }
    let chunk_hash = h.hash(substr(content, start, end))

    // Number of proof entries = number of tree levels above the leaves =
    // log₂(len(nodes)). Pre-compute so path is a single allocation.
    let levels = 0
    let m = len(nodes)
    while m > 1 {
        levels = levels + 1
        m = m / 2
    }
    let path = makeArray(levels, null)
    let pathIdx = 0

    let idx = chunk_idx
    while len(nodes) > 1 {
        if idx % 2 == 0 {
            path[pathIdx] = nodes[idx + 1]
        } else {
            path[pathIdx] = nodes[idx - 1]
        }
        pathIdx = pathIdx + 1

        let nextLen = len(nodes) / 2
        let next    = makeArray(nextLen, null)
        let i = 0
        let j = 0
        while i < len(nodes) {
            next[j] = h.combineHash(nodes[i], nodes[i + 1])
            j = j + 1
            i = i + 2
        }
        nodes = next
        idx   = idx / 2
    }

    return [chunk_hash, path]
}

// verify(chunk_hash, chunk_idx, path, expected_root) — verify a Merkle inclusion proof.
// Returns true if chunk_hash at chunk_idx produces expected_root via the sibling path.
fn verify(chunk_hash, chunk_idx, path, expected_root) {
    let h_val = chunk_hash
    let idx = chunk_idx
    let i = 0
    while i < len(path) {
        let sibling = path[i]
        if idx % 2 == 0 {
            h_val = h.combineHash(h_val, sibling)
        } else {
            h_val = h.combineHash(sibling, h_val)
        }
        idx = idx / 2
        i = i + 1
    }
    return h_val == expected_root
}
