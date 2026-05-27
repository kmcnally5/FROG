// frep.lex - high-performance pattern matching for large files and directories
// Handles 58GB+ files with chunked reading (1MB for small, 32MB for large)
// Streaming directory walk: constant memory regardless of file count
// File-level parallelization: 16 concurrent workers + 32 sub-workers per large file (>10MB)
// Channel buffer: 2048 entries for producer-consumer pipelining
// Binary detection: staged filtering (extension → header → magic bytes → null bytes)
// Usage: ./klex tests/examples/frep.lex "pattern" [file1 file2 dir1 dir2 ...]
//        cat file | ./klex tests/examples/frep.lex "pattern"

fn hasBinaryExtension(file_path) {
    if indexOf(file_path, ".raw") >= 0 || indexOf(file_path, ".RAW") >= 0 { return true }
    if indexOf(file_path, ".iso") >= 0 || indexOf(file_path, ".ISO") >= 0 { return true }
    if indexOf(file_path, ".img") >= 0 || indexOf(file_path, ".IMG") >= 0 { return true }
    if indexOf(file_path, ".dmg") >= 0 || indexOf(file_path, ".DMG") >= 0 { return true }
    if indexOf(file_path, ".xz") >= 0 || indexOf(file_path, ".XZ") >= 0 { return true }
    if indexOf(file_path, ".zip") >= 0 || indexOf(file_path, ".ZIP") >= 0 { return true }
    if indexOf(file_path, ".gz") >= 0 || indexOf(file_path, ".GZ") >= 0 { return true }
    if indexOf(file_path, ".tar") >= 0 || indexOf(file_path, ".TAR") >= 0 { return true }
    if indexOf(file_path, ".exe") >= 0 || indexOf(file_path, ".EXE") >= 0 { return true }
    if indexOf(file_path, ".dll") >= 0 || indexOf(file_path, ".DLL") >= 0 { return true }
    if indexOf(file_path, ".so") >= 0 || indexOf(file_path, ".SO") >= 0 { return true }
    if indexOf(file_path, ".bin") >= 0 || indexOf(file_path, ".BIN") >= 0 { return true }
    return false
}

fn isLikelyBinaryFile(file_path) {
    if hasBinaryExtension(file_path) {
        return true
    }

    let chunk, _, err = _fsReadChunk(file_path, 0, 512)
    if err != null {
        return false
    }

    if len(chunk) == 0 {
        return false
    }

    if indexOf(chunk, "\x7fELF") >= 0 {
        return true
    }

    if len(chunk) >= 4 {
        let first4 = substr(chunk, 0, 4)
        if first4 == "\xcf\xfa\xed\xfe" || first4 == "\xce\xfa\xed\xfe" || first4 == "\xfe\xed\xfa\xcf" || first4 == "\xfe\xed\xfa\xce" {
            return true
        }
    }

    if indexOf(chunk, "MZ") >= 0 {
        return true
    }

    if indexOf(chunk, "\xca\xfe\xba\xbe") >= 0 {
        return true
    }

    if indexOf(chunk, "\x00") >= 0 {
        return true
    }

    return false
}

fn highlightPattern(line, pattern) {
    return line
}

fn formatOutput(file_path, line_number, line, pattern) {
    let colored_path = color_cyan() + file_path + color_reset()
    let colored_line_num = color_green() + str(line_number) + color_reset()
    return colored_path + ":" + colored_line_num + ":" + line
}

fn search_large_file(file_path, pattern, chunk_size, num_sub_workers) {
    let info, stat_err = _fsStat(file_path)
    if stat_err != null {
        return
    }

    let file_size = info["size"]
    if file_size == 0 {
        return
    }

    let num_chunks = (file_size + chunk_size - 1) / chunk_size
    let total_matches = 0
    let total_lines = 0

    let batch_start = 0
    while batch_start < num_chunks {
        let batch_end = batch_start + num_sub_workers
        if batch_end > num_chunks {
            batch_end = num_chunks
        }
        let batch_size = batch_end - batch_start

        let tasks = makeArray(batch_size, null)

        let b = 0
        while b < batch_size {
            let chunk_idx = batch_start + b
            let chunk_offset = chunk_idx * chunk_size
            let is_last = (chunk_idx == num_chunks - 1)

            let task = async(fn() {
                let chunk, is_eof, err = _fsReadChunk(file_path, chunk_offset, chunk_size)

                if err != null {
                    return {"matches": 0, "line_count": 0}
                }

                if chunk == "" {
                    return {"matches": 0, "line_count": 0}
                }

                let lines = split(chunk, "\n")
                let matches = 0
                let line_count = 0
                let line_number = chunk_offset / chunk_size * (len(split(chunk, "\n")) - 1) + 1

                let process_count = len(lines)
                if is_last == false && len(lines) > 0 {
                    process_count = len(lines) - 1
                }

                let i = 0
                while i < process_count {
                    let line = lines[i]
                    if len(line) > 0 {
                        line_count = line_count + 1
                        // Fast byte-match: indexOf is significantly faster than _regexFindAll
                        if indexOf(line, pattern) >= 0 {
                            println(formatOutput(file_path, line_number, line, pattern))
                            matches = matches + 1
                        }
                    }
                    line_number = line_number + 1
                    i = i + 1
                }

                return {"matches": matches, "line_count": line_count}
            })
            tasks[b] = task
            b = b + 1
        }

        b = 0
        while b < batch_size {
            if tasks[b] != null {
                let result = await(tasks[b])
                total_matches = total_matches + result["matches"]
                total_lines = total_lines + result["line_count"]
            }
            b = b + 1
        }

        batch_start = batch_end
    }

    return {"matches": total_matches, "line_count": total_lines}
}

