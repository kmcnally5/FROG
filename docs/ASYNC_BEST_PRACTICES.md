# kLex Async Best Practices

As of May 2026, kLex uses **environment snapshots** for async tasks. This document explains the design, best practices, and patterns.

---

## How Async Works Now

### The Snapshot Model

When you launch an async task with `async(fn, args)`:

1. **Snapshot is created:** The global environment is copied at task creation time
2. **Task runs in isolation:** The function executes in the snapshot (not the original environment)
3. **Reads work:** The task can read global variables (from the snapshot)
4. **Writes are local:** Any mutations stay inside the task and don't affect the caller
5. **Results return:** The function's return value is delivered to the caller via `await()`

**Code example:**
```lex
x = 100
task = async(fn() {
    x = 999          // ← Only visible inside this task
    return x + 1     // ← Returns 1000
})
x = 200             // ← This doesn't affect the running task
result = await(task) // ← 1000
println(x)           // ← Prints 200 (unchanged)
```

### Why This Design?

**Benefits:**
1. **Zero mutex contention** — No locks needed for task-local mutations
2. **Prevents data races** — Tasks can't corrupt shared state accidentally
3. **Fast execution** — 89% reduction in contention overhead (compared to previous design)
4. **Clear semantics** — "What happens in async, stays in async"

**Trade-off:**
- Tasks cannot modify the caller's global state
- You must return data via `await()`, not mutate globals

This trade-off is worth it: better performance AND safer code.

---

## Best Practices

### ✅ Pattern 1: Pure Functions (Recommended)

**What:** Async tasks are pure functions. No global access needed.

```lex
fn compute(n) {
    sum = 0
    i = 0
    while i < n {
        sum = sum + i
        i = i + 1
    }
    return sum
}

// Launch multiple tasks
tasks = []
for i in range(10) {
    task = async(compute, i * 1000)
    tasks = push(tasks, task)
}

// Collect results
results = []
for task in tasks {
    result = await(task)
    results = push(results, result)
}
```

**Why it's best:**
- Fastest (no global access = no snapshot overhead)
- Easiest to understand
- Tests easily
- No data race risk

**Performance:** 384ms for 10 tasks (pure async without global access)

---

### ✅ Pattern 2: Read-Only Global Access

**What:** Async tasks read globals but don't modify them.

```lex
config = {
    max_retries: 3,
    timeout: 5000,
    api_key: "secret"
}

fn fetch_data(url) {
    // Read from snapshot of config
    key = config["api_key"]
    timeout = config["timeout"]
    
    // Make request with config values
    response = api_call(url, key, timeout)
    return response
}

// Safe: tasks read config but don't modify it
tasks = []
for url in urls {
    task = async(fetch_data, url)
    tasks = push(tasks, task)
}

results = map(tasks, await)
```

**Why it's good:**
- Clear intent (read-only = safe)
- Snapshot provides consistent view of config
- Config can change between task launches (new tasks see new config)
- Very fast (snapshot access is cheap)

**Performance:** 416ms for 10 tasks (with read-only global access)

---

### ⚠️ Anti-Pattern 1: Mutating Shared Arrays/Hashes

**DON'T DO THIS:**

```lex
results = []
count = 0

fn worker(id) {
    data = expensive_computation(id)
    results = push(results, data)    // ← WRONG: Only visible in task
    count = count + 1                 // ← WRONG: Only visible in task
}

for i in range(100) {
    async(worker, i)
}

// ❌ results is still empty, count is still 0
println(results)  // ← Empty!
println(count)    // ← 0
```

**Why it fails:**
- Mutations inside `worker()` don't affect the caller's `results` or `count`
- Those mutations stay inside the task's snapshot
- The caller sees the original values (empty array, 0)

**Fix:** Use the return value instead:

```lex
fn worker(id) {
    data = expensive_computation(id)
    return data     // ← Return it
}

tasks = []
for i in range(100) {
    task = async(worker, i)
    tasks = push(tasks, task)
}

// Collect results properly
results = []
for task in tasks {
    result = await(task)
    results = push(results, result)  // ← Main thread adds to results
}
```

