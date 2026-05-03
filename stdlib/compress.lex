// ============================================================================
// compress.lex — Data compression utilities
// ============================================================================
//
// Provides gzip and deflate compression/decompression for strings and binary data.
// Primary use: reducing size of large payloads (files, API responses, logs).
//
// Compression ratio depends on data type:
//   - Text (JSON, CSV, logs): 5–10x reduction
//   - Images (PNG, JPEG): 1.01–1.1x (already compressed)
//   - Binary: 2–4x reduction (varies by entropy)
//
// Example:
//   data = readFile("huge.json")
//   compressed, err = compress(data)
//   if err != null { return null, err }
//   writeFile("huge.json.gz", compressed)

// ============================================================================
// GZIP COMPRESSION (default, most compatible)
// ============================================================================

// compress(data) → (compressed, error)
// Compresses a string using gzip. This is the default compression format.
// Gzip is widely supported and provides good compression ratio.
// Use this when writing to files or sending over HTTP.
//
// Example:
//   result, err = compress("hello world")  // gzip format
//   println(len(result))  // much smaller than original
fn compress(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "compress expects string, got " + type(data))
    }
    return safe(_gzipCompress, data)
}

// decompress(data) → (decompressed, error)
// Decompresses gzip data. Assumes data was compressed with compress().
// Returns the original string.
//
// Example:
//   original, err = decompress(compressed_data)
//   if err != null { println("decompression failed:", err) }
fn decompress(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "decompress expects string, got " + type(data))
    }
    return safe(_gzipDecompress, data)
}

// ============================================================================
// DEFLATE COMPRESSION (alternative, lighter header)
// ============================================================================

// deflate(data) → (compressed, error)
// Compresses a string using deflate (no gzip header).
// Deflate is lighter than gzip but less compatible.
// Use this for internal data interchange when you control both sides.
//
// Example:
//   result, err = deflate("hello world")
//   // result is slightly smaller than gzip but less widely supported
fn deflate(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "deflate expects string, got " + type(data))
    }
    return safe(_deflateCompress, data)
}

// inflate(data) → (decompressed, error)
// Decompresses deflate data. Assumes data was compressed with deflate().
// Returns the original string.
//
// Example:
//   original, err = inflate(deflated_data)
fn inflate(data) {
    if type(data) != "STRING" {
        return null, error("TYPE_ERROR", "inflate expects string, got " + type(data))
    }
    return safe(_deflateDecompress, data)
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// compressionRatio(original, compressed) → float
// Returns the ratio of original size to compressed size.
// Useful for understanding compression effectiveness.
//
// Returns:
//   > 1.0  — data was compressed (smaller after compression)
//   = 1.0  — no compression (same size)
//   < 1.0  — data expanded (compression made it larger, shouldn't happen)
//
// Example:
//   ratio = compressionRatio(data, compressed)
//   println("Compression ratio: " + str(ratio) + "x")
fn compressionRatio(original, compressed) {
    origLen = len(original)
    compLen = len(compressed)

    if compLen == 0 {
        return 1.0
    }

    return float(origLen) / float(compLen)
}

// compressFile(path) → (compressed_data, error)
// Reads a file and returns its gzip-compressed contents.
// Useful for archival or transfer.
//
// Example:
//   compressed, err = compressFile("large.json")
//   if err == null {
//       writeFile("large.json.gz", compressed)
//   }
fn compressFile(path) {
    import "fs.lex" as fs

    data, err = fs.read(path)
    if err != null { return null, err }

    return compress(data)
}

// decompressFile(path) → (decompressed_data, error)
// Reads a gzip-compressed file and returns its contents.
// Useful for reading archived files.
//
// Example:
//   data, err = decompressFile("large.json.gz")
fn decompressFile(path) {
    import "fs.lex" as fs

    data, err = fs.read(path)
    if err != null { return null, err }

    return decompress(data)
}

// ============================================================================
// COMPRESSION STATISTICS
// ============================================================================

// compressedSize(original) → int
// Returns the size of the data after gzip compression.
// Useful for estimating storage or network transfer costs.
//
// Example:
//   size = compressedSize(large_json)
//   println("Will transfer " + str(size) + " bytes")
fn compressedSize(data) {
    compressed, err = compress(data)
    if err != null { return 0 }
    return len(compressed)
}

// savings(original, compressed) → string
// Returns a human-readable string showing compression savings.
// Shows both absolute bytes saved and percentage.
//
// Example:
//   println(savings(data, compressed))
//   // Output: "Saved 4,500 bytes (94.5%)"
fn savings(original, compressed) {
    origLen = len(original)
    compLen = len(compressed)

    if compLen >= origLen {
        return "No savings (data expanded by " + str(compLen - origLen) + " bytes)"
    }

    bytesSaved = origLen - compLen
    percent = (float(bytesSaved) / float(origLen)) * 100.0

    return "Saved " + str(bytesSaved) + " bytes (" + str(percent) + "%)"
}

// ============================================================================
// BATCH COMPRESSION (for parallel processing)
// ============================================================================

// compressMany(items) → (compressed_items, error)
// Compresses an array of strings. Useful for parallel compression.
//
// Example:
//   items = ["data1", "data2", "data3", ...]
//   compressed, err = compressMany(items)
//   if err == null {
//       writeFile("archive.tar.gz", compress(join(compressed, "")))
//   }
fn compressMany(items) {
    if type(items) != "ARRAY" {
        return null, error("TYPE_ERROR", "compressMany expects array")
    }

    results = makeArray(len(items), null)
    i = 0
    while i < len(items) {
        compressed, err = compress(items[i])
        if err != null { return null, err }
        results[i] = compressed
        i = i + 1
    }

    return results, null
}

// decompressMany(compressed_items) → (items, error)
// Decompresses an array of gzip-compressed strings.
//
// Example:
//   items, err = decompressMany(compressed_array)
fn decompressMany(items) {
    if type(items) != "ARRAY" {
        return null, error("TYPE_ERROR", "decompressMany expects array")
    }

    results = makeArray(len(items), null)
    i = 0
    while i < len(items) {
        decompressed, err = decompress(items[i])
        if err != null { return null, err }
        results[i] = decompressed
        i = i + 1
    }

    return results, null
}