fn shouldSkipDir(path) {
    if indexOf(path, "/node_modules") >= 0 { return true }
    if indexOf(path, "/.git") >= 0 { return true }
    if indexOf(path, "/vendor") >= 0 { return true }
    if indexOf(path, "/build") >= 0 { return true }
    if indexOf(path, "/dist") >= 0 { return true }
    if indexOf(path, "klex_darwin") >= 0 { return true }
    if indexOf(path, "klex_linux") >= 0 { return true }
    if indexOf(path, "klex_win") >= 0 { return true }
    if indexOf(path, "/linux") >= 0 { return true }
    if indexOf(path, "/windows") >= 0 { return true }
    if indexOf(path, "/mac") >= 0 { return true }
    return false
}

fn walk_files_streaming_recursive(path, ch) {
    let info, err = _fsStat(path)
    if err != null {
        return
    }

    if info["isDir"] == false {
        send(ch, {"path": path, "size": info["size"], "isDir": false})
        return
    }

    if shouldSkipDir(path) {
        return
    }

    let entries, list_err = _fsReadDir(path)
    if list_err != null {
        return
    }

    let i = 0
    while i < len(entries) {
        let entry = entries[i]
        let name = entry["name"]
        let full_path = path + "/" + name

        if entry["isDir"] {
            walk_files_streaming_recursive(full_path, ch)
        } else {
            send(ch, {"path": full_path, "size": entry["size"], "isDir": false})
        }
        i = i + 1
    }
}

fn main() {
    if len(__args__) < 1 {
        println("Usage: frep PATTERN [FILE...]")
        return
    }

    let pattern = __args__[0]
    let raw_paths = slice(__args__, 1)

    if len(raw_paths) == 0 {
        let line_num = 1
        while true {
            let line = input("")
            if line == "" {
                break
            }
            let matches, err = _regexFindAll(pattern, line)
            if matches != null && len(matches) > 0 {
                println(formatOutput("<stdin>", line_num, line, pattern))
            }
            line_num = line_num + 1
        }
        return
    }

    let file_ch = channel(2048)
    let num_workers = 16

    let walker_task = async(fn() {
        let i = 0
        while i < len(raw_paths) {
            walk_files_streaming_recursive(raw_paths[i], file_ch)
            i = i + 1
        }
        close(file_ch)
    })

    let worker_tasks = makeArray(num_workers, null)
    let i = 0
    while i < num_workers {
        let task = async(fn() {
            let file_meta, ok = recv(file_ch)
            while ok {
                let file_path = file_meta["path"]
                let file_size = file_meta["size"]
                let large_file_threshold = 10485760

                if !isLikelyBinaryFile(file_path) {
                    if file_size > large_file_threshold {
                        let result = search_large_file(file_path, pattern, 33554432, 32)
                    } else {
                        // Note: small files use _regexFindAll (regex overhead), while large files use indexOf (fast byte-match)
                        // Optimization attempted: wrapper function to detect literal patterns and use indexOf was slower
                        // Reason: calling pattern detection function per-line is more expensive than _regexFindAll overhead
                        // To optimize further would require checking pattern type once at startup, not per-line
                        let chunk_size = 1048576
                        let offset = 0
                        let leftover = ""
                        let line_number = 1
                        let total_matches = 0
                        let total_lines = 0

                        while true {
                            let chunk, is_eof, err = _fsReadChunk(file_path, offset, chunk_size)
                            if err != null {
                                break
                            }

                            if chunk == "" {
                                break
                            }

                            let combined = leftover + chunk
                            let lines = split(combined, "\n")
                            leftover = lines[len(lines) - 1]

                            let k = 0
                            while k < len(lines) - 1 {
                                if lines[k] != "" {
                                    let matches, match_err = _regexFindAll(pattern, lines[k])
                                    if matches != null && len(matches) > 0 {
                                        println(formatOutput(file_path, line_number, lines[k], pattern))
                                        total_matches = total_matches + 1
                                    }
                                }
                                line_number = line_number + 1
                                total_lines = total_lines + 1
                                k = k + 1
                            }

                            offset = offset + len(chunk)
                        }

                        if leftover != "" {
                            let matches, match_err = _regexFindAll(pattern, leftover)
                            if matches != null && len(matches) > 0 {
                                println(formatOutput(file_path, line_number, leftover, pattern))
                                total_matches = total_matches + 1
                            }
                        }

                        let result = {"matches": total_matches, "line_count": total_lines}
                    }
                }

                file_meta, ok = recv(file_ch)
            }
        })
        worker_tasks[i] = task
        i = i + 1
    }

    await(walker_task)

    i = 0
    while i < num_workers {
        await(worker_tasks[i])
        i = i + 1
    }
}

main()
main()
main()
main()