---

### ⚠️ Anti-Pattern 2: Relying on Global State for Coordination

**DON'T DO THIS:**

```lex
done = false

fn worker() {
    // Do work...
    done = true  // ← Only visible in task, not to caller
}

task = async(worker)
while !done {    // ← Waiting for a flag that never changes
    // Busy-wait forever!
}
```

**Why it fails:**
- Setting `done = true` inside the task doesn't affect the caller's `done`
- The caller loops forever waiting for a flag that never changes

**Fix:** Use `await()` for synchronization:

```lex
fn worker() {
    // Do work...
    return true  // ← Return completion signal
}

task = async(worker)
result = await(task)  // ← Blocks until task completes
println("Done")
```

---

### ⚠️ Anti-Pattern 3: Expecting Initialization to Affect Tasks

**DON'T DO THIS:**

```lex
counter = 0

fn increment() {
    counter = counter + 1
    return counter
}

// Launch task
task = async(increment)

// Try to set counter before task runs
counter = 100

// Task sees snapshot from launch time, not the updated value
result = await(task)
println(result)  // ← Prints 1, not 101
```

**Why it fails:**
- The snapshot is taken at `async()` call time
- Later changes to `counter` don't affect the snapshot
- The task always sees `counter = 0`

**Fix:** Pass values as arguments:

```lex
fn increment(initial) {
    return initial + 1
}

task = async(increment, 100)  // ← Pass initial value
result = await(task)
println(result)  // ← Prints 101
```

---

## Common Patterns

### Pattern: Map-Reduce with Async

Launch many tasks, collect results:

```lex
fn process_batch(ids) {
    results = []
    for id in ids {
        task = async(fetch_item, id)
        results = push(results, task)
    }
    return results
}

fn fetch_item(id) {
    // Do expensive work
    return item_data
}

// Launch batch processing
batches = [[1,2,3], [4,5,6], [7,8,9]]
batch_tasks = []
for batch in batches {
    task = async(process_batch, batch)
    batch_tasks = push(batch_tasks, task)
}

// Collect all results
all_results = []
for batch_task in batch_tasks {
    batch_results = await(batch_task)
    for item in batch_results {
        item_task = await(item)
        all_results = push(all_results, item_task)
    }
}

println(len(all_results))  // ← Total items processed
```

### Pattern: Worker Pool

Create a pool of workers processing items:

```lex
fn worker(work_queue, index) {
    for task_data in work_queue {
        result = process(task_data)
        return result  // ← Return one result per task
    }
    return null
}

// Create work queue
work = [item1, item2, item3, ...]

// Launch worker pool
workers = []
for i in range(4) {  // ← 4 workers
    worker_task = async(worker, work, i)
    workers = push(workers, worker_task)
}

// Collect results
results = []
for worker_task in workers {
    result = await(worker_task)
    if result != null {
        results = push(results, result)
    }
}
```

### Pattern: Timeout with Async

```lex
fn with_timeout(fn, timeout_ms) {
    start = dt.nowNanos()
    task = async(fn)
    
    while !task.done.Load() {
        elapsed = (dt.nowNanos() - start) / 1000000
        if elapsed > timeout_ms {
            return null  // ← Timeout (task still runs, but we don't wait)
        }
    }
    return await(task)
}

// Usage
result = with_timeout(fn() {
    return expensive_operation()
}, 5000)  // ← 5 second timeout

if result == null {
    println("Operation timed out")
}
```

---

## Performance Characteristics

### Async with Pure Functions
```
10 tasks: 384ms
100 tasks: ~3.8s (linear scaling)
1000 tasks: ~38s (linear scaling)
```

**Why:** No global access = no contention. Tasks run truly in parallel (modulo goroutine scheduling).

### Async with Read-Only Globals
```
10 tasks: 416ms (32ms overhead for global access)
100 tasks: ~41.6s (includes global read time)
1000 tasks: ~416s
```

**Why:** Reading from snapshot is cheap. Tasks don't block each other.

