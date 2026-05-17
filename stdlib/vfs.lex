// stdlib/vfs.lex — in-memory block store
//
// A lightweight key-value store backed by an integer-indexed hash.
// Slots are addressed by integer ID; use save_fragment / load_fragment
// to persist data across calls within the same process lifetime.
// Data does not survive process exit — this is intentional for temporary
// or test workloads. For persistent storage use stdlib/fs.lex.

let vfs_blocks = {}

fn init_vfs() {
    println("VFS: Storage pond initialized.")
}

fn save_fragment(id, data) {
    vfs_blocks[id] = data
    return true
}

fn load_fragment(id) {
    return vfs_blocks[id]
}

fn has_fragment(id) {
    return vfs_blocks[id] != null
}

fn list_fragments() {
    // Count occupied slots first, then fill — two-pass to avoid push() antipattern.
    count = 0
    for id in range(0, 10) {
        if vfs_blocks[id] != null { count = count + 1 }
    }
    ids = makeArray(count)
    idx = 0
    for id in range(0, 10) {
        if vfs_blocks[id] != null {
            ids[idx] = id
            idx = idx + 1
        }
    }
    return ids
}
