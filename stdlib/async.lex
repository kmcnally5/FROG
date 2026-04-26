// stdlib/async.lex — async utilities
//
// Provides higher-level helpers built on top of the async/await/sleep builtins.
//
// Usage:
//   import "async.lex" as a
//   results = a.await_all([t1, t2, t3])

// await_all(tasks) — await an array of tasks and return an array of their results.
// All tasks run concurrently; await_all collects results in the order the tasks
// were passed, not the order they finish.
//
// Example:
//   t1 = async(fn() { sleep(200)  return "slow" })
//   t2 = async(fn() { sleep(50)   return "fast" })
//   results = await_all([t1, t2])
//   println(results[0])   // "slow"
//   println(results[1])   // "fast"
fn await_all(tasks) {
    results = []
    for t in tasks {
        results = push(results, await(t))
    }
    return results
}

fn parallel(tasks) {
    return await_all(tasks)
}
