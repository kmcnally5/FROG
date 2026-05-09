// leapfrog.lex - High-performance file classifier with parallel deep analysis
// Sub-parallelization for large files (>1MB): 32 concurrent chunk analysis workers
// File-level parallelization: 16 workers + 32 sub-workers per large file = 512 max goroutines
// Scans filesystem streaming (constant memory), classifies via: extension → magic bytes → content patterns
// Usage: timeout 30 ./klex projects/leapfrog/leapfrog.lex /path [/path2 ...]

fn getExtension(file_path) {
    // Scan backwards: stop at first '.' (extension) or '/' (no extension).
    // Typical paths visit 3-5 chars instead of walking the whole path forward.
    n = len(file_path)
    i = n - 1
    while i >= 0 {
        c = file_path[i]
        if c == "." {
            return lower(substr(file_path, i))
        }
        if c == "/" {
            return ""
        }
        i = i - 1
    }
    return ""
}

// Built ONCE at module load time. Workers see it via async snapshot.
// O(1) lookup replaces the up-to-60-comparison if-chain on every file.
EXT_LOOKUP = {
    ".pdf": "PDF",
    ".doc": "Word", ".docx": "Word",
    ".xls": "Excel", ".xlsx": "Excel",
    ".ppt": "PowerPoint", ".pptx": "PowerPoint",
    ".rtf": "RTF",
    ".txt": "Text",
    ".md": "Markdown", ".markdown": "Markdown",
    ".go": "Go",
    ".py": "Python",
    ".js": "JavaScript", ".ts": "JavaScript", ".tsx": "JavaScript",
    ".java": "Java",
    ".c": "C/C++", ".cpp": "C/C++", ".cc": "C/C++", ".h": "C/C++", ".hpp": "C/C++",
    ".rs": "Rust",
    ".rb": "Ruby",
    ".php": "PHP",
    ".sh": "Shell", ".bash": "Shell",
    ".html": "HTML", ".htm": "HTML",
    ".css": "CSS",
    ".scss": "SASS", ".sass": "SASS",
    ".less": "LESS",
    ".xml": "XML",
    ".jsx": "JSX",
    ".lex": "Frog",
    ".json": "JSON",
    ".csv": "CSV",
    ".tsv": "TSV",
    ".yaml": "YAML", ".yml": "YAML",
    ".toml": "TOML",
    ".ini": "INI/Config", ".cfg": "INI/Config", ".conf": "INI/Config",
    ".env": "Environment",
    ".sql": "SQL",
    ".sqlite": "SQLite", ".db": "SQLite", ".db3": "SQLite",
    ".zip": "ZIP",
    ".gz": "GZIP", ".gzip": "GZIP",
    ".tar": "TAR",
    ".bz2": "BZIP2",
    ".xz": "XZ",
    ".7z": "7z",
    ".rar": "RAR",
    ".jpg": "JPEG", ".jpeg": "JPEG",
    ".png": "PNG",
    ".gif": "GIF",
    ".bmp": "BMP",
    ".tiff": "TIFF", ".tif": "TIFF",
    ".webp": "WebP",
    ".svg": "SVG",
    ".ico": "ICO",
    ".mp3": "MP3",
    ".mp4": "MP4",
    ".wav": "WAV",
    ".avi": "AVI",
    ".mkv": "Matroska",
    ".mov": "MOV",
    ".flac": "FLAC",
    ".pem": "PEM/Key",
    ".der": "DER/Cert",
    ".pub": "SSH Key",
    ".gpg": "GPG", ".asc": "GPG",
    ".dockerfile": "Dockerfile", "dockerfile": "Dockerfile",
    "makefile": "Makefile",
    ".lock": "Lock File",
    ".log": "Log",
}

fn classifyByExtension(ext) {
    EXT_LOOKUP[ext]  // returns null if not present
}

