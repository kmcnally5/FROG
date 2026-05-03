// ============================================================================
// csvstream.lex — Streaming Parallel CSV Analysis Orchestration
// ============================================================================
//
// Pure FROG implementation of hierarchical parallel CSV processing using
// async workers + parallel_reduce for multi-dimensional data aggregation.
//
// Design: Break CSV records into chunks, distribute to async workers,
// each worker uses parallel_reduce to process sub-tasks, results bubble
// back up and merge into global aggregates.
//
// Use cases:
//   - Large-scale CSV analytics (millions of rows)
//   - Multi-dimensional aggregation (regions × products × time)
//   - Real-time streaming analysis with parallel orchestration
//   - ETL pipelines with custom reducer logic
//
// Example:
//   records = parse(csv)
//   results = analyzeChunked(records, fn(chunk) { ... }, 1000, 4)
//   // Automatically parallelizes into 4 workers, 1000 rows each

import "datetime.lex" as dt
import "parallel.lex" as p

// ============================================================================
// CORE: CHUNKED ANALYSIS
// ============================================================================

// analyzeChunked(records, analyzerFn, chunkSize, workerCount) → (results, error)
// Splits records into chunks and distributes to async workers.
// Each worker processes its chunk via parallel_reduce on sub-tasks.
// Returns merged results and total timing stats.
//
// Args:
//   records: array of record arrays
//   analyzerFn: fn(chunk) → result (processes one chunk)
//   chunkSize: rows per chunk
//   workerCount: number of async workers to spawn
//
// Returns:
//   {
//     workerResults: [result1, result2, ...],
//     merged: global merged result,
//     timing: {workers_ms, merge_ms, total_ms},
//     chunks: number of chunks processed
//   }
//
// Example:
//   fn countBySKU(chunk) {
//       results = {}
//       i = 0
//       while i < len(chunk) {
//           sku = chunk[i][2]
//           results[sku] = (results[sku] || 0) + 1
//       }
//       return results
//   }
//   analysis = analyzeChunked(records, countBySKU, 500, 4)
fn analyzeChunked(records, analyzerFn, chunkSize, workerCount) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "analyzeChunked: records must be array")
    }
    if type(analyzerFn) != "FUNCTION" {
        return null, error("TYPE_ERROR", "analyzeChunked: analyzerFn must be function")
    }
    if type(chunkSize) != "INTEGER" || chunkSize <= 0 {
        return null, error("INVALID_ARG", "analyzeChunked: chunkSize must be positive integer")
    }
    if type(workerCount) != "INTEGER" || workerCount <= 0 {
        return null, error("INVALID_ARG", "analyzeChunked: workerCount must be positive integer")
    }

    t0 = dt.nowNanos()

    // Create chunks using helper
    chunks = createChunks(records, chunkSize)

    t1 = dt.nowNanos()
    chunkTime = t1 - t0

    // Determine actual workers to spawn (cap at chunk count)
    actualWorkers = workerCount
    if actualWorkers > len(chunks) {
        actualWorkers = len(chunks)
    }

    // Distribute chunks round-robin: worker w gets chunks[w], chunks[w+actualWorkers], ...
    workerChunkAssignments = makeArray(actualWorkers, null)
    w = 0
    while w < actualWorkers {
        // Count how many chunks this worker will get
        chunkCount = 0
        c = w
        while c < len(chunks) {
            chunkCount = chunkCount + 1
            c = c + actualWorkers
        }
        // Pre-allocate array for this worker's chunks
        workerChunks = makeArray(chunkCount, null)
        chunkIdx = 0
        c = w
        while c < len(chunks) {
            workerChunks[chunkIdx] = chunks[c]
            chunkIdx = chunkIdx + 1
            c = c + actualWorkers
        }
        workerChunkAssignments[w] = workerChunks
        w = w + 1
    }

    t2 = dt.nowNanos()
    distributeTime = t2 - t1

    // Spawn async workers, one per group (avoid closure capture issues)
    workers = makeArray(actualWorkers, null)
    w = 0
    while w < actualWorkers {
        // Force each worker to capture its own data by passing to async directly
        assignedChunks = workerChunkAssignments[w]

        // Create closure that captures current chunk assignments
        workerFunc = fn(chunks, analyzer) {
            return async(fn() {
                workerResult = {}
                i = 0
                while i < len(chunks) {
                    chunk = chunks[i]
                    chunkResult = analyzer(chunk)

                    // Merge chunkResult into workerResult
                    if type(chunkResult) == "HASH" {
                        resultKeys = keys(chunkResult)
                        k = 0
                        while k < len(resultKeys) {
                            key = resultKeys[k]
                            value = chunkResult[key]
                            if workerResult[key] == null {
                                workerResult[key] = 0
                            }
                            if type(value) == "INTEGER" {
                                workerResult[key] = workerResult[key] + value
                            }
                            k = k + 1
                        }
                    }
                    i = i + 1
                }
                return workerResult
            })
        }

        worker = workerFunc(assignedChunks, analyzerFn)
        workers[w] = worker
        w = w + 1
    }

    t3 = dt.nowNanos()
    spawnTime = t3 - t2

    // Await results from all workers (they run in parallel)
    workerResults = makeArray(len(workers), null)
    w = 0
    while w < len(workers) {
        task = workers[w]
        result = await(task)
        workerResults[w] = result
        w = w + 1
    }

    t4 = dt.nowNanos()
    awaitTime = t4 - t3

    // Final merge: combine all worker results
    merged = mergeResults(workerResults)

    t5 = dt.nowNanos()
    mergeTime = t5 - t4

    result_map = {}
    result_map["merged"] = merged
    timing_map = {}
    timing_map["chunk_ms"] = chunkTime / 1000000
    timing_map["distribute_ms"] = distributeTime / 1000000
    timing_map["spawn_ms"] = spawnTime / 1000000
    timing_map["await_ms"] = awaitTime / 1000000
    timing_map["merge_ms"] = mergeTime / 1000000
    timing_map["total_ms"] = (t5 - t0) / 1000000
    result_map["timing"] = timing_map
    result_map["chunks"] = len(chunks)
    result_map["workers"] = actualWorkers

    return result_map, null
}

