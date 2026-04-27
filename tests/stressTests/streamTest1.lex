fn scan_chunk(start, end) {
    let found = []
    for i in range(start, end) {
        let status = 200
        if i % 50 == 0 { status = 403 }
        if status == 403 {
            found = push(found, { "id": i, "status": status })
        }
    }
    return found
}

fn parallel_scan(limit, threads) {
    let chunk = limit / threads
    let tasks = []

    for i in range(threads) {
        let start = i * chunk
        let end = start + chunk
        tasks = push(tasks, async(scan_chunk, start, end))
    }

    let results = []
    for t in tasks {
        let partial = await(t)
        for item in partial {
            results = push(results, item)
        }
    }
    return results
}

println("--- PARALLEL SCAN ---")
let threats = parallel_scan(1000000, 8)
println("Found:", len(threats), "threats")