fn classifyByMagicBytes(file_path) {
    chunk, _, err = _fsReadChunk(file_path, 0, 512)
    if err != null {
        return null
    }

    if len(chunk) == 0 {
        return null
    }

    if indexOf(chunk, "%PDF") >= 0 { return "PDF" }
    if indexOf(chunk, "PK\x03\x04") >= 0 { return "ZIP" }
    if indexOf(chunk, "\x1f\x8b") >= 0 { return "GZIP" }
    if indexOf(chunk, "BZh") >= 0 { return "BZIP2" }
    if indexOf(chunk, "\xfd7zXZ\x00") >= 0 { return "XZ" }
    if indexOf(chunk, "7z\xbc\xaf\x27\x1c") >= 0 { return "7z" }
    if indexOf(chunk, "Rar!") >= 0 { return "RAR" }
    if indexOf(chunk, "SQLite format 3") >= 0 { return "SQLite" }

    if indexOf(chunk, "\xff\xd8\xff") >= 0 { return "JPEG" }
    if indexOf(chunk, "\x89PNG\r\n\x1a\n") >= 0 { return "PNG" }
    if indexOf(chunk, "GIF8") >= 0 { return "GIF" }
    if indexOf(chunk, "BM") >= 0 { return "BMP" }
    if indexOf(chunk, "II\x2a\x00") >= 0 || indexOf(chunk, "MM\x00\x2a") >= 0 { return "TIFF" }
    if indexOf(chunk, "RIFF") >= 0 && indexOf(chunk, "WEBP") >= 0 { return "WebP" }

    if indexOf(chunk, "ID3") >= 0 { return "MP3" }
    if indexOf(chunk, "\xff\xfb") >= 0 { return "MP3" }
    if indexOf(chunk, "ftyp") >= 0 { return "MP4" }
    if indexOf(chunk, "RIFF") >= 0 && indexOf(chunk, "WAVE") >= 0 { return "WAV" }
    if indexOf(chunk, "RIFF") >= 0 && indexOf(chunk, "AVI") >= 0 { return "AVI" }
    if indexOf(chunk, "\x1a\x45\xdf\xa3") >= 0 { return "Matroska" }

    if indexOf(chunk, "\x7fELF") >= 0 { return "ELF" }
    if len(chunk) >= 4 {
        first4 = substr(chunk, 0, 4)
        if first4 == "\xcf\xfa\xed\xfe" || first4 == "\xce\xfa\xed\xfe" || first4 == "\xfe\xed\xfa\xcf" || first4 == "\xfe\xed\xfa\xce" {
            return "Mach-O"
        }
    }
    if indexOf(chunk, "MZ") >= 0 { return "PE" }
    if indexOf(chunk, "\xca\xfe\xba\xbe") >= 0 { return "Java Class" }

    if indexOf(chunk, "-----BEGIN") >= 0 { return "PEM/Key" }
    if indexOf(chunk, "ssh-rsa") >= 0 || indexOf(chunk, "ssh-ed25519") >= 0 { return "SSH Key" }

    return null
}