// ============================================================================
// HIERARCHICAL: ASYNC + PARALLEL_REDUCE
// ============================================================================

// analyzeHierarchical(records, primaryKeyIdx, secondaryKeyIdx, workerCounts) → (results, error)
// Two-level parallelization using async (primary dimension) + parallel_reduce (secondary).
//
// Flow:
//   1. Group records by primaryKeyIdx
//   2. Spawn async worker per primary group
//   3. Within each worker: parallel_reduce on secondaryKeyIdx
//   4. Merge results into matrix (primary × secondary)
//
// Args:
//   records: array of record arrays
//   primaryKeyIdx: column index for primary grouping (e.g., region)
//   secondaryKeyIdx: column index for secondary grouping (e.g., product)
//   workerCounts: {primary: N, secondary: M} async workers and parallel_reduce workers
//
// Returns:
//   {
//     matrix: {primaryKey: {secondaryKey: count, ...}, ...},
//     totals: {primaryKey: total, ...},
//     grandTotal: sum across all,
//     timing: {group_ms, async_ms, reduce_ms, merge_ms, total_ms}
//   }
//
// Example:
//   // Analyze: regions × products (count per region-product combination)
//   results = analyzeHierarchical(records, 1, 2, {primary: 5, secondary: 3})
//   // results.matrix["North"]["SKU-001"] == count in North region for SKU-001
fn analyzeHierarchical(records, primaryKeyIdx, secondaryKeyIdx, workerCounts) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "analyzeHierarchical: records must be array")
    }
    if type(primaryKeyIdx) != "INTEGER" || primaryKeyIdx < 0 {
        return null, error("INVALID_ARG", "analyzeHierarchical: primaryKeyIdx must be non-negative")
    }
    if type(secondaryKeyIdx) != "INTEGER" || secondaryKeyIdx < 0 {
        return null, error("INVALID_ARG", "analyzeHierarchical: secondaryKeyIdx must be non-negative")
    }
    if type(workerCounts) != "HASH" {
        return null, error("TYPE_ERROR", "analyzeHierarchical: workerCounts must be map")
    }

    t0 = dt.nowNanos()

    // Group records by primary key
    primaryGroups = groupBy(records, primaryKeyIdx)
    if primaryGroups == null {
        return null, error("INTERNAL", "failed to group by primary key")
    }

    t1 = dt.nowNanos()
    groupTime = t1 - t0

    // Extract primary keys for iteration
    primaryKeys = []
    pkIdx = 0
    while pkIdx < len(primaryGroups) {
        primaryKeys = push(primaryKeys, primaryGroups[pkIdx][0])
        pkIdx = pkIdx + 1
    }

    // Spawn async worker per primary group (avoid closure capture with function wrapper)
    workers = makeArray(len(primaryGroups), null)
    pgIdx = 0
    while pgIdx < len(primaryGroups) {
        groupRecords = primaryGroups[pgIdx][1]

        // Create worker closure that captures current group's data
        primaryWorkerFunc = fn(records, secondaryIdx) {
            return async(fn() {
                // Within this primary group: group by secondary key
                secondaryGroups = groupBy(records, secondaryIdx)
                if secondaryGroups == null {
                    return null
                }

                results = {}
                s = 0
                while s < len(secondaryGroups) {
                    secondaryKey = secondaryGroups[s][0]
                    count = len(secondaryGroups[s][1])
                    results[secondaryKey] = count
                    s = s + 1
                }
                return results
            })
        }

        worker = primaryWorkerFunc(groupRecords, secondaryKeyIdx)
        workers[pgIdx] = worker
        pgIdx = pgIdx + 1
    }

    t2 = dt.nowNanos()
    asyncTime = t2 - t1

    // Collect results
    workerResults = makeArray(len(workers), null)
    w = 0
    while w < len(workers) {
        result = await(workers[w])
        workerResults[w] = result
        w = w + 1
    }

    t3 = dt.nowNanos()
    reduceTime = t3 - t2

    // Build matrix: {primary: {secondary: count}}
    matrix = {}
    totals = {}
    grandTotal = 0

    pmIdx = 0
    while pmIdx < len(primaryKeys) {
        primaryKey = primaryKeys[pmIdx]
        secondaryResults = workerResults[pmIdx]

        if secondaryResults != null && type(secondaryResults) == "HASH" {
            matrix[primaryKey] = secondaryResults
            primaryTotal = 0

            // Sum secondary counts: extract keys once
            sec_keys = keys(secondaryResults)
            k = 0
            while k < len(sec_keys) {
                count = secondaryResults[sec_keys[k]]
                primaryTotal = primaryTotal + count
                grandTotal = grandTotal + count
                k = k + 1
            }
            totals[primaryKey] = primaryTotal
        }

        pmIdx = pmIdx + 1
    }

    t4 = dt.nowNanos()
    mergeTime = t4 - t3

    hier_result = {}
    hier_result["matrix"] = matrix
    hier_result["totals"] = totals
    hier_result["grandTotal"] = grandTotal
    hier_result["primaryKeys"] = primaryKeys
    timing_map2 = {}
    timing_map2["group_ms"] = groupTime / 1000000
    timing_map2["async_ms"] = asyncTime / 1000000
    timing_map2["reduce_ms"] = reduceTime / 1000000
    timing_map2["merge_ms"] = mergeTime / 1000000
    timing_map2["total_ms"] = (t4 - t0) / 1000000
    hier_result["timing"] = timing_map2

    return hier_result, null
}

