// VFS: In-memory block storage (Phase 1 implementation)
// Note: File I/O builtins are not yet implemented in kLex,
// so blocks are stored in memory for now. This is sufficient for Phase 1.

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
    let ids = []
    for id in range(0, 10) {
        if vfs_blocks[id] != null {
            ids = push(ids, id)
        }
    }
    return ids
}