fn classifyByDeepAnalysis(file_path, chunk_size, num_sub_workers) {
    info, stat_err = _fsStat(file_path)
    if stat_err != null {
        return null
    }

    file_size = info["size"]
    if file_size == 0 {
        return null
    }

    num_chunks = (file_size + chunk_size - 1) / chunk_size

    // Lock-free atomic counters - sub-workers update in parallel, no merge step.
    //   slot 0: total lines
    //   slot 1: js   2: go   3: py   4: java
    //   slot 5: xml  6: html 7: sql  8: json
    counters = atomicIntArray(9)

    batch_start = 0
    while batch_start < num_chunks {
        batch_end = batch_start + num_sub_workers
        if batch_end > num_chunks {
            batch_end = num_chunks
        }
        batch_size = batch_end - batch_start

        tasks = makeArray(batch_size, null)

        b = 0
        while b < batch_size {
            chunk_idx = batch_start + b
            chunk_offset = chunk_idx * chunk_size
            is_last = (chunk_idx == num_chunks - 1)

            let cnt = counters
            let task = async(fn() {
                chunk, is_eof, err = _fsReadChunk(file_path, chunk_offset, chunk_size)
                if err != null { return null }
                if chunk == "" { return null }

                lines = split(chunk, "\n")
                process_count = len(lines)
                if is_last == false && len(lines) > 0 {
                    process_count = len(lines) - 1
                }

                // Local accumulators - one batched atomicAdd per slot at the end
                // (instead of one per matching line). Reduces atomic contention
                // dramatically while keeping the merge step trivially correct.
                local_js = 0
                local_go = 0
                local_py = 0
                local_java = 0
                local_xml = 0
                local_html = 0
                local_sql = 0
                local_json = 0

                i = 0
                while i < process_count {
                    line = lines[i]
                    if len(line) > 0 {
                        if indexOf(line, "import ") >= 0 && indexOf(line, " from ") >= 0 {
                            local_js = local_js + 1
                        }
                        if indexOf(line, "package ") >= 0 && indexOf(line, "import") >= 0 {
                            local_go = local_go + 1
                        }
                        if indexOf(line, "def ") >= 0 && indexOf(line, "import ") >= 0 {
                            local_py = local_py + 1
                        }
                        if indexOf(line, "public class") >= 0 || indexOf(line, "package ") >= 0 {
                            local_java = local_java + 1
                        }
                        if indexOf(line, "<?xml") >= 0 {
                            local_xml = local_xml + 1
                        }
                        if indexOf(line, "<html") >= 0 || indexOf(line, "<!DOCTYPE") >= 0 {
                            local_html = local_html + 1
                        }
                        if indexOf(line, "CREATE TABLE") >= 0 || indexOf(line, "INSERT INTO") >= 0 {
                            local_sql = local_sql + 1
                        }
                        if indexOf(line, "\{") >= 0 && indexOf(line, "}") >= 0 {
                            local_json = local_json + 1
                        }
                    }
                    i = i + 1
                }

                // Single batched atomic add per slot - lock-free merge with no contention bottleneck
                atomicAdd(cnt, 0, process_count)
                if local_js   > 0 { atomicAdd(cnt, 1, local_js) }
                if local_go   > 0 { atomicAdd(cnt, 2, local_go) }
                if local_py   > 0 { atomicAdd(cnt, 3, local_py) }
                if local_java > 0 { atomicAdd(cnt, 4, local_java) }
                if local_xml  > 0 { atomicAdd(cnt, 5, local_xml) }
                if local_html > 0 { atomicAdd(cnt, 6, local_html) }
                if local_sql  > 0 { atomicAdd(cnt, 7, local_sql) }
                if local_json > 0 { atomicAdd(cnt, 8, local_json) }

                return null
            })
            tasks[b] = task
            b = b + 1
        }

        // Wait for batch to finish (no merge work - atomics already aggregated)
        b = 0
        while b < batch_size {
            if tasks[b] != null { await(tasks[b]) }
            b = b + 1
        }

        batch_start = batch_end
    }

    if atomicLoad(counters, 1) > 0 { return "JavaScript" }
    if atomicLoad(counters, 2) > 0 { return "Go" }
    if atomicLoad(counters, 3) > 0 { return "Python" }
    if atomicLoad(counters, 4) > 0 { return "Java" }
    if atomicLoad(counters, 5) > 0 { return "XML" }
    if atomicLoad(counters, 6) > 0 { return "HTML" }
    if atomicLoad(counters, 7) > 0 { return "SQL" }
    if atomicLoad(counters, 8) > 0 { return "JSON" }

    return null
}