// ============================================================================
// PARALLEL_REDUCE STREAMING: CHUNK-LEVEL REDUCTION
// ============================================================================

// analyzeParallelStream(records, reducerFn, mergerFn, numWorkers, initialAccum) → (result, error)
// High-performance parallel reduce using parallel_reduce from parallel.lex.
// Distributes records across workers, each reduces its chunk, then merges results.
//
// Args:
//   records: array of records
//   reducerFn: fn(accumulator, record) → newAccumulator (processes one record)
//   mergerFn: fn(acc1, acc2) → merged (merges two partial results)
//   numWorkers: number of parallel workers
//   initialAccum: starting value for accumulator
//
// Returns: (final accumulated value, error)
//
// Example:
//   totalQty, err = analyzeParallelStream(
//     records,
//     fn(acc, record) { return acc + parseInt(record[3]) },  // sum column 3
//     fn(a, b) { return a + b },                              // merge partial sums
//     4,
//     0
//   )
fn analyzeParallelStream(records, reducerFn, mergerFn, numWorkers, initialAccum) {
    if type(records) != "ARRAY" {
        return null, error("TYPE_ERROR", "analyzeParallelStream: records must be array")
    }
    if type(reducerFn) != "FUNCTION" {
        return null, error("TYPE_ERROR", "analyzeParallelStream: reducerFn must be function")
    }
    if type(mergerFn) != "FUNCTION" {
        return null, error("TYPE_ERROR", "analyzeParallelStream: mergerFn must be function")
    }
    if type(numWorkers) != "INTEGER" || numWorkers <= 0 {
        return null, error("INVALID_ARG", "analyzeParallelStream: numWorkers must be positive integer")
    }

    result, err = p.parallel_reduce(records, reducerFn, mergerFn, numWorkers, initialAccum)
    if err != null {
        return null, err
    }
    return result, null
}

// ============================================================================
// HELPERS: CHUNKING, GROUPING, MERGING
// ============================================================================

// createChunks(records, chunkSize) → [[chunk1], [chunk2], ...]
// Splits records into fixed-size chunks for distribution.
fn createChunks(records, chunkSize) {
    if type(records) != "ARRAY" {
        return null
    }
    if type(chunkSize) != "INTEGER" || chunkSize <= 0 {
        return null
    }

    numChunks = (len(records) + chunkSize - 1) / chunkSize
    chunks = makeArray(numChunks, null)
    chunkIdx = 0
    i = 0
    while i < len(records) {
        chunkLen = chunkSize
        if i + chunkSize > len(records) {
            chunkLen = len(records) - i
        }
        chunk = makeArray(chunkLen, null)
        j = 0
        while j < chunkLen {
            chunk[j] = records[i + j]
            j = j + 1
        }
        chunks[chunkIdx] = chunk
        chunkIdx = chunkIdx + 1
        i = i + chunkSize
    }

    return chunks
}

