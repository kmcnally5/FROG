// dedup.lex - High-Performance Data Deduplication Engine
// Finds duplicate files across directory trees using parallel hashing and lock-free aggregation.
// Uses full SHA-256 hashing for all files to guarantee correctness.
//
// Architecture: 5-phase pipeline:
//   1. Walk & Collect: async recursive walk → channel → 8 collector workers → all_files array
//   2. Size-Group: group files by size → filter candidates (≥2 files per size only)
//   3. Parallel Hash: 16 workers hash candidate files concurrently with full SHA-256
//   4. Hash-Group: group by hash → confirmed duplicate sets
//   5. Report: sort by wasted space descending → print duplicate groups + summary
//
// Demonstrates:
//   - Streaming directory walks with channels (constant memory)
//   - Lock-free collection via atomic index counter (no O(n²) push in loops)
//   - Parallel SHA-256 hashing with 16 concurrent workers (full content hashing)
//   - sortBy() for custom ordering (by wasted space descending)
//
// Usage:
//   ./klex tests/examples/Dedup/dedup.lex /path/to/scan [/another ...]
//   ./klex tests/examples/Dedup/dedup.lex --quiet --min-size 102400 ~/Documents
//
// Flags:
//   --quiet       summary only (no per-group output)
//   --min-size N  skip files smaller than N bytes (default: 1)
//   --workers N   number of hash workers (default: 16)

// ===================================================================
// UTILITY FUNCTIONS
// ===================================================================

fn shouldSkipDir(path) {
    n = len(path)
    i = n - 1
    while i >= 0 && path[i] != "/" {
        i = i - 1
    }
    name = substr(path, i + 1)

    skip = {
        ".git": true,
        "node_modules": true,
        ".Trash": true,
        "__pycache__": true,
        ".cache": true,
        "vendor": true,
        ".next": true,
        "build": true,
        "dist": true
    }
    return skip[name] == true
}

fn humanSize(bytes) {
    if bytes >= 1073741824 {
        return str(bytes / 1073741824) + " GB"
    }
    if bytes >= 1048576 {
        return str(bytes / 1048576) + " MB"
    }
    if bytes >= 1024 {
        return str(bytes / 1024) + " KB"
    }
    return str(bytes) + " B"
}

fn hashFile(path, size) {
    content, err = _fsRead(path)
    if err != null {
        return null
    }
    return _sha256(content)
}

fn repeat(char, count) {
    result = ""
    i = 0
    while i < count {
        result = result + char
        i = i + 1
    }
    return result
}

// ===================================================================
// DIRECTORY WALK
// ===================================================================

fn walk_files_streaming_recursive(path, ch) {
    info, err = _fsStat(path)
    if err != null {
        return
    }

    if info["isDir"] == false {
        meta = makeArray(2, null)
        meta[0] = path
        meta[1] = info["size"]
        send(ch, meta)
        return
    }

    if shouldSkipDir(path) {
        return
    }

    entries, list_err = _fsReadDir(path)
    if list_err != null {
        return
    }

    i = 0
    while i < len(entries) {
        entry = entries[i]
        full_path = path + "/" + entry["name"]
        if entry["isDir"] {
            walk_files_streaming_recursive(full_path, ch)
        } else {
            meta = makeArray(2, null)
            meta[0] = full_path
            meta[1] = entry["size"]
            send(ch, meta)
        }
        i = i + 1
    }
}

// ===================================================================
// MAIN
// ===================================================================