fn classifyFile(file_path, size) {
    // Stage 1: Extension (fastest)
    ext = getExtension(file_path)
    type_by_ext = classifyByExtension(ext)
    if type_by_ext != null {
        return {"type": type_by_ext, "confidence": "high"}
    }

    // Stage 2: Magic bytes (512 byte header read)
    type_by_magic = classifyByMagicBytes(file_path)
    if type_by_magic != null {
        return {"type": type_by_magic, "confidence": "high"}
    }

    // Stage 3: Deep analysis for large files (parallelized with 32MB chunks, 32 sub-workers)
    large_file_threshold = 1048576
    if size > large_file_threshold {
        chunk_size = 33554432
        type_by_content = classifyByDeepAnalysis(file_path, chunk_size, 32)
        if type_by_content != null {
            return {"type": type_by_content, "confidence": "medium"}
        }
    }

    if size < 1024 {
        return {"type": "Text", "confidence": "low"}
    }
    return {"type": "Unknown", "confidence": "low"}
}

fn getTypeColor(type_name) {
    if indexOf(type_name, "Text") >= 0 || indexOf(type_name, "Markdown") >= 0 { return color_green() }
    if indexOf(type_name, "Python") >= 0 || indexOf(type_name, "Go") >= 0 || indexOf(type_name, "Java") >= 0 { return color_green() }
    if indexOf(type_name, "JSON") >= 0 || indexOf(type_name, "CSV") >= 0 || indexOf(type_name, "YAML") >= 0 || indexOf(type_name, "XML") >= 0 { return color_yellow() }
    if indexOf(type_name, "MP3") >= 0 || indexOf(type_name, "MP4") >= 0 || indexOf(type_name, "WAV") >= 0 || indexOf(type_name, "JPEG") >= 0 || indexOf(type_name, "PNG") >= 0 { return color_magenta() }
    if indexOf(type_name, "ZIP") >= 0 || indexOf(type_name, "GZIP") >= 0 || indexOf(type_name, "TAR") >= 0 || indexOf(type_name, "7z") >= 0 { return color_blue() }
    if indexOf(type_name, "ELF") >= 0 || indexOf(type_name, "PE") >= 0 || indexOf(type_name, "Mach-O") >= 0 { return color_red() }
    return color_white()
}

fn getTypeEmoji(type_name) {
    if indexOf(type_name, "Python") >= 0 { return "🐍" }
    if indexOf(type_name, "Go") >= 0 { return "🐹" }
    if indexOf(type_name, "Java") >= 0 { return "☕" }
    if indexOf(type_name, "JavaScript") >= 0 || indexOf(type_name, "JSX") >= 0 { return "📜" }
    if indexOf(type_name, "C/C++") >= 0 { return "⚙️" }
    if indexOf(type_name, "Rust") >= 0 { return "🦀" }
    if indexOf(type_name, "Ruby") >= 0 { return "💎" }
    if indexOf(type_name, "PHP") >= 0 { return "🐘" }
    if indexOf(type_name, "Shell") >= 0 { return "🐚" }
    if indexOf(type_name, "Frog") >= 0 { return "🐸" }

    if indexOf(type_name, "JSON") >= 0 { return "\{}" }
    if indexOf(type_name, "CSV") >= 0 { return "📊" }
    if indexOf(type_name, "YAML") >= 0 { return "📋" }
    if indexOf(type_name, "XML") >= 0 { return "📝" }
    if indexOf(type_name, "HTML") >= 0 { return "🌐" }
    if indexOf(type_name, "CSS") >= 0 { return "🎨" }
    if indexOf(type_name, "SQL") >= 0 { return "🗄️" }
    if indexOf(type_name, "Markdown") >= 0 { return "📄" }
    if indexOf(type_name, "Text") >= 0 { return "📃" }

    if indexOf(type_name, "JPEG") >= 0 || indexOf(type_name, "PNG") >= 0 || indexOf(type_name, "GIF") >= 0 { return "🖼️" }
    if indexOf(type_name, "MP3") >= 0 || indexOf(type_name, "FLAC") >= 0 { return "🎵" }
    if indexOf(type_name, "MP4") >= 0 || indexOf(type_name, "AVI") >= 0 { return "🎬" }
    if indexOf(type_name, "WAV") >= 0 { return "🔊" }

    if indexOf(type_name, "ZIP") >= 0 || indexOf(type_name, "GZIP") >= 0 || indexOf(type_name, "TAR") >= 0 || indexOf(type_name, "7z") >= 0 { return "📦" }
    if indexOf(type_name, "PDF") >= 0 { return "📕" }
    if indexOf(type_name, "Word") >= 0 || indexOf(type_name, "Document") >= 0 { return "📘" }
    if indexOf(type_name, "Excel") >= 0 { return "📗" }

    if indexOf(type_name, "ELF") >= 0 || indexOf(type_name, "PE") >= 0 || indexOf(type_name, "Mach-O") >= 0 { return "⚡" }

    return "❓"
}

