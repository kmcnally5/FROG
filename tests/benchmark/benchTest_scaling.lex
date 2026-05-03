// ============================================================
// STRESSTEST5: SCALING ANALYSIS
// ============================================================
// Measure throughput and efficiency across worker counts.
// Identifies: task overhead, scheduling limits, memory bandwidth.
//
// Interpretation:
// - Linear scaling → memory bandwidth-limited (good)
// - Sub-linear → task overhead or scheduler contention
// - Collapse → lock contention or merge bottleneck

import "parallel.lex" as p
import "datetime.lex" as dt

fn elapsed(startNanos) {
    let endNanos = dt.nowNanos()
    let elapsedNanos = endNanos - startNanos
    return elapsedNanos / 1000000000.0
}

// ============================================================
// SCALING TEST: Parsing workload
// ============================================================
fn scaling_parse(workerCounts) {
    println("\n============================================================")
    println("SCALING: PARSING + TRANSFORMATION")
    println("============================================================")
    println("Workers | Time (sec) | Throughput (rec/sec) | Efficiency")
    println("--------|------------|---------------------|------------")

    let n = 1000000

    for w in range(len(workerCounts)) {
        let workers = workerCounts[w]
        let arr = makeArray(n, 0)

        for i in range(n) {
            arr[i] = i
        }

        let start = dt.nowNanos()

        result, err = p.parallel_reduce(
            arr,
            fn(acc, elem) {
                let value = (elem * 7) % 100
                if value > 10 {
                    acc = acc + 1
                }
                return acc
            },
            fn(a, b) { a + b },
            workers,
            0
        )

        let t = elapsed(start)
        let throughput = n / t
        let efficiency = (throughput / (n / 0.425)) * 100

        let ws = str(workers)
        let ts = str(t)
        let tps = str(throughput)
        let eff = str(efficiency)

        println(ws + "      | " + ts + "    | " + tps + " | " + eff + "%")

        if err != null {
            println("ERROR:", err)
        }
    }
}

// ============================================================
// SCALING TEST: Aggregation (heavier workload)
// ============================================================
fn scaling_agg(workerCounts) {
    println("\n============================================================")
    println("SCALING: AGGREGATION (10M elements)")
    println("============================================================")
    println("Workers | Time (sec) | Throughput (elem/sec) | Efficiency")
    println("--------|------------|----------------------|------------")

    let n = 10000000
    let arr = makeArray(n, 0)

    for i in range(n) {
        arr[i] = (i * 73 + 17) % 1000000
    }

    for w in range(len(workerCounts)) {
        let workers = workerCounts[w]

        let start = dt.nowNanos()

        sum, err = p.parallel_reduce(
            arr,
            fn(acc, x) { acc + x },
            fn(a, b) { a + b },
            workers,
            0
        )

        let t = elapsed(start)
        let throughput = n / t
        let baseline = n / 4.73
        let efficiency = (throughput / baseline) * 100

        let ws = str(workers)
        let ts = str(t)
        let tps = str(throughput)
        let eff = str(efficiency)

        println(ws + "      | " + ts + "    | " + tps + " | " + eff + "%")

        if err != null {
            println("ERROR:", err)
        }
    }
}

// ============================================================
// MERGE PHASE ANALYSIS: Many small reduces
// ============================================================
fn scaling_merge(workerCounts) {
    println("\n============================================================")
    println("SCALING: MANY SMALL REDUCES (fine-grained merge stress)")
    println("============================================================")
    println("Workers | Time (sec) | Throughput (op/sec) | Efficiency")
    println("--------|------------|---------------------|------------")

    let ops = 100000
    let arr = makeArray(ops, 0)

    for i in range(ops) {
        arr[i] = i % 1000
    }

    for w in range(len(workerCounts)) {
        let workers = workerCounts[w]

        let start = dt.nowNanos()

        result, err = p.parallel_reduce(
            arr,
            fn(acc, x) { if x > 500 { acc + 1 } else { acc } },
            fn(a, b) { a + b },
            workers,
            0
        )

        let t = elapsed(start)
        let throughput = ops / t
        let baseline = ops / 0.015
        let efficiency = (throughput / baseline) * 100

        let ws = str(workers)
        let ts = str(t)
        let tps = str(throughput)
        let eff = str(efficiency)

        println(ws + "      | " + ts + "      | " + tps + "        | " + eff + "%")

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
    println("STRESSTEST5: SCALING ANALYSIS")
    println("============================================================")
    println("Identifying bottlenecks: task overhead vs merge contention")
    println("")

    let workerCounts = [1, 2, 4, 8, 16]

    scaling_parse(workerCounts)
    scaling_agg(workerCounts)
    scaling_merge(workerCounts)

    println("\n============================================================")
    println("ANALYSIS COMPLETE")
    println("============================================================")
    println("")
    println("Interpretation guide:")
    println("- Linear scaling (e.g., 2x workers → 2x throughput) → good")
    println("- Sub-linear (e.g., 2x workers → 1.5x throughput) → overhead")
    println("- Collapse at high worker count → merge bottleneck or lock contention")
    println("")
}

main()