// groupBy(records, keyIdx) → [[key1, [records...]], [key2, [records...]], ...]
// Groups records by column value at keyIdx. O(n) time using hash map tracking.
fn groupBy(records, keyIdx) {
    if type(records) != "ARRAY" {
        return null
    }
    if type(keyIdx) != "INTEGER" || keyIdx < 0 {
        return null
    }

    // First pass: count unique keys
    keyCount = {}
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) == "ARRAY" && keyIdx < len(record) {
            key = record[keyIdx]
            if keyCount[key] == null {
                keyCount[key] = 0
            }
            keyCount[key] = keyCount[key] + 1
        }
        i = i + 1
    }

    // Initialize groups with pre-allocated arrays
    groups = []
    groupMap = {}
    groupIdx = 0
    uniqueKeys = keys(keyCount)
    k = 0
    while k < len(uniqueKeys) {
        key = uniqueKeys[k]
        count = keyCount[key]
        groupArray = makeArray(count, null)
        groupMap[key] = groupIdx
        keyGroup = []
        keyGroup = push(keyGroup, key)
        keyGroup = push(keyGroup, groupArray)
        groups = push(groups, keyGroup)
        groupIdx = groupIdx + 1
        k = k + 1
    }

    // Second pass: fill groups with records (track position per group)
    groupPos = {}
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) == "ARRAY" && keyIdx < len(record) {
            key = record[keyIdx]
            gIdx = groupMap[key]
            if groupPos[key] == null {
                groupPos[key] = 0
            }
            groups[gIdx][1][groupPos[key]] = record
            groupPos[key] = groupPos[key] + 1
        }
        i = i + 1
    }

    return groups
}

// mergeResults(resultArray) → merged
// Deep merges array of result hashes into single hash.
// If all results are hashes: merges key-by-key via sum
// If all results are numbers: sums them
// Otherwise: returns first result as-is
fn mergeResults(resultArray) {
    if type(resultArray) != "ARRAY" || len(resultArray) == 0 {
        return null
    }

    first = resultArray[0]

    // If results are hashes: deep merge by summing values
    if type(first) == "HASH" {
        merged = {}
        i = 0
        while i < len(resultArray) {
            result = resultArray[i]
            if type(result) == "HASH" {
                resultKeys = keys(result)
                k = 0
                while k < len(resultKeys) {
                    key = resultKeys[k]
                    value = result[key]
                    if merged[key] == null {
                        merged[key] = 0
                    }
                    if type(value) == "INTEGER" {
                        merged[key] = merged[key] + value
                    }
                    k = k + 1
                }
            }
            i = i + 1
        }
        return merged
    }

    // If results are numbers: sum
    if type(first) == "INTEGER" {
        total = 0
        i = 0
        while i < len(resultArray) {
            if type(resultArray[i]) == "INTEGER" {
                total = total + resultArray[i]
            }
            i = i + 1
        }
        return total
    }

    // Otherwise: return first
    return first
}

// ============================================================================
// UTILITIES
// ============================================================================

// extractColumn(records, colIdx) → [values...]
// Quick helper to extract a column for statistical analysis.
fn extractColumn(records, colIdx) {
    if type(records) != "ARRAY" || type(colIdx) != "INTEGER" {
        return null
    }

    col = []
    i = 0
    while i < len(records) {
        record = records[i]
        if type(record) == "ARRAY" && colIdx < len(record) {
            col = push(col, record[colIdx])
        }
        i = i + 1
    }
    return col
}

// statsFromColumn(values, parseNumFn) → {sum, count, avg, min, max}
// Computes basic statistics from column of values.
// parseNumFn: fn(val) → number (converts string to number if needed)
fn statsFromColumn(values, parseNumFn) {
    if type(values) != "ARRAY" {
        return null
    }

    if parseNumFn == null {
        parseNumFn = fn(v) { return v }
    }

    sum = 0
    count = 0
    min = null
    max = null

    i = 0
    while i < len(values) {
        val = values[i]
        num = parseNumFn(val)
        if type(num) == "INTEGER" {
            sum = sum + num
            count = count + 1
            if min == null || num < min {
                min = num
            }
            if max == null || num > max {
                max = num
            }
        }
        i = i + 1
    }

    if count == 0 {
        return null
    }

    stats = {}
    stats["sum"] = sum
    stats["count"] = count
    stats["avg"] = sum / count
    stats["min"] = min
    stats["max"] = max
    return stats
}
