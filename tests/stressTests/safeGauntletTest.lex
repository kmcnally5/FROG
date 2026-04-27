// =============================================================================
// FROG SAFE GAUNTLET: Isolated Concurrency Test
// =============================================================================

// We avoid shared closures here to prevent the 'concurrent map read/write' panic.
// Every task is self-contained.
fn isolated_worker(id, iterations) {
    let local_sum = 0
    for i in range(iterations) {
        // We do the math directly instead of spawning sub-tasks
        // This keeps the work inside this specific goroutine's scope
        local_sum = local_sum + (i * 10)
    }
    return local_sum
}

// Data crunching is generally safer if the array is local to the function
fn data_crunch(size) {
    let data = []
    for i in range(size) {
        // Creating local maps is fine as long as they aren't shared
        let item = { "id": i, "val": i * 1.5 }
        data = push(data, item)
    }

    let total = 0
    for item in data {
        total = total + item["val"]
    }
    return total
}

// --- EXECUTION ---

println("--- STARTING SAFE GAUNTLET ---")

// 1. Stress the Scheduler with Isolated Tasks
// We will spawn 5,000 tasks, but each is a "island"
println("1. Spawning 5,000 isolated tasks...")
let tasks = []
for i in range(5000) {
    tasks = push(tasks, async(isolated_worker, i, 500))
}

let grand_total = 0
for t in tasks {
    grand_total = grand_total + await(t)
}
println("   Concurrency Total: ", grand_total)

// 2. Stress Memory with Objects
println("2. Crunching 30,000 Complex Objects...")
let crunch_res = data_crunch(30000)
println("   Crunch Result: ", crunch_res)

println("--- SAFE GAUNTLET COMPLETE ---")