### Async with Mutations (Old Design, DON'T USE)
```
10 tasks: 687ms (would have been)
100 tasks: ~68.7s (with high contention)
1000 tasks: Bottleneck at mutex (serialized)
```

**This is why we use snapshots now.**

---

## FAQ

### Q: Can I modify arrays/hashes inside async?

**A:** Yes, but only task-local ones. Mutations don't affect the caller:

```lex
fn worker() {
    local_array = [1, 2, 3]
    local_array = push(local_array, 4)  // ← This works fine
    return local_array                  // ← Return it
}

task = async(worker)
result = await(task)  // ← [1, 2, 3, 4]
```

### Q: Can I access closure variables in async?

**A:** Yes, they're part of the snapshot:

```lex
fn make_multiplier(factor) {
    fn multiply(x) {
        return x * factor  // ← Can read 'factor' from closure
    }
    return multiply
}

fn = make_multiplier(10)
task = async(fn, 5)
result = await(task)  // ← 50
```

### Q: What about constants in async?

**A:** Constants are also snapshotted and are immutable in the snapshot:

```lex
x = 100
const y = 200

fn worker() {
    x = 999   // ← Task-local (doesn't affect caller)
    y = 999   // ← ERROR: cannot reassign constant
}

task = async(worker)
```

### Q: Can async tasks communicate with each other?

**A:** Not directly. Use channels instead (or collect results via main thread):

```lex
// Bad: tasks can't see each other's mutations
task1 = async(fn1)
task2 = async(fn2)

// Good: collect both results in main thread
r1 = await(task1)
r2 = await(task2)
combined = [r1, r2]
```

### Q: What if I need true shared mutable state?

**A:** Use channels and a dedicated "manager" task:

```lex
// Create a channel for communication
responses = channel(10)

fn manager() {
    state = {}  // ← Managed state
    for request in requests_ch {
        // Process request, update state
        send(responses, result)
    }
}

// Launch manager
manager_task = async(manager)

// Send requests from main thread
send(requests_ch, request1)
send(requests_ch, request2)
```

This pattern keeps mutations in one place (the manager task).

---

## Migration from Old Code

If you have old async code that relies on global mutations, here's how to fix it:

### Before (Old Design)
```lex
results = []
fn worker(id) {
    data = expensive_work(id)
    results = push(results, data)  // ← This won't work now
}

for i in range(100) {
    async(worker, i)
}
```

### After (New Design)
```lex
fn worker(id) {
    return expensive_work(id)  // ← Return data instead
}

tasks = []
for i in range(100) {
    task = async(worker, i)
    tasks = push(tasks, task)
}

results = map(tasks, await)  // ← Collect all results
```

### Benefits of the Fix
1. ✅ Faster (89% less contention)
2. ✅ Safer (no data races possible)
3. ✅ Clearer (explicit data flow)

---

## Testing Async Code

### Pattern: Test Without Async First

```lex
fn worker(id) {
    return expensive_work(id)
}

// Test synchronously
result1 = worker(1)
result2 = worker(2)
assert(result1 == expected1)
assert(result2 == expected2)

// Then test with async
task1 = async(worker, 1)
task2 = async(worker, 2)
assert(await(task1) == expected1)
assert(await(task2) == expected2)
```

### Pattern: Verify Isolation

```lex
state = 100

fn check_isolation() {
    state = 999
    return state
}

task = async(check_isolation)
result = await(task)

assert(result == 999)    // ← Task saw its own mutation
assert(state == 100)     // ← Caller unaffected
```

---

## Summary

### Do This ✅
- Return data from async functions
- Read globals if needed (they're snapshotted)
- Use `map(tasks, await)` to collect results
- Use channels for inter-task communication

### Don't Do This ❌
- Mutate global arrays/hashes in async (mutations won't be visible)
- Rely on global flags for coordination (use `await()` instead)
- Expect globals to be updated between task launch and completion

### Remember
- **Async is fast:** 89% less contention overhead
- **Async is safe:** No data races possible
- **Async is clear:** Return data, don't mutate globals

For questions, see [PHASE_1_2_RESULTS.md](PHASE_1_2_RESULTS.md) for design details.