fn formatFileSize(size) {
    if size < 1024 { return str(size) + "B" }
    if size < 1048576 { return str(size / 1024) + "KB" }
    if size < 1073741824 { return str(size / 1048576) + "MB" }
    return str(size / 1073741824) + "GB"
}

fn shortenPath(file_path, max_len) {
    if len(file_path) <= max_len {
        return file_path
    }

    // Keep the last parts of the path and truncate the beginning
    truncated = substr(file_path, len(file_path) - max_len + 3)
    return "..." + truncated
}

fn formatRealtime(file_path, size, result) {
    short_path = shortenPath(file_path, 70)
    colored_path = color_cyan() + short_path + color_reset()
    type_color = getTypeColor(result["type"])
    colored_type = type_color + color_bold() + result["type"] + color_reset()
    colored_size = color_dim() + formatFileSize(size) + color_reset()
    return colored_path + " → " + colored_type + " " + colored_size
}

fn shouldSkipDir(path) {
    if indexOf(path, "/node_modules") >= 0 { return true }
    if indexOf(path, "/.git") >= 0 { return true }
    if indexOf(path, "/vendor") >= 0 { return true }
    if indexOf(path, "/.Trash") >= 0 { return true }
    if indexOf(path, "/Library/Caches") >= 0 { return true }
    if indexOf(path, "/.cache") >= 0 { return true }
    if indexOf(path, "/System") >= 0 { return true }
    if indexOf(path, "/private/var") >= 0 { return true }
    if indexOf(path, "/Volumes") >= 0 { return true }
    return false
}

