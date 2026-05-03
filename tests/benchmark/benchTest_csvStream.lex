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
generateCSV = fn(size) {
    rows = makeArray(size + 1, null)
    rows[0] = "ID,FirstName,LastName,Email,Department,Salary,Address,City"
    i = 1
    while i <= size {
        id = str(i)
        fname = "Employee" + str(i)
        lname = "User" + str(i)
        email = "emp" + id + "@company.com"
        dept = ["Sales", "Engineering", "Marketing", "HR", "Finance"][i % 5]
        salary = str(50000 + (i * 10))
        addr = "\"" + str(100 + i) + " Street, Suite " + str(i % 100) + "\""
        city = ["NYC", "SF", "CHI", "SEA", "BOS"][i % 5]
        row = id + "," + fname + "," + lname + "," + email + "," + dept + "," + salary + "," + addr + "," + city
        rows[i] = row
        i = i + 1
    }
    return join(rows, "\n")
}

println("Setup: Generating 500K row CSV...")
tSetup = dt.nowNanos()
csvData = generateCSV(500000)
tSetupDone = dt.nowNanos()
setupTime = (tSetupDone - tSetup) / 1000000
println("Done in " + str(setupTime) + " ms")
println("")

// ============================================================================
// Worker that processes rows (extracts fields, does CPU work)
// ============================================================================
processWorker = fn(workerCh) {
    checksum = 0
    count = 0

    while count < 1000000 {
        row, ok = recv(workerCh)
        if !ok { break }
        if type(row) == "ERROR" { break }

        // Do real work: process each field in the row
        // (In a real app, this might be validation, transformation, aggregation, etc.)
        i = 0
        while i < len(row) {
            field = row[i]
            // Process field: count characters as a simple CPU-bound operation
            j = 0
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

tStart1 = dt.nowNanos()
ch1 = csv.stream(csvData, ",")
result1 = processWorker(ch1)
tEnd1 = dt.nowNanos()
time1 = (tEnd1 - tStart1) / 1000000

println("  Time: " + str(time1) + " ms")
println("  Checksum: " + str(result1))
println("")

// ============================================================================
// STRATEGY 2: 4 workers with streaming
// ============================================================================
println("STRATEGY 2: 4 workers process streaming rows in parallel")
println("")

tStart2 = dt.nowNanos()
ch2 = csv.stream(csvData, ",")

t1 = async(fn() { return processWorker(ch2) })
t2 = async(fn() { return processWorker(ch2) })
t3 = async(fn() { return processWorker(ch2) })
t4 = async(fn() { return processWorker(ch2) })

r1 = await(t1)
r2 = await(t2)
r3 = await(t3)
r4 = await(t4)

totalResult2 = r1 + r2 + r3 + r4
tEnd2 = dt.nowNanos()
time2 = (tEnd2 - tStart2) / 1000000

println("  Time: " + str(time2) + " ms")
println("  Checksum: " + str(totalResult2))
println("")

// ============================================================================
// STRATEGY 3: 10 workers with streaming
// ============================================================================
println("STRATEGY 3: 10 workers process streaming rows in parallel")
println("")

tStart3 = dt.nowNanos()
ch3 = csv.stream(csvData, ",")

t1 = async(fn() { return processWorker(ch3) })
t2 = async(fn() { return processWorker(ch3) })
t3 = async(fn() { return processWorker(ch3) })
t4 = async(fn() { return processWorker(ch3) })
t5 = async(fn() { return processWorker(ch3) })
t6 = async(fn() { return processWorker(ch3) })
t7 = async(fn() { return processWorker(ch3) })
t8 = async(fn() { return processWorker(ch3) })
t9 = async(fn() { return processWorker(ch3) })
t10 = async(fn() { return processWorker(ch3) })

r1 = await(t1)
r2 = await(t2)
r3 = await(t3)
r4 = await(t4)
r5 = await(t5)
r6 = await(t6)
r7 = await(t7)
r8 = await(t8)
r9 = await(t9)
r10 = await(t10)

totalResult3 = r1 + r2 + r3 + r4 + r5 + r6 + r7 + r8 + r9 + r10
tEnd3 = dt.nowNanos()
time3 = (tEnd3 - tStart3) / 1000000

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

speedup4 = time1 / time2
speedup10 = time1 / time3

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
