# Async Quick Reference

Quick cheat sheet for using async in kLex.

---

## One-Line Summary

**async launches a task in an isolated environment. Return data via `await()`, don't mutate globals.**

---

## Basic Pattern

```lex
// 1. Define a function
fn worker(x) {
    return expensive_work(x)  // ← Return data
}

// 2. Launch tasks
tasks = []
for i in range(10) {
    task = async(worker, i)
    tasks = push(tasks, task)
}

// 3. Collect results
results = map(tasks, await)
```

---

## The Two Rules

### Rule 1: Return Data, Don't Mutate Globals

```lex
❌ WRONG:
fn worker(id) {
    global_list = push(global_list, id)  // ← Won't work
}

✅ RIGHT:
fn worker(id) {
    return id  // ← Return it
}
result = await(async(worker, 1))
```

### Rule 2: Use `await()` to Get Results

```lex
❌ WRONG:
value = 0
task = async(fn() { value = 100 })
println(value)  // ← Still 0

✅ RIGHT:
task = async(fn() { return 100 })
value = await(task)
println(value)  // ← 100
```

---

## Common Patterns

### Parallel Execution
```lex
tasks = []
for item in items {
    task = async(process, item)
    tasks = push(tasks, task)
}
results = map(tasks, await)
```

### Sequential Collection
```lex
result1 = await(async(fn1, arg1))
result2 = await(async(fn2, arg2))
result3 = await(async(fn3, arg3))
```

### Fan-Out/Fan-In
```lex
fn fetch(url) { return http_get(url) }

urls = [url1, url2, url3]
tasks = map(urls, fn(u) { return async(fetch, u) })
responses = map(tasks, await)
```

---

## What You Can Do in Async

✅ **Read globals** — They're snapshotted  
✅ **Return data** — Via return value  
✅ **Use local variables** — They're isolated  
✅ **Call functions** — Closures work fine  
✅ **Read function arguments** — Passed normally  

❌ **Don't mutate global arrays/hashes** — Changes are local only  
❌ **Don't mutate global scalars** — Changes are local only  
❌ **Don't use globals for coordination** — Use `await()` instead  

---

## Performance

| Pattern | Speed | Notes |
|---------|-------|-------|
| Pure async | 384ms/10 tasks | No globals = fastest |
| Read-only globals | 416ms/10 tasks | Snapshot is cheap to read |
| Old design | 687ms/10 tasks | (Don't use: high contention) |

**89% faster with snapshot model**

---

## FAQ

**Q: Why can't I mutate globals in async?**
A: Mutations stay in the task's snapshot. Return data instead.

**Q: Can I pass data via globals?**
A: Read-only yes (it's snapshotted). Mutations no (return the data).

**Q: How do I synchronize tasks?**
A: Use `await()` or channels. Globals don't work.

**Q: Can tasks communicate?**
A: No shared memory. Use channels or main thread for coordination.

**Q: What's the snapshot?**
A: A copy of globals taken when `async()` is called. Tasks see that copy.

---

## Side-by-Side Comparison

### ❌ OLD PATTERN (Don't Use)
```lex
results = []
fn worker(id) {
    results = push(results, compute(id))  // Slow, doesn't work
}
for i in range(100) {
    async(worker, i)
}
```

### ✅ NEW PATTERN (Use This)
```lex
fn worker(id) {
    return compute(id)  // Fast, works correctly
}
tasks = []
for i in range(100) {
    tasks = push(tasks, async(worker, i))
}
results = map(tasks, await)
```

---

## Remember

1. **Return data from async functions**
2. **Use `await()` to collect results**
3. **Don't mutate globals in async**
4. **Read globals if needed (they're snapshotted)**
5. **It's safe** — No data races possible

For full documentation, see [ASYNC_BEST_PRACTICES.md](ASYNC_BEST_PRACTICES.md)