fn walk_files_streaming_recursive(path, ch) {
    info, err = _fsStat(path)
    if err != null {
        return
    }

    if info["isDir"] == false {
        // Send 2-element array - cheaper allocation than a 2-key hash.
        // Worker reads file_meta[0]=path, file_meta[1]=size.
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
        name = entry["name"]
        full_path = path + "/" + name

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

fn show_help() {
    println(color_bold() + color_cyan() + "leapfrog - High-Performance File Classifier" + color_reset())
    println("")
    println(color_bold() + "USAGE:" + color_reset())
    println("  ./klex projects/leapfrog/leapfrog.lex [OPTIONS] /path [/path2 ...]")
    println("")
    println(color_bold() + "OPTIONS:" + color_reset())
    println("  " + color_green() + "-h, --help" + color_reset() + "         Show this help message")
    println("  " + color_green() + "--quiet" + color_reset() + "           Disable realtime filepath streaming (show summary only)")
    println("  " + color_green() + "--realtime" + color_reset() + "        Enable realtime streaming (default)")
    println("")
    println(color_bold() + "EXAMPLES:" + color_reset())
    println("  # Scan with realtime output and summary:")
    println("  ./klex projects/leapfrog/leapfrog.lex /Users/karlmcnally/Documents")
    println("")
    println("  # Scan with summary only (quiet mode):")
    println("  ./klex projects/leapfrog/leapfrog.lex --quiet /Users/karlmcnally/Documents")
    println("")
    println("  # Scan multiple paths:")
    println("  ./klex projects/leapfrog/leapfrog.lex --quiet /path1 /path2 /path3")
    println("")
}

fn main() {
    show_realtime = true
    paths = makeArray(len(__args__), null)
    path_count = 0

    i = 0
    while i < len(__args__) {
        arg = __args__[i]
        if arg == "-h" || arg == "--help" {
            show_help()
            return
        } else if arg == "--quiet" {
            show_realtime = false
            i = i + 1
        } else if arg == "--realtime" {
            show_realtime = true
            i = i + 1
        } else {
            paths[path_count] = arg
            path_count = path_count + 1
            i = i + 1
        }
    }

    if path_count == 0 {
        println(color_red() + "✗ Error: " + color_reset() + "No paths specified")
        println("")
        println("Use " + color_yellow() + "--help" + color_reset() + " for usage information")
        return
    }

    file_ch = channel(2048)

    // Lock-free atomic counters for live, contention-free aggregation across
    // all workers. Used as the single shared source of truth for file_count
    // and total_size — no channel sends per-file, no serial summary funnel.
    //   slot 0: file_count
    //   slot 1: total_size (in bytes)
    progress = atomicIntArray(2)

    walker_task = async(fn() {
        i = 0
        while i < path_count {
            if paths[i] != null && paths[i] != "" {
                walk_files_streaming_recursive(paths[i], file_ch)
            }
            i = i + 1
        }
        close(file_ch)
    })

    num_workers = 16
    worker_tasks = makeArray(num_workers, null)

    // Single shared ConcurrentHash for type counts across all workers.
    // atomicHashIncr is lock-free under contention - no per-worker hash to
    // merge afterwards, just read the final state directly.
    type_counts = concurrentHash()

    i = 0
    while i < num_workers {
        let prog = progress
        let counts = type_counts
        let task = async(fn() {
            file_meta, ok = recv(file_ch)
            while ok {
                file_path = file_meta[0]
                file_size = file_meta[1]

                result = classifyFile(file_path, file_size)
                file_type = result["type"]

                if show_realtime {
                    println(formatRealtime(file_path, file_size, result))
                }

                // Lock-free atomic updates to shared state - no per-worker
                // local hash, no merge step at the end
                atomicHashIncr(counts, file_type, 1)
                atomicAdd(prog, 0, 1)
                atomicAdd(prog, 1, file_size)

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

    file_count = atomicLoad(progress, 0)
    total_size = atomicLoad(progress, 1)

    // Snapshot the type names for ordered display
    type_order = keys(type_counts)
    type_count = len(type_order)

    println("")
    println(color_bold() + color_cyan() + "╔════════════════════════════════════════╗" + color_reset())
    println(color_bold() + color_cyan() + "║  📊 Classification Summary Report 📊   ║" + color_reset())
    println(color_bold() + color_cyan() + "╚════════════════════════════════════════╝" + color_reset())
    println("")

    println(color_bold() + "📈 Statistics:" + color_reset())
    println("  " + color_green() + "✓" + color_reset() + " Total files:  " + color_yellow() + color_bold() + str(file_count) + color_reset())
    println("  " + color_cyan() + "📦" + color_reset() + " Total size:   " + color_yellow() + color_bold() + formatFileSize(total_size) + color_reset())
    println("")

    if file_count > 0 {
        println(color_bold() + "🎯 Breakdown by Type:" + color_reset())
        println("")

        i = 0
        while i < type_count {
            type_name = type_order[i]
            count = type_counts[type_name]
            emoji = getTypeEmoji(type_name)
            tcolor = getTypeColor(type_name)
            percent = (count * 100) / file_count
            bar_len = percent / 5
            bar = ""
            b = 0
            while b < bar_len {
                bar = bar + "█"
                b = b + 1
            }
            colored_type = tcolor + color_bold() + type_name + color_reset()
            println("  " + emoji + "  " + colored_type + " " + color_dim() + "(" + str(count) + " files, " + str(percent) + "%)" + color_reset())
            println("     " + color_blue() + bar + color_reset())
            i = i + 1
        }
    }

    println("")
    println(color_bold() + color_green() + "✓ Analysis complete!" + color_reset())
}

main()