fn main() {
    // Parse arguments
    if len(__args__) == 0 {
        println("Usage: dedup [--quiet] [--min-size N] [--workers N] <path> [<path> ...]")
        return
    }

    quiet = false
    min_size = 1
    num_workers = 16
    paths = makeArray(len(__args__), null)
    path_count = 0

    i = 0
    while i < len(__args__) {
        arg = __args__[i]
        if arg == "--quiet" {
            quiet = true
            i = i + 1
        } else if arg == "--min-size" {
            i = i + 1
            if i < len(__args__) {
                min_size = 0 + __args__[i]
            }
            i = i + 1
        } else if arg == "--workers" {
            i = i + 1
            if i < len(__args__) {
                num_workers = 0 + __args__[i]
            }
            i = i + 1
        } else {
            paths[path_count] = arg
            path_count = path_count + 1
            i = i + 1
        }
    }

    // ===================================================================
    // PHASE 1: WALK & COLLECT
    // ===================================================================

    MAX_FILES = 2000000
    all_files = makeArray(MAX_FILES, null)
    file_idx = atomicIntArray(1)

    file_ch = channel(2048)

    walker_task = async(fn() {
        i = 0
        while i < path_count {
            path = paths[i]
            if path != null {
                walk_files_streaming_recursive(path, file_ch)
            }
            i = i + 1
        }
        close(file_ch)
    })

    NUM_COLLECTORS = 8
    collector_tasks = makeArray(NUM_COLLECTORS, null)
    i = 0
    while i < NUM_COLLECTORS {
        let idx = file_idx
        let arr = all_files
        collector_tasks[i] = async(fn() {
            meta, ok = recv(file_ch)
            while ok {
                slot = atomicAdd(idx, 0, 1) - 1
                arr[slot] = meta
                meta, ok = recv(file_ch)
            }
        })
        i = i + 1
    }

    await(walker_task)
    i = 0
    while i < NUM_COLLECTORS {
        await(collector_tasks[i])
        i = i + 1
    }

    total_files = atomicLoad(file_idx, 0)

    if !quiet {
        println("Phase 1: Collected " + str(total_files) + " files")
    }

    // ===================================================================
    // PHASE 2: GROUP BY SIZE
    // ===================================================================

    size_groups = {}
    size_counts = {}

    i = 0
    while i < total_files {
        meta = all_files[i]
        if meta != null {
            path = meta[0]
            size = meta[1]
            if size >= min_size {
                if !hasKey(size_counts, size) {
                    size_counts[size] = 0
                    size_groups[size] = []
                }
                size_counts[size] = size_counts[size] + 1
                size_groups[size] = push(size_groups[size], path)
            }
        }
        i = i + 1
    }

    candidate_count = 0
    sz_keys = keys(size_groups)
    i = 0
    while i < len(sz_keys) {
        sz = sz_keys[i]
        if size_counts[sz] >= 2 {
            candidate_count = candidate_count + size_counts[sz]
        }
        i = i + 1
    }

    candidates = makeArray(candidate_count, null)
    cand_idx = 0
    i = 0
    while i < len(sz_keys) {
        sz = sz_keys[i]
        if size_counts[sz] >= 2 {
            paths_for_size = size_groups[sz]
            j = 0
            while j < len(paths_for_size) {
                entry = makeArray(2, null)
                entry[0] = paths_for_size[j]
                entry[1] = sz
                candidates[cand_idx] = entry
                cand_idx = cand_idx + 1
                j = j + 1
            }
        }
        i = i + 1
    }

    if !quiet {
        println("Phase 2: " + str(candidate_count) + " files qualify for hashing (>=2 with same size)")
    }

    if candidate_count == 0 {
        println("\nNo duplicates found.")
        return
    }

    // ===================================================================
    // PHASE 3: PARALLEL HASHING
    // ===================================================================

    work_ch = channel(2048)
    hash_results = makeArray(candidate_count, null)

    feed_task = async(fn() {
        i = 0
        while i < candidate_count {
            cand_with_idx = makeArray(3, null)
            cand_with_idx[0] = candidates[i][0]
            cand_with_idx[1] = candidates[i][1]
            cand_with_idx[2] = i
            send(work_ch, cand_with_idx)
            i = i + 1
        }
        close(work_ch)
    })

    hasher_tasks = makeArray(num_workers, null)
    i = 0
    while i < num_workers {
        let results = hash_results
        hasher_tasks[i] = async(fn() {
            item, ok = recv(work_ch)
            while ok {
                path = item[0]
                size = item[1]
                idx = item[2]
                h = hashFile(path, size)
                if h != null {
                    r = makeArray(3, null)
                    r[0] = path
                    r[1] = h
                    r[2] = size
                    results[idx] = r
                }
                item, ok = recv(work_ch)
            }
        })
        i = i + 1
    }

    await(feed_task)
    i = 0
    while i < num_workers {
        await(hasher_tasks[i])
        i = i + 1
    }

    if !quiet {
        println("Phase 3: Hashed " + str(candidate_count) + " candidates with " + str(num_workers) + " workers")
    }

    // ===================================================================
    // PHASE 4: GROUP BY HASH
    // ===================================================================

    hash_groups = {}
    hash_sizes = {}

    i = 0
    while i < candidate_count {
        r = hash_results[i]
        if r != null {
            path = r[0]
            h = r[1]
            size = r[2]
            if !hasKey(hash_groups, h) {
                hash_groups[h] = []
                hash_sizes[h] = size
            }
            hash_groups[h] = push(hash_groups[h], path)
        }
        i = i + 1
    }

    dup_hashes = []
    h_keys = keys(hash_groups)
    i = 0
    while i < len(h_keys) {
        h = h_keys[i]
        if len(hash_groups[h]) >= 2 {
            dup_hashes = push(dup_hashes, h)
        }
        i = i + 1
    }

    if !quiet {
        println("Phase 4: Found " + str(len(dup_hashes)) + " duplicate groups")
    }

    // ===================================================================
    // PHASE 5: REPORT
    // ===================================================================

    if len(dup_hashes) == 0 {
        println("\nNo duplicates found.")
        return
    }

    dup_groups_sorted = sortBy(dup_hashes, fn(ha, hb) {
        waste_a = hash_sizes[ha] * (len(hash_groups[ha]) - 1)
        waste_b = hash_sizes[hb] * (len(hash_groups[hb]) - 1)
        return waste_a > waste_b
    })

    total_recoverable = 0
    total_dup_files = 0

    if !quiet {
        println("\n" + repeat("─", 70))
        println("DUPLICATE GROUPS (sorted by wasted space)")
        println(repeat("─", 70))
    }

    i = 0
    while i < len(dup_groups_sorted) {
        h = dup_groups_sorted[i]
        paths = hash_groups[h]
        sz = hash_sizes[h]
        wasted = sz * (len(paths) - 1)
        total_recoverable = total_recoverable + wasted
        total_dup_files = total_dup_files + (len(paths) - 1)

        if !quiet {
            println("")
            println(format("[%d copies] %s — wasted %s",
                len(paths), humanSize(sz), humanSize(wasted)))
            println("  KEEP: " + paths[0])
            j = 1
            while j < len(paths) {
                println("  DUP:  " + paths[j])
                j = j + 1
            }
        }
        i = i + 1
    }

    println("\n" + repeat("═", 70))
    println(format("Files scanned:        %d", total_files))
    println(format("Candidates (≥2 size): %d", candidate_count))
    println(format("Duplicate files:      %d", total_dup_files))
    println(format("Duplicate groups:     %d", len(dup_groups_sorted)))
    println(format("Recoverable space:    %s", humanSize(total_recoverable)))
    println(repeat("═", 70))
}

main()
