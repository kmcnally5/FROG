// ============================================================
// STRESSTEST7: TASK GRANULARITY OPTIMIZATION
// ============================================================
// Test different task granularities to find the sweet spot.
// Hypothesis: coarser tasks (fewer goroutines) → better CPU cache locality
//
// Metric: throughput at fixed array size
// Vary: items_per_task (coarser = higher, finer = lower)

import "parallel.lex" as p
import "datetime.lex" as dt

fn elapsed(startNanos) {
    let endNanos = dt.nowNanos()
    let elapsedNanos = endNanos - startNanos
    return elapsedNanos / 1000000000.0
}

// ============================================================
// Granularity test: vary task size for fixed array
// ============================================================
fn test_granularity(arraySize, granularities) {
    println("\n============================================================")
    println("GRANULARITY TEST: " + str(arraySize) + " items")
    println("============================================================")
    println("Items/Task | Workers | Time (sec) | Throughput (items/sec)")
    println("-----------|---------|-----------|----------------------")

    let arr = makeArray(arraySize, 0)
    for i in range(arraySize) {
        arr[i] = (i * 73 + 17) % 1000000
    }

    for g in range(len(granularities)) {
        let itemsPerTask = granularities[g]
        let workers = arraySize / itemsPerTask

        if workers < 1 { workers = 1 }
        if workers > 32 { workers = 32 }

        let start = dt.nowNanos()

        sum, err = p.parallel_reduce(
            arr,
            fn(acc, x) { acc + x },
            fn(a, b) { a + b },
            workers,
            0
        )

        let t = elapsed(start)
        let throughput = arraySize / t

        let gstr = str(itemsPerTask)
        let wstr = str(workers)
        let tstr = str(t)
        let tpstr = str(throughput)

        println(gstr + "       | " + wstr + "       | " + tstr + "  | " + tpstr)

        if err != null {
            println("ERROR:", err)
        }
    }
}

// ============================================================
// RUNNER
// ============================================================
fn main() {
    println("\n============================================================")
    println("STRESSTEST7: TASK GRANULARITY")
    println("============================================================")
    println("Find optimal items_per_task: finer ← → coarser")
    println("")

    // Fine-grained: 100k items
    // Granularities: 1k, 2.5k, 5k, 10k, 25k, 50k, 100k
    test_granularity(100000, [
        1000,
        2500,
        5000,
        10000,
        25000,
        50000,
        100000
    ])

    // Coarse-grained: 10M items
    // Granularities: 50k, 100k, 250k, 500k, 1M, 2.5M, 5M, 10M
    test_granularity(10000000, [
        50000,
        100000,
        250000,
        500000,
        1000000,
        2500000,
        5000000,
        10000000
    ])

    println("\n============================================================")
    println("GRANULARITY TEST COMPLETE")
    println("============================================================")
    println("")
    println("Interpretation:")
    println("- Peak throughput → optimal granularity for this hardware")
    println("- Left of peak: overhead dominates (too many goroutines)")
    println("- Right of peak: parallelism wasted (too few workers)")
    println("")
}

main()
