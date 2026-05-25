// ============================================================================
// csvStreamProcessing.lex — Streaming CSV with REAL work per row
// ============================================================================
// The previous test just counted rows (no work). This one processes them:
// - Parse each row
// - Extract fields
// - Perform calculation
// - Accumulate results
//
// This shows where streaming truly helps: when there's real work being done,
// parsing and processing can overlap in background.

import "csv.lex" as csv
import "datetime.lex" as dt

println("=== STREAMING CSV WITH REAL PROCESSING WORK ===")
println("")

// Generate CSV with numeric data for calculations
let generateCSV = fn(size) {
    let rows = makeArray(size + 1, null)
    rows[0] = "ID,FirstName,LastName,Email,Department,Salary,Address,City"
    let i = 1
    while i <= size {
        let id = str(i)
        let fname = "Employee" + str(i)
        let lname = "User" + str(i)
        let email = "emp" + id + "@company.com"
        let dept = ["Sales", "Engineering", "Marketing", "HR", "Finance"][i % 5]
        let salary = str(50000 + (i * 10))
        let addr = "\"" + str(100 + i) + " Street, Suite " + str(i % 100) + "\""
        let city = ["NYC", "SF", "CHI", "SEA", "BOS"][i % 5]
        let row = id + "," + fname + "," + lname + "," + email + "," + dept + "," + salary + "," + addr + "," + city
        rows[i] = row
        i = i + 1
    }
    return join(rows, "\n")
}

println("Setup: Generating 500K row CSV...")
let tSetup = dt.nowNanos()
let csvData = generateCSV(500000)
let tSetupDone = dt.nowNanos()
let setupTime = (tSetupDone - tSetup) / 1000000
println("Done in " + str(setupTime) + " ms")
println("")

// ============================================================================
// Worker that processes rows (extracts fields, does CPU work)
// ============================================================================
let processWorker = fn(workerCh) {
    let checksum = 0
    let count = 0

    while count < 1000000 {
        let row, ok = recv(workerCh)
        if !ok { break }
        if type(row) == "ERROR" { break }

        // Do real work: process each field in the row
        // (In a real app, this might be validation, transformation, aggregation, etc.)
        let i = 0
        while i < len(row) {
            let field = row[i]
            // Process field: count characters as a simple CPU-bound operation
            let j = 0
            while j < len(field) {
                checksum = checksum + len(field)
                j = j + 1
            }
            i = i + 1
        }

        count = count + 1
    }

    return checksum
}

// ============================================================================
// STRATEGY 1: Single worker with streaming
// ============================================================================
println("STRATEGY 1: Single worker processes streaming rows")
println("")

let tStart1 = dt.nowNanos()
let ch1 = csv.stream(csvData, ",")
let result1 = processWorker(ch1)
let tEnd1 = dt.nowNanos()
let time1 = (tEnd1 - tStart1) / 1000000

println("  Time: " + str(time1) + " ms")
println("  Checksum: " + str(result1))
println("")

// ============================================================================
// STRATEGY 2: 4 workers with streaming
// ============================================================================
println("STRATEGY 2: 4 workers process streaming rows in parallel")
println("")

let tStart2 = dt.nowNanos()
let ch2 = csv.stream(csvData, ",")

let t1 = async(fn() { return processWorker(ch2) })
let t2 = async(fn() { return processWorker(ch2) })
let t3 = async(fn() { return processWorker(ch2) })
let t4 = async(fn() { return processWorker(ch2) })

let r1 = await(t1)
let r2 = await(t2)
let r3 = await(t3)
let r4 = await(t4)

let totalResult2 = r1 + r2 + r3 + r4
let tEnd2 = dt.nowNanos()
let time2 = (tEnd2 - tStart2) / 1000000

println("  Time: " + str(time2) + " ms")
println("  Checksum: " + str(totalResult2))
println("")

// ============================================================================
// STRATEGY 3: 10 workers with streaming
// ============================================================================
println("STRATEGY 3: 10 workers process streaming rows in parallel")
println("")

let tStart3 = dt.nowNanos()
let ch3 = csv.stream(csvData, ",")

t1 = async(fn() { return processWorker(ch3) })
t2 = async(fn() { return processWorker(ch3) })
t3 = async(fn() { return processWorker(ch3) })
t4 = async(fn() { return processWorker(ch3) })
let t5 = async(fn() { return processWorker(ch3) })
let t6 = async(fn() { return processWorker(ch3) })
let t7 = async(fn() { return processWorker(ch3) })
let t8 = async(fn() { return processWorker(ch3) })
let t9 = async(fn() { return processWorker(ch3) })
let t10 = async(fn() { return processWorker(ch3) })

r1 = await(t1)
r2 = await(t2)
r3 = await(t3)
r4 = await(t4)
let r5 = await(t5)
let r6 = await(t6)
let r7 = await(t7)
let r8 = await(t8)
let r9 = await(t9)
let r10 = await(t10)

let totalResult3 = r1 + r2 + r3 + r4 + r5 + r6 + r7 + r8 + r9 + r10
let tEnd3 = dt.nowNanos()
let time3 = (tEnd3 - tStart3) / 1000000

println("  Time: " + str(time3) + " ms")
println("  Checksum: " + str(totalResult3))
println("")

// ============================================================================
// COMPARISON
// ============================================================================
println("=== RESULTS ===")
println("")
println("Single worker: " + str(time1) + " ms")
println("4 workers:    " + str(time2) + " ms  (" + str(time1 / time2) + "x speedup)")
println("10 workers:   " + str(time3) + " ms  (" + str(time1 / time3) + "x speedup)")
println("")

let speedup4 = time1 / time2
let speedup10 = time1 / time3

if speedup4 > 3.5 {
    println("✓ EXCELLENT: Near-linear scaling with streaming")
} else if speedup4 > 2.5 {
    println("✓ VERY GOOD: Strong parallelism (parsing + processing overlapping)")
} else if speedup4 > 1.8 {
    println("✓ GOOD: Solid speedup — streaming architecture is working")
} else {
    println("⚠ WEAK: Channel contention limiting parallel benefit")
}
println("")
