// stdlib/vfs.lex — in-memory block store
// @module    vfs
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   in-memory block store
//
// A lightweight key-value store backed by an integer-indexed hash.
// Slots are addressed by integer ID; use save_fragment / load_fragment
// to persist data across calls within the same process lifetime.
// Data does not survive process exit — this is intentional for temporary
// or test workloads. For persistent storage use stdlib/fs.lex.

let vfs_blocks = {}

// init_vfs() — initialise the in-memory block store. Prints a confirmation message.
// Call once before any save_fragment / load_fragment calls.
fn init_vfs() {
    println("VFS: Storage pond initialized.")
}

// save_fragment(id, data) — store data at integer slot id. Overwrites any previous value.
fn save_fragment(id, data) {
    vfs_blocks[id] = data
    return true
}

// load_fragment(id) — retrieve data from integer slot id. Returns null if the slot is empty.
fn load_fragment(id) {
    return vfs_blocks[id]
}

// has_fragment(id) — return true if integer slot id contains data.
fn has_fragment(id) {
    return vfs_blocks[id] != null
}

// list_fragments() — return an array of integer IDs for all occupied slots (0..9).
fn list_fragments() {
    // Count occupied slots first, then fill — two-pass to avoid push() antipattern.
    let count = 0
    for id in range(0, 10) {
        if vfs_blocks[id] != null { count = count + 1 }
    }
    let ids = makeArray(count)
    let idx = 0
    for id in range(0, 10) {
        if vfs_blocks[id] != null {
            ids[idx] = id
            idx = idx + 1
        }
    }
    return ids
}
