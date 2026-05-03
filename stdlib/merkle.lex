import "hash.lex" as h

CHUNK_SIZE = 16

fn chunk_count(content, chunk_size) {
    n = len(content) / chunk_size
    if len(content) % chunk_size != 0 {
        n = n + 1
    }
    return n
}

fn next_power_of_2(n) {
    p = 1
    while p < n {
        p = p * 2
    }
    return p
}

fn build_leaves(content, chunk_size) {
    count = chunk_count(content, chunk_size)
    padded = next_power_of_2(count)
    leaves = []
    i = 0
    while i < padded {
        if i < count {
            start = i * chunk_size
            end = start + chunk_size
            if end > len(content) { end = len(content) }
            chunk = substr(content, start, end)
            leaves = push(leaves, h.hash(chunk))
        } else {
            leaves = push(leaves, 0)
        }
        i = i + 1
    }
    return leaves
}

fn tree_root(content, chunk_size) {
    nodes = build_leaves(content, chunk_size)
    while len(nodes) > 1 {
        next = []
        i = 0
        while i < len(nodes) {
            combined = h.combineHash(nodes[i], nodes[i + 1])
            next = push(next, combined)
            i = i + 2
        }
        nodes = next
    }
    return nodes[0]
}

fn get_proof(content, chunk_idx, chunk_size) {
    nodes = build_leaves(content, chunk_size)

    start = chunk_idx * chunk_size
    end = start + chunk_size
    if end > len(content) { end = len(content) }
    chunk = substr(content, start, end)
    chunk_hash = h.hash(chunk)

    path = []
    idx = chunk_idx
    while len(nodes) > 1 {
        if idx % 2 == 0 {
            sibling = nodes[idx + 1]
        } else {
            sibling = nodes[idx - 1]
        }
        path = push(path, sibling)

        next = []
        i = 0
        while i < len(nodes) {
            combined = h.combineHash(nodes[i], nodes[i + 1])
            next = push(next, combined)
            i = i + 2
        }
        nodes = next
        idx = idx / 2
    }

    return [chunk_hash, path]
}

fn verify(chunk_hash, chunk_idx, path, expected_root) {
    h_val = chunk_hash
    idx = chunk_idx
    i = 0
    while i < len(path) {
        sibling = path[i]
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
